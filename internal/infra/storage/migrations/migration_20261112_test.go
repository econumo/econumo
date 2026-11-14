package migrations_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
)

const (
	dbUser     = "03a00000-0000-7000-8000-000000000001"
	dbBudget   = "03a00000-0000-7000-8000-000000000002"
	dbCategory = "03a00000-0000-7000-8000-000000000003"
	dbAccount  = "03a00000-0000-7000-8000-000000000004"
	dbTx       = "03a00000-0000-7000-8000-000000000005"
	dbDebt     = "03a00000-0000-7000-8000-000000000006"
	dbDebtRow  = "03a00000-0000-7000-8000-000000000007"
	dbCatRow   = "03a00000-0000-7000-8000-000000000008"
	dbDebtNew  = "03a00000-0000-7000-8000-000000000009"
	dbDebtTwo  = "03a00000-0000-7000-8000-00000000000a"
	dbRule     = "03a00000-0000-7000-8000-00000000000b"
	dbLimitA   = "03a00000-0000-7000-8000-00000000000c"
	dbLimitB   = "03a00000-0000-7000-8000-00000000000d"
)

// 20261112000000 deletes every debt of the unreleased user-owned shape, with
// its snapshots, splits and budget rows (type 6, their plans and comments),
// and recreates the tables keyed by budget; other budget rows stay.
func TestMigration20261112_DropsDebtsAndDebtRows(t *testing.T) {
	db := runUpTo(t, "budget_debts", "20261112000000")
	checkBudgetDebtsReset(t, db, func(q string) string { return q }, func() {
		runBefore(t, db, "20261114000000")
		if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
			t.Fatal(err)
		}
	})
}

// runBefore applies the outstanding migrations strictly before version.
func runBefore(t *testing.T, db *sql.DB, version string) {
	t.Helper()
	var before []migrate.Migration
	for _, f := range migrations.SQLite() {
		if f.Version < version {
			before = append(before, migrate.Migration{Version: f.Version, SQL: f.SQL, Command: f.Command})
		}
	}
	if err := migrate.Run(context.Background(), db, before, migrate.WithCommandRunner(migrate.NoCommands)); err != nil {
		t.Fatalf("migrations before %s: %v", version, err)
	}
}

