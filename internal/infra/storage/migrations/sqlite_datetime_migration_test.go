package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/migrations"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// TestSQLiteDatetimeNormalizationMigration_Idempotent pins the 20260712000000
// migration: dbtest.New(t) applies every migration (including this one) to a
// still-empty schema, so the interesting behavior can only be exercised by
// re-running the migration's own UPDATE text against a row seeded AFTER
// setup, in the exact long format modernc/sqlite wrote before the driver.go
// fix. Re-running proves the migration is idempotent by construction (a
// bare-format row no longer matches the '%UTC' LIKE pattern).
func TestSQLiteDatetimeNormalizationMigration_Idempotent(t *testing.T) {
	db := dbtest.New(t)
	if db.Engine != "sqlite" {
		t.Skip("sqlite-only migration")
	}
	f := fixture.New(t, db)
	uid := f.User(fixture.User{Name: "u"})
	ctx := context.Background()

	const long = "2026-06-15 12:00:00 +0000 UTC"
	if _, err := db.Raw.ExecContext(ctx, `UPDATE users SET created_at = ? WHERE id = ?`, long, uid); err != nil {
		t.Fatalf("seed long-format row: %v", err)
	}

	stmt := usersCreatedAtMigrationStatement(t)

	readCreatedAt := func() string {
		t.Helper()
		// CAST ... AS TEXT bypasses modernc's automatic text->time.Time
		// column-type conversion (declared type DATETIME), which would
		// otherwise reformat the read value (e.g. to RFC3339) and mask what
		// is actually stored.
		var got string
		if err := db.Raw.QueryRowContext(ctx, `SELECT CAST(created_at AS TEXT) FROM users WHERE id = ?`, uid).Scan(&got); err != nil {
			t.Fatalf("read back created_at: %v", err)
		}
		return got
	}

	if _, err := db.Raw.ExecContext(ctx, stmt); err != nil {
		t.Fatalf("run migration statement: %v", err)
	}
	const want = "2026-06-15 12:00:00"
	if got := readCreatedAt(); got != want {
		t.Fatalf("created_at = %q, want %q", got, want)
	}

	// Idempotent: a second run against the now-bare row is a no-op.
	if _, err := db.Raw.ExecContext(ctx, stmt); err != nil {
		t.Fatalf("re-run migration statement: %v", err)
	}
	if got := readCreatedAt(); got != want {
		t.Fatalf("created_at after re-run = %q, want %q", got, want)
	}
}

// usersCreatedAtMigrationStatement extracts the users.created_at UPDATE
// statement's exact text from the committed 20260712000000 migration, so the
// test exercises the real migration SQL rather than a hand-copied duplicate.
// Each UPDATE in that file is one self-contained line, so a line scan (unlike
// a naive split on ';') isn't tripped up by the semicolons inside the file's
// leading comment block.
func usersCreatedAtMigrationStatement(t *testing.T) string {
	t.Helper()
	for _, file := range migrations.SQLite() {
		if file.Version != "20260712000000" {
			continue
		}
		for _, raw := range strings.Split(file.SQL, "\n") {
			line := strings.TrimSpace(raw)
			if strings.HasPrefix(line, "UPDATE users SET created_at = ") {
				return strings.TrimSuffix(line, ";")
			}
		}
	}
	t.Fatal("20260712000000: UPDATE users SET created_at = ... statement not found")
	return ""
}
