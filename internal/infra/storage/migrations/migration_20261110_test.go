package migrations_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
)

func TestMigration20261110CompletesOnboardingForUsersWithTransactions(t *testing.T) {
	db := runUpTo(t, "onboarding_backfill", "20261110000000")
	checkOnboardingBackfill(t, db, func(q string) string { return q }, func() { runAll(t, db) })
}

func checkOnboardingBackfill(t *testing.T, db *sql.DB, rebind func(string) string, migrate func()) {
	t.Helper()
	ctx := context.Background()
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, rebind(q), args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	const ts = "'2026-01-01 00:00:00'"
	n := 0
	id := func() string {
		n++
		return fmt.Sprintf("00000000-0000-4000-8000-%012d", n)
	}
	user := func(name, option string) string {
		u := id()
		mustExec(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
			VALUES (?, ?, ?, 'U', 'face:sky', 'x', 's', 'argon2id', `+ts+`, `+ts+`, TRUE)`, u, u, name+"@x.test")
		if option != "" {
			mustExec(`INSERT INTO users_options (id, user_id, name, value, created_at, updated_at) VALUES (?, ?, 'onboarding', ?, `+ts+`, `+ts+`)`, id(), u, option)
		}
		return u
	}
	account := func(owner string, deleted bool) string {
		a := id()
		del := "FALSE"
		if deleted {
			del = "TRUE"
		}
		mustExec(`INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at)
			VALUES (?, ?, ?, 'A', 2, 'wallet', `+del+`, `+ts+`, `+ts+`)`, a, usdSeed, owner)
		return a
	}
	transaction := func(author, acc, recipient string) {
		var rcp any
		if recipient != "" {
			rcp = recipient
		}
		mustExec(`INSERT INTO transactions (id, user_id, account_id, account_recipient_id, description, created_at, updated_at, spent_at, type, amount)
			VALUES (?, ?, ?, ?, '', `+ts+`, `+ts+`, `+ts+`, 0, 1)`, id(), author, acc, rcp)
	}

	// a partner with no onboarding option: owns an account others write on
	partner := user("partner", "")
	shared := account(partner, false)

	want := map[string]string{}
	accountsOnly := user("accounts_only", "started")
	account(accountsOnly, false)
	want[accountsOnly] = "started"

	nothing := user("nothing", "categories")
	want[nothing] = "categories"

	ownTxn := user("own_txn", "started")
	transaction(ownTxn, account(ownTxn, false), "")
	want[ownTxn] = "completed"

	// the partner writes on the user's deleted account: the owner side alone counts
	deletedTxn := user("deleted_txn", "accounts")
	transaction(partner, account(deletedTxn, true), "")
	want[deletedTxn] = "completed"

	// the partner writes a plain transaction on an account the user owns
	sourceSide := user("source_side", "started")
	transaction(partner, account(sourceSide, false), "")
	want[sourceSide] = "completed"

	authored := user("authored_on_partner", "started")
	transaction(authored, shared, "")
	want[authored] = "completed"

	recipient := user("recipient_side", "started")
	transaction(partner, shared, account(recipient, false))
	want[recipient] = "completed"

	done := user("done", "completed")
	want[done] = "completed"

	migrate()

	for u, w := range want {
		var got string
		if err := db.QueryRowContext(ctx, rebind(`SELECT value FROM users_options WHERE user_id = ? AND name = 'onboarding'`), u).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != w {
			t.Errorf("%s: onboarding = %q, want %q", u, got, w)
		}
	}
	var options int
	if err := db.QueryRowContext(ctx, rebind(`SELECT COUNT(*) FROM users_options WHERE user_id = ?`), partner).Scan(&options); err != nil || options != 0 {
		t.Fatalf("the partner got options: %d %v", options, err)
	}
}
