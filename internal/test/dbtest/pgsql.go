//go:build enginecompare

// PostgreSQL test opener, compiled ONLY under the `enginecompare` build tag so
// the fast (sqlite-only) suite never pulls lib/pq or requires a running
// Postgres. Build/run with: go test -tags enginecompare ./...
//
// It connects to the Postgres named by DATABASE_TEST_PGSQL_URL (e.g.
// postgres://econumo:econumo@localhost:5432/econumo_test?sslmode=disable). Each
// test gets a private, freshly-migrated SCHEMA (search_path) so tests are
// isolated and parallelizable against one database; the schema is dropped at
// test end. If the env var is unset the test is SKIPPED (not failed), so the
// tag can be enabled in CI where Postgres is available and skipped locally.
package dbtest

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	"github.com/econumo/econumo/internal/infra/storage/pgsql"
)

// PgsqlURLEnv is the env var naming the test Postgres connection URL.
const PgsqlURLEnv = "DATABASE_TEST_PGSQL_URL"

// NewPostgres opens a connection to the test Postgres, creates a private schema
// for this test, runs the PostgreSQL migrations into it, and returns a DB whose
// statements run with that schema first on the search_path. The schema is
// dropped on cleanup. SKIPS the test when DATABASE_TEST_PGSQL_URL is unset.
func NewPostgres(t testing.TB) *DB {
	t.Helper()
	url := os.Getenv(PgsqlURLEnv)
	if url == "" {
		t.Skipf("dbtest: %s not set — skipping PostgreSQL engine test", PgsqlURLEnv)
	}

	// Use the production opener (pgx, simple protocol) so the engine-parity suite
	// exercises the exact driver + protocol mode the server runs.
	raw, err := pgsql.OpenDB(url)
	if err != nil {
		t.Fatalf("dbtest: open postgres: %v", err)
	}
	if err := raw.PingContext(context.Background()); err != nil {
		_ = raw.Close()
		t.Fatalf("dbtest: ping postgres (%s): %v", PgsqlURLEnv, err)
	}
	// Pin to one connection so the session-level search_path sticks for every
	// statement this test issues.
	raw.SetMaxOpenConns(1)

	schema := pgSchemaName(t.Name())
	ctx := context.Background()
	if _, err := raw.ExecContext(ctx, fmt.Sprintf(`DROP SCHEMA IF EXISTS %q CASCADE; CREATE SCHEMA %q`, schema, schema)); err != nil {
		_ = raw.Close()
		t.Fatalf("dbtest: create schema %s: %v", schema, err)
	}
	if _, err := raw.ExecContext(ctx, fmt.Sprintf(`SET search_path TO %q`, schema)); err != nil {
		_ = raw.Close()
		t.Fatalf("dbtest: set search_path %s: %v", schema, err)
	}
	t.Cleanup(func() {
		_, _ = raw.ExecContext(context.Background(), fmt.Sprintf(`DROP SCHEMA IF EXISTS %q CASCADE`, schema))
		_ = raw.Close()
	})

	if err := migrate.Run(ctx, raw, toMigrationsPg(migrations.Pgsql())); err != nil {
		t.Fatalf("dbtest: migrate postgres: %v", err)
	}
	return &DB{Raw: raw, TX: backend.NewTxManager(raw), Engine: "postgresql"}
}

func toMigrationsPg(files []migrations.File) []migrate.Migration {
	out := make([]migrate.Migration, len(files))
	for i, f := range files {
		out[i] = migrate.Migration{Version: f.Version, SQL: f.SQL}
	}
	return out
}

// pgSchemaName derives a safe, unique schema identifier from a test name.
func pgSchemaName(testName string) string {
	r := strings.NewReplacer("/", "_", " ", "_", "-", "_", ".", "_")
	name := strings.ToLower(r.Replace(testName))
	// Keep it within Postgres' 63-char identifier limit, prefixed for clarity.
	const prefix = "test_"
	if len(name) > 63-len(prefix) {
		name = name[:63-len(prefix)]
	}
	return prefix + name
}
