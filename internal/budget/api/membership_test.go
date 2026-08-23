package api_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/test/fixture"
)

// fixedAugust puts "now" at 2026-08-17 10:00 UTC, so the current month starts
// 2026-08-01 and everything before it is a closed month.
func fixedAugust() tzClock { return tzClock{t: time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)} }

type filtersOut struct {
	Item struct {
		Filters struct {
			Accounts []struct {
				Id        string `json:"id"`
				Removable bool   `json:"removable"`
			} `json:"accounts"`
		} `json:"filters"`
	} `json:"item"`
}

// mustDo issues a request and fails the test unless it returned 200.
func (h *harness) mustDo(t *testing.T, method, path, token string, body any) envelope {
	t.Helper()
	st, env := h.do(t, method, path, token, body)
	if st != http.StatusOK {
		t.Fatalf("%s %s: status=%d body=%s", method, path, st, env.raw)
	}
	return env
}

// containsField reports whether the envelope's errors[field] carries a message
// containing substr.
func containsField(env envelope, field, substr string) bool {
	for _, m := range env.errorsMap()[field] {
		if strings.Contains(m, substr) {
			return true
		}
	}
	return false
}

func TestCreateBudget_RequiresAtLeastOneOwnedAccount(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "accountIds": []string{}})
	if st != http.StatusBadRequest || !containsField(env, "accountIds", "Select at least one account") {
		t.Fatalf("status=%d body=%s", st, env.raw)
	}
	// deleted account cannot be a member
	h.f.Account(fixture.Account{ID: deadAccountID, UserID: seedUserID, CurrencyID: usdID, Deleted: true})
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "accountIds": []string{deadAccountID}})
	if st != http.StatusBadRequest {
		t.Fatalf("deleted account accepted: %s", env.raw)
	}
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok, createBudgetReq(budgetID1, "Budget"))
	if st != http.StatusOK {
		t.Fatalf("status=%d body=%s", st, env.raw)
	}
	res := mustUnmarshal[filtersOut](t, env.Data)
	if len(res.Item.Filters.Accounts) != 1 || res.Item.Filters.Accounts[0].Id != accountID || !res.Item.Filters.Accounts[0].Removable {
		t.Fatalf("filters=%+v", res.Item.Filters)
	}
}

// TestMembership_DeletedMemberKeepsCounting is the core regression: spend on a
// since-deleted member stays in spentBefore, so a category with a 100 limit and
// 100 spend in July shows available 0 in August.
func TestMembership_DeletedMemberKeepsCounting(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, CategoryID: catID, Type: 0, Amount: "100", SpentAt: "2026-07-10 12:00:00"})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "startDate": "2026-07-01", "accountIds": []string{accountID}})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/set-limit", tok, map[string]any{"budgetId": budgetID1, "elementId": catID, "period": "2026-07-01", "amount": "100"})
	// soft-delete the account directly (the account feature isn't wired in this harness)
	if _, err := h.db.Exec(`UPDATE accounts SET is_deleted = 1 WHERE id = ?`, accountID); err != nil {
		t.Fatal(err)
	}
	st, env := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1+"&date=2026-08-15", tok, nil)
	if st != http.StatusOK {
		t.Fatalf("status=%d body=%s", st, env.raw)
	}
	res := mustUnmarshal[elementView](t, env.Data)
	found := false
	for _, el := range res.Item.Structure.Elements {
		if el.Id == catID {
			found = true
			if el.Available != "0" {
				t.Fatalf("available=%q want 0 (spentBefore must include the deleted member)", el.Available)
			}
		}
	}
	if !found {
		t.Fatal("Food element missing")
	}
}

