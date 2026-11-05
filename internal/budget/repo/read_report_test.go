package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

const acctOther = "aaaa1111-0000-0000-0000-0000000000a2"

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
