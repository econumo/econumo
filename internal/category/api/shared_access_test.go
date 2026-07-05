package api_test

// Coverage for creating a category in the context of an account SHARED with the
// caller (the create-category request carries an accountId — the transaction
// modal sends it when a category is added inline while entering a transaction on
// the selected account). The rule: only the account owner or an admin grantee
// may add a category for the account, and the category is created owned by the
// ACCOUNT OWNER (so it is visible to the owner and co-sharers).

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

const sharedAcctID = "aaaa3333-0000-0000-0000-0000000000a3"

// roles (admin=0, user=1, guest=2) — matches connection.Role.
const (
	roleAdmin = 0
	roleUser  = 1
	roleGuest = 2
)

// seedSharedAccount seeds an account owned by otherUserID and (optionally) grants
// the seed user the given role on it.
func (h *harness) seedSharedAccount(t *testing.T, role int, grant bool) {
	t.Helper()
	f := fixture.New(t, h.tdb).WithCrypto(testDataSalt)
	f.Account(fixture.Account{ID: sharedAcctID, UserID: otherUserID, Name: "Shared"})
	if grant {
		f.AccountAccess(sharedAcctID, seedUserID, role)
	}
}

func createReqWithAccount(id, name, acctID string) map[string]any {
	return map[string]any{"id": id, "name": name, "type": "expense", "icon": "plane", "accountId": acctID}
}

func TestCreateCategory_SharedAccount_AdminRole_OwnedByAccountOwner(t *testing.T) {
	h := newHarness(t)
	h.seedSharedAccount(t, roleAdmin, true)
	tok := h.issueToken(t)
	const newCat = "c0000000-0000-0000-0000-0000000000a1"
	status, env := h.do(t, http.MethodPost, "/api/v1/category/create-category", tok, createReqWithAccount(newCat, "Travel", sharedAcctID))
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	item := mustUnmarshal[struct {
		Item categoryItem `json:"item"`
	}](t, env.Data).Item
	// Creating for an account assigns ownership to the account owner.
	if item.OwnerUserID != otherUserID {
		t.Fatalf("ownerUserId = %q, want account owner %q", item.OwnerUserID, otherUserID)
	}
}

func TestCreateCategory_SharedAccount_UserRole_Denied(t *testing.T) {
	h := newHarness(t)
	h.seedSharedAccount(t, roleUser, true)
	tok := h.issueToken(t)
	const newCat = "c0000000-0000-0000-0000-0000000000a2"
	status, env := h.do(t, http.MethodPost, "/api/v1/category/create-category", tok, createReqWithAccount(newCat, "Travel", sharedAcctID))
	assertAccessDenied(t, status, env)
}

func TestCreateCategory_SharedAccount_NoGrant_Denied(t *testing.T) {
	h := newHarness(t)
	h.seedSharedAccount(t, 0, false) // account owned by another user, not shared to caller
	tok := h.issueToken(t)
	const newCat = "c0000000-0000-0000-0000-0000000000a3"
	status, env := h.do(t, http.MethodPost, "/api/v1/category/create-category", tok, createReqWithAccount(newCat, "Travel", sharedAcctID))
	assertAccessDenied(t, status, env)
}

// TestCreateCategory_OwnAccount_OwnedByCaller guards the owner path: an accountId
// the caller owns creates the category owned by the caller (as before).
func TestCreateCategory_OwnAccount_OwnedByCaller(t *testing.T) {
	h := newHarness(t)
	f := fixture.New(t, h.tdb).WithCrypto(testDataSalt)
	const ownAcct = "aaaa4444-0000-0000-0000-0000000000a4"
	f.Account(fixture.Account{ID: ownAcct, UserID: seedUserID, Name: "Mine"})
	tok := h.issueToken(t)
	const newCat = "c0000000-0000-0000-0000-0000000000a4"
	status, env := h.do(t, http.MethodPost, "/api/v1/category/create-category", tok, createReqWithAccount(newCat, "Travel", ownAcct))
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	item := mustUnmarshal[struct {
		Item categoryItem `json:"item"`
	}](t, env.Data).Item
	if item.OwnerUserID != seedUserID {
		t.Fatalf("ownerUserId = %q, want caller %q", item.OwnerUserID, seedUserID)
	}
}

// assertAccessDenied checks the access-denied envelope: HTTP 403, success false,
// message "Access is not allowed".
func assertAccessDenied(t *testing.T, status int, env envelope) {
	t.Helper()
	if status != http.StatusForbidden {
		t.Fatalf("status=%d want 403 (access denied); body: %s", status, env.raw)
	}
	if env.Success {
		t.Fatalf("success=true, want false; body: %s", env.raw)
	}
	if env.Message != "Access is not allowed" {
		t.Fatalf("message = %q, want %q", env.Message, "Access is not allowed")
	}
}
