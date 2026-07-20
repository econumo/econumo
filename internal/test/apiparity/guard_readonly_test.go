package apiparity

import (
	"testing"

	"github.com/econumo/econumo/internal/web/middleware"
)

// TestGuard_ReadonlyAllowedPathsAreRealRoutes fails when a path in
// middleware.ReadonlyAllowedPaths is not an actually-registered POST route —
// e.g. after a route rename — which would otherwise silently strand a
// restricted user unable to log out or rotate a password (both the map and
// its own unit tests would stay green while production 402s the request).
func TestGuard_ReadonlyAllowedPathsAreRealRoutes(t *testing.T) {
	routes := registeredRoutes(t)
	for path := range middleware.ReadonlyAllowedPaths {
		if !routes["POST "+path] {
			t.Errorf("readonlyAllowedPaths has %q, but no registered POST route matches it — the route was likely renamed", path)
		}
	}
}
