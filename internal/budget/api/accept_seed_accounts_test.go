package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

// Ids for the accept-access seeding scenarios: the joining user's deleted
// account, and an account merely SHARED with them.
const (
	otherDeadAccountID   = "aaaa2222-0000-7000-8000-000000000002"
	otherSharedAccountID = "aaaa2222-0000-7000-8000-000000000003"
)

// members returns the membership rows of a budget as a set of account ids.
func (h *harness) members(t *testing.T, budgetID string) map[string]bool {
	t.Helper()
	rows, err := h.db.Query(`SELECT account_id FROM budgets_accounts WHERE budget_id = ?`, budgetID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		out[id] = true
	}
	return out
}

// grantAndInvite creates budgetID1 for the seed user, connects otherUserID and
// grants them the given role. The invite is left pending.
func (h *harness) grantAndInvite(t *testing.T, role string) string {
	t.Helper()
	tok := h.token(t)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/create-budget", tok, map[string]any{
		"id": budgetID1, "name": "Budget", "currencyId": usdID, "startDate": "2026-06-01", "accountIds": []string{accountID},
	})
	h.f.Connect(seedUserID, otherUserID)
	h.mustDo(t, http.MethodPost, "/api/v1/budget/grant-access", tok, map[string]any{"budgetId": budgetID1, "userId": otherUserID, "role": role})
	return tok
}

// TestAcceptAccess_SeedsAcceptorsLiveAccounts: a joining non-guest participant's
// own live accounts become budget members at accept, so their spending reaches
// the same population their limits do. Deleted accounts and accounts merely
// shared with them stay out; a later revoke takes the seeded rows away again.
func TestAcceptAccess_SeedsAcceptorsLiveAccounts(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	tok := h.token(t)
	h.f.User(fixture.User{ID: otherUserID, Email: "o@e.test", Name: "O", Password: "pw", Salt: seedSalt})
	h.f.Account(fixture.Account{ID: otherAccountID, UserID: otherUserID, CurrencyID: usdID, Name: "Theirs"})
	h.f.Account(fixture.Account{ID: otherDeadAccountID, UserID: otherUserID, CurrencyID: usdID, Name: "Gone", Deleted: true})
	// An account owned by the seed user and shared with the joining user: it is
	// "available" to them but not owned, so it must not join.
	h.f.Account(fixture.Account{ID: otherSharedAccountID, UserID: seedUserID, CurrencyID: usdID, Name: "Shared"})
	h.f.AccountAccess(otherSharedAccountID, otherUserID, 1)

	h.grantAndInvite(t, "user")
	h.mustDo(t, http.MethodPost, "/api/v1/budget/accept-access", otherUserID, map[string]any{"budgetId": budgetID1})

	got := h.members(t, budgetID1)
	want := map[string]bool{accountID: true, otherAccountID: true}
	if len(got) != len(want) || !got[accountID] || !got[otherAccountID] {
		t.Fatalf("members=%v want %v (the acceptor's live account joins; deleted + shared stay out)", got, want)
	}

	h.mustDo(t, http.MethodPost, "/api/v1/budget/revoke-access", tok, map[string]any{"budgetId": budgetID1, "userId": otherUserID})
	if got := h.members(t, budgetID1); len(got) != 1 || !got[accountID] {
		t.Fatalf("after revoke members=%v want only the owner's account", got)
	}
}

// TestAcceptAccess_GuestSeedsNoAccounts mirrors the migration's seed rule
// (`is_accepted AND role <> 2`): a guest contributes no accounts.
func TestAcceptAccess_GuestSeedsNoAccounts(t *testing.T) {
	h := newHarnessWithClock(t, fixedAugust())
	h.f.User(fixture.User{ID: otherUserID, Email: "o@e.test", Name: "O", Password: "pw", Salt: seedSalt})
	h.f.Account(fixture.Account{ID: otherAccountID, UserID: otherUserID, CurrencyID: usdID, Name: "Theirs"})

	h.grantAndInvite(t, "guest")
	h.mustDo(t, http.MethodPost, "/api/v1/budget/accept-access", otherUserID, map[string]any{"budgetId": budgetID1})

	if got := h.members(t, budgetID1); len(got) != 1 || !got[accountID] {
		t.Fatalf("members=%v want only the owner's account (a guest contributes none)", got)
	}
}
