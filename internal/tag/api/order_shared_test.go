package api_test

// Regression coverage for issue #108: order-tag-list is OWNER-ONLY (mirrors
// category). A sharee — whatever their role, guest included — must not be able
// to rewrite the position of the account owner's tags; shared ids in the
// changes list are silently ignored, exactly like category's order semantics.

import (
	"net/http"
	"testing"
)

const orderSharedAcctID = "aaaa5555-0000-0000-0000-0000000000a5"

// seedForeignTagShared seeds a tag owned by otherUserID plus an accepted grant
// of one of otherUserID's accounts to the caller, so the tag is VISIBLE to the
// caller via the shared read view.
func (h *harness) seedForeignTagShared(t *testing.T, tagID string, position int, role int) {
	t.Helper()
	h.seedTag(t, tagID, otherUserID, "#foreign", position, false)
	h.seedAccount(t, orderSharedAcctID, otherUserID, "Other's account")
	h.seedGrant(t, orderSharedAcctID, seedUserID, role)
}

func (h *harness) tagRow(t *testing.T, id string) (position int, updatedAt string) {
	t.Helper()
	if err := h.db.QueryRow(`SELECT position, updated_at FROM tags WHERE id = ?`, id).Scan(&position, &updatedAt); err != nil {
		t.Fatalf("query tag %s: %v", id, err)
	}
	return position, updatedAt
}

func TestOrderTagList_SharedTag_NotReordered(t *testing.T) {
	for _, tc := range []struct {
		name string
		role int
	}{
		{"guest", roleGuest},
		{"admin", roleAdmin},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			token := h.issueToken(t)
			h.seedForeignTagShared(t, tagID2, 5, tc.role)
			_, wantUpdated := h.tagRow(t, tagID2)

			status, env := h.do(t, http.MethodPost, "/api/v1/tag/order-tag-list", token, map[string]any{
				"changes": []map[string]any{{"id": tagID2, "position": 0}},
			})
			if status != http.StatusOK {
				t.Fatalf("status = %d, want 200 (shared ids are ignored, not an error); body: %s", status, env.raw)
			}

			pos, updated := h.tagRow(t, tagID2)
			if pos != 5 {
				t.Errorf("shared tag position = %d, want 5 (sharee must not reorder the owner's tags)", pos)
			}
			if updated != wantUpdated {
				t.Errorf("shared tag updated_at changed (%q -> %q), want untouched", wantUpdated, updated)
			}
		})
	}
}

func TestOrderTagList_MixedChanges_UpdatesOwnOnly(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	h.seedTag(t, tagID1, seedUserID, "#mine", 0, false)
	h.seedForeignTagShared(t, tagID2, 5, roleGuest)

	status, env := h.do(t, http.MethodPost, "/api/v1/tag/order-tag-list", token, map[string]any{
		"changes": []map[string]any{
			{"id": tagID1, "position": 3},
			{"id": tagID2, "position": 0},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}

	if pos, _ := h.tagRow(t, tagID1); pos != 3 {
		t.Errorf("own tag position = %d, want 3", pos)
	}
	if pos, _ := h.tagRow(t, tagID2); pos != 5 {
		t.Errorf("shared tag position = %d, want 5 (untouched)", pos)
	}

	// The response is still the full available list, shared tag included.
	got := map[string]int{}
	for _, it := range mustUnmarshal[itemsWrapper](t, env.Data).Items {
		got[it.ID] = it.Position
	}
	if p, ok := got[tagID2]; !ok || p != 5 {
		t.Errorf("shared tag in response = (%d, present=%v), want position 5 present", p, ok)
	}
}
