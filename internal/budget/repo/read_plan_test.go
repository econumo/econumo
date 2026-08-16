package repo_test

// Integration tests for the three grouped-by-month plan queries
// (SpendingByMonth, IncomeByMonth, LimitsByMonth). Regression-locks the
// window-boundary datetime binding (the 2024-07-01 row must be EXCLUDED from
// a [Apr, Jul) window) and the empty-account-set short-circuit.

import (
	"context"
	"strconv"
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
		key := "ext=" + r.ExternalID + " type=" + strconv.Itoa(int(r.Type)) + " month=" + r.Month
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

// seedTransferAccounts sets up the boundary scenario shared by the two
// transfer queries: A (USD, included), B (EUR, included), X (USD, excluded),
// Y (EUR, excluded). Returns the included set.
func seedTransferAccounts(t *testing.T, f *fixture.Builder) (included []vo.Id, acctB, acctX, acctY, eurID string) {
	t.Helper()
	eurID = f.Currency(fixture.Currency{ID: "eeee0000-0000-0000-0000-0000000000e1", Code: "EUR", Symbol: "€", Name: "Euro"})
	acctB = "aaaa1111-0000-0000-0000-0000000000b2"
	acctX = "aaaa1111-0000-0000-0000-0000000000c3"
	acctY = "aaaa1111-0000-0000-0000-0000000000d4"
	f.Account(fixture.Account{ID: acctB, UserID: userA, CurrencyID: eurID, Name: "B"})
	f.Account(fixture.Account{ID: acctX, UserID: userA, CurrencyID: usdID, Name: "X"})
	f.Account(fixture.Account{ID: acctY, UserID: userA, CurrencyID: eurID, Name: "Y"})
	return []vo.Id{vo.MustParseId(acctA), vo.MustParseId(acctB)}, acctB, acctX, acctY, eurID
}

func TestTransfersByMonth(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	included, acctB, acctX, acctY, eurID := seedTransferAccounts(t, f)

	// out: A(USD) -> X, same currency, first instant of the window (boundary row kept)
	f.Transaction(fixture.Transaction{ID: "7b000000-0000-0000-0000-000000000001", UserID: userA, AccountID: acctA, AccountRecipientID: acctX, Type: 2, Amount: "100.00", AmountRecipient: "100.00", SpentAt: "2024-04-01 00:00:00"})
	// out: A(USD) -> Y(EUR), cross-currency: counted in USD at the source amount
	f.Transaction(fixture.Transaction{ID: "7b000000-0000-0000-0000-000000000002", UserID: userA, AccountID: acctA, AccountRecipientID: acctY, Type: 2, Amount: "50.00", AmountRecipient: "45.00", SpentAt: "2024-04-15 00:00:00"})
	// in: X -> B(EUR), cross-currency: counted in EUR at the recipient amount
	f.Transaction(fixture.Transaction{ID: "7b000000-0000-0000-0000-000000000003", UserID: userA, AccountID: acctX, AccountRecipientID: acctB, Type: 2, Amount: "20.00", AmountRecipient: "18.00", SpentAt: "2024-05-03 00:00:00"})
	// in: Y -> A(USD) — same month as an out on A, so the USD row carries both
	f.Transaction(fixture.Transaction{ID: "7b000000-0000-0000-0000-000000000004", UserID: userA, AccountID: acctY, AccountRecipientID: acctA, Type: 2, Amount: "9.00", AmountRecipient: "10.00", SpentAt: "2024-04-20 00:00:00"})
	// both sides included: cancels, not a row
	f.Transaction(fixture.Transaction{ID: "7b000000-0000-0000-0000-000000000005", UserID: userA, AccountID: acctA, AccountRecipientID: acctB, Type: 2, Amount: "30.00", AmountRecipient: "27.00", SpentAt: "2024-05-10 00:00:00"})
	// neither side included: ignored
	f.Transaction(fixture.Transaction{ID: "7b000000-0000-0000-0000-000000000006", UserID: userA, AccountID: acctX, AccountRecipientID: acctY, Type: 2, Amount: "70.00", AmountRecipient: "63.00", SpentAt: "2024-05-11 00:00:00"})
	// outside the window: the end bound is exclusive
	f.Transaction(fixture.Transaction{ID: "7b000000-0000-0000-0000-000000000007", UserID: userA, AccountID: acctA, AccountRecipientID: acctX, Type: 2, Amount: "999.00", AmountRecipient: "999.00", SpentAt: "2024-07-01 00:00:00"})
	// expenses/incomes never count as transfers
	f.Transaction(fixture.Transaction{ID: "7b000000-0000-0000-0000-000000000008", UserID: userA, AccountID: acctA, Type: 0, Amount: "5.00", SpentAt: "2024-04-02 00:00:00"})

	apr := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	jul := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)
	rows, err := read.TransfersByMonth(ctx, included, apr, jul)
	if err != nil {
		t.Fatalf("TransfersByMonth: %v", err)
	}
	type inOut struct{ in, out string }
	got := map[string]inOut{}
	for _, r := range rows {
		key := r.Month + " " + r.CurrencyID
		if _, dup := got[key]; dup {
			t.Fatalf("duplicate row %q: %+v", key, rows)
		}
		got[key] = inOut{vo.NewDecimal(r.In).String(), vo.NewDecimal(r.Out).String()}
	}
	want := map[string]inOut{
		"2024-04-01 " + usdID: {"10", "150"},
		"2024-05-01 " + eurID: {"18", "0"},
	}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for key, w := range want {
		if got[key] != w {
			t.Errorf("row %q = %+v, want %+v", key, got[key], w)
		}
	}
	// month, then currency id
	for i := 1; i < len(rows); i++ {
		if rows[i-1].Month > rows[i].Month || (rows[i-1].Month == rows[i].Month && rows[i-1].CurrencyID > rows[i].CurrencyID) {
			t.Errorf("rows not sorted: %+v", rows)
		}
	}
}

