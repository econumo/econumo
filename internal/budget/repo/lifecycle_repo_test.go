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

func TestBudgetRepo_EndedAtAndArchivedRoundTrip(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	u := vo.NewId()
	f.User(fixture.User{ID: u.String(), Email: "u@e.test", Name: "U", Password: "pw", Salt: "s"})
	id := vo.NewId()
	f.Budget(fixture.Budget{ID: id.String(), UserID: u.String()})

	r := budgetrepo.NewRepo(db.Engine, db.TX)
	ctx := context.Background()
	now := time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC)

	b, err := r.GetByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if b.EndedAt != nil || b.IsArchived {
		t.Fatalf("fresh budget: endedAt=%v isArchived=%v", b.EndedAt, b.IsArchived)
	}

	end := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if err := b.EndAt(&end, now); err != nil {
		t.Fatal(err)
	}
	b.Archive(now)
	if err := r.Save(ctx, b); err != nil {
		t.Fatal(err)
	}

	got, err := r.GetByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.EndedAt == nil || !got.EndedAt.Equal(end) || !got.IsArchived {
		t.Fatalf("round-trip: endedAt=%v isArchived=%v", got.EndedAt, got.IsArchived)
	}

	if err := got.EndAt(nil, now); err != nil {
		t.Fatal(err)
	}
	got.Unarchive(now)
	if err := r.Save(ctx, got); err != nil {
		t.Fatal(err)
	}
	back, err := r.GetByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if back.EndedAt != nil || back.IsArchived {
		t.Fatalf("clear round-trip: endedAt=%v isArchived=%v", back.EndedAt, back.IsArchived)
	}
}
