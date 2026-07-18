package api_test

// Endpoint-envelope contract tests for the grant/accept/decline/revoke access
// handshake. Service-level semantics (folder auto-creation, position
// assignment, ...) are covered by internal/account/access_test.go; these tests
// pin the HTTP contract: status codes, envelope shape, and the get-account-list
// projection the grantee sees at each step.

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

// TestGrantAccess_Owner_Success grants a connected user access on an owned
// account; the new grant is pending, so it rides the grantee's list with a
// null folderId and their own sharedAccess entry carrying isAccepted 0.
func TestGrantAccess_Owner_Success(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	acctID, _ := h.createAccount(t, acctID1, "Cash", "0")

	status, env := h.do(t, http.MethodPost, "/api/v1/account/grant-access", tok, map[string]any{
		"accountId": acctID, "userId": otherUserID, "role": "user",
	})
	if status != http.StatusOK {
		t.Fatalf("grant-access = %d, want 200; body: %s", status, env.raw)
	}
	if string(env.Data) != "{}" {
		t.Fatalf("data=%s want {}", env.Data)
	}

	granteeTok := h.tokenFor(t, otherUserID, "other@example.test")
	_, listEnv := h.do(t, http.MethodGet, "/api/v1/account/get-account-list", granteeTok, nil)
	var wrap struct {
		Items []accountListItem `json:"items"`
	}
	mustDecode(t, listEnv.Data, &wrap)
	found := false
	for _, it := range wrap.Items {
		if it.Id != acctID {
			continue
		}
		found = true
		if it.FolderId != nil {
			t.Fatalf("folderId=%v want nil (pending)", *it.FolderId)
		}
		saFound := false
		for _, sa := range it.SharedAccess {
			if sa.User.ID == otherUserID {
				saFound = true
				if sa.IsAccepted != 0 {
					t.Fatalf("isAccepted=%d want 0 (pending)", sa.IsAccepted)
				}
				if sa.Role != "user" {
					t.Fatalf("role=%q want user", sa.Role)
				}
			}
		}
		if !saFound {
			t.Fatalf("sharedAccess missing grantee's own entry: %+v", it.SharedAccess)
		}
	}
	if !found {
		t.Fatalf("grantee list missing pending account %q; body: %s", acctID, listEnv.raw)
	}
}

// TestAcceptAccess_WithFolder_Success accepts a pending grant with an explicit
// folderId: the account moves from the inert placeholder to a real
// folder/position, and the grantee's sharedAccess entry flips to accepted.
func TestAcceptAccess_WithFolder_Success(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	acctID, _ := h.createAccount(t, acctID1, "Cash", "0")
	if status, env := h.do(t, http.MethodPost, "/api/v1/account/grant-access", tok, map[string]any{
		"accountId": acctID, "userId": otherUserID, "role": "user",
	}); status != http.StatusOK {
		t.Fatalf("grant-access = %d, want 200; body: %s", status, env.raw)
	}

	granteeTok := h.tokenFor(t, otherUserID, "other@example.test")
	const granteeFolderID = "ffffffff-0000-0000-0000-00000000f02f"
	h.f.Folder(fixture.Folder{ID: granteeFolderID, UserID: otherUserID, Name: "Mine", Position: 0})

	status, env := h.do(t, http.MethodPost, "/api/v1/account/accept-access", granteeTok, map[string]any{
		"accountId": acctID, "folderId": granteeFolderID,
	})
	if status != http.StatusOK {
		t.Fatalf("accept-access = %d, want 200; body: %s", status, env.raw)
	}
	if string(env.Data) != "{}" {
		t.Fatalf("data=%s want {}", env.Data)
	}

	_, listEnv := h.do(t, http.MethodGet, "/api/v1/account/get-account-list", granteeTok, nil)
	var wrap struct {
		Items []accountListItem `json:"items"`
	}
	mustDecode(t, listEnv.Data, &wrap)
	var item *accountListItem
	for i := range wrap.Items {
		if wrap.Items[i].Id == acctID {
			item = &wrap.Items[i]
		}
	}
	if item == nil {
		t.Fatalf("accepted account missing from grantee list; body: %s", listEnv.raw)
	}
	if item.FolderId == nil || *item.FolderId != granteeFolderID {
		t.Fatalf("folderId=%v want %q", item.FolderId, granteeFolderID)
	}
	saFound := false
	for _, sa := range item.SharedAccess {
		if sa.User.ID == otherUserID {
			saFound = true
			if sa.IsAccepted != 1 {
				t.Fatalf("isAccepted=%d want 1 (accepted)", sa.IsAccepted)
			}
		}
	}
	if !saFound {
		t.Fatalf("sharedAccess missing grantee's own entry: %+v", item.SharedAccess)
	}
}

