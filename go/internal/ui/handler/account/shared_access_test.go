package account_test

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// seedGrant inserts an accounts_access row (role: admin=0/user=1/guest=2).
func (h *harness) seedGrant(t *testing.T, accountID, userID string, role int) {
	t.Helper()
	now := time.Unix(1690000000, 0).UTC()
	if _, err := h.db.ExecContext(context.Background(),
		`INSERT INTO accounts_access (account_id, user_id, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		accountID, userID, role, now, now,
	); err != nil {
		t.Fatalf("seed grant: %v", err)
	}
}

// sharedAccessItem mirrors one sharedAccess[] entry: {user:{id,avatar,name}, role}.
type sharedAccessItem struct {
	User struct {
		ID     string `json:"id"`
		Avatar string `json:"avatar"`
		Name   string `json:"name"`
	} `json:"user"`
	Role string `json:"role"`
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

// An available-accounts list now includes accounts shared with the user (own +
// shared via accounts_access), matching getAvailableForUserId.
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
