package repo_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

const acctOther = "aaaa1111-0000-0000-0000-0000000000a2"

func flowKey(r model.MonthlyFlowRow) string {
	g := ""
	if r.GroupID != nil {
		g = *r.GroupID
	}
	return r.Month + "|" + string(rune('0'+r.Type)) + "|" + g + "|" + r.CurrencyID
}

func assertFlow(t *testing.T, rows []model.MonthlyFlowRow, want map[string]string) {
	t.Helper()
	got := map[string]string{}
	for _, r := range rows {
		got[flowKey(r)] = vo.NewDecimal(r.Amount).String()
	}
	if len(got) != len(rows) {
		t.Fatalf("duplicate group keys in %v", rows)
	}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
}

func TestFlowByMonth_GroupsByCategory(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	cat := "c1000000-0000-0000-0000-0000000000e1"
	incCat := "c1000000-0000-0000-0000-0000000000e2"
	f.Category(fixture.Category{ID: cat, UserID: userA, Name: "Food", Type: 0})
	f.Category(fixture.Category{ID: incCat, UserID: userA, Name: "Salary", Type: 1})
	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000001", UserID: userA, AccountID: acctA, CategoryID: cat, Type: 0, Amount: "10.00", SpentAt: "2024-03-02 10:00:00"})
	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000002", UserID: userA, AccountID: acctA, CategoryID: cat, Type: 0, Amount: "5.50", SpentAt: "2024-03-20 10:00:00"})
	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000003", UserID: userA, AccountID: acctA, CategoryID: incCat, Type: 1, Amount: "100.00", SpentAt: "2024-04-05 10:00:00"})
	// Outside [from, to).
	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000004", UserID: userA, AccountID: acctA, CategoryID: cat, Type: 0, Amount: "99.00", SpentAt: "2024-05-01 00:00:00"})

	rows, err := read.FlowByMonth(ctx, []vo.Id{vo.MustParseId(acctA)},
		time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC), "category")
	if err != nil {
		t.Fatal(err)
	}
	assertFlow(t, rows, map[string]string{
		"2024-03-01|0|" + cat + "|" + usdID:    vo.NewDecimal("15.5").String(),
		"2024-04-01|1|" + incCat + "|" + usdID: vo.NewDecimal("100").String(),
	})
}

func TestFlowByMonth_GroupsByPayeeAndTag(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	payee := "9a000000-0000-0000-0000-0000000000e1"
	tag := "7a100000-0000-0000-0000-0000000000e1"
	f.Payee(fixture.Payee{ID: payee, UserID: userA, Name: "Shop"})
	f.Tag(fixture.Tag{ID: tag, UserID: userA, Name: "Trip"})
	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000011", UserID: userA, AccountID: acctA, PayeeID: payee, TagID: tag, Type: 0, Amount: "7.00", SpentAt: "2024-03-02 10:00:00"})
	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000012", UserID: userA, AccountID: acctA, Type: 0, Amount: "3.00", SpentAt: "2024-03-03 10:00:00"})
	from := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	ids := []vo.Id{vo.MustParseId(acctA)}

	rows, err := read.FlowByMonth(ctx, ids, from, to, "payee")
	if err != nil {
		t.Fatal(err)
	}
	assertFlow(t, rows, map[string]string{
		"2024-03-01|0|" + payee + "|" + usdID: "7",
		"2024-03-01|0||" + usdID:              "3",
	})

	rows, err = read.FlowByMonth(ctx, ids, from, to, "tag")
	if err != nil {
		t.Fatal(err)
	}
	assertFlow(t, rows, map[string]string{
		"2024-03-01|0|" + tag + "|" + usdID: "7",
		"2024-03-01|0||" + usdID:            "3",
	})

	if _, err := read.FlowByMonth(ctx, ids, from, to, "bogus"); err == nil {
		t.Error("unknown groupBy must error")
	}
	if rows, err := read.FlowByMonth(ctx, nil, from, to, "category"); rows != nil || err != nil {
		t.Errorf("empty accounts = %v, %v", rows, err)
	}
}

