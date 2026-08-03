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
