package user_test

import (
	"context"
	"testing"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

// trialNow is fixed so the assertion cannot straddle a month boundary between
// the service's clock and the test's own time.Now().
var trialNow = time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)

type trialClock struct{}

func (trialClock) Now() time.Time { return trialNow }

func newTrialSvc(t *testing.T, db *dbtest.DB, trial string) (*appuser.Service, *userrepo.Repo, *auth.EncodeService) {
	t.Helper()
	enc := auth.NewEncodeService(testSalt)
	hasher := auth.NewPasswordHasher()
	repo := userrepo.NewRepo(db.Engine, db.TX)
	tokens := userrepo.NewAccessTokenRepo(db.Engine, db.TX)
	lookup := currencyrepo.New(db.Engine, db.TX)
	budgets := server.NewUserBudgetAccess(db.Engine, db.TX)
	svc := appuser.NewService(repo, db.TX, enc, hasher, tokens, lookup, budgets, nil, nil,
		appuser.FixedAvatarPicker(appuser.DefaultAvatar), trialClock{}, nil, true, trial)
	return svc, repo, enc
}

func TestRegister_GrantsTrialWhenEnabled(t *testing.T) {
	db := dbtest.New(t)
	svc, repo, enc := newTrialSvc(t, db, "end-of-next-month")
	ctx := context.Background()

	if _, err := svc.Register(ctx, model.RegisterRequest{
		Name: "Trial User", Email: "trial@econumo.test", Password: "secretpass",
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	u, err := repo.GetByIdentifier(ctx, enc.Hash("trial@econumo.test"))
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if u.AccessLevel != model.AccessLevelFull {
		t.Fatalf("level: got %q want full", u.AccessLevel)
	}
	if u.AccessUntil == nil {
		t.Fatal("access_until: got nil, want the start of the month after next")
	}
	want := model.TrialEnd(trialNow) // 2026-09-01 00:00:00 UTC
	if !u.AccessUntil.Equal(want) {
		t.Fatalf("access_until: got %v want %v", *u.AccessUntil, want)
	}
}

func TestRegister_NoTrialByDefault(t *testing.T) {
	db := dbtest.New(t)
	svc, repo, enc := newTrialSvc(t, db, "none")
	ctx := context.Background()

	if _, err := svc.Register(ctx, model.RegisterRequest{
		Name: "Plain User", Email: "plain@econumo.test", Password: "secretpass",
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	u, err := repo.GetByIdentifier(ctx, enc.Hash("plain@econumo.test"))
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if u.AccessUntil != nil {
		t.Fatalf("access_until: got %v want nil", *u.AccessUntil)
	}
}

func TestAdminCreateUser_NeverGrantsTrial(t *testing.T) {
	db := dbtest.New(t)
	svc, repo, enc := newTrialSvc(t, db, "end-of-next-month")
	ctx := context.Background()

	if _, err := svc.AdminCreateUser(ctx, "Ops User", "ops@econumo.test", "secretpass"); err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}

	u, err := repo.GetByIdentifier(ctx, enc.Hash("ops@econumo.test"))
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if u.AccessUntil != nil {
		t.Fatalf("access_until: got %v want nil (operator grants are not trials)", *u.AccessUntil)
	}
}
