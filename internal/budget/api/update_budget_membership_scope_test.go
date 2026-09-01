package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	scopeBudgetID    = "bbbb6666-0000-7000-8000-000000000006"
	partnerAcctID    = "aaaa6666-0000-7000-8000-000000000006"
	partnerAcctBothB = "aaaa6666-0000-7000-8000-000000000007"
)

// TestUpdateBudget_KeepsPartnerMembers: update-budget's accountIds set is
// scoped to the CALLER's own accounts. get-budget only ever reports the
// requester's own member accounts (buildFilters), so a client round-tripping
// that list back cannot name its partner's accounts — replacing the whole
// budget-wide set would silently drop every account the partner added.
func TestUpdateBudget_KeepsPartnerMembers(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		createBudgetReq(scopeBudgetID, "Shared Budget")); st != http.StatusOK {
		t.Fatalf("create-budget=%d body=%s", st, e.raw)
	}

	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.User(fixture.User{ID: otherUserID, Name: "Partner"})
	f.BudgetAccess(scopeBudgetID, otherUserID, 1, true) // role=user, accepted
	f.Account(fixture.Account{ID: partnerAcctID, UserID: otherUserID, CurrencyID: usdID, Name: "Partner Cash"})

	// The partner adds their own account to the shared budget.
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/add-account", otherUserID, map[string]any{
		"id": scopeBudgetID, "accountId": partnerAcctID,
	}); st != http.StatusOK {
		t.Fatalf("partner add-account=%d body=%s", st, e.raw)
	}

	// The owner opens the budget dialog and submits their OWN account set. The
	// partner's membership is invisible to them and must survive.
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/update-budget", tok, map[string]any{
		"id": scopeBudgetID, "name": "Shared Budget", "currencyId": usdID,
		"accountIds": []string{accountID},
	})
	if status != http.StatusOK {
		t.Fatalf("update-budget=%d body=%s", status, env.raw)
	}

	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM budgets_accounts WHERE budget_id=? AND account_id=?`,
		scopeBudgetID, partnerAcctID).Scan(&n); err != nil {
		t.Fatalf("count partner membership: %v", err)
	}
	if n != 1 {
		t.Fatalf("partner membership rows=%d want 1 (update-budget must not drop another user's account)", n)
	}
	// The caller's own membership is intact.
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM budgets_accounts WHERE budget_id=? AND account_id=?`,
		scopeBudgetID, accountID).Scan(&n); err != nil {
		t.Fatalf("count own membership: %v", err)
	}
	if n != 1 {
		t.Fatalf("own membership rows=%d want 1", n)
	}
}

// TestUpdateBudget_IgnoresForeignAccountMembership: the mirror of the above —
// accountIds must not let a caller add an account they do not own (add-account
// enforces the same ownership rule).
func TestUpdateBudget_IgnoresForeignAccountMembership(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)

	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		createBudgetReq(scopeBudgetID, "Shared Budget")); st != http.StatusOK {
		t.Fatalf("create-budget=%d body=%s", st, e.raw)
	}

	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	f.User(fixture.User{ID: otherUserID, Name: "Partner"})
	f.BudgetAccess(scopeBudgetID, otherUserID, 1, true)
	f.Account(fixture.Account{ID: partnerAcctBothB, UserID: otherUserID, CurrencyID: usdID, Name: "Partner Cash"})

	status, env := h.do(t, http.MethodPost, "/api/v1/budget/update-budget", tok, map[string]any{
		"id": scopeBudgetID, "name": "Shared Budget", "currencyId": usdID,
		"accountIds": []string{accountID, partnerAcctBothB},
	})
	if status != http.StatusOK {
		t.Fatalf("update-budget=%d body=%s", status, env.raw)
	}

	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM budgets_accounts WHERE budget_id=? AND account_id=?`,
		scopeBudgetID, partnerAcctBothB).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("foreign membership rows=%d want 0 (caller may only add accounts they own)", n)
	}
}