// TestMembership_DeletedLockedMemberSurvivesUpdate: a soft-deleted member stays
// listed in filters.accounts (it keeps counting) and can never be removed once
// locked, so the client round-trips its id back on every save. Rejecting it
// would wedge update-budget — even a pure rename — forever.
func TestMembership_DeletedLockedMemberSurvivesUpdate(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "5", SpentAt: "2026-07-10 12:00:00"})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "startDate": "2026-06-01", "accountIds": []string{accountID}})
	if _, err := h.db.Exec(`UPDATE accounts SET is_deleted = 1 WHERE id = ?`, accountID); err != nil {
		t.Fatal(err)
	}

	_, env := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, tok, nil)
	res := mustUnmarshal[filtersOut](t, env.Data)
	if len(res.Item.Filters.Accounts) != 1 || res.Item.Filters.Accounts[0].Id != accountID || res.Item.Filters.Accounts[0].Removable {
		t.Fatalf("filters=%+v want the deleted member listed and locked", res.Item.Filters)
	}

	env = h.mustDo(t, http.MethodPost, "/api/v1/budget/update-budget", tok, map[string]any{
		"id": budgetID1, "name": "Renamed Budget", "currencyId": usdID, "accountIds": []string{accountID},
	})
	meta := mustUnmarshal[struct {
		Item struct {
			Name string `json:"name"`
		} `json:"item"`
	}](t, env.Data)
	if meta.Item.Name != "Renamed Budget" {
		t.Fatalf("name=%q want the rename applied", meta.Item.Name)
	}
	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_accounts WHERE budget_id = ? AND account_id = ?`, budgetID1, accountID).Scan(&n)
	if n != 1 {
		t.Fatalf("membership rows=%d want 1 (the deleted member stays)", n)
	}
}

// TestAddAccount_DeletedMemberIsNoOp: re-adding an existing member is a no-op
// even when the account is soft-deleted, while a deleted NON-member is refused.
func TestAddAccount_DeletedMemberIsNoOp(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "startDate": "2026-06-01", "accountIds": []string{accountID}})
	if _, err := h.db.Exec(`UPDATE accounts SET is_deleted = 1 WHERE id = ?`, accountID); err != nil {
		t.Fatal(err)
	}

	h.mustDo(t, http.MethodPost, "/api/v1/budget/add-account", tok, map[string]any{"id": budgetID1, "accountId": accountID})
	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_accounts WHERE budget_id = ?`, budgetID1).Scan(&n)
	if n != 1 {
		t.Fatalf("membership rows=%d want 1 (re-adding a deleted member is a no-op)", n)
	}

	h.f.Account(fixture.Account{ID: deadAccountID, UserID: seedUserID, CurrencyID: usdID, Deleted: true})
	st, env := h.do(t, http.MethodPost, "/api/v1/budget/add-account", tok, map[string]any{"id": budgetID1, "accountId": deadAccountID})
	if st != http.StatusBadRequest {
		t.Fatalf("deleted non-member accepted: status=%d body=%s", st, env.raw)
	}
	h.db.QueryRow(`SELECT COUNT(*) FROM budgets_accounts WHERE budget_id = ?`, budgetID1).Scan(&n)
	if n != 1 {
		t.Fatalf("membership rows=%d want 1 (the refused add must write nothing)", n)
	}
}

func TestMembership_RemovalRule(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	h.f.Account(fixture.Account{ID: accountID2, UserID: seedUserID, CurrencyID: usdID, Name: "Savings"})
	// accountID: transaction in July (closed month) → locked. accountID2: only August → removable.
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "1", SpentAt: "2026-07-20 12:00:00"})
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID2, Type: 0, Amount: "1", SpentAt: "2026-08-02 12:00:00"})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "startDate": "2026-06-01", "accountIds": []string{accountID, accountID2}})

	_, env := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, tok, nil)
	res := mustUnmarshal[filtersOut](t, env.Data)
	removable := map[string]bool{}
	for _, a := range res.Item.Filters.Accounts {
		removable[a.Id] = a.Removable
	}
	if removable[accountID] || !removable[accountID2] {
		t.Fatalf("removable=%v", removable)
	}

	st, env := h.do(t, http.MethodPost, "/api/v1/budget/remove-account", tok, map[string]any{"id": budgetID1, "accountId": accountID})
	if st != http.StatusBadRequest || !containsField(env, "accountId", "can no longer be removed") {
		t.Fatalf("locked removal: status=%d body=%s", st, env.raw)
	}
	h.mustDo(t, http.MethodPost, "/api/v1/budget/remove-account", tok, map[string]any{"id": budgetID1, "accountId": accountID2})
	// update-budget replace-set: omitting the locked member is refused
	st, env = h.do(t, http.MethodPost, "/api/v1/budget/update-budget", tok, map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "accountIds": []string{}})
	if st != http.StatusBadRequest {
		t.Fatalf("update omitting locked member: %s", env.raw)
	}
	// update-budget without accountIds leaves membership alone
	h.mustDo(t, http.MethodPost, "/api/v1/budget/update-budget", tok, map[string]any{"id": budgetID1, "name": "Budget 2", "currencyId": usdID})
	// re-add + transaction before started_at doesn't lock
	h.mustDo(t, http.MethodPost, "/api/v1/budget/add-account", tok, map[string]any{"id": budgetID1, "accountId": accountID2})
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID2, Type: 0, Amount: "1", SpentAt: "2026-05-20 12:00:00"})
	_, env = h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, tok, nil)
	res = mustUnmarshal[filtersOut](t, env.Data)
	for _, a := range res.Item.Filters.Accounts {
		if a.Id == accountID2 && !a.Removable {
			t.Fatal("pre-start transaction must not lock")
		}
	}
}

