package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

// A transaction dated exactly at the cutoff is NOT before it, whether the row
// was stored as bare 'Y-m-d H:i:s' text (legacy rows) or bound as a time.Time.
// On SQLite the cutoff was bound as "2026-06-22 00:00:00 +0000 UTC", which the
// bare legacy text sorts below, so the cutoff row was wrongly counted.
func TestAccountRepo_Balance_ExcludesTransactionAtCutoff(t *testing.T) {
	repo, _, f := newAccountRepo(t)
	ctx := context.Background()

	seedUser(t, f, userA, "A")
	seedAccount(t, f, acctCash, userA, "Cash")

	f.Transaction(fixture.Transaction{
		ID: "d0000000-0000-0000-0000-0000000000b1", UserID: userA, AccountID: acctCash,
		Type: 1, Amount: "100.00", SpentAt: "2026-06-21 23:59:59",
	})
	f.Transaction(fixture.Transaction{
		ID: "d0000000-0000-0000-0000-0000000000b2", UserID: userA, AccountID: acctCash,
		Type: 1, Amount: "999.00", SpentAt: "2026-06-22 00:00:00", // == cutoff
	})

	before := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)

	bal, err := repo.Balance(ctx, vo.MustParseId(acctCash), before)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if got := vo.NewDecimal(bal).String(); got != "100" {
		t.Errorf("single-account balance = %q, want 100 (cutoff tx must be excluded)", got)
	}

	balances, err := repo.Balances(ctx, vo.MustParseId(userA), before)
	if err != nil {
		t.Fatalf("Balances: %v", err)
	}
	if got := balances[acctCash]; got != "100" {
		t.Errorf("list balance = %q, want 100 (cutoff tx must be excluded)", got)
	}
}
