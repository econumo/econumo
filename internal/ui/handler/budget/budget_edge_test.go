package budget_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// secondUserID is a second seeded user used for access / ownership tests.
const secondUserID = "22222222-2222-2222-2222-222222222222"

// seedSecondUser inserts a second active user so grant-access / ownership-403
// tests have a distinct principal. Returns a JWT for that user.
func (h *harness) seedSecondUser(t *testing.T) string {
	t.Helper()
	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.User(fixture.User{ID: secondUserID, Name: "Second User", Avatar: "https://avatar.test/2", Salt: seedSalt})
	tok, err := h.jwt.Issue(secondUserID, "second@example.test", time.Now())
	if err != nil {
		t.Fatalf("issue second token: %v", err)
	}
	return tok
}

// getBudget fetches the full budget result for assertions.
func (h *harness) getBudget(t *testing.T, tok, id string) budgetResult {
	t.Helper()
	status, env := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+id, tok, nil)
	if status != http.StatusOK {
		t.Fatalf("get-budget=%d body=%s", status, env.raw)
	}
	return mustUnmarshal[budgetResult](t, env.Data)
}

func TestUpdateBudget_RenameAndCurrency(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)

	status, env := h.do(t, http.MethodPost, "/api/v1/budget/update-budget", tok, map[string]any{
		"id": budgetID1, "name": "Renamed Budget", "currencyId": usdID,
	})
	if status != http.StatusOK {
		t.Fatalf("update-budget=%d body=%s", status, env.raw)
	}
	type metaWrap struct {
		Item struct {
			Name string `json:"name"`
		} `json:"item"`
	}
	if got := mustUnmarshal[metaWrap](t, env.Data).Item.Name; got != "Renamed Budget" {
		t.Fatalf("returned name=%q want Renamed Budget", got)
	}
	// Persisted: get-budget reflects the new name.
	if got := h.getBudget(t, tok, budgetID1).Item.Meta.Name; got != "Renamed Budget" {
		t.Fatalf("persisted name=%q want Renamed Budget", got)
	}
}

func TestUpdateBudget_ExcludedAccountsReplaced(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)

	// Exclude the account via update-budget's excludedAccounts set.
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/update-budget", tok, map[string]any{
		"id": budgetID1, "name": "Test Budget", "currencyId": usdID,
		"excludedAccounts": []string{accountID},
	})
	if status != http.StatusOK {
		t.Fatalf("update-budget exclude=%d body=%s", status, env.raw)
	}
	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_excluded_accounts WHERE budget_id=? AND account_id=?`, budgetID1, accountID).Scan(&n)
	if n != 1 {
		t.Fatalf("excluded rows after add=%d want 1", n)
	}
	// Now send an empty set -> the previously-excluded account is re-included.
	status, env = h.do(t, http.MethodPost, "/api/v1/budget/update-budget", tok, map[string]any{
		"id": budgetID1, "name": "Test Budget", "currencyId": usdID,
		"excludedAccounts": []string{},
	})
	if status != http.StatusOK {
		t.Fatalf("update-budget clear=%d body=%s", status, env.raw)
	}
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_excluded_accounts WHERE budget_id=? AND account_id=?`, budgetID1, accountID).Scan(&n)
	if n != 0 {
		t.Fatalf("excluded rows after clear=%d want 0", n)
	}
}

func TestUpdateBudget_BlankName_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	status, _ := h.do(t, http.MethodPost, "/api/v1/budget/update-budget", tok, map[string]any{
		"id": budgetID1, "name": "", "currencyId": usdID,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("update-budget blank name=%d want 400", status)
	}
}

func TestUpdateBudget_NonOwner_403(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	other := h.seedSecondUser(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/update-budget", other, map[string]any{
		"id": budgetID1, "name": "Hijacked", "currencyId": usdID,
	})
	if status != http.StatusForbidden {
		t.Fatalf("update-budget by non-member=%d want 403 body=%s", status, env.raw)
	}
}

