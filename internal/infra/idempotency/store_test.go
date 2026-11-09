package idempotency_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/idempotency"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time { return c.t }

func newStore(t *testing.T) (*idempotency.Store, *dbtest.DB, *fakeClock) {
	t.Helper()
	db := dbtest.New(t)
	clock := &fakeClock{t: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)}
	return idempotency.NewStore(db.Engine, db.TX, clock), db, clock
}

func TestInsertThenFind(t *testing.T) {
	s, _, _ := newStore(t)
	ctx := context.Background()
	user := vo.NewId()
	resource := vo.NewId().String()

	if err := s.Insert(ctx, user, "key-1", resource); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	got, found, err := s.Find(ctx, user, "key-1")
	if err != nil || !found || got != resource {
		t.Fatalf("Find = %q, %v, %v; want %q, true, nil", got, found, err, resource)
	}

	if err := s.Insert(ctx, user, "key-2", ""); err != nil {
		t.Fatalf("Insert empty: %v", err)
	}
	got, found, err = s.Find(ctx, user, "key-2")
	if err != nil || !found || got != "" {
		t.Fatalf("Find empty = %q, %v, %v; want \"\", true, nil", got, found, err)
	}

	if _, found, err = s.Find(ctx, user, "missing"); err != nil || found {
		t.Fatalf("Find missing = %v, %v; want false, nil", found, err)
	}
}

func TestFindIsPerUser(t *testing.T) {
	s, _, _ := newStore(t)
	ctx := context.Background()
	a, b := vo.NewId(), vo.NewId()
	if err := s.Insert(ctx, a, "shared", ""); err != nil {
		t.Fatal(err)
	}
	if _, found, err := s.Find(ctx, b, "shared"); err != nil || found {
		t.Fatalf("other user's Find = %v, %v; want false, nil", found, err)
	}
}

func TestInsertDuplicateErrors(t *testing.T) {
	s, _, _ := newStore(t)
	ctx := context.Background()
	user := vo.NewId()
	if err := s.Insert(ctx, user, "dup", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Insert(ctx, user, "dup", ""); err == nil {
		t.Fatal("second Insert of the same key succeeded; want an error")
	}
}

func TestDelete(t *testing.T) {
	s, _, _ := newStore(t)
	ctx := context.Background()
	user := vo.NewId()
	if err := s.Insert(ctx, user, "gone", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, user, "gone"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, found, _ := s.Find(ctx, user, "gone"); found {
		t.Fatal("key still found after Delete")
	}
	if err := s.Delete(ctx, user, "gone"); err != nil {
		t.Fatalf("Delete of a missing key: %v", err)
	}
}

func TestMaybePurgeRemovesOnlyOlderThanSevenDays(t *testing.T) {
	s, _, clock := newStore(t)
	ctx := context.Background()
	user := vo.NewId()
	start := clock.t

	clock.t = start.Add(-8 * 24 * time.Hour)
	if err := s.Insert(ctx, user, "old", ""); err != nil {
		t.Fatal(err)
	}
	clock.t = start.Add(-6 * 24 * time.Hour)
	if err := s.Insert(ctx, user, "recent", ""); err != nil {
		t.Fatal(err)
	}

	clock.t = start
	s.MaybePurge(ctx)
	if _, found, _ := s.Find(ctx, user, "old"); found {
		t.Error("8-day-old key survived the purge")
	}
	if _, found, _ := s.Find(ctx, user, "recent"); !found {
		t.Error("6-day-old key was purged")
	}

	// A second call within 10 minutes deletes nothing, even for a row that is
	// already older than seven days; after 10 minutes it does.
	clock.t = start.Add(-8 * 24 * time.Hour)
	if err := s.Insert(ctx, user, "old2", ""); err != nil {
		t.Fatal(err)
	}
	clock.t = start.Add(5 * time.Minute)
	s.MaybePurge(ctx)
	if _, found, _ := s.Find(ctx, user, "old2"); !found {
		t.Error("purge ran again within 10 minutes")
	}
	clock.t = start.Add(11 * time.Minute)
	s.MaybePurge(ctx)
	if _, found, _ := s.Find(ctx, user, "old2"); found {
		t.Error("purge did not run after 10 minutes")
	}
}
