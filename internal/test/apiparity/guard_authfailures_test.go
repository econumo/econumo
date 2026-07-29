package apiparity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestGuard_EveryAuthenticatedRouteDocuments401And500 stops the OpenAPI spec's
// failure documentation from rotting the way the budget handlers had (issue
// #131): each documented only 200, understating what a client must handle even
// though every one returns 401 on a missing/invalid token and 500 on an
// unhandled error. Those two statuses are reachable on EVERY authenticated
// route, GET or POST, so the spec must say so — this guard makes that a
// build-time invariant, the same way TestGuard_EveryRestrictedPostDocuments402
// does for 402.
//
// Public routes (login/register/remind/reset/…) require no token, so 401 is not
// asserted there; they are classified exactly as in the 402 guard, by their
// registration line lacking the auth( wrapper.
func TestGuard_EveryAuthenticatedRouteDocuments401And500(t *testing.T) {
	specPath := filepath.Join(repoRoot(t), "internal", "web", "apidoc", "docs", "swagger.json")
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read swagger.json (run `make swagger`): %v", err)
	}
	var spec struct {
		Paths map[string]map[string]struct {
			Responses map[string]any `json:"responses"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parse swagger.json: %v", err)
	}
	if len(spec.Paths) == 0 {
		t.Fatal("swagger.json has no paths — the spec is stale or malformed")
	}

	lines := routeSourceLines(t)
	var missing []string
	for route := range registeredRoutes(t) {
		method, path, ok := strings.Cut(route, " ")
		if !ok {
			continue
		}
		if !strings.Contains(lines[route], "auth(") {
			continue // public route: no token required, so no 401
		}
		op, found := spec.Paths[path][strings.ToLower(method)]
		if !found {
			missing = append(missing, route+" absent from swagger.json")
			continue
		}
		for _, code := range []string{"401", "500"} {
			if _, has := op.Responses[code]; !has {
				missing = append(missing, route+" missing "+code)
			}
		}
	}
	sort.Strings(missing)

	if len(missing) > 0 {
		t.Errorf("authenticated routes must document 401 (`@Failure 401 {object} apidoc.JsonResponseUnauthorized`) and 500 (`@Failure 500 {object} apidoc.JsonResponseException`); add the annotation, then `make swagger`:\n  %s",
			strings.Join(missing, "\n  "))
	}
}
