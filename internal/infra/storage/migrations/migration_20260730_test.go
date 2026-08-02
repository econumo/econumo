package migrations_test

// Verifies the consolidated 20260730000000 migration: the single-table
// currencies rebuild (under the runner's hoisted PRAGMA foreign_keys) must
// preserve every referencing row; the partial unique indexes must enforce
// per-owner code uniqueness; the profile currency option must come out as a
// live currency id for EVERY user (the no-fallback invariant).

import (
	"context"
	"database/sql"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	_ "modernc.org/sqlite"
)

const newVersion = "20260730000000"
const usdSeed = "dffc2a06-6f29-4704-8575-31709adee926"

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

func seedUser(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), `INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
		 VALUES (?, ?, ?, 'U', 'face:sky', 'x', 's', 'argon2id', '2026-01-01 00:00:00', '2026-01-01 00:00:00', 1)`, id, id, id+"@x.test"); err != nil {
		t.Fatal(err)
	}
}

func seedOption(t *testing.T, db *sql.DB, id, userID, value string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), `INSERT INTO users_options (id, user_id, name, value, created_at, updated_at)
		 VALUES (?, ?, 'currency', ?, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`, id, userID, value); err != nil {
		t.Fatal(err)
	}
}

func TestMigration20260730_RebuildPreservesDataAndNormalizesOptions(t *testing.T) {
	db := openFK(t)
	ctx := context.Background()
	all := toRunList(migrations.SQLite())
	var before []migrate.Migration
	for _, m := range all {
		if m.Version < newVersion {
			before = append(before, m)
		}
	}
	if err := migrate.Run(ctx, db, before); err != nil {
		t.Fatalf("pre-migrations: %v", err)
	}

	for _, u := range []string{"u1", "u2", "u3"} {
		seedUser(t, db, u)
	}
	// Referencing rows across the FK web of currencies.
	seed := []string{
		`INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at)
		 VALUES ('a1', '` + usdSeed + `', 'u1', 'Cash', 2, 'wallet', 0, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`,
		`INSERT INTO transactions (id, user_id, account_id, description, amount, type, spent_at, created_at, updated_at)
		 VALUES ('t1', 'u1', 'a1', '', '10.00000000', 0, '2026-01-02 00:00:00', '2026-01-02 00:00:00', '2026-01-02 00:00:00')`,
		`INSERT INTO currencies_rates (id, currency_id, base_currency_id, published_at, rate)
		 VALUES ('r1', '` + usdSeed + `', '` + usdSeed + `', '2026-01-01', '1.00000000')`,
		`INSERT INTO budgets (id, currency_id, user_id, name, started_at, created_at, updated_at)
		 VALUES ('b1', '` + usdSeed + `', 'u1', 'Budget', '2026-01-01 00:00:00', '2026-01-01 00:00:00', '2026-01-01 00:00:00')`,
	}
	for _, s := range seed {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("seed: %v\n%s", err, s)
		}
	}
	// Option matrix: resolvable global code / unresolvable code / no row.
	seedOption(t, db, "o1", "u1", "USD") // -> USD id
	seedOption(t, db, "o2", "u2", "ZZZ") // unresolvable -> normalized to USD id
	// u3 has NO currency option -> seeded with the USD id.

	if err := migrate.Run(ctx, db, all); err != nil {
		t.Fatalf("target migration: %v", err)
	}

	for table, want := range map[string]int{"accounts": 1, "transactions": 1, "currencies_rates": 1, "budgets": 1} {
		var n int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil || n != want {
			t.Errorf("%s rows = %d (err %v), want %d (rebuild lost data)", table, n, err, want)
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
	for _, u := range []string{"u1", "u2", "u3"} {
		var v string
		if err := db.QueryRowContext(ctx, `SELECT value FROM users_options WHERE user_id = ? AND name = 'currency'`, u).Scan(&v); err != nil {
			t.Fatalf("%s: currency option missing (invariant broken): %v", u, err)
		}
		if v != usdSeed {
			t.Errorf("%s option value = %q, want the USD id (invariant)", u, v)
		}
	}
}

func TestMigration20260730_OwnFirstResolutionAndAlreadyIDUntouched(t *testing.T) {
	db := openFK(t)
	ctx := context.Background()
	all := toRunList(migrations.SQLite())
	var before []migrate.Migration
	for _, m := range all {
		if m.Version < newVersion {
			before = append(before, m)
		}
	}
	if err := migrate.Run(ctx, db, before); err != nil {
		t.Fatal(err)
	}
	seedUser(t, db, "u1")
	seedUser(t, db, "u2")
	// Pre-rebuild currencies still has UNIQUE(code): a custom PTS cannot
	// coexist with a global PTS yet, so give u1 a custom-coded... it CANNOT
	// exist pre-migration at all (no user_id column). Own-first resolution is
	// therefore only reachable for post-release data, which the migration
	// never sees — assert the global path + already-id here.
	if _, err := db.ExecContext(ctx, `INSERT INTO currencies (id, code, symbol, created_at) VALUES ('e1', 'EUR', 'E', '2026-01-01 00:00:00')`); err != nil {
		t.Fatal(err)
	}
	seedOption(t, db, "o1", "u1", "EUR") // -> e1
	seedOption(t, db, "o2", "u2", "e1")  // already an id -> untouched

	if err := migrate.Run(ctx, db, all); err != nil {
		t.Fatal(err)
	}
	var v string
	if err := db.QueryRowContext(ctx, `SELECT value FROM users_options WHERE user_id = 'u1' AND name = 'currency'`).Scan(&v); err != nil || v != "e1" {
		t.Fatalf("u1 = %q (err %v), want e1", v, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT value FROM users_options WHERE user_id = 'u2' AND name = 'currency'`).Scan(&v); err != nil || v != "e1" {
		t.Fatalf("u2 = %q (err %v), want e1 untouched", v, err)
	}
}

func TestMigration20260730_PartialUniquesAndCascades(t *testing.T) {
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
	seedUser(t, db, "u1")
	seedUser(t, db, "u2")
	mustExec(`INSERT INTO currencies (id, code, symbol, created_at, user_id, rate) VALUES ('p1', 'PTS', 'p', '2026-01-01 00:00:00', 'u1', '10')`)
	mustExec(`INSERT INTO currencies (id, code, symbol, created_at, user_id, rate) VALUES ('p2', 'PTS', 'p', '2026-01-01 00:00:00', 'u2', '5')`)
	mustFail(`INSERT INTO currencies (id, code, symbol, created_at, user_id) VALUES ('p3', 'PTS', 'p', '2026-01-01 00:00:00', 'u2')`, "duplicate (user, code)")
	mustFail(`INSERT INTO currencies (id, code, symbol, created_at) VALUES ('x1', 'USD', '$', '2026-01-01 00:00:00')`, "duplicate global code")
	mustExec(`INSERT INTO users_hidden_currencies (user_id, currency_id, created_at) VALUES ('u1', 'p1', '2026-01-01 00:00:00')`)
	mustFail(`INSERT INTO users_hidden_currencies (user_id, currency_id, created_at) VALUES ('u1', 'p1', '2026-01-01 00:00:00')`, "duplicate hidden PK")
	mustExec(`DELETE FROM users WHERE id = 'u1'`)
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM currencies WHERE user_id = 'u1'").Scan(&n); err != nil || n != 0 {
		t.Fatalf("user-delete cascade to custom currencies: n=%d err=%v", n, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users_hidden_currencies WHERE user_id = 'u1'").Scan(&n); err != nil || n != 0 {
		t.Fatalf("user-delete cascade to hidden rows: n=%d err=%v", n, err)
	}
}
