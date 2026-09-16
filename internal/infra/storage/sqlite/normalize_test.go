package sqlite_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/sqlite"
	"github.com/econumo/econumo/internal/test/dbtest"
)

// seedUser writes created_at/updated_at/access_until verbatim: CAST(? AS TEXT)
// keeps the driver's time handling out of the stored form under test.
func seedUser(t *testing.T, db *sql.DB, id, createdAt, updatedAt string, accessUntil any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), `INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, access_until, is_active)
		VALUES (?, ?, ?, 'U', 'face:sky', 'x', 's', 'argon2id', CAST(? AS TEXT), CAST(? AS TEXT), ?, 1)`,
		id, id, id+"@x.test", createdAt, updatedAt, accessUntil); err != nil {
		t.Fatalf("seed user %s: %v", id, err)
	}
}

func text(t *testing.T, db *sql.DB, query string, args ...any) sql.NullString {
	t.Helper()
	var s sql.NullString
	if err := db.QueryRowContext(context.Background(), query, args...).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNormalizeDatetimes_RewritesDriverStringForm(t *testing.T) {
	db := dbtest.NewSQLite(t).Raw
	ctx := context.Background()

	seedUser(t, db, "go-utc", "2026-09-14 10:00:00.123456789 +0000 UTC", "2026-09-14 10:00:00 +0000 UTC", nil)
	seedUser(t, db, "go-offset", "2026-09-14 13:00:00 +0300 MSK", "2026-01-01 00:30:00.5 -0100 -01", "2026-10-01 00:00:00 +0000 UTC m=+0.000012345")
	// A zone with no abbreviation (an RFC3339 offset parsed by the CSV
	// importer) prints its offset twice.
	seedUser(t, db, "go-fixed", "2024-04-10 10:00:00 +0300 +0300", "2024-04-10 10:00:00.25 -0130 -0130", nil)
	seedUser(t, db, "legacy", "2021-08-12 21:05:48", "2021-08-12 21:05:48", "2026-10-01 00:00:00")
	seedUser(t, db, "odd", "2026-08-01T00:00:00Z", "2026-02-30 10:00:00 +0000 UTC", nil)
	seedUser(t, db, "odd-fraction", "2026-01-01 10:00:00.5x +0000 UTC", "2026-01-01 10:00:00", nil)
	seedUser(t, db, "odd-suffix", "2026-09-14 10:00:00 +0000 UTC-not-a-driver-value", "2026-09-14 10:00:00", nil)
	seedUser(t, db, "odd-mono", "2026-09-14 10:00:00 +0000 UTC m=not-a-monotonic-reading", "2026-09-14 10:00:00 +0300 MSK m=+1.5", nil)
	firstMigration := text(t, db, `SELECT CAST(applied_at AS TEXT) FROM schema_migrations ORDER BY version LIMIT 1`)

	report, err := sqlite.NormalizeDatetimes(ctx, db)
	if err != nil {
		t.Fatalf("NormalizeDatetimes: %v", err)
	}

	for _, c := range []struct{ id, col, want string }{
		{"go-utc", "created_at", "2026-09-14 10:00:00"},
		{"go-utc", "updated_at", "2026-09-14 10:00:00"},
		{"go-offset", "created_at", "2026-09-14 10:00:00"},
		{"go-offset", "updated_at", "2026-01-01 01:30:00"},
		{"go-offset", "access_until", "2026-10-01 00:00:00"},
		{"go-fixed", "created_at", "2024-04-10 07:00:00"},
		{"go-fixed", "updated_at", "2024-04-10 11:30:00"},
		{"legacy", "created_at", "2021-08-12 21:05:48"},
		{"legacy", "access_until", "2026-10-01 00:00:00"},
		// Not the driver's form: left exactly as stored.
		{"odd", "created_at", "2026-08-01T00:00:00Z"},
		{"odd", "updated_at", "2026-02-30 10:00:00 +0000 UTC"},
		{"odd-fraction", "created_at", "2026-01-01 10:00:00.5x +0000 UTC"},
		{"odd-suffix", "created_at", "2026-09-14 10:00:00 +0000 UTC-not-a-driver-value"},
		// Go prints a monotonic reading as sign, seconds, '.', nine digits.
		{"odd-mono", "created_at", "2026-09-14 10:00:00 +0000 UTC m=not-a-monotonic-reading"},
		{"odd-mono", "updated_at", "2026-09-14 10:00:00 +0300 MSK m=+1.5"},
	} {
		if got := text(t, db, `SELECT CAST(`+c.col+` AS TEXT) FROM users WHERE id = ?`, c.id); got.String != c.want {
			t.Errorf("%s.%s = %q, want %q", c.id, c.col, got.String, c.want)
		}
	}
	if got := text(t, db, `SELECT access_until FROM users WHERE id = 'go-utc'`); got.Valid {
		t.Errorf("NULL access_until became %q", got.String)
	}
	// The instance id hashes the first schema_migrations row; never rewrite it.
	if got := text(t, db, `SELECT CAST(applied_at AS TEXT) FROM schema_migrations ORDER BY version LIMIT 1`); got != firstMigration {
		t.Errorf("schema_migrations.applied_at changed: %q -> %q", firstMigration.String, got.String)
	}

	if report.Rewritten != 7 {
		t.Errorf("Rewritten = %d, want 7", report.Rewritten)
	}
	if report.Unparseable != 6 {
		t.Errorf("Unparseable = %d, want 6 (odd x2, odd-fraction, odd-suffix, odd-mono x2)", report.Unparseable)
	}

	again, err := sqlite.NormalizeDatetimes(ctx, db)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if again.Rewritten != 0 {
		t.Errorf("second run rewrote %d value(s), want 0", again.Rewritten)
	}
}

// Columns are discovered from the live schema, so a table this code has never
// heard of is covered too, across more rows than one batch.
func TestNormalizeDatetimes_DiscoversTablesAndBatches(t *testing.T) {
	db := dbtest.NewSQLite(t).Raw
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE zz_extra (id INTEGER PRIMARY KEY, at DATETIME, note TEXT)`); err != nil {
		t.Fatal(err)
	}
	const rows = 1234
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < rows; i++ {
		// note is TEXT, not a datetime column: it must be left alone.
		v := fmt.Sprintf("2026-09-14 10:%02d:%02d +0000 UTC", i/60%60, i%60)
		if _, err := tx.ExecContext(ctx, `INSERT INTO zz_extra (at, note) VALUES (CAST(? AS TEXT), ?)`, v, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	report, err := sqlite.NormalizeDatetimes(ctx, db)
	if err != nil {
		t.Fatalf("NormalizeDatetimes: %v", err)
	}
	if report.Rewritten != rows {
		t.Errorf("Rewritten = %d, want %d", report.Rewritten, rows)
	}
	var left, notes int
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM zz_extra WHERE length(CAST(at AS TEXT)) <> 19),
		(SELECT COUNT(*) FROM zz_extra WHERE note LIKE '% UTC')`).Scan(&left, &notes); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d zz_extra.at value(s) not rewritten", left)
	}
	if notes != rows {
		t.Errorf("TEXT column touched: %d of %d notes keep their original form", notes, rows)
	}
}

