package api_test

import (
	"database/sql"
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// Savings budget elements (type 5): one row per savings member, kept by
// the lazy element sync. These tests observe the rows through budgets_elements
// directly (get-budget/get-budget-plan's own savings surface is covered by
// savings_budget_test.go and savings_plan_test.go) and read the expense side
// through get-budget.

const (
	savingsAccountID  = "aaaa1111-0000-7000-8000-00000000005a"
	savingsAccountID2 = "aaaa1111-0000-7000-8000-00000000005b"
	savingsFolderID   = "ffffffff-0000-7000-8000-00000000005f"
	savingsCatID2     = "cccc1111-0000-7000-8000-00000000005c"
)

type savingsRow struct {
	ID, ExternalID, SortKey string
	CurrencyID, FolderID    sql.NullString
}

// savingsRows lists the budget's savings element rows, key-ordered.
func savingsRows(t *testing.T, h *harness) []savingsRow {
	t.Helper()
	rows, err := h.db.Query(`SELECT id, external_id, sort_key, currency_id, folder_id FROM budgets_elements
		WHERE budget_id = ? AND type = 5 ORDER BY sort_key, external_id`, budgetID1)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []savingsRow
	for rows.Next() {
		var r savingsRow
		if err := rows.Scan(&r.ID, &r.ExternalID, &r.SortKey, &r.CurrencyID, &r.FolderID); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// flagSavings sets a member's per-budget savings flag directly, standing in
// for the write paths that own it.
func flagSavings(t *testing.T, db *dbtest.DB, budgetID, accountID string, on bool) {
	t.Helper()
	res, err := db.Raw.Exec(db.Rebind(`UPDATE budgets_accounts SET is_savings = ? WHERE budget_id = ? AND account_id = ?`), on, budgetID, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		t.Fatalf("flagSavings(%s, %s): %d membership rows updated, want 1", budgetID, accountID, n)
	}
}

func savingsIDs(rows []savingsRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ExternalID
	}
	return out
}

// noFolderOrder is the budget's ungrouped get-budget elements, in wire order.
func noFolderOrder(t *testing.T, h *harness, tok string) []string {
	t.Helper()
	env := h.mustDo(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2026-08-15", tok, nil)
	view := mustUnmarshal[elementOrderView](t, env.Data)
	out := []string{}
	for _, e := range view.Item.Structure.Elements {
		if e.FolderId == nil {
			out = append(out, e.Id)
		}
	}
	return out
}

func equalIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// newSavingsHarness: the seeded regular account plus the given accounts, all
// members of budgetID1 (started 2026-06-01, now 2026-08-17) and flagged
// savings there.
func newSavingsHarness(t *testing.T, savingsIDs ...string) (*harness, string, string) {
	t.Helper()
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	eur := h.f.Currency(fixture.Currency{Code: "EUR", Symbol: "€", Name: "Euro"})
	for _, id := range savingsIDs {
		h.f.Account(fixture.Account{ID: id, UserID: seedUserID, CurrencyID: eur, Name: "Rainy day", Icon: "savings"})
	}
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{
		"id": budgetID1, "name": "Budget", "currencyId": usdID, "startDate": "2026-06-01",
		"accountIds": append([]string{accountID}, savingsIDs...),
	})
	for _, id := range savingsIDs {
		flagSavings(t, h.tdb, budgetID1, id, true)
	}
	return h, tok, eur
}

func (h *harness) moveElement(t *testing.T, tok string, body map[string]any) (int, envelope) {
	t.Helper()
	body["budgetId"] = budgetID1
	return h.do(t, http.MethodPost, "/api/v1/budget/move-element", tok, body)
}

// syncBudget runs a budget write that reconciles the element rows: re-placing
// the seeded category where it already is.
func (h *harness) syncBudget(t *testing.T, tok string) {
	t.Helper()
	order := noFolderOrder(t, h, tok)
	var after any
	for i, id := range order {
		if id == catID && i > 0 {
			after = order[i-1]
		}
	}
	if st, env := h.moveElement(t, tok, map[string]any{"id": catID, "folderId": nil, "afterId": after}); st != http.StatusOK {
		t.Fatalf("move-element (sync): status=%d body=%s", st, env.raw)
	}
}

func TestSavingsSync_OneRowPerSavingsMember(t *testing.T) {
	h, tok, eur := newSavingsHarness(t, savingsAccountID)
	h.syncBudget(t, tok)

	rows := savingsRows(t, h)
	if len(rows) != 1 {
		t.Fatalf("savings rows = %+v, want exactly one (the regular account gets none)", rows)
	}
	r := rows[0]
	if r.ExternalID != savingsAccountID {
		t.Errorf("external_id = %q, want the savings account id", r.ExternalID)
	}
	if !r.CurrencyID.Valid || r.CurrencyID.String != eur {
		t.Errorf("currency_id = %+v, want the account's currency %s", r.CurrencyID, eur)
	}
	if r.FolderID.Valid {
		t.Errorf("folder_id = %q, want NULL", r.FolderID.String)
	}
	if r.SortKey == "" {
		t.Error("sort key is empty, want a live key")
	}
	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_elements WHERE budget_id = ? AND external_id = ?`, budgetID1, accountID).Scan(&n)
	if n != 0 {
		t.Errorf("regular account has %d element rows, want 0", n)
	}
}

func TestSavingsSync_SetLimitSelfHeals(t *testing.T) {
	h, tok, _ := newSavingsHarness(t, savingsAccountID)
	// No sync has run yet: create-budget seeds only categories and tags.
	if rows := savingsRows(t, h); len(rows) != 0 {
		t.Fatalf("savings rows before any write = %+v, want none", rows)
	}
	h.mustDo(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
		"budgetId": budgetID1, "elementId": savingsAccountID, "period": "2026-06-01", "amount": "250",
	})
	rows := savingsRows(t, h)
	if len(rows) != 1 || rows[0].ExternalID != savingsAccountID {
		t.Fatalf("savings rows = %+v, want the self-healed row", rows)
	}
	var amount string
	if err := h.db.QueryRow(`SELECT amount FROM budgets_elements_limits WHERE element_id = ?`, rows[0].ID).Scan(&amount); err != nil {
		t.Fatalf("limit row: %v", err)
	}
}

func TestSavingsSync_FlagOffDropsRowAndLimits(t *testing.T) {
	h, tok, _ := newSavingsHarness(t, savingsAccountID)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
		"budgetId": budgetID1, "elementId": savingsAccountID, "period": "2026-06-01", "amount": "250",
	})
	rows := savingsRows(t, h)
	if len(rows) != 1 {
		t.Fatalf("savings rows = %+v, want one", rows)
	}
	elementID := rows[0].ID

	flagSavings(t, h.tdb, budgetID1, savingsAccountID, false)
	h.syncBudget(t, tok)

	if rows := savingsRows(t, h); len(rows) != 0 {
		t.Fatalf("savings rows = %+v, want none once the member is no longer flagged", rows)
	}
	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_elements_limits WHERE element_id = ?`, elementID).Scan(&n)
	if n != 0 {
		t.Fatalf("limit rows = %d, want 0 (cascaded with the element)", n)
	}
}

func TestSavingsSync_RemovedMemberDropsRow(t *testing.T) {
	h, tok, _ := newSavingsHarness(t, savingsAccountID)
	h.syncBudget(t, tok)
	if rows := savingsRows(t, h); len(rows) != 1 {
		t.Fatalf("savings rows = %+v, want one", rows)
	}
	h.mustDo(t, http.MethodPost, "/api/v1/budget/remove-account", tok, map[string]any{"id": budgetID1, "accountId": savingsAccountID})
	h.syncBudget(t, tok)
	if rows := savingsRows(t, h); len(rows) != 0 {
		t.Fatalf("savings rows = %+v, want none after the account left the budget", rows)
	}
}

func TestSavingsSync_DeletedMemberKeepsRow(t *testing.T) {
	h, tok, _ := newSavingsHarness(t, savingsAccountID)
	if _, err := h.db.Exec(`UPDATE accounts SET is_deleted = 1 WHERE id = ?`, savingsAccountID); err != nil {
		t.Fatal(err)
	}
	h.syncBudget(t, tok)
	rows := savingsRows(t, h)
	if len(rows) != 1 || rows[0].ExternalID != savingsAccountID || rows[0].SortKey == "" {
		t.Fatalf("savings rows = %+v, want the deleted member's live row", rows)
	}
}

func TestSavingsMove_FolderRefused(t *testing.T) {
	h, tok, _ := newSavingsHarness(t, savingsAccountID)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-folder", tok, map[string]any{"budgetId": budgetID1, "id": savingsFolderID, "name": "Folder"})
	h.syncBudget(t, tok)

	st, env := h.moveElement(t, tok, map[string]any{"id": savingsAccountID, "folderId": savingsFolderID, "afterId": nil})
	if st != http.StatusBadRequest || !containsField(env, "folderId", "Savings cannot be put into a folder") {
		t.Fatalf("status=%d body=%s, want 400 on folderId", st, env.raw)
	}
	rows := savingsRows(t, h)
	if len(rows) != 1 || rows[0].FolderID.Valid {
		t.Fatalf("savings rows = %+v, want the row left folder-less", rows)
	}
}

func TestSavingsMove_FirstWithinSavingsGroupOnly(t *testing.T) {
	h, tok, _ := newSavingsHarness(t, savingsAccountID, savingsAccountID2)
	h.f.Category(fixture.Category{ID: savingsCatID2, UserID: seedUserID, Name: "Rent", Type: 0, Icon: "home", Position: 1})
	h.syncBudget(t, tok)

	expenseBefore := noFolderOrder(t, h, tok)
	before := savingsIDs(savingsRows(t, h))
	if len(before) != 2 {
		t.Fatalf("savings rows = %v, want two", before)
	}
	last := before[1]

	if st, env := h.moveElement(t, tok, map[string]any{"id": last, "folderId": nil, "afterId": nil}); st != http.StatusOK {
		t.Fatalf("status=%d body=%s", st, env.raw)
	}
	if got := savingsIDs(savingsRows(t, h)); !equalIDs(got, []string{last, before[0]}) {
		t.Fatalf("savings order = %v, want %v first", got, last)
	}
	if got := noFolderOrder(t, h, tok); !equalIDs(got, expenseBefore) {
		t.Fatalf("expense order = %v, want it unchanged from %v", got, expenseBefore)
	}
}

func TestSavingsMove_AnchorFromExpenseGroupAppends(t *testing.T) {
	h, tok, _ := newSavingsHarness(t, savingsAccountID, savingsAccountID2)
	h.syncBudget(t, tok)
	before := savingsIDs(savingsRows(t, h))
	if len(before) != 2 {
		t.Fatalf("savings rows = %v, want two", before)
	}
	first := before[0]

	// catID sorts before every savings row, so resolving it as an anchor would
	// keep first in front; unknown to the savings group, it appends instead.
	if st, env := h.moveElement(t, tok, map[string]any{"id": first, "folderId": nil, "afterId": catID}); st != http.StatusOK {
		t.Fatalf("status=%d body=%s", st, env.raw)
	}
	if got := savingsIDs(savingsRows(t, h)); !equalIDs(got, []string{before[1], first}) {
		t.Fatalf("savings order = %v, want %v appended to the end", got, first)
	}
}

func TestSavingsMove_SavingsAnchorForCategoryAppends(t *testing.T) {
	h, tok, _ := newSavingsHarness(t, savingsAccountID, savingsAccountID2)
	h.f.Category(fixture.Category{ID: savingsCatID2, UserID: seedUserID, Name: "Rent", Type: 0, Icon: "home", Position: 1})
	h.syncBudget(t, tok)

	expenseBefore := noFolderOrder(t, h, tok)
	if len(expenseBefore) < 2 || expenseBefore[0] != catID {
		t.Fatalf("expense order = %v, want %s first", expenseBefore, catID)
	}
	savingsBefore := savingsRows(t, h)
	if len(savingsBefore) != 2 {
		t.Fatalf("savings rows = %+v, want two", savingsBefore)
	}

	if st, env := h.moveElement(t, tok, map[string]any{"id": catID, "folderId": nil, "afterId": savingsBefore[0].ExternalID}); st != http.StatusOK {
		t.Fatalf("status=%d body=%s", st, env.raw)
	}
	got := noFolderOrder(t, h, tok)
	if got[len(got)-1] != catID {
		t.Fatalf("expense order = %v, want %s appended to the end of its group", got, catID)
	}
	savingsAfter := savingsRows(t, h)
	if len(savingsAfter) != len(savingsBefore) {
		t.Fatalf("savings rows changed: %+v -> %+v", savingsBefore, savingsAfter)
	}
	for i := range savingsBefore {
		if savingsAfter[i].ExternalID != savingsBefore[i].ExternalID || savingsAfter[i].SortKey != savingsBefore[i].SortKey {
			t.Fatalf("savings rows changed: %+v -> %+v", savingsBefore, savingsAfter)
		}
	}
}
