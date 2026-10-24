//go:build enginecompare

package migrations_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	"github.com/econumo/econumo/internal/infra/storage/pgsql"
)

// The PostgreSQL twin of TestMigration20261024_SortKeyBackfillKeepsOrder.
func TestMigration20261024_Pgsql(t *testing.T) {
	url := os.Getenv("DATABASE_TEST_PGSQL_URL")
	if url == "" {
		t.Skip("DATABASE_TEST_PGSQL_URL not set")
	}
	db, err := pgsql.OpenDB(url)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	const schema = "test_migration_20261024"
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`DROP SCHEMA IF EXISTS %[1]s CASCADE; CREATE SCHEMA %[1]s; SET search_path TO %[1]s`, schema)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
		_ = db.Close()
	})
	upTo := func(version string) []migrate.Migration {
		var out []migrate.Migration
		for _, f := range migrations.Pgsql() {
			if version == "" || f.Version < version {
				out = append(out, migrate.Migration{Version: f.Version, SQL: f.SQL, Command: f.Command})
			}
		}
		return out
	}
	if err := migrate.Run(ctx, db, upTo("20261024000000"), migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatalf("pre-migrations: %v", err)
	}
	rebind := func(q string) string {
		var b strings.Builder
		n := 0
		for _, r := range q {
			if r == '?' {
				n++
				fmt.Fprintf(&b, "$%d", n)
				continue
			}
			b.WriteRune(r)
		}
		return b.String()
	}
	checkSortKeyMigration(t, db, rebind, func() {
		if err := migrate.Run(ctx, db, upTo(""), migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
			t.Fatalf("target migration: %v", err)
		}
	})
}
