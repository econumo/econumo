package api_test

import (
	"net/http"
	"testing"
)

func TestSortCategoryList_AppliesTheGivenOrder(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	id1, id2, id3 := seedThree(t, h, token)

	status, env := h.do(t, http.MethodPost, "/api/v1/category/sort-category-list", token,
		map[string]any{"ids": []string{id3, id1, id2}})
	if status != http.StatusOK {
		t.Fatalf("status = %d, body: %s", status, env.raw)
	}
	assertOrder(t, orderedIDs(t, env), []string{id3, id1, id2})
	assertDensePositions(t, env)
}

// TestSortCategoryList_AlreadySortedWritesNothing is what makes the bulk sort
// cheap: re-applying the current order touches no rows.
func TestSortCategoryList_AlreadySortedWritesNothing(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	id1, id2, id3 := seedThree(t, h, token)

	before := h.sortKeys(t)
	h.do(t, http.MethodPost, "/api/v1/category/sort-category-list", token,
		map[string]any{"ids": []string{id1, id2, id3}})
	after := h.sortKeys(t)

	for id, k := range after {
		if before[id] != k {
			t.Fatalf("row %s was rewritten (%q -> %q) for a no-op sort", id, before[id], k)
		}
	}
}

// TestSortCategoryList_ReverseWritesFewerRowsThanItReorders pins the minimal-write
// pass: reversing three rows needs two writes, not three.
func TestSortCategoryList_ReverseWritesFewerRowsThanItReorders(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	id1, id2, id3 := seedThree(t, h, token)

	before := h.sortKeys(t)
	_, env := h.do(t, http.MethodPost, "/api/v1/category/sort-category-list", token,
		map[string]any{"ids": []string{id3, id2, id1}})
	after := h.sortKeys(t)

	changed := 0
	for id, k := range after {
		if before[id] != k {
			changed++
		}
	}
	if changed != 2 {
		t.Fatalf("%d rows rewritten, want 2 (the leading row already sorts first)", changed)
	}
	assertOrder(t, orderedIDs(t, env), []string{id3, id2, id1})
}

func TestSortCategoryList_UnknownIdIsSkipped(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	id1, id2, id3 := seedThree(t, h, token)

	status, env := h.do(t, http.MethodPost, "/api/v1/category/sort-category-list", token,
		map[string]any{"ids": []string{"00000000-0000-0000-0000-0000000000ff", id3, id2, id1}})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (an id the caller does not own is skipped); body: %s", status, env.raw)
	}
	assertOrder(t, orderedIDs(t, env), []string{id3, id2, id1})
}

func TestSortCategoryList_EmptyIds_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	seedThree(t, h, token)

	status, _ := h.do(t, http.MethodPost, "/api/v1/category/sort-category-list", token,
		map[string]any{"ids": []string{}})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

func TestSortCategoryList_MalformedId_400(t *testing.T) {
	h := newHarness(t)
	token := h.issueToken(t)
	seedThree(t, h, token)

	status, _ := h.do(t, http.MethodPost, "/api/v1/category/sort-category-list", token,
		map[string]any{"ids": []string{"not-a-uuid"}})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}