func TestFlowByMonth_IgnoresTransfersAndOtherAccounts(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	f.Account(fixture.Account{ID: acctOther, UserID: userA, Name: "Other"})
	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000021", UserID: userA, AccountID: acctA, Type: 0, Amount: "4.00", SpentAt: "2024-03-02 10:00:00"})
	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000022", UserID: userA, AccountID: acctA, AccountRecipientID: acctOther, Type: 2, Amount: "50.00", AmountRecipient: "50.00", SpentAt: "2024-03-03 10:00:00"})
	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000023", UserID: userA, AccountID: acctOther, Type: 0, Amount: "80.00", SpentAt: "2024-03-04 10:00:00"})

	rows, err := read.FlowByMonth(ctx, []vo.Id{vo.MustParseId(acctA)},
		time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC), "category")
	if err != nil {
		t.Fatal(err)
	}
	assertFlow(t, rows, map[string]string{"2024-03-01|0||" + usdID: "4"})
}

func TestFirstTransactionDate(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	f.Account(fixture.Account{ID: acctOther, UserID: userA, Name: "Other"})
	ids := []vo.Id{vo.MustParseId(acctA)}

	got, err := read.FirstTransactionDate(ctx, ids)
	if err != nil || got != nil {
		t.Fatalf("no rows = %v, %v", got, err)
	}

	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000031", UserID: userA, AccountID: acctA, Type: 0, Amount: "1.00", SpentAt: "2024-03-10 08:30:00"})
	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000032", UserID: userA, AccountID: acctOther, AccountRecipientID: acctA, Type: 2, Amount: "5.00", AmountRecipient: "5.00", SpentAt: "2024-02-15 09:00:00"})
	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000033", UserID: userA, AccountID: acctOther, Type: 0, Amount: "1.00", SpentAt: "2023-01-01 00:00:00"})

	got, err = read.FirstTransactionDate(ctx, ids)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2024, 2, 15, 9, 0, 0, 0, time.UTC)
	if got == nil || !got.Equal(want) {
		t.Fatalf("first = %v, want %v", got, want)
	}
}

// SQLite sums REAL, so a few dozen cent amounts can drift off their exact total
// (1412.58999999 for 1412.59). Every flow column must still read the exact sum,
// as PostgreSQL's NUMERIC SUM does.
func TestBudgetReadRepo_AccountsReportSumsExactly(t *testing.T) {
	read, db := newReadRepo(t)
	f := fixture.New(t, db)
	r := rand.New(rand.NewPCG(20261029, 1))
	cents := func() string {
		c := 1 + r.IntN(50000)
		return fmt.Sprintf("%d.%02d", c/100, c%100)
	}
	type sums map[string]vo.DecimalNumber
	want := map[string]sums{}
	add := func(account, col, amount string) {
		if want[account] == nil {
			want[account] = sums{}
		}
		cur, ok := want[account][col]
		if !ok {
			cur = vo.NewDecimal("0")
		}
		want[account][col] = cur.Add(vo.NewDecimal(amount))
	}

	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	var ids []vo.Id
	var accounts []string
	for i := range 30 {
		id := fmt.Sprintf("aaaa2222-0000-0000-0000-%012d", i)
		f.Account(fixture.Account{ID: id, CurrencyID: usdID, UserID: userA, Name: fmt.Sprint("A", i), Icon: "x"})
		ids = append(ids, vo.MustParseId(id))
		accounts = append(accounts, id)
	}
	for i := range 1200 {
		account := accounts[r.IntN(len(accounts))]
		other := accounts[r.IntN(len(accounts))]
		if other == account {
			continue
		}
		amount := cents()
		tx := fixture.Transaction{UserID: userA, AccountID: account, Amount: amount, SpentAt: day.Add(time.Duration(i) * time.Minute)}
		switch r.IntN(4) {
		case 0:
			add(account, "expenses", amount)
		case 1:
			tx.Type = 1
			add(account, "incomes", amount)
		case 2:
			tx.Type, tx.AccountRecipientID, tx.AmountRecipient = 2, other, amount
			add(account, "transfer_expenses", amount)
			add(other, "transfer_incomes", amount)
		case 3:
			// an exchange: the credited amount differs
			credited := cents()
			tx.Type, tx.AccountRecipientID, tx.AmountRecipient = 2, other, credited
			add(account, "transfer_expenses", amount)
			add(account, "exchange_expenses", amount)
			add(other, "transfer_incomes", credited)
			add(other, "exchange_incomes", credited)
		}
		f.Transaction(tx)
	}

	// past 2^26 a float64 cannot hold every cent: the double nearest 123456789.01
	// prints as 123456789.01000001 at 8 decimals, so the sum must be rounded from
	// its shortest decimal form, not from the binary value
	large := "aaaa2222-0000-0000-0000-000000000999"
	f.Account(fixture.Account{ID: large, CurrencyID: usdID, UserID: userA, Name: "Large", Icon: "x"})
	ids = append(ids, vo.MustParseId(large))
	for i, amount := range []string{"123456789.01", "1000000000.07", "876543210.39"} {
		tx := fixture.Transaction{UserID: userA, AccountID: large, Amount: amount, SpentAt: day.Add(time.Duration(i) * time.Hour)}
		if i == 0 {
			tx.Type = 1
			add(large, "incomes", amount)
		} else {
			add(large, "expenses", amount)
		}
		f.Transaction(tx)
	}

	rows, err := read.AccountsReport(context.Background(), ids, day, day.AddDate(0, 1, 0))
	if err != nil {
		t.Fatalf("AccountsReport: %v", err)
	}
	seen := 0
	for _, row := range rows {
		exp, ok := want[row.AccountID]
		if !ok {
			continue
		}
		seen++
		for _, c := range []struct{ col, got string }{
			{"expenses", row.Expenses}, {"incomes", row.Incomes}, {"transfer_expenses", row.TransferExpenses},
			{"transfer_incomes", row.TransferIncomes}, {"exchange_expenses", row.ExchangeExpenses}, {"exchange_incomes", row.ExchangeIncomes},
		} {
			w, ok := exp[c.col]
			if !ok {
				w = vo.NewDecimal("0")
			}
			if got := vo.NewDecimal(c.got); got.String() != w.String() {
				t.Errorf("%s %s = %q, want %s", row.AccountID, c.col, c.got, w)
			}
		}
	}
	if seen != len(want) {
		t.Fatalf("report rows for %d accounts, want %d", seen, len(want))
	}
}