func TestBudgetTransactionsTransfers(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	included, acctB, acctX, acctY, eurID := seedTransferAccounts(t, f)

	f.Transaction(fixture.Transaction{ID: "7c000000-0000-0000-0000-000000000001", UserID: userA, AccountID: acctA, AccountRecipientID: acctY, Type: 2, Amount: "50.00", AmountRecipient: "45.00", SpentAt: "2024-04-01 00:00:00", Description: "to savings"})
	f.Transaction(fixture.Transaction{ID: "7c000000-0000-0000-0000-000000000002", UserID: userA, AccountID: acctX, AccountRecipientID: acctB, Type: 2, Amount: "20.00", AmountRecipient: "18.00", SpentAt: "2024-04-20 00:00:00"})
	// both included / neither included / other month / not a transfer: excluded
	f.Transaction(fixture.Transaction{ID: "7c000000-0000-0000-0000-000000000003", UserID: userA, AccountID: acctA, AccountRecipientID: acctB, Type: 2, Amount: "30.00", AmountRecipient: "27.00", SpentAt: "2024-04-10 00:00:00"})
	f.Transaction(fixture.Transaction{ID: "7c000000-0000-0000-0000-000000000004", UserID: userA, AccountID: acctX, AccountRecipientID: acctY, Type: 2, Amount: "70.00", AmountRecipient: "63.00", SpentAt: "2024-04-11 00:00:00"})
	f.Transaction(fixture.Transaction{ID: "7c000000-0000-0000-0000-000000000005", UserID: userA, AccountID: acctA, AccountRecipientID: acctX, Type: 2, Amount: "1.00", AmountRecipient: "1.00", SpentAt: "2024-05-01 00:00:00"})
	f.Transaction(fixture.Transaction{ID: "7c000000-0000-0000-0000-000000000006", UserID: userA, AccountID: acctA, Type: 0, Amount: "5.00", SpentAt: "2024-04-02 00:00:00"})

	apr := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	may := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	rows, err := read.BudgetTransactionsTransfers(ctx, included, apr, may)
	if err != nil {
		t.Fatalf("BudgetTransactionsTransfers: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d: %+v", len(rows), rows)
	}
	// newest first
	if rows[0].ID != "7c000000-0000-0000-0000-000000000002" || rows[1].ID != "7c000000-0000-0000-0000-000000000001" {
		t.Fatalf("unexpected order: %+v", rows)
	}
	in, out := rows[0], rows[1]
	if in.Direction != "in" || in.CurrencyID != eurID || vo.NewDecimal(in.Amount).String() != "18" {
		t.Errorf("incoming row should carry the recipient side: %+v", in)
	}
	if out.Direction != "out" || out.CurrencyID != usdID || vo.NewDecimal(out.Amount).String() != "50" || out.Description != "to savings" {
		t.Errorf("outgoing row should carry the source side: %+v", out)
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
	if rows, err := read.TransfersByMonth(ctx, nil, apr, jul); err != nil || rows != nil {
		t.Errorf("TransfersByMonth empty accounts should be nil,nil; got %v, %v", rows, err)
	}
	if rows, err := read.BudgetTransactionsTransfers(ctx, nil, apr, jul); err != nil || rows != nil {
		t.Errorf("BudgetTransactionsTransfers empty accounts should be nil,nil; got %v, %v", rows, err)
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
