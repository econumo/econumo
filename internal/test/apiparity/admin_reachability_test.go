package apiparity

import (
	"net/http"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
)

// The whole admin design rests on those routes being unreachable from the
// public mux: a misconfigured reverse proxy must not be able to expose them,
// because they are not registered there at all. This asserts that property
// instead of leaving it to be maintained by hand.
//
// Paired with the internal/admin exclusion in guard_test.go's route scan.
func TestAdminRoutesAreNotOnThePublicMux(t *testing.T) {
	h := NewHarness(t, dbtest.New(t))

	// Bare /admin/* lives outside the /api/ subtree, so the SPA catch-all owns
	// it. The catch-all never emits our JSON envelope — so ANY envelope here
	// means something server-side claimed the path.
	t.Run("outside the api subtree", func(t *testing.T) {
		for _, tc := range []struct{ method, path string }{
			{http.MethodPost, "/admin/set-access"},
			{http.MethodGet, "/admin/user-context"},
			{http.MethodGet, "/admin/user-context?userId=" + OwnerID},
		} {
			// Present the admin bearer: if the admin chain were mounted here it
			// would authenticate and serve rather than fall through.
			status, body := h.Call(t, tc.method, tc.path, adminBearer, nil)
			if strings.Contains(string(body), `"success"`) {
				t.Fatalf("%s %s is served by a handler on the public mux (%d): %s",
					tc.method, tc.path, status, body)
			}
		}
	})

	// Under /api/ the mux answers with a JSON envelope for everything, so the
	// signal is narrower: an admin route registered here would either succeed or
	// challenge for the admin credential.
	t.Run("inside the api subtree", func(t *testing.T) {
		for _, tc := range []struct{ method, path string }{
			{http.MethodPost, "/api/v1/admin/set-access"},
			{http.MethodGet, "/api/v1/admin/user-context"},
			{http.MethodGet, "/api/v1/admin/user-context?userId=" + OwnerID},
		} {
			status, body := h.Call(t, tc.method, tc.path, adminBearer, nil)
			if status != http.StatusNotFound {
				t.Fatalf("%s %s answered %d, want 404 — nothing should serve it: %s",
					tc.method, tc.path, status, body)
			}
			if strings.Contains(string(body), `accessLevel`) {
				t.Fatalf("%s %s returned admin data: %s", tc.method, tc.path, body)
			}
		}
	})
}

// adminBearer is the token the apiparity harness configures as
// cfg.AdminToken, so these probes carry a credential that WOULD work if the
// admin chain were reachable.
const adminBearer = "0123456789abcdef0123456789abcdef"
