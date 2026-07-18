package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// seedGrant inserts an accounts_access row (role: admin=0/user=1/guest=2). role
// is stored verbatim, so role=0 (admin) is faithfully represented.
func (h *harness) seedGrant(t *testing.T, accountID, userID string, role int) {
	t.Helper()
	fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"}).AccountAccess(accountID, userID, role)
}

// sharedAccessItem mirrors one sharedAccess[] entry: {user:{id,avatar,name}, role, isAccepted}.
type sharedAccessItem struct {
	User struct {
		ID     string `json:"id"`
		Avatar string `json:"avatar"`
		Name   string `json:"name"`
	} `json:"user"`
	Role       string `json:"role"`
	IsAccepted int    `json:"isAccepted"`
}

type accountWithShared struct {
	Id           string             `json:"id"`
	SharedAccess []sharedAccessItem `json:"sharedAccess"`
}

// An account owned by the seed user, shared with the other user, lists that
// grant in sharedAccess[] (resolved user + role alias).
func TestGetAccountList_SharedAccessPopulated(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	acctID, _ := h.createAccount(t, acctID1, "Cash", "0")
	// Grant the other user "user" role (1) on the seed user's account.
	h.seedGrant(t, acctID, otherUserID, 1)

	_, env := h.do(t, http.MethodGet, "/api/v1/account/get-account-list", tok, nil)
	var wrap struct {
		Items []accountWithShared `json:"items"`
	}
	mustDecode(t, env.Data, &wrap)
	if len(wrap.Items) != 1 {
		t.Fatalf("items=%d want 1", len(wrap.Items))
	}
	sa := wrap.Items[0].SharedAccess
	if len(sa) != 1 {
		t.Fatalf("sharedAccess=%+v want 1 entry; body=%s", sa, env.raw)
	}
	if sa[0].User.ID != otherUserID {
		t.Fatalf("shared user id=%q want %q", sa[0].User.ID, otherUserID)
	}
	if sa[0].Role != "user" {
		t.Fatalf("role=%q want user", sa[0].Role)
	}
}

// sharedAccess[] carries isAccepted: 1 for an accepted grant and 0 for a
// pending one, distinguishing the two.
func TestGetAccountList_SharedAccessIsAccepted(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	acctID, _ := h.createAccount(t, acctID1, "Cash", "0")
	pendingUserID := h.f.User(fixture.User{Name: "Pending User"})
	h.seedGrant(t, acctID, otherUserID, 1)
	h.f.AccountAccessPending(acctID, pendingUserID, 1)

	_, env := h.do(t, http.MethodGet, "/api/v1/account/get-account-list", tok, nil)
	var wrap struct {
		Items []accountWithShared `json:"items"`
	}
	mustDecode(t, env.Data, &wrap)
	if len(wrap.Items) != 1 {
		t.Fatalf("items=%d want 1", len(wrap.Items))
	}
	sa := wrap.Items[0].SharedAccess
	if len(sa) != 2 {
		t.Fatalf("sharedAccess=%+v want 2 entries; body=%s", sa, env.raw)
	}
	byUser := map[string]int{}
	for _, e := range sa {
		byUser[e.User.ID] = e.IsAccepted
	}
	if byUser[otherUserID] != 1 {
		t.Fatalf("accepted grant isAccepted=%d want 1", byUser[otherUserID])
	}
	if byUser[pendingUserID] != 0 {
		t.Fatalf("pending grant isAccepted=%d want 0", byUser[pendingUserID])
	}
}

