// Package reqctx carries request-scoped values through context.Context so any
// layer can read them without importing the HTTP/middleware layer that sets
// them. It is a stdlib-only leaf package (imported by both internal/ui/middleware
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
	"time"
)

type ctxKey int

const locationKey ctxKey = iota

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
