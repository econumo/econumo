//go:build enginecompare

package enginecompare

// Replays the shared API scenario catalogue (internal/test/apiparity) against
// the production handler on BOTH SQLite and PostgreSQL from an identical seed,
// asserting every call's (status, raw body) is byte-identical, with SQLite as
// the reference (target) engine. The catalogue itself — Call/Scenario
// definitions, the harness, and the fixture seed — lives in the untagged
// apiparity package so the same scenarios also back the sqlite-only smoke
// suite (golden files, every `make go-test`).
//
// PostgreSQL SKIPs when DATABASE_TEST_PGSQL_URL is unset; the SQLite half still
// runs, so every scenario is always exercised.

import (
	"testing"

	"github.com/econumo/econumo/internal/test/apiparity"
	"github.com/econumo/econumo/internal/test/dbtest"
)

func TestAPIParity_Catalogue(t *testing.T) {
	for _, sc := range apiparity.Catalogue() {
		sc := sc
		t.Run(sc.Name, func(t *testing.T) {
			calls := sc.Calls()
			var refStatus []int
			var refBody [][]byte
			t.Run("sqlite", func(t *testing.T) {
				h := apiparity.NewHarness(t, dbtest.NewSQLite(t))
				refStatus, refBody = h.Replay(t, calls)
			})
			t.Run("postgresql", func(t *testing.T) {
				h := apiparity.NewHarness(t, dbtest.NewPostgres(t)) // SKIPs if env unset
				pgStatus, pgBody := h.Replay(t, calls)
				for i := range calls {
					if pgStatus[i] != refStatus[i] {
						t.Errorf("[%s] status mismatch: sqlite=%d pgsql=%d", calls[i].Label, refStatus[i], pgStatus[i])
					}
					if ref, pg := apiparity.NormalizeParity(refBody[i]), apiparity.NormalizeParity(pgBody[i]); ref != pg {
						t.Errorf("[%s] body mismatch:\n  sqlite: %s\n  pgsql : %s", calls[i].Label, ref, pg)
					}
				}
			})
		})
	}
}
