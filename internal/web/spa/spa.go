// Package spa serves the built single-page application from a directory on
// disk (web/dist) with SPA history-mode fallback: any request
// that does not map to an existing file and is not an API or internal route is
// served index.html so the client-side router can take over.
package spa

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// indexFile is the SPA entrypoint served for client-routed paths.
const indexFile = "index.html"

// Handler returns an http.Handler that serves static files from dir, falling
// back to index.html for unknown paths (SPA history mode). Requests under /api
// or /_ are never rewritten to index.html (they should be handled by the API /
// internal routes; if they reach here they 404 honestly rather than masquerade
// as the SPA shell).
func Handler(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean the request path to prevent directory traversal. path.Clean on
		// an absolute-rooted path collapses ".." segments safely.
		upath := r.URL.Path
		if !strings.HasPrefix(upath, "/") {
			upath = "/" + upath
		}
		cleaned := path.Clean(upath)

		// API and internal routes must not fall back to the SPA shell.
		if isReservedPath(cleaned) {
			http.NotFound(w, r)
			return
		}

		// Map the cleaned URL path onto a filesystem path within dir. filepath
		// .Clean + the leading-slash trim keep us anchored under dir.
		rel := strings.TrimPrefix(cleaned, "/")
		fsPath := filepath.Join(dir, filepath.FromSlash(rel))

		if fileExists(fsPath) {
			fs.ServeHTTP(w, r)
			return
		}

		// A missing path that LOOKS like a static asset (has a file extension) must
		// 404, not fall back to the SPA shell. Returning index.html (200) for a
		// missing .svg/.js/.png masks the error: an <object data="...">, <img>, or
		// fetch() for that asset receives HTML with a 200 and never triggers its
		// error/fallback path. (Concretely: the app-header logo uses
		// <object data="~assets/econumo.svg"> with an <img> fallback; under nginx
		// the missing data URL 404'd so the <img> rendered, but the SPA-shell
		// fallback hid that 404 and the logo vanished.) Client routes are
		// extensionless and still fall through to index.html below.
		if path.Ext(cleaned) != "" {
			http.NotFound(w, r)
			return
		}

		// SPA fallback: serve index.html for client-side routes.
		http.ServeFile(w, r, filepath.Join(dir, indexFile))
	})
}

// isReservedPath reports whether the path belongs to a server-side route group
// (API or internal) that must never be served the SPA shell.
func isReservedPath(p string) bool {
	return p == "/api" || strings.HasPrefix(p, "/api/") ||
		p == "/_" || strings.HasPrefix(p, "/_/")
}

// fileExists reports whether name is an existing regular file (not a
// directory). Directories fall through to the SPA fallback so that e.g.
// "/accounts" does not accidentally serve a directory listing.
func fileExists(name string) bool {
	info, err := os.Stat(name)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
