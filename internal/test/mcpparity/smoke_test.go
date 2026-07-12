package mcpparity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/test/apiparity"
	"github.com/econumo/econumo/internal/test/dbtest"
)

// TestSmoke_Catalogue replays every mcpparity scenario against the REAL
// production handler (server.BuildAPI, mounted at /mcp) on a fresh sqlite DB
// and compares each normalized step transcript against the committed golden
// file. Regenerate goldens with: UPDATE_GOLDEN=1 go test ./internal/test/mcpparity/
// — then INSPECT the diff before committing: a golden change means observable
// MCP wire behavior changed.
func TestSmoke_Catalogue(t *testing.T) {
	if len(Catalogue()) < 6 {
		t.Fatal("mcp catalogue shrank")
	}
	for _, sc := range Catalogue() {
		sc := sc
		t.Run(sc.Name, func(t *testing.T) {
			h := apiparity.NewHarness(t, dbtest.NewSQLite(t))
			blocks := Run(t, h, sc)
			got := strings.Join(blocks, "\n") + "\n"

			golden := filepath.Join("testdata", "golden", sc.Name+".golden")
			if os.Getenv("UPDATE_GOLDEN") != "" {
				if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("missing golden %s (run with UPDATE_GOLDEN=1): %v", golden, err)
			}
			if string(want) != got {
				t.Errorf("golden mismatch for %s.\n--- want\n%s\n--- got\n%s", sc.Name, want, got)
			}
		})
	}
}
