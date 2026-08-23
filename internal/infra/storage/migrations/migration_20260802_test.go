package migrations_test

// Verifies 20260802000000: the is_deleted column defaults to not-deleted for
// existing rows, and the rewritten partial unique index frees a deleted row's
// (user_id, code) pair while still rejecting duplicates among live rows.

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	_ "modernc.org/sqlite"
)

const softDeleteVersion = "20260802000000"

func TestMigration20260802_BackfillsNotDeletedAndFreesDeletedCodes(t *testing.T) {
	db := openFK(t)
	ctx := context.Background()
	all := toRunList(migrations.SQLite())
	var before []migrate.Migration
	for _, m := range all {
		if m.Version < softDeleteVersion {
			before = append(before, m)
		}
	}
	if err := migrate.Run(ctx, db, before, migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatalf("pre-migrations: %v", err)
	}
	seedUser(t, db, "u1")
	if _, err := db.ExecContext(ctx, `INSERT INTO currencies (id, code, symbol, name, fraction_digits, user_id, rate, created_at)
		VALUES ('c1', 'PTS', 'P', NULL, 2, 'u1', '10.00000000', '2026-01-01 00:00:00')`); err != nil {
		t.Fatalf("seed custom: %v", err)
	}

	if err := migrate.Run(ctx, db, all, migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatalf("target migration: %v", err)
	}

	var deleted bool
	if err := db.QueryRowContext(ctx, `SELECT is_deleted FROM currencies WHERE id = 'c1'`).Scan(&deleted); err != nil {
		t.Fatalf("scan is_deleted: %v", err)
	}
	if deleted {
		t.Fatal("existing row came out deleted; the column must default to not-deleted")
	}

	// A live duplicate is still rejected.
	if _, err := db.ExecContext(ctx, `INSERT INTO currencies (id, code, symbol, name, fraction_digits, user_id, rate, created_at, is_deleted)
		VALUES ('c2', 'PTS', 'P', NULL, 2, 'u1', '10.00000000', '2026-01-01 00:00:00', 0)`); err == nil {
		t.Fatal("duplicate live (user_id, code) was accepted; the partial unique index is not enforcing")
	}

	// Once the original is deleted, the code is free again.
	if _, err := db.ExecContext(ctx, `UPDATE currencies SET is_deleted = 1 WHERE id = 'c1'`); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO currencies (id, code, symbol, name, fraction_digits, user_id, rate, created_at, is_deleted)
		VALUES ('c2', 'PTS', 'P', NULL, 2, 'u1', '10.00000000', '2026-01-01 00:00:00', 0)`); err != nil {
		t.Fatalf("re-creating a deleted code must succeed: %v", err)
	}

	// Two users may still share a code.
	seedUser(t, db, "u2")
	if _, err := db.ExecContext(ctx, `INSERT INTO currencies (id, code, symbol, name, fraction_digits, user_id, rate, created_at, is_deleted)
		VALUES ('c3', 'PTS', 'P', NULL, 2, 'u2', '10.00000000', '2026-01-01 00:00:00', 0)`); err != nil {
		t.Fatalf("cross-user same code must succeed: %v", err)
	}
}