// TestMembership_BoundaryFollowsCallerTimezone: 2026-08-01 02:00 UTC is still
// July 31 in America/Los_Angeles, so a July 31 transaction is in the CURRENT
// month for that caller and the account stays removable there.
func TestMembership_BoundaryFollowsCallerTimezone(t *testing.T) {
	h := newHarnessWithClock(t, tzClock{t: time.Date(2026, 8, 1, 2, 0, 0, 0, time.UTC)})
	tok := h.token(t)
	h.f.Transaction(fixture.Transaction{UserID: seedUserID, AccountID: accountID, Type: 0, Amount: "1", SpentAt: "2026-07-31 12:00:00"})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "startDate": "2026-06-01", "accountIds": []string{accountID}})
	_, env := h.doH(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, tok, nil, map[string]string{"X-Timezone": "America/Los_Angeles"})
	if res := mustUnmarshal[filtersOut](t, env.Data); !res.Item.Filters.Accounts[0].Removable {
		t.Fatal("LA caller: still removable")
	}
	_, env = h.doH(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, tok, nil, map[string]string{"X-Timezone": "UTC"})
	if res := mustUnmarshal[filtersOut](t, env.Data); res.Item.Filters.Accounts[0].Removable {
		t.Fatal("UTC caller: locked")
	}
}

func TestMembership_LeavingParticipantTakesTheirAccounts(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	// second user with an account that has a closed-month transaction (locked)
	h.f.User(fixture.User{ID: otherUserID, Email: "o@e.test", Name: "O", Password: "pw", Salt: seedSalt})
	h.f.Account(fixture.Account{ID: otherAccountID, UserID: otherUserID, CurrencyID: usdID})
	h.f.Transaction(fixture.Transaction{UserID: otherUserID, AccountID: otherAccountID, Type: 0, Amount: "7", SpentAt: "2026-07-05 12:00:00"})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{"id": budgetID1, "name": "Budget", "currencyId": usdID, "startDate": "2026-06-01", "accountIds": []string{accountID}})
	// grant-access requires a users_connections link
	h.f.Connect(seedUserID, otherUserID)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/grant-access", tok, map[string]any{"budgetId": budgetID1, "userId": otherUserID, "role": "user"})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/accept-access", otherUserID, map[string]any{"budgetId": budgetID1})
	h.mustDo(t, http.MethodPost, "/api/v1/budget/add-account", otherUserID, map[string]any{"id": budgetID1, "accountId": otherAccountID})
	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM budgets_accounts WHERE budget_id = ?`, budgetID1).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("members=%d want 2", n)
	}
	h.mustDo(t, http.MethodPost, "/api/v1/budget/revoke-access", tok, map[string]any{"budgetId": budgetID1, "userId": otherUserID})
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM budgets_accounts WHERE budget_id = ? AND account_id = ?`, budgetID1, otherAccountID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("departing member's locked account must be dropped")
	}
}
