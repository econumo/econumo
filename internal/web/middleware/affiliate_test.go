package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/web/middleware"
)

func TestAffiliateMiddleware(t *testing.T) {
	cases := []struct {
		name   string
		cookie *http.Cookie
		want   string
	}{
		{"well formed", &http.Cookie{Name: "affonso_referral", Value: "abc123"}, "abc123"},
		{"malformed", &http.Cookie{Name: "affonso_referral", Value: "a%20b"}, ""},
		{"too long", &http.Cookie{Name: "affonso_referral", Value: strings.Repeat("a", 65)}, ""},
		{"other cookie", &http.Cookie{Name: "other", Value: "abc123"}, ""},
		{"no cookie", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			h := middleware.Affiliate("affonso_referral")(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = reqctx.Affiliate(r.Context())
			}))
			req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
			if tc.cookie != nil {
				req.AddCookie(tc.cookie)
			}
			h.ServeHTTP(httptest.NewRecorder(), req)
			if got != tc.want {
				t.Fatalf("Affiliate = %q, want %q", got, tc.want)
			}
		})
	}
}
