package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/authserver"
	handlerauthserver "github.com/econumo/econumo/internal/authserver/api"
	authrepo "github.com/econumo/econumo/internal/authserver/repo"
	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/authstub"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/web/router"
)

const (
	appURL      = "https://econumo.example.test"
	callbackURL = "https://claude.ai/api/mcp/auth_callback"
	challenge   = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	verifier    = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
)

type stubCreds struct{ issued int }

func (c *stubCreds) LockForOAuth(context.Context, vo.Id) (int64, error)          { return 0, nil }
func (c *stubCreds) CredentialsGeneration(context.Context, vo.Id) (int64, error) { return 0, nil }
func (c *stubCreds) RevokeOAuthGrantTokens(context.Context, vo.Id) error         { return nil }
func (c *stubCreds) IssueOAuthAccessToken(context.Context, vo.Id, vo.Id, string, int64, time.Duration) (string, bool, error) {
	c.issued++
	return "eco_oat_stub", true, nil
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

type harness struct {
	srv  *httptest.Server
	svc  *authserver.Service
	db   *dbtest.DB
	user string
}

type envelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	raw     []byte
	data    map[string]any
}

func newHarness(t *testing.T, appURL string) *harness {
	t.Helper()
	db := dbtest.NewSQLite(t)
	user := fixture.New(t, db).User(fixture.User{})
	svc := authserver.NewService(authrepo.NewRepo(db.Engine, db.TX), &stubCreds{}, db.TX, realClock{}, nil, appURL)
	h := router.New(router.Deps{
		Cfg:         config.Config{CORSAllowedOrigins: []string{"*"}},
		RegisterAPI: handlerauthserver.RegisterAPI(handlerauthserver.NewHandlers(svc), authstub.Authenticator{}),
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &harness{srv: srv, svc: svc, db: db, user: user}
}

func (h *harness) do(t *testing.T, method, path, token string, body any) (int, envelope) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, h.srv.URL+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := h.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env envelope
	_ = json.Unmarshal(raw, &env)
	env.raw = raw
	_ = json.Unmarshal(env.Data, &env.data)
	return resp.StatusCode, env
}

func (h *harness) registerClient(t *testing.T) string {
	t.Helper()
	res, err := h.svc.Register(context.Background(), model.ClientRegistrationRequest{ClientName: "Claude", RedirectURIs: []string{callbackURL}})
	if err != nil {
		t.Fatal(err)
	}
	return res.ClientID
}

func authBody(cid string) map[string]any {
	return map[string]any{"clientId": cid, "redirectUri": callbackURL, "responseType": "code",
		"codeChallenge": challenge, "codeChallengeMethod": "S256", "state": "s1"}
}

// connect approves an authorization and exchanges its code, returning the grant id.
func (h *harness) connect(t *testing.T) string {
	t.Helper()
	cid := h.registerClient(t)
	status, env := h.do(t, "POST", "/api/v1/authserver/approve-authorization", h.user, authBody(cid))
	if status != 200 {
		t.Fatalf("approve: %d %s", status, env.raw)
	}
	u, err := url.Parse(env.data["redirectUrl"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Token(context.Background(), model.TokenRequest{
		GrantType: "authorization_code", Code: u.Query().Get("code"), RedirectURI: callbackURL,
		CodeVerifier: verifier, ClientID: cid,
	}); err != nil {
		t.Fatal(err)
	}
	status, env = h.do(t, "GET", "/api/v1/authserver/get-connected-app-list", h.user, nil)
	var list []struct{ ID string }
	_ = json.Unmarshal(env.Data, &list)
	if status != 200 || len(list) != 1 {
		t.Fatalf("list: %d %s", status, env.raw)
	}
	return list[0].ID
}

func TestConsentEndpoints(t *testing.T) {
	h := newHarness(t, appURL)
	cid := h.registerClient(t)
	q := "clientId=" + cid + "&redirectUri=" + url.QueryEscape(callbackURL) +
		"&responseType=code&codeChallenge=" + challenge + "&codeChallengeMethod=S256&state=s1"
	status, env := h.do(t, "GET", "/api/v1/authserver/get-authorization-request?"+q, h.user, nil)
	if status != 200 || env.data["clientName"] != "Claude" || env.data["redirectHost"] != "claude.ai" {
		t.Fatalf("%d %s", status, env.raw)
	}

	body := authBody(cid)
	status, env = h.do(t, "POST", "/api/v1/authserver/approve-authorization", h.user, body)
	if status != 200 || !strings.HasPrefix(env.data["redirectUrl"].(string), callbackURL+"?") {
		t.Fatalf("approve: %d %s", status, env.raw)
	}
	status, env = h.do(t, "POST", "/api/v1/authserver/decline-authorization", h.user, body)
	if status != 200 || !strings.Contains(env.data["redirectUrl"].(string), "error=access_denied") {
		t.Fatalf("decline: %d %s", status, env.raw)
	}

	body["clientId"] = vo.NewId().String()
	status, env = h.do(t, "POST", "/api/v1/authserver/approve-authorization", h.user, body)
	if status != 400 || !strings.Contains(string(env.raw), "This app is not registered") {
		t.Fatalf("unknown client: %d %s", status, env.raw)
	}
}

func TestConsentEndpoints_RequireAuth(t *testing.T) {
	h := newHarness(t, appURL)
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/v1/authserver/get-authorization-request"},
		{"POST", "/api/v1/authserver/approve-authorization"},
		{"POST", "/api/v1/authserver/decline-authorization"},
		{"GET", "/api/v1/authserver/get-connected-app-list"},
		{"POST", "/api/v1/authserver/revoke-connected-app"},
	} {
		if status, env := h.do(t, c.method, c.path, "", nil); status != 401 {
			t.Errorf("%s %s without a token: %d %s", c.method, c.path, status, env.raw)
		}
	}
}

func TestConsentEndpoints_DisabledWithoutAppURL(t *testing.T) {
	h := newHarness(t, "")
	status, env := h.do(t, "GET", "/api/v1/authserver/get-authorization-request?clientId="+vo.NewId().String(), h.user, nil)
	if status != 400 || !strings.Contains(string(env.raw), "not available") {
		t.Fatalf("describe: %d %s", status, env.raw)
	}
	status, env = h.do(t, "GET", "/api/v1/authserver/get-connected-app-list", h.user, nil)
	if status != 200 || string(env.Data) != "[]" {
		t.Fatalf("list works regardless: %d %s", status, env.raw)
	}
}

func TestConnectedAppEndpoints(t *testing.T) {
	h := newHarness(t, appURL)
	grantID := h.connect(t)
	status, env := h.do(t, "GET", "/api/v1/authserver/get-connected-app-list", h.user, nil)
	if status != 200 || !strings.Contains(string(env.raw), grantID) {
		t.Fatalf("%d %s", status, env.raw)
	}
	status, env = h.do(t, "POST", "/api/v1/authserver/revoke-connected-app", h.user, map[string]any{"id": "nope"})
	if status != 400 {
		t.Fatalf("validation: %d %s", status, env.raw)
	}
	foreign := fixture.New(t, h.db).User(fixture.User{Email: "foreign@example.test"})
	status, env = h.do(t, "POST", "/api/v1/authserver/revoke-connected-app", foreign, map[string]any{"id": grantID})
	if status != 400 || !strings.Contains(string(env.raw), "Connected app not found") {
		t.Fatalf("foreign revoke: %d %s", status, env.raw)
	}
	status, env = h.do(t, "POST", "/api/v1/authserver/revoke-connected-app", h.user, map[string]any{"id": grantID})
	if status != 200 || string(env.Data) != "{}" {
		t.Fatalf("revoke: %d %s", status, env.raw)
	}
	status, env = h.do(t, "GET", "/api/v1/authserver/get-connected-app-list", h.user, nil)
	if status != 200 || string(env.Data) != "[]" {
		t.Fatalf("list after revoke: %d %s", status, env.raw)
	}
	status, _ = h.do(t, "POST", "/api/v1/authserver/revoke-connected-app", h.user, map[string]any{"id": grantID})
	if status != 400 {
		t.Fatalf("second revoke: %d", status)
	}
}
