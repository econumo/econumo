package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/web/httpx"
)

// AdminAuth guards the private admin listener with a single shared bearer
// token. Unlike Auth there is no user, no session, and no access-level gate:
// the caller is a service (the payment portal), not a person.
func AdminAuth(token string) Middleware {
	want := []byte(token)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got, ok := bearerToken(r)
			if !ok || subtle.ConstantTimeCompare([]byte(got), want) != 1 {
				httpx.WriteError(r.Context(), w, errs.NewUnauthorized("Invalid access token"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
