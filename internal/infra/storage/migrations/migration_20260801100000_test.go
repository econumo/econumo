package migrations_test

// Verifies the 20260801100000 migration: the profile currency option value
// becomes a currency ID. Codes resolve own-custom-first then global; values
// that already ARE ids stay untouched; unresolvable codes lose the row (the
// absent-option USD fallback preserves behavior).

import (
	"context"
	"database/sql"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
)

func TestMigration20260801100000_CurrencyOptionToID(t *testing.T) {
	db := openFK(t)
	ctx := context.Background()
	all := toRunList(migrations.SQLite())
	var before []migrate.Migration
	for _, m := range all {
		if m.Version < "20260801100000" {
			before = append(before, m)
		}
	}
	if err := migrate.Run(ctx, db, before); err != nil {
		t.Fatalf("pre-migrations: %v", err)
	}
	seedUser := func(id string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, `INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
			 VALUES (?, ?, ?, 'U', 'face:sky', 'x', 's', 'argon2id', '2026-01-01 00:00:00', '2026-01-01 00:00:00', 1)`, id, id, id+"@x.test"); err != nil {
			t.Fatal(err)
		}
	}
	seedOption := func(id, userID, value string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, `INSERT INTO users_options (id, user_id, name, value, created_at, updated_at)
			 VALUES (?, ?, 'currency', ?, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`, id, userID, value); err != nil {
			t.Fatal(err)
		}
	}
	for _, u := range []string{"u1", "u2", "u3", "u4"} {
		seedUser(u)
	}
	// u1 owns a custom PTS AND a global PTS exists: own-first must win.
	if _, err := db.ExecContext(ctx, `INSERT INTO currencies (id, code, symbol, created_at, user_id) VALUES ('p1', 'PTS', 'p', '2026-01-01 00:00:00', 'u1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO currencies (id, code, symbol, created_at) VALUES ('pg', 'PTS', 'p', '2026-01-01 00:00:00')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO currencies (id, code, symbol, created_at) VALUES ('e1', 'EUR', 'E', '2026-01-01 00:00:00')`); err != nil {
		t.Fatal(err)
	}
	seedOption("o1", "u1", "PTS") // own-first -> p1
	seedOption("o2", "u2", "EUR") // global -> e1
	seedOption("o3", "u3", "ZZZ") // unresolvable -> row deleted
	seedOption("o4", "u4", "e1")  // already an id -> untouched

	if err := migrate.Run(ctx, db, all); err != nil {
		t.Fatalf("target migration: %v", err)
	}

	get := func(userID string) (string, bool) {
		t.Helper()
		var v sql.NullString
		err := db.QueryRowContext(ctx, `SELECT value FROM users_options WHERE user_id = ? AND name = 'currency'`, userID).Scan(&v)
		if err == sql.ErrNoRows {
			return "", false
		}
		if err != nil {
			t.Fatal(err)
		}
		return v.String, true
	}
	if v, ok := get("u1"); !ok || v != "p1" {
		t.Fatalf("u1 value = %q ok=%v, want own custom p1 (own-first)", v, ok)
	}
	if v, ok := get("u2"); !ok || v != "e1" {
		t.Fatalf("u2 value = %q ok=%v, want global e1", v, ok)
	}
	if _, ok := get("u3"); ok {
		t.Fatal("u3 unresolvable code must lose the option row")
	}
	if v, ok := get("u4"); !ok || v != "e1" {
		t.Fatalf("u4 value = %q ok=%v, want already-id e1 untouched", v, ok)
	}
}
