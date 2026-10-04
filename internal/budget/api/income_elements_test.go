package api_test

import (
	"database/sql"
	"net/http"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// elementTypeAndKey reads the persisted element row for an external id. Takes
// the raw *sql.DB (pass h.db) so it does not depend on the harness type name.
func elementTypeAndKey(t *testing.T, db *sql.DB, budgetID, externalID string) (typ int, sortKey string, found bool) {
	t.Helper()
	err := db.QueryRow(
		`SELECT type, sort_key FROM budgets_elements WHERE budget_id = ? AND external_id = ?`,
		budgetID, externalID,
	).Scan(&typ, &sortKey)
	if err != nil {
		return 0, "", false
	}
	return typ, sortKey, true
}

// seedIncomeEnvelope creates a type=4 (income) envelope through create-envelope
// with side=income, which validates the children against the income side.
func seedIncomeEnvelope(t *testing.T, h *harness, tok, budgetID, envID, name string, categories []string) {
	t.Helper()
	if categories == nil {
		categories = []string{}
	}
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, map[string]any{
		"budgetId": budgetID, "id": envID, "name": name, "icon": "payments",
		"currencyId": usdID, "folderId": nil, "side": "income", "categories": categories,
	})
	if st != http.StatusOK {
		t.Fatalf("seed income envelope = %d; body=%s", st, env.raw)
	}
}

func limitCount(t *testing.T, db *sql.DB, externalID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM budgets_elements_limits l JOIN budgets_elements e ON l.element_id = e.id WHERE e.external_id = ?`,
		externalID,
	).Scan(&n); err != nil {
		t.Fatalf("limit count: %v", err)
	}
	return n
}

// TestSetLimit_IncomeCategorySelfHeals: setting a planned income value on an
// income category with no element row must create a type=3 row via the
// syncElements self-heal — and the row plus its limit must SURVIVE a later
// envelope write (the reconciler's delete-unseen pass is the regression here).
func TestSetLimit_IncomeCategorySelfHeals_AndSurvivesSync(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Income Reconciler Budget"))

	const incomeCatID = "cccc2222-0000-7000-8000-0000000000aa"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Category(fixture.Category{ID: incomeCatID, UserID: seedUserID, Name: "Salary Later", Type: 1, Icon: "payments"})

	st, env := h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
		"budgetId": budgetID1, "elementId": incomeCatID, "period": "2099-01-01", "amount": "3000",
	})
	if st != http.StatusOK {
		t.Fatalf("set-limit on an income category = %d, want 200; body=%s", st, env.raw)
	}
	typ, _, found := elementTypeAndKey(t, h.db, budgetID1, incomeCatID)
	if !found || typ != 3 {
		t.Fatalf("self-healed income element: found=%v type=%d, want type=3", found, typ)
	}

	// Any element-mutating write reruns syncElements. Its delete-unseen pass
	// must keep the income element and its limit.
	const envID = "beee2222-0000-7000-8000-0000000000aa"
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": envID, "name": "Sync Env", "icon": "cart",
		"currencyId": usdID, "folderId": nil, "categories": []string{},
	})
	if st != http.StatusOK {
		t.Fatalf("create-envelope = %d; body=%s", st, env.raw)
	}
	typ, _, found = elementTypeAndKey(t, h.db, budgetID1, incomeCatID)
	if !found || typ != 3 {
		t.Fatalf("income element after sync: found=%v type=%d, want it kept with type=3", found, typ)
	}
	if n := limitCount(t, h.db, incomeCatID); n != 1 {
		t.Fatalf("planned income limits after sync = %d, want 1 (cascade delete = data loss)", n)
	}
}

// TestSyncElements_KeepsEnvelopeStoredType: a type=4 (income) envelope element
// must not be re-ensured as expense (duplicate row -> UNIQUE violation) nor
// deleted+recreated.
func TestSyncElements_KeepsEnvelopeStoredType(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Stored Side Budget"))

	const envID = "beee2222-0000-7000-8000-0000000000ab"
	seedIncomeEnvelope(t, h, tok, budgetID1, envID, "Salaries", nil)

	// move-element on an unrelated element reruns syncElements.
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/move-element", tok, map[string]any{
		"budgetId": budgetID1, "id": envID, "folderId": nil, "afterId": nil,
	})
	if st != http.StatusOK {
		t.Fatalf("move-element = %d; body=%s", st, env.raw)
	}
	typ, _, found := elementTypeAndKey(t, h.db, budgetID1, envID)
	if !found || typ != 4 {
		t.Fatalf("envelope stored type after sync: found=%v type=%d, want 4 (side is immutable)", found, typ)
	}
	var rows int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM budgets_elements WHERE budget_id = ? AND external_id = ?`, budgetID1, envID).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("element rows for envelope = %d (err=%v), want exactly 1", rows, err)
	}
}

