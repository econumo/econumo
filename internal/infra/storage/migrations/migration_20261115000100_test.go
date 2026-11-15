package migrations_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/migrations"
)

const debtsVersion = "20261115000100"

func debtsSQL(t *testing.T) string {
	t.Helper()
	for _, f := range migrations.SQLite() {
		if f.Version == debtsVersion {
			return f.SQL
		}
	}
	t.Fatalf("migration %s not found", debtsVersion)
	return ""
}

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

// An unreleased build created the same tables under versions whose files are
// gone: they are recorded, the tables exist with rows, and this migration keeps them.
func TestMigration20261115000100KeepsTablesAnEarlierBuildCreated(t *testing.T) {
	ctx := context.Background()
	db := runUpTo(t, "debts_beta", debtsVersion)
	if _, err := db.ExecContext(ctx, debtsSQL(t)); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"20261110000500", "20261112000000", "20261113000000", "20261113000100", "20261114000000"} {
		if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, '2026-11-14 00:00:00')`, v); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO debts (id, budget_id, element_external_id, element_type, currency_id, payment, schedule, created_day, created_at, updated_at)
		VALUES ('d1', 'b1', 'c1', 'category', 'cur', 100, 'monthly', '2026-11-14', '2026-11-14 00:00:00', '2026-11-14 00:00:00')`); err != nil {
		t.Fatal(err)
	}
	runAll(t, db)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM debts WHERE id = 'd1'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("debt kept: count %d, err %v", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, debtsVersion).Scan(&n); err != nil || n != 1 {
		t.Fatalf("version recorded: count %d, err %v", n, err)
	}
}
