package category_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

// usdID is the baseline USD currency (seeded by migration 20210812210548).
const usdID = fixture.USD

// seedAccount inserts an account owned by ownerID.
func (h *harness) seedAccount(t *testing.T, id, ownerID, name string) {
	t.Helper()
	fixture.New(t, h.tdb).Account(fixture.Account{
		ID:         id,
		UserID:     ownerID,
		CurrencyID: usdID,
		Name:       name,
	})
}

// seedGrant inserts an accounts_access row granting userID access to accountID.
func (h *harness) seedGrant(t *testing.T, accountID, userID string, role int) {
	t.Helper()
	fixture.New(t, h.tdb).AccountAccess(accountID, userID, role)
}

// TestGetCategoryList_IncludesSharedOwners verifies the list returns the user's
// OWN categories plus categories of users who shared an account WITH this user,
// matching PHP CategoryRepository::findAvailableForUserId. Regression for the
// api-compare finding where Go returned own-only (WHERE user_id = ?).
func TestGetCategoryList_IncludesSharedOwners(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	// The seed user owns one category.
	h.seedCategory(t, catID1, seedUserID, "Mine", 0, 1, false)
	// otherUser owns category catID2 and an account they SHARE with the seed user
	// -> catID2 must appear (the own+shared rule).
	h.seedCategory(t, catID2, otherUserID, "Shared", 0, 1, false)
	h.seedAccount(t, "33333333-3333-3333-3333-333333333333", otherUserID, "Other's account")
	h.seedGrant(t, "33333333-3333-3333-3333-333333333333", seedUserID, 1)
	// otherUser also owns catID3, but it is reachable only via the same shared
	// owner, so it appears too (PHP includes ALL of a sharing owner's categories).
	// To assert the negative (a NON-sharing owner is excluded) we revoke: catID3
	// stays owned by otherUser, so it is included. Instead, verify that WITHOUT a
	// grant the shared owner's categories are hidden — see the sibling subtest.

	status, env := h.do(t, http.MethodGet, "/api/v1/category/get-category-list", token, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	items := mustUnmarshal[itemsWrapper](t, env.Data).Items

	got := map[string]bool{}
	for _, it := range items {
		got[it.ID] = true
	}
	if !got[catID1] {
		t.Errorf("own category %s missing from list", catID1)
	}
	if !got[catID2] {
		t.Errorf("shared-owner category %s missing from list (own+shared not applied)", catID2)
	}
}

// TestGetCategoryList_ExcludesUnsharedOwners verifies that without an account
// grant, another user's categories do NOT appear — the negative of own+shared.
func TestGetCategoryList_ExcludesUnsharedOwners(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.seedCategory(t, catID1, seedUserID, "Mine", 0, 1, false)
	// otherUser owns a category but shares NO account with the seed user.
	h.seedCategory(t, catID2, otherUserID, "NotShared", 0, 1, false)

	status, env := h.do(t, http.MethodGet, "/api/v1/category/get-category-list", token, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	items := mustUnmarshal[itemsWrapper](t, env.Data).Items
	got := map[string]bool{}
	for _, it := range items {
		got[it.ID] = true
	}
	if !got[catID1] {
		t.Errorf("own category %s missing", catID1)
	}
	if got[catID2] {
		t.Errorf("category %s of a non-sharing user must NOT appear", catID2)
	}
}
