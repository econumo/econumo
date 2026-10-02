package repo_test

// Integration tests for AccountsNetByMonth. Regression-locks the sign rules
// (incoming transfers count amount_recipient, outgoing subtract amount, income
// adds, expense subtracts, every counterparty), cross-currency amounts staying
// in each account's own currency, the month-boundary datetime binding, and the
// empty-id-set short-circuit.

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestAccountsNetByMonth(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)

	eurID := f.Currency(fixture.Currency{ID: "eeee0000-0000-0000-0000-0000000000e2", Code: "EUR", Symbol: "€", Name: "Euro"})

	e1 := acctA // everyday, USD (already seeded by newReadRepo)
	e2 := "aaaa2222-0000-0000-0000-0000000000e2"
	s1 := "aaaa2222-0000-0000-0000-00000000005a"
	s2 := "aaaa2222-0000-0000-0000-00000000005b"
	x := "aaaa2222-0000-0000-0000-0000000000c1"
	f.Account(fixture.Account{ID: e2, UserID: userA, CurrencyID: usdID, Name: "E2", Type: 2})
	f.Account(fixture.Account{ID: s1, UserID: userA, CurrencyID: usdID, Name: "S1"})
	f.Account(fixture.Account{ID: s2, UserID: userA, CurrencyID: eurID, Name: "S2"})
	f.Account(fixture.Account{ID: x, UserID: userA, CurrencyID: usdID, Name: "X", Type: 2})

	// S1 March: +500 (in) -120 (out) = 380
	f.Transaction(fixture.Transaction{ID: "7d000000-0000-0000-0000-000000000001", UserID: userA, AccountID: e1, AccountRecipientID: s1, Type: 2, Amount: "500.00", AmountRecipient: "500.00", SpentAt: "2026-03-10 00:00:00"})
	f.Transaction(fixture.Transaction{ID: "7d000000-0000-0000-0000-000000000002", UserID: userA, AccountID: s1, AccountRecipientID: e2, Type: 2, Amount: "120.00", AmountRecipient: "120.00", SpentAt: "2026-03-20 00:00:00"})
	// S2 March: +100 EUR (uses amount_recipient), last instant of March
	f.Transaction(fixture.Transaction{ID: "7d000000-0000-0000-0000-000000000003", UserID: userA, AccountID: e1, AccountRecipientID: s2, Type: 2, Amount: "110.00", AmountRecipient: "100.00", SpentAt: "2026-03-31 23:59:59"})
	// S1 April: +200 (month boundary — first instant of April must be INCLUDED)
	f.Transaction(fixture.Transaction{ID: "7d000000-0000-0000-0000-000000000004", UserID: userA, AccountID: e1, AccountRecipientID: s1, Type: 2, Amount: "200.00", AmountRecipient: "200.00", SpentAt: "2026-04-01 00:00:00"})
	// savings <-> savings
	f.Transaction(fixture.Transaction{ID: "7d000000-0000-0000-0000-000000000005", UserID: userA, AccountID: s1, AccountRecipientID: s2, Type: 2, Amount: "50.00", AmountRecipient: "45.00", SpentAt: "2026-03-15 00:00:00"})
	// non-member -> savings
	f.Transaction(fixture.Transaction{ID: "7d000000-0000-0000-0000-000000000006", UserID: userA, AccountID: x, AccountRecipientID: s1, Type: 2, Amount: "70.00", AmountRecipient: "70.00", SpentAt: "2026-03-16 00:00:00"})
	// income/expense on S1
	f.Transaction(fixture.Transaction{ID: "7d000000-0000-0000-0000-000000000007", UserID: userA, AccountID: s1, Type: 1, Amount: "3.00", SpentAt: "2026-03-17 00:00:00"})
	f.Transaction(fixture.Transaction{ID: "7d000000-0000-0000-0000-000000000008", UserID: userA, AccountID: s1, Type: 0, Amount: "1.00", SpentAt: "2026-03-18 00:00:00"})

	mar := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	may := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	rows, err := read.AccountsNetByMonth(ctx, []vo.Id{vo.MustParseId(s1), vo.MustParseId(s2)}, mar, may)
	if err != nil {
		t.Fatalf("AccountsNetByMonth: %v", err)
	}
	type want struct {
		accountID, month, amount string
	}
	wants := []want{
		{s1, "2026-03-01", "402"}, // +500 -120 -50 +70 +3 -1
		{s2, "2026-03-01", "145"}, // +100 +45, amount_recipient in EUR
		{s1, "2026-04-01", "200"},
	}
	if len(rows) != len(wants) {
		t.Fatalf("want %d rows, got %d: %+v", len(wants), len(rows), rows)
	}
	for i, w := range wants {
		r := rows[i]
		if r.AccountID != w.accountID || r.Month != w.month {
			t.Fatalf("row %d = %+v, want account=%s month=%s", i, r, w.accountID, w.month)
		}
		if got := vo.NewDecimal(r.Amount).String(); got != vo.NewDecimal(w.amount).String() {
			t.Errorf("row %d amount = %s, want %s", i, got, w.amount)
		}
	}
}

func TestAccountsNetByMonth_EmptyIDSet(t *testing.T) {
	read, _ := newReadRepo(t)
	ctx := context.Background()
	mar := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	may := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	if rows, err := read.AccountsNetByMonth(ctx, nil, mar, may); err != nil || rows != nil {
		t.Errorf("AccountsNetByMonth empty accountIDs should be nil,nil; got %v, %v", rows, err)
	}
}
