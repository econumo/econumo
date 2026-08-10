package api_test

// Regression coverage for issue #108: move-tag is OWNER-ONLY (mirrors
// category). A sharee — whatever their role, guest included — must not be able
// to rewrite the position of the account owner's tags; shared ids in the
// tags owned by someone else are silently ignored, exactly like category's move semantics.

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

func (h *harness) tagRow(t *testing.T, id string) (sortKey, updatedAt string) {
	t.Helper()
	if err := h.db.QueryRow(`SELECT sort_key, updated_at FROM tags WHERE id = ?`, id).Scan(&sortKey, &updatedAt); err != nil {
		t.Fatalf("query tag %s: %v", id, err)
	}
	return sortKey, updatedAt
}

func TestMoveTag_SharedTag_NotReordered(t *testing.T) {
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
			wantKey, wantUpdated := h.tagRow(t, tagID2)

			status, env := h.do(t, http.MethodPost, "/api/v1/tag/move-tag", token, map[string]any{
				"id": tagID2, "afterId": nil,
			})
			if status != http.StatusOK {
				t.Fatalf("status = %d, want 200 (shared ids are ignored, not an error); body: %s", status, env.raw)
			}

			key, updated := h.tagRow(t, tagID2)
			if key != wantKey {
				t.Errorf("shared tag sort_key = %q, want %q (a sharee must not reorder the owner's tags)", key, wantKey)
			}
			if updated != wantUpdated {
				t.Errorf("shared tag updated_at changed (%q -> %q), want untouched", wantUpdated, updated)
			}
		})
	}
}

func TestMoveTag_ForeignRow_UpdatesNothing(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	h.seedTag(t, tagID1, seedUserID, "#mine", 0, false)
	h.seedForeignTagShared(t, tagID2, 5, roleGuest)
	ownKey, ownUpdated := h.tagRow(t, tagID1)
	foreignKey, foreignUpdated := h.tagRow(t, tagID2)

	// Ask to move the FOREIGN row. It is not in the caller's owned set, so the
	// move resolves to nothing -- and it must not drag the caller's own row
	// along either.
	status, env := h.do(t, http.MethodPost, "/api/v1/tag/move-tag", token, map[string]any{
		"id": tagID2, "afterId": nil,
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (a foreign id is ignored, not an error); body: %s", status, env.raw)
	}

	if key, updated := h.tagRow(t, tagID2); key != foreignKey || updated != foreignUpdated {
		t.Errorf("foreign tag row changed (%q/%q -> %q/%q), want untouched", foreignKey, foreignUpdated, key, updated)
	}
	if key, updated := h.tagRow(t, tagID1); key != ownKey || updated != ownUpdated {
		t.Errorf("own tag row changed (%q/%q -> %q/%q), want untouched", ownKey, ownUpdated, key, updated)
	}

	// The response is still the full available list, the shared row included.
	present := false
	for _, it := range mustUnmarshal[itemsWrapper](t, env.Data).Items {
		if it.ID == tagID2 {
			present = true
		}
	}
	if !present {
		t.Error("shared tag missing from the response list")
	}
}