// TestCreateBudget_SeedsIncomeCategories: a pre-existing income category gets a
// live type=3 element at budget creation — and get-budget must not show it
// (the frozen contract).
func TestCreateBudget_SeedsIncomeCategories(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	const incomeCatID = "cccc2222-0000-7000-8000-0000000000ab"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Category(fixture.Category{ID: incomeCatID, UserID: seedUserID, Name: "Wages", Type: 1, Icon: "payments"})

	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Seeded Income Budget"))

	typ, sortKey, found := elementTypeAndKey(t, h.db, budgetID1, incomeCatID)
	if !found {
		t.Fatalf("income category must be seeded at budget creation")
	}
	if typ != 3 {
		t.Errorf("seeded type=%d want 3", typ)
	}
	if sortKey == "" {
		t.Errorf("live income element must carry a sort key")
	}

	st, b := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, tok, nil)
	if st != http.StatusOK {
		t.Fatalf("get-budget=%d body=%s", st, b.raw)
	}
	if strings.Contains(string(b.Data), incomeCatID) {
		t.Errorf("get-budget must not surface the income category anywhere; body=%s", b.Data)
	}
}

// TestMoveElement_CrossSideRejected: a folder's side is fixed at creation, so an
// element of the other side is refused with the coded error -- by an empty
// folder too.
func TestMoveElement_CrossSideRejected(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	const incomeCatID = "cccc2222-0000-7000-8000-0000000000ac"
	const expenseCatID = "cccc2222-0000-7000-8000-0000000000ad"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Category(fixture.Category{ID: incomeCatID, UserID: seedUserID, Name: "Wages Move", Type: 1, Icon: "payments"})
	f.Category(fixture.Category{ID: expenseCatID, UserID: seedUserID, Name: "Rent Move", Type: 0, Icon: "home"})
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Folder Sides Budget"))

	const expenseFolderID = "bfff2222-0000-7000-8000-0000000000aa"
	const incomeFolderID = "bfff2222-0000-7000-8000-0000000000a9"
	h.do(t, http.MethodPost, "/api/v1/budget/create-folder", tok, map[string]any{
		"budgetId": budgetID1, "id": expenseFolderID, "name": "Bills", "side": "expense",
	})
	h.do(t, http.MethodPost, "/api/v1/budget/create-folder", tok, map[string]any{
		"budgetId": budgetID1, "id": incomeFolderID, "name": "Earnings", "side": "income",
	})

	move := func(id, folderID string) (int, envelope) {
		return h.do(t, http.MethodPost, "/api/v1/budget/move-element", tok, map[string]any{
			"budgetId": budgetID1, "id": id, "folderId": folderID, "afterId": nil,
		})
	}
	refused := func(what string, st int, env envelope) {
		t.Helper()
		if st != http.StatusBadRequest || !strings.Contains(string(env.raw), "A folder cannot contain both income and expenses") {
			t.Fatalf("%s: st=%d body=%s (want the folder side-mixing error)", what, st, env.raw)
		}
	}

	st, env := move(incomeCatID, expenseFolderID)
	refused("income into an empty expense folder", st, env)
	st, env = move(expenseCatID, incomeFolderID)
	refused("expense into an empty income folder", st, env)

	if st, env = move(incomeCatID, incomeFolderID); st != http.StatusOK {
		t.Fatalf("income into income folder = %d; body=%s", st, env.raw)
	}
	if st, env = move(expenseCatID, expenseFolderID); st != http.StatusOK {
		t.Fatalf("expense into expense folder = %d; body=%s", st, env.raw)
	}
	st, env = move(incomeCatID, expenseFolderID)
	refused("income from its folder into the expense folder", st, env)
}

