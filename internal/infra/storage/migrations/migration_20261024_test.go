package migrations_test

import (
	"context"
	"database/sql"
	"testing"
)

const (
	skU1  = "01a10000-0000-7000-8000-000000000001"
	skA   = "01a10000-0000-7000-8000-0000000000a1"
	skB   = "01a10000-0000-7000-8000-0000000000a2"
	skR1  = "01a10000-0000-7000-8000-0000000000d1"
	skR2  = "01a10000-0000-7000-8000-0000000000d2"
	skR3  = "01a10000-0000-7000-8000-0000000000d3"
	skT   = "01a10000-0000-7000-8000-0000000000d4"
	skOld = "01a10000-0000-7000-8000-0000000000d5"
	skE1  = "01a10000-0000-7000-8000-0000000000e1"
	skE2  = "01a10000-0000-7000-8000-0000000000e2"
	skIn  = "01a10000-0000-7000-8000-0000000000e3"
)

// 20261024000000 adds the per-side sort keys and numbers every account's days
// in the order the list showed before (spent_at DESC, id), so an upgrade
// changes nothing visible.
func TestMigration20261024_SortKeyBackfillKeepsOrder(t *testing.T) {
	db := runUpTo(t, "sort_key", "20261024000000")
	checkSortKeyMigration(t, db, func(q string) string { return q }, func() { runAll(t, db) })
}

func checkSortKeyMigration(t *testing.T, db *sql.DB, rebind func(string) string, finish func()) {
	t.Helper()
	ctx := context.Background()
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, rebind(q), args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	const ts = "'2026-01-01 00:00:00'"
	mustExec(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
		VALUES (?, ?, 'u1@x.test', 'U', 'face:sky', 'x', 's', 'argon2id', `+ts+`, `+ts+`, TRUE)`, skU1, skU1)
	for _, a := range []string{skA, skB} {
		mustExec(`INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at) VALUES (?, ?, ?, 'A', 2, 'wallet', FALSE, `+ts+`, `+ts+`)`, a, usdSeed, skU1)
	}
	tx := func(id, account string, typ int, spentAt string, recipient any) {
		mustExec(`INSERT INTO transactions (id, user_id, account_id, account_recipient_id, description, type, amount, spent_at, created_at, updated_at) VALUES (?, ?, ?, ?, '', ?, 5, ?, `+ts+`, `+ts+`)`, id, skU1, account, recipient, typ, spentAt)
	}
	tx(skR1, skA, 0, "2026-10-01 10:00:00", nil)
	tx(skR2, skA, 0, "2026-10-01 10:00:00", nil)
	tx(skR3, skA, 0, "2026-10-01 09:00:00", nil)
	tx(skT, skA, 2, "2026-10-01 08:00:00", skB)
	tx(skOld, skA, 0, "2026-09-30 23:00:00", nil)
	// a's own rows and a transfer INTO a share one day: both sides number together
	tx(skE1, skA, 0, "2026-10-02 10:00:00", nil)
	tx(skIn, skB, 2, "2026-10-02 12:00:00", skA)
	tx(skE2, skA, 0, "2026-10-02 14:00:00", nil)

	finish()

	keys := func(id string) (sk int64, rk sql.NullInt64) {
		t.Helper()
		if err := db.QueryRowContext(ctx, rebind(`SELECT sort_key, recipient_sort_key FROM transactions WHERE id = ?`), id).Scan(&sk, &rk); err != nil {
			t.Fatal(err)
		}
		return
	}
	for id, want := range map[string]int64{skR1: 4, skR2: 3, skR3: 2, skT: 1, skOld: 1} {
		sk, rk := keys(id)
		if sk != want {
			t.Errorf("%s: sort_key = %d, want %d", id, sk, want)
		}
		if id != skT && rk.Valid {
			t.Errorf("%s: a non-transfer carries recipient_sort_key %d", id, rk.Int64)
		}
	}
	if k, _ := keys(skE1); k != 1 {
		t.Errorf("e1: sort_key = %d, want 1", k)
	}
	if k, _ := keys(skE2); k != 3 {
		t.Errorf("e2: sort_key = %d, want 3", k)
	}
	if k, rk := keys(skIn); !rk.Valid || rk.Int64 != 2 || k != 1 {
		t.Errorf("incoming transfer: sort_key %d recipient_sort_key %v, want 1 on b and 2 between a's rows", k, rk)
	}
	if _, rk := keys(skT); !rk.Valid || rk.Int64 != 1 {
		t.Errorf("transfer recipient_sort_key = %v, want 1", rk)
	}
	for _, a := range []string{skA, skB} {
		var rev int
		if err := db.QueryRowContext(ctx, rebind(`SELECT transactions_revision FROM accounts WHERE id = ?`), a).Scan(&rev); err != nil {
			t.Fatal(err)
		}
		if rev != 1 {
			t.Errorf("account %s revision = %d, want 1", a, rev)
		}
	}
}
