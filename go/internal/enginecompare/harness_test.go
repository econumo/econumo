//go:build enginecompare

package enginecompare

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/testutil"
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
type scenario func(t *testing.T, db *testutil.DB) string

// runOnBoth executes the scenario on a fresh SQLite DB and a fresh PostgreSQL DB
// and asserts the two snapshots match. The PostgreSQL half SKIPS (via
// testutil.NewPostgres) when DATABASE_TEST_PGSQL_URL is unset — but the SQLite
// half still runs, so the scenario itself is always exercised.
func runOnBoth(t *testing.T, s scenario) {
	t.Helper()

	var sqliteSnap string
	t.Run("sqlite", func(t *testing.T) {
		sqliteSnap = s(t, testutil.NewSQLite(t))
	})

	t.Run("postgresql", func(t *testing.T) {
		pg := testutil.NewPostgres(t) // SKIPs if DATABASE_TEST_PGSQL_URL unset
		pgSnap := s(t, pg)
		if pgSnap != sqliteSnap {
			t.Fatalf("engine snapshots differ:\n  sqlite: %s\n  pgsql : %s", sqliteSnap, pgSnap)
		}
	})
}

// ---- portable seeding ----

// rebind converts ?-style placeholders to the engine's form ($1.. for pgsql,
// left as ? for sqlite), so one seed statement works on both engines.
func rebind(engine, query string) string {
	if engine != "postgresql" {
		return query
	}
	var b strings.Builder
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
		} else {
			b.WriteByte(query[i])
		}
	}
	return b.String()
}

// seed runs an INSERT (or other statement) against the raw DB with engine-aware
// placeholders, failing the test on error.
func seed(t *testing.T, db *testutil.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Raw.ExecContext(context.Background(), rebind(db.Engine, query), args...); err != nil {
		t.Fatalf("seed (%s) %q: %v", db.Engine, query, err)
	}
}

// seedUser inserts a minimal active user. Both engines store identical column
// values; the encrypted-email/identifier are not exercised here (those have
// their own crypto golden-vector tests), so plain placeholder values are used.
func seedUser(t *testing.T, db *testutil.DB, id, email string) {
	t.Helper()
	// Boolean columns use TRUE/FALSE literals (Postgres has real BOOLEAN columns;
	// SQLite accepts the literals too and stores 1/0) — integer 1/0 would fail on
	// Postgres with "column is of type boolean but expression is of type integer".
	seed(t, db, `INSERT INTO users (id, identifier, email, name, avatar_url, password, salt, created_at, updated_at, is_active)
		VALUES (?, ?, ?, ?, '', 'x', 'salt', ?, ?, TRUE)`,
		id, "ident-"+id[:8], email, "User "+id[:4], fixedTime, fixedTime)
}
