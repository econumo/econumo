package api_test

import (
	"database/sql"
	"net/http"
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
// deleted+recreated. We force the stored type via SQL because create-envelope
// only grows the side field in a later task — this pins the reconciler
// independently of the write path.
func TestSyncElements_KeepsEnvelopeStoredType(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Stored Side Budget"))

	const envID = "beee2222-0000-7000-8000-0000000000ab"
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": envID, "name": "Salaries", "icon": "payments",
		"currencyId": usdID, "folderId": nil, "categories": []string{},
	})
	if st != http.StatusOK {
		t.Fatalf("create-envelope = %d; body=%s", st, env.raw)
	}
	if _, err := h.db.Exec(`UPDATE budgets_elements SET type = 4 WHERE budget_id = ? AND external_id = ?`, budgetID1, envID); err != nil {
		t.Fatalf("force income type: %v", err)
	}

	// move-element on an unrelated element reruns syncElements.
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/move-element", tok, map[string]any{
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
