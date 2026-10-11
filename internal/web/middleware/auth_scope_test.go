package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/middleware"
)

type scopedAuthn struct{ scope model.TokenScope }

func (a scopedAuthn) Authenticate(context.Context, string) (model.Principal, error) {
	id := vo.NewId()
	return model.Principal{UserID: id, TokenID: id, Level: model.AccessLevelFull, Scope: a.scope}, nil
}

func TestAuth_ScopeGate(t *testing.T) {
	cases := []struct {
		name   string
		scope  model.TokenScope
		method string
		path   string
		want   int
	}{
		{"full anywhere", model.TokenScopeFull, http.MethodGet, "/api/v1/user/get-user-data", http.StatusOK},
		{"ingest on ingest route", model.TokenScopeIngest, http.MethodPost, "/api/v1/import/ingest-apple-wallet", http.StatusOK},
		{"ingest on read route", model.TokenScopeIngest, http.MethodGet, "/api/v1/user/get-user-data", http.StatusUnauthorized},
		{"ingest on other import route", model.TokenScopeIngest, http.MethodPost, "/api/v1/import/create-source", http.StatusUnauthorized},
		{"empty scope fails closed", model.TokenScope(""), http.MethodGet, "/api/v1/user/get-user-data", http.StatusUnauthorized},
		{"unknown scope on ingest route", model.TokenScope("admin"), http.MethodPost, "/api/v1/import/ingest-apple-wallet", http.StatusUnauthorized},
		{"empty scope on ingest route", model.TokenScope(""), http.MethodPost, "/api/v1/import/ingest-apple-wallet", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ran bool
			h := middleware.Auth(scopedAuthn{scope: tc.scope})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { ran = true }))
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("Authorization", "Bearer x")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tc.want, rec.Body.String())
			}
			if (tc.want == http.StatusOK) != ran {
				t.Fatalf("handler ran = %v", ran)
			}
			if tc.want == http.StatusUnauthorized && !strings.Contains(rec.Body.String(), `"message":"Invalid access token"`) {
				t.Fatalf("401 body must carry the frozen message, got %s", rec.Body.String())
			}
		})
	}
}

func TestAuthWith_MCPScopeNeedsAllowMCPScope(t *testing.T) {
	cases := []struct {
		name  string
		scope model.TokenScope
		path  string
		allow bool
		want  int
	}{
		{"mcp token on /mcp, not allowed", model.TokenScopeMCP, "/mcp", false, http.StatusUnauthorized},
		{"mcp token on /mcp, allowed", model.TokenScopeMCP, "/mcp", true, http.StatusOK},
		{"mcp token off /mcp, allowed", model.TokenScopeMCP, "/api/v1/user/get-user-data", true, http.StatusUnauthorized},
		{"full token on /mcp, not allowed", model.TokenScopeFull, "/mcp", false, http.StatusOK},
		{"full token on /mcp, allowed", model.TokenScopeFull, "/mcp", true, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ran bool
			h := middleware.AuthWith(scopedAuthn{scope: tc.scope}, middleware.AuthOptions{AllowMCPScope: tc.allow})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { ran = true }))
			req := httptest.NewRequest(http.MethodPost, tc.path, nil)
			req.Header.Set("Authorization", "Bearer x")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want || (tc.want == http.StatusOK) != ran {
				t.Fatalf("status = %d (ran %v), want %d; body %s", rec.Code, ran, tc.want, rec.Body.String())
			}
			if tc.want == http.StatusUnauthorized && !strings.Contains(rec.Body.String(), `"message":"Invalid access token"`) {
				t.Fatalf("401 body must carry the frozen message, got %s", rec.Body.String())
			}
		})
	}
}

func TestAuthWith_RefusedMCPTokenCarriesChallenge(t *testing.T) {
	const challenge = `Bearer resource_metadata="https://x.test/.well-known/oauth-protected-resource/mcp"`
	h := middleware.AuthWith(scopedAuthn{scope: model.TokenScopeMCP}, middleware.AuthOptions{Challenge: challenge})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer x")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") != challenge {
		t.Fatalf("%d %q", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}
}