func TestDeleteBudget_RemovesFromList(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)

	status, env := h.do(t, http.MethodPost, "/api/v1/budget/delete-budget", tok, map[string]any{"id": budgetID1})
	if status != http.StatusOK {
		t.Fatalf("delete-budget=%d body=%s", status, env.raw)
	}
	// get-budget-list no longer contains it.
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/budget/get-budget-list", tok, nil)
	type listWrap struct {
		Items []struct {
			Id string `json:"id"`
		} `json:"items"`
	}
	for _, it := range mustUnmarshal[listWrap](t, listEnv.Data).Items {
		if it.Id == budgetID1 {
			t.Fatalf("deleted budget still listed")
		}
	}
	// And the row is gone.
	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets WHERE id=?`, budgetID1).Scan(&n)
	if n != 0 {
		t.Fatalf("budget rows=%d want 0 after delete", n)
	}
}

func TestDeleteBudget_NonOwner_403(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	other := h.seedSecondUser(t)
	status, _ := h.do(t, http.MethodPost, "/api/v1/budget/delete-budget", other, map[string]any{"id": budgetID1})
	if status != http.StatusForbidden {
		t.Fatalf("delete-budget non-owner=%d want 403", status)
	}
}

func TestDeleteBudget_Blank_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	status, _ := h.do(t, http.MethodPost, "/api/v1/budget/delete-budget", tok, map[string]any{"id": ""})
	if status != http.StatusBadRequest {
		t.Fatalf("delete-budget blank id=%d want 400", status)
	}
}

func TestResetBudget_ClearsLimits(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	// Set a limit first.
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
		"budgetId": budgetID1, "elementId": catID, "period": "2099-01-01", "amount": "200",
	}); st != http.StatusOK {
		t.Fatalf("set-limit precondition=%d body=%s", st, e.raw)
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/reset-budget", tok, map[string]any{
		"id": budgetID1, "startedAt": "2099-02-01 00:00:00",
	})
	if status != http.StatusOK {
		t.Fatalf("reset-budget=%d body=%s", status, env.raw)
	}
	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_elements_limits l
		JOIN budgets_elements e ON e.id=l.element_id WHERE e.budget_id=?`, budgetID1).Scan(&n)
	if n != 0 {
		t.Fatalf("limit rows after reset=%d want 0", n)
	}
}

func TestResetBudget_BadStartedAt_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	status, _ := h.do(t, http.MethodPost, "/api/v1/budget/reset-budget", tok, map[string]any{
		"id": budgetID1, "startedAt": "not-a-date",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("reset-budget bad date=%d want 400", status)
	}
}

func seedEnvelope(t *testing.T, h *harness, tok string) {
	t.Helper()
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": envID1, "name": "Groceries", "icon": "wallet",
		"currencyId": usdID, "categories": []string{catID},
	}); st != http.StatusOK {
		t.Fatalf("create-envelope=%d body=%s", st, e.raw)
	}
}

func TestUpdateEnvelope_RenameAndArchive(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	seedEnvelope(t, h, tok)

	status, env := h.do(t, http.MethodPost, "/api/v1/budget/update-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": envID1, "name": "Food & Drink", "icon": "restaurant",
		"currencyId": usdID, "isArchived": 1, "categories": []string{catID},
	})
	if status != http.StatusOK {
		t.Fatalf("update-envelope=%d body=%s", status, env.raw)
	}
	res := mustUnmarshal[itemEnvelope](t, env.Data)
	if res.Item.Id != envID1 {
		t.Fatalf("update-envelope id=%q want %q", res.Item.Id, envID1)
	}
	// The envelope name persisted.
	var name string
	var archived int
	h.db.QueryRow(`SELECT name, is_archived FROM budgets_envelopes WHERE id=?`, envID1).Scan(&name, &archived)
	if name != "Food & Drink" {
		t.Fatalf("persisted envelope name=%q want Food & Drink", name)
	}
	if archived != 1 {
		t.Fatalf("persisted is_archived=%d want 1", archived)
	}
}

