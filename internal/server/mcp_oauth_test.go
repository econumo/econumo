package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/infra/mailer"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/test/dbtest"
	appuser "github.com/econumo/econumo/internal/user"
)

func buildOAuthTestAPI(t *testing.T, appURL string) http.Handler {
	t.Helper()
	return buildOAuthTestAPIWithMailer(t, appURL, nil)
}

func buildOAuthTestAPIWithMailer(t *testing.T, appURL string, m mailer.Mailer) http.Handler {
	t.Helper()
	db := dbtest.NewSQLite(t)
	return server.BuildAPI(config.Config{
		DatabaseDriver:     db.Engine,
		CurrencyBase:       "USD",
		AllowRegistration:  true,
		CORSAllowedOrigins: []string{"*"},
		RateLimitLogin:     5,
		RateLimitReset:     5,
		RateLimitRemind:    3,
		RateLimitRegister:  5,
		RateLimitWindow:    15 * time.Minute,
		RateLimitGlobal:    60,
		AppURL:             appURL,
	}, db.Raw, server.Seams{Avatars: appuser.FixedAvatarPicker(appuser.DefaultAvatar), Mailer: m})
}

func oauthDo(t *testing.T, h http.Handler, method, path, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

func TestMCPOAuth_Enabled(t *testing.T) {
	h := buildOAuthTestAPI(t, "https://econumo.example.test/")
	const initBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`

	resp := oauthDo(t, h, "POST", "/mcp", initBody)
	body, _ := io.ReadAll(resp.Body)
	want := `Bearer resource_metadata="https://econumo.example.test/.well-known/oauth-protected-resource/mcp", scope="mcp"`
	if resp.StatusCode != 401 || resp.Header.Get("WWW-Authenticate") != want || !strings.Contains(string(body), `"Access token not found"`) {
		t.Fatalf("mcp 401: %d %q %s", resp.StatusCode, resp.Header.Get("WWW-Authenticate"), body)
	}

	resp = oauthDo(t, h, "GET", "/.well-known/oauth-authorization-server", "")
	var meta map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&meta)
	if resp.StatusCode != 200 || meta["issuer"] != "https://econumo.example.test" || meta["registration_endpoint"] != "https://econumo.example.test/oauth/register" {
		t.Fatalf("metadata: %d %v", resp.StatusCode, meta)
	}
	resp = oauthDo(t, h, "GET", "/.well-known/oauth-protected-resource/mcp", "")
	meta = nil
	_ = json.NewDecoder(resp.Body).Decode(&meta)
	if resp.StatusCode != 200 || meta["resource"] != "https://econumo.example.test/mcp" {
		t.Fatalf("resource metadata: %d %v", resp.StatusCode, meta)
	}

	resp = oauthDo(t, h, "POST", "/oauth/register", `{"client_name":"Claude","redirect_uris":["https://claude.ai/api/mcp/auth_callback"]}`)
	var reg map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&reg)
	if resp.StatusCode != 201 || reg["client_id"] == "" || reg["client_id"] == nil {
		t.Fatalf("register: %d %v", resp.StatusCode, reg)
	}
}

func TestMCPOAuth_DisabledWithoutAppURL(t *testing.T) {
	h := buildOAuthTestAPI(t, "")
	// Mounted but disabled: every route answers the handler's JSON 404 rather
	// than falling through to the SPA shell.
	for _, p := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp", "/.well-known/oauth-authorization-server"} {
		resp := oauthDo(t, h, "GET", p, "")
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 404 || !strings.Contains(string(body), `"error":"invalid_request"`) {
			t.Errorf("GET %s = %d %s", p, resp.StatusCode, body)
		}
	}
	for _, p := range []string{"/oauth/register", "/oauth/token"} {
		resp := oauthDo(t, h, "POST", p, `{}`)
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 404 || !strings.Contains(string(body), `"error":"invalid_request"`) {
			t.Errorf("POST %s = %d %s", p, resp.StatusCode, body)
		}
	}
	resp := oauthDo(t, h, "POST", "/mcp", `{}`)
	if resp.StatusCode != 401 || resp.Header.Get("WWW-Authenticate") != "" {
		t.Fatalf("mcp 401 must carry no challenge: %d %q", resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
	}
}

const (
	oauthCallback  = "https://claude.ai/api/mcp/auth_callback"
	oauthVerifier  = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	oauthChallenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	flowEmail      = "flow@example.test"
	flowPassword   = "flow-password-1"
	mcpInitBody    = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`
)

type captureMailer struct{ last mailer.Message }

func (m *captureMailer) Send(_ context.Context, msg mailer.Message) error {
	m.last = msg
	return nil
}

var flowResetCodeRe = regexp.MustCompile(`code is: (\d{6})`)

func flowDo(t *testing.T, h http.Handler, method, path, token, contentType, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out == nil {
		out = map[string]any{"_raw": rec.Body.String()}
	}
	return rec.Code, out
}

func flowJSON(t *testing.T, h http.Handler, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	b := ""
	if body != nil {
		raw, _ := json.Marshal(body)
		b = string(raw)
	}
	return flowDo(t, h, method, path, token, "application/json", b)
}

func flowTokenRequest(t *testing.T, h http.Handler, form url.Values) (int, map[string]any) {
	t.Helper()
	return flowDo(t, h, "POST", "/oauth/token", "", "application/x-www-form-urlencoded", form.Encode())
}

// flowSignIn registers a user, logs in, registers an OAuth client and approves
// its authorization request, returning the session token, the client id and
// the one-time code.
func flowSignIn(t *testing.T, h http.Handler) (session, clientID, code string) {
	t.Helper()
	if status, out := flowJSON(t, h, "POST", "/api/v1/user/register-user", "", map[string]any{"email": flowEmail, "password": flowPassword, "name": "Flow"}); status != 200 {
		t.Fatalf("register-user: %d %v", status, out)
	}
	status, out := flowJSON(t, h, "POST", "/api/v1/user/login-user", "", map[string]any{"username": flowEmail, "password": flowPassword})
	session, _ = out["token"].(string)
	if status != 200 || session == "" {
		t.Fatalf("login-user: %d %v", status, out)
	}
	status, out = flowJSON(t, h, "POST", "/oauth/register", "", map[string]any{"client_name": "Claude", "redirect_uris": []string{oauthCallback}})
	clientID, _ = out["client_id"].(string)
	if status != 201 || clientID == "" {
		t.Fatalf("register client: %d %v", status, out)
	}

	req := map[string]any{"clientId": clientID, "redirectUri": oauthCallback, "responseType": "code",
		"codeChallenge": oauthChallenge, "codeChallengeMethod": "S256", "resource": "https://econumo.example.test/mcp", "state": "st"}
	q := url.Values{"clientId": {clientID}, "redirectUri": {oauthCallback}, "responseType": {"code"},
		"codeChallenge": {oauthChallenge}, "codeChallengeMethod": {"S256"}, "resource": {"https://econumo.example.test/mcp"}, "state": {"st"}}
	status, out = flowJSON(t, h, "GET", "/api/v1/authserver/get-authorization-request?"+q.Encode(), session, nil)
	if data, _ := out["data"].(map[string]any); status != 200 || data["clientName"] != "Claude" || data["errorRedirectUrl"] != "" {
		t.Fatalf("get-authorization-request: %d %v", status, out)
	}
	status, out = flowJSON(t, h, "POST", "/api/v1/authserver/approve-authorization", session, req)
	data, _ := out["data"].(map[string]any)
	redirect, _ := data["redirectUrl"].(string)
	u, err := url.Parse(redirect)
	if status != 200 || err != nil || u.Query().Get("code") == "" || u.Query().Get("state") != "st" {
		t.Fatalf("approve-authorization: %d %v", status, out)
	}
	return session, clientID, u.Query().Get("code")
}

func TestMCPOAuth_FullFlow(t *testing.T) {
	h := buildOAuthTestAPI(t, "https://econumo.example.test")
	session, clientID, code := flowSignIn(t, h)

	status, tok := flowTokenRequest(t, h, url.Values{"grant_type": {"authorization_code"}, "code": {code},
		"redirect_uri": {oauthCallback}, "code_verifier": {oauthVerifier}, "client_id": {clientID}, "resource": {"https://econumo.example.test/mcp"}})
	access, _ := tok["access_token"].(string)
	refresh, _ := tok["refresh_token"].(string)
	if status != 200 || access == "" || refresh == "" || tok["token_type"] != "Bearer" {
		t.Fatalf("token exchange: %d %v", status, tok)
	}

	if status, out := flowDo(t, h, "POST", "/mcp", access, "application/json", mcpInitBody); status != 200 {
		t.Fatalf("mcp with oauth token: %d %v", status, out)
	}
	if status, out := flowJSON(t, h, "GET", "/api/v1/user/get-user-data", access, nil); status != 401 {
		t.Fatalf("an oauth token must not reach the REST API: %d %v", status, out)
	}

	status, tok = flowTokenRequest(t, h, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {clientID}})
	access2, _ := tok["access_token"].(string)
	if status != 200 || access2 == "" || access2 == access || tok["refresh_token"] == refresh {
		t.Fatalf("refresh: %d %v", status, tok)
	}
	if status, out := flowDo(t, h, "POST", "/mcp", access2, "application/json", mcpInitBody); status != 200 {
		t.Fatalf("mcp with refreshed token: %d %v", status, out)
	}

	status, out := flowJSON(t, h, "GET", "/api/v1/authserver/get-connected-app-list", session, nil)
	apps, _ := out["data"].([]any)
	if status != 200 || len(apps) != 1 {
		t.Fatalf("get-connected-app-list: %d %v", status, out)
	}
	grantID, _ := apps[0].(map[string]any)["id"].(string)
	if status, out := flowJSON(t, h, "POST", "/api/v1/authserver/revoke-connected-app", session, map[string]any{"id": grantID}); status != 200 {
		t.Fatalf("revoke-connected-app: %d %v", status, out)
	}
	if status, out := flowDo(t, h, "POST", "/mcp", access2, "application/json", mcpInitBody); status != 401 {
		t.Fatalf("mcp after revoke: %d %v", status, out)
	}
	if status, _ := flowTokenRequest(t, h, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tok["refresh_token"].(string)}, "client_id": {clientID}}); status != 400 {
		t.Fatalf("refresh after revoke: %d", status)
	}
}

