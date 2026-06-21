package tag_test

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// usdID is the baseline USD currency (seeded by migration 20210812210548).
const usdID = "dffc2a06-6f29-4704-8575-31709adee926"

func (h *harness) seedAccount(t *testing.T, id, ownerID, name string) {
	t.Helper()
	now := time.Unix(1690000000, 0).UTC()
	if _, err := h.db.ExecContext(context.Background(),
		`INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 2, 'wallet', 0, ?, ?)`,
		id, usdID, ownerID, name, now, now,
	); err != nil {
		t.Fatalf("seed account %s: %v", id, err)
	}
}

func (h *harness) seedGrant(t *testing.T, accountID, userID string, role int) {
	t.Helper()
	now := time.Unix(1690000000, 0).UTC()
	if _, err := h.db.ExecContext(context.Background(),
		`INSERT INTO accounts_access (account_id, user_id, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		accountID, userID, role, now, now,
	); err != nil {
		t.Fatalf("seed grant %s->%s: %v", accountID, userID, err)
	}
}

// TestGetTagList_IncludesSharedOwners: own + tags of users who shared an account
// with this user (PHP TagRepository::findAvailableForUserId). Regression for the
// api-compare own-only gap.
func TestGetTagList_IncludesSharedOwners(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.seedTag(t, tagID1, seedUserID, "#mine", 0, false)
	h.seedTag(t, tagID2, otherUserID, "#shared", 0, false)
	h.seedAccount(t, "33333333-3333-3333-3333-333333333333", otherUserID, "Other's account")
	h.seedGrant(t, "33333333-3333-3333-3333-333333333333", seedUserID, 1)

	status, env := h.do(t, http.MethodGet, "/api/v1/tag/get-tag-list", token, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	got := map[string]bool{}
	for _, it := range mustUnmarshal[itemsWrapper](t, env.Data).Items {
		got[it.ID] = true
	}
	if !got[tagID1] {
		t.Errorf("own tag %s missing", tagID1)
	}
	if !got[tagID2] {
		t.Errorf("shared-owner tag %s missing (own+shared not applied)", tagID2)
	}
}

// TestGetTagList_ExcludesUnsharedOwners: without a grant, another user's tags
// are hidden.
func TestGetTagList_ExcludesUnsharedOwners(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)

	h.seedTag(t, tagID1, seedUserID, "#mine", 0, false)
	h.seedTag(t, tagID2, otherUserID, "#notshared", 0, false)

	status, env := h.do(t, http.MethodGet, "/api/v1/tag/get-tag-list", token, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	got := map[string]bool{}
	for _, it := range mustUnmarshal[itemsWrapper](t, env.Data).Items {
		got[it.ID] = true
	}
	if got[tagID2] {
		t.Errorf("tag %s of a non-sharing user must NOT appear", tagID2)
	}
}
