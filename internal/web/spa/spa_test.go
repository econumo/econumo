package spa

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// newSPAFS builds a minimal built-SPA layout (index.html + one real asset)
// as an in-memory fs.FS.
func newSPAFS(t *testing.T) fstest.MapFS {
	t.Helper()
	return fstest.MapFS{
		"index.html":                {Data: []byte("<!doctype html><title>spa</title>")},
		"assets/econumo.abc123.svg": {Data: []byte("<svg/>")},
		"econumo-config.js":         {Data: []byte("window.econumoConfig={}")},
	}
}

func get(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code, rec.Body.String()
}

func TestSPA_Serving(t *testing.T) {
	h := Handler(newSPAFS(t), nil)

	cases := []struct {
		name     string
		path     string
		wantCode int
		wantBody string // substring; "" = don't check
	}{
		{"existing asset", "/assets/econumo.abc123.svg", http.StatusOK, "<svg/>"},
		{"index served at root", "/", http.StatusOK, "<title>spa</title>"},
		{"client route -> index.html", "/accounts", http.StatusOK, "<title>spa</title>"},
		{"nested client route -> index.html", "/budget/123", http.StatusOK, "<title>spa</title>"},
		// The regression: a MISSING asset-looking path must 404, not return the SPA
		// shell — otherwise <object data>/<img> fallbacks break (the app logo bug).
		{"missing svg -> 404", "/~assets/econumo.svg", http.StatusNotFound, ""},
		{"missing js -> 404", "/assets/missing.js", http.StatusNotFound, ""},
		{"missing png -> 404", "/img/nope.png", http.StatusNotFound, ""},
		// API / internal routes never masquerade as the SPA shell.
		{"api 404", "/api/v1/whatever", http.StatusNotFound, ""},
		{"reserved internal 404", "/_/anything", http.StatusNotFound, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, body := get(t, h, tc.path)
			if code != tc.wantCode {
				t.Errorf("%s: code = %d, want %d", tc.path, code, tc.wantCode)
			}
			if tc.wantBody != "" && !strings.Contains(body, tc.wantBody) {
				t.Errorf("%s: body %q missing %q", tc.path, body, tc.wantBody)
			}
		})
	}
}

// Without an explicit Cache-Control, iOS home-screen web apps heuristically
// cache index.html across launches and keep serving a stale shell (pointing at
// old hashed bundles) long after a deploy. The shell and other non-fingerprinted
// files must force revalidation; Vite-fingerprinted /assets/ files are immutable.
func TestSPA_CacheHeaders(t *testing.T) {
	h := Handler(newSPAFS(t), nil)

	cases := []struct {
		name string
		path string
		want string
	}{
		{"index at root revalidates", "/", "no-cache"},
		{"index direct revalidates", "/index.html", "no-cache"},
		{"client route fallback revalidates", "/accounts", "no-cache"},
		{"runtime config revalidates", "/econumo-config.js", "no-cache"},
		{"hashed asset immutable", "/assets/econumo.abc123.svg", "public, max-age=31536000, immutable"},
		{"missing asset 404 not immutable", "/assets/missing.js", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if got := rec.Header().Get("Cache-Control"); got != tc.want {
				t.Errorf("%s: Cache-Control = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestSPA_RuntimeConfigOverride(t *testing.T) {
	h := Handler(newSPAFS(t), map[string]any{"ANALYTICS": false, "ALLOW_REGISTRATION": true})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/econumo-config.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(body, "window.econumoConfig={}") {
		t.Fatalf("body does not start with the dist config: %q", body)
	}
	// encoding/json sorts map keys, so the merge line is deterministic.
	want := `Object.assign(window.econumoConfig, {"ALLOW_REGISTRATION":true,"ANALYTICS":false});`
	if !strings.Contains(body, want) {
		t.Fatalf("body missing %q: %q", want, body)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q, want %q", got, "no-cache")
	}
	if got := rec.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
}

func TestSPA_RuntimeConfigNoOverrides(t *testing.T) {
	h := Handler(newSPAFS(t), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/econumo-config.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "window.econumoConfig={}" {
		t.Fatalf("body = %q, want the dist file verbatim", got)
	}
}

func TestSPA_RuntimeConfigMissingFile(t *testing.T) {
	h := Handler(fstest.MapFS{}, map[string]any{"ANALYTICS": true})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/econumo-config.js", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// Traversal attempts must never leak content from outside the served FS. In
// a normal HTTP request path.Clean collapses any ".." against the leading
// "/", so these payloads are refused before reaching the handler's
// fs.ValidPath backstop (they 4xx). This asserts the observable invariant:
// no payload yields file content other than the SPA shell. TestValidPathGate
// below covers the fs.ValidPath backstop line directly, standing in for the
// removed resolvePath unit tests.
func TestSPA_TraversalAttempts(t *testing.T) {
	h := Handler(newSPAFS(t), nil)
	for _, p := range []string{"/../etc/passwd", "/..", "/a/../../etc/passwd", "/%2e%2e/etc/passwd"} {
		code, body := get(t, h, p)
		// Legal outcomes: 404, or the SPA shell for an extensionless clean
		// result — never file content from outside the fixture FS.
		if code == http.StatusOK && !strings.Contains(body, "<title>spa</title>") {
			t.Errorf("%s: served unexpected content: %q", p, body)
		}
	}
}

// TestValidPathGate covers the fs.ValidPath containment backstop directly:
// the handler maps a cleaned URL path onto an fs name and refuses it unless
// fs.ValidPath accepts it, so any name carrying a "." or ".." element — the
// forms an escape would take — must be rejected. Reached via HTTP only if
// path.Clean is ever weakened, so it is asserted here in isolation.
func TestValidPathGate(t *testing.T) {
	rejected := []string{"..", "../etc/passwd", "../../root/.ssh/id_rsa", "a/../../etc/passwd", "./x", "/etc/passwd"}
	for _, name := range rejected {
		if fs.ValidPath(name) {
			t.Errorf("fs.ValidPath(%q) = true, want false (escape/non-rooted name must be refused)", name)
		}
	}
	accepted := []string{"index.html", "assets/app.js", "econumo-config.js"}
	for _, name := range accepted {
		if !fs.ValidPath(name) {
			t.Errorf("fs.ValidPath(%q) = false, want true (legitimate asset name)", name)
		}
	}
}
