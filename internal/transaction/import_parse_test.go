package transaction

import (
	"testing"
)

// TestSplitLabelCell_TrimsBlanksAndDedupesCaseInsensitively pins the
// resolution-independent contract splitLabelCell owns: dedup happens on the
// SPLIT PIECES themselves, case-insensitively, before any name resolution
// runs. Asserting len(names)==1 directly on the helper's output (rather than
// only on the resolved label count, which a case-insensitive nameCache
// lookup + an ON CONFLICT DO NOTHING insert both already collapse to one row
// regardless of this function's own behavior) is what would catch a future
// regression — e.g. Task 8's per-row cap counting len(names) against a limit,
// where "A;a" must count as 1, not 2.
func TestSplitLabelCell_TrimsBlanksAndDedupesCaseInsensitively(t *testing.T) {
	tests := []struct {
		name string
		cell string
		sep  string
		want []string
	}{
		{name: "default separator, case-insensitive dup + blank piece", cell: "Kid A;kid a; ;Kid A", sep: ";", want: []string{"Kid A"}},
		{name: "blank cell", cell: "", sep: ";", want: nil},
		{name: "custom separator", cell: "Kid A|Kid B", sep: "|", want: []string{"Kid A", "Kid B"}},
		{name: "all blank pieces", cell: ",,", sep: ",", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitLabelCell(tt.cell, tt.sep)
			if len(got) != len(tt.want) {
				t.Fatalf("splitLabelCell(%q, %q) = %#v, want %#v", tt.cell, tt.sep, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("splitLabelCell(%q, %q) = %#v, want %#v", tt.cell, tt.sep, got, tt.want)
				}
			}
		})
	}
}

// TestResolveOverrideLabelIDs_AllBlankPieces_TreatedAsAbsent pins the "only
// separators" edge case: idsCSV == "," must behave exactly like an absent
// override (nil, true), not an explicit "no labels" (a non-nil empty slice),
// so a mapped labels column still resolves per row instead of being silently
// suppressed.
func TestResolveOverrideLabelIDs_AllBlankPieces_TreatedAsAbsent(t *testing.T) {
	csv := ","
	ids, ok := resolveOverrideLabelIDs(&csv, nil)
	if !ok {
		t.Fatalf("ok = false, want true (blank pieces are not a not-found error)")
	}
	if ids != nil {
		t.Fatalf("ids = %#v, want nil (treated as absent, not an explicit empty override)", ids)
	}
}
