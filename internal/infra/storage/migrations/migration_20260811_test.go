package migrations_test

// Verifies the 20260811000000 migration: income-category links silently stored
// on (expense) envelopes are removed; expense links are untouched.

import (
	"testing"
)

const envelopeSideCleanupVersion = "20260811000000"

func TestMigration20260811_RemovesIncomeEnvelopeChildren(t *testing.T) {
	db := runUpTo(t, "envelope_side_cleanup", envelopeSideCleanupVersion)

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustExec(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, created_at, updated_at)
	          VALUES ('u1', 'u1', 'u@e.test', 'U', '', 'x', '', '2026-01-01 00:00:00', '2026-01-01 00:00:00')`)
	mustExec(`INSERT INTO currencies (id, code, name, symbol, fraction_digits, created_at)
	          VALUES ('cur1', 'EUR', 'Euro', '€', 2, '2026-01-01 00:00:00')`)
	mustExec(`INSERT INTO categories (id, user_id, name, type, icon, is_archived, created_at, updated_at)
	          VALUES ('cat-exp', 'u1', 'Food', 0, 'cart', 0, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`)
	mustExec(`INSERT INTO categories (id, user_id, name, type, icon, is_archived, created_at, updated_at)
	          VALUES ('cat-inc', 'u1', 'Salary', 1, 'cash', 0, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`)
	mustExec(`INSERT INTO budgets (id, currency_id, user_id, name, started_at, created_at, updated_at)
	          VALUES ('b1', 'cur1', 'u1', 'B', '2026-01-01 00:00:00', '2026-01-01 00:00:00', '2026-01-01 00:00:00')`)
	mustExec(`INSERT INTO budgets_envelopes (id, budget_id, name, icon, is_archived, created_at, updated_at)
	          VALUES ('env1', 'b1', 'Env', 'cart', 0, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`)
	mustExec(`INSERT INTO budgets_envelopes_categories (budget_envelope_id, category_id) VALUES ('env1', 'cat-exp')`)
	mustExec(`INSERT INTO budgets_envelopes_categories (budget_envelope_id, category_id) VALUES ('env1', 'cat-inc')`)

	runAll(t, db)

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM budgets_envelopes_categories WHERE category_id = 'cat-inc'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("income child links after migration = %d (err=%v), want 0", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM budgets_envelopes_categories WHERE category_id = 'cat-exp'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("expense child links after migration = %d (err=%v), want 1 (untouched)", n, err)
	}
}
