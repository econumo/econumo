package server_test

import (
	"encoding/json"
	"io"
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

type importServer struct {
	t     *testing.T
	url   string
	f     *fixture.Builder
	user  string
	token string
}

func newImportServer(t *testing.T, appleWallet, simpleFIN bool) importServer {
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
	cfg.ImportAppleWallet = appleWallet
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
		name                   string
		appleWallet, simpleFIN bool
	}{
		{"both off", false, false},
		{"apple wallet only", true, false},
		{"simplefin only", false, true},
		{"both on", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newImportServer(t, tc.appleWallet, tc.simpleFIN)
			check := func(routes [][2]string, mounted bool) {
				for _, r := range routes {
					status, _ := s.do(r[0], r[1], "{}")
					if mounted == (status == http.StatusNotFound) {
						t.Errorf("%s %s: status %d, mounted=%v", r[0], r[1], status, mounted)
					}
				}
			}
			check(sharedImportRoutes, tc.appleWallet || tc.simpleFIN)
			check(appleWalletRoutes, tc.appleWallet)
			check(simpleFINRoutes, tc.simpleFIN)
		})
	}
}

func TestImportProviders_DisabledProviderSourceIsRefusedAndHidden(t *testing.T) {
	s := newImportServer(t, false, true)
	srcID := s.f.ImportSource(fixture.ImportSource{UserID: s.user, Provider: "apple-wallet", Name: "Old iPhone"})
	s.f.ImportTransactionLink(fixture.ImportTransactionLink{
		SourceID: srcID, ExternalAccountID: "Apple Card", ExternalTransactionID: "tap-1", Status: "queued",
		ExternalPayee: "Old Coffee", ExternalAmount: "4.50000000", ExternalCurrency: "USD",
	})

	status, body := s.do(http.MethodPost, "/api/v1/import/create-source", `{"provider":"apple-wallet","name":"iPhone"}`)
	if status != http.StatusBadRequest || !strings.Contains(body, "This import provider is not supported.") {
		t.Fatalf("create-source for a disabled provider: %d %s", status, body)
	}
	status, body = s.do(http.MethodGet, "/api/v1/import/get-source-list", "")
	if status != http.StatusOK || strings.Contains(body, "Old iPhone") {
		t.Fatalf("a disabled provider's source must not be listed: %d %s", status, body)
	}
	status, body = s.do(http.MethodGet, "/api/v1/import/get-queued-event-list", "")
	if status != http.StatusOK || strings.Contains(body, "Old Coffee") {
		t.Fatalf("a disabled provider's queued rows must not be listed: %d %s", status, body)
	}
}

func TestSuggestRules_AnthropicWiring(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	accountID := f.Account(fixture.Account{UserID: userID})
	categoryID := f.Category(fixture.Category{UserID: userID, Name: "Coffee"})
	txID := f.Transaction(fixture.Transaction{UserID: userID, AccountID: accountID, CategoryID: categoryID, Amount: "4.50"})
	sourceID := f.ImportSource(fixture.ImportSource{UserID: userID, Name: "iPhone"})
	f.ImportTransactionLink(fixture.ImportTransactionLink{
		SourceID: sourceID, ExternalAccountID: "Card", ExternalTransactionID: "coffee-1",
		TransactionID: txID, ExternalPayee: "Blue Bottle", ExternalAmount: "4.50", ExternalCurrency: "USD",
	})
	full := "eco_pat_anthropic-wiring-test-full-00000000000000"
	exp := time.Now().UTC().Add(24 * time.Hour)
	f.AccessToken(fixture.AccessToken{UserID: userID, Kind: "personal", TokenHash: appuser.HashAccessToken(full), ExpiresAt: &exp})

	calls := 0
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/anthropic/v1/messages" || r.Header.Get("x-api-key") != "test-key" || r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("unexpected Anthropic request: %s %v", r.URL.Path, r.Header)
		}
		var body struct {
			System   string                           `json:"system"`
			Messages []struct{ Role, Content string } `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.System == "" || len(body.Messages) != 1 || body.Messages[0].Role != "user" || !strings.Contains(body.Messages[0].Content, "Blue Bottle") {
			t.Errorf("missing suggestion prompt: %+v", body)
		}
		suggestion := `{"rules":[{"matchField":"external_payee","matchType":"contains","matchValue":"Blue Bottle","categoryId":"` + categoryID + `","reason":"Coffee purchases"}]}`
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{{"type": "text", "text": suggestion}}, "stop_reason": "end_turn",
		})
	}))
	defer endpoint.Close()
	cfg := baseTestConfig(db.Engine)
	cfg.ImportAppleWallet = true
	cfg.AIEnabled = true
	cfg.AIDialect = config.AIDialectAnthropic
	cfg.AIEndpoint = endpoint.URL + "/anthropic/v1"
	cfg.AIAPIKey = "test-key"
	cfg.AIModel = "claude-sonnet-4-6"
	ts := httptest.NewServer(server.BuildAPI(cfg, db.Raw, server.Seams{Avatars: appuser.FixedAvatarPicker(appuser.DefaultAvatar)}))
	defer ts.Close()
	s := importServer{t: t, url: ts.URL, f: f, user: userID, token: full}
	status, body := s.do(http.MethodPost, "/api/v1/import/suggest-rules", `{"scope":"all"}`)
	if status != http.StatusOK || !strings.Contains(body, categoryID) || !strings.Contains(body, "Coffee purchases") || calls != 1 {
		t.Fatalf("status=%d body=%s calls=%d", status, body, calls)
	}
}
