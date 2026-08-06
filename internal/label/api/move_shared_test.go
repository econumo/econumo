package api_test

// Regression coverage mirroring tag/category: move-label and sort-label-list
// are OWNER-ONLY. A sharee — whatever their role, guest included — must not be
// able to reposition the account owner's labels; ids owned by someone else are
// silently ignored.

import (
	"net/http"
	"testing"
)

const orderSharedAcctID = "aaaa5555-0000-0000-0000-0000000000a5"

// seedForeignLabelShared seeds a label owned by otherUserID plus an accepted
// grant of one of otherUserID's accounts to the caller, so the label is
// VISIBLE to the caller via the shared read view.
func (h *harness) seedForeignLabelShared(t *testing.T, id string, position int, role int) {
	t.Helper()
	h.seedLabel(t, id, otherUserID, "#foreign", position, false)
	h.seedAccount(t, orderSharedAcctID, otherUserID, "Other's account")
	h.seedGrant(t, orderSharedAcctID, seedUserID, role)
}

func (h *harness) labelRow(t *testing.T, id string) (sortKey, updatedAt string) {
	t.Helper()
	if err := h.db.QueryRow(`SELECT sort_key, updated_at FROM labels WHERE id = ?`, id).Scan(&sortKey, &updatedAt); err != nil {
		t.Fatalf("query label %s: %v", id, err)
	}
	return sortKey, updatedAt
}

func TestMoveLabel_SharedLabel_NotReordered(t *testing.T) {
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
			h.seedForeignLabelShared(t, labelID2, 5, tc.role)
			wantKey, wantUpdated := h.labelRow(t, labelID2)

			status, env := h.do(t, http.MethodPost, "/api/v1/label/move-label", token, map[string]any{
				"id": labelID2, "afterId": nil,
			})
			if status != http.StatusOK {
				t.Fatalf("status = %d, want 200 (shared ids are ignored, not an error); body: %s", status, env.raw)
			}

			key, updated := h.labelRow(t, labelID2)
			if key != wantKey {
				t.Errorf("shared label sort_key = %q, want %q (a sharee must not reorder the owner's labels)", key, wantKey)
			}
			if updated != wantUpdated {
				t.Errorf("shared label updated_at changed (%q -> %q), want untouched", wantUpdated, updated)
			}
		})
	}
}

func TestMoveLabel_ForeignRow_UpdatesNothing(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	h.seedLabel(t, labelID1, seedUserID, "#mine", 0, false)
	h.seedForeignLabelShared(t, labelID2, 5, roleGuest)
	ownKey, ownUpdated := h.labelRow(t, labelID1)
	foreignKey, foreignUpdated := h.labelRow(t, labelID2)

	status, env := h.do(t, http.MethodPost, "/api/v1/label/move-label", token, map[string]any{
		"id": labelID2, "afterId": nil,
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (a foreign id is ignored, not an error); body: %s", status, env.raw)
	}

	if key, updated := h.labelRow(t, labelID2); key != foreignKey || updated != foreignUpdated {
		t.Errorf("foreign label row changed (%q/%q -> %q/%q), want untouched", foreignKey, foreignUpdated, key, updated)
	}
	if key, updated := h.labelRow(t, labelID1); key != ownKey || updated != ownUpdated {
		t.Errorf("own label row changed (%q/%q -> %q/%q), want untouched", ownKey, ownUpdated, key, updated)
	}

	// The response is still the full available list, the shared row included.
	present := false
	for _, it := range mustUnmarshal[itemsWrapper](t, env.Data).Items {
		if it.ID == labelID2 {
			present = true
		}
	}
	if !present {
		t.Error("shared label missing from the response list")
	}
}

// TestSortLabelList_SkipsForeignIds: a sharee naming the owner's label in an
// explicit order must not rewrite it.
func TestSortLabelList_SkipsForeignIds(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	h.seedLabel(t, labelID1, seedUserID, "#mine", 0, false)
	h.seedForeignLabelShared(t, labelID2, 5, roleGuest)
	foreignKey, foreignUpdated := h.labelRow(t, labelID2)

	status, env := h.do(t, http.MethodPost, "/api/v1/label/sort-label-list", token, map[string]any{
		"ids": []string{labelID2, labelID1},
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}
	if key, updated := h.labelRow(t, labelID2); key != foreignKey || updated != foreignUpdated {
		t.Errorf("foreign label row changed (%q/%q -> %q/%q), want untouched", foreignKey, foreignUpdated, key, updated)
	}
}
