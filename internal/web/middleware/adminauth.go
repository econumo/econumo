package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/web/httpx"
)

// AdminShareStatusPath is the one admin route the share app's narrow token may
// call, and only with GET.
const AdminShareStatusPath = "/admin/user-status"

// AdminAuth guards the private admin listener with a shared bearer token.
// Unlike Auth there is no user, no session, and no access-level gate: the
// caller is a service (the payment portal), not a person. shareToken, when
// non-empty, is a second credential accepted only for GET on
// AdminShareStatusPath, so a leak of it exposes nothing else.
func AdminAuth(token, shareToken string) Middleware {
	want := []byte(token)
	wantShare := []byte(shareToken)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got, ok := bearerToken(r)
			if ok {
				// Both comparisons always run so timing does not reveal which
				// credential a caller was close to.
				mainOK := subtle.ConstantTimeCompare([]byte(got), want) == 1
				shareOK := len(wantShare) > 0 && subtle.ConstantTimeCompare([]byte(got), wantShare) == 1
				if mainOK || (shareOK && r.Method == http.MethodGet && r.URL.Path == AdminShareStatusPath) {
					next.ServeHTTP(w, r)
					return
				}
			}
			httpx.WriteError(r.Context(), w, errs.NewUnauthorized("Invalid access token"))
		})
	}
}
