package vo

import (
	"testing"

	"github.com/google/uuid"
)

// TestNewId_IsV7 confirms generated ids are UUIDv7 (time-ordered), so index
// inserts stay local. Existing rows keep their original ids; only new ids are v7.
func TestNewId_IsV7(t *testing.T) {
	id := NewId()
	u, err := uuid.Parse(id.String())
	if err != nil {
		t.Fatalf("NewId produced an unparseable uuid %q: %v", id.String(), err)
	}
	if got := u.Version(); got != 7 {
		t.Fatalf("NewId version = %d, want 7", got)
	}
}

// TestNewId_TimeOrdered confirms successive ids sort in creation order (the
// whole point of v7 — consistent index growth). String sort == time order.
func TestNewId_TimeOrdered(t *testing.T) {
	const n = 50
	var prev string
	for i := 0; i < n; i++ {
		cur := NewId().String()
		if i > 0 && cur < prev {
			t.Fatalf("id %d (%s) sorts before previous (%s) — not time-ordered", i, cur, prev)
		}
		prev = cur
	}
}

// TestId_IsZero confirms the zero-value Id reports IsZero, while a parsed
// non-zero Id does not.
func TestId_IsZero(t *testing.T) {
	var zero Id
	if !zero.IsZero() {
		t.Error("zero-value Id.IsZero() = false, want true")
	}
	id := MustParseId("f680553f-6b40-407d-a528-5123913be0aa")
	if id.IsZero() {
		t.Error("parsed non-zero Id.IsZero() = true, want false")
	}
}

// TestId_MarshalJSON confirms an Id marshals to its bare quoted string form.
func TestId_MarshalJSON(t *testing.T) {
	const s = "f680553f-6b40-407d-a528-5123913be0aa"
	id := MustParseId(s)
	got, err := id.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	want := `"` + s + `"`
	if string(got) != want {
		t.Errorf("MarshalJSON() = %s, want %s", got, want)
	}
}

// TestParseId_AcceptsExistingV4 confirms we still accept the existing (v4) ids
// already in the database — the migration to v7 is for NEW ids only.
func TestParseId_AcceptsExistingV4(t *testing.T) {
	const existingV4 = "f680553f-6b40-407d-a528-5123913be0aa"
	id, err := ParseId(existingV4)
	if err != nil {
		t.Fatalf("ParseId rejected an existing v4 id: %v", err)
	}
	if id.String() != existingV4 {
		t.Fatalf("ParseId mutated the id: got %q want %q", id.String(), existingV4)
	}
}