// TestDeclineAccess_Success declines a fresh pending grant: the account drops
// out of the grantee's list entirely.
func TestDeclineAccess_Success(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	acctID, _ := h.createAccount(t, acctID1, "Cash", "0")
	if status, env := h.do(t, http.MethodPost, "/api/v1/account/grant-access", tok, map[string]any{
		"accountId": acctID, "userId": otherUserID, "role": "guest",
	}); status != http.StatusOK {
		t.Fatalf("grant-access = %d, want 200; body: %s", status, env.raw)
	}

	granteeTok := h.tokenFor(t, otherUserID, "other@example.test")
	status, env := h.do(t, http.MethodPost, "/api/v1/account/decline-access", granteeTok, map[string]any{
		"accountId": acctID,
	})
	if status != http.StatusOK {
		t.Fatalf("decline-access = %d, want 200; body: %s", status, env.raw)
	}
	if string(env.Data) != "{}" {
		t.Fatalf("data=%s want {}", env.Data)
	}

	_, listEnv := h.do(t, http.MethodGet, "/api/v1/account/get-account-list", granteeTok, nil)
	var wrap struct {
		Items []accountListItem `json:"items"`
	}
	mustDecode(t, listEnv.Data, &wrap)
	for _, it := range wrap.Items {
		if it.Id == acctID {
			t.Fatalf("declined account still present in grantee list; body: %s", listEnv.raw)
		}
	}
}

// TestRevokeAccess_Owner_Success revokes an accepted grant as the owner: the
// accounts_access row is gone.
func TestRevokeAccess_Owner_Success(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	acctID, _ := h.createAccount(t, acctID1, "Cash", "0")
	if status, env := h.do(t, http.MethodPost, "/api/v1/account/grant-access", tok, map[string]any{
		"accountId": acctID, "userId": otherUserID, "role": "user",
	}); status != http.StatusOK {
		t.Fatalf("grant-access = %d, want 200; body: %s", status, env.raw)
	}

	status, env := h.do(t, http.MethodPost, "/api/v1/account/revoke-access", tok, map[string]any{
		"accountId": acctID, "userId": otherUserID,
	})
	if status != http.StatusOK {
		t.Fatalf("revoke-access = %d, want 200; body: %s", status, env.raw)
	}
	if string(env.Data) != "{}" {
		t.Fatalf("data=%s want {}", env.Data)
	}

	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM accounts_access WHERE account_id = ? AND user_id = ?`, acctID, otherUserID).Scan(&n); err != nil {
		t.Fatalf("count grants: %v", err)
	}
	if n != 0 {
		t.Fatalf("grant count=%d want 0 (revoked)", n)
	}
}

// TestGrantAccess_Stranger_403: a caller with no ownership/admin grant on the
// account cannot grant access to anyone on it.
func TestGrantAccess_Stranger_403(t *testing.T) {
	h := newHarness(t)
	h.seedAccount(t, victimAccountID, otherUserID, "Theirs")
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/account/grant-access", tok, map[string]any{
		"accountId": victimAccountID, "userId": seedUserID, "role": "user",
	})
	assertDenied(t, status, env)
}

// TestGrantAccess_UnconnectedUser_403: an account may only be shared with a
// connected user, so granting to a stranger (existing but not connected) is
// denied.
func TestGrantAccess_UnconnectedUser_403(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	acctID, _ := h.createAccount(t, acctID1, "Cash", "0")
	const strangerID = "44444444-4444-4444-4444-444444444444"
	h.f.User(fixture.User{ID: strangerID, Email: "stranger@example.test", Name: "Stranger", Avatar: "https://avatar.test/s", Password: "pw", Salt: seedSalt})

	status, env := h.do(t, http.MethodPost, "/api/v1/account/grant-access", tok, map[string]any{
		"accountId": acctID, "userId": strangerID, "role": "user",
	})
	assertDenied(t, status, env)
}

// TestGrantAccess_Self_403: a self-grant is never allowed (you are not
// "connected" to yourself).
func TestGrantAccess_Self_403(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	acctID, _ := h.createAccount(t, acctID1, "Cash", "0")

	status, env := h.do(t, http.MethodPost, "/api/v1/account/grant-access", tok, map[string]any{
		"accountId": acctID, "userId": seedUserID, "role": "user",
	})
	assertDenied(t, status, env)
}

// TestAcceptAccess_NoPending_403: a caller with no grant at all (pending or
// otherwise) on the account cannot accept.
func TestAcceptAccess_NoPending_403(t *testing.T) {
	h := newHarness(t)
	h.seedAccount(t, victimAccountID, otherUserID, "Theirs")
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/account/accept-access", tok, map[string]any{
		"accountId": victimAccountID,
	})
	assertDenied(t, status, env)
}

// TestGrantAccess_BlankFields_400: blank accountId/userId/role each surface
// the standard NotBlank field error.
func TestGrantAccess_BlankFields_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/account/grant-access", tok, map[string]any{
		"accountId": "", "userId": "", "role": "",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400; body: %s", status, env.raw)
	}
	errsMap := env.errorsMap()
	for _, key := range []string{"accountId", "userId", "role"} {
		msgs := errsMap[key]
		if len(msgs) == 0 || msgs[0] != "This value should not be blank." {
			t.Fatalf("errors[%s]=%v want blank message; body: %s", key, msgs, env.raw)
		}
	}
}

// TestGrantAccess_UnknownRole_400: an unrecognized role alias fails validation
// with the exact frozen message.
func TestGrantAccess_UnknownRole_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	acctID, _ := h.createAccount(t, acctID1, "Cash", "0")
	status, env := h.do(t, http.MethodPost, "/api/v1/account/grant-access", tok, map[string]any{
		"accountId": acctID, "userId": otherUserID, "role": "xyz",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400; body: %s", status, env.raw)
	}
	msgs := env.errorsMap()["role"]
	if len(msgs) == 0 || msgs[0] != "AccountRole with alias xyz not exists" {
		t.Fatalf("errors[role]=%v; body: %s", msgs, env.raw)
	}
}
