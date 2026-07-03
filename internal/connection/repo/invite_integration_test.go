package repo_test

// Regression-locks the datetime-bound expiry lookup: expired_at is stored as a
// bare "Y-m-d H:i:s" string and read via datetime(), so a lookup-by-code with
// `now` before expiry finds it and after expiry does not.

import (
	"context"
	"errors"
	"testing"
	"time"

	domconnection "github.com/econumo/econumo/internal/connection"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func newInviteRepo(t *testing.T) (*connectionrepo.InviteRepo, *dbtest.DB) {
	t.Helper()
	db := dbtest.NewSQLite(t)
	seedUser(t, fixture.New(t, db), userA)
	return connectionrepo.NewInviteRepo("sqlite", db.TX), db
}

func newInvite(userID vo.Id, code string, expiredAt *time.Time) *domconnection.ConnectionInvite {
	return &domconnection.ConnectionInvite{
		UserID: userID, Code: domconnection.ReconstituteConnectionCode(code), ExpiredAt: expiredAt,
	}
}

func TestInviteRepo_SaveAndGetByUser(t *testing.T) {
	repo, _ := newInviteRepo(t)
	ctx := context.Background()
	exp := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	inv := newInvite(vo.MustParseId(userA), "CODE1", &exp)
	if err := repo.Save(ctx, inv); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.GetByUser(ctx, vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("GetByUser: %v", err)
	}
	if got == nil {
		t.Fatal("expected invite, got nil")
	}
	if got.Code.Value() != "CODE1" {
		t.Errorf("code mismatch: %q", got.Code.Value())
	}
	if got.ExpiredAt == nil || !got.ExpiredAt.Equal(exp) {
		t.Errorf("expiredAt mismatch: %v", got.ExpiredAt)
	}
}

func TestInviteRepo_GetByUser_None(t *testing.T) {
	repo, _ := newInviteRepo(t)
	got, err := repo.GetByUser(context.Background(), vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("GetByUser: %v", err)
	}
	if got != nil {
		t.Errorf("want nil for user without invite, got %+v", got)
	}
}

func TestInviteRepo_GetByCode_ExpiryBoundary(t *testing.T) {
	repo, _ := newInviteRepo(t)
	ctx := context.Background()
	exp := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	inv := newInvite(vo.MustParseId(userA), "LIVEX", &exp)
	if err := repo.Save(ctx, inv); err != nil {
		t.Fatalf("Save: %v", err)
	}
	code, err := domconnection.NewConnectionCode("LIVEX")
	if err != nil {
		t.Fatalf("NewConnectionCode: %v", err)
	}

	// `now` BEFORE expiry -> found.
	before := exp.Add(-time.Hour)
	got, err := repo.GetByCode(ctx, code, before)
	if err != nil {
		t.Fatalf("GetByCode before expiry: %v", err)
	}
	if got == nil || got.UserID.String() != userA {
		t.Fatalf("want invite for userA before expiry, got %+v", got)
	}

	// `now` AFTER expiry -> NotFound.
	after := exp.Add(time.Hour)
	_, err = repo.GetByCode(ctx, code, after)
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFound after expiry, got %v", err)
	}
}

func TestInviteRepo_GetByCode_Missing(t *testing.T) {
	repo, _ := newInviteRepo(t)
	code, _ := domconnection.NewConnectionCode("NOPEX")
	_, err := repo.GetByCode(context.Background(), code, time.Now())
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFound for missing code, got %v", err)
	}
}

func TestInviteRepo_Save_Upsert(t *testing.T) {
	repo, _ := newInviteRepo(t)
	ctx := context.Background()
	exp1 := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	if err := repo.Save(ctx, newInvite(vo.MustParseId(userA), "FIRS1", &exp1)); err != nil {
		t.Fatalf("Save first: %v", err)
	}
	exp2 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := repo.Save(ctx, newInvite(vo.MustParseId(userA), "SECN2", &exp2)); err != nil {
		t.Fatalf("Save second: %v", err)
	}
	got, err := repo.GetByUser(ctx, vo.MustParseId(userA))
	if err != nil {
		t.Fatalf("GetByUser: %v", err)
	}
	if got.Code.Value() != "SECN2" || !got.ExpiredAt.Equal(exp2) {
		t.Errorf("upsert did not overwrite: code=%q exp=%v", got.Code.Value(), got.ExpiredAt)
	}
}
