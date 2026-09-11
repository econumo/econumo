package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/clock"
	"github.com/econumo/econumo/internal/infra/ratelimit"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
	userrepo "github.com/econumo/econumo/internal/user/repo"
)

func TestBuildAPI_MountsOAuthRoutesAndProviderList(t *testing.T) {
	db := dbtest.NewSQLite(t)
	cfg := config.Config{DatabaseDriver: db.Engine, CurrencyBase: "USD", AllowRegistration: true,
		AppURL: "https://app.example.test", OAuthGoogleClientID: "g", OAuthGoogleClientSecret: "s",
		RateLimitWindow: 15 * time.Minute}
	srv := httptest.NewServer(BuildAPI(cfg, db.Raw, Seams{}))
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL + "/api/v1/oauth/get-provider-list")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(string(body), `{"id":"google","name":"Google"}`) {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
}

func TestBuildAPI_NoProvidersIsEmptyList(t *testing.T) {
	db := dbtest.NewSQLite(t)
	cfg := config.Config{DatabaseDriver: db.Engine, CurrencyBase: "USD", RateLimitWindow: 15 * time.Minute}
	srv := httptest.NewServer(BuildAPI(cfg, db.Raw, Seams{}))
	t.Cleanup(srv.Close)
	resp, _ := http.Get(srv.URL + "/api/v1/oauth/get-provider-list")
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"data":[]`) {
		t.Fatalf("%s", body)
	}
}

// TestOAuthUsers_FullyHydratesFromRealUserService is a deferred finding from
// Task 8: the last-identity-unlink guard depends on OAuthUsers.FindByID/
// FindByEmail returning the SAME hydration as the rest of the user feature
// (in particular users.Algorithm, which HasPassword() reads). A partial read
// path (e.g. swapping in a summary/read-model lookup) would silently make
// every OAuth-provisioned user look password-protected, which would let the
// unlink guard be bypassed for the account's only identity, or would break
// FindByEmail resolution outright. Build a real *appuser.Service the same way
// Build() does and assert both lookups round-trip a seeded passwordless user.
func TestOAuthUsers_FullyHydratesFromRealUserService(t *testing.T) {
	db := dbtest.NewSQLite(t)
	fx := fixture.New(t, db)
	userID := fx.User(fixture.User{Email: "oauth-user@example.test", Algorithm: "none"})

	txm := backend.NewTxManager(db.Raw)
	userRepo := userrepo.NewRepo(db.Engine, txm)
	accessTokens := userrepo.NewAccessTokenRepo(db.Engine, txm)
	authLimiter := ratelimit.New(ratelimit.Config{Window: 15 * time.Minute, Global: 60}, clock.New())
	userSvc := appuser.NewService(
		userRepo, txm, auth.NewEncodeService(""), auth.NewPasswordHasher(), accessTokens,
		nil, nil, nil, nil, nil, nil, nil, nil,
		appuser.FixedAvatarPicker(appuser.DefaultAvatar), clock.New(), authLimiter, true, 0, false,
	)

	oauthUsers := NewOAuthUsers(userSvc)
	ctx := context.Background()

	byID, err := oauthUsers.FindByID(ctx, vo.MustParseId(userID))
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if byID.HasPassword() {
		t.Fatalf("passwordless user hydrated with HasPassword()=true: %+v", byID)
	}

	byEmail, err := oauthUsers.FindByEmail(ctx, "oauth-user@example.test")
	if err != nil {
		t.Fatalf("FindByEmail: %v", err)
	}
	if byEmail.ID.Value() != userID {
		t.Fatalf("FindByEmail resolved id %q, want %q", byEmail.ID.Value(), userID)
	}
}
