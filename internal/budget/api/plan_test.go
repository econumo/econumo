package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// decEq compares two decimal strings as normalized values (sqlite float
// rendering vs stored text differ in trailing digits, never in value).
func decEq(a, b string) bool {
	return vo.NewDecimal(a).Sub(vo.NewDecimal(b)).IsZero()
}

func getPlan(t *testing.T, h *harness, tok, query string) (int, envelope) {
	t.Helper()
	return h.do(t, http.MethodGet, "/api/v1/budget/get-budget-plan?"+query, tok, nil)
}

func planItem(t *testing.T, env envelope) model.BudgetPlanResult {
	t.Helper()
	res := mustUnmarshal[model.GetBudgetPlanResult](t, env.Data)
	return res.Item
}

// TestGetBudgetPlan_SkeletonShape: months list, meta, openingBalances,
// per-month currencyRates and folders — the response frame Tasks 3-4 fill.
func TestGetBudgetPlan_SkeletonShape(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Plan Budget", "currencyId": usdID, "startDate": "2024-04-01"})

	const folderID2 = "bfff2222-0000-7000-8000-0000000000b0"
	h.do(t, http.MethodPost, "/api/v1/budget/create-folder", tok, map[string]any{
		"budgetId": budgetID1, "id": folderID2, "name": "Bills",
	})

	st, env := getPlan(t, h, tok, "id="+budgetID1+"&from=2024-04-15&months=3")
	if st != http.StatusOK {
		t.Fatalf("get-budget-plan = %d; body=%s", st, env.raw)
	}
	item := planItem(t, env)

	// from snaps to first-of-month; months are ordered date strings.
	wantMonths := []string{"2024-04-01", "2024-05-01", "2024-06-01"}
	if len(item.Months) != 3 || item.Months[0] != wantMonths[0] || item.Months[1] != wantMonths[1] || item.Months[2] != wantMonths[2] {
		t.Fatalf("months = %v, want %v", item.Months, wantMonths)
	}
	if item.Meta.Id != budgetID1 {
		t.Errorf("meta.id = %q", item.Meta.Id)
	}
	// One included USD account with no transactions before the window: one
	// opening balance row, zero-valued.
	if len(item.OpeningBalances) != 1 || item.OpeningBalances[0].CurrencyId != usdID || !decEq(item.OpeningBalances[0].Amount, "0") {
		t.Errorf("openingBalances = %+v", item.OpeningBalances)
	}
	if len(item.CurrencyRates) != 3 {
		t.Fatalf("currencyRates length = %d, want 3", len(item.CurrencyRates))
	}
	for i, cr := range item.CurrencyRates {
		if cr.Period != wantMonths[i] {
			t.Errorf("currencyRates[%d].period = %q want %q", i, cr.Period, wantMonths[i])
		}
		if cr.Rates == nil {
			t.Errorf("currencyRates[%d].rates must be [] not null", i)
		}
	}
	if len(item.Structure.Folders) != 1 || item.Structure.Folders[0].Id != folderID2 || item.Structure.Folders[0].Position != 0 {
		t.Errorf("folders = %+v", item.Structure.Folders)
	}
	if item.Structure.Elements == nil {
		t.Errorf("structure.elements must be [] not null")
	}
}

// TestGetBudgetPlan_Validation: id blank; months non-integer / out of 1..24
// rejected with the frozen invalid-choice message; empty months defaults to 12.
func TestGetBudgetPlan_Validation(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Plan Bounds Budget"))

	st, env := getPlan(t, h, tok, "from=2024-04-01&months=3")
	if st != http.StatusBadRequest {
		t.Fatalf("missing id = %d; body=%s", st, env.raw)
	}
	if msgs := env.errorsMap()["id"]; len(msgs) == 0 || msgs[0] != "This value should not be blank." {
		t.Errorf("id error = %v", env.errorsMap())
	}

	for _, months := range []string{"0", "25", "-1", "junk", "1.5"} {
		st, env = getPlan(t, h, tok, "id="+budgetID1+"&months="+months)
		if st != http.StatusBadRequest {
			t.Fatalf("months=%s -> %d, want 400; body=%s", months, st, env.raw)
		}
		if msgs := env.errorsMap()["months"]; len(msgs) == 0 || msgs[0] != "The value you selected is not a valid choice." {
			t.Errorf("months=%s error = %v", months, env.errorsMap())
		}
	}

	st, env = getPlan(t, h, tok, "id="+budgetID1)
	if st != http.StatusOK {
		t.Fatalf("default window = %d; body=%s", st, env.raw)
	}
	if item := planItem(t, env); len(item.Months) != 12 {
		t.Errorf("default months = %d, want 12", len(item.Months))
	}
}

