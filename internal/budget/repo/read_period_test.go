package repo_test

// The by-month reads bucketed by a budget's period start day. Each case puts
// rows on both sides of the day-28 boundary (23:59:59 the day before, 00:00
// on the day) and one on the window's exclusive end, then reads the same
// rows with startDay 1 over calendar months.

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

var (
	psSep28 = time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	psNov28 = time.Date(2026, 11, 28, 0, 0, 0, 0, time.UTC)
	psOct1  = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	psDec1  = time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
)

func TestSpendingByMonthStartDay(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	catFood := "c1000000-0000-0000-0000-0000000000f1"
	f.Category(fixture.Category{ID: catFood, UserID: userA, Name: "Food", Type: 0})

	f.Transaction(fixture.Transaction{ID: "7d000000-0000-0000-0000-000000000001", UserID: userA, AccountID: acctA, CategoryID: catFood, Type: 0, Amount: "100.00", SpentAt: "2026-10-27 23:59:59"})
	f.Transaction(fixture.Transaction{ID: "7d000000-0000-0000-0000-000000000002", UserID: userA, AccountID: acctA, CategoryID: catFood, Type: 0, Amount: "50.00", SpentAt: "2026-10-28 00:00:00"})
	f.Transaction(fixture.Transaction{ID: "7d000000-0000-0000-0000-000000000003", UserID: userA, AccountID: acctA, CategoryID: catFood, Type: 0, Amount: "20.00", SpentAt: "2026-11-27 10:00:00"})
	f.Transaction(fixture.Transaction{ID: "7d000000-0000-0000-0000-000000000004", UserID: userA, AccountID: acctA, CategoryID: catFood, Type: 0, Amount: "10.00", SpentAt: "2026-11-28 00:00:00"})

	cats, accs := []vo.Id{vo.MustParseId(catFood)}, []vo.Id{vo.MustParseId(acctA)}
	rows, err := read.SpendingByMonth(ctx, cats, accs, psSep28, psNov28, 28)
	if err != nil {
		t.Fatalf("SpendingByMonth d=28: %v", err)
	}
	// The two Nov rows (Oct 28 and Nov 27) merge into one.
	assertPlanSpendingRows(t, rows, map[string]string{
		"month=2026-10-01 cat=" + catFood + " tag=": "100",
		"month=2026-11-01 cat=" + catFood + " tag=": "70",
	})
	for _, r := range rows {
		if r.Amount != vo.NewDecimal(r.Amount).String() {
			t.Errorf("folded amount %q is not in normalized decimal form", r.Amount)
		}
	}

	rows, err = read.SpendingByMonth(ctx, cats, accs, psOct1, psDec1, 1)
	if err != nil {
		t.Fatalf("SpendingByMonth d=1: %v", err)
	}
	assertPlanSpendingRows(t, rows, map[string]string{
		"month=2026-10-01 cat=" + catFood + " tag=": "150",
		"month=2026-11-01 cat=" + catFood + " tag=": "30",
	})
}

func TestIncomeByMonthStartDay(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	catSalary := "c1000000-0000-0000-0000-0000000000f2"
	f.Category(fixture.Category{ID: catSalary, UserID: userA, Name: "Salary", Type: 1})

	f.Transaction(fixture.Transaction{ID: "7e000000-0000-0000-0000-000000000001", UserID: userA, AccountID: acctA, CategoryID: catSalary, Type: 1, Amount: "100.00", SpentAt: "2026-10-27 23:59:59"})
	f.Transaction(fixture.Transaction{ID: "7e000000-0000-0000-0000-000000000002", UserID: userA, AccountID: acctA, CategoryID: catSalary, Type: 1, Amount: "0.10", SpentAt: "2026-10-28 00:00:00"})
	f.Transaction(fixture.Transaction{ID: "7e000000-0000-0000-0000-000000000003", UserID: userA, AccountID: acctA, CategoryID: catSalary, Type: 1, Amount: "0.20", SpentAt: "2026-11-27 10:00:00"})
	f.Transaction(fixture.Transaction{ID: "7e000000-0000-0000-0000-000000000004", UserID: userA, AccountID: acctA, CategoryID: catSalary, Type: 1, Amount: "10.00", SpentAt: "2026-11-28 00:00:00"})

	accs := []vo.Id{vo.MustParseId(acctA)}
	rows, err := read.IncomeByMonth(ctx, accs, psSep28, psNov28, 28)
	if err != nil {
		t.Fatalf("IncomeByMonth d=28: %v", err)
	}
	// Decimal addition, not float: 0.10 + 0.20 is exactly 0.3.
	assertIncomeRows(t, rows, map[string]string{"2026-10-01": "100", "2026-11-01": "0.3"}, true)

	rows, err = read.IncomeByMonth(ctx, accs, psOct1, psDec1, 1)
	if err != nil {
		t.Fatalf("IncomeByMonth d=1: %v", err)
	}
	assertIncomeRows(t, rows, map[string]string{"2026-10-01": "100.1", "2026-11-01": "10.2"}, false)
}

