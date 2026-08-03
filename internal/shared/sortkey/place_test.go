package sortkey

import "testing"

// sibs builds a sibling list with single-letter ids "a", "b", "c", ... in the
// order given, which is also the order the callers keep them sorted in.
func sibs(keys ...Key) []Item {
	out := make([]Item, 0, len(keys))
	for i, k := range keys {
		out = append(out, Item{ID: string(rune('a' + i)), Key: k})
	}
	return out
}

// TestPlace_EmptyListUsesSeed covers the first row in a list, where there is no
// neighbour to anchor against.
func TestPlace_EmptyListUsesSeed(t *testing.T) {
	got, err := Place(nil, "", GrowsUp)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if got != "a0" {
		t.Fatalf("Place(nil, \"\", GrowsUp) = %q, want \"a0\"", got)
	}
	got, err = Place(nil, "", GrowsDown)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if got != "c000" {
		t.Fatalf("Place(nil, \"\", GrowsDown) = %q, want \"c000\"", got)
	}
}

// TestPlace_NilAnchorMovesToFront: an empty afterID means "put it first".
func TestPlace_NilAnchorMovesToFront(t *testing.T) {
	got, err := Place(sibs("a0", "a1", "a2"), "", GrowsUp)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if got >= "a0" {
		t.Fatalf("Place = %q, want a key sorting before \"a0\"", got)
	}
}

// TestPlace_BetweenNeighbours anchors on a middle sibling.
func TestPlace_BetweenNeighbours(t *testing.T) {
	got, err := Place(sibs("a0", "a1", "a2"), "a", GrowsUp)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if !(got > "a0" && got < "a1") {
		t.Fatalf("Place = %q, want strictly between a0 and a1", got)
	}
}

// TestPlace_AfterLastAppends anchors on the tail.
func TestPlace_AfterLastAppends(t *testing.T) {
	got, err := Place(sibs("a0", "a1", "a2"), "c", GrowsUp)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if got <= "a2" {
		t.Fatalf("Place = %q, want a key sorting after \"a2\"", got)
	}
}

// TestPlace_UnknownAnchorAppends is the documented degradation: an anchor that
// is not in the caller's scope (deleted concurrently, another user's row, a
// different folder) appends rather than erroring. This matches the pre-existing
// silent-skip behaviour of the order-*-list use cases and avoids a new errs code,
// which would need a translation in all 11 locale catalogues.
func TestPlace_UnknownAnchorAppends(t *testing.T) {
	got, err := Place(sibs("a0", "a1"), "no-such-id", GrowsUp)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if got <= "a1" {
		t.Fatalf("Place = %q, want an append past \"a1\"", got)
	}
}

// TestPlace_ToleratesDuplicateKeys: rows backfilled from the legacy integer
// positions can share a key, so the search for the following neighbour must skip
// any sibling that does not sort strictly after the anchor.
func TestPlace_ToleratesDuplicateKeys(t *testing.T) {
	got, err := Place([]Item{{ID: "a", Key: "a0"}, {ID: "b", Key: "a0"}}, "a", GrowsUp)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if got <= "a0" {
		t.Fatalf("Place = %q, want a key after the duplicated \"a0\"", got)
	}
}

// TestPlace_ProducesAConsistentOrderAcrossManyMoves is the end-to-end property
// the seven use cases depend on: repeatedly relocating items must always leave a
// list whose keys sort in the intended order.
func TestPlace_ProducesAConsistentOrderAcrossManyMoves(t *testing.T) {
	// Build a 10-item list by appending.
	items := []Item{}
	for i := 0; i < 10; i++ {
		anchor := ""
		if len(items) > 0 {
			anchor = items[len(items)-1].ID
		}
		k, err := Place(items, anchor, GrowsUp)
		if err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
		items = append(items, Item{ID: string(rune('A' + i)), Key: k})
	}

	// Move each item, in turn, to the front; then verify keys stay ordered.
	for round := 0; round < 30; round++ {
		idx := round % len(items)
		moved := items[idx]
		rest := append(append([]Item{}, items[:idx]...), items[idx+1:]...)

		k, err := Place(rest, "", GrowsUp)
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		moved.Key = k
		items = append([]Item{moved}, rest...)

		for i := 1; i < len(items); i++ {
			if items[i-1].Key >= items[i].Key {
				t.Fatalf("round %d: keys out of order at %d: %q >= %q", round, i, items[i-1].Key, items[i].Key)
			}
		}
	}
}

type row struct {
	id  string
	key Key
}

func rowItem(r row) Item { return Item{ID: r.id, Key: r.key} }

// TestRelocate_MovesWithinItsOwnList confirms the moved row is excluded from its
// own sibling set, so it never anchors against its current key.
func TestRelocate_MovesWithinItsOwnList(t *testing.T) {
	items := []row{{"x", "a0"}, {"y", "a1"}, {"z", "a2"}}
	got, found, err := Relocate(items, "z", "x", rowItem, GrowsUp)
	if err != nil || !found {
		t.Fatalf("Relocate: key=%q found=%v err=%v", got, found, err)
	}
	if !(got > "a0" && got < "a1") {
		t.Fatalf("key = %q, want strictly between a0 and a1", got)
	}
}

// TestRelocate_ReportsMissingRow is the silent no-op every caller relies on.
func TestRelocate_ReportsMissingRow(t *testing.T) {
	items := []row{{"x", "a0"}}
	_, found, err := Relocate(items, "nope", "", rowItem, GrowsUp)
	if err != nil {
		t.Fatalf("Relocate: %v", err)
	}
	if found {
		t.Fatal("found = true for a row that is not in the list")
	}
}

// TestRelocate_ToFrontOfARemainingList pins the null-anchor case after exclusion.
func TestRelocate_ToFrontOfARemainingList(t *testing.T) {
	items := []row{{"x", "a0"}, {"y", "a1"}}
	got, found, err := Relocate(items, "y", "", rowItem, GrowsUp)
	if err != nil || !found {
		t.Fatalf("Relocate: found=%v err=%v", found, err)
	}
	if got >= "a0" {
		t.Fatalf("key = %q, want before a0", got)
	}
}

// TestRelocate_OnlyRowInTheListFallsBackToTheSeed: excluding the moved row can
// leave no siblings at all.
func TestRelocate_OnlyRowInTheListFallsBackToTheSeed(t *testing.T) {
	got, found, err := Relocate([]row{{"x", "a5"}}, "x", "", rowItem, GrowsUp)
	if err != nil || !found {
		t.Fatalf("Relocate: found=%v err=%v", found, err)
	}
	if got != "a0" {
		t.Fatalf("key = %q, want the seed \"a0\"", got)
	}
}

func TestAppendAndPrepend(t *testing.T) {
	empty := []row(nil)
	if got, err := Append(empty, rowItem, GrowsUp); err != nil || got != "a0" {
		t.Errorf("Append(empty) = %q, %v; want \"a0\"", got, err)
	}
	if got, err := Prepend(empty, rowItem, GrowsDown); err != nil || got != "c000" {
		t.Errorf("Prepend(empty) = %q, %v; want \"c000\"", got, err)
	}
	items := []row{{"x", "c000"}, {"y", "c001"}}
	got, err := Append(items, rowItem, GrowsUp)
	if err != nil || got <= "c001" {
		t.Errorf("Append = %q, %v; want a key after c001", got, err)
	}
	got, err = Prepend(items, rowItem, GrowsDown)
	if err != nil || got >= "c000" {
		t.Errorf("Prepend = %q, %v; want a key before c000", got, err)
	}
}
