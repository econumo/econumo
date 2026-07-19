package api_test

// Cross-tenant + role-matrix permission tests for the budget module at the HTTP
// boundary. Budgets have their own sharing model (owner/admin/user/guest, plus a
// pending-vs-accepted distinction). These tests assert, through the real router:
//   - a stranger (no access row) can neither read nor list another user's budget;
//   - a pending (unaccepted) invitee cannot read;
//   - an accepted guest can read but cannot write;
//   - an accepted user can write element data (set-limit) but cannot delete the budget.
// The role-predicate unit matrix lives in internal/budget/access_test.go; this
// is the end-to-end counterpart.

import (
	"net/http"
	"testing"
)

func assertBudgetDenied(t *testing.T, status int, env envelope) {
	t.Helper()
	if status != http.StatusForbidden {
		t.Fatalf("status=%d want 403; body: %s", status, env.raw)
	}
	if env.Success {
		t.Fatalf("success=true, want false; body: %s", env.raw)
	}
	if env.Message != "Access denied" {
		t.Fatalf("message=%q want %q; body: %s", env.Message, "Access denied", env.raw)
	}
}

// shareAndAccept has the owner grant the second user `role` on budgetID1 and the
// second user accept it (so they become an accepted member at that role).
func (h *harness) shareAndAccept(t *testing.T, ownerTok, otherTok, role string) {
	t.Helper()
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/grant-access", ownerTok, map[string]any{
		"budgetId": budgetID1, "userId": secondUserID, "role": role,
	}); st != http.StatusOK {
		t.Fatalf("grant-access role=%s = %d; body: %s", role, st, e.raw)
	}
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/accept-access", otherTok, map[string]any{
		"budgetId": budgetID1,
	}); st != http.StatusOK {
		t.Fatalf("accept-access = %d; body: %s", st, e.raw)
	}
}

func TestGetBudget_Stranger_403(t *testing.T) {
	h := newHarness(t)
	seedBudget(t, h, h.token(t))
	other := h.seedSecondUser(t)
	status, env := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, other, nil)
	assertBudgetDenied(t, status, env)
}

func TestGetBudget_UnacceptedInvite_403(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	other := h.seedSecondUser(t)
	// Grant but DO NOT accept — a pending invitee must not be able to read.
	if st, e := h.do(t, http.MethodPost, "/api/v1/budget/grant-access", tok, map[string]any{
		"budgetId": budgetID1, "userId": secondUserID, "role": "user",
	}); st != http.StatusOK {
		t.Fatalf("grant precondition=%d body=%s", st, e.raw)
	}
	status, env := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, other, nil)
	assertBudgetDenied(t, status, env)
}

func TestGetBudgetList_ExcludesStrangerBudget(t *testing.T) {
	h := newHarness(t)
	seedBudget(t, h, h.token(t))
	other := h.seedSecondUser(t)
	_, env := h.do(t, http.MethodGet, "/api/v1/budget/get-budget-list", other, nil)
	type listWrap struct {
		Items []struct {
			Id string `json:"id"`
		} `json:"items"`
	}
	for _, it := range mustUnmarshal[listWrap](t, env.Data).Items {
		if it.Id == budgetID1 {
			t.Fatalf("stranger's budget-list leaked another user's budget %q; body: %s", budgetID1, env.raw)
		}
	}
}

func TestGetBudget_AcceptedGuest_CanRead(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	other := h.seedSecondUser(t)
	h.shareAndAccept(t, tok, other, "guest")
	status, env := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, other, nil)
	if status != http.StatusOK {
		t.Fatalf("accepted guest get-budget = %d, want 200; body: %s", status, env.raw)
	}
}

func TestSetLimit_Guest_403(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	other := h.seedSecondUser(t)
	h.shareAndAccept(t, tok, other, "guest")
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/set-limit", other, map[string]any{
		"budgetId": budgetID1, "elementId": catID, "period": "2099-01-01", "amount": "200",
	})
	assertBudgetDenied(t, status, env)
}

func TestCreateEnvelope_Guest_403(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	other := h.seedSecondUser(t)
	h.shareAndAccept(t, tok, other, "guest")
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/create-envelope", other, map[string]any{
		"budgetId": budgetID1, "id": envID1, "name": "Groceries", "icon": "wallet",
		"currencyId": usdID, "categories": []string{catID},
	})
	assertBudgetDenied(t, status, env)
}

func TestSetLimit_AcceptedUser_OK(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	other := h.seedSecondUser(t)
	h.shareAndAccept(t, tok, other, "user")
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/set-limit", other, map[string]any{
		"budgetId": budgetID1, "elementId": catID, "period": "2099-01-01", "amount": "200",
	})
	if status != http.StatusOK {
		t.Fatalf("accepted user set-limit = %d, want 200; body: %s", status, env.raw)
	}
}

func TestDeleteBudget_AcceptedUser_403(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	other := h.seedSecondUser(t)
	h.shareAndAccept(t, tok, other, "user")
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/delete-budget", other, map[string]any{"id": budgetID1})
	assertBudgetDenied(t, status, env)
}

// A non-admin accepted member (guest) must not be able to revoke anyone —
// canShare is owner|admin only, and fails closed for everyone else.
func TestRevokeAccess_NonAdminMember_403(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	other := h.seedSecondUser(t)
	h.shareAndAccept(t, tok, other, "guest")
	// Guest tries to revoke the owner.
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/revoke-access", other, map[string]any{
		"budgetId": budgetID1, "userId": seedUserID,
	})
	assertBudgetDenied(t, status, env)
	// Owner still owns and can read the budget.
	if st, e := h.do(t, http.MethodGet, "/api/v1/budget/get-budget?id="+budgetID1, tok, nil); st != http.StatusOK {
		t.Fatalf("owner get-budget after failed revoke = %d; body: %s", st, e.raw)
	}
}

// Revoking the budget owner's id must be rejected even for an admin — it would
// otherwise delete the owner's own elements/limits via removeMemberRecords.
func TestRevokeAccess_Owner_403(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/revoke-access", tok, map[string]any{
		"budgetId": budgetID1, "userId": seedUserID,
	})
	assertBudgetDenied(t, status, env)
}

// Revoking a user who has no grant on the budget is denied (no silent cleanup of
// a non-member's records).
func TestRevokeAccess_NonMember_403(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	other := h.seedSecondUser(t) // connected, but never granted access
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/revoke-access", tok, map[string]any{
		"budgetId": budgetID1, "userId": other,
	})
	assertBudgetDenied(t, status, env)
}

// A read-only guest must not change budget metadata (currency, excluded
// accounts) by submitting the unchanged name.
func TestUpdateBudget_Guest_403(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	seedBudget(t, h, tok)
	other := h.seedSecondUser(t)
	h.shareAndAccept(t, tok, other, "guest")
	status, env := h.do(t, http.MethodPost, "/api/v1/budget/update-budget", other, map[string]any{
		"id": budgetID1, "name": "Test Budget", "currencyId": eurID,
	})
	assertBudgetDenied(t, status, env)
}
