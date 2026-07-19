package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/httpx"
)

// TokenAuthenticator is the narrow contract the auth middleware needs; the
// user feature's Service satisfies it (opaque-token lookup in the DB).
// Defining it here keeps the middleware from hard-depending on the concrete
// type, so tests (and any future authenticator) can substitute their own.
type TokenAuthenticator interface {
	Authenticate(ctx context.Context, token string) (userID vo.Id, tokenID vo.Id, level model.AccessLevel, err error)
}

// ctxKeyUserID is the context key under which the authenticated user id is
// stored. It is distinct from the iota-based keys in middleware.go (a separate
// unexported type) so the two key spaces cannot collide.
type ctxKeyUserIDType struct{}

var ctxKeyUserID ctxKeyUserIDType

type ctxKeyTokenIDType struct{}

var ctxKeyTokenID ctxKeyTokenIDType

// Auth builds the authentication middleware. It reads the
// "Authorization: Bearer <token>" header, authenticates the opaque token via
// authn, and on success stores the user id and the token row id in the request
// context (retrievable with UserIDFromCtx / TokenIDFromCtx). A missing header,
// malformed header, or failed authentication produces the frozen 401 envelope
// (via httpx.WriteError on an *errs.UnauthorizedError) and the downstream
// handler is not called.
//
// dev controls only the unhandled-500 path inside httpx.WriteError; the 401
// path does not expose internals — a non-Unauthorized authenticator error
// (e.g. the DB being down) is mapped to the generic 401.
func Auth(authn TokenAuthenticator, dev bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				httpx.WriteError(w, errs.NewUnauthorized("Access token not found"), dev)
				return
			}
			userID, tokenID, _, err := authn.Authenticate(r.Context(), token)
			if err != nil {
				var ue *errs.UnauthorizedError
				if !errors.As(err, &ue) {
					err = errs.NewUnauthorized("Invalid access token")
				}
				httpx.WriteError(w, err, dev)
				return
			}
			ctx := context.WithValue(r.Context(), ctxKeyUserID, userID)
			ctx = context.WithValue(ctx, ctxKeyTokenID, tokenID)
			reqctx.AddLogAttr(ctx, "user_id", userID.String())
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

// UserIDFromCtx returns the authenticated user id stored by the auth
// middleware, reporting whether one was present. Handlers behind the auth
// middleware can rely on ok being true; public handlers will see ok=false.
func UserIDFromCtx(ctx context.Context) (vo.Id, bool) {
	id, ok := ctx.Value(ctxKeyUserID).(vo.Id)
	return id, ok
}

// TokenIDFromCtx returns the authenticated request's access-token row id
// (the "current session" for logout / revoke-current / isCurrent marking).
func TokenIDFromCtx(ctx context.Context) (vo.Id, bool) {
	id, ok := ctx.Value(ctxKeyTokenID).(vo.Id)
	return id, ok
}

// RequireUser pulls the user id placed by the auth middleware; absence is
// treated as unauthorized (defense in depth — every route is already behind
// that middleware). The dev flag only gates the 500 exception path inside
// httpx.WriteError, which this 401 branch never reaches, so it is passed as
// false unconditionally without changing the emitted envelope.
func RequireUser(w http.ResponseWriter, r *http.Request) (vo.Id, bool) {
	id, ok := UserIDFromCtx(r.Context())
	if !ok {
		httpx.WriteError(w, errs.NewUnauthorized("Access token not found"), false)
		return vo.Id{}, false
	}
	return id, true
}
