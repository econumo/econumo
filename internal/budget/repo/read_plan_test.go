package repo_test

// Integration tests for the three grouped-by-month plan queries
// (SpendingByMonth, IncomeByMonth, LimitsByMonth). Regression-locks the
// window-boundary datetime binding (the 2024-07-01 row must be EXCLUDED from
// a [Apr, Jul) window) and the empty-account-set short-circuit.

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestSpendingByMonth(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)

	catFood := "c1000000-0000-0000-0000-0000000000f1"
	catSalary := "c1000000-0000-0000-0000-0000000000f2"
	tagTrip := "7a100000-0000-0000-0000-0000000000f1"
	f.Category(fixture.Category{ID: catFood, UserID: userA, Name: "Food", Type: 0})
	f.Category(fixture.Category{ID: catSalary, UserID: userA, Name: "Salary", Type: 1})
	f.Tag(fixture.Tag{ID: tagTrip, UserID: userA, Name: "Trip"})

	f.Transaction(fixture.Transaction{ID: "78000000-0000-0000-0000-000000000001", UserID: userA, AccountID: acctA, CategoryID: catFood, Type: 0, Amount: "40.00", SpentAt: "2024-04-10 10:00:00"})
	f.Transaction(fixture.Transaction{ID: "78000000-0000-0000-0000-000000000002", UserID: userA, AccountID: acctA, CategoryID: catFood, Type: 0, Amount: "60.00", SpentAt: "2024-05-02 09:00:00"})
	f.Transaction(fixture.Transaction{ID: "78000000-0000-0000-0000-000000000003", UserID: userA, AccountID: acctA, CategoryID: catFood, TagID: tagTrip, Type: 0, Amount: "15.00", SpentAt: "2024-05-20 12:00:00"})
	f.Transaction(fixture.Transaction{ID: "78000000-0000-0000-0000-000000000004", UserID: userA, AccountID: acctA, Type: 0, Amount: "5.00", SpentAt: "2024-05-21 12:00:00"})
	// Outside the window [Apr..Jul) — must not appear.
	f.Transaction(fixture.Transaction{ID: "78000000-0000-0000-0000-000000000005", UserID: userA, AccountID: acctA, CategoryID: catFood, Type: 0, Amount: "99.00", SpentAt: "2024-07-01 00:00:00"})
	// Income rows must not leak into SpendingByMonth.
	f.Transaction(fixture.Transaction{ID: "78000000-0000-0000-0000-000000000006", UserID: userA, AccountID: acctA, CategoryID: catSalary, Type: 1, Amount: "1000.00", SpentAt: "2024-04-05 08:00:00"})
	f.Transaction(fixture.Transaction{ID: "78000000-0000-0000-0000-000000000007", UserID: userA, AccountID: acctA, Type: 1, Amount: "50.00", SpentAt: "2024-05-06 08:00:00"})

	apr := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	jul := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)

	rows, err := read.SpendingByMonth(ctx, []vo.Id{vo.MustParseId(catFood)}, []vo.Id{vo.MustParseId(acctA)}, apr, jul)
	if err != nil {
		t.Fatalf("SpendingByMonth: %v", err)
	}

	assertPlanSpendingRows(t, rows, map[string]string{
		"month=2024-04-01 cat=" + catFood + " tag=":           "40",
		"month=2024-05-01 cat=" + catFood + " tag=":           "60",
		"month=2024-05-01 cat=" + catFood + " tag=" + tagTrip: "15",
		"month=2024-05-01 cat= tag=":                          "5",
	})
}

func TestIncomeByMonth(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)

	catFood := "c1000000-0000-0000-0000-0000000000f1"
	catSalary := "c1000000-0000-0000-0000-0000000000f2"
	f.Category(fixture.Category{ID: catFood, UserID: userA, Name: "Food", Type: 0})
	f.Category(fixture.Category{ID: catSalary, UserID: userA, Name: "Salary", Type: 1})

	// Expenses must not leak into IncomeByMonth.
	f.Transaction(fixture.Transaction{ID: "79000000-0000-0000-0000-000000000001", UserID: userA, AccountID: acctA, CategoryID: catFood, Type: 0, Amount: "40.00", SpentAt: "2024-04-10 10:00:00"})
	f.Transaction(fixture.Transaction{ID: "79000000-0000-0000-0000-000000000002", UserID: userA, AccountID: acctA, CategoryID: catSalary, Type: 1, Amount: "1000.00", SpentAt: "2024-04-05 08:00:00"})
	f.Transaction(fixture.Transaction{ID: "79000000-0000-0000-0000-000000000003", UserID: userA, AccountID: acctA, Type: 1, Amount: "50.00", SpentAt: "2024-05-06 08:00:00"})
	// Outside the window — must not appear.
	f.Transaction(fixture.Transaction{ID: "79000000-0000-0000-0000-000000000004", UserID: userA, AccountID: acctA, CategoryID: catSalary, Type: 1, Amount: "777.00", SpentAt: "2024-07-01 00:00:00"})

	apr := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	jul := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)

	rows, err := read.IncomeByMonth(ctx, []vo.Id{vo.MustParseId(acctA)}, apr, jul)
	if err != nil {
		t.Fatalf("IncomeByMonth: %v", err)
	}

	got := map[string]string{}
	for _, r := range rows {
		key := "month=" + r.Month + " cat="
		if r.CategoryID != nil {
			key += *r.CategoryID
		}
		if _, dup := got[key]; dup {
			t.Fatalf("duplicate row for %q: %+v", key, rows)
		}
		got[key] = vo.NewDecimal(r.Amount).String()
	}
	want := map[string]string{
		"month=2024-04-01 cat=" + catSalary: "1000",
		"month=2024-05-01 cat=":             "50",
	}
	if len(got) != len(want) {
		t.Fatalf("want %d rows %v, got %d %v", len(want), want, len(got), got)
	}
	for key, amount := range want {
		a, ok := got[key]
		if !ok {
			t.Fatalf("missing row %q; got %v", key, got)
		}
		if a != vo.NewDecimal(amount).String() {
			t.Errorf("row %q amount = %s, want %s", key, a, amount)
		}
	}
}

