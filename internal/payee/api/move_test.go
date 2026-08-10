package api_test

import (
	"net/http"
	"testing"
)

// orderedIDs returns the ids from an items response, in response order.
func orderedIDs(t *testing.T, env envelope) []string {
	t.Helper()
	res := mustUnmarshal[itemsWrapper](t, env.Data)
	out := make([]string, 0, len(res.Items))
	for _, it := range res.Items {
		out = append(out, it.ID)
	}
	return out
}

func assertOrder(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

// assertDensePositions pins the wire contract: position is the item's 0-based
// index in the returned list, never a stored value.
func assertDensePositions(t *testing.T, env envelope) {
	t.Helper()
	res := mustUnmarshal[itemsWrapper](t, env.Data)
	for i, it := range res.Items {
		if it.Position != i {
			t.Fatalf("item %d (%s) position = %d, want %d", i, it.ID, it.Position, i)
		}
	}
}

// seedPayeeTrio seeds three owned payees in a known order.
func seedPayeeTrio(t *testing.T, h *harness) (string, string, string) {
	t.Helper()
	h.seedPayee(t, payeeID1, seedUserID, "one", 0, false)
	h.seedPayee(t, payeeID2, seedUserID, "two", 1, false)
	h.seedPayee(t, payeeID3, seedUserID, "three", 2, false)
	return payeeID1, payeeID2, payeeID3
}

func TestMovePayee_ToFront(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	id1, id2, id3 := seedPayeeTrio(t, h)

	status, env := h.do(t, http.MethodPost, "/api/v1/payee/move-payee", token,
		map[string]any{"id": id3, "afterId": nil})
	if status != http.StatusOK {
		t.Fatalf("status = %d, body: %s", status, env.raw)
	}
	assertOrder(t, orderedIDs(t, env), []string{id3, id1, id2})
	assertDensePositions(t, env)
}

func TestMovePayee_AfterAnchor(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	id1, id2, id3 := seedPayeeTrio(t, h)

	_, env := h.do(t, http.MethodPost, "/api/v1/payee/move-payee", token,
		map[string]any{"id": id1, "afterId": id2})
	assertOrder(t, orderedIDs(t, env), []string{id2, id1, id3})
}

func TestMovePayee_ToEnd(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	id1, id2, id3 := seedPayeeTrio(t, h)

	_, env := h.do(t, http.MethodPost, "/api/v1/payee/move-payee", token,
		map[string]any{"id": id1, "afterId": id3})
	assertOrder(t, orderedIDs(t, env), []string{id2, id3, id1})
}

// TestMovePayee_WritesExactlyOneRow is the whole point of the change: the
// absolute-position endpoint this replaces rewrote every row above the target.
func TestMovePayee_WritesExactlyOneRow(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	_, _, id3 := seedPayeeTrio(t, h)

	before := h.allSortKeys(t)
	h.do(t, http.MethodPost, "/api/v1/payee/move-payee", token,
		map[string]any{"id": id3, "afterId": nil})
	after := h.allSortKeys(t)

	changed := 0
	for id, k := range after {
		if before[id] != k {
			changed++
		}
	}
	if changed != 1 {
		t.Fatalf("%d rows changed sort_key, want exactly 1 (before=%v after=%v)", changed, before, after)
	}
}

// TestMovePayee_UnknownAnchorAppends pins the documented degradation: an anchor
// outside the caller's scope appends instead of erroring.
func TestMovePayee_UnknownAnchorAppends(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	id1, id2, id3 := seedPayeeTrio(t, h)

	status, env := h.do(t, http.MethodPost, "/api/v1/payee/move-payee", token,
		map[string]any{"id": id1, "afterId": "00000000-0000-0000-0000-0000000000ff"})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (unknown anchor appends, never errors); body: %s", status, env.raw)
	}
	assertOrder(t, orderedIDs(t, env), []string{id2, id3, id1})
}

func TestMovePayee_MalformedID_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	seedPayeeTrio(t, h)

	status, _ := h.do(t, http.MethodPost, "/api/v1/payee/move-payee", token,
		map[string]any{"id": "not-a-uuid", "afterId": nil})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

// TestMovePayee_SurvivesRepeatedFrontMoves exercises the midpoint path many
// times over: every move targets the same insertion point, which is where a
// numeric scheme would run out of room.
func TestMovePayee_SurvivesRepeatedFrontMoves(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	id1, id2, id3 := seedPayeeTrio(t, h)

	var env envelope
	for i := 0; i < 40; i++ {
		target := []string{id1, id2, id3}[i%3]
		var st int
		st, env = h.do(t, http.MethodPost, "/api/v1/payee/move-payee", token,
			map[string]any{"id": target, "afterId": nil})
		if st != http.StatusOK {
			t.Fatalf("iteration %d: status = %d, body: %s", i, st, env.raw)
		}
	}
	assertDensePositions(t, env)
}
