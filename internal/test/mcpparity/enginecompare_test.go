//go:build enginecompare

package mcpparity

// Replays the mcpparity catalogue against the production /mcp handler on BOTH
// SQLite and PostgreSQL from an identical seed, asserting every step's
// parity-normalized (UUIDv7/token/invite-code redacted, but datetimes kept)
// body is byte-identical, with SQLite as the reference engine. Mirrors
// internal/test/enginecompare/apiparity_test.go's loop shape.
//
// PostgreSQL SKIPs when DATABASE_TEST_PGSQL_URL is unset; the SQLite half
// still runs, so every scenario is always exercised.

import (
	"testing"

	"github.com/econumo/econumo/internal/test/apiparity"
	"github.com/econumo/econumo/internal/test/dbtest"
)

func TestMCPParity_Catalogue(t *testing.T) {
	for _, sc := range Catalogue() {
		sc := sc
		t.Run(sc.Name, func(t *testing.T) {
			var ref []stepResult
			t.Run("sqlite", func(t *testing.T) {
				h := apiparity.NewHarness(t, dbtest.NewSQLite(t))
				ref = runSteps(t, h, sc)
			})
			t.Run("postgresql", func(t *testing.T) {
				h := apiparity.NewHarness(t, dbtest.NewPostgres(t)) // SKIPs if env unset
				pg := runSteps(t, h, sc)
				for i, st := range sc.Steps {
					if ref[i].Status != pg[i].Status {
						t.Errorf("[%s] status mismatch: sqlite=%d pgsql=%d", st.Label, ref[i].Status, pg[i].Status)
					}
					refBody, pgBody := apiparity.NormalizeParity(ref[i].Body), apiparity.NormalizeParity(pg[i].Body)
					if refBody != pgBody {
						t.Errorf("[%s] body mismatch:\n  sqlite: %s\n  pgsql : %s", st.Label, refBody, pgBody)
					}
				}
			})
		})
	}
}