func TestUpdateEnvelope_RemovesCategoryChild(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	seedEnvelope(t, h, tok)

	// Update with an empty categories set -> the category is detached.
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/update-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": envID1, "name": "Groceries", "icon": "wallet",
		"currencyId": usdID, "isArchived": 0, "categories": []string{},
	})
	if status != http.StatusOK {
		t.Fatalf("update-envelope detach=%d body=%s", status, env.raw)
	}
	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_envelopes_categories WHERE budget_envelope_id=? AND category_id=?`, envID1, catID).Scan(&n)
	if n != 0 {
		t.Fatalf("envelope-category rows=%d want 0 after detach", n)
	}
}

func TestDeleteEnvelope_RemovesEnvelopeAndElement(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	seedEnvelope(t, h, tok)

	status, env := h.do(t, http.MethodPost, "/api/v1/budget/delete-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": envID1,
	})
	if status != http.StatusOK {
		t.Fatalf("delete-envelope=%d body=%s", status, env.raw)
	}
	var nEnv, nEl int
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_envelopes WHERE id=?`, envID1).Scan(&nEnv)
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_elements WHERE budget_id=? AND external_id=?`, budgetID1, envID1).Scan(&nEl)
	if nEnv != 0 || nEl != 0 {
		t.Fatalf("after delete-envelope: envelope rows=%d element rows=%d want 0/0", nEnv, nEl)
	}
}

func TestCreateEnvelope_BlankName_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	status, _ := h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", tok, map[string]any{
		"budgetId": budgetID1, "id": envID1, "name": "", "icon": "wallet", "currencyId": usdID,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("create-envelope blank name=%d want 400", status)
	}
}

func TestCreateEnvelope_NonMember_403(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	other := h.seedSecondUser(t)
	status, _ := h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", other, map[string]any{
		"budgetId": budgetID1, "id": envID1, "name": "Hijack", "icon": "wallet", "currencyId": usdID,
	})
	if status != http.StatusForbidden {
		t.Fatalf("create-envelope non-member=%d want 403", status)
	}
}

const (
	bFolderID1 = "ffff2222-0000-7000-8000-000000000001"
	bFolderID2 = "ffff2222-0000-7000-8000-000000000002"
)

func TestCreateFolder_AtFront(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)

	status, env := h.do(t, http.MethodPost, "/api/v1/budget/create-folder", tok, map[string]any{
		"budgetId": budgetID1, "id": bFolderID1, "name": "Bills",
	})
	if status != http.StatusOK {
		t.Fatalf("create-folder=%d body=%s", status, env.raw)
	}
	type folderWrap struct {
		Item struct {
			Id       string `json:"id"`
			Name     string `json:"name"`
			Position int    `json:"position"`
		} `json:"item"`
	}
	it := mustUnmarshal[folderWrap](t, env.Data).Item
	if it.Id != bFolderID1 || it.Name != "Bills" {
		t.Fatalf("create-folder item=%+v want id/name", it)
	}
	if it.Position != 0 {
		t.Fatalf("new folder position=%d want 0 (front)", it.Position)
	}
	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_folders WHERE id=?`, bFolderID1).Scan(&n)
	if n != 1 {
		t.Fatalf("folder rows=%d want 1", n)
	}
}

func TestUpdateFolder_Renames(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	if st, _ := h.do(t, http.MethodPost, "/api/v1/budget/create-folder", tok, map[string]any{"budgetId": budgetID1, "id": bFolderID1, "name": "Bills"}); st != 200 {
		t.Fatalf("create-folder precondition=%d", st)
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/update-folder", tok, map[string]any{
		"budgetId": budgetID1, "id": bFolderID1, "name": "Recurring",
	})
	if status != http.StatusOK {
		t.Fatalf("update-folder=%d body=%s", status, env.raw)
	}
	var name string
	h.db.QueryRow(`SELECT name FROM budgets_folders WHERE id=?`, bFolderID1).Scan(&name)
	if name != "Recurring" {
		t.Fatalf("persisted folder name=%q want Recurring", name)
	}
}

