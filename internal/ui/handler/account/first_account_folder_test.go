package account_test

import (
	"net/http"
	"testing"
	"time"
)

// tokenFor issues a JWT for an arbitrary seeded user (h.token is seedUser-only).
func (h *harness) tokenFor(t *testing.T, userID, email string) string {
	t.Helper()
	tok, err := h.jwt.Issue(userID, email, time.Now())
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

// A brand-new user has no folders (registration/onboarding never creates one).
// Creating the very first account with no folderId must succeed by
// auto-creating a default folder and placing the account in it — otherwise
// onboarding's first "Add account" is impossible.
func TestCreateAccount_FirstAccount_AutoCreatesDefaultFolder(t *testing.T) {
	h := newHarness(t)
	// otherUserID is seeded with NO folder (see seedUsers).
	tok := h.tokenFor(t, otherUserID, "other@example.test")

	req := map[string]any{
		"id": acctID1, "name": "Cash", "currencyId": usdID,
		"balance": "0", "icon": "wallet",
		// folderId intentionally omitted — the client has no folder yet.
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/account/create-account", tok, req)
	if status != http.StatusOK {
		t.Fatalf("create-account = %d, want 200; body: %s", status, env.raw)
	}
	it := mustUnmarshal[accountItemWrapper](t, env.Data).Item
	if it.FolderID == nil || *it.FolderID == "" {
		t.Fatalf("folderId = %v, want the auto-created default folder id; body: %s", it.FolderID, env.raw)
	}

	// The user now owns exactly one folder, and the account lives in it.
	_, fenv := h.do(t, http.MethodGet, "/api/v1/account/get-folder-list", tok, nil)
	folders := mustUnmarshal[struct {
		Items []folderItem `json:"items"`
	}](t, fenv.Data).Items
	if len(folders) != 1 {
		t.Fatalf("folder count = %d, want 1; body: %s", len(folders), fenv.raw)
	}
	if folders[0].ID != *it.FolderID {
		t.Fatalf("account folderId %s != created folder %s", *it.FolderID, folders[0].ID)
	}
	if folders[0].Name != "General" {
		t.Fatalf("default folder name = %q, want General", folders[0].Name)
	}
}

// Once a user already owns a folder, a blank folderId stays a hard error (the
// frozen tier-1 contract) — auto-creation is strictly the zero-folder case.
func TestCreateAccount_BlankFolderId_RejectedWhenUserHasFolders(t *testing.T) {
	h := newHarness(t) // seedUser already owns "Main"
	tok := h.token(t)

	req := map[string]any{
		"id": acctID1, "name": "Cash", "currencyId": usdID, "balance": "0", "icon": "wallet",
	}
	status, env := h.do(t, http.MethodPost, "/api/v1/account/create-account", tok, req)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", status, env.raw)
	}
	if msgs := env.errorsMap()["folderId"]; len(msgs) != 1 || msgs[0] != "This value should not be blank." {
		t.Fatalf("folderId errors = %v, want [\"This value should not be blank.\"]; body: %s", msgs, env.raw)
	}
}
