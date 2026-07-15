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
// not expose its patterns. If registration files move, update handlerGlobs —
// the guard failing loudly on zero matches protects against silently scanning
// nothing.
func registeredRoutes(t *testing.T) map[string]bool {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	handlerGlobs := []string{"internal/*/api/routes.go"}
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
	// minRoutes is a floor, not a target: it catches the scan silently finding
	// fewer routes than it used to, which happens for one of two reasons —
	// (1) a registration file moved outside handlerGlobs, or (2) a route is no
	// longer written as a literal "METHOD /path" string (e.g. built from a
	// constant or concatenation) so routePatternRe stops matching it. Update
	// handlerGlobs for cause (1); for cause (2), keep routes literal or extend
	// the regex. Raise minRoutes as routes are added — never lower it.
	const minRoutes = 93
	if len(routes) < minRoutes {
		t.Fatalf("route scan found only %d routes, want >= %d — a registration file moved outside handlerGlobs, or a route is no longer a literal \"METHOD /path\" string (see comment above)", len(routes), minRoutes)
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

// TestGuard_NoOrphanedGoldens fails when testdata/golden holds a file whose
// scenario was renamed or deleted — a stale golden that TestSmoke_Catalogue
// will never read again, silently rotting instead of being cleaned up.
func TestGuard_NoOrphanedGoldens(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	goldenDir := filepath.Join(filepath.Dir(thisFile), "testdata", "golden")
	files, err := filepath.Glob(filepath.Join(goldenDir, "*.golden"))
	if err != nil {
		t.Fatal(err)
	}
	scenarios := map[string]bool{}
	for _, sc := range Catalogue() {
		scenarios[sc.Name] = true
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".golden")
		if !scenarios[name] {
			t.Errorf("orphaned golden (scenario renamed or deleted): %s — delete the stale golden or restore the scenario", f)
		}
	}
}
