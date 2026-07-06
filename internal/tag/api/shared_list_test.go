package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

// usdID is the baseline USD currency (seeded by migration 20210812210548).
const usdID = fixture.USD

func (h *harness) seedAccount(t *testing.T, id, ownerID, name string) {
	t.Helper()
	fixture.New(t, h.tdb).Account(fixture.Account{
		ID:         id,
		UserID:     ownerID,
		CurrencyID: usdID,
		Name:       name,
	})
}

func (h *harness) seedGrant(t *testing.T, accountID, userID string, role int) {
	t.Helper()
	fixture.New(t, h.tdb).AccountAccess(accountID, userID, role)
}

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
