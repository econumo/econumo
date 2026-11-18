package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/web/middleware"
)

func TestAffiliateMiddleware(t *testing.T) {
	programs := []model.AffiliateCookie{{Program: "affonso", Cookie: "affonso_referral"}, {Program: "other", Cookie: "other_ref"}}
	c := func(name, value string) *http.Cookie { return &http.Cookie{Name: name, Value: value} }
	cases := []struct {
		name        string
		cookies     []*http.Cookie
		wantProgram string
		wantCode    string
	}{
		{"well formed", []*http.Cookie{c("affonso_referral", "abc123")}, "affonso", "abc123"},
		{"malformed", []*http.Cookie{c("affonso_referral", "a%20b")}, "", ""},
		{"too long", []*http.Cookie{c("affonso_referral", strings.Repeat("a", 65))}, "", ""},
		{"unrelated cookie", []*http.Cookie{c("unrelated", "abc123")}, "", ""},
		{"no cookie", nil, "", ""},
		{"both present: first in config order", []*http.Cookie{c("other_ref", "two"), c("affonso_referral", "one")}, "affonso", "one"},
		{"only the second", []*http.Cookie{c("other_ref", "two")}, "other", "two"},
		{"invalid first, valid second", []*http.Cookie{c("affonso_referral", "a b"), c("other_ref", "two")}, "other", "two"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var program, code string
			h := middleware.Affiliate(programs)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				program, code = reqctx.Affiliate(r.Context())
			}))
			req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
			for _, ck := range tc.cookies {
				req.AddCookie(ck)
			}
			h.ServeHTTP(httptest.NewRecorder(), req)
			if program != tc.wantProgram || code != tc.wantCode {
				t.Fatalf("Affiliate = %q %q, want %q %q", program, code, tc.wantProgram, tc.wantCode)
			}
		})
	}
}
