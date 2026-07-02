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

// TestGuard_EveryRouteHasScenario fails when a registered route has no
// catalogue call — so a new endpoint cannot ship without landing in the
// smoke+parity safety net.
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
		if !covered[r] {
			missing = append(missing, r)
		}
	}
	if len(missing) > 0 {
		t.Errorf("routes with no catalogue scenario (add one):\n  %s", strings.Join(missing, "\n  "))
	}
}
