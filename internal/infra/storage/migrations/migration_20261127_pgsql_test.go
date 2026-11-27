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

func TestMigration20261127Pgsql(t *testing.T) {
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
	const schema = "test_migration_20261127"
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
	if err := migrate.Run(ctx, db, upTo("20261127000000"), migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
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
	checkUserDeletionSchema(t, db, rebind, func() {
		if err := migrate.Run(ctx, db, upTo(""), migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
			t.Fatalf("target migration: %v", err)
		}
	})

	// Whole seconds: the admin cursor pages on (created_at, id), and a
	// fractional created_at would repeat a page forever.
	rows, err := db.QueryContext(ctx, `SELECT table_name, column_name, data_type, datetime_precision FROM information_schema.columns
		WHERE table_schema = $1 AND (table_name IN ('users_audit', 'users_deletion_requests') OR column_name IN ('deletion_marked_at', 'last_active_at'))
		AND data_type LIKE 'timestamp%'`, schema)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var table, column, typ string
		var precision int
		if err := rows.Scan(&table, &column, &typ, &precision); err != nil {
			t.Fatal(err)
		}
		seen++
		if typ != "timestamp without time zone" || precision != 0 {
			t.Errorf("%s.%s is %s(%d), want timestamp without time zone(0)", table, column, typ, precision)
		}
	}
	if seen != 5 {
		t.Fatalf("saw %d datetime columns, want 5", seen)
	}
}
