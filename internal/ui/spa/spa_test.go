package spa

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newSPADir writes a minimal built-SPA layout (index.html + one real asset) to a
// temp dir and returns it.
func newSPADir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>spa</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "econumo.abc123.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func get(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code, rec.Body.String()
}

func TestSPA_Serving(t *testing.T) {
	h := Handler(newSPADir(t))

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
		{"internal 404", "/_/health-check", http.StatusNotFound, ""},
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
