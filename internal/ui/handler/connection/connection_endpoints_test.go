package connection_test

import (
	"net/http"
	"regexp"
	"testing"
)

// apiDatetime matches the wire datetime format "2006-01-02 15:04:05".
var apiDatetime = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`)

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

// inviteResult is the {item:{code,expiredAt}} generate-invite response.
type inviteResult struct {
	Item struct {
		Code      string `json:"code"`
		ExpiredAt string `json:"expiredAt"`
	} `json:"item"`
}

func TestGenerateInvite_ReturnsCodeAndExpiry(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t, ownerUserID, ownerEmail)

	status, env := h.do(t, http.MethodPost, "/api/v1/connection/generate-invite", tok, map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200; body: %s", status, env.raw)
	}
	res := mustUnmarshal[inviteResult](t, env.Data)
	if len([]rune(res.Item.Code)) != 5 {
		t.Fatalf("code = %q, want a 5-char code", res.Item.Code)
	}
	if !apiDatetime.MatchString(res.Item.ExpiredAt) {
		t.Fatalf("expiredAt = %q, want Y-m-d H:i:s", res.Item.ExpiredAt)
	}
	// The invite is persisted: a second generate refreshes (still valid, may differ).
	_, env2 := h.do(t, http.MethodPost, "/api/v1/connection/generate-invite", tok, map[string]any{})
	res2 := mustUnmarshal[inviteResult](t, env2.Data)
	if len([]rune(res2.Item.Code)) != 5 {
		t.Fatalf("second code = %q, want a 5-char code", res2.Item.Code)
	}
}

func TestDeleteInvite_ClearsCode(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t, ownerUserID, ownerEmail)

	// Generate, then delete.
	_, gen := h.do(t, http.MethodPost, "/api/v1/connection/generate-invite", tok, map[string]any{})
	code := mustUnmarshal[inviteResult](t, gen.Data).Item.Code

	status, env := h.do(t, http.MethodPost, "/api/v1/connection/delete-invite", tok, map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("delete-invite status=%d want 200; body: %s", status, env.raw)
	}
	if string(env.Data) != "{}" {
		t.Fatalf("delete-invite data=%s want {}", env.Data)
	}

	// The code is now cleared: a third user trying to accept it gets a 400.
	thirdTok := h.token(t, thirdUserID, thirdEmail)
	st, _ := h.do(t, http.MethodPost, "/api/v1/connection/accept-invite", thirdTok, map[string]any{"code": code})
	if st != http.StatusBadRequest {
		t.Fatalf("accept after delete status=%d want 400 (code cleared)", st)
	}
}

func TestAcceptInvite_ConnectsUsers(t *testing.T) {
	h := newHarness(t)
	ownerTok := h.token(t, ownerUserID, ownerEmail)
	thirdTok := h.token(t, thirdUserID, thirdEmail)

	// Owner generates an invite; the (unconnected) third user accepts it.
	_, gen := h.do(t, http.MethodPost, "/api/v1/connection/generate-invite", ownerTok, map[string]any{})
	code := mustUnmarshal[inviteResult](t, gen.Data).Item.Code

	status, env := h.do(t, http.MethodPost, "/api/v1/connection/accept-invite", thirdTok, map[string]any{"code": code})
	if status != http.StatusOK {
		t.Fatalf("accept status=%d want 200; body: %s", status, env.raw)
	}
	// The response is the third user's connection list, now including the owner.
	res := mustUnmarshal[listResult](t, env.Data)
	var sawOwner bool
	for _, c := range res.Items {
		if c.User.ID == ownerUserID {
			sawOwner = true
		}
	}
	if !sawOwner {
		t.Fatalf("accept result must list the owner as a connection; got %+v", res.Items)
	}
	// The symmetric link exists: owner's connection list now includes the third user.
	_, ownerList := h.do(t, http.MethodGet, "/api/v1/connection/get-connection-list", ownerTok, nil)
	ol := mustUnmarshal[listResult](t, ownerList.Data)
	var sawThird bool
	for _, c := range ol.Items {
		if c.User.ID == thirdUserID {
			sawThird = true
		}
	}
	if !sawThird {
		t.Fatalf("owner's connection list must include the third user after accept")
	}
}

func TestAcceptInvite_BadCodeLength_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t, thirdUserID, thirdEmail)
	status, _ := h.do(t, http.MethodPost, "/api/v1/connection/accept-invite", tok, map[string]any{"code": "toolong"})
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (code must be 5 chars)", status)
	}
}

func TestAcceptInvite_Blank_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t, thirdUserID, thirdEmail)
	status, env := h.do(t, http.MethodPost, "/api/v1/connection/accept-invite", tok, map[string]any{"code": ""})
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400; body: %s", status, env.raw)
	}
}

func TestAcceptInvite_SelfInvite_400(t *testing.T) {
	h := newHarness(t)
	ownerTok := h.token(t, ownerUserID, ownerEmail)
	_, gen := h.do(t, http.MethodPost, "/api/v1/connection/generate-invite", ownerTok, map[string]any{})
	code := mustUnmarshal[inviteResult](t, gen.Data).Item.Code

	// Owner accepting their own invite is rejected ("Inviting yourself?").
	status, _ := h.do(t, http.MethodPost, "/api/v1/connection/accept-invite", ownerTok, map[string]any{"code": code})
	if status != http.StatusBadRequest {
		t.Fatalf("self-invite status=%d want 400", status)
	}
}

func TestDeleteConnection_RemovesLink(t *testing.T) {
	h := newHarness(t)
	ownerTok := h.token(t, ownerUserID, ownerEmail)

	// Owner and guest are connected in the seed; delete the connection.
	status, env := h.do(t, http.MethodPost, "/api/v1/connection/delete-connection", ownerTok, map[string]any{"id": guestUserID})
	if status != http.StatusOK {
		t.Fatalf("delete-connection status=%d want 200; body: %s", status, env.raw)
	}
	if string(env.Data) != "{}" {
		t.Fatalf("delete-connection data=%s want {}", env.Data)
	}

	// Both directions of the link are gone: neither lists the other.
	_, ol := h.do(t, http.MethodGet, "/api/v1/connection/get-connection-list", ownerTok, nil)
	if items := mustUnmarshal[listResult](t, ol.Data).Items; len(items) != 0 {
		t.Fatalf("owner still has %d connections after delete, want 0", len(items))
	}
	guestTok := h.token(t, guestUserID, guestEmail)
	_, gl := h.do(t, http.MethodGet, "/api/v1/connection/get-connection-list", guestTok, nil)
	if items := mustUnmarshal[listResult](t, gl.Data).Items; len(items) != 0 {
		t.Fatalf("guest still has %d connections after delete, want 0", len(items))
	}
}

func TestDeleteConnection_Self_400(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t, ownerUserID, ownerEmail)
	status, _ := h.do(t, http.MethodPost, "/api/v1/connection/delete-connection", tok, map[string]any{"id": ownerUserID})
	if status != http.StatusBadRequest {
		t.Fatalf("self-delete status=%d want 400", status)
	}
}

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
	// No grant exists -> domain NotFound, which the error map sends as 400
	// (documented convention).
	status, _ := h.do(t, http.MethodPost, "/api/v1/connection/revoke-account-access", tok, map[string]any{"accountId": ownerAccount, "userId": guestUserID})
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (no grant to revoke)", status)
	}
}

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
