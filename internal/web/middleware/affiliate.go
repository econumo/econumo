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
				c, err := r.Cookie(p.Cookie)
				if err != nil {
					continue
				}
				if code := model.NormalizeAffiliateCode(c.Value); code != "" {
					r = r.WithContext(reqctx.WithAffiliate(r.Context(), p.Program, code))
					break
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
