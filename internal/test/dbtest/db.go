// Package dbtest provides shared test helpers for spinning up a migrated
// database and a TxManager. It centralizes what every handler/repo test
// previously duplicated (open + migrate + single-connection pin) so repository
// tests, app-service tests, and the engine-comparison suite all build their
// fixtures the same way.
//
// It is a TEST-ONLY package (imported only from *_test.go files). It is a normal
// package — not itself under _test.go — so it can be imported across packages;
// it must therefore avoid pulling production wiring it doesn't need.
package dbtest

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"testing"

	_ "modernc.org/sqlite" // register the pure-Go sqlite driver for tests

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
)

// DB bundles a migrated *sql.DB with a TxManager over it, plus the engine name.
// Tests use TX for repos (which take a *backend.TxManager) and DB.Raw for direct
// seeding/assertions.
type DB struct {
	Raw    *sql.DB
	TX     *backend.TxManager
	Engine string // "sqlite" or "postgresql"
}

// Rebind converts a query written with sqlite-style '?' placeholders to the
// engine's dialect, so raw verification SQL in a test runs on either engine
// ('?' -> '$1','$2',... on PostgreSQL; unchanged on sqlite). Use it whenever a
// test issues raw SQL through DB.Raw/DB.TX instead of a repository method.
func (d *DB) Rebind(query string) string {
	if d.Engine != "postgresql" {
		return query
	}
	var b strings.Builder
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(query[i])
	}
	return b.String()
}

// NewSQLite opens a fresh in-memory SQLite database, runs every migration, and
// returns it pinned to a single connection (SQLite's shared-cache in-memory DB
// needs MaxOpenConns(1) so all statements see the same data and the single
// writer rule holds). It is closed automatically at test end.
func NewSQLite(t testing.TB) *DB {
	t.Helper()
	// A per-test-named shared-cache in-memory DB: isolated between tests, shared
	// across this test's (single) connection.
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("dbtest: open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = raw.Close() })

	// Tests must run with the same pragmas as production: sqlite.Backend.Open
	// enables FK enforcement right after opening, and the schema relies on FK
	// cascade/SET NULL semantics.
	if _, err := raw.ExecContext(context.Background(), "PRAGMA foreign_keys = ON;"); err != nil {
		t.Fatalf("dbtest: sqlite pragma foreign_keys: %v", err)
	}

	if err := migrate.Run(context.Background(), raw, toMigrations(migrations.SQLite())); err != nil {
		t.Fatalf("dbtest: migrate sqlite: %v", err)
	}
	return &DB{Raw: raw, TX: backend.NewTxManager(raw), Engine: "sqlite"}
}

// toMigrations adapts the embedded migration files to the runner's type.
func toMigrations(files []migrations.File) []migrate.Migration {
	out := make([]migrate.Migration, len(files))
	for i, f := range files {
		out[i] = migrate.Migration{Version: f.Version, SQL: f.SQL}
	}
	return out
}

// Exec runs a statement against the raw DB, failing the test on error. A thin
// helper for seeding fixture rows in tests.
func (d *DB) Exec(t testing.TB, query string, args ...any) {
	t.Helper()
	if _, err := d.Raw.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("dbtest: exec %q: %v", query, err)
	}
}
