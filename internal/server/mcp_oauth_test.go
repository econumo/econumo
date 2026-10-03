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
	appuser "github.com/econumo/econumo/internal/user"
)

func buildOAuthTestAPI(t *testing.T, appURL string) http.Handler {
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
	}, db.Raw, server.Seams{Avatars: appuser.FixedAvatarPicker(appuser.DefaultAvatar)})
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
	want := `Bearer resource_metadata="https://econumo.example.test/.well-known/oauth-protected-resource/mcp"`
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