func TestLimitsByMonth(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)

	catFood := "c1000000-0000-0000-0000-0000000000f1"
	catSalary := "c1000000-0000-0000-0000-0000000000f2"
	f.Category(fixture.Category{ID: catFood, UserID: userA, Name: "Food", Type: 0})
	f.Category(fixture.Category{ID: catSalary, UserID: userA, Name: "Salary", Type: 1})

	f.Budget(fixture.Budget{ID: budgetID, UserID: userA, CurrencyID: usdID, Name: "B", StartedAt: startedAt})
	elFood := "e1000000-0000-0000-0000-0000000000f1"
	elSalary := "e1000000-0000-0000-0000-0000000000f2"
	f.BudgetElement(fixture.BudgetElement{ID: elFood, BudgetID: budgetID, ExternalID: catFood, Type: 1, Position: 0})
	f.BudgetElement(fixture.BudgetElement{ID: elSalary, BudgetID: budgetID, ExternalID: catSalary, Type: 3, Position: 1})

	f.BudgetLimit(fixture.BudgetLimit{ID: "7a000000-0000-0000-0000-0000000000f1", ElementID: elFood, Period: "2024-04-01 00:00:00", Amount: "100"})
	f.BudgetLimit(fixture.BudgetLimit{ID: "7a000000-0000-0000-0000-0000000000f2", ElementID: elFood, Period: "2024-05-01 00:00:00", Amount: "120"})
	f.BudgetLimit(fixture.BudgetLimit{ID: "7a000000-0000-0000-0000-0000000000f3", ElementID: elSalary, Period: "2024-05-01 00:00:00", Amount: "1500"})
	// Outside the window — must not appear.
	f.BudgetLimit(fixture.BudgetLimit{ID: "7a000000-0000-0000-0000-0000000000f4", ElementID: elFood, Period: "2024-07-01 00:00:00", Amount: "999"})

	apr := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	jul := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)

	rows, err := read.LimitsByMonth(ctx, vo.MustParseId(budgetID), apr, jul)
	if err != nil {
		t.Fatalf("LimitsByMonth: %v", err)
	}

	got := map[string]string{}
	for _, r := range rows {
		key := "ext=" + r.ExternalID + " type=" + itoaTest(int(r.Type)) + " month=" + r.Month
		if _, dup := got[key]; dup {
			t.Fatalf("duplicate row for %q: %+v", key, rows)
		}
		got[key] = vo.NewDecimal(r.Amount).String()
	}
	want := map[string]string{
		"ext=" + catFood + " type=1 month=2024-04-01":   "100",
		"ext=" + catFood + " type=1 month=2024-05-01":   "120",
		"ext=" + catSalary + " type=3 month=2024-05-01": "1500",
	}
	if len(got) != len(want) {
		t.Fatalf("want %d rows %v, got %d %v", len(want), want, len(got), got)
	}
	for key, amount := range want {
		a, ok := got[key]
		if !ok {
			t.Fatalf("missing row %q; got %v", key, got)
		}
		if a != vo.NewDecimal(amount).String() {
			t.Errorf("row %q amount = %s, want %s", key, a, amount)
		}
	}
}

func TestPlanQueries_EmptyAccountSet(t *testing.T) {
	read, _ := newReadRepo(t)
	ctx := context.Background()
	apr := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	jul := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)

	// Empty accountIDs must short-circuit to (nil, nil) on BOTH engines — an
	// empty IN () is a pgsql syntax error (same guard as CountSpending).
	if rows, err := read.SpendingByMonth(ctx, nil, nil, apr, jul); err != nil || rows != nil {
		t.Errorf("SpendingByMonth empty accounts should be nil,nil; got %v, %v", rows, err)
	}
	if rows, err := read.IncomeByMonth(ctx, nil, apr, jul); err != nil || rows != nil {
		t.Errorf("IncomeByMonth empty accounts should be nil,nil; got %v, %v", rows, err)
	}
}

// assertPlanSpendingRows checks the exact identity of a MonthlySpendingRow
// set keyed by (month, category, tag). Amounts go through vo.NewDecimal
// because the engines render sums differently (sqlite float text
// "40.00000000" vs pgsql NUMERIC "40.00").
func assertPlanSpendingRows(t *testing.T, rows []model.MonthlySpendingRow, want map[string]string) {
	t.Helper()
	got := map[string]string{}
	for _, r := range rows {
		key := "month=" + r.Month + " cat="
		if r.CategoryID != nil {
			key += *r.CategoryID
		}
		key += " tag="
		if r.TagID != nil {
			key += *r.TagID
		}
		if _, dup := got[key]; dup {
			t.Fatalf("duplicate row for %q: %+v", key, rows)
		}
		got[key] = vo.NewDecimal(r.Amount).String()
	}
	if len(got) != len(want) {
		t.Fatalf("want %d rows %v, got %d %v", len(want), want, len(got), got)
	}
	for key, amount := range want {
		a, ok := got[key]
		if !ok {
			t.Fatalf("missing row %q; got %v", key, got)
		}
		if a != vo.NewDecimal(amount).String() {
			t.Errorf("row %q amount = %s, want %s", key, a, amount)
		}
	}
}

func itoaTest(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
