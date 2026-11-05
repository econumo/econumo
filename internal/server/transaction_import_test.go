package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
)

type importServer struct {
	t     *testing.T
	url   string
	f     *fixture.Builder
	user  string
	token string
}

func newImportServer(t *testing.T, simpleFIN bool) importServer {
	t.Helper()
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	exp := time.Now().UTC().Add(24 * time.Hour)
	full := "eco_pat_transaction-import-test-full-0000000000000"
	ingest := "eco_pat_transaction-import-test-ingest-00000000000"
	f.AccessToken(fixture.AccessToken{UserID: userID, Kind: "personal", TokenHash: appuser.HashAccessToken(full), ExpiresAt: &exp})
	f.AccessToken(fixture.AccessToken{UserID: userID, Kind: "personal", Scope: "ingest", TokenHash: appuser.HashAccessToken(ingest), ExpiresAt: &exp})

	cfg := baseTestConfig(db.Engine)
	cfg.ImportSimpleFIN = simpleFIN
	ts := httptest.NewServer(server.BuildAPI(cfg, db.Raw, server.Seams{Avatars: appuser.FixedAvatarPicker(appuser.DefaultAvatar)}))
	t.Cleanup(ts.Close)
	return importServer{t: t, url: ts.URL, f: f, user: userID, token: full}
}

func (s importServer) do(method, path, body string) (int, string) {
	s.t.Helper()
	token := s.token
	if strings.Contains(path, "/ingest-") {
		token = "eco_pat_transaction-import-test-ingest-00000000000"
	}
	req, err := http.NewRequest(method, s.url+path, strings.NewReader(body))
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

var (
	sharedImportRoutes = [][2]string{
		{http.MethodGet, "/api/v1/import/get-source-list"},
		{http.MethodGet, "/api/v1/import/get-rule-list"},
		{http.MethodGet, "/api/v1/import/get-queued-event-list"},
	}
	appleWalletRoutes = [][2]string{{http.MethodPost, "/api/v1/import/ingest-apple-wallet-event"}}
	simpleFINRoutes   = [][2]string{
		{http.MethodGet, "/api/v1/import/get-credential-key"},
		{http.MethodPost, "/api/v1/import/sync-source"},
		{http.MethodPost, "/api/v1/import/claim-setup-token"},
	}
)

func TestImportProviders_RoutesFollowTheFlags(t *testing.T) {
	for _, tc := range []struct {
		name      string
		simpleFIN bool
	}{
		{"simplefin off", false},
		{"simplefin on", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newImportServer(t, tc.simpleFIN)
			check := func(routes [][2]string, mounted bool) {
				for _, r := range routes {
					status, _ := s.do(r[0], r[1], "{}")
					if mounted == (status == http.StatusNotFound) {
						t.Errorf("%s %s: status %d, mounted=%v", r[0], r[1], status, mounted)
					}
				}
			}
			check(sharedImportRoutes, true)
			check(appleWalletRoutes, true)
			check(simpleFINRoutes, tc.simpleFIN)
		})
	}
}

func TestImportProviders_DisabledProviderSourceIsRefusedAndHidden(t *testing.T) {
	s := newImportServer(t, false)
	srcID := s.f.ImportSource(fixture.ImportSource{UserID: s.user, Provider: "simplefin", Name: "Old Bank"})
	s.f.ImportTransactionLink(fixture.ImportTransactionLink{
		SourceID: srcID, ExternalAccountID: "Checking", ExternalTransactionID: "txn-1", Status: "queued",
		ExternalPayee: "Old Coffee", ExternalAmount: "4.50000000", ExternalCurrency: "USD",
	})

	status, body := s.do(http.MethodPost, "/api/v1/import/create-source", `{"provider":"simplefin","name":"My Bank","credentialCiphertext":"v1:aXY=:Y3Q="}`)
	if status != http.StatusBadRequest || !strings.Contains(body, "This import provider is not supported.") {
		t.Fatalf("create-source for a disabled provider: %d %s", status, body)
	}
	status, body = s.do(http.MethodGet, "/api/v1/import/get-source-list", "")
	if status != http.StatusOK || strings.Contains(body, "Old Bank") {
		t.Fatalf("a disabled provider's source must not be listed: %d %s", status, body)
	}
	status, body = s.do(http.MethodGet, "/api/v1/import/get-queued-event-list", "")
	if status != http.StatusOK || strings.Contains(body, "Old Coffee") {
		t.Fatalf("a disabled provider's queued rows must not be listed: %d %s", status, body)
	}
}

func TestImportRuleSuggestionsRemovedWithAIConfigured(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	full := "eco_pat_ai-configured-test-full-0000000000000000"
	exp := time.Now().UTC().Add(24 * time.Hour)
	f.AccessToken(fixture.AccessToken{UserID: userID, Kind: "personal", TokenHash: appuser.HashAccessToken(full), ExpiresAt: &exp})

	var calls atomic.Int64
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer endpoint.Close()
	cfg := baseTestConfig(db.Engine)
	cfg.AIEnabled = true
	cfg.AIDialect = config.AIDialectAnthropic
	cfg.AIEndpoint = endpoint.URL + "/anthropic/v1"
	cfg.AIAPIKey = "test-key"
	cfg.AIModel = "claude-sonnet-4-6"
	ts := httptest.NewServer(server.BuildAPI(cfg, db.Raw, server.Seams{Avatars: appuser.FixedAvatarPicker(appuser.DefaultAvatar)}))
	defer ts.Close()
	s := importServer{t: t, url: ts.URL, f: f, user: userID, token: full}
	status, body := s.do(http.MethodPost, "/api/v1/import/suggest-rules", `{"scope":"all"}`)
	if status != http.StatusNotFound || calls.Load() != 0 {
		t.Fatalf("removed suggestions endpoint: status=%d body=%s AI calls=%d", status, body, calls.Load())
	}
	status, body = s.do(http.MethodGet, "/api/v1/import/get-rule-list", "")
	if status != http.StatusOK {
		t.Fatalf("rule management must remain available: status=%d body=%s", status, body)
	}
}
