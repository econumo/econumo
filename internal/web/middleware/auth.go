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

// StoredLanguageResolver is an optional capability of the wired
// TokenAuthenticator: it resolves the authenticated user's persisted UI
// language ("" = none) for requests that carried no supported Accept-Language
// header, so server-rendered error text follows the user's preference on both
// edges (REST and /mcp). Implemented by the server's authenticator decorator;
// test stubs that don't implement it simply get no fallback.
type StoredLanguageResolver interface {
	StoredLanguage(ctx context.Context, userID vo.Id) string
}

// ctxKeyUserID is the context key under which the authenticated user id is
// stored. It is distinct from the iota-based keys in middleware.go (a separate
// unexported type) so the two key spaces cannot collide.
type ctxKeyUserIDType struct{}

var ctxKeyUserID ctxKeyUserIDType

type ctxKeyTokenIDType struct{}

var ctxKeyTokenID ctxKeyTokenIDType

// ReadonlyAllowedPaths are the POST endpoints a restricted caller may still
// reach. The principle: a restricted user may always secure their account,
// leave it, or pay to restore it, but may not add data. update-password and
// the email-change flow (request/confirm/resend) are security operations, so
// locking someone out of rotating a compromised password or a compromised
// email address would be indefensible; create-billing-link is how a lapsed
// user reaches the payment portal, so blocking it would 402 exactly the
// person trying to fix their access; create-personal-token is excluded
// because it mints new write-capable credentials. Account deletion joins this
// list when it exists.
//
// Exported so a guard test (internal/test/apiparity) can assert every path
// here is still a real registered route, catching a route rename that would
// otherwise leave a lapsed user unable to log out or rotate a password.
//
// /mcp is deliberately absent, which 402s a restricted caller off the WHOLE MCP
// surface — JSON-RPC rides on POST, so even read-only tools are unreachable and
// the client gets the REST error envelope rather than a JSON-RPC error. That
// reads like a bug next to the "GET reads are never restricted" rule for REST,
// and it is not: gating at the transport is the fail-closed choice, since
// allowlisting /mcp to restore reads would open every write tool at once.
// Per-tool enforcement is what this would need first.
var ReadonlyAllowedPaths = map[string]bool{
	"/api/v1/user/logout-user":              true,
	"/api/v1/user/revoke-session":           true,
	"/api/v1/user/revoke-other-sessions":    true,
	"/api/v1/user/revoke-personal-token":    true,
	"/api/v1/user/update-password":          true,
	"/api/v1/user/create-billing-link":      true,
	"/api/v1/user/request-email-change":     true,
	"/api/v1/user/confirm-email-change":     true,
	"/api/v1/user/resend-email-change-code": true,
}

// Auth builds the authentication middleware. It reads the
// "Authorization: Bearer <token>" header, authenticates the opaque token via
// authn, and on success stores the user id and the token row id in the request
// context (retrievable with UserIDFromCtx / TokenIDFromCtx). A missing header,
// malformed header, or failed authentication produces the frozen 401 envelope
// (via httpx.WriteError on an *errs.UnauthorizedError) and the downstream
// handler is not called.
//
// The 401 path does not expose internals — a non-Unauthorized authenticator
// error (e.g. the DB being down) is mapped to the generic 401.
func Auth(authn TokenAuthenticator) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				httpx.WriteError(r.Context(), w, errs.NewUnauthorized("Access token not found"))
				return
			}
			userID, tokenID, level, err := authn.Authenticate(r.Context(), token)
			if err != nil {
				var ue *errs.UnauthorizedError
				if !errors.As(err, &ue) {
					err = errs.NewUnauthorized("Invalid access token")
				}
				httpx.WriteError(r.Context(), w, err)
				return
			}
			ctx := r.Context()
			// A request with no (supported) Accept-Language falls back to the
			// user's stored UI language, so error rendering follows the caller's
			// preference on every authenticated route — including the 402 below.
			if lr, ok := authn.(StoredLanguageResolver); ok && !reqctx.IsLanguageExplicit(ctx) {
				if lang := lr.StoredLanguage(ctx, userID); lang != "" {
					ctx = reqctx.WithLanguage(ctx, lang)
					reqctx.AddLogAttr(ctx, "language", lang)
				}
			}
			// Checked as "not full" rather than "is readonly": the access-level
			// domain is expected to widen, and an unrecognized level (empty
			// string, or a future third level) must fail closed to read-only
			// gating, not fall through to unrestricted write access.
			if level != model.AccessLevelFull && r.Method == http.MethodPost && !ReadonlyAllowedPaths[r.URL.Path] {
				httpx.WriteError(ctx, w, &errs.PaymentRequiredError{
					Msg:  "Read-only access. Write operations are disabled.",
					Code: errs.CodeReadonlyAccess,
				})
				return
			}
			ctx = context.WithValue(ctx, ctxKeyUserID, userID)
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
// that middleware).
func RequireUser(w http.ResponseWriter, r *http.Request) (vo.Id, bool) {
	id, ok := UserIDFromCtx(r.Context())
	if !ok {
		httpx.WriteError(r.Context(), w, errs.NewUnauthorized("Access token not found"))
		return vo.Id{}, false
	}
	return id, true
}
