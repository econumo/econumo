package txorder_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/txorder"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestPlacerTop(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	u := f.User(fixture.User{})
	a := f.Account(fixture.Account{UserID: u})
	b := f.Account(fixture.Account{UserID: u})
	p := txorder.New(db.Engine, db.TX)
	ctx := context.Background()
	day := time.Date(2026, 10, 1, 15, 4, 5, 0, time.UTC)

	top := func(account string, at time.Time) int64 {
		t.Helper()
		k, err := p.Top(ctx, account, at)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	if k := top(a, day); k != 1 {
		t.Fatalf("empty day: %d, want 1", k)
	}
	f.Transaction(fixture.Transaction{UserID: u, AccountID: a, Type: 0, SpentAt: "2026-10-01 01:00:00", SortKey: 1})
	f.Transaction(fixture.Transaction{UserID: u, AccountID: a, Type: 0, SpentAt: "2026-10-01 23:59:59", SortKey: 3})
	f.Transaction(fixture.Transaction{UserID: u, AccountID: b, AccountRecipientID: a, Type: 2, SpentAt: "2026-10-01 00:00:00", SortKey: 9, RecipientSortKey: 5})
	f.Transaction(fixture.Transaction{UserID: u, AccountID: a, Type: 0, SpentAt: "2026-10-02 00:00:00", SortKey: 40})
	f.Transaction(fixture.Transaction{UserID: u, AccountID: a, Type: 0, SpentAt: "2026-09-30 23:59:59", SortKey: 50})
	if k := top(a, day); k != 6 {
		t.Errorf("a's day: %d, want 6 (max of its source keys 3 and recipient key 5, plus one)", k)
	}
	if k := top(b, day); k != 10 {
		t.Errorf("b's day: %d, want 10", k)
	}
}
