//go:build !enginecompare

package dbtest

import "testing"

// New returns a migrated test DB. In the default (untagged) build only SQLite
// is linked, so it always returns a SQLite DB. Build with -tags enginecompare
// to make New honour DBTEST_ENGINE=pgsql (see select_pgsql.go).
func New(t testing.TB) *DB { return NewSQLite(t) }
