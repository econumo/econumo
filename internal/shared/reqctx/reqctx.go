// Package reqctx carries request-scoped values through context.Context so any
// layer can read them without importing the HTTP/middleware layer that sets
// them. It is a stdlib-only leaf package (imported by both internal/web/middleware
// and the application services), which keeps the dependency direction clean: the
// app never imports ui.
//
// Currently it carries the caller's timezone (the X-Timezone request header,
// resolved to a *time.Location), used to compute day boundaries — e.g. an
// account's "balance as of end of today" — in the user's local day rather than
// the server's UTC day; and the caller's UI language (resolved from
// Accept-Language), used to select translated backend text (e.g. emails).
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
	languageKey
	explicitLocationKey
	clientIPKey
	idempotencyResourceKey
)

// WithClientIP returns a context carrying the caller's rate-limit bucket: an IP
// address, or the /64 network of an IPv6 one.
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPKey, ip)
}

// ClientIP returns the caller's rate-limit bucket, "" when none was resolved.
func ClientIP(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey).(string)
	return ip
}

// idempotencyResource is a mutable holder: the auth middleware installs it and
// reads it back, a handler far below fills it in.
type idempotencyResource struct {
	mu     sync.Mutex
	id     string
	pinned bool
}

// WithIdempotencyResource installs an empty holder for the id of the resource
// the request creates, which a replay of the request is told about.
func WithIdempotencyResource(ctx context.Context) context.Context {
	return context.WithValue(ctx, idempotencyResourceKey, &idempotencyResource{})
}

// SetIdempotencyResource is a no-op on a request with no Idempotency-Key.
func SetIdempotencyResource(ctx context.Context, id string) {
	h, ok := ctx.Value(idempotencyResourceKey).(*idempotencyResource)
	if !ok {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.id = id
}

// IdempotencyResource returns the id set for this request, "" when none.
func IdempotencyResource(ctx context.Context) string {
	h, ok := ctx.Value(idempotencyResourceKey).(*idempotencyResource)
	if !ok {
		return ""
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.id
}

// PinIdempotency says the request's business write is done: if the transaction
// it is called in commits, the key stays even when the response then fails, and
// a retry is told the write landed instead of repeating it. A no-op on a request
// with no Idempotency-Key.
func PinIdempotency(ctx context.Context) {
	h, ok := ctx.Value(idempotencyResourceKey).(*idempotencyResource)
	if !ok {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pinned = true
}

// IdempotencyPinned reports whether PinIdempotency was called for this request.
func IdempotencyPinned(ctx context.Context) bool {
	h, ok := ctx.Value(idempotencyResourceKey).(*idempotencyResource)
	if !ok {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.pinned
}

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

// WithLanguage returns a context carrying the caller's UI language (a
// supported two-letter tag resolved from Accept-Language by the middleware).
func WithLanguage(ctx context.Context, lang string) context.Context {
	return context.WithValue(ctx, languageKey, lang)
}

// Language returns the request's language, or "en" when none was set.
func Language(ctx context.Context) string {
	if lang, ok := ctx.Value(languageKey).(string); ok && lang != "" {
		return lang
	}
	return "en"
}

// WithExplicitLocation is WithLocation for a timezone the CALLER supplied
// (X-Timezone header) rather than the UTC default; the distinction lets the
// timezone-persist path ignore defaulted requests.
func WithExplicitLocation(ctx context.Context, loc *time.Location) context.Context {
	return context.WithValue(WithLocation(ctx, loc), explicitLocationKey, true)
}

// IsLocationExplicit reports whether the request's timezone came from the
// caller (WithExplicitLocation) rather than being defaulted or unset.
func IsLocationExplicit(ctx context.Context) bool {
	v, _ := ctx.Value(explicitLocationKey).(bool)
	return v
}

// IsLanguageExplicit reports whether the request's language came from the
// caller (a matched Accept-Language header, via the Language middleware)
// rather than being unset. The middleware calls WithLanguage only on a match,
// so "the language key holds a non-empty value" is exactly "explicit header".
func IsLanguageExplicit(ctx context.Context) bool {
	lang, ok := ctx.Value(languageKey).(string)
	return ok && lang != ""
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
