package apiparity

import "testing"

// The catalogue is populated by init() registration; a refactor that drops a
// scenario file would otherwise fail only as a silently-smaller test run.
// The floor tracks the registered scenario count — raise it as scenarios are
// added, never lower it.
func TestCatalogueSize(t *testing.T) {
	const min = 54 // raised for the §5a deleted-account scenario (2026-08)
	if n := len(Catalogue()); n < min {
		t.Fatalf("catalogue has %d scenarios, want >= %d — a scenario file was dropped", n, min)
	}
	seen := map[string]bool{}
	for _, sc := range Catalogue() {
		if sc.Name == "" {
			t.Error("scenario with empty name")
		}
		if seen[sc.Name] {
			t.Errorf("duplicate scenario name %q", sc.Name)
		}
		seen[sc.Name] = true
	}
}
