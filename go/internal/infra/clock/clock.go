// Package clock provides the production time source. It is a one-liner over
// time.Now, isolated behind a type so the app and handler layers depend on a
// Clock seam (and tests can substitute a fixed clock) rather than calling
// time.Now directly.
package clock

import "time"

// Real is the production clock: Now returns the current wall-clock time.
type Real struct{}

// New returns a real clock.
func New() Real { return Real{} }

// Now returns the current time in UTC. Persisted timestamps (createdAt/updatedAt,
// spentAt, operation-request times) are formatted as a bare "Y-m-d H:i:s" string
// with no zone, so the wall-clock MUST be UTC to match the PHP backend, whose
// DatetimeService does `new DateTimeImmutable()` under the container's UTC default
// timezone. Returning local time here would write/echo timestamps shifted by the
// host's offset (e.g. -07:00), diverging from PHP on every created/updated row.
func (Real) Now() time.Time { return time.Now().UTC() }
