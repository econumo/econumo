package connection_test

import (
	"net/http"
	"strings"
	"testing"
)

type accessItem struct {
	ID          string `json:"id"`
	OwnerUserID string `json:"ownerUserId"`
	Role        string `json:"role"`
}

type connItem struct {
	User struct {
		ID, Avatar, Name string
	} `json:"user"`
	SharedAccounts []accessItem `json:"sharedAccounts"`
}

type listResult struct {
	Items []connItem `json:"items"`
}

// --- 501 stubs (self-hosted) ---

func TestInviteAndConnectionStubs_501(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t, ownerUserID, ownerEmail)
	for _, path := range []string{
		"/api/v1/connection/generate-invite",
		"/api/v1/connection/delete-invite",
		"/api/v1/connection/accept-invite",
		"/api/v1/connection/delete-connection",
	} {
		status, raw := h.doRaw(t, http.MethodPost, path, tok, map[string]any{"code": "abcde", "id": guestUserID})
		if status != http.StatusNotImplemented {
			t.Fatalf("%s status=%d want 501; body: %s", path, status, raw)
		}
		// 501 envelope emits errors as [] (not {}) -- decode loosely.
		body := string(raw)
		if !strings.Contains(body, `"success":false`) || !strings.Contains(body, "Not supported in Econumo One") || !strings.Contains(body, `"errors":[]`) {
			t.Fatalf("%s 501 envelope = %s, want not-supported + errors:[]", path, body)
		}
	}
}

// --- set-account-access ---

