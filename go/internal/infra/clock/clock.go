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

// Now returns the current time.
func (Real) Now() time.Time { return time.Now() }
