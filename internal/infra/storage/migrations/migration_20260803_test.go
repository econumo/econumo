package migrations_test

// Verifies the 20260803000000 migration: every ordered table gains a sort_key
// whose byte order reproduces the previous (position, id) order within its
// scope, and whose backfilled values are well-formed keys.

import (
	"context"
	"database/sql"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	"github.com/econumo/econumo/internal/shared/sortkey"
	_ "modernc.org/sqlite"
)

const sortKeyVersion = "20260803000000"

// runUpTo applies every migration strictly before version and returns the db.
func runUpTo(t *testing.T, name, version string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+name+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	var before []migrate.Migration
	for _, f := range migrations.SQLite() {
		if f.Version < version {
			before = append(before, migrate.Migration{Version: f.Version, SQL: f.SQL, Command: f.Command})
		}
	}
	if err := migrate.Run(context.Background(), db, before, migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatalf("pre-migrations: %v", err)
	}
	return db
}

// runAll replays the full set; migrate.Run is idempotent, so only the
// outstanding migrations actually apply.
func runAll(t *testing.T, db *sql.DB) {
	t.Helper()
	var all []migrate.Migration
	for _, f := range migrations.SQLite() {
		all = append(all, migrate.Migration{Version: f.Version, SQL: f.SQL, Command: f.Command})
	}
	if err := migrate.Run(context.Background(), db, all, migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatalf("target migration: %v", err)
	}
}

func idsInKeyOrder(t *testing.T, db *sql.DB, query string, args ...any) []string {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func assertSame(t *testing.T, got, want []string, what string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %v, want %v", what, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s: got %v, want %v", what, got, want)
		}
	}
}

