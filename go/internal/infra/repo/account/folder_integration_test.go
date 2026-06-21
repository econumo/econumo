package accountrepo_test

// Integration tests for the account FolderRepo: folder CRUD, NotFound, delete,
// per-user listing/count, and accounts_folders membership maps.

import (
	"context"
	"errors"
	"testing"

	domaccount "github.com/econumo/econumo/internal/domain/account"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	accountrepo "github.com/econumo/econumo/internal/infra/repo/account"
	"github.com/econumo/econumo/internal/test/dbtest"
)

const (
	folder1 = "ffffffff-0000-0000-0000-00000000f001"
	folder2 = "ffffffff-0000-0000-0000-00000000f002"
)

func newFolderRepo(t *testing.T) (*accountrepo.FolderRepo, *dbtest.DB) {
	t.Helper()
	db := dbtest.NewSQLite(t)
	return accountrepo.NewFolderRepo("sqlite", db.TX), db
}

func TestFolderRepo_SaveGetRoundTrip(t *testing.T) {
	repo, db := newFolderRepo(t)
	ctx := context.Background()
	seedUser(t, db, userA, "A")

	id := vo.MustParseId(folder1)
	f := domaccount.FolderFromState(id, vo.MustParseId(userA), "Main", 5, true, fixedTime, fixedTime)
	if err := repo.Save(ctx, f); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name() != "Main" || got.Position() != 5 || !got.IsVisible() {
		t.Errorf("fields mismatch: name=%q pos=%d visible=%v", got.Name(), got.Position(), got.IsVisible())
	}
	if got.UserId().String() != userA {
		t.Errorf("user mismatch: %s", got.UserId())
	}
	if !got.CreatedAt().Equal(fixedTime) {
		t.Errorf("createdAt mismatch: %v", got.CreatedAt())
	}
}

func TestFolderRepo_GetByID_NotFound(t *testing.T) {
	repo, db := newFolderRepo(t)
	seedUser(t, db, userA, "A")
	_, err := repo.GetByID(context.Background(), vo.NewId())
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFoundError, got %v", err)
	}
}

func TestFolderRepo_ListAndCountByUser(t *testing.T) {
	repo, db := newFolderRepo(t)
	ctx := context.Background()
	seedUser(t, db, userA, "A")
	for _, id := range []string{folder1, folder2} {
		f := domaccount.FolderFromState(vo.MustParseId(id), vo.MustParseId(userA), "F", 0, true, fixedTime, fixedTime)
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
	repo, db := newFolderRepo(t)
	ctx := context.Background()
	seedUser(t, db, userA, "A")
	id := vo.MustParseId(folder1)
	f := domaccount.FolderFromState(id, vo.MustParseId(userA), "Main", 0, true, fixedTime, fixedTime)
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
	repo, db := newFolderRepo(t)
	ctx := context.Background()
	seedUser(t, db, userA, "A")
	seedAccount(t, db, acctCash, userA, "Cash")
	seedAccount(t, db, acctBank, userA, "Bank")
	f := domaccount.FolderFromState(vo.MustParseId(folder1), vo.MustParseId(userA), "Main", 0, true, fixedTime, fixedTime)
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
