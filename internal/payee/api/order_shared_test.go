package api_test

// Regression coverage for issue #108: order-payee-list is OWNER-ONLY (mirrors
// category). A sharee — whatever their role, guest included — must not be able
// to rewrite the position of the account owner's payees; shared ids in the
// changes list are silently ignored, exactly like category's order semantics.

import (
	"net/http"
	"testing"
)

const orderSharedAcctID = "aaaa5555-0000-0000-0000-0000000000a5"

// seedForeignPayeeShared seeds a payee owned by otherUserID plus an accepted
// grant of one of otherUserID's accounts to the caller, so the payee is
// VISIBLE to the caller via the shared read view.
func (h *harness) seedForeignPayeeShared(t *testing.T, payeeID string, position int, role int) {
	t.Helper()
	h.seedPayee(t, payeeID, otherUserID, "Foreign", position, false)
	h.seedAccount(t, orderSharedAcctID, otherUserID, "Other's account")
	h.seedGrant(t, orderSharedAcctID, seedUserID, role)
}

func (h *harness) payeeRow(t *testing.T, id string) (position int, updatedAt string) {
	t.Helper()
	if err := h.db.QueryRow(`SELECT position, updated_at FROM payees WHERE id = ?`, id).Scan(&position, &updatedAt); err != nil {
		t.Fatalf("query payee %s: %v", id, err)
	}
	return position, updatedAt
}

func TestOrderPayeeList_SharedPayee_NotReordered(t *testing.T) {
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
			h.seedForeignPayeeShared(t, payeeID2, 5, tc.role)
			_, wantUpdated := h.payeeRow(t, payeeID2)

			status, env := h.do(t, http.MethodPost, "/api/v1/payee/order-payee-list", token, map[string]any{
				"changes": []map[string]any{{"id": payeeID2, "position": 0}},
			})
			if status != http.StatusOK {
				t.Fatalf("status = %d, want 200 (shared ids are ignored, not an error); body: %s", status, env.raw)
			}

			pos, updated := h.payeeRow(t, payeeID2)
			if pos != 5 {
				t.Errorf("shared payee position = %d, want 5 (sharee must not reorder the owner's payees)", pos)
			}
			if updated != wantUpdated {
				t.Errorf("shared payee updated_at changed (%q -> %q), want untouched", wantUpdated, updated)
			}
		})
	}
}

func TestOrderPayeeList_MixedChanges_UpdatesOwnOnly(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	h.seedPayee(t, payeeID1, seedUserID, "Mine", 0, false)
	h.seedForeignPayeeShared(t, payeeID2, 5, roleGuest)

	status, env := h.do(t, http.MethodPost, "/api/v1/payee/order-payee-list", token, map[string]any{
		"changes": []map[string]any{
			{"id": payeeID1, "position": 3},
			{"id": payeeID2, "position": 0},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, env.raw)
	}

	if pos, _ := h.payeeRow(t, payeeID1); pos != 3 {
		t.Errorf("own payee position = %d, want 3", pos)
	}
	if pos, _ := h.payeeRow(t, payeeID2); pos != 5 {
		t.Errorf("shared payee position = %d, want 5 (untouched)", pos)
	}

	// The response is still the full available list, shared payee included.
	got := map[string]int{}
	for _, it := range mustUnmarshal[itemsWrapper](t, env.Data).Items {
		got[it.ID] = it.Position
	}
	if p, ok := got[payeeID2]; !ok || p != 5 {
		t.Errorf("shared payee in response = (%d, present=%v), want position 5 present", p, ok)
	}
}
