package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
)

func importRouteStatus(t *testing.T, enabled bool, scope, method, path string) int {
	t.Helper()
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	rawToken := "eco_pat_transaction-import-test-token-000000000000"
	exp := time.Now().UTC().Add(24 * time.Hour)
	f.AccessToken(fixture.AccessToken{
		UserID:    userID,
		Kind:      "personal",
		Scope:     scope,
		TokenHash: appuser.HashAccessToken(rawToken),
		ExpiresAt: &exp,
	})

	cfg := baseTestConfig(db.Engine)
	cfg.TransactionImport = enabled
	ts := httptest.NewServer(server.BuildAPI(cfg, db.Raw, server.Seams{Avatars: appuser.FixedAvatarPicker(appuser.DefaultAvatar)}))
	t.Cleanup(ts.Close)

	req, err := http.NewRequest(method, ts.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+rawToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestTransactionImport_DisabledRoutesAreNotMounted(t *testing.T) {
	for _, tc := range []struct{ scope, method, path string }{
		{"full", http.MethodGet, "/api/v1/import/get-source-list"},
		{"full", http.MethodGet, "/api/v1/import/get-rule-list"},
		{"ingest", http.MethodPost, "/api/v1/import/ingest-apple-wallet-event"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			if got := importRouteStatus(t, false, tc.scope, tc.method, tc.path); got != http.StatusNotFound {
				t.Errorf("%s %s with transaction import off: status %d, want 404", tc.method, tc.path, got)
			}
		})
	}
}

func TestTransactionImport_EnabledRoutesAreMounted(t *testing.T) {
	if got := importRouteStatus(t, true, "full", http.MethodGet, "/api/v1/import/get-source-list"); got != http.StatusOK {
		t.Fatalf("get-source-list with transaction import on: status %d, want 200", got)
	}
}
