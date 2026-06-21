package budget

import "testing"

// The structure builder accumulates elements from Go maps (tags, categories),
// whose iteration order is randomized. Sorting by position alone leaves
// equal-position elements in that random order, so the get-budget response was
// non-deterministic run-to-run. The order must therefore break position ties by
// id (deterministic, and the frontend reorders when it needs a different order).
func TestSortByPositionThenID_BreaksTiesByID(t *testing.T) {
	type row struct {
		pos int
		id  string
	}
	// Same position for several rows, ids deliberately out of order.
	items := []row{
		{pos: 2, id: "ccc"},
		{pos: 0, id: "bbb"},
		{pos: 0, id: "aaa"},
		{pos: 1, id: "zzz"},
		{pos: 0, id: "ddd"},
		{pos: 1, id: "aaa"},
	}
	sortByPositionThenID(items, func(r row) int { return r.pos }, func(r row) string { return r.id })

	want := []row{
		{pos: 0, id: "aaa"},
		{pos: 0, id: "bbb"},
		{pos: 0, id: "ddd"},
		{pos: 1, id: "aaa"},
		{pos: 1, id: "zzz"},
		{pos: 2, id: "ccc"},
	}
	for i := range want {
		if items[i] != want[i] {
			t.Fatalf("index %d = %+v, want %+v (full: %+v)", i, items[i], want[i], items)
		}
	}
}