// checkBudgetDebtsReset is shared with the PostgreSQL twin (enginecompare tag).
func checkBudgetDebtsReset(t *testing.T, db *sql.DB, rebind func(string) string, migrate func()) {
	t.Helper()
	ctx := context.Background()
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, rebind(q), args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	count := func(q string, args ...any) int {
		t.Helper()
		var n int
		if err := db.QueryRowContext(ctx, rebind(q), args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return n
	}
	const ts = "'2026-01-01 00:00:00'"
	mustExec(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
		VALUES (?, ?, 'debts@x.test', 'U', 'face:sky', 'x', 's', 'argon2id', `+ts+`, `+ts+`, TRUE)`, dbUser, dbUser)
	mustExec(`INSERT INTO categories (id, user_id, name, type, icon, is_archived, created_at, updated_at)
		VALUES (?, ?, 'Loan', 0, 'cart', FALSE, `+ts+`, `+ts+`)`, dbCategory, dbUser)
	mustExec(`INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at)
		VALUES (?, ?, ?, 'A', 2, 'wallet', FALSE, `+ts+`, `+ts+`)`, dbAccount, usdSeed, dbUser)
	mustExec(`INSERT INTO transactions (id, user_id, account_id, category_id, description, created_at, updated_at, spent_at, type, amount)
		VALUES (?, ?, ?, ?, '', `+ts+`, `+ts+`, `+ts+`, 0, 100)`, dbTx, dbUser, dbAccount, dbCategory)
	mustExec(`INSERT INTO budgets (id, currency_id, user_id, name, started_at, created_at, updated_at)
		VALUES (?, ?, ?, 'B', `+ts+`, `+ts+`, `+ts+`)`, dbBudget, usdSeed, dbUser)
	mustExec(`INSERT INTO debts (id, user_id, name, currency_id, category_id, payment, schedule, created_day, created_at, updated_at)
		VALUES (?, ?, 'Loan', ?, ?, 100, 'monthly', '2026-01-01', `+ts+`, `+ts+`)`, dbDebt, dbUser, usdSeed, dbCategory)
	mustExec(`INSERT INTO debts_snapshots (id, debt_id, as_of, balance, payment, schedule, last_payment_date, created_at)
		VALUES (?, ?, '2026-01-01', 1000, 100, 'monthly', '2027-01-01', `+ts+`)`, dbRule, dbDebt)
	mustExec(`INSERT INTO debts_payments (debt_id, transaction_id, interest) VALUES (?, ?, 1)`, dbDebt, dbTx)
	for _, el := range []struct {
		id, external, limit string
		typ                 int
	}{{dbDebtRow, dbDebt, dbLimitA, 6}, {dbCatRow, dbCategory, dbLimitB, 0}} {
		mustExec(`INSERT INTO budgets_elements (id, budget_id, currency_id, external_id, type, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, `+ts+`, `+ts+`)`, el.id, dbBudget, usdSeed, el.external, el.typ)
		mustExec(`INSERT INTO budgets_elements_limits (id, element_id, period, created_at, updated_at, amount)
			VALUES (?, ?, `+ts+`, `+ts+`, `+ts+`, 50)`, el.limit, el.id)
	}
	mustExec(`INSERT INTO budgets_elements_comments (id, element_id, period, user_id, comment, created_at, updated_at)
		VALUES (?, ?, `+ts+`, ?, 'note', `+ts+`, `+ts+`)`, dbDebtNew, dbDebtRow, dbUser)

	migrate()

	for _, table := range []string{"debts", "debts_snapshots", "debts_payments", "debts_rules"} {
		if n := count(`SELECT COUNT(*) FROM ` + table); n != 0 {
			t.Errorf("%s holds %d rows, want none", table, n)
		}
	}
	if n := count(`SELECT COUNT(*) FROM budgets_elements WHERE type = 6`); n != 0 {
		t.Errorf("debt rows left: %d", n)
	}
	if n := count(`SELECT COUNT(*) FROM budgets_elements_comments`); n != 0 {
		t.Errorf("debt row comments left: %d", n)
	}
	if n := count(`SELECT COUNT(*) FROM budgets_elements_limits WHERE element_id = ?`, dbCatRow); n != 1 {
		t.Errorf("the category row's plan: %d, want 1", n)
	}
	if n := count(`SELECT COUNT(*) FROM budgets_elements_limits`); n != 1 {
		t.Errorf("plans left: %d, want the category row's only", n)
	}

	// the new shape: debts belong to a budget, one transaction may count for
	// two debts (of different budgets)
	if _, err := db.ExecContext(ctx, rebind(`SELECT user_id FROM debts`)); err == nil {
		t.Error("debts still has user_id")
	}
	for _, d := range []string{dbDebtNew, dbDebtTwo} {
		mustExec(`INSERT INTO debts (id, budget_id, name, currency_id, payment, schedule, created_day, created_at, updated_at)
			VALUES (?, ?, 'Loan', ?, 100, 'monthly', '2026-01-01', `+ts+`, `+ts+`)`, d, dbBudget, usdSeed)
		mustExec(`INSERT INTO debts_payments (debt_id, transaction_id, interest) VALUES (?, ?, 1)`, d, dbTx)
	}
	mustExec(`INSERT INTO debts_rules (id, debt_id, category_id, created_at) VALUES (?, ?, ?, `+ts+`)`, dbRule, dbDebtNew, dbCategory)
	if n := count(`SELECT COUNT(*) FROM debts_payments WHERE transaction_id = ?`, dbTx); n != 2 {
		t.Errorf("splits of one transaction = %d, want 2", n)
	}
	mustExec(`DELETE FROM budgets WHERE id = ?`, dbBudget)
	for _, table := range []string{"debts", "debts_payments", "debts_rules"} {
		if n := count(`SELECT COUNT(*) FROM ` + table); n != 0 {
			t.Errorf("%s after the budget delete holds %d rows, want none", table, n)
		}
	}
}
