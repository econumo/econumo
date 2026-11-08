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
	cases := []struct {
		name, user, option, account string
		start, want                 string
		accountDeleted              string
	}{
		{"with_account", "00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000011", "00000000-0000-4000-8000-000000000021", "started", "completed", "FALSE"},
		{"no_account", "00000000-0000-4000-8000-000000000002", "00000000-0000-4000-8000-000000000012", "", "started", "started", ""},
		{"done_with_account", "00000000-0000-4000-8000-000000000003", "00000000-0000-4000-8000-000000000013", "00000000-0000-4000-8000-000000000023", "completed", "completed", "FALSE"},
		{"deleted_account", "00000000-0000-4000-8000-000000000004", "00000000-0000-4000-8000-000000000014", "00000000-0000-4000-8000-000000000024", "started", "completed", "TRUE"},
	}
	for _, c := range cases {
		mustExec(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
			VALUES (?, ?, ?, 'U', 'face:sky', 'x', 's', 'argon2id', `+ts+`, `+ts+`, TRUE)`, c.user, c.user, c.name+"@x.test")
		mustExec(`INSERT INTO users_options (id, user_id, name, value, created_at, updated_at) VALUES (?, ?, 'onboarding', ?, `+ts+`, `+ts+`)`, c.option, c.user, c.start)
		if c.account != "" {
			mustExec(`INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at)
				VALUES (?, ?, ?, 'A', 2, 'wallet', `+c.accountDeleted+`, `+ts+`, `+ts+`)`, c.account, usdSeed, c.user)
		}
	}

	migrate()

	for _, c := range cases {
		var got string
		if err := db.QueryRowContext(ctx, rebind(`SELECT value FROM users_options WHERE user_id = ? AND name = 'onboarding'`), c.user).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("%s: onboarding = %q, want %q", c.name, got, c.want)
		}
	}
}