func TestDeleteFolder_Removes(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	if st, _ := h.do(t, http.MethodPost, "/api/v1/budget/create-folder", tok, map[string]any{"budgetId": budgetID1, "id": bFolderID1, "name": "Bills"}); st != 200 {
		t.Fatalf("create-folder precondition=%d", st)
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/delete-folder", tok, map[string]any{
		"budgetId": budgetID1, "id": bFolderID1,
	})
	if status != http.StatusOK {
		t.Fatalf("delete-folder=%d body=%s", status, env.raw)
	}
	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_folders WHERE id=?`, bFolderID1).Scan(&n)
	if n != 0 {
		t.Fatalf("folder rows=%d want 0 after delete", n)
	}
}

func TestOrderFolderList_AppliesPositions(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	for _, f := range []struct{ id, name string }{{bFolderID1, "Alpha"}, {bFolderID2, "Bravo"}} {
		if st, _ := h.do(t, http.MethodPost, "/api/v1/budget/create-folder", tok, map[string]any{"budgetId": budgetID1, "id": f.id, "name": f.name}); st != 200 {
			t.Fatalf("create-folder %s=%d", f.name, st)
		}
	}
	// Force folder1->position 5, folder2->position 9.
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/order-folder-list", tok, map[string]any{
		"budgetId": budgetID1,
		"items": []map[string]any{
			{"id": bFolderID1, "position": 5},
			{"id": bFolderID2, "position": 9},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("order-folder-list=%d body=%s", status, env.raw)
	}
	var p1, p2 int
	h.db.QueryRow(`SELECT position FROM budgets_folders WHERE id=?`, bFolderID1).Scan(&p1)
	h.db.QueryRow(`SELECT position FROM budgets_folders WHERE id=?`, bFolderID2).Scan(&p2)
	if p1 != 5 || p2 != 9 {
		t.Fatalf("folder positions=%d,%d want 5,9", p1, p2)
	}
}

func TestCreateFolder_NonMember_403(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	other := h.seedSecondUser(t)
	status, _ := h.do(t, http.MethodPost, "/api/v1/budget/create-folder", other, map[string]any{
		"budgetId": budgetID1, "id": bFolderID1, "name": "Hijack",
	})
	if status != http.StatusForbidden {
		t.Fatalf("create-folder non-member=%d want 403", status)
	}
}

func TestCreateFolder_Blank_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	status, _ := h.do(t, http.MethodPost, "/api/v1/budget/create-folder", tok, map[string]any{
		"budgetId": budgetID1, "id": "", "name": "Bills",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("create-folder blank id=%d want 400", status)
	}
}

func TestExcludeAccount_ReflectedInGetBudget(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)

	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/exclude-account", tok, map[string]any{"id": budgetID1, "accountId": accountID}); st != http.StatusOK {
		t.Fatalf("exclude-account=%d body=%s", st, e.raw)
	}
	// get-budget filters.excludedAccountsIds must contain the account.
	status, env := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, tok, nil)
	if status != http.StatusOK {
		t.Fatalf("get-budget=%d body=%s", status, env.raw)
	}
	res := mustUnmarshal[struct {
		Item struct {
			Filters struct {
				ExcludedAccountsIds []string `json:"excludedAccountsIds"`
			} `json:"filters"`
		} `json:"item"`
	}](t, env.Data)
	var found bool
	for _, id := range res.Item.Filters.ExcludedAccountsIds {
		if id == accountID {
			found = true
		}
	}
	if !found {
		t.Fatalf("excludedAccountsIds=%v want to contain %s", res.Item.Filters.ExcludedAccountsIds, accountID)
	}
}

func TestExcludeAccount_NotOwnerOfAccount_403(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	other := h.seedSecondUser(t)
	// The second user does not own the seeded account -> AccessDenied.
	status, _ := h.do(t, http.MethodPost, "/api/v1/budget/exclude-account", other, map[string]any{
		"id": budgetID1, "accountId": accountID,
	})
	if status != http.StatusForbidden {
		t.Fatalf("exclude-account non-account-owner=%d want 403", status)
	}
}

func TestChangeElementCurrency_UpdatesElement(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)

	// Seed a EUR currency so the change is a real mutation.
	const eurID = "dffc2a06-6f29-4704-8575-31709adee927"
	fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"}).Currency(fixture.Currency{ID: eurID, Code: "EUR", Symbol: "€"})

	status, env := h.do(t, http.MethodPost, "/api/v1/budget/change-element-currency", tok, map[string]any{
		"budgetId": budgetID1, "elementId": catID, "currencyId": eurID,
	})
	if status != http.StatusOK {
		t.Fatalf("change-element-currency=%d body=%s", status, env.raw)
	}
	var cur string
	h.db.QueryRow(`SELECT currency_id FROM budgets_elements WHERE budget_id=? AND external_id=?`, budgetID1, catID).Scan(&cur)
	if cur != eurID {
		t.Fatalf("element currency=%q want %q", cur, eurID)
	}
}

func TestChangeElementCurrency_Blank_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	status, _ := h.do(t, http.MethodPost, "/api/v1/budget/change-element-currency", tok, map[string]any{
		"budgetId": budgetID1, "elementId": "", "currencyId": usdID,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("change-element-currency blank elementId=%d want 400", status)
	}
}

func TestSetLimit_OverwriteThenClear(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)

	limitRow := func() (int, string) {
		var n int
		var amt string
		h.db.QueryRow(`SELECT COUNT(*), COALESCE(MAX(l.amount),'') FROM budgets_elements_limits l
			JOIN budgets_elements e ON e.id=l.element_id
			WHERE e.budget_id=? AND e.external_id=?`, budgetID1, catID).Scan(&n, &amt)
		return n, amt
	}

	// Create.
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
		"budgetId": budgetID1, "elementId": catID, "period": "2099-01-01", "amount": "200",
	}); st != http.StatusOK {
		t.Fatalf("set-limit create=%d body=%s", st, e.raw)
	}
	if n, _ := limitRow(); n != 1 {
		t.Fatalf("limit rows after create=%d want 1", n)
	}
	// Overwrite same period with a new amount.
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
		"budgetId": budgetID1, "elementId": catID, "period": "2099-01-15", "amount": "350",
	}); st != http.StatusOK {
		t.Fatalf("set-limit overwrite=%d body=%s", st, e.raw)
	}
	n, amt := limitRow()
	if n != 1 {
		t.Fatalf("limit rows after overwrite=%d want 1 (same period)", n)
	}
	if amt != "350" {
		t.Fatalf("limit amount after overwrite=%q want 350", amt)
	}
	// Clear: amount null/absent removes the limit.
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
		"budgetId": budgetID1, "elementId": catID, "period": "2099-01-01", "amount": nil,
	}); st != http.StatusOK {
		t.Fatalf("set-limit clear=%d body=%s", st, e.raw)
	}
	if n, _ := limitRow(); n != 0 {
		t.Fatalf("limit rows after clear=%d want 0", n)
	}
}

func TestSetLimit_ClearWhenAbsent_NoOp(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	// No existing limit; amount absent -> no-op, still 200.
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
		"budgetId": budgetID1, "elementId": catID, "period": "2099-01-01",
	})
	if status != http.StatusOK {
		t.Fatalf("set-limit clear-when-absent=%d body=%s", status, env.raw)
	}
	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_elements_limits l
		JOIN budgets_elements e ON e.id=l.element_id WHERE e.budget_id=?`, budgetID1).Scan(&n)
	if n != 0 {
		t.Fatalf("limit rows=%d want 0", n)
	}
}

