package api_test

// Regression coverage for issue #108: move-payee is OWNER-ONLY (mirrors
// category). A sharee — whatever their role, guest included — must not be able
// to rewrite the position of the account owner's payees; shared ids in the
// payees owned by someone else are silently ignored, exactly like category's move semantics.

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

func (h *harness) payeeRow(t *testing.T, id string) (sortKey, updatedAt string) {
	t.Helper()
	if err := h.db.QueryRow(`SELECT sort_key, updated_at FROM payees WHERE id = ?`, id).Scan(&sortKey, &updatedAt); err != nil {
		t.Fatalf("query payee %s: %v", id, err)
	}
	return sortKey, updatedAt
}

func TestMovePayee_SharedPayee_NotReordered(t *testing.T) {
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
			wantKey, wantUpdated := h.payeeRow(t, payeeID2)

			status, env := h.do(t, http.MethodPost, "/api/v1/payee/move-payee", token, map[string]any{
				"id": payeeID2, "afterId": nil,
			})
			if status != http.StatusOK {
				t.Fatalf("status = %d, want 200 (shared ids are ignored, not an error); body: %s", status, env.raw)
			}

			key, updated := h.payeeRow(t, payeeID2)
			if key != wantKey {
				t.Errorf("shared payee sort_key = %q, want %q (a sharee must not reorder the owner's payees)", key, wantKey)
			}
			if updated != wantUpdated {
				t.Errorf("shared payee updated_at changed (%q -> %q), want untouched", wantUpdated, updated)
			}
		})
	}
}

func TestMovePayee_ForeignRow_UpdatesNothing(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	h.seedPayee(t, payeeID1, seedUserID, "#mine", 0, false)
	h.seedForeignPayeeShared(t, payeeID2, 5, roleGuest)
	ownKey, ownUpdated := h.payeeRow(t, payeeID1)
	foreignKey, foreignUpdated := h.payeeRow(t, payeeID2)

	// Ask to move the FOREIGN row. It is not in the caller's owned set, so the
	// move resolves to nothing -- and it must not drag the caller's own row
	// along either.
	status, env := h.do(t, http.MethodPost, "/api/v1/payee/move-payee", token, map[string]any{
		"id": payeeID2, "afterId": nil,
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (a foreign id is ignored, not an error); body: %s", status, env.raw)
	}

	if key, updated := h.payeeRow(t, payeeID2); key != foreignKey || updated != foreignUpdated {
		t.Errorf("foreign payee row changed (%q/%q -> %q/%q), want untouched", foreignKey, foreignUpdated, key, updated)
	}
	if key, updated := h.payeeRow(t, payeeID1); key != ownKey || updated != ownUpdated {
		t.Errorf("own payee row changed (%q/%q -> %q/%q), want untouched", ownKey, ownUpdated, key, updated)
	}

	// The response is still the full available list, the shared row included.
	present := false
	for _, it := range mustUnmarshal[itemsWrapper](t, env.Data).Items {
		if it.ID == payeeID2 {
			present = true
		}
	}
	if !present {
		t.Error("shared payee missing from the response list")
	}
}
