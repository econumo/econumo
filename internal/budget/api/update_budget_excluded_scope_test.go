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

// TestUpdateBudget_KeepsPartnerExclusions: update-budget's excludedAccounts set
// is scoped to the CALLER's own accounts. get-budget only ever reports the
// requester's own exclusions (buildFilters), so a client round-tripping that
// list back cannot name its partner's excluded accounts — replacing the whole
// budget-wide set would silently re-include every account the partner excluded.
func TestUpdateBudget_KeepsPartnerExclusions(t *testing.T) {
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

	// The partner excludes their own account from the shared budget.
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/exclude-account", otherUserID, map[string]any{
		"id": scopeBudgetID, "accountId": partnerAcctID,
	}); st != http.StatusOK {
		t.Fatalf("partner exclude-account=%d body=%s", st, e.raw)
	}

	// The owner opens the budget dialog and excludes one of THEIR OWN accounts.
	// The submitted set carries only the owner's accounts — the partner's
	// exclusion is invisible to them and must survive.
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/update-budget", tok, map[string]any{
		"id": scopeBudgetID, "name": "Shared Budget", "currencyId": usdID,
		"excludedAccounts": []string{accountID},
	})
	if status != http.StatusOK {
		t.Fatalf("update-budget=%d body=%s", status, env.raw)
	}

	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM budgets_excluded_accounts WHERE budget_id=? AND account_id=?`,
		scopeBudgetID, partnerAcctID).Scan(&n); err != nil {
		t.Fatalf("count partner exclusion: %v", err)
	}
	if n != 1 {
		t.Fatalf("partner exclusion rows=%d want 1 (update-budget must not re-include another user's account)", n)
	}
	// The caller's own exclusion still applied.
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM budgets_excluded_accounts WHERE budget_id=? AND account_id=?`,
		scopeBudgetID, accountID).Scan(&n); err != nil {
		t.Fatalf("count own exclusion: %v", err)
	}
	if n != 1 {
		t.Fatalf("own exclusion rows=%d want 1", n)
	}
}

// TestUpdateBudget_IgnoresForeignAccountExclusion: the mirror of the above —
// excludedAccounts must not let a caller exclude an account they do not own
// (exclude-account enforces the same ownership rule).
func TestUpdateBudget_IgnoresForeignAccountExclusion(t *testing.T) {
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
		"excludedAccounts": []string{partnerAcctBothB},
	})
	if status != http.StatusOK {
		t.Fatalf("update-budget=%d body=%s", status, env.raw)
	}

	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM budgets_excluded_accounts WHERE budget_id=? AND account_id=?`,
		scopeBudgetID, partnerAcctBothB).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("foreign exclusion rows=%d want 0 (caller may only exclude accounts they own)", n)
	}
}
