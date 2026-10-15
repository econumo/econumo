package repo_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

// The count mirrors get-account-transaction-list: a transfer is in both its
// accounts' lists, so it counts on both sides.
func TestAccountRepo_TransactionsCounts(t *testing.T) {
	repo, _, f := newAccountRepo(t)
	ctx := context.Background()

	seedUser(t, f, userA, "A")
	seedUser(t, f, userB, "B")
	seedAccount(t, f, acctCash, userA, "Cash")
	seedAccount(t, f, acctBank, userA, "Bank")
	seedAccount(t, f, acctOther, userB, "Other")
	f.AccountAccess(acctOther, userA, 1)

	tx := func(id, account, recipient string) {
		f.Transaction(fixture.Transaction{
			ID: id, UserID: userA, AccountID: account, AccountRecipientID: recipient,
			Type: 0, Amount: "1", SpentAt: "2026-06-01 10:00:00",
		})
	}
	tx("d0000000-0000-0000-0000-0000000000c1", acctCash, "")
	tx("d0000000-0000-0000-0000-0000000000c2", acctCash, "")
	f.Transaction(fixture.Transaction{
		ID: "d0000000-0000-0000-0000-0000000000c3", UserID: userA, AccountID: acctCash, AccountRecipientID: acctOther,
		Type: 2, Amount: "5", AmountRecipient: "5", SpentAt: "2026-06-02 10:00:00",
	})

	counts, err := repo.TransactionsCounts(ctx, vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("TransactionsCounts: %v", err)
	}
	want := map[string]int64{acctCash: 3, acctBank: 0, acctOther: 1}
	if len(counts) != len(want) {
		t.Fatalf("counts = %v, want %v", counts, want)
	}
	for id, n := range want {
		if counts[id] != n {
			t.Errorf("counts[%s] = %d, want %d", id, counts[id], n)
		}
		one, err := repo.TransactionsCount(ctx, vo.MustParseId(id))
		if err != nil {
			t.Fatalf("TransactionsCount: %v", err)
		}
		if one != n {
			t.Errorf("TransactionsCount(%s) = %d, want %d", id, one, n)
		}
	}
}
