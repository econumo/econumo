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
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func seedAccount(t *testing.T, db *dbtest.DB, id, userID string) {
	t.Helper()
	fixture.New(t, db).Account(fixture.Account{ID: id, CurrencyID: usdID, UserID: userID, Name: "A", Icon: "x"})
}

func seedCategory(t *testing.T, db *dbtest.DB, id, userID string) {
	t.Helper()
	fixture.New(t, db).Category(fixture.Category{ID: id, UserID: userID, Name: "C", Icon: "x"})
}

func seedExpense(t *testing.T, db *dbtest.DB, id, account, category, amount, spentAt string) {
	t.Helper()
	fixture.New(t, db).Transaction(fixture.Transaction{ID: id, UserID: userA, AccountID: account, CategoryID: category, Type: 0, Amount: amount, SpentAt: spentAt})
}

func newReadRepo(t *testing.T) (*budgetrepo.ReadRepo, *dbtest.DB) {
	t.Helper()
	db := dbtest.NewSQLite(t)
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
	f := fixture.New(t, db)
	f.Transaction(fixture.Transaction{ID: "70000000-0000-0000-0000-000000000001", UserID: userA, AccountID: acctA, Type: 1, Amount: "100.10", SpentAt: "2024-03-10 00:00:00"})
	f.Transaction(fixture.Transaction{ID: "70000000-0000-0000-0000-000000000002", UserID: userA, AccountID: acctA, Type: 1, Amount: "200.20", SpentAt: "2024-03-11 00:00:00"})
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
	f := fixture.New(t, db)
	f.Budget(fixture.Budget{ID: budgetID, UserID: userA, CurrencyID: usdID, Name: "B", StartedAt: startedAt})
	eid := "e0000000-0000-0000-0000-0000000000e1"
	externalID := "ec000000-0000-0000-0000-0000000000c1"
	f.BudgetElement(fixture.BudgetElement{ID: eid, BudgetID: budgetID, ExternalID: externalID, Type: 1, Position: 0})
	// Two limits: April (in range) + May (out of range for an April-only window).
	f.BudgetLimit(fixture.BudgetLimit{ID: "71000000-0000-0000-0000-000000000001", ElementID: eid, Period: "2024-04-01 00:00:00", Amount: "120.55"})
	f.BudgetLimit(fixture.BudgetLimit{ID: "71000000-0000-0000-0000-000000000002", ElementID: eid, Period: "2024-05-01 00:00:00", Amount: "300.00"})

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
