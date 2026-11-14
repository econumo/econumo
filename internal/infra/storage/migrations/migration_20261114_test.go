package migrations_test

import (
	"context"
	"database/sql"
	"testing"
)

const (
	elUser     = "03b00000-0000-7000-8000-000000000001"
	elBudget   = "03b00000-0000-7000-8000-000000000002"
	elCategory = "03b00000-0000-7000-8000-000000000003"
	elAccount  = "03b00000-0000-7000-8000-000000000004"
	elTx       = "03b00000-0000-7000-8000-000000000005"
	elDebt     = "03b00000-0000-7000-8000-000000000006"
	elDebtRow  = "03b00000-0000-7000-8000-000000000007"
	elCatRow   = "03b00000-0000-7000-8000-000000000008"
	elSnapshot = "03b00000-0000-7000-8000-000000000009"
	elRule     = "03b00000-0000-7000-8000-00000000000a"
	elLimitA   = "03b00000-0000-7000-8000-00000000000b"
	elLimitB   = "03b00000-0000-7000-8000-00000000000c"
	elComment  = "03b00000-0000-7000-8000-00000000000d"
	elRate     = "03b00000-0000-7000-8000-00000000000e"
	elDebtTwo  = "03b00000-0000-7000-8000-00000000000f"
)

// 20261114000000 deletes every debt of the rule-based shape, with its
// snapshots, splits, rate changes and budget rows (type 6, their plans and
// comments), drops debts_rules and recreates debts attached to an element;
// other budget rows keep their plans.
func TestMigration20261114_DebtsOnElements(t *testing.T) {
	db := runUpTo(t, "debts_on_elements", "20261114000000")
	checkDebtsOnElements(t, db, func(q string) string { return q }, func() {
		runAll(t, db)
		if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
			t.Fatal(err)
		}
	})
}

