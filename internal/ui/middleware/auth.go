package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	"github.com/econumo/econumo/internal/ui/httpx"
	"github.com/econumo/econumo/pkg/jwt"
)

// TokenVerifier is the narrow contract the JWT middleware needs. pkg/jwt.JWT
// satisfies it. Defining it here keeps the middleware from hard-depending on the
// concrete type, so tests (and any future verifier) can substitute their own.
type TokenVerifier interface {
	Verify(token string) (jwt.Claims, error)
}

// ctxKeyUserID is the context key under which the authenticated user id is
// stored. It is distinct from the iota-based keys in middleware.go (a separate
// unexported type) so the two key spaces cannot collide.
type ctxKeyUserIDType struct{}

var ctxKeyUserID ctxKeyUserIDType

// JWT builds the authentication middleware. It reads the
// "Authorization: Bearer <token>" header, verifies the RS256 token via verifier,
// and on success stores the parsed user id (Claims.ID) in the request context
// (retrievable with UserIDFromCtx). A missing header, malformed header,
// verification failure, or an unparsable id claim all produce the frozen 401
// envelope (via httpx.WriteError on an *errs.UnauthorizedError) and the
// downstream handler is not called.
//
// dev controls only the unhandled-500 path inside httpx.WriteError; the 401
// path does not expose internals.
func JWT(verifier TokenVerifier, dev bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				httpx.WriteError(w, errs.NewUnauthorized("JWT Token not found"), dev)
				return
			}
			claims, err := verifier.Verify(token)
			if err != nil {
				httpx.WriteError(w, errs.NewUnauthorized("Invalid JWT Token"), dev)
				return
			}
			id, perr := vo.ParseId(claims.ID)
			if perr != nil {
				httpx.WriteError(w, errs.NewUnauthorized("Invalid JWT Token"), dev)
				return
			}
			ctx := context.WithValue(r.Context(), ctxKeyUserID, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// bearerToken extracts the token from an "Authorization: Bearer <token>"
// header. The scheme match is case-insensitive (RFC 7235); a missing header or
// non-Bearer scheme returns ok=false.
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", false
	}
	const prefix = "bearer "
	if len(h) < len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(h[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

// UserIDFromCtx returns the authenticated user id stored by the JWT middleware,
// reporting whether one was present. Handlers behind the JWT middleware can
// rely on ok being true; public handlers will see ok=false.
func UserIDFromCtx(ctx context.Context) (vo.Id, bool) {
	id, ok := ctx.Value(ctxKeyUserID).(vo.Id)
	return id, ok
}
