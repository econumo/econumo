package repo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	usdID = "dffc2a06-6f29-4704-8575-31709adee926"
	userA = "11111111-1111-1111-1111-111111111111"
	userB = "22222222-2222-2222-2222-222222222222"
	tagA1 = "ca700000-0000-0000-0000-0000000000a1"
	tagA2 = "ca700000-0000-0000-0000-0000000000a2"
	tagB1 = "ca700000-0000-0000-0000-0000000000b1"
)

var fixedTime = time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)

func seedUser(t *testing.T, f *fixture.Builder, id string) {
	t.Helper()
	f.User(fixture.User{ID: id, Name: "u"})
}

func newRepo(t *testing.T) (*tagrepo.Repo, *tagrepo.ReadRepo, *dbtest.DB, *fixture.Builder) {
	t.Helper()
	db := dbtest.New(t)
	return tagrepo.NewRepo(db.Engine, db.TX), tagrepo.NewReadRepo(db.Engine, db.TX), db, fixture.New(t, db)
}

func tag(id, userID, name string, pos int16) *model.Tag {
	return &model.Tag{ID: vo.MustParseId(id), UserID: vo.MustParseId(userID), Name: name, Position: pos,
		IsArchived: false, CreatedAt: fixedTime, UpdatedAt: fixedTime}
}

func TestTagRepo_SaveGetRoundTrip(t *testing.T) {
	repo, _, _, f := newRepo(t)
	ctx := context.Background()
	seedUser(t, f, userA)
	if err := repo.Save(ctx, tag(tagA1, userA, "Holiday", 4)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.GetByID(ctx, vo.MustParseId(tagA1))
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Holiday" || got.Position != 4 || got.IsArchived {
		t.Errorf("mismatch: name=%q pos=%d archived=%v", got.Name, got.Position, got.IsArchived)
	}
	if !got.UpdatedAt.Equal(fixedTime) {
		t.Errorf("updatedAt mismatch: %v", got.UpdatedAt)
	}
}

func TestTagRepo_GetByID_NotFound(t *testing.T) {
	repo, _, _, f := newRepo(t)
	seedUser(t, f, userA)
	_, err := repo.GetByID(context.Background(), vo.NewId())
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFoundError, got %v", err)
	}
}

func TestTagRepo_ListAndCountByOwner(t *testing.T) {
	repo, _, _, f := newRepo(t)
	ctx := context.Background()
	seedUser(t, f, userA)
	seedUser(t, f, userB)
	_ = repo.Save(ctx, tag(tagA1, userA, "A1", 1))
	_ = repo.Save(ctx, tag(tagA2, userA, "A2", 0))
	_ = repo.Save(ctx, tag(tagB1, userB, "B1", 0))

	list, err := repo.ListByOwner(ctx, vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("ListByOwner: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2, got %d", len(list))
	}
	if list[0].ID.String() != tagA2 || list[1].ID.String() != tagA1 {
		t.Errorf("order by position wrong: %s, %s", list[0].ID, list[1].ID)
	}
	n, err := repo.CountByOwner(ctx, vo.MustParseId(userA))
	if err != nil || n != 2 {
		t.Errorf("CountByOwner = %d, %v; want 2", n, err)
	}
}

func TestTagRepo_Delete(t *testing.T) {
	repo, _, _, f := newRepo(t)
	ctx := context.Background()
	seedUser(t, f, userA)
	_ = repo.Save(ctx, tag(tagA1, userA, "A1", 0))
	if err := repo.Delete(ctx, vo.MustParseId(tagA1)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := repo.GetByID(ctx, vo.MustParseId(tagA1))
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFound after delete, got %v", err)
	}
}

func TestTagRepo_NextIdentity(t *testing.T) {
	repo, _, _, _ := newRepo(t)
	a := repo.NextIdentity()
	b := repo.NextIdentity()
	if a.String() == "" || b.String() == "" {
		t.Fatal("NextIdentity returned an empty id")
	}
	if a == b {
		t.Error("successive NextIdentity calls should not collide")
	}
}

func TestTagReadRepo_OwnPlusShared(t *testing.T) {
	repo, read, _, f := newRepo(t)
	ctx := context.Background()
	seedUser(t, f, userA)
	seedUser(t, f, userB)
	_ = repo.Save(ctx, tag(tagA1, userA, "A1", 0))
	_ = repo.Save(ctx, tag(tagB1, userB, "B1", 0))

	own, err := read.TagListView(ctx, userA)
	if err != nil {
		t.Fatalf("TagListView: %v", err)
	}
	if len(own) != 1 || own[0].ID != tagA1 {
		t.Fatalf("want only own A1, got %+v", own)
	}
	if own[0].CreatedAt != "2024-04-01 12:00:00" {
		t.Errorf("datetime format wrong: %q", own[0].CreatedAt)
	}

	f.Account(fixture.Account{ID: "acc00000-0000-0000-0000-0000000000b1", UserID: userB, CurrencyID: usdID, Name: "Shared", Type: 2, Icon: "x"})
	f.AccountAccess("acc00000-0000-0000-0000-0000000000b1", userA, 1)

	shared, err := read.TagListView(ctx, userA)
	if err != nil {
		t.Fatalf("TagListView shared: %v", err)
	}
	if len(shared) != 2 {
		t.Fatalf("want own + shared (2), got %d", len(shared))
	}
}

func TestTagRepo_UsageCounts(t *testing.T) {
	repo, _, _, f := newRepo(t)
	ctx := context.Background()
	seedUser(t, f, userA)
	_ = repo.Save(ctx, tag(tagA1, userA, "X", 0))
	_ = repo.Save(ctx, tag(tagA2, userA, "Y", 1))
	tagZ := "ca700000-0000-0000-0000-0000000000a3" // never used
	_ = repo.Save(ctx, tag(tagZ, userA, "Z", 2))

	f.Account(fixture.Account{ID: "acc00000-0000-0000-0000-0000000000c1", UserID: userA, CurrencyID: usdID, Name: "C"})
	seed := func(id, tagID, spent string) {
		f.Transaction(fixture.Transaction{ID: id, UserID: userA, AccountID: "acc00000-0000-0000-0000-0000000000c1",
			TagID: tagID, Type: 0, Amount: "1.00000000", SpentAt: spent})
	}
	seed("7c000000-0000-0000-0000-000000000010", tagA1, "2026-06-01 10:00:00")
	seed("7c000000-0000-0000-0000-000000000011", tagA1, "2026-06-02 10:00:00")
	seed("7c000000-0000-0000-0000-000000000012", tagA1, "2026-06-03 10:00:00")
	seed("7c000000-0000-0000-0000-000000000013", tagA2, "2026-06-01 10:00:00")
	seed("7c000000-0000-0000-0000-000000000014", tagA2, "2025-01-01 10:00:00") // outside the window

	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	counts, err := repo.UsageCounts(ctx, vo.MustParseId(userA), since)
	if err != nil {
		t.Fatalf("UsageCounts: %v", err)
	}
	if counts[tagA1] != 3 || counts[tagA2] != 1 {
		t.Errorf("counts = %v, want tagA1=3 tagA2=1", counts)
	}
	if _, ok := counts[tagZ]; ok {
		t.Errorf("unused tag must be absent from the map, got %v", counts)
	}
}