// TestCreateEnvelope_IncomeSide covers the envelope-side matrix: side=income
// stores a type=4 element holding its income child, homogeneity rejections both
// directions, unknown category rejected, invalid side rejected, cross-side
// folder placement rejected at create time, and update keeping the stored side.
func TestCreateEnvelope_IncomeSide(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	const incomeCatID = "cccc2222-0000-7000-8000-0000000000ae"
	const expenseCatID = "cccc2222-0000-7000-8000-0000000000af"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Category(fixture.Category{ID: incomeCatID, UserID: seedUserID, Name: "Wages Env", Type: 1, Icon: "payments"})
	f.Category(fixture.Category{ID: expenseCatID, UserID: seedUserID, Name: "Rent Env", Type: 0, Icon: "home"})
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Envelope Sides Budget"))

	envBody := func(id, side string, cats []string, folderID any) map[string]any {
		return map[string]any{
			"budgetId": budgetID1, "id": id, "name": "Env " + id[:4], "icon": "cart",
			"currencyId": usdID, "folderId": folderID, "side": side, "categories": cats,
		}
	}

	const incomeEnvID = "beee2222-0000-7000-8000-0000000000b0"
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, envBody(incomeEnvID, "income", []string{incomeCatID}, nil))
	if st != http.StatusOK || !strings.Contains(string(env.Data), incomeCatID) {
		t.Fatalf("create income envelope: st=%d body=%s", st, env.raw)
	}
	if typ, _, _ := elementTypeAndKey(t, h.db, budgetID1, incomeEnvID); typ != 4 {
		t.Fatalf("income envelope element type=%d want 4", typ)
	}

	// Homogeneity, the other way: an expense child in an income envelope.
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok,
		envBody("beee2222-0000-7000-8000-0000000000b1", "income", []string{expenseCatID}, nil))
	if st != http.StatusBadRequest || !strings.Contains(string(env.raw), "An envelope cannot contain both income and expense categories") {
		t.Fatalf("expense child in income envelope: st=%d body=%s", st, env.raw)
	}

	// Homogeneity: income child in a (default expense) envelope.
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok,
		envBody("beee2222-0000-7000-8000-0000000000b2", "", []string{incomeCatID}, nil))
	if st != http.StatusBadRequest || !strings.Contains(string(env.raw), "An envelope cannot contain both income and expense categories") {
		t.Fatalf("income child in expense envelope: st=%d body=%s", st, env.raw)
	}

	// Existence/ownership: unknown category id.
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok,
		envBody("beee2222-0000-7000-8000-0000000000b3", "", []string{"9999aaaa-0000-7000-8000-00000000dead"}, nil))
	if st != http.StatusBadRequest || !strings.Contains(string(env.raw), "Category not found") {
		t.Fatalf("unknown child category: st=%d body=%s", st, env.raw)
	}

	// Invalid side alias.
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok,
		envBody("beee2222-0000-7000-8000-0000000000b4", "both", []string{}, nil))
	if st != http.StatusBadRequest {
		t.Fatalf("invalid side: st=%d body=%s", st, env.raw)
	}

	// Cross-side folder placement at create time: put the income category into
	// a folder (income-sided now), then try to create a default (expense)
	// envelope inside it.
	const folderID = "bfff2222-0000-7000-8000-0000000000ab"
	h.do(t, http.MethodPost, "/api/v1/budget/create-folder", tok, map[string]any{
		"budgetId": budgetID1, "id": folderID, "name": "Income", "side": "income",
	})
	h.do(t, http.MethodPost, "/api/v1/budget/move-element", tok, map[string]any{
		"budgetId": budgetID1, "id": incomeCatID, "folderId": folderID, "afterId": nil,
	})
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok,
		envBody("beee2222-0000-7000-8000-0000000000b5", "", []string{}, folderID))
	if st != http.StatusBadRequest || !strings.Contains(string(env.raw), "A folder cannot contain both income and expenses") {
		t.Fatalf("expense envelope into income folder: st=%d body=%s", st, env.raw)
	}

	// An income envelope goes into the income folder.
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok,
		envBody("beee2222-0000-7000-8000-0000000000b6", "income", []string{}, folderID))
	if st != http.StatusOK {
		t.Fatalf("income envelope into income folder: st=%d body=%s", st, env.raw)
	}

	// The income envelope keeps its stored side on update: an income child is
	// accepted and rendered, an expense child is still rejected (side is
	// immutable, no side field on update).
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/update-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": incomeEnvID, "name": "Salaries", "icon": "payments",
		"currencyId": usdID, "isArchived": 0, "categories": []string{incomeCatID},
	})
	if st != http.StatusOK {
		t.Fatalf("update income envelope with income child = %d; body=%s", st, env.raw)
	}
	if !strings.Contains(string(env.Data), incomeCatID) {
		t.Errorf("income child must be rendered in the result; body=%s", env.Data)
	}
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/update-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": incomeEnvID, "name": "Salaries", "icon": "payments",
		"currencyId": usdID, "isArchived": 0, "categories": []string{expenseCatID},
	})
	if st != http.StatusBadRequest || !strings.Contains(string(env.raw), "An envelope cannot contain both income and expense categories") {
		t.Fatalf("update income envelope with expense child: st=%d body=%s", st, env.raw)
	}
}

