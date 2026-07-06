package api_test

// Cross-tenant permission tests for the connection module: managing account
// access (grant/revoke) requires owning the account or holding an ADMIN grant on
// it — a user-role grantee must not be able to re-share or revoke. And a stranger
// must never see another user's connections/shares. Positive counterparts (owner
// grants/revokes) live in connection_endpoints_test.go.

import (
	"net/http"
	"testing"
)

func assertDenied(t *testing.T, status int, env envelope) {
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

// grantRole has the owner grant the guest the given role on ownerAccount.
func (h *harness) grantRole(t *testing.T, role string) {
	t.Helper()
	ownerTok := h.token(t, ownerUserID, ownerEmail)
	st, env := h.do(t, http.MethodPost, "/api/v1/connection/set-account-access", ownerTok, map[string]any{
		"accountId": ownerAccount, "userId": guestUserID, "role": role,
	})
	if st != http.StatusOK {
		t.Fatalf("setup grant role=%s failed: %d; body: %s", role, st, env.raw)
	}
}

// A user-role grantee cannot grant access on the owner's account.
func TestSetAccountAccess_UserGrantee_403(t *testing.T) {
	h := newHarness(t)
	h.grantRole(t, "user")
	guestTok := h.token(t, guestUserID, guestEmail)
	status, env := h.do(t, http.MethodPost, "/api/v1/connection/set-account-access", guestTok, map[string]any{
		"accountId": ownerAccount, "userId": thirdUserID, "role": "user",
	})
	assertDenied(t, status, env)
}

// A user-role grantee cannot revoke access on the owner's account.
func TestRevokeAccountAccess_UserGrantee_403(t *testing.T) {
	h := newHarness(t)
	h.grantRole(t, "user")
	guestTok := h.token(t, guestUserID, guestEmail)
	status, env := h.do(t, http.MethodPost, "/api/v1/connection/revoke-account-access", guestTok, map[string]any{
		"accountId": ownerAccount, "userId": guestUserID,
	})
	assertDenied(t, status, env)
}

// Positive: an ADMIN grantee CAN manage access on the owner's account (the admin
// branch of requireOwnerAdmin).
func TestSetAccountAccess_AdminGrantee_OK(t *testing.T) {
	h := newHarness(t)
	h.grantRole(t, "admin")
	guestTok := h.token(t, guestUserID, guestEmail)
	status, env := h.do(t, http.MethodPost, "/api/v1/connection/set-account-access", guestTok, map[string]any{
		"accountId": ownerAccount, "userId": thirdUserID, "role": "user",
	})
	if status != http.StatusOK {
		t.Fatalf("admin grantee set-access = %d, want 200; body: %s", status, env.raw)
	}
	var role int
	if err := h.db.QueryRow(`SELECT role FROM accounts_access WHERE account_id=? AND user_id=?`, ownerAccount, thirdUserID).Scan(&role); err != nil {
		t.Fatalf("grant row missing after admin grantee set-access: %v", err)
	}
	if role != 1 {
		t.Fatalf("role=%d want 1 (user)", role)
	}
}

// A stranger (unconnected, no shares) sees nothing in their connection list.
func TestGetConnectionList_StrangerSeesNothing(t *testing.T) {
	h := newHarness(t)
	// Owner shares an account with the connected guest, creating real connection
	// data — which the unconnected third user must still never see.
	h.grantRole(t, "user")
	thirdTok := h.token(t, thirdUserID, thirdEmail)
	_, env := h.do(t, http.MethodGet, "/api/v1/connection/get-connection-list", thirdTok, nil)
	if items := mustUnmarshal[listResult](t, env.Data).Items; len(items) != 0 {
		t.Fatalf("stranger connection list = %+v, want empty", items)
	}
}

// The owner's connection list never includes the unconnected third user.
func TestGetConnectionList_OwnerDoesNotSeeStranger(t *testing.T) {
	h := newHarness(t)
	h.grantRole(t, "user")
	ownerTok := h.token(t, ownerUserID, ownerEmail)
	_, env := h.do(t, http.MethodGet, "/api/v1/connection/get-connection-list", ownerTok, nil)
	for _, c := range mustUnmarshal[listResult](t, env.Data).Items {
		if c.User.ID == thirdUserID {
			t.Fatalf("owner connection list leaked unconnected third user; body: %s", env.raw)
		}
	}
}
