// Package mcptest provides the context plumbing feature mcp tests need: a
// context carrying the auth middleware's user id, produced by running the
// REAL middleware over a stub authenticator.
package mcptest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/econumo/econumo/internal/test/authstub"
	"github.com/econumo/econumo/internal/web/middleware"
)

func CtxWithUser(t testing.TB, userID string) context.Context {
	t.Helper()
	var ctx context.Context
	h := middleware.Auth(authstub.Authenticator{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx = r.Context()
	}))
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+userID)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if ctx == nil {
		t.Fatal("auth middleware rejected the stub token")
	}
	return ctx
}