func TestMCPOAuth_PasswordResetBetweenApproveAndExchange(t *testing.T) {
	mail := &captureMailer{}
	h := buildOAuthTestAPIWithMailer(t, "https://econumo.example.test", mail)
	_, clientID, code := flowSignIn(t, h)

	if status, out := flowJSON(t, h, "POST", "/api/v1/user/remind-password", "", map[string]any{"username": flowEmail}); status != 200 {
		t.Fatalf("remind-password: %d %v", status, out)
	}
	m := flowResetCodeRe.FindStringSubmatch(mail.last.Text)
	if m == nil {
		t.Fatalf("no reset code in %q", mail.last.Text)
	}
	if status, out := flowJSON(t, h, "POST", "/api/v1/user/reset-password", "", map[string]any{"username": flowEmail, "code": m[1], "password": "brand-new-pass-2"}); status != 200 {
		t.Fatalf("reset-password: %d %v", status, out)
	}

	status, tok := flowTokenRequest(t, h, url.Values{"grant_type": {"authorization_code"}, "code": {code},
		"redirect_uri": {oauthCallback}, "code_verifier": {oauthVerifier}, "client_id": {clientID}})
	if status != 400 || tok["error"] != "invalid_grant" || tok["access_token"] != nil {
		t.Fatalf("exchange after a credential reclaim must be invalid_grant: %d %v", status, tok)
	}
}
