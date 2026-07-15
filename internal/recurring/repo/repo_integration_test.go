package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	recurringrepo "github.com/econumo/econumo/internal/recurring/repo"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	userA    = "0197c000-0000-7000-8000-00000000000a"
	accountA = "0197c000-0000-7000-8000-00000000000b"
	accountB = "0197c000-0000-7000-8000-00000000000c"
	rtA      = "0197c000-0000-7000-8000-00000000000d"
)

var fixedTime = time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

func newRepo(t *testing.T) (*recurringrepo.Repo, *fixture.Builder) {
	t.Helper()
	db := dbtest.New(t)
	return recurringrepo.NewRepo(db.Engine, db.TX), fixture.New(t, db)
}

func seed(t *testing.T, f *fixture.Builder) {
	t.Helper()
	f.User(fixture.User{ID: userA})
	f.Account(fixture.Account{ID: accountA, UserID: userA})
	f.Account(fixture.Account{ID: accountB, UserID: userA})
}

func template(id string) *model.RecurringTransaction {
	return model.NewRecurringTransaction(model.RecurringNewState{
		ID: vo.MustParseId(id), UserID: vo.MustParseId(userA),
		Type: model.TransactionTypeExpense, AccountID: vo.MustParseId(accountA),
		Amount: "50.5", Description: "rent",
		Schedule:      model.RecurringScheduleMonthly,
		NextPaymentAt: time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC),
		CreatedAt:     fixedTime,
	})
}

func TestRecurringRepo_SaveGetRoundTrip(t *testing.T) {
	repo, f := newRepo(t)
	ctx := context.Background()
	seed(t, f)

	if err := repo.Save(ctx, template(rtA)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.GetByID(ctx, vo.MustParseId(rtA))
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if vo.NewDecimal(got.Amount).String() != vo.NewDecimal("50.5").String() ||
		got.Schedule != model.RecurringScheduleMonthly || got.ScheduledDay != 31 {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if !got.NextPaymentAt.Equal(time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("NextPaymentAt = %s", got.NextPaymentAt)
	}
}

func TestRecurringRepo_UpsertUpdates(t *testing.T) {
	repo, f := newRepo(t)
	ctx := context.Background()
	seed(t, f)

	rt := template(rtA)
	if err := repo.Save(ctx, rt); err != nil {
		t.Fatalf("Save: %v", err)
	}
	rt.Advance(fixedTime.Add(time.Hour))
	if err := repo.Save(ctx, rt); err != nil {
		t.Fatalf("re-Save: %v", err)
	}
	got, _ := repo.GetByID(ctx, vo.MustParseId(rtA))
	if !got.NextPaymentAt.Equal(time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("NextPaymentAt after advance = %s, want 2026-08-31", got.NextPaymentAt)
	}
}

func TestRecurringRepo_ListByAccountIDs(t *testing.T) {
	repo, f := newRepo(t)
	ctx := context.Background()
	seed(t, f)

	rtB := "0197c000-0000-7000-8000-00000000000e"
	a := template(rtA)
	b := template(rtB)
	b.AccountID = vo.MustParseId(accountB)
	for _, rt := range []*model.RecurringTransaction{a, b} {
		if err := repo.Save(ctx, rt); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	got, err := repo.ListByAccountIDs(ctx, []vo.Id{vo.MustParseId(accountA)})
	if err != nil || len(got) != 1 || got[0].ID.String() != rtA {
		t.Fatalf("ListByAccountIDs(accountA) = %v items, err %v", len(got), err)
	}
	both, _ := repo.ListByAccountIDs(ctx, []vo.Id{vo.MustParseId(accountA), vo.MustParseId(accountB)})
	if len(both) != 2 {
		t.Fatalf("ListByAccountIDs(both) = %d items, want 2", len(both))
	}
	none, err := repo.ListByAccountIDs(ctx, nil)
	if err != nil || len(none) != 0 {
		t.Fatalf("empty id list must return empty slice, no error")
	}
}

func TestRecurringRepo_Delete_AndGetMissing(t *testing.T) {
	repo, f := newRepo(t)
	ctx := context.Background()
	seed(t, f)

	if err := repo.Save(ctx, template(rtA)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := repo.Delete(ctx, vo.MustParseId(rtA)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(ctx, vo.MustParseId(rtA)); err == nil {
		t.Fatal("GetByID after delete must return not-found")
	}
}
