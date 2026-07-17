package migrations_test

// Verifies the 20260715000000 sqlite migration: the currencies FK-closure
// rebuild must preserve data under PRAGMA foreign_keys = ON (a naive rebuild
// cascade-deletes children), and the new partial unique indexes must enforce
// per-owner code uniqueness.

import (
	"context"
	"database/sql"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	_ "modernc.org/sqlite"
)

const newVersion = "20260715000000"

func openFK(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.ExecContext(context.Background(), "PRAGMA foreign_keys = ON;"); err != nil {
		t.Fatal(err)
	}
	return db
}

func toRunList(src []migrations.File) []migrate.Migration {
	out := make([]migrate.Migration, 0, len(src))
	for _, m := range src {
		out = append(out, migrate.Migration{Version: m.Version, SQL: m.SQL})
	}
	return out
}

func TestMigration20260715_DataSurvivesRebuild(t *testing.T) {
	db := openFK(t)
	ctx := context.Background()
	all := toRunList(migrations.SQLite())
	var before, target []migrate.Migration
	for _, m := range all {
		if m.Version == newVersion {
			target = append(target, m)
			continue
		}
		if m.Version < newVersion {
			before = append(before, m)
		}
	}
	if len(target) != 1 {
		t.Fatalf("migration %s not found in embed", newVersion)
	}
	if err := migrate.Run(ctx, db, before); err != nil {
		t.Fatalf("pre-migrations: %v", err)
	}
	seed := []string{
		`INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
		 VALUES ('u1', 'ident1', 'a@b.c', 'A', 'face:sky', 'x', 's', 'argon2id', '2026-01-01 00:00:00', '2026-01-01 00:00:00', 1)`,
		`INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at)
		 VALUES ('a1', 'dffc2a06-6f29-4704-8575-31709adee926', 'u1', 'Cash', 2, 'wallet', 0, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`,
		`INSERT INTO transactions (id, user_id, account_id, description, amount, type, spent_at, created_at, updated_at)
		 VALUES ('t1', 'u1', 'a1', '', '10.00000000', 0, '2026-01-02 00:00:00', '2026-01-02 00:00:00', '2026-01-02 00:00:00')`,
		`INSERT INTO currencies_rates (id, currency_id, base_currency_id, published_at, rate)
		 VALUES ('r1', 'dffc2a06-6f29-4704-8575-31709adee926', 'dffc2a06-6f29-4704-8575-31709adee926', '2026-01-01', '1.00000000')`,
		`INSERT INTO budgets (id, currency_id, user_id, name, started_at, created_at, updated_at)
		 VALUES ('b1', 'dffc2a06-6f29-4704-8575-31709adee926', 'u1', 'Budget', '2026-01-01 00:00:00', '2026-01-01 00:00:00', '2026-01-01 00:00:00')`,
		`INSERT INTO budgets_elements (id, budget_id, currency_id, external_id, type, created_at, updated_at, position)
		 VALUES ('be1', 'b1', 'dffc2a06-6f29-4704-8575-31709adee926', 'x1', 1, '2026-01-01 00:00:00', '2026-01-01 00:00:00', 0)`,
	}
	for _, s := range seed {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("seed: %v\n%s", err, s)
		}
	}
	if err := migrate.Run(ctx, db, all); err != nil {
		t.Fatalf("target migration: %v", err)
	}
	for table, want := range map[string]int{
		"accounts": 1, "transactions": 1, "currencies_rates": 1,
		"budgets": 1, "budgets_elements": 1,
	} {
		var n int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != want {
			t.Errorf("%s rows = %d, want %d (rebuild lost data)", table, n, want)
		}
	}
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign_key_check reported violations after migration")
	}
}

func TestMigration20260715_PartialUniqueAndNewTable(t *testing.T) {
	db := openFK(t)
	ctx := context.Background()
	if err := migrate.Run(ctx, db, toRunList(migrations.SQLite())); err != nil {
		t.Fatal(err)
	}
	mustExec := func(q string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	mustFail := func(q, why string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q); err == nil {
			t.Fatalf("expected failure (%s):\n%s", why, q)
		}
	}
	mustExec(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
	          VALUES ('u1', 'i1', 'a@b.c', 'A', 'face:sky', 'x', 's', 'argon2id', '2026-01-01 00:00:00', '2026-01-01 00:00:00', 1)`)
	mustExec(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
	          VALUES ('u2', 'i2', 'b@b.c', 'B', 'face:sky', 'x', 's', 'argon2id', '2026-01-01 00:00:00', '2026-01-01 00:00:00', 1)`)
	mustExec(`INSERT INTO currencies (id, code, symbol, created_at, user_id) VALUES ('p1', 'PTS', 'pts', '2026-01-01 00:00:00', 'u1')`)
	mustExec(`INSERT INTO currencies (id, code, symbol, created_at, user_id) VALUES ('p2', 'PTS', 'pts', '2026-01-01 00:00:00', 'u2')`)
	mustFail(`INSERT INTO currencies (id, code, symbol, created_at, user_id) VALUES ('p3', 'PTS', 'pts', '2026-01-01 00:00:00', 'u2')`, "duplicate (user, code)")
	mustFail(`INSERT INTO currencies (id, code, symbol, created_at) VALUES ('x1', 'USD', '$', '2026-01-01 00:00:00')`, "duplicate global code")
	mustExec(`INSERT INTO users_hidden_currencies (user_id, currency_id, created_at) VALUES ('u1', 'dffc2a06-6f29-4704-8575-31709adee926', '2026-01-01 00:00:00')`)
	mustFail(`INSERT INTO users_hidden_currencies (user_id, currency_id, created_at) VALUES ('u1', 'dffc2a06-6f29-4704-8575-31709adee926', '2026-01-01 00:00:00')`, "duplicate hidden PK")
	mustExec(`DELETE FROM users WHERE id = 'u1'`)
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM currencies WHERE user_id = 'u1'").Scan(&n); err != nil || n != 0 {
		t.Fatalf("user-delete cascade to custom currencies: n=%d err=%v", n, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users_hidden_currencies WHERE user_id = 'u1'").Scan(&n); err != nil || n != 0 {
		t.Fatalf("user-delete cascade to hidden rows: n=%d err=%v", n, err)
	}
}
