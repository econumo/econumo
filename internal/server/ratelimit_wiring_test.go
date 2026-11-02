package server_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
)

func TestGlobalRateCap_ExemptsOnlyEndpointsThatStayInsideTheAccount(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	rawToken := "eco_ses_ratelimit-wiring-token"
	exp := time.Now().UTC().Add(24 * time.Hour)
	f.AccessToken(fixture.AccessToken{
		UserID:    userID,
		Kind:      "session",
		TokenHash: appuser.HashAccessToken(rawToken),
		UserAgent: "ratelimit-test",
		ExpiresAt: &exp,
	})

	const perKey = 100
	handler := server.BuildAPI(config.Config{
		DatabaseDriver:              db.Engine,
		CurrencyBase:                "USD",
		AllowRegistration:           true,
		ImportSimpleFIN:             true,
		RateLimitLogin:              perKey,
		RateLimitReset:              perKey,
		RateLimitRemind:             perKey,
		RateLimitRegister:           perKey,
		RateLimitRequestEmailChange: perKey,
		RateLimitConfirmEmailChange: perKey,
		RateLimitAccept:             perKey,
		RateLimitIngest:             perKey,
		RateLimitClaimSetupToken:    perKey,
		RateLimitSync:               perKey,
		RateLimitPreviewRule:        perKey,
		RateLimitWindow:             15 * time.Minute,
		RateLimitGlobal:             1,
	}, db.Raw, server.Seams{Avatars: appuser.FixedAvatarPicker(appuser.DefaultAvatar)})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	post := func(t *testing.T, path, token, body string) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, ts.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	exempt := []struct{ name, path, body string }{
		{"ingest", "/api/v1/import/ingest-apple-wallet-event", `{"account":"x","amount":"1","currency":"USD"}`},
		{"preview-rule", "/api/v1/import/preview-rule", `{"action":"skip","matchField":"external_payee","matchType":"contains","matchValue":"Blue Bottle","scope":"all"}`},
	}
	for _, tc := range exempt {
		t.Run("exempt/"+tc.name, func(t *testing.T) {
			post(t, tc.path, rawToken, tc.body)
			if got := post(t, tc.path, rawToken, tc.body); got == http.StatusTooManyRequests {
				t.Fatalf("second call = 429, want it past the global cap")
			}
		})
	}

	// Each of these reaches past the caller's own account, so many accounts
	// together must still hit one ceiling.
	capped := []struct{ name, path, token, bodyFormat string }{
		{"login", "/api/v1/user/login-user", "", `{"username":"anon%d@example.test","password":"wrong"}`},
		{"remind", "/api/v1/user/remind-password", "", `{"username":"anon%d@example.test"}`},
		{"register", "/api/v1/user/register-user", "", `{"email":"anon%d@example.test","password":"long-enough-password","name":"Anon"}`},
		{"reset", "/api/v1/user/reset-password", "", `{"username":"anon%d@example.test","code":"00000000-0000-0000-0000-000000000000","password":"irrelevant-pw"}`},
		{"accept-invite", "/api/v1/connection/accept-invite", rawToken, `{"code":"00000%d"}`},
		{"request-email-change", "/api/v1/user/request-email-change", rawToken, `{"newEmail":"new%d@example.test","password":"wrong-password"}`},
		{"claim-setup-token", "/api/v1/import/claim-setup-token", rawToken, `{"setupToken":"aGVsbG8%d"}`},
		{"sync-source", "/api/v1/import/sync-source", rawToken, `{"sourceId":"0190a000-0000-7000-8000-00000000000%d","accessUrl":"https://bridge.invalid/simplefin","startDate":"2026-10-01"}`},
	}
	for _, tc := range capped {
		t.Run("capped/"+tc.name, func(t *testing.T) {
			post(t, tc.path, tc.token, fmt.Sprintf(tc.bodyFormat, 1))
			if got := post(t, tc.path, tc.token, fmt.Sprintf(tc.bodyFormat, 2)); got != http.StatusTooManyRequests {
				t.Fatalf("second call = %d, want 429 from the global cap", got)
			}
		})
	}
}
