package migrations_test

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
)

// emailVerifiedMigration is the version that adds users.email_verified.
const emailVerifiedMigration = "20260721000000"

// TestEmailVerifiedBackfillAndDefault pins the two properties the column's
// migration must hold: pre-existing users are grandfathered in as verified,
// and the column default is false so any future insert that omits the field
// fails closed (unverified) instead of open.
func TestEmailVerifiedBackfillAndDefault(t *testing.T) {
	db, err := sql.Open("sqlite", "file:backfill?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	var before, after []migrate.Migration
	for _, f := range migrations.SQLite() {
		m := migrate.Migration{Version: f.Version, SQL: f.SQL}
		if f.Version < emailVerifiedMigration {
			before = append(before, m)
		} else {
			after = append(after, m)
		}
	}

	// Apply everything up to (not including) the email_verified migration, then
	// seed a legacy user that predates the column.
	if err := migrate.Run(ctx, db, before); err != nil {
		t.Fatalf("pre-migrate: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO users (id, identifier, email, name, avatar, password, salt, created_at, updated_at) VALUES ('u1','ident1','e','n','a','p','s','2020-01-01 00:00:00','2020-01-01 00:00:00')`); err != nil {
		t.Fatalf("seed legacy user: %v", err)
	}

	// Run is idempotent (tracks applied versions), so replaying the full set
	// only applies the remaining migrations, including email_verified.
	if err := migrate.Run(ctx, db, append(before, after...)); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var legacy bool
	if err := db.QueryRowContext(ctx, `SELECT email_verified FROM users WHERE id='u1'`).Scan(&legacy); err != nil {
		t.Fatalf("read legacy user: %v", err)
	}
	if !legacy {
		t.Error("pre-existing user must be backfilled email_verified=true")
	}

	// A raw insert omitting email_verified must default to false (fail closed).
	if _, err := db.ExecContext(ctx, `INSERT INTO users (id, identifier, email, name, avatar, password, salt, created_at, updated_at) VALUES ('u2','ident2','e','n','a','p','s','2026-01-01 00:00:00','2026-01-01 00:00:00')`); err != nil {
		t.Fatalf("insert new user: %v", err)
	}
	var fresh bool
	if err := db.QueryRowContext(ctx, `SELECT email_verified FROM users WHERE id='u2'`).Scan(&fresh); err != nil {
		t.Fatalf("read new user: %v", err)
	}
	if fresh {
		t.Error("column default must be false so a field-omitting insert fails closed")
	}
}
