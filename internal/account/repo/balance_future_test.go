package repo_test

// Research/confirmation test for a reported bug: an account's balance should be
// the balance as of END OF TODAY and must EXCLUDE future-dated transactions.
// The repo is called with before = start of tomorrow and the SQL filters
// spent_at < before, so a transaction dated next week must not count. This
// inserts a today transaction and a clearly-future one and checks the balance
// excludes the future amount.
//
// (Repo integration tests are sqlite-only by design; the PostgreSQL adapter for
// the same balance path is exercised by the enginecompare suite.)

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestAccountRepo_Balance_ExcludesFutureTransactions(t *testing.T) {
	repo, _, f := newAccountRepo(t)
	ctx := context.Background()

	seedUser(t, f, userA, "A")
	seedAccount(t, f, acctCash, userA, "Cash")

	// "Today" is 2026-06-21; the cutoff is start of tomorrow (2026-06-22 00:00).
	f.Transaction(fixture.Transaction{
		ID: "d0000000-0000-0000-0000-0000000000a1", UserID: userA, AccountID: acctCash,
		Type: 1, Amount: "100.00", SpentAt: "2026-06-21 12:00:00", // today income
	})
	f.Transaction(fixture.Transaction{
		ID: "d0000000-0000-0000-0000-0000000000a2", UserID: userA, AccountID: acctCash,
		Type: 1, Amount: "999.00", SpentAt: "2026-06-28 12:00:00", // a week in the FUTURE
	})

	before := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC) // start of tomorrow

	bal, err := repo.Balance(ctx, vo.MustParseId(acctCash), before)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if got := vo.NewDecimal(bal).String(); got != "100" {
		t.Errorf("single-account balance = %q, want 100 (future tx must be excluded); raw=%q", got, bal)
	}

	balances, err := repo.Balances(ctx, vo.MustParseId(userA), before)
	if err != nil {
		t.Fatalf("Balances: %v", err)
	}
	if got := balances[acctCash]; got != "100" {
		t.Errorf("list balance = %q, want 100 (future tx must be excluded)", got)
	}
}
