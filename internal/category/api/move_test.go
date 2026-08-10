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

// assertOrder compares a response's item order against the expected ids.
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

func seedThree(t *testing.T, h *harness, token string) (string, string, string) {
	t.Helper()
	return createCategory(t, h, token, catID1, "First", "expense"),
		createCategory(t, h, token, catID2, "Second", "expense"),
		createCategory(t, h, token, catID3, "Third", "expense")
}

func TestMoveCategory_ToFront(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	id1, id2, id3 := seedThree(t, h, token)

	status, env := h.do(t, http.MethodPost, "/api/v1/category/move-category", token,
		map[string]any{"id": id3, "afterId": nil})
	if status != http.StatusOK {
		t.Fatalf("status = %d, body: %s", status, env.raw)
	}
	assertOrder(t, orderedIDs(t, env), []string{id3, id1, id2})
	assertDensePositions(t, env)
}

func TestMoveCategory_AfterAnchor(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	id1, id2, id3 := seedThree(t, h, token)

	status, env := h.do(t, http.MethodPost, "/api/v1/category/move-category", token,
		map[string]any{"id": id1, "afterId": id2})
	if status != http.StatusOK {
		t.Fatalf("status = %d, body: %s", status, env.raw)
	}
	assertOrder(t, orderedIDs(t, env), []string{id2, id1, id3})
}

func TestMoveCategory_ToEnd(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	id1, id2, id3 := seedThree(t, h, token)

	_, env := h.do(t, http.MethodPost, "/api/v1/category/move-category", token,
		map[string]any{"id": id1, "afterId": id3})
	assertOrder(t, orderedIDs(t, env), []string{id2, id3, id1})
}

// TestMoveCategory_WritesExactlyOneRow is the whole point of the change: the
// absolute-position endpoint this replaces rewrote every row above the target.
func TestMoveCategory_WritesExactlyOneRow(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	_, _, id3 := seedThree(t, h, token)

	before := h.sortKeys(t)
	h.do(t, http.MethodPost, "/api/v1/category/move-category", token,
		map[string]any{"id": id3, "afterId": nil})
	after := h.sortKeys(t)

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

// TestMoveCategory_UnknownAnchorAppends pins the documented degradation: an
// anchor outside the caller's scope appends instead of erroring.
func TestMoveCategory_UnknownAnchorAppends(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	id1, id2, id3 := seedThree(t, h, token)

	status, env := h.do(t, http.MethodPost, "/api/v1/category/move-category", token,
		map[string]any{"id": id1, "afterId": "00000000-0000-0000-0000-0000000000ff"})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (unknown anchor appends, never errors); body: %s", status, env.raw)
	}
	assertOrder(t, orderedIDs(t, env), []string{id2, id3, id1})
}

func TestMoveCategory_MalformedID_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	seedThree(t, h, token)

	status, _ := h.do(t, http.MethodPost, "/api/v1/category/move-category", token,
		map[string]any{"id": "not-a-uuid", "afterId": nil})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

// TestMoveCategory_SurvivesRepeatedFrontMoves exercises the midpoint path many
// times over: every move targets the same insertion point, which is where a
// numeric scheme would run out of room.
func TestMoveCategory_SurvivesRepeatedFrontMoves(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	id1, id2, id3 := seedThree(t, h, token)

	var env envelope
	for i := 0; i < 40; i++ {
		target := []string{id1, id2, id3}[i%3]
		var st int
		st, env = h.do(t, http.MethodPost, "/api/v1/category/move-category", token,
			map[string]any{"id": target, "afterId": nil})
		if st != http.StatusOK {
			t.Fatalf("iteration %d: status = %d, body: %s", i, st, env.raw)
		}
	}
	assertDensePositions(t, env)
	if got := orderedIDs(t, env); len(got) != 3 {
		t.Fatalf("list = %v, want 3 items", got)
	}
}
