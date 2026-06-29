package tag_test

// Coverage for creating a tag in the context of an account SHARED with the
// caller. The create-tag request carries an accountId — the transaction modal
// sends it when a tag is added inline while entering a transaction on the
// selected account. Rules: only the account owner or an admin grantee may add a
// tag for the account, and the tag is created owned by the ACCOUNT OWNER (so it
// is visible to the owner and co-sharers). Regression guard against ignoring
// accountId (creating for the caller with no access check).

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

const sharedAcctID = "aaaa3333-0000-0000-0000-0000000000a3"

// roles (admin=0, user=1, guest=2) — matches domain/connection.Role.
const (
	roleAdmin = 0
	roleUser  = 1
	roleGuest = 2
)

func (h *harness) seedSharedAccount(t *testing.T, role int, grant bool) {
	t.Helper()
	f := fixture.New(t, h.tdb).WithCrypto(testDataSalt)
	f.Account(fixture.Account{ID: sharedAcctID, UserID: otherUserID, Name: "Shared"})
	if grant {
		f.AccountAccess(sharedAcctID, seedUserID, role)
	}
}

func createTagReqWithAccount(id, name, acctID string) map[string]any {
	return map[string]any{"id": id, "name": name, "accountId": acctID}
}

func TestCreateTag_SharedAccount_AdminRole_OwnedByAccountOwner(t *testing.T) {
	h := newHarness(t)
	h.seedSharedAccount(t, roleAdmin, true)
	tok := h.issueToken(t)
	const newTag = "10000000-0000-0000-0000-0000000000a1"
	status, env := h.do(t, http.MethodPost, "/api/v1/tag/create-tag", tok, createTagReqWithAccount(newTag, "Urgent", sharedAcctID))
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	item := mustUnmarshal[struct {
		Item tagItem `json:"item"`
	}](t, env.Data).Item
	if item.OwnerUserID != otherUserID {
		t.Fatalf("ownerUserId = %q, want account owner %q", item.OwnerUserID, otherUserID)
	}
}

func TestCreateTag_SharedAccount_UserRole_Denied(t *testing.T) {
	h := newHarness(t)
	h.seedSharedAccount(t, roleUser, true)
	tok := h.issueToken(t)
	const newTag = "10000000-0000-0000-0000-0000000000a2"
	status, env := h.do(t, http.MethodPost, "/api/v1/tag/create-tag", tok, createTagReqWithAccount(newTag, "Urgent", sharedAcctID))
	assertAccessDenied(t, status, env)
}

func TestCreateTag_SharedAccount_NoGrant_Denied(t *testing.T) {
	h := newHarness(t)
	h.seedSharedAccount(t, 0, false)
	tok := h.issueToken(t)
	const newTag = "10000000-0000-0000-0000-0000000000a3"
	status, env := h.do(t, http.MethodPost, "/api/v1/tag/create-tag", tok, createTagReqWithAccount(newTag, "Urgent", sharedAcctID))
	assertAccessDenied(t, status, env)
}

func TestCreateTag_OwnAccount_OwnedByCaller(t *testing.T) {
	h := newHarness(t)
	f := fixture.New(t, h.tdb).WithCrypto(testDataSalt)
	const ownAcct = "aaaa4444-0000-0000-0000-0000000000a4"
	f.Account(fixture.Account{ID: ownAcct, UserID: seedUserID, Name: "Mine"})
	tok := h.issueToken(t)
	const newTag = "10000000-0000-0000-0000-0000000000a4"
	status, env := h.do(t, http.MethodPost, "/api/v1/tag/create-tag", tok, createTagReqWithAccount(newTag, "Urgent", ownAcct))
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	item := mustUnmarshal[struct {
		Item tagItem `json:"item"`
	}](t, env.Data).Item
	if item.OwnerUserID != seedUserID {
		t.Fatalf("ownerUserId = %q, want caller %q", item.OwnerUserID, seedUserID)
	}
}

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
