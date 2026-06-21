//go:build enginecompare

package enginecompare

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/test/dbtest"
)

// fixed identifiers + time used across scenarios. UUIDs are literal so both
// engines store the same keys; the comparison is over the repo OUTPUTS.
const (
	userA = "11111111-1111-1111-1111-111111111111"
	userB = "22222222-2222-2222-2222-222222222222"
	usdID = "dffc2a06-6f29-4704-8575-31709adee926" // seeded by the baseline migration
)

var fixedTime = time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)

// scenario runs a sequence of repo operations against db and returns a
// deterministic, comparable snapshot string. The SAME scenario is run on both
// engines and the snapshots must be identical.
type scenario func(t *testing.T, db *dbtest.DB) string

// runOnBoth executes the scenario on a fresh SQLite DB and a fresh PostgreSQL DB
// and asserts the two snapshots match. The PostgreSQL half SKIPS (via
// dbtest.NewPostgres) when DATABASE_TEST_PGSQL_URL is unset — but the SQLite
// half still runs, so the scenario itself is always exercised.
func runOnBoth(t *testing.T, s scenario) {
	t.Helper()

	var sqliteSnap string
	t.Run("sqlite", func(t *testing.T) {
		sqliteSnap = s(t, dbtest.NewSQLite(t))
	})

	t.Run("postgresql", func(t *testing.T) {
		pg := dbtest.NewPostgres(t) // SKIPs if DATABASE_TEST_PGSQL_URL unset
		pgSnap := s(t, pg)
		if pgSnap != sqliteSnap {
			t.Fatalf("engine snapshots differ:\n  sqlite: %s\n  pgsql : %s", sqliteSnap, pgSnap)
		}
	})
}
