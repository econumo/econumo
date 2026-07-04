package repo_test

import (
	"context"
	"errors"
	"testing"

	accountrepo "github.com/econumo/econumo/internal/account/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const (
	folder1 = "ffffffff-0000-0000-0000-00000000f001"
	folder2 = "ffffffff-0000-0000-0000-00000000f002"
)

func newFolderRepo(t *testing.T) (*accountrepo.FolderRepo, *dbtest.DB, *fixture.Builder) {
	t.Helper()
	db := dbtest.New(t)
	return accountrepo.NewFolderRepo(db.Engine, db.TX), db, fixture.New(t, db)
}

// newTestFolder builds a visible fixedTime-stamped folder for userA.
func newTestFolder(id vo.Id, name string, position int16) *model.Folder {
	return &model.Folder{
		ID: id, UserID: vo.MustParseId(userA), Name: name, Position: position,
		IsVisible: true, CreatedAt: fixedTime, UpdatedAt: fixedTime,
	}
}

func TestFolderRepo_SaveGetRoundTrip(t *testing.T) {
	repo, _, fx := newFolderRepo(t)
	ctx := context.Background()
	seedUser(t, fx, userA, "A")

	id := vo.MustParseId(folder1)
	f := newTestFolder(id, "Main", 5)
	if err := repo.Save(ctx, f); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Main" || got.Position != 5 || !got.IsVisible {
		t.Errorf("fields mismatch: name=%q pos=%d visible=%v", got.Name, got.Position, got.IsVisible)
	}
	if got.UserID.String() != userA {
		t.Errorf("user mismatch: %s", got.UserID)
	}
	if !got.CreatedAt.Equal(fixedTime) {
		t.Errorf("createdAt mismatch: %v", got.CreatedAt)
	}
}

func TestFolderRepo_GetByID_NotFound(t *testing.T) {
	repo, _, fx := newFolderRepo(t)
	seedUser(t, fx, userA, "A")
	_, err := repo.GetByID(context.Background(), vo.NewId())
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFoundError, got %v", err)
	}
}

func TestFolderRepo_ListAndCountByUser(t *testing.T) {
	repo, _, fx := newFolderRepo(t)
	ctx := context.Background()
	seedUser(t, fx, userA, "A")
	for _, id := range []string{folder1, folder2} {
		f := newTestFolder(vo.MustParseId(id), "F", 0)
		if err := repo.Save(ctx, f); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
	}
	list, err := repo.ListByUser(ctx, vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 folders, got %d", len(list))
	}
	n, err := repo.CountByUser(ctx, vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("CountByUser: %v", err)
	}
	if n != 2 {
		t.Errorf("want count 2, got %d", n)
	}
}

func TestFolderRepo_Delete(t *testing.T) {
	repo, _, fx := newFolderRepo(t)
	ctx := context.Background()
	seedUser(t, fx, userA, "A")
	id := vo.MustParseId(folder1)
	f := newTestFolder(id, "Main", 0)
	if err := repo.Save(ctx, f); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := repo.Delete(ctx, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := repo.GetByID(ctx, id)
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFoundError after delete, got %v", err)
	}
}

func TestFolderRepo_Membership(t *testing.T) {
	repo, _, fx := newFolderRepo(t)
	ctx := context.Background()
	seedUser(t, fx, userA, "A")
	seedAccount(t, fx, acctCash, userA, "Cash")
	seedAccount(t, fx, acctBank, userA, "Bank")
	f := newTestFolder(vo.MustParseId(folder1), "Main", 0)
	if err := repo.Save(ctx, f); err != nil {
		t.Fatalf("Save folder: %v", err)
	}

	if err := repo.AddAccount(ctx, vo.MustParseId(folder1), vo.MustParseId(acctCash)); err != nil {
		t.Fatalf("AddAccount: %v", err)
	}
	// Idempotent re-add.
	if err := repo.AddAccount(ctx, vo.MustParseId(folder1), vo.MustParseId(acctCash)); err != nil {
		t.Fatalf("AddAccount idempotent: %v", err)
	}
	if err := repo.AddAccount(ctx, vo.MustParseId(folder1), vo.MustParseId(acctBank)); err != nil {
		t.Fatalf("AddAccount bank: %v", err)
	}

	ids, err := repo.FolderAccountIDs(ctx, vo.MustParseId(folder1))
	if err != nil {
		t.Fatalf("FolderAccountIDs: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("want 2 accounts in folder, got %d", len(ids))
	}

	m, err := repo.MembershipsByUser(ctx, vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("MembershipsByUser: %v", err)
	}
	if len(m[folder1]) != 2 {
		t.Errorf("want 2 memberships for folder, got %d", len(m[folder1]))
	}

	if err := repo.RemoveAccount(ctx, vo.MustParseId(folder1), vo.MustParseId(acctCash)); err != nil {
		t.Fatalf("RemoveAccount: %v", err)
	}
	ids, _ = repo.FolderAccountIDs(ctx, vo.MustParseId(folder1))
	if len(ids) != 1 || ids[0] != acctBank {
		t.Errorf("want only bank after remove, got %v", ids)
	}
}