func assertIncomeRows(t *testing.T, rows []model.MonthlyIncomeRow, want map[string]string, exact bool) {
	t.Helper()
	got := map[string]string{}
	for _, r := range rows {
		if _, dup := got[r.Month]; dup {
			t.Fatalf("duplicate row for %s: %+v", r.Month, rows)
		}
		if exact {
			got[r.Month] = r.Amount
		} else {
			got[r.Month] = vo.NewDecimal(r.Amount).String()
		}
	}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for m, a := range want {
		if got[m] != a {
			t.Errorf("%s = %q, want %q (all %v)", m, got[m], a, got)
		}
	}
}

func TestTransfersByMonthStartDay(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	included, _, acctX, _, _ := seedTransferAccounts(t, f)

	tr := func(id, src, dst, amount, at string) {
		f.Transaction(fixture.Transaction{ID: id, UserID: userA, AccountID: src, AccountRecipientID: dst, Type: 2, Amount: amount, AmountRecipient: amount, SpentAt: at})
	}
	tr("7f000000-0000-0000-0000-000000000001", acctA, acctX, "100.00", "2026-10-27 23:59:59")
	tr("7f000000-0000-0000-0000-000000000002", acctA, acctX, "50.00", "2026-10-28 00:00:00")
	tr("7f000000-0000-0000-0000-000000000003", acctA, acctX, "20.00", "2026-11-27 10:00:00")
	tr("7f000000-0000-0000-0000-000000000004", acctA, acctX, "10.00", "2026-11-28 00:00:00")
	tr("7f000000-0000-0000-0000-000000000005", acctX, acctA, "3.00", "2026-10-27 23:59:59")
	tr("7f000000-0000-0000-0000-000000000006", acctX, acctA, "4.00", "2026-10-28 00:00:00")
	tr("7f000000-0000-0000-0000-000000000007", acctX, acctA, "5.00", "2026-11-27 10:00:00")

	check := func(rows []model.MonthlyTransferRow, want map[string][2]string) {
		t.Helper()
		got := map[string][2]string{}
		for _, r := range rows {
			if r.CurrencyID != usdID {
				t.Errorf("unexpected currency row %+v", r)
			}
			if _, dup := got[r.Month]; dup {
				t.Fatalf("duplicate row for %s: %+v", r.Month, rows)
			}
			got[r.Month] = [2]string{vo.NewDecimal(r.In).String(), vo.NewDecimal(r.Out).String()}
		}
		if len(got) != len(want) {
			t.Fatalf("rows = %v, want %v", got, want)
		}
		for m, w := range want {
			if got[m] != w {
				t.Errorf("%s in/out = %v, want %v", m, got[m], w)
			}
		}
		for i := 1; i < len(rows); i++ {
			if rows[i-1].Month > rows[i].Month {
				t.Errorf("rows not sorted by month: %+v", rows)
			}
		}
	}

	rows, err := read.TransfersByMonth(ctx, included, psSep28, psNov28, 28)
	if err != nil {
		t.Fatalf("TransfersByMonth d=28: %v", err)
	}
	check(rows, map[string][2]string{"2026-10-01": {"3", "100"}, "2026-11-01": {"9", "70"}})

	rows, err = read.TransfersByMonth(ctx, included, psOct1, psDec1, 1)
	if err != nil {
		t.Fatalf("TransfersByMonth d=1: %v", err)
	}
	check(rows, map[string][2]string{"2026-10-01": {"7", "150"}, "2026-11-01": {"5", "30"}})
}

func assertSavingsMonths(t *testing.T, rows []model.SavingsMonthRow, want map[string]string) {
	t.Helper()
	got := map[string]string{}
	for _, r := range rows {
		if r.AccountID != acctA {
			t.Errorf("unexpected account row %+v", r)
		}
		if _, dup := got[r.Month]; dup {
			t.Fatalf("duplicate row for %s: %+v", r.Month, rows)
		}
		got[r.Month] = vo.NewDecimal(r.Amount).String()
	}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for m, a := range want {
		if got[m] != a {
			t.Errorf("%s = %q, want %q (all %v)", m, got[m], a, got)
		}
	}
}