// A non-owner WITH a grant deleting the account drops their own access (200) and
// the grant is gone — instead of the owner-only 403.
func TestDeleteAccount_NotOwnedButGranted_RevokesOwnAccess(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	// Account owned by the OTHER user, shared with the seed user (admin grant so
	// canDeleteAccount=hasAccess is satisfied).
	h.seedAccount(t, acctID2, otherUserID, "Theirs")
	h.seedGrant(t, acctID2, seedUserID, 0)

	status, _ := h.do(t, http.MethodPost, "/api/v1/account/delete-account", tok, map[string]any{"id": acctID2})
	if status != http.StatusOK {
		t.Fatalf("delete shared = %d, want 200 (revoke own access)", status)
	}

	// The grant is gone.
	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM accounts_access WHERE account_id = ? AND user_id = ?`, acctID2, seedUserID).Scan(&n); err != nil {
		t.Fatalf("count grants: %v", err)
	}
	if n != 0 {
		t.Fatalf("grant count=%d want 0 (revoked)", n)
	}
	// The account itself still exists (not deleted — only the grant was revoked).
	var del int
	h.db.QueryRow(`SELECT is_deleted FROM accounts WHERE id = ?`, acctID2).Scan(&del)
	if del != 0 {
		t.Fatalf("account is_deleted=%d want 0 (only access revoked)", del)
	}
}

// The account list includes accounts shared with the user (own + shared via
// accounts_access).
func TestGetAccountList_IncludesSharedAccounts(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	ownID, _ := h.createAccount(t, acctID1, "Mine", "0")
	// An account owned by the other user, shared with the seed user.
	h.seedAccount(t, acctID2, otherUserID, "Shared")
	h.seedGrant(t, acctID2, seedUserID, 1)

	_, env := h.do(t, http.MethodGet, "/api/v1/account/get-account-list", tok, nil)
	var wrap struct {
		Items []accountWithShared `json:"items"`
	}
	mustDecode(t, env.Data, &wrap)
	ids := map[string]bool{}
	for _, it := range wrap.Items {
		ids[it.Id] = true
	}
	if !ids[ownID] || !ids[acctID2] {
		t.Fatalf("list ids=%v want both own (%s) + shared (%s)", ids, ownID, acctID2)
	}
}

// accountListItem is accountWithShared plus the folder/position/balance fields
// needed to pin the pending-grant "inert entry" shape.
type accountListItem struct {
	Id           string             `json:"id"`
	FolderId     *string            `json:"folderId"`
	Position     int                `json:"position"`
	Balance      string             `json:"balance"`
	SharedAccess []sharedAccessItem `json:"sharedAccess"`
}

// A PENDING grant is inert everywhere except the recipient's own account list:
// get-account-list still surfaces the account for the grantee, as an inert
// placeholder entry (folderId null, position 0, balance "0"), with the
// grantee's own sharedAccess entry carrying isAccepted: 0 — so they can see and
// act on the invite. Once the grant is accepted and the grantee has real
// folder/position rows, the account carries those instead.
func TestGetAccountList_PendingGrant_RidesListThenShowsRealFolderOnceAccepted(t *testing.T) {
	h := newHarness(t)
	acctID, _ := h.createAccount(t, acctID1, "Cash", "100")
	h.f.AccountAccessPending(acctID, otherUserID, 1)

	granteeTok := h.tokenFor(t, otherUserID, "other@example.test")

	_, env := h.do(t, http.MethodGet, "/api/v1/account/get-account-list", granteeTok, nil)
	var wrap struct {
		Items []accountListItem `json:"items"`
	}
	mustDecode(t, env.Data, &wrap)
	if len(wrap.Items) != 1 {
		t.Fatalf("items=%d want 1 (pending account rides the grantee's list); body=%s", len(wrap.Items), env.raw)
	}
	item := wrap.Items[0]
	if item.Id != acctID {
		t.Fatalf("id=%q want %q", item.Id, acctID)
	}
	if item.FolderId != nil {
		t.Fatalf("folderId=%v want nil for a pending account", *item.FolderId)
	}
	if item.Position != 0 {
		t.Fatalf("position=%d want 0 for a pending account", item.Position)
	}
	if item.Balance != "0" {
		t.Fatalf("balance=%q want \"0\" for a pending account (inert)", item.Balance)
	}
	found := false
	for _, sa := range item.SharedAccess {
		if sa.User.ID == otherUserID {
			found = true
			if sa.IsAccepted != 0 {
				t.Fatalf("my sharedAccess entry isAccepted=%d want 0 (pending)", sa.IsAccepted)
			}
		}
	}
	if !found {
		t.Fatalf("sharedAccess=%+v missing my own (pending) entry", item.SharedAccess)
	}

	// Accept the grant and seed real folder/position for the grantee, mirroring
	// how the apiparity fixture seeds an accepted shared account (its own folder
	// + accounts_options row) — the account should now carry those instead of
	// the inert null/0 placeholders.
	if _, err := h.db.Exec(`UPDATE accounts_access SET is_accepted = 1 WHERE account_id = ? AND user_id = ?`, acctID, otherUserID); err != nil {
		t.Fatalf("accept grant: %v", err)
	}
	const granteeFolderID = "ffffffff-0000-0000-0000-00000000f02e"
	h.f.Folder(fixture.Folder{ID: granteeFolderID, UserID: otherUserID, Name: "Mine", Position: 0})
	h.f.AccountInFolder(granteeFolderID, acctID)
	h.f.AccountOption(acctID, otherUserID, 3)

	_, env2 := h.do(t, http.MethodGet, "/api/v1/account/get-account-list", granteeTok, nil)
	var wrap2 struct {
		Items []accountListItem `json:"items"`
	}
	mustDecode(t, env2.Data, &wrap2)
	if len(wrap2.Items) != 1 {
		t.Fatalf("items=%d want 1 after accepting; body=%s", len(wrap2.Items), env2.raw)
	}
	item2 := wrap2.Items[0]
	if item2.FolderId == nil || *item2.FolderId != granteeFolderID {
		t.Fatalf("folderId=%v want %q after accepting", item2.FolderId, granteeFolderID)
	}
	if item2.Position != 3 {
		t.Fatalf("position=%d want 3 after accepting", item2.Position)
	}
}
