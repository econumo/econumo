// Package spa serves the built single-page application from an fs.FS (the
// SPA embedded in the binary, or a directory on disk) with SPA history-mode
// fallback: any request that does not map to an existing file and is not an
// API or internal route is served index.html so the client-side router can
// take over.
package spa

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// indexFile is the SPA entrypoint served for client-routed paths.
const indexFile = "index.html"

// Handler returns an http.Handler that serves static files from fsys, falling
// back to index.html for unknown paths (SPA history mode). Requests under /api
// or /_ are never rewritten to index.html (they should be handled by the API /
// internal routes; if they reach here they 404 honestly rather than masquerade
// as the SPA shell).
func Handler(fsys fs.FS, overrides map[string]any) http.Handler {
	fileServer := http.FileServerFS(fsys)

	// The runtime config is the one templated response: the dist file plus a
	// merge of the server-owned keys, so the instance's environment genuinely
	// controls the shipped SPA. Overrides are fixed for the process lifetime,
	// so the merge line is built once here (encoding/json sorts map keys —
	// the output is deterministic). Keys the server does not own stay
	// whatever the dist file says.
	var configSuffix []byte
	if len(overrides) > 0 {
		merged, err := json.Marshal(overrides)
		if err != nil {
			panic(fmt.Sprintf("spa: unmarshalable config overrides: %v", err))
		}
		configSuffix = fmt.Appendf(nil, "\nObject.assign(window.econumoConfig, %s);\n", merged)
	}
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

		if cleaned == "/econumo-config.js" && configSuffix != nil {
			serveRuntimeConfig(w, r, fsys, configSuffix)
			return
		}

		// Map the cleaned URL path onto an fs.FS name. path.Clean already
		// collapsed any ".." against the leading "/", but fs.ValidPath
		// re-asserts containment (rooted, no ".."), so the lookup can never
		// escape fsys even if the cleaning above is later weakened.
		name := strings.TrimPrefix(cleaned, "/")
		if name == "" {
			name = "."
		}
		if !fs.ValidPath(name) {
			http.NotFound(w, r)
			return
		}

		if fileExists(fsys, name) {
			setCacheControl(w, cleaned)
			fileServer.ServeHTTP(w, r)
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
		setCacheControl(w, "/"+indexFile)
		http.ServeFileFS(w, r, fsys, indexFile)
	})
}

func serveRuntimeConfig(w http.ResponseWriter, r *http.Request, fsys fs.FS, configSuffix []byte) {
	content, err := fs.ReadFile(fsys, "econumo-config.js")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(content)
	w.Write(configSuffix)
}

// setCacheControl picks the caching policy by path. Vite-fingerprinted files
// under /assets/ are content-addressed, so they never change and cache forever.
// Everything else (index.html, econumo-config.js, manifest, icons) keeps its
// name across deploys and must revalidate on every load: without an explicit
// Cache-Control, iOS home-screen web apps heuristically cache the shell across
// launches and keep running the old bundle until the icon is re-added.
// no-cache still allows storing — revalidation is a cheap 304 via
// Last-Modified/If-Modified-Since.
func setCacheControl(w http.ResponseWriter, cleaned string) {
	if strings.HasPrefix(cleaned, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
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
func fileExists(fsys fs.FS, name string) bool {
	info, err := fs.Stat(fsys, name)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