func TestAccountsNetByMonthStartDay(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	_, _, acctX, _, _ := seedTransferAccounts(t, f)

	f.Transaction(fixture.Transaction{ID: "7f100000-0000-0000-0000-000000000001", UserID: userA, AccountID: acctA, Type: 1, Amount: "100.00", SpentAt: "2026-10-27 23:59:59"})
	f.Transaction(fixture.Transaction{ID: "7f100000-0000-0000-0000-000000000002", UserID: userA, AccountID: acctA, Type: 0, Amount: "50.00", SpentAt: "2026-10-28 00:00:00"})
	f.Transaction(fixture.Transaction{ID: "7f100000-0000-0000-0000-000000000003", UserID: userA, AccountID: acctX, AccountRecipientID: acctA, Type: 2, Amount: "20.00", AmountRecipient: "20.00", SpentAt: "2026-11-27 10:00:00"})
	f.Transaction(fixture.Transaction{ID: "7f100000-0000-0000-0000-000000000004", UserID: userA, AccountID: acctA, AccountRecipientID: acctX, Type: 2, Amount: "5.00", AmountRecipient: "5.00", SpentAt: "2026-11-27 23:00:00"})
	f.Transaction(fixture.Transaction{ID: "7f100000-0000-0000-0000-000000000005", UserID: userA, AccountID: acctA, Type: 0, Amount: "10.00", SpentAt: "2026-11-28 00:00:00"})

	accs := []vo.Id{vo.MustParseId(acctA)}
	rows, err := read.AccountsNetByMonth(ctx, accs, psSep28, psNov28, 28)
	if err != nil {
		t.Fatalf("AccountsNetByMonth d=28: %v", err)
	}
	assertSavingsMonths(t, rows, map[string]string{"2026-10-01": "100", "2026-11-01": "-35"})

	rows, err = read.AccountsNetByMonth(ctx, accs, psOct1, psDec1, 1)
	if err != nil {
		t.Fatalf("AccountsNetByMonth d=1: %v", err)
	}
	assertSavingsMonths(t, rows, map[string]string{"2026-10-01": "50", "2026-11-01": "5"})
}

func TestAccountsIncomeExpenseByMonthStartDay(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	_, _, acctX, _, _ := seedTransferAccounts(t, f)

	f.Transaction(fixture.Transaction{ID: "7f200000-0000-0000-0000-000000000001", UserID: userA, AccountID: acctA, Type: 1, Amount: "100.00", SpentAt: "2026-10-27 23:59:59"})
	f.Transaction(fixture.Transaction{ID: "7f200000-0000-0000-0000-000000000002", UserID: userA, AccountID: acctA, Type: 0, Amount: "50.00", SpentAt: "2026-10-28 00:00:00"})
	f.Transaction(fixture.Transaction{ID: "7f200000-0000-0000-0000-000000000003", UserID: userA, AccountID: acctA, Type: 1, Amount: "20.00", SpentAt: "2026-11-27 10:00:00"})
	f.Transaction(fixture.Transaction{ID: "7f200000-0000-0000-0000-000000000004", UserID: userA, AccountID: acctA, Type: 0, Amount: "10.00", SpentAt: "2026-11-28 00:00:00"})
	// Transfers do not count.
	f.Transaction(fixture.Transaction{ID: "7f200000-0000-0000-0000-000000000005", UserID: userA, AccountID: acctA, AccountRecipientID: acctX, Type: 2, Amount: "999.00", AmountRecipient: "999.00", SpentAt: "2026-10-28 00:00:00"})

	accs := []vo.Id{vo.MustParseId(acctA)}
	rows, err := read.AccountsIncomeExpenseByMonth(ctx, accs, psSep28, psNov28, 28)
	if err != nil {
		t.Fatalf("AccountsIncomeExpenseByMonth d=28: %v", err)
	}
	assertSavingsMonths(t, rows, map[string]string{"2026-10-01": "100", "2026-11-01": "-30"})

	rows, err = read.AccountsIncomeExpenseByMonth(ctx, accs, psOct1, psDec1, 1)
	if err != nil {
		t.Fatalf("AccountsIncomeExpenseByMonth d=1: %v", err)
	}
	assertSavingsMonths(t, rows, map[string]string{"2026-10-01": "50", "2026-11-01": "10"})
}
