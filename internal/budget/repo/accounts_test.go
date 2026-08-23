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

func TestBudgetAccounts_AddListRemoveAndRemoveByOwner(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	u1, u2 := vo.NewId(), vo.NewId()
	f.User(fixture.User{ID: u1.String(), Email: "u1@e.test", Name: "U1", Password: "pw", Salt: "s"})
	f.User(fixture.User{ID: u2.String(), Email: "u2@e.test", Name: "U2", Password: "pw", Salt: "s"})
	b, a1, a2, a3 := vo.NewId(), vo.NewId(), vo.NewId(), vo.NewId()
	f.Account(fixture.Account{ID: a1.String(), UserID: u1.String()})
	f.Account(fixture.Account{ID: a2.String(), UserID: u1.String()})
	f.Account(fixture.Account{ID: a3.String(), UserID: u2.String()})
	f.Budget(fixture.Budget{ID: b.String(), UserID: u1.String()})

	r := budgetrepo.NewRepo(db.Engine, db.TX)
	ctx := context.Background()
	t0 := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	for i, a := range []vo.Id{a2, a1, a3} {
		if err := r.AddAccount(ctx, b, a, t0.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	// re-adding a member keeps its original created_at
	if err := r.AddAccount(ctx, b, a2, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := r.MemberAccounts(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || !got[0].AccountID.Equal(a2) || !got[1].AccountID.Equal(a1) || !got[2].AccountID.Equal(a3) {
		t.Fatalf("order/len: %+v", got)
	}
	if !got[0].CreatedAt.Equal(t0) {
		t.Fatalf("created_at refreshed on re-add: %v", got[0].CreatedAt)
	}
	if err := r.RemoveAccountsOwnedBy(ctx, b, u2); err != nil {
		t.Fatal(err)
	}
	if got, _ = r.MemberAccounts(ctx, b); len(got) != 2 {
		t.Fatalf("after RemoveAccountsOwnedBy: %+v", got)
	}
	if err := r.RemoveAccount(ctx, b, a1); err != nil {
		t.Fatal(err)
	}
	if err := r.RemoveAccount(ctx, b, a1); err != nil { // non-member: no-op
		t.Fatal(err)
	}
	if got, _ = r.MemberAccounts(ctx, b); len(got) != 1 || !got[0].AccountID.Equal(a2) {
		t.Fatalf("after RemoveAccount: %+v", got)
	}
}