// TestGetBudgetPlan_ExpenseCells: envelope parent/child cells, tag rows, the
// standalone category, the Uncategorized row, planned-from-limit cells and
// month alignment — all in one window.
func TestGetBudgetPlan_ExpenseCells(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	const rentCatID = "cccc2222-0000-7000-8000-0000000000c0"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Category(fixture.Category{ID: rentCatID, UserID: seedUserID, Name: "Rent", Type: 0, Icon: "home"})

	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Cells Budget", "currencyId": usdID, "startDate": "2024-04-01"})

	// Envelope over the seeded expense category catID ("Food").
	const envID = "beee2222-0000-7000-8000-0000000000c0"
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": envID, "name": "Living", "icon": "cart",
		"currencyId": usdID, "folderId": nil, "categories": []string{catID},
	})
	if st != http.StatusOK {
		t.Fatalf("create-envelope = %d; body=%s", st, env.raw)
	}

	// Transactions: Food in Apr + May, tagged expense in May, uncategorized in
	// May, Rent untouched (still visible: non-archived standalone).
	f.Transaction(fixture.Transaction{ID: "d0002222-0000-7000-8000-000000000001", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, Type: 0, Amount: "40.00000000", SpentAt: "2024-04-10 10:00:00"})
	f.Transaction(fixture.Transaction{ID: "d0002222-0000-7000-8000-000000000002", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, Type: 0, Amount: "60.00000000", SpentAt: "2024-05-02 09:00:00"})
	f.Transaction(fixture.Transaction{ID: "d0002222-0000-7000-8000-000000000003", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, TagID: tagID, Type: 0, Amount: "15.00000000", SpentAt: "2024-05-20 12:00:00"})
	f.Transaction(fixture.Transaction{ID: "d0002222-0000-7000-8000-000000000004", UserID: seedUserID, AccountID: accountID,
		Type: 0, Amount: "5.00000000", SpentAt: "2024-05-21 12:00:00"})

	// Limits: envelope Apr=100, May=120; Rent May=300.
	for _, l := range []struct{ id, period, amount string }{
		{envID, "2024-04-01", "100"}, {envID, "2024-05-01", "120"}, {rentCatID, "2024-05-01", "300"},
	} {
		st, env = h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
			"budgetId": budgetID1, "elementId": l.id, "period": l.period, "amount": l.amount,
		})
		if st != http.StatusOK {
			t.Fatalf("set-limit %s %s = %d; body=%s", l.id, l.period, st, env.raw)
		}
	}

	st, env = getPlan(t, h, tok, "id="+budgetID1+"&from=2024-04-01&months=3")
	if st != http.StatusOK {
		t.Fatalf("get-budget-plan = %d; body=%s", st, env.raw)
	}
	item := planItem(t, env)

	byID := map[string]model.PlanElementResult{}
	for _, el := range item.Structure.Elements {
		byID[el.Id] = el
	}

	// Envelope: parent actual = child (Food) totals; planned from its limits.
	envRow, ok := byID[envID]
	if !ok {
		t.Fatalf("envelope row missing; elements=%s", env.Data)
	}
	if len(envRow.Cells) != 3 {
		t.Fatalf("envelope cells = %d, want 3", len(envRow.Cells))
	}
	for i, want := range []struct{ actual, planned string }{
		{"40", "100"}, {"60", "120"}, {"0", ""},
	} {
		c := envRow.Cells[i]
		if !decEq(c.Actual, want.actual) {
			t.Errorf("envelope cell[%d].actual = %q want %q", i, c.Actual, want.actual)
		}
		if want.planned == "" && c.Planned != "" {
			t.Errorf("envelope cell[%d].planned = %q want empty", i, c.Planned)
		}
		if want.planned != "" && !decEq(c.Planned, want.planned) {
			t.Errorf("envelope cell[%d].planned = %q want %q", i, c.Planned, want.planned)
		}
	}
	if len(envRow.Children) != 1 || envRow.Children[0].Id != catID {
		t.Fatalf("envelope children = %+v", envRow.Children)
	}
	if !decEq(envRow.Children[0].Cells[0].Actual, "40") || !decEq(envRow.Children[0].Cells[1].Actual, "60") {
		t.Errorf("child cells = %+v", envRow.Children[0].Cells)
	}

	// Tag row: leaf, May actual 15 (the tagged row belongs to the tag, not Food).
	tagRow, ok := byID[tagID]
	if !ok {
		t.Fatalf("tag row missing")
	}
	if len(tagRow.Children) != 0 {
		t.Errorf("tag rows are leaves in the plan; children=%+v", tagRow.Children)
	}
	if !decEq(tagRow.Cells[1].Actual, "15") {
		t.Errorf("tag May actual = %q", tagRow.Cells[1].Actual)
	}

	// Standalone non-archived category with no activity still shows, planned May=300.
	rentRow, ok := byID[rentCatID]
	if !ok {
		t.Fatalf("standalone category row missing")
	}
	if !decEq(rentRow.Cells[1].Planned, "300") || !decEq(rentRow.Cells[0].Actual, "0") {
		t.Errorf("rent cells = %+v", rentRow.Cells)
	}

	// Uncategorized: actual-only May=5, planned always "".
	uncat, ok := byID[model.UncategorizedID]
	if !ok {
		t.Fatalf("uncategorized row missing")
	}
	if uncat.Type != 1 {
		t.Errorf("uncategorized type = %d want 1", uncat.Type)
	}
	if !decEq(uncat.Cells[1].Actual, "5") || uncat.Cells[1].Planned != "" {
		t.Errorf("uncategorized cells = %+v", uncat.Cells)
	}

	// Food must NOT appear standalone (it lives inside the envelope).
	if _, dup := byID[catID]; dup {
		t.Errorf("envelope child must not surface as a standalone row")
	}
}

