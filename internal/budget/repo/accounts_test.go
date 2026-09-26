package repo_test

import (
	"context"
	"testing"
	"time"

	budgetrepo "github.com/econumo/econumo/internal/budget/repo"
	"github.com/econumo/econumo/internal/model"
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
		if err := r.AddAccount(ctx, b, a, false, t0.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	// re-adding a member keeps its original created_at
	if err := r.AddAccount(ctx, b, a2, false, t0.Add(time.Hour)); err != nil {
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

func TestBudgetAccounts_SavingsFlag(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	u := vo.NewId()
	f.User(fixture.User{ID: u.String(), Email: "u@e.test", Name: "U", Password: "pw", Salt: "s"})
	b, everyday, savings := vo.NewId(), vo.NewId(), vo.NewId()
	f.Account(fixture.Account{ID: everyday.String(), UserID: u.String()})
	f.Account(fixture.Account{ID: savings.String(), UserID: u.String()})
	f.Budget(fixture.Budget{ID: b.String(), UserID: u.String()})

	r := budgetrepo.NewRepo(db.Engine, db.TX)
	ctx := context.Background()
	t0 := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	if err := r.AddAccount(ctx, b, everyday, false, t0); err != nil {
		t.Fatal(err)
	}
	if err := r.AddAccount(ctx, b, savings, true, t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	flags := func() map[string]bool {
		t.Helper()
		got, err := r.MemberAccounts(ctx, b)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, m := range got {
			out[m.AccountID.String()] = m.IsSavings
		}
		return out
	}
	if got := flags(); len(got) != 2 || got[everyday.String()] || !got[savings.String()] {
		t.Fatalf("after AddAccount: %+v, want only the savings member flagged", got)
	}
	// re-adding an existing member leaves its flag alone
	if err := r.AddAccount(ctx, b, savings, false, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got := flags(); !got[savings.String()] {
		t.Fatalf("re-add cleared the flag: %+v", got)
	}
	if err := r.SetAccountSavings(ctx, b, savings, false); err != nil {
		t.Fatal(err)
	}
	if err := r.SetAccountSavings(ctx, b, everyday, true); err != nil {
		t.Fatal(err)
	}
	if got := flags(); !got[everyday.String()] || got[savings.String()] {
		t.Fatalf("after SetAccountSavings: %+v, want the flags swapped", got)
	}
}

func TestSavingsElementHasData(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	u := vo.NewId()
	f.User(fixture.User{ID: u.String(), Email: "has-data@e.test", Name: "U", Password: "pw", Salt: "s"})
	b, other, s, cat := vo.NewId(), vo.NewId(), vo.NewId(), vo.NewId()
	f.Account(fixture.Account{ID: s.String(), UserID: u.String()})
	f.Budget(fixture.Budget{ID: b.String(), UserID: u.String()})
	f.Budget(fixture.Budget{ID: other.String(), UserID: u.String()})
	el := f.BudgetElement(fixture.BudgetElement{BudgetID: b.String(), ExternalID: s.String(), Type: 5, Position: 0})
	otherEl := f.BudgetElement(fixture.BudgetElement{BudgetID: other.String(), ExternalID: s.String(), Type: 5, Position: 0})
	catEl := f.BudgetElement(fixture.BudgetElement{BudgetID: b.String(), ExternalID: cat.String(), Type: 1, Position: 1})
	f.BudgetLimit(fixture.BudgetLimit{ElementID: otherEl, Period: "2026-08-01 00:00:00", Amount: "10"})
	f.BudgetLimit(fixture.BudgetLimit{ElementID: catEl, Period: "2026-08-01 00:00:00", Amount: "10"})

	r := budgetrepo.NewRepo(db.Engine, db.TX)
	ctx := context.Background()
	has := func(budgetID, accountID vo.Id) bool {
		t.Helper()
		got, err := r.SavingsElementHasData(ctx, budgetID, accountID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if has(b, s) {
		t.Fatal("an empty savings element reports data (another budget's limit leaked in?)")
	}
	if has(b, cat) {
		t.Fatal("a category element's limit counts as savings data")
	}
	if has(b, vo.NewId()) {
		t.Fatal("an account without an element reports data")
	}

	elID, _ := vo.ParseId(el)
	now := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	limit := model.NewBudgetElementLimit(vo.NewId(), elID, vo.NewDecimal("5"), time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), now)
	if err := r.SaveLimit(ctx, limit); err != nil {
		t.Fatal(err)
	}
	if !has(b, s) {
		t.Fatal("a limit on the savings element is not reported")
	}
	if err := r.DeleteLimit(ctx, limit.ID); err != nil {
		t.Fatal(err)
	}
	c, err := model.NewBudgetElementComment(vo.NewId(), elID, u, "note", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.InsertComment(ctx, c); err != nil {
		t.Fatal(err)
	}
	if !has(b, s) {
		t.Fatal("a comment on the savings element is not reported")
	}
}
