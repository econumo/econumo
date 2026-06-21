package tagrepo_test

// Integration tests for the tag write Repo and ReadRepo (own + shared via
// accounts_access) against a real migrated in-memory SQLite.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	domtag "github.com/econumo/econumo/internal/domain/tag"
	tagrepo "github.com/econumo/econumo/internal/infra/repo/tag"
	"github.com/econumo/econumo/internal/testutil"
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

func seedUser(t *testing.T, db *testutil.DB, id string) {
	t.Helper()
	db.Exec(t, `INSERT INTO users (id, identifier, email, name, avatar_url, password, salt, created_at, updated_at, is_active) VALUES (?, ?, '', 'u', '', '', '', ?, ?, 1)`,
		id, id, fixedTime, fixedTime)
}

func newRepo(t *testing.T) (*tagrepo.Repo, *tagrepo.ReadRepo, *testutil.DB) {
	t.Helper()
	db := testutil.NewSQLite(t)
	return tagrepo.NewRepo("sqlite", db.TX), tagrepo.NewReadRepo("sqlite", db.TX), db
}

func tag(id, userID, name string, pos int16) *domtag.Tag {
	return domtag.FromState(vo.MustParseId(id), vo.MustParseId(userID), name, pos, false, fixedTime, fixedTime)
}

func TestTagRepo_SaveGetRoundTrip(t *testing.T) {
	repo, _, db := newRepo(t)
	ctx := context.Background()
	seedUser(t, db, userA)
	if err := repo.Save(ctx, tag(tagA1, userA, "Holiday", 4)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.GetByID(ctx, vo.MustParseId(tagA1))
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name() != "Holiday" || got.Position() != 4 || got.IsArchived() {
		t.Errorf("mismatch: name=%q pos=%d archived=%v", got.Name(), got.Position(), got.IsArchived())
	}
	if !got.UpdatedAt().Equal(fixedTime) {
		t.Errorf("updatedAt mismatch: %v", got.UpdatedAt())
	}
}

func TestTagRepo_GetByID_NotFound(t *testing.T) {
	repo, _, db := newRepo(t)
	seedUser(t, db, userA)
	_, err := repo.GetByID(context.Background(), vo.NewId())
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFoundError, got %v", err)
	}
}

func TestTagRepo_ListAndCountByOwner(t *testing.T) {
	repo, _, db := newRepo(t)
	ctx := context.Background()
	seedUser(t, db, userA)
	seedUser(t, db, userB)
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
	if list[0].Id().String() != tagA2 || list[1].Id().String() != tagA1 {
		t.Errorf("order by position wrong: %s, %s", list[0].Id(), list[1].Id())
	}
	n, err := repo.CountByOwner(ctx, vo.MustParseId(userA))
	if err != nil || n != 2 {
		t.Errorf("CountByOwner = %d, %v; want 2", n, err)
	}
}

func TestTagRepo_Delete(t *testing.T) {
	repo, _, db := newRepo(t)
	ctx := context.Background()
	seedUser(t, db, userA)
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

func TestTagReadRepo_OwnPlusShared(t *testing.T) {
	repo, read, db := newRepo(t)
	ctx := context.Background()
	seedUser(t, db, userA)
	seedUser(t, db, userB)
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

	db.Exec(t, `INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at) VALUES (?, ?, ?, 'Shared', 2, 'x', 0, ?, ?)`,
		"acc00000-0000-0000-0000-0000000000b1", usdID, userB, fixedTime, fixedTime)
	db.Exec(t, `INSERT INTO accounts_access (account_id, user_id, role, created_at, updated_at) VALUES (?, ?, 1, ?, ?)`,
		"acc00000-0000-0000-0000-0000000000b1", userA, fixedTime, fixedTime)

	shared, err := read.TagListView(ctx, userA)
	if err != nil {
		t.Fatalf("TagListView shared: %v", err)
	}
	if len(shared) != 2 {
		t.Fatalf("want own + shared (2), got %d", len(shared))
	}
}
