package api_test

// Regression coverage mirroring tag/category: order-label-list is OWNER-ONLY.
// A sharee — whatever their role, guest included — must not be able to
// rewrite the position of the account owner's labels; shared ids in the
// changes list are silently ignored, exactly like tag's order semantics.

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

func (h *harness) labelRow(t *testing.T, id string) (position int, updatedAt string) {
	t.Helper()
	if err := h.db.QueryRow(`SELECT position, updated_at FROM labels WHERE id = ?`, id).Scan(&position, &updatedAt); err != nil {
		t.Fatalf("query label %s: %v", id, err)
	}
	return position, updatedAt
}

func TestOrderLabelList_SharedLabel_NotReordered(t *testing.T) {
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
			_, wantUpdated := h.labelRow(t, labelID2)

			status, env := h.do(t, http.MethodPost, "/api/v1/label/order-label-list", token, map[string]any{
				"changes": []map[string]any{{"id": labelID2, "position": 0}},
			})
			if status != http.StatusOK {
				t.Fatalf("status = %d, want 200 (shared ids are ignored, not an error); body: %s", status, env.raw)
			}

			pos, updated := h.labelRow(t, labelID2)
			if pos != 5 {
				t.Errorf("shared label position = %d, want 5 (sharee must not reorder the owner's labels)", pos)
			}
			if updated != wantUpdated {
				t.Errorf("shared label updated_at changed (%q -> %q), want untouched", wantUpdated, updated)
			}
		})
	}
}

func TestOrderLabelList_MixedChanges_UpdatesOwnOnly(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	h.seedLabel(t, labelID1, seedUserID, "#mine", 0, false)
	h.seedForeignLabelShared(t, labelID2, 5, roleGuest)

	status, env := h.do(t, http.MethodPost, "/api/v1/label/order-label-list", token, map[string]any{
		"changes": []map[string]any{
			{"id": labelID1, "position": 3},
			{"id": labelID2, "position": 0},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}

	if pos, _ := h.labelRow(t, labelID1); pos != 3 {
		t.Errorf("own label position = %d, want 3", pos)
	}
	if pos, _ := h.labelRow(t, labelID2); pos != 5 {
		t.Errorf("shared label position = %d, want 5 (untouched)", pos)
	}

	// The response is still the full available list, shared label included.
	got := map[string]int{}
	for _, it := range mustUnmarshal[itemsWrapper](t, env.Data).Items {
		got[it.ID] = it.Position
	}
	if p, ok := got[labelID2]; !ok || p != 5 {
		t.Errorf("shared label in response = (%d, present=%v), want position 5 present", p, ok)
	}
}
