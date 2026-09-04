package migrations_test

// Verifies the 20260817000002 migration: seeds budgets_accounts from the
// implicit population it replaces (owner's accounts plus an accepted
// non-guest participant's accounts, minus the blacklist), applies the
// deleted-after-start-and-after-creation carve-out, and drops
// budgets_excluded_accounts.

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
)

const membershipSchemaVersion = "20260817000000"

func TestMigration20260817_SeedsMembershipAndDropsBlacklist(t *testing.T) {
	db := runUpTo(t, "membership_seed", membershipSchemaVersion)
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	const ts = "'2026-01-01 00:00:00'"
	for _, u := range []string{"owner", "member", "guest"} {
		mustExec(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, created_at, updated_at) VALUES (?, ?, ?, 'U', '', 'x', '', `+ts+`, `+ts+`)`, u, u, u+"@e.test")
	}
	mustExec(`INSERT INTO currencies (id, code, name, symbol, fraction_digits, created_at) VALUES ('cur1', 'EUR', 'Euro', '€', 2, ` + ts + `)`)
	// budget started 2026-03, created 2026-04-01
	mustExec(`INSERT INTO budgets (id, currency_id, user_id, name, started_at, created_at, updated_at) VALUES ('b1', 'cur1', 'owner', 'B', '2026-03-01 00:00:00', '2026-04-01 00:00:00', '2026-04-01 00:00:00')`)
	mustExec(`INSERT INTO budgets_access (budget_id, user_id, role, is_accepted, created_at, updated_at) VALUES ('b1', 'member', 1, 1, ` + ts + `, ` + ts + `)`)
	mustExec(`INSERT INTO budgets_access (budget_id, user_id, role, is_accepted, created_at, updated_at) VALUES ('b1', 'guest', 2, 1, ` + ts + `, ` + ts + `)`)
	acct := func(id, user string, deleted int, updated string) {
		mustExec(`INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at) VALUES (?, 'cur1', ?, 'A', 2, 'wallet', ?, `+ts+`, ?)`, id, user, deleted, updated)
	}
	acct("a-live", "owner", 0, "2026-01-01 00:00:00")
	acct("a-excl", "owner", 0, "2026-01-01 00:00:00")
	acct("a-del-after", "owner", 1, "2026-05-10 00:00:00")   // deleted after start AND after creation → seeded
	acct("a-del-between", "owner", 1, "2026-03-15 00:00:00") // after start, before creation → not seeded
	acct("a-del-before", "owner", 1, "2026-02-01 00:00:00")  // before start → not seeded
	acct("a-member", "member", 0, "2026-01-01 00:00:00")
	acct("a-guest", "guest", 0, "2026-01-01 00:00:00")
	mustExec(`INSERT INTO budgets_excluded_accounts (budget_id, account_id) VALUES ('b1', 'a-excl')`)

	var calls []string
	all := make([]migrate.Migration, 0)
	for _, f := range migrations.SQLite() {
		all = append(all, migrate.Migration{Version: f.Version, SQL: f.SQL, Command: f.Command})
	}
	if err := migrate.Run(context.Background(), db, all, migrate.WithCommandRunner(func(ctx context.Context, name string) error {
		calls = append(calls, name)
		return nil
	})); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Both command steps registered after the membership schema version run:
	// zero-deleted-accounts (20260817000001) and the analytics backfill
	// (20260903000000).
	wantCalls := []string{"migration:zero-deleted-accounts", "migration:seed-analytics-option"}
	if len(calls) != len(wantCalls) || calls[0] != wantCalls[0] || calls[1] != wantCalls[1] {
		t.Fatalf("command steps = %v, want %v", calls, wantCalls)
	}
	got := idsInKeyOrder(t, db, `SELECT account_id FROM budgets_accounts WHERE budget_id = 'b1' ORDER BY account_id`)
	assertSame(t, got, []string{"a-del-after", "a-live", "a-member"}, "seeded members")
	if _, err := db.Exec(`SELECT 1 FROM budgets_excluded_accounts`); err == nil {
		t.Fatal("budgets_excluded_accounts still exists")
	}
}
