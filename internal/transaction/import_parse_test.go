package transaction

import (
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
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
	ids, ok, _ := resolveOverrideLabelIDs(&csv, nil)
	if !ok {
		t.Fatalf("ok = false, want true (blank pieces are not a not-found error)")
	}
	if ids != nil {
		t.Fatalf("ids = %#v, want nil (treated as absent, not an explicit empty override)", ids)
	}
}

// TestResolveOverrideLabelIDs_DedupesRepeatedIds: the same id repeated in the
// CSV must resolve to one entry, not one per occurrence — otherwise the
// per-row ReplaceLabels write would carry duplicate ids for every imported
// row.
func TestResolveOverrideLabelIDs_DedupesRepeatedIds(t *testing.T) {
	id1 := vo.NewId()
	list := []model.ImportNamed{{ID: id1.String(), Name: "L1", OwnerID: vo.NewId().String()}}
	csv := id1.String() + "," + id1.String() + "," + id1.String()
	ids, ok, tooMany := resolveOverrideLabelIDs(&csv, list)
	if !ok || tooMany {
		t.Fatalf("ok=%v tooMany=%v, want ok=true tooMany=false", ok, tooMany)
	}
	if len(ids) != 1 || !ids[0].Equal(id1) {
		t.Fatalf("ids = %#v, want exactly [%s] (deduped)", ids, id1)
	}
}

// TestResolveOverrideLabelIDs_ExceedsCap_RejectedAsTooMany pins
// maxLabelsPerImportRow as the bound on the DISTINCT id count (after dedupe):
// this override fans out to every imported row, so an unbounded value would
// turn a single bad request into per-row work sized to whatever the caller
// sent. tooMany must be reported so the caller sees a dedicated message
// rather than the generic "not found" one.
func TestResolveOverrideLabelIDs_ExceedsCap_RejectedAsTooMany(t *testing.T) {
	list := make([]model.ImportNamed, maxLabelsPerImportRow+1)
	ownerID := vo.NewId().String()
	ids := make([]string, maxLabelsPerImportRow+1)
	for i := range list {
		id := vo.NewId()
		list[i] = model.ImportNamed{ID: id.String(), Name: id.String(), OwnerID: ownerID}
		ids[i] = id.String()
	}
	csv := strings.Join(ids, ",")
	resolved, ok, tooMany := resolveOverrideLabelIDs(&csv, list)
	if ok || !tooMany {
		t.Fatalf("ok=%v tooMany=%v, want ok=false tooMany=true (%d distinct ids exceeds the cap of %d)", ok, tooMany, len(ids), maxLabelsPerImportRow)
	}
	if resolved != nil {
		t.Fatalf("resolved = %#v, want nil on rejection", resolved)
	}
}

// TestResolveOverrideLabelIDs_ExactlyAtCap_Accepted proves the check is an
// off-by-one-safe ">", not ">=".
func TestResolveOverrideLabelIDs_ExactlyAtCap_Accepted(t *testing.T) {
	list := make([]model.ImportNamed, maxLabelsPerImportRow)
	ownerID := vo.NewId().String()
	ids := make([]string, maxLabelsPerImportRow)
	for i := range list {
		id := vo.NewId()
		list[i] = model.ImportNamed{ID: id.String(), Name: id.String(), OwnerID: ownerID}
		ids[i] = id.String()
	}
	csv := strings.Join(ids, ",")
	resolved, ok, tooMany := resolveOverrideLabelIDs(&csv, list)
	if !ok || tooMany {
		t.Fatalf("ok=%v tooMany=%v, want ok=true tooMany=false at exactly the cap", ok, tooMany)
	}
	if len(resolved) != maxLabelsPerImportRow {
		t.Fatalf("resolved count = %d, want %d", len(resolved), maxLabelsPerImportRow)
	}
}