func TestSetLimit_PeriodBeforeStart_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	// The budget starts at "now" (2023-07-22 from the harness clock). A period in
	// 1990 is before the start month -> invalid-date guard -> 400.
	status, _ := h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
		"budgetId": budgetID1, "elementId": catID, "period": "1990-01-01", "amount": "10",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("set-limit period-before-start=%d want 400", status)
	}
}

func TestSetLimit_BadPeriod_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	status, _ := h.do(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{
		"budgetId": budgetID1, "elementId": catID, "period": "garbage", "amount": "10",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("set-limit bad period=%d want 400", status)
	}
}

func TestGrantAccess_ThenListReflects_ThenRevoke(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	other := h.seedSecondUser(t)

	// Owner grants the second user "user" access.
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/grant-access", tok, map[string]any{
		"budgetId": budgetID1, "userId": secondUserID, "role": "user",
	})
	if status != http.StatusOK {
		t.Fatalf("grant-access=%d body=%s", status, env.raw)
	}
	// An access row now exists for the invited user (pending).
	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_access WHERE budget_id=? AND user_id=?`, budgetID1, secondUserID).Scan(&n)
	if n != 1 {
		t.Fatalf("access rows after grant=%d want 1", n)
	}
	// The invited user can now see the budget in their list (ListForUser includes
	// any access, accepted or not).
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/budget/get-budget-list", other, nil)
	type listWrap struct {
		Items []struct {
			Id string `json:"id"`
		} `json:"items"`
	}
	var seen bool
	for _, it := range mustUnmarshal[listWrap](t, listEnv.Data).Items {
		if it.Id == budgetID1 {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("invited user's budget-list does not contain the granted budget")
	}

	// Owner revokes.
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/revoke-access", tok, map[string]any{
		"budgetId": budgetID1, "userId": secondUserID,
	}); st != http.StatusOK {
		t.Fatalf("revoke-access=%d body=%s", st, e.raw)
	}
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_access WHERE budget_id=? AND user_id=?`, budgetID1, secondUserID).Scan(&n)
	if n != 0 {
		t.Fatalf("access rows after revoke=%d want 0", n)
	}
}

