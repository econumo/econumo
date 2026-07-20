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

// repoRoot resolves the repository root from this file's location, so every
// path a guard reads (route sources, the committed swagger.json) derives from
// one hand-counted ".." chain instead of several.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
}

// routeSourceLines scans the route-registration source files and maps each
// literal "METHOD /path" mux pattern to its registration source line, so a
// caller can also classify a route by HOW it is mounted (e.g. with or without
// the auth wrapper). Source-scanning (vs runtime introspection) is deliberate:
// http.ServeMux does not expose its patterns. If registration files move,
// update handlerGlobs — the guards failing loudly on zero matches protect
// against silently scanning nothing.
func routeSourceLines(t *testing.T) map[string]string {
	t.Helper()
	handlerGlobs := []string{"internal/*/api/routes.go"}
	// internal/admin registers the PRIVATE admin listener, served by a separate
	// http.Server and mounted on no public mux — so it has no parity scenario
	// and no golden, and scanning it would demand both. Excluded explicitly
	// rather than by naming the file outside the glob, so the reason survives
	// the next refactor. TestAdminRoutesAreNotOnThePublicMux asserts the
	// separation actually holds.
	const excluded = "internal/admin/api/routes.go"
	routes := map[string]string{}
	for _, g := range handlerGlobs {
		files, err := filepath.Glob(filepath.Join(repoRoot(t), g))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if rel, rerr := filepath.Rel(repoRoot(t), f); rerr == nil && filepath.ToSlash(rel) == excluded {
				continue
			}
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for _, line := range strings.Split(string(src), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "//") {
					continue // a route quoted in a comment is not a registration
				}
				if m := routePatternRe.FindStringSubmatch(line); m != nil {
					routes[m[1]] = line
				}
			}
		}
	}
	if len(routes) == 0 {
		t.Fatal("route scan found nothing — handlerGlobs are stale")
	}
	return routes
}

func registeredRoutes(t *testing.T) map[string]bool {
	t.Helper()
	routes := map[string]bool{}
	for r := range routeSourceLines(t) {
		routes[r] = true
	}
	// minRoutes is a floor, not a target: it catches the scan silently finding
	// fewer routes than it used to, which happens for one of two reasons —
	// (1) a registration file moved outside handlerGlobs, or (2) a route is no
	// longer written as a literal "METHOD /path" string (e.g. built from a
	// constant or concatenation) so routePatternRe stops matching it. Update
	// handlerGlobs for cause (1); for cause (2), keep routes literal or extend
	// the regex. Raise minRoutes as routes are added — never lower it.
	const minRoutes = 88
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
