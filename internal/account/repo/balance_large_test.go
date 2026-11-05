package repo_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

// From 2^26 up a float64 holds no exact cent, so a balance must come from the
// float's shortest decimal form, not its binary value: 123456789.01 printed at
// 8 places is 123456789.01000001. PostgreSQL's NUMERIC is exact, so both engines
// must return the same text.
func TestAccountRepo_Balance_AboveFloatCentPrecision(t *testing.T) {
	cases := []struct {
		name    string
		amounts []string
		want    string
	}{
		{"single amount", []string{"123456789.01"}, "123456789.01"},
		{"just above 2^26", []string{"67108864.07"}, "67108864.07"},
		{"fractional sum", []string{"1000000000.07", "876543210.39"}, "1876543210.46"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, _, f := newAccountRepo(t)
			ctx := context.Background()

			seedUser(t, f, userA, "A")
			seedAccount(t, f, acctCash, userA, "Cash")
			for i, amount := range tc.amounts {
				f.Transaction(fixture.Transaction{
					ID: fmt.Sprintf("d0000000-0000-0000-0000-0000000001%02d", i), UserID: userA, AccountID: acctCash,
					Type: 1, Amount: amount, SpentAt: "2026-06-21 12:00:00",
				})
			}
			before := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)

			bal, err := repo.Balance(ctx, vo.MustParseId(acctCash), before)
			if err != nil {
				t.Fatalf("Balance: %v", err)
			}
			if got := vo.NewDecimal(bal).String(); got != tc.want {
				t.Errorf("single-account balance = %q, want %q", got, tc.want)
			}

			balances, err := repo.Balances(ctx, vo.MustParseId(userA), before)
			if err != nil {
				t.Fatalf("Balances: %v", err)
			}
			if got := balances[acctCash]; got != tc.want {
				t.Errorf("list balance = %q, want %q", got, tc.want)
			}
		})
	}
}