func TestSetAccountAccess_GrantsToConnectedUser(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t, ownerUserID, ownerEmail)
	status, env := h.do(t, http.MethodPost, "/api/v1/connection/set-account-access", tok, map[string]any{
		"accountId": ownerAccount, "userId": guestUserID, "role": "user",
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	// grant row exists with role=1 (user).
	var role int
	if err := h.db.QueryRow(`SELECT role FROM accounts_access WHERE account_id=? AND user_id=?`, ownerAccount, guestUserID).Scan(&role); err != nil {
		t.Fatalf("grant row not found: %v", err)
	}
	if role != 1 {
		t.Fatalf("role=%d want 1 (user)", role)
	}
	// guest got an accounts_options row (position seeded) ...
	var pos int
	if err := h.db.QueryRow(`SELECT position FROM accounts_options WHERE account_id=? AND user_id=?`, ownerAccount, guestUserID).Scan(&pos); err != nil {
		t.Fatalf("guest options row not found: %v", err)
	}
	// ... and the account was added to the guest's last folder.
	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM accounts_folders WHERE folder_id=? AND account_id=?`, guestFolderID, ownerAccount).Scan(&n)
	if n != 1 {
		t.Fatalf("account not added to guest folder (count=%d)", n)
	}
}

func TestSetAccountAccess_UpdatesExistingRole(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t, ownerUserID, ownerEmail)
	h.do(t, http.MethodPost, "/api/v1/connection/set-account-access", tok, map[string]any{"accountId": ownerAccount, "userId": guestUserID, "role": "user"})
	status, env := h.do(t, http.MethodPost, "/api/v1/connection/set-account-access", tok, map[string]any{"accountId": ownerAccount, "userId": guestUserID, "role": "admin"})
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	var role int
	h.db.QueryRow(`SELECT role FROM accounts_access WHERE account_id=? AND user_id=?`, ownerAccount, guestUserID).Scan(&role)
	if role != 0 {
		t.Fatalf("role=%d want 0 (admin)", role)
	}
}

func TestSetAccountAccess_NonOwner_403(t *testing.T) {
	h := newHarness(t)
	// guest does not own the account and has no admin grant -> access denied.
	tok := h.token(t, guestUserID, guestEmail)
	status, env := h.do(t, http.MethodPost, "/api/v1/connection/set-account-access", tok, map[string]any{
		"accountId": ownerAccount, "userId": ownerUserID, "role": "user",
	})
	if status != http.StatusForbidden {
		t.Fatalf("status=%d want 403; body: %s", status, env.raw)
	}
}

func TestSetAccountAccess_BadRole_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t, ownerUserID, ownerEmail)
	status, _ := h.do(t, http.MethodPost, "/api/v1/connection/set-account-access", tok, map[string]any{
		"accountId": ownerAccount, "userId": guestUserID, "role": "superuser",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", status)
	}
}

func TestSetAccountAccess_Blank_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t, ownerUserID, ownerEmail)
	status, _ := h.do(t, http.MethodPost, "/api/v1/connection/set-account-access", tok, map[string]any{"accountId": "", "userId": "", "role": ""})
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", status)
	}
}

// --- revoke-account-access ---

func TestRevokeAccountAccess_RemovesGrantAndCleansUp(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t, ownerUserID, ownerEmail)
	h.do(t, http.MethodPost, "/api/v1/connection/set-account-access", tok, map[string]any{"accountId": ownerAccount, "userId": guestUserID, "role": "user"})

	status, env := h.do(t, http.MethodPost, "/api/v1/connection/revoke-account-access", tok, map[string]any{"accountId": ownerAccount, "userId": guestUserID})
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	var n int
	h.db.QueryRow(`SELECT COUNT(*) FROM accounts_access WHERE account_id=? AND user_id=?`, ownerAccount, guestUserID).Scan(&n)
	if n != 0 {
		t.Fatalf("grant still present (count=%d)", n)
	}
	h.db.QueryRow(`SELECT COUNT(*) FROM accounts_options WHERE account_id=? AND user_id=?`, ownerAccount, guestUserID).Scan(&n)
	if n != 0 {
		t.Fatalf("guest options still present (count=%d)", n)
	}
	h.db.QueryRow(`SELECT COUNT(*) FROM accounts_folders WHERE folder_id=? AND account_id=?`, guestFolderID, ownerAccount).Scan(&n)
	if n != 0 {
		t.Fatalf("account still in guest folder (count=%d)", n)
	}
}

func TestRevokeAccountAccess_Missing_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t, ownerUserID, ownerEmail)
	// No grant exists -> domain NotFound. The Go error map sends NotFound -> 400
	// (documented convention; PHP's bare NotFoundException is unhandled -> 500).
	status, _ := h.do(t, http.MethodPost, "/api/v1/connection/revoke-account-access", tok, map[string]any{"accountId": ownerAccount, "userId": guestUserID})
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (no grant to revoke)", status)
	}
}

// --- get-connection-list ---

func TestGetConnectionList_ReflectsSharedAccounts(t *testing.T) {
	h := newHarness(t)
	ownerTok := h.token(t, ownerUserID, ownerEmail)
	h.do(t, http.MethodPost, "/api/v1/connection/set-account-access", ownerTok, map[string]any{"accountId": ownerAccount, "userId": guestUserID, "role": "user"})

	// Owner's view: one connection (the guest), zero shared accounts FROM guest's
	// side (the grant is issued by owner; owner is the account owner, so it shows
	// under the connection as a shared account owned by owner).
	_, env := h.do(t, http.MethodGet, "/api/v1/connection/get-connection-list", ownerTok, nil)
	owl := mustUnmarshal[listResult](t, env.Data)
	if len(owl.Items) != 1 || owl.Items[0].User.ID != guestUserID {
		t.Fatalf("owner list = %+v, want one connection to guest", owl.Items)
	}
	if len(owl.Items[0].SharedAccounts) != 1 || owl.Items[0].SharedAccounts[0].ID != ownerAccount {
		t.Fatalf("owner shared = %+v, want the shared account", owl.Items[0].SharedAccounts)
	}
	if owl.Items[0].SharedAccounts[0].OwnerUserID != ownerUserID || owl.Items[0].SharedAccounts[0].Role != "user" {
		t.Fatalf("owner shared entry = %+v, want owner/user", owl.Items[0].SharedAccounts[0])
	}

	// Guest's view: one connection (the owner) with the received account.
	guestTok := h.token(t, guestUserID, guestEmail)
	_, env2 := h.do(t, http.MethodGet, "/api/v1/connection/get-connection-list", guestTok, nil)
	gl := mustUnmarshal[listResult](t, env2.Data)
	if len(gl.Items) != 1 || gl.Items[0].User.ID != ownerUserID {
		t.Fatalf("guest list = %+v, want one connection to owner", gl.Items)
	}
	if len(gl.Items[0].SharedAccounts) != 1 || gl.Items[0].SharedAccounts[0].ID != ownerAccount {
		t.Fatalf("guest shared = %+v, want the received account", gl.Items[0].SharedAccounts)
	}
}

func TestGetConnectionList_NoToken_401(t *testing.T) {
	h := newHarness(t)
	status, _ := h.do(t, http.MethodGet, "/api/v1/connection/get-connection-list", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", status)
	}
}