// TestGetBudgetPlan_EnvelopeCategoryDedup: a category claimed by two envelopes
// (the write path never enforces cross-envelope uniqueness) must surface as a
// child of exactly one — the first-owning envelope, matching where its
// spending is filed — never doubled, and never standalone.
func TestGetBudgetPlan_EnvelopeCategoryDedup(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Dedup Budget", "currencyId": usdID, "startDate": "2024-04-01"})

	const envAID = "beee3333-0000-7000-8000-0000000000d0"
	const envBID = "beee4444-0000-7000-8000-0000000000d1"
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": envAID, "name": "Living A", "icon": "cart",
		"currencyId": usdID, "folderId": nil, "categories": []string{catID},
	})
	if st != http.StatusOK {
		t.Fatalf("create-envelope A = %d; body=%s", st, env.raw)
	}
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": envBID, "name": "Living B", "icon": "cart",
		"currencyId": usdID, "folderId": nil, "categories": []string{},
	})
	if st != http.StatusOK {
		t.Fatalf("create-envelope B = %d; body=%s", st, env.raw)
	}
	// The write path accepts a category already claimed by another envelope —
	// nothing enforces cross-envelope uniqueness.
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/update-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": envBID, "name": "Living B", "icon": "cart",
		"currencyId": usdID, "isArchived": 0, "categories": []string{catID},
	})
	if st != http.StatusOK {
		t.Fatalf("update-envelope B = %d; body=%s", st, env.raw)
	}

	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Transaction(fixture.Transaction{ID: "d0003333-0000-7000-8000-000000000001", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, Type: 0, Amount: "40.00000000", SpentAt: "2024-04-10 10:00:00"})

	st, env = getPlan(t, h, tok, "id="+budgetID1+"&from=2024-04-01&months=1")
	if st != http.StatusOK {
		t.Fatalf("get-budget-plan = %d; body=%s", st, env.raw)
	}
	item := planItem(t, env)

	byID := map[string]model.PlanElementResult{}
	for _, el := range item.Structure.Elements {
		byID[el.Id] = el
	}

	envA, ok := byID[envAID]
	if !ok {
		t.Fatalf("envelope A missing; elements=%+v", item.Structure.Elements)
	}
	if len(envA.Children) != 1 || envA.Children[0].Id != catID {
		t.Fatalf("envelope A children = %+v, want exactly [catID]", envA.Children)
	}
	if !decEq(envA.Children[0].Cells[0].Actual, "40") {
		t.Errorf("envelope A child actual = %q want 40", envA.Children[0].Cells[0].Actual)
	}
	if !decEq(envA.Cells[0].Actual, "40") {
		t.Errorf("envelope A actual = %q want 40", envA.Cells[0].Actual)
	}

	envB, ok := byID[envBID]
	if !ok {
		t.Fatalf("envelope B missing; elements=%+v", item.Structure.Elements)
	}
	if len(envB.Children) != 0 {
		t.Errorf("envelope B children = %+v, want none (category claimed by A)", envB.Children)
	}
	if !decEq(envB.Cells[0].Actual, "0") {
		t.Errorf("envelope B actual = %q want 0", envB.Cells[0].Actual)
	}

	if _, dup := byID[catID]; dup {
		t.Errorf("category must not surface as a standalone row")
	}
}

