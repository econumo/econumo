package apiparity

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
)

// TestSmoke_Catalogue replays every catalogue scenario against the REAL
// production handler (server.BuildAPI) on a fresh sqlite DB and compares each
// normalized response against the committed golden file. Regenerate goldens
// with: UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ — then INSPECT the
// diff before committing: a golden change means observable behavior changed.
func TestSmoke_Catalogue(t *testing.T) {
	for _, sc := range Catalogue() {
		sc := sc
		t.Run(sc.Name, func(t *testing.T) {
			h := NewHarness(t, dbtest.NewSQLite(t))
			calls := sc.Calls()
			statuses, bodies := h.Replay(t, calls)

			var got strings.Builder
			for i, c := range calls {
				if !strings.HasPrefix(c.Label, "err:") && (statuses[i] < 200 || statuses[i] > 299) {
					t.Errorf("[%s] expected 2xx, got %d: %s", c.Label, statuses[i], bodies[i])
				}
				fmt.Fprintf(&got, "== %s %s %s [%s] -> %d\n%s\n", c.Label, c.Method, c.Path, c.Auth, statuses[i], NormalizeGolden(bodies[i]))
			}

			golden := filepath.Join("testdata", "golden", sc.Name+".golden")
			if os.Getenv("UPDATE_GOLDEN") != "" {
				if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, []byte(got.String()), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("missing golden %s (run with UPDATE_GOLDEN=1): %v", golden, err)
			}
			if string(want) != got.String() {
				t.Errorf("golden mismatch for %s.\n--- want\n%s\n--- got\n%s", sc.Name, want, got.String())
			}
		})
	}
}
