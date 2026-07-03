// Package middleware provides the global HTTP middleware chain for the Econumo
// API. Each middleware is a plain func(http.Handler) http.Handler (stdlib only
// — no router dependency). The chain order (outer -> inner) is:
//
//	requestid -> recover -> cors -> timezone -> [jwt]
//
// The JWT authentication middleware is intentionally NOT implemented here: it
// is security-sensitive and is built as part of the user module (Phase 2),
// where it will verify the RS256 token and stash the user id into the request
// context. See the documented placeholder near the bottom of this file.
package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/ui/httpx"
)

// Middleware is a single composable HTTP middleware.
type Middleware func(http.Handler) http.Handler

// Chain composes middlewares so that the first argument is the outermost
// wrapper and the last is closest to the final handler. Given
// Chain(a, b, c)(h) the request flows a -> b -> c -> h.
func Chain(mws ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		// Apply in reverse so the first middleware ends up outermost.
		for i := len(mws) - 1; i >= 0; i-- {
			next = mws[i](next)
		}
		return next
	}
}

type ctxKey int

const (
	ctxKeyRequestID ctxKey = iota
)

// requestIDHeader is the response header carrying the generated id.
const requestIDHeader = "X-Request-Id"

// RequestID generates a random hex id for each request, sets it on the
// X-Request-Id response header, and stashes it in the request context.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := newRequestID()
		w.Header().Set(requestIDHeader, id)
		ctx := context.WithValue(r.Context(), ctxKeyRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromCtx returns the request id stored by RequestID, or "" if absent.
func RequestIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyRequestID).(string); ok {
		return v
	}
	return ""
}

func newRequestID() string {
	// UUIDv7: time-ordered, so request ids sort by arrival and correlate cleanly
	// across log lines. Same lib/idiom as domain ids (vo.NewId). NewV7 only fails
	// if the OS entropy source fails (effectively never); fall back to a random
	// UUID so a request id is always present.
	if u, err := uuid.NewV7(); err == nil {
		return u.String()
	}
	return uuid.NewString()
}

// Recover catches panics from downstream handlers, logs them with the request
// id, and writes the frozen 500 exception envelope. The recovered value and
// stack trace are exposed in the response body only when dev is true.
func Recover(dev bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					stack := debug.Stack()
					slog.Error("panic recovered",
						"request_id", RequestIDFromCtx(r.Context()),
						"method", r.Method,
						"path", r.URL.Path,
						"panic", rec,
						"stack", string(stack),
					)
					// Surface the panic to the access log's operation line too
					// (the detailed stack stays in the dedicated record above).
					if lw, ok := w.(*logResponseWriter); ok {
						lw.SetError(fmt.Errorf("panic: %v", rec))
					}
					// stackTrace payload is only surfaced in dev (Exception
					// honors the dev flag internally).
					var trace any
					if dev {
						trace = string(stack)
					}
					httpx.Exception(w, "Internal Server Error", "panic", trace, dev)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// CORS controls cross-origin access via an allowlist (ECONUMO_CORS_ALLOW_ORIGIN). The
// default (empty list) is same-domain only: no Access-Control-Allow-Origin is
// emitted, so cross-origin reads are blocked while same-origin requests — which
// the browser never subjects to CORS — keep working (the bundled SPA and API
// share an origin). A configured list reflects a matching request Origin back
// (with Vary: Origin); an unmatched Origin gets no CORS headers. The special
// entry "*" restores allow-all.
//
// The method/header/max-age values are frozen (wire-compatible with existing
// clients, see CLAUDE.md) and are emitted only when an origin is allowed.
// Preflight OPTIONS requests short-circuit with 200.
func CORS(origins []string) Middleware {
	allowAll := false
	allowed := make(map[string]struct{}, len(origins))
	for _, o := range origins {
		o = strings.TrimSpace(o)
		switch {
		case o == "":
			continue
		case o == "*":
			allowAll = true
		default:
			allowed[o] = struct{}{}
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			origin := r.Header.Get("Origin")
			switch {
			case allowAll:
				h.Set("Access-Control-Allow-Origin", "*")
			case origin != "":
				if _, ok := allowed[origin]; ok {
					h.Set("Access-Control-Allow-Origin", origin)
					h.Add("Vary", "Origin")
				}
			}
			if h.Get("Access-Control-Allow-Origin") != "" {
				h.Set("Access-Control-Allow-Methods", "OPTIONS, POST, GET")
				h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Timezone")
				h.Set("Access-Control-Max-Age", "3600")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Timezone reads the X-Timezone request header, resolves it to a *time.Location
// via time.LoadLocation, and stashes it in the request context. On a missing or
// invalid header the location defaults to UTC.
func Timezone(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loc := time.UTC
		if tz := r.Header.Get("X-Timezone"); tz != "" {
			if l, err := time.LoadLocation(tz); err == nil {
				loc = l
			}
		}
		ctx := reqctx.WithLocation(r.Context(), loc)
		// Record the resolved timezone as a log dimension. Timezone runs inside
		// AccessLog, so the pointer accumulator carries it back out to both lines.
		reqctx.AddLogAttr(ctx, "timezone", loc.String())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// LocationFromCtx returns the *time.Location stored by Timezone, or time.UTC if
// none was set. It delegates to reqctx so the app layer can read the same value
// without importing this (ui) package.
func LocationFromCtx(ctx context.Context) *time.Location {
	return reqctx.Location(ctx)
}
