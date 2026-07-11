// Package reqctx carries request-scoped values through context.Context so any
// layer can read them without importing the HTTP/middleware layer that sets
// them. It is a stdlib-only leaf package (imported by both internal/web/middleware
// and the application services), which keeps the dependency direction clean: the
// app never imports ui.
//
// Currently it carries the caller's timezone (the X-Timezone request header,
// resolved to a *time.Location), used to compute day boundaries — e.g. an
// account's "balance as of end of today" — in the user's local day rather than
// the server's UTC day.
package reqctx

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type ctxKey int

const (
	locationKey ctxKey = iota
	logAttrsKey
)

// WithLocation returns a context carrying the request's timezone.
func WithLocation(ctx context.Context, loc *time.Location) context.Context {
	return context.WithValue(ctx, locationKey, loc)
}

// Location returns the request's timezone, or time.UTC when none was set (or a
// nil location was stored).
func Location(ctx context.Context) *time.Location {
	if loc, ok := ctx.Value(locationKey).(*time.Location); ok && loc != nil {
		return loc
	}
	return time.UTC
}

// logAccumulator is a request-scoped, pointer-backed bag of structured log
// attributes. A single pointer is placed in the context once (by the access-log
// middleware); every layer that knows something worth logging — the auth
// middleware, handlers, application services — appends to the SAME instance via
// AddLogAttr, and the middleware reads them all back with LogAttrs when it emits
// the operation line. The mutex tolerates a handler that fans work out to
// goroutines sharing the request context.
type logAccumulator struct {
	mu    sync.Mutex
	attrs []slog.Attr
}

// WithLogAttrs installs a fresh, empty log accumulator in the context. Call it
// once at the start of a request; downstream AddLogAttr calls then have somewhere
// to record.
func WithLogAttrs(ctx context.Context) context.Context {
	return context.WithValue(ctx, logAttrsKey, &logAccumulator{})
}

// AddLogAttr records one structured attribute (custom dimension) on the
// request's log accumulator. It is a no-op when no accumulator is present (e.g.
// background jobs, CLI, or tests not wired through the middleware), so callers
// never need to check. Keep values to ids/counts — never PII.
func AddLogAttr(ctx context.Context, key string, value any) {
	acc, ok := ctx.Value(logAttrsKey).(*logAccumulator)
	if !ok || acc == nil {
		return
	}
	acc.mu.Lock()
	acc.attrs = append(acc.attrs, slog.Any(key, value))
	acc.mu.Unlock()
}

// LogAttrs returns a copy of the attributes accumulated for this request, or nil
// when no accumulator is present.
func LogAttrs(ctx context.Context) []slog.Attr {
	acc, ok := ctx.Value(logAttrsKey).(*logAccumulator)
	if !ok || acc == nil {
		return nil
	}
	acc.mu.Lock()
	defer acc.mu.Unlock()
	out := make([]slog.Attr, len(acc.attrs))
	copy(out, acc.attrs)
	return out
}
