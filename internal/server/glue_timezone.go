package server

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	appuser "github.com/econumo/econumo/internal/user"
	"github.com/econumo/econumo/internal/web/middleware"
)

// timezoneTrackingAuthenticator decorates the per-request authenticator to
// opportunistically persist the caller's X-Timezone so header-less clients
// (MCP) can fall back to it. The in-memory last-seen cache keeps writes to
// ~one per user per boot/change; persist failures never fail the request.
type timezoneTrackingAuthenticator struct {
	inner middleware.TokenAuthenticator
	users *appuser.Service
	// seen: vo.Id -> string (last persisted IANA name). Load-then-Store below is
	// deliberately non-atomic under concurrent requests for the same user — a
	// duplicate persist just repeats the same idempotent UPDATE, so "~one write
	// per user" (not "exactly one") is the actual guarantee. It also records
	// names PersistTimezone silently drops (e.g. "Local", or anything
	// time.LoadLocation rejects) so a client that keeps sending one of those
	// doesn't cause a repo call on every request.
	seen sync.Map
}

func NewTimezoneTrackingAuthenticator(inner middleware.TokenAuthenticator, users *appuser.Service) middleware.TokenAuthenticator {
	return &timezoneTrackingAuthenticator{inner: inner, users: users}
}

// The access level is passed through untouched: this decorator only observes
// the timezone, and must never widen or narrow the caller's authorization.
func (a *timezoneTrackingAuthenticator) Authenticate(ctx context.Context, token string) (model.Principal, error) {
	p, err := a.inner.Authenticate(ctx, token)
	if err != nil || !reqctx.IsLocationExplicit(ctx) {
		return p, err
	}
	tz := reqctx.Location(ctx).String()
	if prev, ok := a.seen.Load(p.UserID); ok && prev.(string) == tz {
		return p, nil
	}
	if perr := a.users.PersistTimezone(ctx, p.UserID, tz); perr != nil {
		slog.WarnContext(ctx, "timezone persist failed", slog.Any("err", perr))
	} else {
		a.seen.Store(p.UserID, tz)
	}
	return p, nil
}

// timezoneFallback installs the stored timezone for requests that carried no
// X-Timezone header. Applied to /mcp only; REST keeps header-or-UTC.
func timezoneFallback(users *appuser.Service) middleware.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if !reqctx.IsLocationExplicit(ctx) {
				if userID, ok := middleware.UserIDFromCtx(ctx); ok {
					if tz, err := users.GetTimezone(ctx, userID); err == nil && tz != "" {
						if loc, lerr := time.LoadLocation(tz); lerr == nil {
							ctx = reqctx.WithLocation(ctx, loc)
							reqctx.AddLogAttr(ctx, "timezone", loc.String())
							r = r.WithContext(ctx)
						}
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
