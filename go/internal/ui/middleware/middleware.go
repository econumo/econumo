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
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/econumo/econumo/internal/reqctx"
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

// ---- context keys (unexported; exported accessors below) ----

type ctxKey int

const (
	ctxKeyRequestID ctxKey = iota
)

// ---- RequestID ----

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
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is effectively fatal for the process, but a
		// request-id is non-essential; fall back to a timestamp-derived value.
		return hex.EncodeToString([]byte(time.Now().UTC().Format("20060102150405.000000000")))
	}
	return hex.EncodeToString(b[:])
}

// ---- Recover ----

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

// ---- CORS ----

// CORS sets the frozen CORS headers (wire-compatible with existing clients, see
// COMPATIBILITY.md) and short-circuits preflight OPTIONS requests with 200. The
// allowed origin comes
// from config (CORS_ALLOW_ORIGIN, default "*").
func CORS(origin string) Middleware {
	if origin == "" {
		origin = "*"
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Methods", "OPTIONS, POST, GET")
			h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Timezone")
			h.Set("Access-Control-Max-Age", "3600")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ---- Timezone ----

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
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// LocationFromCtx returns the *time.Location stored by Timezone, or time.UTC if
// none was set. It delegates to reqctx so the app layer can read the same value
// without importing this (ui) package.
func LocationFromCtx(ctx context.Context) *time.Location {
	return reqctx.Location(ctx)
}

// ---- JWT (placeholder) ----
//
// The JWT authentication middleware is built in the user module (Phase 2). It
// will:
//   - read the "Authorization: Bearer <token>" header,
//   - verify the RS256 signature against the configured public key,
//   - reject expired/invalid tokens with the 401 envelope (httpx.WriteError on
//     an *errs.UnauthorizedError),
//   - extract the "id" claim (user UUID) and place a typed user id into the
//     request context for downstream handlers.
//
// It will be exposed as a Middleware (func(http.Handler) http.Handler) so the
// router can wrap the authenticated route group with it. Do NOT implement JWT
// parsing here — keep crypto isolated in infra/auth for separate review.
