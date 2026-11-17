package middleware

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
)

// Affiliate copies a well-formed referral cookie into the request context. A
// missing or malformed value passes through untouched: it must never fail a
// request.
func Affiliate(cookieName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c, err := r.Cookie(cookieName); err == nil {
				if v := model.NormalizeAffiliate(c.Value); v != "" {
					r = r.WithContext(reqctx.WithAffiliate(r.Context(), v))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
