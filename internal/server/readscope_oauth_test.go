package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
)

// Read tokens follow the OAuth server: an instance whose ECONUMO_URL no longer
// enables it stops admitting the ones it already issued, on the real handler.
func TestReadTokenFollowsTheOAuthServer(t *testing.T) {
	const token = "eco_oat_read-token-followed-by-the-oauth-server-00000"
	for _, tc := range []struct {
		name, appURL string
		want         int
	}{
		{"oauth on", "https://app.example.test", http.StatusOK},
		{"oauth off, no url", "", http.StatusUnauthorized},
		{"oauth off, plain http", "http://app.example.test", http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := dbtest.New(t)
			f := fixture.New(t, db)
			user := f.User(fixture.User{Email: "a@example.test", Name: "Alex"})
			expires := time.Now().Add(time.Hour)
			f.AccessToken(fixture.AccessToken{UserID: user, Kind: model.TokenKindOAuth, TokenHash: appuser.HashAccessToken(token),
				Name: "Reports", Scope: string(model.TokenScopeRead), ExpiresAt: &expires})

			h, _, _, err := server.Build(config.Config{DatabaseDriver: db.Engine, CurrencyBase: "USD", AppURL: tc.appURL}, db.Raw, server.Seams{})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/currency/get-currency-list", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}
