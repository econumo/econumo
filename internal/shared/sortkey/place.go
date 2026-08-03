package sortkey

// Item is one sibling in an ordered list: the id a caller anchors against and
// the key that decides its slot.
type Item struct {
	ID  string
	Key Key
}

// Place returns the key for an item dropped immediately after afterID within
// siblings, which must already be sorted by Key. The moved item itself must not
// appear in siblings, or it would anchor against its own current key.
//
// An empty afterID means "move to the front". An afterID that is not present in
// siblings appends to the end rather than erroring: the anchor may have been
// deleted concurrently, or belong to another user or folder, and the use cases
// this replaces silently skipped such ids too. Degrading here preserves that
// behaviour and avoids a coded error, which would need a translation in all 11
// locale catalogues plus an entry in the errs.AllCodes parity guard.
//
// Siblings holding duplicate keys are tolerated, because rows backfilled from
// the legacy integer positions can share one: the search for the following
// neighbour skips any sibling that does not sort strictly after the anchor.
func Place(siblings []Item, afterID string, g Growth) (Key, error) {
	if len(siblings) == 0 {
		return Seed(g), nil
	}

	if afterID == "" {
		return Between("", siblings[0].Key)
	}

	idx := -1
	for i, s := range siblings {
		if s.ID == afterID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return Between(siblings[len(siblings)-1].Key, "")
	}

	prev := siblings[idx].Key
	for _, s := range siblings[idx+1:] {
		if s.Key > prev {
			return Between(prev, s.Key)
		}
	}
	return Between(prev, "")
}

// MoveWithin gives movedID a key placing it immediately after afterID within
// items, and returns the item itself so the caller does not have to search the
// slice a second time. ok is false when movedID is not present.
//
// It exists so the move use cases do not each re-implement the same
// "project siblings, place, then find the row again to update it" loop, where
// forgetting to stop at the match is a silent bug. The caller still applies the
// key through the entity's own mutator, which is what bumps UpdatedAt -- hiding
// that here would make a real behaviour easy to lose track of.
//
// of maps a caller's type to an Item; items must already be sorted by key. A
// duplicate id resolves to the first occurrence.
func MoveWithin[T any](items []T, movedID, afterID string, of func(T) Item, g Growth) (moved T, key Key, ok bool, err error) {
	siblings := make([]Item, 0, len(items))
	for _, it := range items {
		e := of(it)
		if e.ID == movedID && !ok {
			moved, ok = it, true
			continue
		}
		siblings = append(siblings, e)
	}
	if !ok {
		var zero T
		return zero, "", false, nil
	}
	key, err = Place(siblings, afterID, g)
	return moved, key, true, err
}

// Append returns the key for a row added at the end of items, which is how
// categories, tags, payees, accounts and account folders are created.
func Append[T any](items []T, of func(T) Item, g Growth) (Key, error) {
	if len(items) == 0 {
		return Seed(g), nil
	}
	return Between(of(items[len(items)-1]).Key, "")
}

// Prepend returns the key for a row added at the front of items, which is how
// budget folders and budget envelope elements are created.
func Prepend[T any](items []T, of func(T) Item, g Growth) (Key, error) {
	if len(items) == 0 {
		return Seed(g), nil
	}
	return Between("", of(items[0]).Key)
}