// TestGetBudgetPlan_RequiresMembership: a non-member gets the standard 403.
func TestGetBudgetPlan_RequiresMembership(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Private Budget"))

	const strangerID = "99992222-0000-7000-8000-0000000000aa"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.User(fixture.User{ID: strangerID, Email: "stranger@example.test", Name: "Stranger", Password: "pw"})

	st, env := getPlan(t, h, strangerID, "id="+budgetID1+"&months=3")
	if st != http.StatusForbidden {
		t.Fatalf("stranger = %d, want 403; body=%s", st, env.raw)
	}
	_ = json.RawMessage(env.raw)
}

// TestGetBudgetPlan_IncomeRows: income envelope with child cells, standalone
// income category, income-side Uncategorized, planned income from set-limit.
func TestGetBudgetPlan_IncomeRows(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	const salaryCatID = "cccc2222-0000-7000-8000-0000000000c1"
	const bonusCatID = "cccc2222-0000-7000-8000-0000000000c2"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Category(fixture.Category{ID: salaryCatID, UserID: seedUserID, Name: "Salary", Type: 1, Icon: "payments"})
	f.Category(fixture.Category{ID: bonusCatID, UserID: seedUserID, Name: "Bonus", Type: 1, Icon: "star"})

	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Income Plan Budget", "currencyId": usdID, "startDate": "2024-04-01"})

	const incomeEnvID = "beee2222-0000-7000-8000-0000000000c1"
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": incomeEnvID, "name": "Salaries", "icon": "payments",
		"currencyId": usdID, "folderId": nil, "side": "income", "categories": []string{salaryCatID},
	})
	if st != http.StatusOK {
		t.Fatalf("create income envelope = %d; body=%s", st, env.raw)
	}

	// Income transactions: salary Apr+May, bonus May, category-less June.
	f.Transaction(fixture.Transaction{ID: "d0003222-0000-7000-8000-000000000001", UserID: seedUserID, AccountID: accountID,
		CategoryID: salaryCatID, Type: 1, Amount: "1000.00000000", SpentAt: "2024-04-05 08:00:00"})
	f.Transaction(fixture.Transaction{ID: "d0003222-0000-7000-8000-000000000002", UserID: seedUserID, AccountID: accountID,
		CategoryID: salaryCatID, Type: 1, Amount: "1100.00000000", SpentAt: "2024-05-05 08:00:00"})
	f.Transaction(fixture.Transaction{ID: "d0003222-0000-7000-8000-000000000003", UserID: seedUserID, AccountID: accountID,
		CategoryID: bonusCatID, Type: 1, Amount: "200.00000000", SpentAt: "2024-05-15 08:00:00"})
	f.Transaction(fixture.Transaction{ID: "d0003222-0000-7000-8000-000000000004", UserID: seedUserID, AccountID: accountID,
		Type: 1, Amount: "50.00000000", SpentAt: "2024-06-06 08:00:00"})

	// Planned income: envelope May=1500, standalone bonus May=250.
	for _, l := range []struct{ id, amount string }{{incomeEnvID, "1500"}, {bonusCatID, "250"}} {
		st, env = h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
			"budgetId": budgetID1, "elementId": l.id, "period": "2024-05-01", "amount": l.amount,
		})
		if st != http.StatusOK {
			t.Fatalf("set-limit %s = %d; body=%s", l.id, st, env.raw)
		}
	}

	st, env = getPlan(t, h, tok, "id="+budgetID1+"&from=2024-04-01&months=3")
	if st != http.StatusOK {
		t.Fatalf("get-budget-plan = %d; body=%s", st, env.raw)
	}
	item := planItem(t, env)

	rows := map[string][]model.PlanElementResult{}
	for _, el := range item.Structure.Elements {
		rows[el.Id] = append(rows[el.Id], el)
	}

	// Income envelope: type 4, child salary cells, planned May=1500.
	envRows := rows[incomeEnvID]
	if len(envRows) != 1 || envRows[0].Type != 4 {
		t.Fatalf("income envelope rows = %+v", envRows)
	}
	er := envRows[0]
	if !decEq(er.Cells[0].Actual, "1000") || !decEq(er.Cells[1].Actual, "1100") || !decEq(er.Cells[1].Planned, "1500") {
		t.Errorf("income envelope cells = %+v", er.Cells)
	}
	if len(er.Children) != 1 || er.Children[0].Id != salaryCatID || er.Children[0].Type != 3 {
		t.Fatalf("income envelope children = %+v", er.Children)
	}
	if !decEq(er.Children[0].Cells[1].Actual, "1100") {
		t.Errorf("salary child cells = %+v", er.Children[0].Cells)
	}

	// Standalone income category: type 3, May actual 200 + planned 250.
	bonusRows := rows[bonusCatID]
	if len(bonusRows) != 1 || bonusRows[0].Type != 3 {
		t.Fatalf("bonus rows = %+v", bonusRows)
	}
	if !decEq(bonusRows[0].Cells[1].Actual, "200") || !decEq(bonusRows[0].Cells[1].Planned, "250") {
		t.Errorf("bonus cells = %+v", bonusRows[0].Cells)
	}
	// Salary must not ALSO appear standalone.
	if len(rows[salaryCatID]) != 0 {
		t.Errorf("income envelope child must not surface standalone")
	}

	// TWO Uncategorized rows can coexist (same id, distinguished by type).
	var uncatTypes []int
	for _, u := range rows[model.UncategorizedID] {
		uncatTypes = append(uncatTypes, u.Type)
	}
	if len(uncatTypes) != 1 || uncatTypes[0] != 3 {
		// only the income one exists here (no uncategorized EXPENSE in this test)
		t.Fatalf("uncategorized rows types = %v, want [3]", uncatTypes)
	}
	for _, u := range rows[model.UncategorizedID] {
		if u.Type == 3 && !decEq(u.Cells[2].Actual, "50") {
			t.Errorf("income uncategorized cells = %+v", u.Cells)
		}
	}
}

