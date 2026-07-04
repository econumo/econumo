//go:build enginecompare

package dbtest

import (
	"os"
	"testing"
)

// New returns a migrated test DB for the engine named by DBTEST_ENGINE
// ("pgsql" -> Postgres, else SQLite). The Postgres path skips when
// DATABASE_TEST_PGSQL_URL is unset. Only compiled under -tags enginecompare,
// where the Postgres opener (pgsql.go) is available.
func New(t testing.TB) *DB {
	if os.Getenv("DBTEST_ENGINE") == "pgsql" {
		return NewPostgres(t)
	}
	return NewSQLite(t)
}