// TestMigration20260803_BackfillPreservesOrderPerScope seeds deliberately sparse
// AND duplicated positions, which the old schema allowed (there was never a
// unique constraint on (owner, position)), and asserts the key order reproduces
// the (position, id) order the read paths used.
func TestMigration20260803_BackfillPreservesOrderPerScope(t *testing.T) {
	db := runUpTo(t, "sortkey_order", sortKeyVersion)
	ctx := context.Background()
	seedUser(t, db, "u1")
	seedUser(t, db, "u2")

	rows := []struct {
		id, user string
		pos      int
	}{
		{"c1", "u1", 5}, {"c2", "u1", 0}, {"c3", "u1", 5}, {"c4", "u2", 3},
	}
	for _, r := range rows {
		if _, err := db.ExecContext(ctx, `INSERT INTO categories (id, user_id, name, position, type, icon, is_archived, created_at, updated_at)
			VALUES (?, ?, 'n', ?, 0, 'i', 0, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`, r.id, r.user, r.pos); err != nil {
			t.Fatalf("seed %s: %v", r.id, err)
		}
	}

	runAll(t, db)

	got := idsInKeyOrder(t, db, `SELECT id FROM categories WHERE user_id = 'u1' ORDER BY sort_key, id`)
	assertSame(t, got, []string{"c2", "c1", "c3"}, "u1 category order")

	// Scopes are independent: u2's single row gets the same first key as u1's.
	var k1, k2 string
	if err := db.QueryRowContext(ctx, `SELECT sort_key FROM categories WHERE id = 'c2'`).Scan(&k1); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT sort_key FROM categories WHERE id = 'c4'`).Scan(&k2); err != nil {
		t.Fatal(err)
	}
	if k1 != k2 {
		t.Errorf("per-scope rank should restart: u1 first = %q, u2 first = %q", k1, k2)
	}
}

// TestMigration20260803_BackfilledKeysAreWellFormed runs the SQL-generated keys
// through the Go validator, so a divergence between the two encodings is caught
// here rather than at the first move.
func TestMigration20260803_BackfilledKeysAreWellFormed(t *testing.T) {
	db := runUpTo(t, "sortkey_wellformed", sortKeyVersion)
	ctx := context.Background()
	seedUser(t, db, "u1")
	for i, id := range []string{"c1", "c2", "c3"} {
		if _, err := db.ExecContext(ctx, `INSERT INTO categories (id, user_id, name, position, type, icon, is_archived, created_at, updated_at)
			VALUES (?, 'u1', 'n', ?, 0, 'i', 0, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`, id, i); err != nil {
			t.Fatal(err)
		}
	}
	runAll(t, db)

	rows, err := db.QueryContext(ctx, `SELECT sort_key FROM categories ORDER BY sort_key`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatal(err)
		}
		if err := sortkey.Validate(sortkey.Key(k)); err != nil {
			t.Errorf("backfilled key %q is not well-formed: %v", k, err)
		}
		n++
	}
	if n != 3 {
		t.Fatalf("scanned %d keys, want 3", n)
	}
}

// TestMigration20260803_BudgetElementsScopeByFolder pins the one two-column
// partition: elements are ordered per (budget, folder), and NULL folders group
// together.
func TestMigration20260803_BudgetElementsScopeByFolder(t *testing.T) {
	db := runUpTo(t, "sortkey_elements", sortKeyVersion)
	ctx := context.Background()
	seedUser(t, db, "u1")
	if _, err := db.ExecContext(ctx, `INSERT INTO budgets (id, currency_id, user_id, name, started_at, created_at, updated_at)
		VALUES ('b1', ?, 'u1', 'B', '2026-01-01 00:00:00', '2026-01-01 00:00:00', '2026-01-01 00:00:00')`, usdSeed); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO budgets_folders (id, budget_id, name, position, created_at, updated_at)
		VALUES ('f1', 'b1', 'F', 0, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`); err != nil {
		t.Fatal(err)
	}
	// Two elements in folder f1, two with no folder; positions overlap across
	// the two groups, which is exactly what per-folder scoping must handle.
	elems := []struct {
		id, folder string
		pos        int
	}{
		{"e1", "f1", 1}, {"e2", "f1", 0}, {"e3", "", 1}, {"e4", "", 0},
	}
	for _, e := range elems {
		var folder any
		if e.folder != "" {
			folder = e.folder
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO budgets_elements (id, budget_id, folder_id, external_id, type, position, created_at, updated_at)
			VALUES (?, 'b1', ?, ?, 1, ?, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`, e.id, folder, e.id, e.pos); err != nil {
			t.Fatalf("seed %s: %v", e.id, err)
		}
	}

	runAll(t, db)

	inFolder := idsInKeyOrder(t, db, `SELECT id FROM budgets_elements WHERE folder_id = 'f1' ORDER BY sort_key, id`)
	assertSame(t, inFolder, []string{"e2", "e1"}, "folder f1 element order")
	noFolder := idsInKeyOrder(t, db, `SELECT id FROM budgets_elements WHERE folder_id IS NULL ORDER BY sort_key, id`)
	assertSame(t, noFolder, []string{"e4", "e3"}, "no-folder element order")

	// The two groups are independent scopes, so both start at the same key.
	var a, b string
	if err := db.QueryRowContext(ctx, `SELECT sort_key FROM budgets_elements WHERE id = 'e2'`).Scan(&a); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT sort_key FROM budgets_elements WHERE id = 'e4'`).Scan(&b); err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("per-folder rank should restart: folder-first = %q, nofolder-first = %q", a, b)
	}
}

// TestMigration20260803_EveryOrderedTableHasTheColumn guards against a table
// being forgotten in the seven-table sweep.
func TestMigration20260803_EveryOrderedTableHasTheColumn(t *testing.T) {
	db := runUpTo(t, "sortkey_columns", sortKeyVersion)
	runAll(t, db)
	for _, table := range []string{"categories", "tags", "payees", "folders", "accounts_options", "budgets_folders", "budgets_elements"} {
		var n int
		q := `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = 'sort_key'`
		if err := db.QueryRowContext(context.Background(), q, table).Scan(&n); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if n != 1 {
			t.Errorf("table %s is missing the sort_key column", table)
		}
	}
}

// TestMigration20260803_SynthesizesMissingAccountOptions covers the legacy gap
// the backfill alone cannot reach: an (account, user) pair with NO
// accounts_options row at all. The old read path tolerated it as "position 0",
// so such rows exist in the wild -- and left keyless they all compare equal,
// pin to the front of the list, and no anchor can place an account between
// them. The migration must synthesize rows for the account's owner and for
// accepted sharees, with keys sorting BEFORE every backfilled 'c...' key
// (preserving the old "position 0 -> first" placement), skipping pending
// grants and deleted accounts.
func TestMigration20260803_SynthesizesMissingAccountOptions(t *testing.T) {
	db := runUpTo(t, "sortkey_synth", sortKeyVersion)
	ctx := context.Background()
	seedUser(t, db, "u1")
	seedUser(t, db, "u2")

	seedAccount := func(id, owner string, deleted int) {
		t.Helper()
		if _, err := db.ExecContext(ctx, `INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at)
			VALUES (?, ?, ?, 'A', 2, 'wallet', ?, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`, id, usdSeed, owner, deleted); err != nil {
			t.Fatalf("seed account %s: %v", id, err)
		}
	}
	seedAccount("acc1", "u1", 0) // owner HAS an options row
	seedAccount("acc2", "u1", 0) // owner has NO row -> synthesized
	seedAccount("acc3", "u2", 0) // owner has NO row; shared to u1 accepted -> two rows synthesized
	seedAccount("acc4", "u1", 1) // deleted -> nothing synthesized

	if _, err := db.ExecContext(ctx, `INSERT INTO accounts_options (account_id, user_id, position, created_at, updated_at)
		VALUES ('acc1', 'u1', 2, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`); err != nil {
		t.Fatal(err)
	}
	// acc3 shared to u1, ACCEPTED; acc1 shared to u2, PENDING (no row expected).
	if _, err := db.ExecContext(ctx, `INSERT INTO accounts_access (account_id, user_id, role, is_accepted, created_at, updated_at)
		VALUES ('acc3', 'u1', 1, 1, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO accounts_access (account_id, user_id, role, is_accepted, created_at, updated_at)
		VALUES ('acc1', 'u2', 1, 0, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`); err != nil {
		t.Fatal(err)
	}

	runAll(t, db)

	keyOf := func(account, user string) (string, bool) {
		t.Helper()
		var k string
		err := db.QueryRowContext(ctx, `SELECT sort_key FROM accounts_options WHERE account_id = ? AND user_id = ?`, account, user).Scan(&k)
		if err == sql.ErrNoRows {
			return "", false
		}
		if err != nil {
			t.Fatalf("read (%s,%s): %v", account, user, err)
		}
		return k, true
	}

	backfilled, ok := keyOf("acc1", "u1")
	if !ok || backfilled == "" {
		t.Fatalf("acc1/u1 backfilled key missing: %q ok=%v", backfilled, ok)
	}
	for _, pair := range []struct{ account, user string }{
		{"acc2", "u1"}, {"acc3", "u2"}, {"acc3", "u1"},
	} {
		k, ok := keyOf(pair.account, pair.user)
		if !ok {
			t.Fatalf("(%s,%s): no synthesized options row", pair.account, pair.user)
		}
		if err := sortkey.Validate(sortkey.Key(k)); err != nil {
			t.Errorf("(%s,%s) key %q not well-formed: %v", pair.account, pair.user, k, err)
		}
		if pair.user == "u1" && k >= backfilled {
			t.Errorf("(%s,%s) key %q must sort before the backfilled %q (old position-0 rows displayed first)", pair.account, pair.user, k, backfilled)
		}
	}
	if _, ok := keyOf("acc1", "u2"); ok {
		t.Error("pending grant must not get a synthesized row")
	}
	if _, ok := keyOf("acc4", "u1"); ok {
		t.Error("deleted account must not get a synthesized row")
	}
	// The two synthesized u1 rows must be distinct and ordered by account id.
	k2, _ := keyOf("acc2", "u1")
	k3, _ := keyOf("acc3", "u1")
	if k2 >= k3 {
		t.Errorf("synthesized keys not in account-id order: acc2=%q acc3=%q", k2, k3)
	}
}