// A WITHOUT ROWID table has no rowid to page by; its values are rewritten by
// value instead, both the UTC fast path and the zoned per-value path.
func TestNormalizeDatetimes_WithoutRowidTable(t *testing.T) {
	db := dbtest.NewSQLite(t).Raw
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE zz_norowid (id TEXT PRIMARY KEY, at DATETIME, seen_at TIMESTAMP) WITHOUT ROWID`); err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct{ id, at, seen string }{
		{"a", "2026-09-14 10:00:00.123456789 +0000 UTC", "2026-09-14 13:00:00 +0300 MSK"},
		{"b", "2026-09-14 10:00:00.123456789 +0000 UTC", "2026-09-14 13:00:00 +0300 MSK"},
		{"c", "2026-09-15 08:00:00", "2026-08-01T00:00:00Z"},
	} {
		if _, err := db.ExecContext(ctx, `INSERT INTO zz_norowid VALUES (?, CAST(? AS TEXT), CAST(? AS TEXT))`, r.id, r.at, r.seen); err != nil {
			t.Fatal(err)
		}
	}

	report, err := sqlite.NormalizeDatetimes(ctx, db)
	if err != nil {
		t.Fatalf("NormalizeDatetimes: %v", err)
	}
	if report.Rewritten != 4 || report.Unparseable != 1 {
		t.Errorf("report = %+v, want Rewritten 4, Unparseable 1", report)
	}
	for _, c := range []struct{ id, col, want string }{
		{"a", "at", "2026-09-14 10:00:00"},
		{"b", "at", "2026-09-14 10:00:00"},
		{"a", "seen_at", "2026-09-14 10:00:00"},
		{"b", "seen_at", "2026-09-14 10:00:00"},
		{"c", "at", "2026-09-15 08:00:00"},
		{"c", "seen_at", "2026-08-01T00:00:00Z"},
	} {
		if got := text(t, db, `SELECT CAST(`+c.col+` AS TEXT) FROM zz_norowid WHERE id = ?`, c.id); got.String != c.want {
			t.Errorf("%s.%s = %q, want %q", c.id, c.col, got.String, c.want)
		}
	}
}

// A declared column named rowid shadows the implicit one, so the table cannot
// be paged by rowid; it is rewritten by value like a WITHOUT ROWID table.
func TestNormalizeDatetimes_DeclaredRowidColumn(t *testing.T) {
	db := dbtest.NewSQLite(t).Raw
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE zz_shadow (rowid TEXT PRIMARY KEY, at DATETIME)`); err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct{ id, at string }{
		{"x", "2026-09-14 10:00:00.5 +0000 UTC"},
		{"y", "2026-09-14 13:00:00 +0300 MSK"},
	} {
		if _, err := db.ExecContext(ctx, `INSERT INTO zz_shadow VALUES (?, CAST(? AS TEXT))`, r.id, r.at); err != nil {
			t.Fatal(err)
		}
	}
	report, err := sqlite.NormalizeDatetimes(ctx, db)
	if err != nil {
		t.Fatalf("NormalizeDatetimes: %v", err)
	}
	if report.Rewritten != 2 || report.Unparseable != 0 {
		t.Errorf("report = %+v, want Rewritten 2, Unparseable 0", report)
	}
	for _, id := range []string{"x", "y"} {
		if got := text(t, db, `SELECT CAST(at AS TEXT) FROM zz_shadow WHERE rowid = ?`, id); got.String != "2026-09-14 10:00:00" {
			t.Errorf("%s.at = %q, want %q", id, got.String, "2026-09-14 10:00:00")
		}
	}
}
