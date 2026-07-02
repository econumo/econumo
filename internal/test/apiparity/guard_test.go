// guard_test.go
package apiparity

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// missingFromCatalogue lists registered routes that do NOT yet have a
// catalogue scenario. Every entry here is a hole in the safety net. Tasks in
// docs/superpowers/plans/2026-07-01-phase0-test-investment.md remove entries
// as scenarios are added; the list must reach empty and then be deleted.
var missingFromCatalogue = map[string]bool{
	"GET /api/v1/budget/get-budget":               true,
	"GET /api/v1/budget/get-transaction-list":     true,
	"POST /api/v1/budget/accept-access":           true,
	"POST /api/v1/budget/change-element-currency": true,
	"POST /api/v1/budget/create-envelope":         true,
	"POST /api/v1/budget/create-folder":           true,
	"POST /api/v1/budget/decline-access":          true,
	"POST /api/v1/budget/delete-envelope":         true,
	"POST /api/v1/budget/delete-folder":           true,
	"POST /api/v1/budget/exclude-account":         true,
	"POST /api/v1/budget/grant-access":            true,
	"POST /api/v1/budget/include-account":         true,
	"POST /api/v1/budget/move-element-list":       true,
	"POST /api/v1/budget/order-folder-list":       true,
	"POST /api/v1/budget/reset-budget":            true,
	"POST /api/v1/budget/revoke-access":           true,
	"POST /api/v1/budget/update-envelope":         true,
	"POST /api/v1/budget/update-folder":           true,
}

var routePatternRe = regexp.MustCompile(`"((?:GET|POST) /api/v1/[a-z-]+/[a-z-]+)"`)

// registeredRoutes scans the route-registration source files for mux patterns.
// Source-scanning (vs runtime introspection) is deliberate: http.ServeMux does
// not expose its patterns. If registration files move (Phase 2 moves them to
// internal/<feature>/api/), update handlerGlobs — the guard failing loudly on
// zero matches protects against silently scanning nothing.
func registeredRoutes(t *testing.T) map[string]bool {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	handlerGlobs := []string{"internal/ui/handler/*/routes.go", "internal/*/api/routes.go"}
	routes := map[string]bool{}
	for _, g := range handlerGlobs {
		files, err := filepath.Glob(filepath.Join(repoRoot, g))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range routePatternRe.FindAllStringSubmatch(string(src), -1) {
				routes[m[1]] = true
			}
		}
	}
	if len(routes) == 0 {
		t.Fatal("route scan found nothing — handlerGlobs are stale")
	}
	return routes
}

// TestGuard_EveryRouteHasScenario fails when a registered route has neither a
// catalogue call nor an allowlist entry — so a new endpoint cannot ship
// without landing in the smoke+parity safety net.
func TestGuard_EveryRouteHasScenario(t *testing.T) {
	covered := map[string]bool{}
	for _, sc := range Catalogue() {
		for _, c := range sc.Calls() {
			path := c.Path
			if i := strings.IndexByte(path, '?'); i >= 0 {
				path = path[:i]
			}
			covered[c.Method+" "+path] = true
		}
	}
	var missing []string
	for r := range registeredRoutes(t) {
		if !covered[r] && !missingFromCatalogue[r] {
			missing = append(missing, r)
		}
	}
	if len(missing) > 0 {
		t.Errorf("routes with no catalogue scenario (add one, or allowlist with a plan reference):\n  %s", strings.Join(missing, "\n  "))
	}
	for r := range missingFromCatalogue {
		if covered[r] {
			t.Errorf("route %s now has a scenario — remove it from missingFromCatalogue", r)
		}
	}
}