// checkDebtsOnElements is shared with the PostgreSQL twin (enginecompare tag).
func checkDebtsOnElements(t *testing.T, db *sql.DB, rebind func(string) string, migrate func()) {
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
		VALUES (?, ?, 'elements@x.test', 'U', 'face:sky', 'x', 's', 'argon2id', `+ts+`, `+ts+`, TRUE)`, elUser, elUser)
	mustExec(`INSERT INTO categories (id, user_id, name, type, icon, is_archived, created_at, updated_at)
		VALUES (?, ?, 'Loan', 0, 'cart', FALSE, `+ts+`, `+ts+`)`, elCategory, elUser)
	mustExec(`INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at)
		VALUES (?, ?, ?, 'A', 2, 'wallet', FALSE, `+ts+`, `+ts+`)`, elAccount, usdSeed, elUser)
	mustExec(`INSERT INTO transactions (id, user_id, account_id, category_id, description, created_at, updated_at, spent_at, type, amount)
		VALUES (?, ?, ?, ?, '', `+ts+`, `+ts+`, `+ts+`, 0, 100)`, elTx, elUser, elAccount, elCategory)
	mustExec(`INSERT INTO budgets (id, currency_id, user_id, name, started_at, created_at, updated_at)
		VALUES (?, ?, ?, 'B', `+ts+`, `+ts+`, `+ts+`)`, elBudget, usdSeed, elUser)
	mustExec(`INSERT INTO debts (id, budget_id, name, currency_id, payment, schedule, created_day, created_at, updated_at)
		VALUES (?, ?, 'Loan', ?, 100, 'monthly', '2026-01-01', `+ts+`, `+ts+`)`, elDebt, elBudget, usdSeed)
	mustExec(`INSERT INTO debts_rules (id, debt_id, category_id, created_at) VALUES (?, ?, ?, `+ts+`)`, elRule, elDebt, elCategory)
	mustExec(`INSERT INTO debts_snapshots (id, debt_id, as_of, balance, payment, schedule, last_payment_date, created_at)
		VALUES (?, ?, '2026-01-01', 1000, 100, 'monthly', '2027-01-01', `+ts+`)`, elSnapshot, elDebt)
	mustExec(`INSERT INTO debts_payments (debt_id, transaction_id, interest) VALUES (?, ?, 1)`, elDebt, elTx)
	mustExec(`INSERT INTO debts_rate_changes (id, debt_id, effective_date, annual_rate, payment, created_at)
		VALUES (?, ?, '2026-02-01', 5, 100, `+ts+`)`, elRate, elDebt)
	for _, el := range []struct {
		id, external, limit string
		typ                 int
	}{{elDebtRow, elDebt, elLimitA, 6}, {elCatRow, elCategory, elLimitB, 1}} {
		mustExec(`INSERT INTO budgets_elements (id, budget_id, currency_id, external_id, type, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, `+ts+`, `+ts+`)`, el.id, elBudget, usdSeed, el.external, el.typ)
		mustExec(`INSERT INTO budgets_elements_limits (id, element_id, period, created_at, updated_at, amount)
			VALUES (?, ?, `+ts+`, `+ts+`, `+ts+`, 50)`, el.limit, el.id)
	}
	mustExec(`INSERT INTO budgets_elements_comments (id, element_id, period, user_id, comment, created_at, updated_at)
		VALUES (?, ?, `+ts+`, ?, 'note', `+ts+`, `+ts+`)`, elComment, elDebtRow, elUser)

	migrate()

	for _, table := range []string{"debts", "debts_snapshots", "debts_payments", "debts_rate_changes"} {
		if n := count(`SELECT COUNT(*) FROM ` + table); n != 0 {
			t.Errorf("%s holds %d rows, want none", table, n)
		}
	}
	if _, err := db.ExecContext(ctx, rebind(`SELECT COUNT(*) FROM debts_rules`)); err == nil {
		t.Error("debts_rules still exists")
	}
	if n := count(`SELECT COUNT(*) FROM budgets_elements WHERE type = 6`); n != 0 {
		t.Errorf("debt rows left: %d", n)
	}
	if n := count(`SELECT COUNT(*) FROM budgets_elements_comments`); n != 0 {
		t.Errorf("debt row comments left: %d", n)
	}
	if n := count(`SELECT COUNT(*) FROM budgets_elements_limits`); n != 1 || count(`SELECT COUNT(*) FROM budgets_elements_limits WHERE element_id = ?`, elCatRow) != 1 {
		t.Errorf("plans left: %d, want the category row's only", n)
	}

	// the new shape: a debt is attached to an element, with a status and an
	// end, and has no name or icon of its own
	if _, err := db.ExecContext(ctx, rebind(`SELECT name FROM debts`)); err == nil {
		t.Error("debts still has name")
	}
	insert := func(id, status string, ended any) error {
		_, err := db.ExecContext(ctx, rebind(`INSERT INTO debts (id, budget_id, element_external_id, element_type, status, ended_at, currency_id,
			payment, schedule, created_day, created_at, updated_at)
			VALUES (?, ?, ?, 'category', ?, ?, ?, 100, 'monthly', '2026-01-01', `+ts+`, `+ts+`)`), id, elBudget, elCategory, status, ended, usdSeed)
		return err
	}
	if err := insert(elDebt, "paid_off", "2026-03-01"); err != nil {
		t.Fatalf("a paid-off debt: %v", err)
	}
	if err := insert(elDebtTwo, "active", nil); err != nil {
		t.Fatalf("an active debt beside a paid-off one: %v", err)
	}
	if err := insert(elSnapshot, "active", nil); err == nil {
		t.Error("a second active debt on the element was stored")
	}
	mustExec(`INSERT INTO debts_payments (debt_id, transaction_id, interest) VALUES (?, ?, 1)`, elDebtTwo, elTx)
	mustExec(`DELETE FROM budgets WHERE id = ?`, elBudget)
	for _, table := range []string{"debts", "debts_payments"} {
		if n := count(`SELECT COUNT(*) FROM ` + table); n != 0 {
			t.Errorf("%s after the budget delete holds %d rows, want none", table, n)
		}
	}
}
