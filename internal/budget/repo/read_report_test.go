package repo_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

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
