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
