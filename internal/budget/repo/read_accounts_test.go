package repo_test

import (
	"context"
	"testing"
	"time"

	budgetrepo "github.com/econumo/econumo/internal/budget/repo"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestAccountsWithTransactions(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	u := vo.NewId().String()
	f.User(fixture.User{ID: u, Email: "u@e.test", Name: "U", Password: "pw", Salt: "s"})
	src, dst, quiet, early := vo.NewId(), vo.NewId(), vo.NewId(), vo.NewId()
	for _, a := range []vo.Id{src, dst, quiet, early} {
		f.Account(fixture.Account{ID: a.String(), UserID: u})
	}
	f.Transaction(fixture.Transaction{UserID: u, AccountID: src.String(), Type: 2, Amount: "10", AccountRecipientID: dst.String(), AmountRecipient: "10", SpentAt: "2026-06-15 12:00:00"})
	f.Transaction(fixture.Transaction{UserID: u, AccountID: early.String(), Type: 0, Amount: "1", SpentAt: "2026-05-31 23:59:59"})
	f.Transaction(fixture.Transaction{UserID: u, AccountID: quiet.String(), Type: 0, Amount: "1", SpentAt: "2026-08-01 00:00:00"}) // == end, excluded

	r := budgetrepo.NewReadRepo(db.Engine, db.TX)
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	got, err := r.AccountsWithTransactions(context.Background(), []vo.Id{quiet, dst, src, early}, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[0].Equal(dst) || !got[1].Equal(src) {
		t.Fatalf("got %v want [dst src] (input order)", got)
	}
	if got, _ := r.AccountsWithTransactions(context.Background(), nil, start, end); got != nil {
		t.Fatal("empty input must return nil")
	}
}