// TestGetBudget_ExcludesIncomeEnvelopesAndFolders: an income envelope never
// renders in get-budget, and neither does an income folder -- empty, holding
// the envelope, or emptied again.
func TestGetBudget_ExcludesIncomeEnvelopesAndFolders(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	const incomeCatID = "cccc2222-0000-7000-8000-0000000000b6"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Category(fixture.Category{ID: incomeCatID, UserID: seedUserID, Name: "Wages GB", Type: 1, Icon: "payments"})
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "GB Exclusion Budget"))

	const incomeEnvID = "beee2222-0000-7000-8000-0000000000b7"
	seedIncomeEnvelope(t, h, tok, budgetID1, incomeEnvID, "Salaries GB", []string{incomeCatID})

	const folderID = "bfff2222-0000-7000-8000-0000000000ac"
	h.do(t, http.MethodPost, "/api/v1/budget/create-folder", tok, map[string]any{
		"budgetId": budgetID1, "id": folderID, "name": "Income Folder", "side": "income",
	})
	absent := func(when string) {
		t.Helper()
		st, b := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, tok, nil)
		if st != http.StatusOK {
			t.Fatalf("get-budget=%d body=%s", st, b.raw)
		}
		for _, id := range []string{folderID, incomeEnvID, incomeCatID} {
			if strings.Contains(string(b.Data), id) {
				t.Fatalf("%s: %s must not render in get-budget; body=%s", when, id, b.Data)
			}
		}
	}
	absent("empty income folder")

	st, env := h.do(t, http.MethodPost, "/api/v1/budget/move-element", tok, map[string]any{
		"budgetId": budgetID1, "id": incomeEnvID, "folderId": folderID, "afterId": nil,
	})
	if st != http.StatusOK {
		t.Fatalf("move income envelope into folder = %d; body=%s", st, env.raw)
	}
	absent("income folder holding the envelope")

	h.do(t, http.MethodPost, "/api/v1/budget/move-element", tok, map[string]any{
		"budgetId": budgetID1, "id": incomeEnvID, "folderId": nil, "afterId": nil,
	})
	absent("emptied income folder")
}