// HoldingsReport sums same-amount transfers across the set on SQLite's float
// SUM too; each day below is its own window, so its own chance to drift.
func TestBudgetReadRepo_HoldingsReportSumsExactly(t *testing.T) {
	read, db := newReadRepo(t)
	f := fixture.New(t, db)
	outside := "aaaa3333-0000-0000-0000-000000000001"
	f.Account(fixture.Account{ID: outside, CurrencyID: usdID, UserID: userA, Name: "Out", Icon: "x"})
	r := rand.New(rand.NewPCG(20261029, 2))
	first := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	const days = 60
	toHoldings := make([]vo.DecimalNumber, days)
	fromHoldings := make([]vo.DecimalNumber, days)
	for d := range days {
		toHoldings[d], fromHoldings[d] = vo.NewDecimal("0"), vo.NewDecimal("0")
		for i := range 30 {
			c := 1 + r.IntN(50000)
			amount := fmt.Sprintf("%d.%02d", c/100, c%100)
			tx := fixture.Transaction{UserID: userA, Type: 2, AccountID: acctA, AccountRecipientID: outside, Amount: amount, AmountRecipient: amount,
				SpentAt: first.AddDate(0, 0, d).Add(time.Duration(i) * time.Minute)}
			if i%2 == 0 {
				toHoldings[d] = toHoldings[d].Add(vo.NewDecimal(amount))
			} else {
				tx.AccountID, tx.AccountRecipientID = outside, acctA
				fromHoldings[d] = fromHoldings[d].Add(vo.NewDecimal(amount))
			}
			f.Transaction(tx)
		}
	}
	for d := range days {
		start := first.AddDate(0, 0, d)
		rows, err := read.HoldingsReport(context.Background(), []vo.Id{vo.MustParseId(acctA)}, start, start.AddDate(0, 0, 1))
		if err != nil {
			t.Fatalf("HoldingsReport: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("%s: %d holdings rows, want 1", start.Format("2006-01-02"), len(rows))
		}
		if got := vo.NewDecimal(rows[0].ToHoldings); got.String() != toHoldings[d].String() {
			t.Errorf("%s to holdings = %q, want %s", start.Format("2006-01-02"), rows[0].ToHoldings, toHoldings[d])
		}
		if got := vo.NewDecimal(rows[0].FromHoldings); got.String() != fromHoldings[d].String() {
			t.Errorf("%s from holdings = %q, want %s", start.Format("2006-01-02"), rows[0].FromHoldings, fromHoldings[d])
		}
	}
}
