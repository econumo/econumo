//go:build enginecompare

package sqliteimport_test

import (
	"context"
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/sqliteimport"
	"github.com/econumo/econumo/internal/test/dbtest"
)

// seedSource inserts one user, one currency, one account and one transaction into
// a migrated SQLite DB, covering a boolean (is_deleted), a NUMERIC amount, a
// timestamp, and a NULL FK (category_id). Returns the user id.
func seedSource(t *testing.T, src *dbtest.DB) string {
	t.Helper()
	const uid = "0190a0aa-0000-7000-8000-000000000001"
	const cid = "0190a0aa-0000-7000-8000-0000000000c1"
	const aid = "0190a0aa-0000-7000-8000-0000000000a1"
	const tid = "0190a0aa-0000-7000-8000-0000000000d1"
	ts := "2026-07-23 10:00:00"
	src.Exec(t, `INSERT INTO currencies (id, code, symbol, created_at) VALUES (?,?,?,?)`,
		cid, "EUR", "€", ts)
	src.Exec(t, `INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, language, is_active, email_verified, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		uid, uid, "a@b.test", "A", "diamond:sky", "x", "", "argon2id", "en", 1, 1, ts, ts)
	src.Exec(t, `INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		aid, cid, uid, "Cash", 1, "wallet", 1, ts, ts)
	src.Exec(t, `INSERT INTO transactions (id, user_id, account_id, type, amount, description, created_at, updated_at, spent_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		tid, uid, aid, 1, "42.50", "lunch", ts, ts, ts)
	return uid
}

func TestImport_CopiesAllTablesWithTypes(t *testing.T) {
	ctx := context.Background()
	src := dbtest.NewSQLite(t)
	dst := dbtest.NewPostgres(t) // migrated; seeds the default USD currency

	seedSource(t, src)

	report, err := sqliteimport.Import(ctx, src.Raw, dst.Raw, false)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Total == 0 {
		t.Fatal("expected rows copied, got 0")
	}

	// Row counts: TRUNCATE ... CASCADE removed the target's seeded USD row, then
	// the copy inserted the source's rows (its own migration-seeded USD plus the
	// inserted EUR).
	var accounts, currencies, txns int
	dst.Raw.QueryRowContext(ctx, `SELECT count(*) FROM accounts`).Scan(&accounts)
	dst.Raw.QueryRowContext(ctx, `SELECT count(*) FROM currencies`).Scan(&currencies)
	dst.Raw.QueryRowContext(ctx, `SELECT count(*) FROM transactions`).Scan(&txns)
	if accounts != 1 || txns != 1 {
		t.Fatalf("expected 1 account and 1 transaction, got %d / %d", accounts, txns)
	}
	// Both engines' migrations seed a default USD currency, so the source itself
	// carries that seed plus the inserted EUR; the target's own seed row is
	// truncated and replaced entirely by the source's two rows.
	if currencies != 2 {
		t.Fatalf("expected 2 currencies from the source (migration seed + inserted EUR), got %d", currencies)
	}

	// Boolean round-trip: is_deleted was 1 in SQLite -> true in Postgres.
	var isDeleted bool
	if err := dst.Raw.QueryRowContext(ctx, `SELECT is_deleted FROM accounts`).Scan(&isDeleted); err != nil {
		t.Fatalf("scan is_deleted: %v", err)
	}
	if !isDeleted {
		t.Fatal("expected is_deleted = true")
	}

	// NUMERIC round-trip: 42.50, rendered at the column's NUMERIC(19,8) scale.
	var amount string
	if err := dst.Raw.QueryRowContext(ctx, `SELECT amount::text FROM transactions`).Scan(&amount); err != nil {
		t.Fatalf("scan amount: %v", err)
	}
	if amount != "42.50000000" {
		t.Fatalf("expected amount 42.50000000, got %q", amount)
	}

	// NULL FK preserved.
	var categoryNull bool
	dst.Raw.QueryRowContext(ctx, `SELECT category_id IS NULL FROM transactions`).Scan(&categoryNull)
	if !categoryNull {
		t.Fatal("expected category_id to be NULL")
	}
}

func TestImport_AbortsWhenTargetHasUsersWithoutForce(t *testing.T) {
	ctx := context.Background()
	src := dbtest.NewSQLite(t)
	dst := dbtest.NewPostgres(t)
	seedSource(t, src)

	// Put a user into the target so the guard trips.
	dst.Exec(t, dst.Rebind(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, language, is_active, email_verified, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`),
		"0190a0aa-0000-7000-8000-0000000000e1", "0190a0aa-0000-7000-8000-0000000000e1", "x@y.test", "X", "diamond:sky", "x", "",
		"argon2id", "en", true, true, "2026-07-23 10:00:00", "2026-07-23 10:00:00")

	if _, err := sqliteimport.Import(ctx, src.Raw, dst.Raw, false); !errors.Is(err, sqliteimport.ErrTargetNotEmpty) {
		t.Fatalf("expected ErrTargetNotEmpty, got %v", err)
	}

	// With force it replaces cleanly.
	if _, err := sqliteimport.Import(ctx, src.Raw, dst.Raw, true); err != nil {
		t.Fatalf("Import(force): %v", err)
	}
	var users int
	dst.Raw.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&users)
	if users != 1 {
		t.Fatalf("expected 1 user after forced replace, got %d", users)
	}
}

func TestImport_AbortsOnSchemaVersionMismatch(t *testing.T) {
	ctx := context.Background()
	src := dbtest.NewSQLite(t)
	dst := dbtest.NewPostgres(t)
	seedSource(t, src)

	// Simulate a source at an older schema by dropping one recorded migration
	// version from its bookkeeping table. force=true proves the schema check
	// runs regardless of the overwrite guard.
	src.Exec(t, `DELETE FROM schema_migrations WHERE version = (SELECT max(version) FROM schema_migrations)`)

	if _, err := sqliteimport.Import(ctx, src.Raw, dst.Raw, true); !errors.Is(err, sqliteimport.ErrSchemaMismatch) {
		t.Fatalf("expected ErrSchemaMismatch, got %v", err)
	}
}
