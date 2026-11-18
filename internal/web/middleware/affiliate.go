package middleware

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
)

// Affiliate copies the referral of the first program, in configured order,
// whose cookie holds a well-formed code into the request context. A missing or
// malformed value passes through untouched: it must never fail a request.
func Affiliate(programs []model.AffiliateCookie) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, p := range programs {
				if code := firstValidCode(r.CookiesNamed(p.Cookie)); code != "" {
					r = r.WithContext(reqctx.WithAffiliate(r.Context(), p.Program, code))
					break
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// A browser can send the same cookie name twice (a host-only one beside a
// domain one), so a malformed first must not hide a valid later one.
func firstValidCode(cookies []*http.Cookie) string {
	for _, c := range cookies {
		if code := model.NormalizeAffiliateCode(c.Value); code != "" {
			return code
		}
	}
	return ""
}
