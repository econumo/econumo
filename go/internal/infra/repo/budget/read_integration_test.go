package budgetrepo_test

// Integration tests for the budget ReadRepo report queries against a real
// migrated in-memory SQLite. Regression-locks the month-boundary datetime
// binding (a first-of-month transaction/limit must be INCLUDED) and the
// exact scale-8 decimal sums from float SUM rendering.

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/domain/shared/vo"
	budgetrepo "github.com/econumo/econumo/internal/infra/repo/budget"
	"github.com/econumo/econumo/internal/testutil"
)

func seedAccount(t *testing.T, db *testutil.DB, id, userID string) {
	t.Helper()
	db.Exec(t, `INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at) VALUES (?, ?, ?, 'A', 2, 'x', 0, ?, ?)`,
		id, usdID, userID, fixedTime, fixedTime)
}

func seedCategory(t *testing.T, db *testutil.DB, id, userID string) {
	t.Helper()
	db.Exec(t, `INSERT INTO categories (id, user_id, name, position, type, icon, is_archived, created_at, updated_at) VALUES (?, ?, 'C', 0, 0, 'x', 0, ?, ?)`,
		id, userID, fixedTime, fixedTime)
}

func seedExpense(t *testing.T, db *testutil.DB, id, account, category, amount, spentAt string) {
	t.Helper()
	db.Exec(t, `INSERT INTO transactions (id, user_id, account_id, category_id, type, amount, description, created_at, updated_at, spent_at) VALUES (?, ?, ?, ?, 0, ?, '', ?, ?, ?)`,
		id, userA, account, category, amount, fixedTime, fixedTime, spentAt)
}

func newReadRepo(t *testing.T) (*budgetrepo.ReadRepo, *testutil.DB) {
	t.Helper()
	db := testutil.NewSQLite(t)
	seedUser(t, db, userA)
	seedAccount(t, db, acctA, userA)
	return budgetrepo.NewReadRepo("sqlite", db.TX), db
}

func TestBudgetReadRepo_AccountsBalances(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	cat := "c0000000-0000-0000-0000-0000000000c1"
	seedCategory(t, db, cat, userA)
	// Two incomes minus one expense; float sum must render clean.
	db.Exec(t, `INSERT INTO transactions (id, user_id, account_id, type, amount, description, created_at, updated_at, spent_at) VALUES (?, ?, ?, 1, '100.10', '', ?, ?, '2024-03-10 00:00:00')`,
		"70000000-0000-0000-0000-000000000001", userA, acctA, fixedTime, fixedTime)
	db.Exec(t, `INSERT INTO transactions (id, user_id, account_id, type, amount, description, created_at, updated_at, spent_at) VALUES (?, ?, ?, 1, '200.20', '', ?, ?, '2024-03-11 00:00:00')`,
		"70000000-0000-0000-0000-000000000002", userA, acctA, fixedTime, fixedTime)
	seedExpense(t, db, "70000000-0000-0000-0000-000000000003", acctA, cat, "0.30", "2024-03-12 00:00:00")

	onDate := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	rows, err := read.AccountsBalancesOnDate(ctx, []vo.Id{vo.MustParseId(acctA)}, onDate)
	if err != nil {
		t.Fatalf("AccountsBalancesOnDate: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 balance row, got %d", len(rows))
	}
	// 100.10 + 200.20 - 0.30 = 300.00; the float SUM renders to scale 8.
	if rows[0].Balance != "300.00000000" {
		t.Errorf("balance mismatch: %q", rows[0].Balance)
	}
	if rows[0].CurrencyID != usdID {
		t.Errorf("currency mismatch: %q", rows[0].CurrencyID)
	}

	// Empty id set -> nil.
	none, err := read.AccountsBalancesOnDate(ctx, nil, onDate)
	if err != nil || none != nil {
		t.Errorf("empty ids should be nil,nil; got %v, %v", none, err)
	}
}

func TestBudgetReadRepo_CountSpending_MonthBoundary(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	cat := "c0000000-0000-0000-0000-0000000000c1"
	seedCategory(t, db, cat, userA)
	// Boundary on the first of the month must be included; previous-month excluded.
	seedExpense(t, db, "70000000-0000-0000-0000-000000000010", acctA, cat, "10.00", "2024-04-01 00:00:00")
	seedExpense(t, db, "70000000-0000-0000-0000-000000000011", acctA, cat, "5.50", "2024-04-15 00:00:00")
	seedExpense(t, db, "70000000-0000-0000-0000-000000000012", acctA, cat, "99.00", "2024-03-31 23:59:59")

	start := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	rows, err := read.CountSpending(ctx, []vo.Id{vo.MustParseId(cat)}, []vo.Id{vo.MustParseId(acctA)}, start, end)
	if err != nil {
		t.Fatalf("CountSpending: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 spending row, got %d", len(rows))
	}
	// 10.00 + 5.50 = 15.50 (incl. Apr 1 boundary, excl. Mar 31).
	if rows[0].Amount != "15.50000000" {
		t.Errorf("spending amount mismatch: %q", rows[0].Amount)
	}
}

func TestBudgetReadRepo_SummarizedLimits_MonthBoundary(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	// Seed a budget + element + limits directly so the period range is testable.
	db.Exec(t, `INSERT INTO budgets (id, user_id, currency_id, name, started_at, created_at, updated_at) VALUES (?, ?, ?, 'B', ?, ?, ?)`,
		budgetID, userA, usdID, startedAt, fixedTime, fixedTime)
	eid := "e0000000-0000-0000-0000-0000000000e1"
	externalID := "ec000000-0000-0000-0000-0000000000c1"
	db.Exec(t, `INSERT INTO budgets_elements (id, budget_id, external_id, type, position, created_at, updated_at) VALUES (?, ?, ?, 1, 0, ?, ?)`,
		eid, budgetID, externalID, fixedTime, fixedTime)
	// Two limits: April (in range) + May (out of range for an April-only window).
	db.Exec(t, `INSERT INTO budgets_elements_limits (id, element_id, period, amount, created_at, updated_at) VALUES (?, ?, '2024-04-01 00:00:00', '120.55', ?, ?)`,
		"71000000-0000-0000-0000-000000000001", eid, fixedTime, fixedTime)
	db.Exec(t, `INSERT INTO budgets_elements_limits (id, element_id, period, amount, created_at, updated_at) VALUES (?, ?, '2024-05-01 00:00:00', '300.00', ?, ?)`,
		"71000000-0000-0000-0000-000000000002", eid, fixedTime, fixedTime)

	start := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	rows, err := read.SummarizedLimits(ctx, vo.MustParseId(budgetID), start, end)
	if err != nil {
		t.Fatalf("SummarizedLimits: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 summarized limit (April only), got %d", len(rows))
	}
	if rows[0].Amount != "120.55000000" {
		t.Errorf("summarized limit mismatch: %q", rows[0].Amount)
	}
	if rows[0].ExternalID != externalID {
		t.Errorf("external id mismatch: %q", rows[0].ExternalID)
	}
}
