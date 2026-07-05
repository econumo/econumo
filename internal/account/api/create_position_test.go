package api_test

import (
	"net/http"
	"testing"
)

// A newly created account must land at the LAST position (one past the current
// max), not collide with the existing last account. Regression: the 3rd+ account
// was assigned maxPos (== the current last account's position) instead of
// maxPos+1, so it stacked on top of the last one instead of after it.
func TestCreateAccount_AppendsAtLastPosition(t *testing.T) {
	h := newHarness(t)

	opIDs := []string{
		"aaaa1111-0000-7000-8000-0000000000a1",
		"aaaa1111-0000-7000-8000-0000000000a2",
		"aaaa1111-0000-7000-8000-0000000000a3",
	}
	var positions []int
	for i, op := range opIDs {
		_, it := h.createAccount(t, op, "Acct"+string(rune('A'+i)), "0")
		positions = append(positions, it.Position)
	}

	// Each account must occupy a strictly greater position than the previous one,
	// and they must all be distinct — the newest is always last.
	for i := 1; i < len(positions); i++ {
		if positions[i] <= positions[i-1] {
			t.Fatalf("account #%d position = %d, want > previous (%d); positions=%v", i+1, positions[i], positions[i-1], positions)
		}
	}

	// The list confirms the newest account carries the maximum position.
	_, env := h.do(t, http.MethodGet, "/api/v1/account/get-account-list", h.token(t), nil)
	items := mustUnmarshal[accountItemsWrapper](t, env.Data).Items
	maxPos, maxName := -1, ""
	for _, it := range items {
		if it.Position > maxPos {
			maxPos, maxName = it.Position, it.Name
		}
	}
	if maxName != "AcctC" {
		t.Fatalf("account at the max position = %q (pos %d), want the last-created AcctC; items=%+v", maxName, maxPos, items)
	}
}
