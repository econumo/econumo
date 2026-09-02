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
