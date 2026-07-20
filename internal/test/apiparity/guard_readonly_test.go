package apiparity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

// publicPostRoutes returns the POST routes mounted WITHOUT the auth wrapper —
// the public group (login/register/remind/reset). Those never reach the
// access-level check, so they cannot return 402. Classification keys on the
// registration line lacking "auth(" (the semantic that matters), not on the
// Handle-vs-HandleFunc spelling, so restyling a single registration cannot
// silently reclassify it.
func publicPostRoutes(t *testing.T) map[string]bool {
	t.Helper()
	public := map[string]bool{}
	for route, line := range routeSourceLines(t) {
		path, isPost := strings.CutPrefix(route, "POST ")
		if isPost && !strings.Contains(line, "auth(") {
			public[path] = true
		}
	}
	if len(public) == 0 {
		t.Fatal("public-route scan found nothing — the auth-wrapper spelling changed; update publicPostRoutes")
	}
	return public
}

// TestGuard_EveryRestrictedPostDocuments402 keeps the OpenAPI spec honest about
// read-only enforcement. The 402 is emitted by the auth middleware, not by a
// handler, so nothing about writing a new endpoint forces its author to think
// about the status — without this guard the per-endpoint `@Failure 402`
// annotations would silently rot as routes are added.
//
// The rule is exact in both directions: every authenticated POST that is not on
// the read-only allowlist MUST document 402, and the exempt ones (public group +
// allowlist) must NOT — a stray annotation there would tell clients to expect a
// status that endpoint can never return.
func TestGuard_EveryRestrictedPostDocuments402(t *testing.T) {
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

	public := publicPostRoutes(t)
	var missing, unexpected, undocumented []string
	for route := range registeredRoutes(t) {
		method, path, ok := strings.Cut(route, " ")
		if !ok || method != "POST" {
			continue
		}
		op, found := spec.Paths[path]["post"]
		if !found {
			undocumented = append(undocumented, path)
			continue
		}
		_, has402 := op.Responses["402"]
		exempt := public[path] || middleware.ReadonlyAllowedPaths[path]
		switch {
		case exempt && has402:
			unexpected = append(unexpected, path)
		case !exempt && !has402:
			missing = append(missing, path)
		}
	}
	sort.Strings(missing)
	sort.Strings(unexpected)
	sort.Strings(undocumented)

	if len(undocumented) > 0 {
		t.Errorf("POST routes absent from swagger.json (add swag annotations, then `make swagger`):\n  %s",
			strings.Join(undocumented, "\n  "))
	}
	if len(missing) > 0 {
		t.Errorf("restricted POST routes missing `@Failure 402 {object} apidoc.JsonResponseError` (add it, then `make swagger`) — UNLESS the route is meant to be exempt: public routes are classified by their registration line lacking the auth( wrapper, and allowlisted ones live on middleware.ReadonlyAllowedPaths; fix the registration/allowlist instead of annotating a route that cannot return 402:\n  %s",
			strings.Join(missing, "\n  "))
	}
	if len(unexpected) > 0 {
		t.Errorf("these POST routes document 402 but can never return it — they are public or on middleware.ReadonlyAllowedPaths (drop the annotation, then `make swagger`):\n  %s",
			strings.Join(unexpected, "\n  "))
	}
}