// TestGetBudgetPlan_DirtyCrossSideLink: a stored income-category link on an
// EXPENSE envelope (pre-migration residue, unreachable via the API) must not
// trap the category — it renders as a standalone income row and the envelope
// shows no such child.
func TestGetBudgetPlan_DirtyCrossSideLink(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	const salaryCatID = "cccc2222-0000-7000-8000-0000000000c3"
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.Category(fixture.Category{ID: salaryCatID, UserID: seedUserID, Name: "Salary Dirty", Type: 1, Icon: "payments"})

	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Dirty Link Budget", "currencyId": usdID, "startDate": "2024-04-01"})

	const envID = "beee2222-0000-7000-8000-0000000000c3"
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": envID, "name": "Expenses", "icon": "cart",
		"currencyId": usdID, "folderId": nil, "categories": []string{},
	})
	if st != http.StatusOK {
		t.Fatalf("create-envelope = %d; body=%s", st, env.raw)
	}
	// Force the dirty link past the write-path validation.
	if _, err := h.db.Exec(`INSERT INTO budgets_envelopes_categories (budget_envelope_id, category_id) VALUES (?, ?)`, envID, salaryCatID); err != nil {
		t.Fatalf("force dirty link: %v", err)
	}

	st, env = getPlan(t, h, tok, "id="+budgetID1+"&from=2024-04-01&months=2")
	if st != http.StatusOK {
		t.Fatalf("get-budget-plan = %d; body=%s", st, env.raw)
	}
	item := planItem(t, env)
	var sawSalaryStandalone, sawSalaryAsChild bool
	for _, el := range item.Structure.Elements {
		if el.Id == salaryCatID && el.Type == 3 {
			sawSalaryStandalone = true
		}
		for _, ch := range el.Children {
			if ch.Id == salaryCatID {
				sawSalaryAsChild = true
			}
		}
	}
	if !sawSalaryStandalone || sawSalaryAsChild {
		t.Fatalf("dirty link: standalone=%v asChild=%v; want standalone income row only. body=%s",
			sawSalaryStandalone, sawSalaryAsChild, env.Data)
	}
}