func TestAcceptAccess_MarksAccepted(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	other := h.seedSecondUser(t)

	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/grant-access", tok, map[string]any{
		"budgetId": budgetID1, "userId": secondUserID, "role": "user",
	}); st != http.StatusOK {
		t.Fatalf("grant precondition=%d body=%s", st, e.raw)
	}
	// The invitee accepts.
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/accept-access", other, map[string]any{
		"budgetId": budgetID1,
	})
	if status != http.StatusOK {
		t.Fatalf("accept-access=%d body=%s", status, env.raw)
	}
	var accepted int
	h.db.QueryRow(`SELECT is_accepted FROM budgets_access WHERE budget_id=? AND user_id=?`, budgetID1, secondUserID).Scan(&accepted)
	if accepted != 1 {
		t.Fatalf("is_accepted=%d want 1 after accept", accepted)
	}
}

func TestDeclineAccess_RemovesOwnRow(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	other := h.seedSecondUser(t)

	if st, _ := h.do(t, http.MethodPost, "/api/v1/budget/grant-access", tok, map[string]any{
		"budgetId": budgetID1, "userId": secondUserID, "role": "user",
	}); st != http.StatusOK {
		t.Fatalf("grant precondition failed")
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/decline-access", other, map[string]any{
		"budgetId": budgetID1,
	})
	if status != http.StatusOK {
		t.Fatalf("decline-access=%d body=%s", status, env.raw)
	}
	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_access WHERE budget_id=? AND user_id=?`, budgetID1, secondUserID).Scan(&n)
	if n != 0 {
		t.Fatalf("access rows after decline=%d want 0", n)
	}
}

func TestGrantAccess_InvalidRole_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	status, _ := h.do(t, http.MethodPost, "/api/v1/budget/grant-access", tok, map[string]any{
		"budgetId": budgetID1, "userId": secondUserID, "role": "superuser",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("grant-access invalid role=%d want 400", status)
	}
}

func TestGrantAccess_Blank_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	status, _ := h.do(t, http.MethodPost, "/api/v1/budget/grant-access", tok, map[string]any{
		"budgetId": budgetID1, "userId": "", "role": "user",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("grant-access blank userId=%d want 400", status)
	}
}

func TestAcceptAccess_NoPendingInvite_403(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	other := h.seedSecondUser(t)
	// No grant issued -> the second user cannot accept.
	status, _ := h.do(t, http.MethodPost, "/api/v1/budget/accept-access", other, map[string]any{
		"budgetId": budgetID1,
	})
	if status != http.StatusForbidden {
		t.Fatalf("accept without invite=%d want 403", status)
	}
}
