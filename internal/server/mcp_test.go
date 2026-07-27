package server_test

import (
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

// buildTestAPIWithToken wires the production handler over a fresh sqlite DB
// with one fixture user and one live session token, returning the handler and
// the raw bearer token.
func buildTestAPIWithToken(t *testing.T) (http.Handler, string) {
	t.Helper()
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})

	rawToken := "eco_ses_u11111111-1111-1111-1111-111111111111000000"
	exp := time.Now().UTC().Add(24 * time.Hour)
	f.AccessToken(fixture.AccessToken{
		UserID:    userID,
		Kind:      "session",
		TokenHash: appuser.HashAccessToken(rawToken),
		UserAgent: "mcp-test",
		ExpiresAt: &exp,
	})

	cfg := config.Config{
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
	}

	handler := server.BuildAPI(cfg, db.Raw, server.Seams{
		Avatars: appuser.FixedAvatarPicker(appuser.DefaultAvatar),
	})
	return handler, rawToken
}

func TestMCP_AuthGate(t *testing.T) {
	handler, rawToken := buildTestAPIWithToken(t)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`

	// No token: the standard 401 envelope, MCP never runs.
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(initBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 401 || !strings.Contains(string(body), `"Access token not found"`) {
		t.Fatalf("unauth: %d %s", resp.StatusCode, body)
	}

	// Valid token: initialize succeeds with serverInfo name econumo.
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(initBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+rawToken)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), `"name":"econumo"`) {
		t.Fatalf("auth init: %d %s", resp.StatusCode, body)
	}
}
