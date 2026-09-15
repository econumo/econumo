package migrations_test

// Verifies the 20260915000001 migration: datetime values the driver used to
// store as time.Time.String() ("2026-09-14 10:00:00.123456789 +0000 UTC") are
// rewritten to the frozen 'Y-m-d H:i:s' UTC layout.

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/migrations"
	_ "modernc.org/sqlite"
)

const datetimeFormatVersion = "20260915000001"

func readText(t *testing.T, db *sql.DB, query string, args ...any) sql.NullString {
	t.Helper()
	var s sql.NullString
	if err := db.QueryRowContext(context.Background(), query, args...).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestMigration20260915000001_CanonicalizesDatetimes(t *testing.T) {
	db := runUpTo(t, "datetime_format", datetimeFormatVersion)
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	// CAST to TEXT on insert keeps the driver from touching the raw forms.
	insertUser := func(id, createdAt, updatedAt string, accessUntil any) {
		t.Helper()
		exec(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, access_until, is_active)
			VALUES (?, ?, ?, 'U', 'face:sky', 'x', 's', 'argon2id', CAST(? AS TEXT), CAST(? AS TEXT), ?, 1)`,
			id, id, id+"@x.test", createdAt, updatedAt, accessUntil)
	}
	insertUser("go-utc", "2026-09-14 10:00:00.123456789 +0000 UTC", "2026-09-14 10:00:00 +0000 UTC", nil)
	insertUser("go-offset", "2026-09-14 13:00:00 +0300 MSK", "2026-01-01 00:30:00.5 -0100 -01", "2026-10-01 00:00:00 +0000 UTC")
	insertUser("legacy", "2021-08-12 21:05:48", "2021-08-12 21:05:48", "2026-10-01 00:00:00")
	firstMigration := readText(t, db, `SELECT CAST(applied_at AS TEXT) FROM schema_migrations ORDER BY version LIMIT 1`)

	runAll(t, db)

	for _, c := range []struct{ id, col, want string }{
		{"go-utc", "created_at", "2026-09-14 10:00:00"},
		{"go-utc", "updated_at", "2026-09-14 10:00:00"},
		{"go-offset", "created_at", "2026-09-14 10:00:00"},
		{"go-offset", "updated_at", "2026-01-01 01:30:00"},
		{"go-offset", "access_until", "2026-10-01 00:00:00"},
		{"legacy", "created_at", "2021-08-12 21:05:48"},
		{"legacy", "access_until", "2026-10-01 00:00:00"},
	} {
		got := readText(t, db, `SELECT CAST(`+c.col+` AS TEXT) FROM users WHERE id = ?`, c.id)
		if got.String != c.want {
			t.Errorf("%s.%s = %q, want %q", c.id, c.col, got.String, c.want)
		}
	}
	if got := readText(t, db, `SELECT access_until FROM users WHERE id = 'go-utc'`); got.Valid {
		t.Errorf("NULL access_until became %q", got.String)
	}
	// The instance id hashes the first schema_migrations row; never rewrite it.
	if got := readText(t, db, `SELECT CAST(applied_at AS TEXT) FROM schema_migrations ORDER BY version LIMIT 1`); got != firstMigration {
		t.Errorf("schema_migrations.applied_at changed: %q -> %q", firstMigration.String, got.String)
	}
}

// Every DATETIME column in the schema must be covered, so no table keeps rows
// in the old form. A new table created later is written in the frozen layout
// from the start and needs no entry.
func TestMigration20260915000001_CoversEveryDatetimeColumn(t *testing.T) {
	db := runUpTo(t, "datetime_format_cover", datetimeFormatVersion)

	var sqlText string
	for _, f := range migrations.SQLite() {
		if f.Version == datetimeFormatVersion {
			sqlText = f.SQL
		}
	}
	covered := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^UPDATE (\w+) SET (\w+) =`).FindAllStringSubmatch(sqlText, -1) {
		covered[m[1]+"."+m[2]] = true
	}

	rows, err := db.QueryContext(context.Background(), `SELECT m.name, p.name FROM sqlite_master m, pragma_table_info(m.name) p
		WHERE m.type = 'table' AND upper(p.type) IN ('DATETIME', 'TIMESTAMP')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var missing []string
	for rows.Next() {
		var table, col string
		if err := rows.Scan(&table, &col); err != nil {
			t.Fatal(err)
		}
		if table == "schema_migrations" {
			continue
		}
		if !covered[table+"."+col] {
			missing = append(missing, table+"."+col)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(missing) > 0 {
		t.Errorf("datetime columns not rewritten by %s: %s", datetimeFormatVersion, strings.Join(missing, ", "))
	}
}
