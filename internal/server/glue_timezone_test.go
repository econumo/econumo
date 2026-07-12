package server_test

import (
	"context"
	"testing"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/authstub"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

func newTimezoneTestUserSvc(t *testing.T, db *dbtest.DB) *appuser.Service {
	t.Helper()
	enc := auth.NewEncodeService("")
	hasher := auth.NewPasswordHasher()
	repo := userrepo.NewRepo(db.Engine, db.TX)
	tokens := userrepo.NewAccessTokenRepo(db.Engine, db.TX)
	lookup := currencyrepo.New(db.Engine, db.TX)
	budgets := server.NewUserBudgetExistence(db.Engine, db.TX)
	return appuser.NewService(repo, db.TX, enc, hasher, tokens, lookup, budgets, nil, nil, appuser.FixedAvatarPicker(appuser.DefaultAvatar), clock.New(), nil, false)
}

func TestTimezoneTrackingAuthenticator(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	userSvc := newTimezoneTestUserSvc(t, db)
	authn := server.NewTimezoneTrackingAuthenticator(authstub.Authenticator{}, userSvc)

	loc, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	uid, err := vo.ParseId(userID)
	if err != nil {
		t.Fatalf("ParseId: %v", err)
	}

	ctx := reqctx.WithLocation(context.Background(), time.UTC)
	if _, _, err := authn.Authenticate(ctx, userID); err != nil {
		t.Fatal(err)
	}
	if tz, _ := userSvc.GetTimezone(context.Background(), uid); tz != "" {
		t.Fatalf("persisted %q for non-explicit request", tz)
	}

	ctx = reqctx.WithExplicitLocation(context.Background(), loc)
	if _, _, err := authn.Authenticate(ctx, userID); err != nil {
		t.Fatal(err)
	}
	if tz, _ := userSvc.GetTimezone(context.Background(), uid); tz != "Europe/Amsterdam" {
		t.Fatalf("timezone = %q, want Europe/Amsterdam", tz)
	}

	if _, _, err := authn.Authenticate(ctx, "not-a-user-id"); err == nil {
		t.Fatal("want auth error")
	}
}
