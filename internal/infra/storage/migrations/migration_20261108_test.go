package migrations_test

import (
	"context"
	"database/sql"
	"testing"
)

func TestMigration20261108CompletesOnboardingForUsersWithAccounts(t *testing.T) {
	db := runUpTo(t, "onboarding_backfill", "20261108000000")
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
	for _, u := range []string{"with_account", "no_account", "done_with_account", "deleted_account"} {
		mustExec(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
			VALUES (?, ?, ?, 'U', 'face:sky', 'x', 's', 'argon2id', `+ts+`, `+ts+`, TRUE)`, u, u, u+"@x.test")
	}
	opts := map[string]string{"with_account": "started", "no_account": "started", "done_with_account": "completed", "deleted_account": "started"}
	for u, v := range opts {
		mustExec(`INSERT INTO users_options (id, user_id, name, value, created_at, updated_at) VALUES (?, ?, 'onboarding', ?, `+ts+`, `+ts+`)`, "opt_"+u, u, v)
	}
	for u, deleted := range map[string]string{"with_account": "FALSE", "done_with_account": "FALSE", "deleted_account": "TRUE"} {
		mustExec(`INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at)
			VALUES (?, ?, ?, 'A', 2, 'wallet', `+deleted+`, `+ts+`, `+ts+`)`, "acc_"+u, usdSeed, u)
	}

	migrate()

	want := map[string]string{"with_account": "completed", "no_account": "started", "done_with_account": "completed", "deleted_account": "completed"}
	for u, w := range want {
		var got string
		if err := db.QueryRowContext(ctx, rebind(`SELECT value FROM users_options WHERE user_id = ? AND name = 'onboarding'`), u).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != w {
			t.Errorf("%s: onboarding = %q, want %q", u, got, w)
		}
	}
}
