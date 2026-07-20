package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/authstub"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/test/mcptest"
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
	budgets := NewUserBudgetAccess(db.Engine, db.TX)
	return appuser.NewService(repo, db.TX, enc, hasher, tokens, lookup, budgets, nil, nil, appuser.FixedAvatarPicker(appuser.DefaultAvatar), clock.New(), nil, false, "none")
}

func TestTimezoneTrackingAuthenticator(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	userSvc := newTimezoneTestUserSvc(t, db)
	authn := NewTimezoneTrackingAuthenticator(authstub.Authenticator{}, userSvc)

	loc, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	uid, err := vo.ParseId(userID)
	if err != nil {
		t.Fatalf("ParseId: %v", err)
	}

	ctx := reqctx.WithLocation(context.Background(), time.UTC)
	if _, _, _, err := authn.Authenticate(ctx, userID); err != nil {
		t.Fatal(err)
	}
	if tz, _ := userSvc.GetTimezone(context.Background(), uid); tz != "" {
		t.Fatalf("persisted %q for non-explicit request", tz)
	}

	ctx = reqctx.WithExplicitLocation(context.Background(), loc)
	if _, _, _, err := authn.Authenticate(ctx, userID); err != nil {
		t.Fatal(err)
	}
	if tz, _ := userSvc.GetTimezone(context.Background(), uid); tz != "Europe/Amsterdam" {
		t.Fatalf("timezone = %q, want Europe/Amsterdam", tz)
	}

	if _, _, _, err := authn.Authenticate(ctx, "not-a-user-id"); err == nil {
		t.Fatal("want auth error")
	}
}

// runTimezoneFallback runs the middleware over ctx and reports the timezone
// the wrapped handler observed via reqctx.Location.
func runTimezoneFallback(t *testing.T, users *appuser.Service, ctx context.Context) string {
	t.Helper()
	var got string
	h := timezoneFallback(users)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = reqctx.Location(r.Context()).String()
	}))
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil).WithContext(ctx)
	h.ServeHTTP(httptest.NewRecorder(), req)
	return got
}

// timezoneFallbackFixture builds a fresh DB per subtest: the fixture's
// no-crypto user identifier is derived from the id's leading bytes, which
// collide for UUIDv7s minted in quick succession, so sharing one DB across
// scenarios would hit spurious UNIQUE violations.
func timezoneFallbackFixture(t *testing.T) (*appuser.Service, *fixture.Builder) {
	t.Helper()
	db := dbtest.NewSQLite(t)
	return newTimezoneTestUserSvc(t, db), fixture.New(t, db)
}

func TestTimezoneFallback(t *testing.T) {
	t.Run("non-explicit uses stored timezone", func(t *testing.T) {
		userSvc, f := timezoneFallbackFixture(t)
		userID := f.User(fixture.User{})
		uid, err := vo.ParseId(userID)
		if err != nil {
			t.Fatal(err)
		}
		if err := userSvc.PersistTimezone(context.Background(), uid, "Europe/Amsterdam"); err != nil {
			t.Fatal(err)
		}
		ctx := mcptest.CtxWithUser(t, userID)
		if got := runTimezoneFallback(t, userSvc, ctx); got != "Europe/Amsterdam" {
			t.Fatalf("timezone = %q, want Europe/Amsterdam", got)
		}
	})

	t.Run("explicit header wins over stored timezone", func(t *testing.T) {
		userSvc, f := timezoneFallbackFixture(t)
		userID := f.User(fixture.User{})
		uid, err := vo.ParseId(userID)
		if err != nil {
			t.Fatal(err)
		}
		if err := userSvc.PersistTimezone(context.Background(), uid, "Europe/Amsterdam"); err != nil {
			t.Fatal(err)
		}
		loc, err := time.LoadLocation("America/New_York")
		if err != nil {
			t.Fatalf("LoadLocation: %v", err)
		}
		ctx := reqctx.WithExplicitLocation(mcptest.CtxWithUser(t, userID), loc)
		if got := runTimezoneFallback(t, userSvc, ctx); got != "America/New_York" {
			t.Fatalf("timezone = %q, want America/New_York", got)
		}
	})

	t.Run("no stored timezone falls back to UTC", func(t *testing.T) {
		userSvc, f := timezoneFallbackFixture(t)
		userID := f.User(fixture.User{})
		ctx := mcptest.CtxWithUser(t, userID)
		if got := runTimezoneFallback(t, userSvc, ctx); got != "UTC" {
			t.Fatalf("timezone = %q, want UTC", got)
		}
	})

	t.Run("no auth user in context defaults to UTC without panic", func(t *testing.T) {
		userSvc, _ := timezoneFallbackFixture(t)
		ctx := context.Background()
		if got := runTimezoneFallback(t, userSvc, ctx); got != "UTC" {
			t.Fatalf("timezone = %q, want UTC", got)
		}
	})
}
