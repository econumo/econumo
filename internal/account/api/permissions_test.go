package api_test

// Cross-tenant permission tests for the account + folder module: a user must not
// be able to read or mutate another user's accounts/folders unless explicitly
// shared with an adequate role. The JWT subject (seedUserID) plays the attacker;
// otherUserID owns the victim resources. Positive counterparts (owner CAN do it)
// live in account_endpoints_test.go / folder_endpoints_test.go.

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	victimAccountID = "aaaa9999-0000-0000-0000-0000000000a9"
	victimFolderID  = "ffffffff-0000-0000-0000-00000000f0aa"
)

// assertDenied checks the account-module access-denied envelope: 403, success
// false, message "Access denied" (errs.NewAccessDenied("Access denied")).
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

func (h *harness) seedFolder(t *testing.T, id, ownerID, name string) {
	t.Helper()
	h.f.Folder(fixture.Folder{ID: id, UserID: ownerID, Name: name, Position: 0})
}

func TestUpdateAccount_NonOwner_403(t *testing.T) {
	h := newHarness(t)
	h.seedAccount(t, victimAccountID, otherUserID, "Theirs")
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/account/update-account", tok, map[string]any{
		"id": victimAccountID, "name": "Hijacked", "balance": "0", "icon": "wallet",
		"updatedAt": "2024-01-01 12:00:00",
	})
	assertDenied(t, status, env)
}

func TestDeleteAccount_StrangerNoGrant_403(t *testing.T) {
	h := newHarness(t)
	h.seedAccount(t, victimAccountID, otherUserID, "Theirs")
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/account/delete-account", tok, map[string]any{"id": victimAccountID})
	assertDenied(t, status, env)
}

func TestUpdateFolder_NonOwner_403(t *testing.T) {
	h := newHarness(t)
	h.seedFolder(t, victimFolderID, otherUserID, "Theirs")
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/account/update-folder", tok, map[string]any{
		"id": victimFolderID, "name": "Hijacked",
	})
	assertDenied(t, status, env)
}

func TestHideFolder_NonOwner_403(t *testing.T) {
	h := newHarness(t)
	h.seedFolder(t, victimFolderID, otherUserID, "Theirs")
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/account/hide-folder", tok, map[string]any{"id": victimFolderID})
	assertDenied(t, status, env)
}

func TestReplaceFolder_ForeignSource_403(t *testing.T) {
	h := newHarness(t)
	h.seedFolder(t, victimFolderID, otherUserID, "Theirs")
	tok := h.token(t)
	// replaceId is the caller's own seeded folder; id is the victim's folder.
	status, env := h.do(t, http.MethodPost, "/api/v1/account/replace-folder", tok, map[string]any{
		"id": victimFolderID, "replaceId": seedFolderID,
	})
	assertDenied(t, status, env)
}

func TestCreateAccount_ForeignFolder_403(t *testing.T) {
	h := newHarness(t)
	h.seedFolder(t, victimFolderID, otherUserID, "Theirs")
	tok := h.token(t)
	status, env := h.do(t, http.MethodPost, "/api/v1/account/create-account", tok, map[string]any{
		"id": "aaaa8888-0000-0000-0000-0000000000a8", "name": "Sneaky", "currencyId": usdID,
		"balance": "0", "icon": "wallet", "folderId": victimFolderID,
	})
	assertDenied(t, status, env)
}

func TestGetAccountList_ExcludesUnsharedAccount(t *testing.T) {
	h := newHarness(t)
	h.seedAccount(t, victimAccountID, otherUserID, "Theirs")
	tok := h.token(t)
	_, env := h.do(t, http.MethodGet, "/api/v1/account/get-account-list", tok, nil)
	list := mustUnmarshal[accountItemsWrapper](t, env.Data)
	for _, it := range list.Items {
		if it.ID == victimAccountID {
			t.Fatalf("get-account-list leaked another user's unshared account %q; body: %s", victimAccountID, env.raw)
		}
	}
}
