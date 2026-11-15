package migrations_test

import (
	"context"
	"testing"
)

const debtsVersion = "20261115000100"

func TestMigration20261115000100CreatesTheDebtTables(t *testing.T) {
	db := runUpTo(t, "debts_fresh", debtsVersion)
	runAll(t, db)
	for _, table := range []string{"debts", "debts_snapshots", "debts_payments", "debts_rate_changes"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&n); err != nil || n != 1 {
			t.Fatalf("table %s: count %d, err %v", table, n, err)
		}
	}
}

// An unreleased build stopped at an earlier debt shape (a beta at
// 20261112000000: debts with rules, debt budget rows of type 6, no rate
// changes table), under versions whose files are gone. Those tables are
// replaced by the current ones and the debt rows go with their plans.
func TestMigration20261115000100ReplacesAnEarlierDebtShape(t *testing.T) {
	ctx := context.Background()
	db := runUpTo(t, "debts_beta", debtsVersion)
	for _, q := range []string{
		`CREATE TABLE debts (id TEXT NOT NULL PRIMARY KEY, budget_id TEXT NOT NULL, name TEXT NOT NULL)`,
		`CREATE TABLE debts_rules (id TEXT NOT NULL PRIMARY KEY, debt_id TEXT NOT NULL)`,
		`CREATE TABLE debts_snapshots (id TEXT NOT NULL PRIMARY KEY, debt_id TEXT NOT NULL)`,
		`CREATE TABLE debts_payments (debt_id TEXT NOT NULL, transaction_id TEXT NOT NULL)`,
		`INSERT INTO debts (id, budget_id, name) VALUES ('d1', 'b1', 'Mortgage')`,
		`INSERT INTO schema_migrations (version, applied_at) VALUES ('20261110000500', '2026-11-12 06:51:18'), ('20261112000000', '2026-11-14 02:55:06')`,
		`INSERT INTO budgets_elements (id, budget_id, external_id, type, created_at, updated_at) VALUES
			('e6', 'b1', 'd1', 6, '2026-11-14 00:00:00', '2026-11-14 00:00:00'),
			('e1', 'b1', 'c1', 1, '2026-11-14 00:00:00', '2026-11-14 00:00:00')`,
		`INSERT INTO budgets_elements_limits (id, element_id, amount, period, created_at, updated_at) VALUES
			('l6', 'e6', 100, '2026-11-01 00:00:00', '2026-11-14 00:00:00', '2026-11-14 00:00:00'),
			('l1', 'e1', 100, '2026-11-01 00:00:00', '2026-11-14 00:00:00', '2026-11-14 00:00:00')`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	runAll(t, db)
	count := func(q string) int {
		t.Helper()
		var n int
		if err := db.QueryRow(q).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return n
	}
	if n := count(`SELECT COUNT(*) FROM pragma_table_info('debts') WHERE name = 'element_external_id'`); n != 1 {
		t.Fatal("debts is not the current shape")
	}
	if n := count(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'debts_rules'`); n != 0 {
		t.Fatal("debts_rules kept")
	}
	if n := count(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'debts_rate_changes'`); n != 1 {
		t.Fatal("debts_rate_changes missing")
	}
	if n := count(`SELECT COUNT(*) FROM debts`); n != 0 {
		t.Fatalf("old debts kept: %d", n)
	}
	if n := count(`SELECT COUNT(*) FROM budgets_elements WHERE id = 'e6'`); n != 0 {
		t.Fatal("debt budget row kept")
	}
	if n := count(`SELECT COUNT(*) FROM budgets_elements_limits`); n != 1 {
		t.Fatalf("limits: %d, want only the category's", n)
	}
	if n := count(`SELECT COUNT(*) FROM schema_migrations WHERE version = '` + debtsVersion + `'`); n != 1 {
		t.Fatal("version not recorded")
	}
}
