# Single-Binary Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship self-contained Linux release binaries (SPA embedded via `go:embed`) attached to every GitHub release, plus a systemd-based bare-metal deployment story, with the Docker image converging on the same embedded-SPA code path.

**Architecture:** A new repo-root `web` Go package embeds `web/dist`; `internal/web/spa` refactors from disk paths onto `fs.FS`; the composition root selects embedded-vs-disk (`ECONUMO_WEB_DIST` override wins). A new `internal/version` package is stamped by ldflags. The release workflow gains a `build-binaries` job whose artifacts are uploaded to the GitHub release.

**Tech Stack:** Go stdlib (`embed`, `io/fs`, `http.FileServerFS`), GitHub Actions, systemd. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-07-22-single-binary-deployment-design.md`

**Branch:** work on `feature/single-binary-deployment` (the spec is on `feature/single-binary-deployment-spec`, PR #138 — branch the implementation from `main` once that merges, or stack on the spec branch).

## Global Constraints

- Platforms: `linux/amd64` + `linux/arm64` only. `CGO_ENABLED=0` everywhere.
- Artifact names are frozen: `econumo-linux-amd64`, `econumo-linux-arm64`, `SHA256SUMS`.
- ldflags variable path (exact): `github.com/econumo/econumo/internal/version.Version`.
- SPA serving semantics are frozen (history fallback, asset-ext 404, reserved `/api` `/_` 404, cache headers, `econumo-config.js` merge) — the `fs.FS` refactor must not change any observable behavior; existing test assertions carry over verbatim.
- Runtime SPA source selection (exact order): explicit non-empty `ECONUMO_WEB_DIST` → embedded dist if it contains `index.html` → disk `web/dist` default.
- Comments: only non-obvious why (repo comment policy); Swagger `@` blocks untouched.
- `make go-test` must pass with NO frontend build present (the placeholder embed keeps Go builds frontend-free).
- Run `make go-test` before every commit that touches Go code.

---

### Task 1: `internal/version` package + `version` subcommand

**Files:**
- Create: `internal/version/version.go`
- Create: `internal/version/version_test.go`
- Modify: `cmd/econumo/main.go` (command switch ~line 62, `printUsage` ~line 98, `run()` boot log ~line 157)

**Interfaces:**
- Produces: `version.Version` (package-level `var Version string`, default `"dev"`), stamped via `-ldflags "-X github.com/econumo/econumo/internal/version.Version=vX.Y.Z"`. Tasks 5–7 use this exact path in build commands.
- Produces: `econumo version` subcommand printing `econumo <version>\n` to stdout, exit 0.

- [ ] **Step 1: Write the failing test**

Create `internal/version/version_test.go`:

```go
package version

import "testing"

func TestDefaultIsDev(t *testing.T) {
	if Version != "dev" {
		t.Fatalf("Version = %q, want %q (the unstamped default)", Version, "dev")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/version/`
Expected: FAIL — `no Go files in .../internal/version` (package doesn't exist yet)

- [ ] **Step 3: Create the package**

Create `internal/version/version.go`:

```go
// Package version carries the build-stamped binary version. Release builds
// overwrite the default at link time:
//
//	go build -ldflags "-X github.com/econumo/econumo/internal/version.Version=v1.2.3"
package version

// Version is "dev" unless stamped by -ldflags at build time.
var Version = "dev"
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/version/`
Expected: PASS

- [ ] **Step 5: Wire the subcommand, usage line, and boot-log field**

In `cmd/econumo/main.go`:

a) Add to the import block: `"github.com/econumo/econumo/internal/version"`

b) In the `switch args[0]` in `main()`, add a case after the `"healthcheck"` case:

```go
	case "version", "-version", "--version":
		fmt.Println("econumo " + version.Version)
		os.Exit(0)
```

c) In `printUsage`, after the `healthcheck` line, add:

```go
	fmt.Fprintf(w, "  %-40s %s\n", "version", "Print the binary version")
```

d) In `run()`, extend the `configuration loaded` log line with the version field:

```go
	slog.Info("configuration loaded",
		"database_driver", cfg.DatabaseDriver,
		"spa_dir", cfg.SPADir,
		"version", version.Version,
	)
```

- [ ] **Step 6: Verify the subcommand end-to-end**

Run: `go run ./cmd/econumo version`
Expected output: `econumo dev`

Run: `go run ./cmd/econumo help | grep -A1 healthcheck`
Expected: the `version   Print the binary version` line appears.

Run: `go build -ldflags "-X github.com/econumo/econumo/internal/version.Version=v9.9.9" -o /tmp/econumo-vtest ./cmd/econumo && /tmp/econumo-vtest version && rm /tmp/econumo-vtest`
Expected output: `econumo v9.9.9`

- [ ] **Step 7: Smoke suite + commit**

Run: `make go-test`
Expected: PASS (coverage gate included)

```bash
git add internal/version/ cmd/econumo/main.go
git commit -m "feat: version subcommand with ldflags-stamped internal/version"
```

---

### Task 2: `web` embed package (`DistFS`/`SelectFS`) + placeholder

**Files:**
- Create: `web/embed.go`
- Create: `web/embed_test.go`
- Create: `web/dist/.gitkeep` (empty file, committed)
- Create: `web/public/.gitkeep` (empty file, committed)
- Modify: `web/.gitignore` (the `dist` line)

**Interfaces:**
- Produces: `web.DistFS() (fs.FS, bool)` — embedded SPA rooted at `dist/`; bool = a real build is present (`index.html` exists).
- Produces: `web.SelectFS(dir string, explicit bool) (fs.FS, string)` — the FS to serve and a log label (`"embedded"` or the dir path). Task 4 calls this from `internal/server` and `cmd/econumo`.

**Why two `.gitkeep` files:** `go:embed all:dist` fails to compile if `dist/` has no files, so a placeholder must be committed at `web/dist/.gitkeep`. But `pnpm build` empties `dist/` before writing — which would delete the tracked placeholder and leave the worktree dirty after every frontend build. Vite copies `web/public/` verbatim into `dist/`, so a second empty `.gitkeep` in `public/` makes every build regenerate the identical placeholder: worktree stays clean.

- [ ] **Step 1: Placeholders + gitignore**

```bash
touch web/dist/.gitkeep web/public/.gitkeep
```

In `web/.gitignore`, replace the line `dist` with:

```
dist/*
!dist/.gitkeep
```

(The `dist-ssr` line below it stays. The old bare `dist` pattern ignored the whole directory, and git cannot re-include a file inside a wholly-ignored directory; `dist/*` ignores the contents instead, letting the `!` exception through.)

Verify: `git check-ignore web/dist/anything; git check-ignore web/dist/.gitkeep || echo "gitkeep tracked"`
Expected: first prints `web/dist/anything`; second prints `gitkeep tracked`.

- [ ] **Step 2: Write the failing test**

Create `web/embed_test.go`:

```go
package web

import (
	"io/fs"
	"testing"
)

// The committed placeholder (.gitkeep) must not read as a real SPA build —
// otherwise a source-checkout binary would serve an empty shell instead of
// falling back to the disk dist.
func TestDistFS_PlaceholderIsNotABuild(t *testing.T) {
	sub, ok := DistFS()
	if sub == nil {
		t.Fatal("DistFS returned a nil fs")
	}
	if _, err := fs.Stat(sub, "index.html"); err == nil {
		// A dev machine with a built SPA embeds the real thing; the placeholder
		// assertion only applies to a frontend-free checkout (CI smoke).
		if !ok {
			t.Fatal("index.html embedded but DistFS reported no build")
		}
		t.Skip("real SPA build present in web/dist")
	}
	if ok {
		t.Fatal("DistFS reported a real build for a placeholder-only embed")
	}
}

func TestSelectFS(t *testing.T) {
	// An explicitly configured directory always wins, embed or not.
	if _, label := SelectFS("/some/dir", true); label != "/some/dir" {
		t.Fatalf("explicit dir: label = %q, want /some/dir", label)
	}
	// Without an explicit dir the outcome depends on whether a build is
	// embedded (dev machines may have one); label and DistFS must agree.
	fsys, label := SelectFS("web/dist", false)
	if fsys == nil {
		t.Fatal("SelectFS returned a nil fs")
	}
	if _, ok := DistFS(); ok && label != "embedded" {
		t.Fatalf("embedded build present but label = %q", label)
	} else if !ok && label != "web/dist" {
		t.Fatalf("no embedded build but label = %q, want web/dist", label)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./web/`
Expected: FAIL — `no Go files in .../web` (package doesn't exist yet)

- [ ] **Step 4: Implement the embed package**

Create `web/embed.go`:

```go
// Package web embeds the built SPA (dist/, the pnpm build output) so a
// release binary is fully self-contained. A source checkout without a
// frontend build embeds only the committed placeholder, which DistFS
// reports as "no build".
package web

import (
	"embed"
	"io/fs"
	"os"
)

//go:embed all:dist
var dist embed.FS

// DistFS returns the embedded SPA rooted at dist/ and whether a real build
// is present (a placeholder-only embed has no index.html).
func DistFS() (fs.FS, bool) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, false
	}
	_, err = fs.Stat(sub, "index.html")
	return sub, err == nil
}

// SelectFS picks the filesystem the SPA is served from: an explicitly
// configured directory always wins (dev override, separately-hosted SPA);
// otherwise the embedded build when present; otherwise the disk default —
// a source checkout with a built SPA but a placeholder-only binary. The
// returned label names the source for the boot log.
func SelectFS(dir string, explicit bool) (fs.FS, string) {
	if !explicit {
		if sub, ok := DistFS(); ok {
			return sub, "embedded"
		}
	}
	return os.DirFS(dir), dir
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./web/`
Expected: PASS (or SKIP on the placeholder test if a local `web/dist` build exists — both are green)

- [ ] **Step 6: Smoke suite + commit**

Run: `make go-test`
Expected: PASS

```bash
git add web/embed.go web/embed_test.go web/dist/.gitkeep web/public/.gitkeep web/.gitignore
git commit -m "feat: embed the built SPA via go:embed (web package, placeholder-safe)"
```

---

### Task 3: Refactor `internal/web/spa` onto `fs.FS`

**Files:**
- Modify: `internal/web/spa/spa.go` (full rewrite below)
- Modify: `internal/web/spa/spa_test.go` (full rewrite below)
- Modify: `internal/web/router/router.go:151` (call site — interim `os.DirFS`, replaced in Task 4)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `spa.Handler(fsys fs.FS, overrides map[string]any) http.Handler` — Task 4 passes it the FS selected by `web.SelectFS`.

- [ ] **Step 1: Rewrite the tests against `fs.FS` fixtures**

Replace the entire content of `internal/web/spa/spa_test.go` with:

```go
package spa

import (
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

// fs.ValidPath is the containment boundary now: path.Clean collapses ".."
// against the leading "/", and any name that still is not a valid rooted fs
// path is refused before lookup. Nothing may escape fsys, however the URL
// path is spelled.
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/web/spa/`
Expected: FAIL to compile — `Handler` still takes a `string` dir.

- [ ] **Step 3: Rewrite the handler onto `fs.FS`**

Replace the entire content of `internal/web/spa/spa.go` with:

```go
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
```

(Deliberately gone: `resolvePath` and the `os`/`path/filepath` imports — `fs.ValidPath` plus the rooted `fs.FS` make directory escape structurally impossible.)

- [ ] **Step 4: Update the router call site (interim)**

In `internal/web/router/router.go`, change line 151 from:

```go
	root.Handle("/", spa.Handler(deps.Cfg.SPADir, overrides))
```

to:

```go
	root.Handle("/", spa.Handler(os.DirFS(deps.Cfg.SPADir), overrides))
```

and add `"os"` to the router's import block. (Behavior is identical to today; Task 4 replaces this with the composition-root selection.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/web/spa/ ./internal/web/router/`
Expected: PASS

- [ ] **Step 6: Smoke suite + commit**

Run: `make go-test`
Expected: PASS (apiparity goldens unchanged — the SPA wire behavior is identical)

```bash
git add internal/web/spa/ internal/web/router/router.go
git commit -m "refactor: serve the SPA from an fs.FS (disk or embedded)"
```

---

### Task 4: Runtime source selection (config + router + server + boot log)

**Files:**
- Modify: `internal/config/config.go` (Config struct ~line 65, `Load` ~line 98)
- Test: `internal/config/config_test.go` (add one test to the existing file)
- Modify: `internal/web/router/router.go` (Deps struct ~line 61, SPA call site from Task 4, imports)
- Modify: `internal/server/server.go` (~line 283, where `router.Deps` is built)
- Modify: `cmd/econumo/main.go` (`run()` boot log)

**Interfaces:**
- Consumes: `web.SelectFS(dir string, explicit bool) (fs.FS, string)` from Task 2; `spa.Handler(fs.FS, map[string]any)` from Task 3.
- Produces: `config.Config.SPADirSet bool`; `router.Deps.SPA fs.FS`.

- [ ] **Step 1: Write the failing config test**

In `internal/config/config_test.go`, add (test env is clean; `Load` hard-requires only `DATABASE_URL` — if this repo's existing config tests use a shared setenv helper, reuse it for these two `Load` calls):

```go
func TestSPADirExplicit(t *testing.T) {
	t.Setenv("DATABASE_URL", "sqlite:///tmp/econumo-test.sqlite")

	// Unset (and set-empty, matching getEnv semantics) = the default, not explicit.
	t.Setenv("ECONUMO_WEB_DIST", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SPADirSet || cfg.SPADir != "web/dist" {
		t.Fatalf("unset: SPADir = %q, SPADirSet = %v; want web/dist, false", cfg.SPADir, cfg.SPADirSet)
	}

	t.Setenv("ECONUMO_WEB_DIST", "/srv/spa")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SPADirSet || cfg.SPADir != "/srv/spa" {
		t.Fatalf("set: SPADir = %q, SPADirSet = %v; want /srv/spa, true", cfg.SPADir, cfg.SPADirSet)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestSPADirExplicit`
Expected: FAIL to compile — `cfg.SPADirSet undefined`

- [ ] **Step 3: Add the config field**

In `internal/config/config.go`, change the SPA section of the struct:

```go
	// SPA
	SPADir    string // disk path to a built SPA (dev default web/dist)
	SPADirSet bool   // ECONUMO_WEB_DIST was set: the disk dir overrides the embedded SPA
```

and in `Load`, next to the existing `SPADir:` line, add to the struct literal:

```go
		SPADirSet:              os.Getenv("ECONUMO_WEB_DIST") != "",
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/`
Expected: PASS

- [ ] **Step 5: Thread the selected FS through router deps**

a) In `internal/web/router/router.go`, add to the `Deps` struct:

```go
	// SPA is the filesystem the SPA catch-all serves — the embedded build or
	// a disk directory, selected by the composition root (web.SelectFS).
	// Nil falls back to Cfg.SPADir on disk.
	SPA fs.FS
```

with `"io/fs"` added to the imports.

b) Replace the Task 3 interim call site:

```go
	spaFS := deps.SPA
	if spaFS == nil {
		spaFS = os.DirFS(deps.Cfg.SPADir)
	}
	root.Handle("/", spa.Handler(spaFS, overrides))
```

(`"os"` stays imported for the fallback.)

c) In `internal/server/server.go`, in `BuildAPI` where `router.Deps` is built (~line 283), add the field and the selection:

```go
	spaFS, _ := web.SelectFS(cfg.SPADir, cfg.SPADirSet)
	return router.New(router.Deps{
		Cfg:                cfg,
		DB:                 pinger{db},
		RegisterAPI:        registerAPI,
		SupportedLanguages: i18n.Supported,
		MCP:                mcpHandler,
		SPA:                spaFS,
	}), adminHandler
```

with `"github.com/econumo/econumo/web"` added to the server's imports.

d) In `cmd/econumo/main.go` `run()`, replace the `"spa_dir", cfg.SPADir` log field with the selected source (add `"github.com/econumo/econumo/web"` to imports):

```go
	_, spaSource := web.SelectFS(cfg.SPADir, cfg.SPADirSet)
	slog.Info("configuration loaded",
		"database_driver", cfg.DatabaseDriver,
		"spa_source", spaSource,
		"version", version.Version,
	)
```

- [ ] **Step 6: Smoke suite (includes archtest) + commit**

Run: `make go-test`
Expected: PASS — archtest must not object to `internal/server` and `cmd` importing the repo-root `web` package (it is not a feature package; if archtest flags it, the fix is adding the `web` module-root package to archtest's infrastructure allowlist, NOT weakening the feature rules).

Manual check: `go run ./cmd/econumo serve` from the repo root with a valid `.env`, then confirm the boot line logs `spa_source=web/dist` (no embed, env unset) or `spa_source=embedded` (after a `make web-bundle` + rebuild), and `curl -s localhost:8181/ | head -1` returns the SPA shell. Stop the server.

```bash
git add internal/config/ internal/web/router/router.go internal/server/server.go cmd/econumo/main.go
git commit -m "feat: select SPA source at boot - explicit dir, embedded build, disk default"
```

---

### Task 5: Dockerfile convergence (embed the SPA, stamp the version)

**Files:**
- Modify: `deployment/docker/Dockerfile`

**Interfaces:**
- Consumes: `web/embed.go` (Task 2), `internal/version` ldflags path (Task 1).

- [ ] **Step 1: Move the dist into the Go build and stamp the version**

In the `gobuild` stage, after `COPY locales/ ./locales/`, replace:

```dockerfile
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /app/econumo ./cmd/econumo
```

with:

```dockerfile
# The built SPA is embedded into the binary (web/embed.go, go:embed): the
# image and the downloadable release binary share one code path, and the
# runtime stage no longer carries a separate /app/web.
COPY web/embed.go ./web/embed.go
COPY --from=frontend /build/web/dist ./web/dist
ARG TARGETOS TARGETARCH
ARG ECONUMO_VERSION=""
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags="-s -w -X github.com/econumo/econumo/internal/version.Version=${ECONUMO_VERSION:-dev}" \
    -o /app/econumo ./cmd/econumo
```

- [ ] **Step 2: Slim the runtime stage**

In the `prod` stage, delete these two lines:

```dockerfile
COPY --from=frontend /build/web/dist /app/web
```

and change:

```dockerfile
ENV ECONUMO_WEB_DIST=/app/web \
    PORT=80
```

to:

```dockerfile
ENV PORT=80
```

Also update the file's header comment first paragraph to say the SPA is embedded in the binary (it currently says "one static Go binary serving the JSON API + SPA" — append ", SPA embedded via go:embed").

- [ ] **Step 3: Verify the image builds and serves the embedded SPA**

```bash
docker build -f deployment/docker/Dockerfile --build-arg ECONUMO_VERSION=v0.0.0-embedtest -t econumo:embedtest .
docker run --rm econumo:embedtest version
```

Expected: `econumo v0.0.0-embedtest`

```bash
docker run --rm -d --name econumo-embedtest -p 8182:80 -e DATABASE_URL=sqlite:///app/var/db/db.sqlite econumo:embedtest serve
sleep 2
curl -sf localhost:8182/ | grep -o '<title>[^<]*</title>'
curl -sf localhost:8182/econumo-config.js | tail -1
docker logs econumo-embedtest 2>&1 | grep spa_source
docker stop econumo-embedtest
```

Expected: an HTML `<title>`, an `Object.assign(window.econumoConfig, …)` line, and `spa_source=embedded` in the boot log.

- [ ] **Step 4: Commit**

```bash
git add deployment/docker/Dockerfile
git commit -m "feat: docker image serves the embedded SPA, drops /app/web"
```

---

### Task 6: `make release-binaries` + gitignore

**Files:**
- Modify: `Makefile` (`.PHONY` line 1, help text ~line 9, new target after `go-build`)
- Modify: `.gitignore` (root)

**Interfaces:**
- Consumes: `internal/version` ldflags path (Task 1), the embed (Task 2).
- Produces: `release-out/econumo-linux-{amd64,arm64}` + `release-out/SHA256SUMS` — the exact filenames Task 7's workflow job replicates and uploads.

- [ ] **Step 1: Add the target**

In `Makefile`: add `release-binaries` to the `.PHONY` list, add a help line `@echo "  make release-binaries - Cross-compile the downloadable release binaries (SPA embedded)"`, and add after the `go-build` target:

```make
# Cross-compile the downloadable release binaries exactly as the release
# workflow does: build the SPA with the version label (embedded via
# web/embed.go), then linux/amd64 + linux/arm64 binaries (CGO off, version
# stamped through ldflags) and SHA256SUMS into release-out/. VERSION
# defaults to dev for local verification of the artifact shape.
VERSION ?= dev
RELEASE_LDFLAGS = -s -w -X github.com/econumo/econumo/internal/version.Version=$(VERSION)

release-binaries: swagger
	cd web && pnpm install --frozen-lockfile && ECONUMO_VERSION=$(VERSION) pnpm run build
	rm -rf release-out && mkdir -p release-out
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(RELEASE_LDFLAGS)" -o release-out/econumo-linux-amd64 ./cmd/econumo
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(RELEASE_LDFLAGS)" -o release-out/econumo-linux-arm64 ./cmd/econumo
	cd release-out && sha256sum econumo-linux-amd64 econumo-linux-arm64 > SHA256SUMS
```

In root `.gitignore`, under the `# Go build/test artifacts` section, add:

```
/release-out/
```

- [ ] **Step 2: Build and verify the artifacts end-to-end**

Run: `make release-binaries VERSION=v0.0.0-local`
Expected: `release-out/` contains the two binaries + `SHA256SUMS`; `cd release-out && sha256sum -c SHA256SUMS` passes.

Boot the amd64 binary from a scratch dir (so the repo `.env` is not picked up, and no disk dist exists — proving the embed):

```bash
mkdir -p /tmp/econumo-bare && cd /tmp/econumo-bare
PORT=8899 DATABASE_URL=sqlite:///tmp/econumo-bare/db.sqlite \
  /path/to/repo/release-out/econumo-linux-amd64 serve &
sleep 2
/path/to/repo/release-out/econumo-linux-amd64 version
curl -sf localhost:8899/ | grep -o '<title>[^<]*</title>'
curl -sf localhost:8899/health
kill %1
```

Expected: `econumo v0.0.0-local`, the SPA `<title>`, a healthy `/health` — and the boot log line shows `spa_source=embedded`.

- [ ] **Step 3: Restore the worktree and commit**

`make release-binaries` rebuilt `web/dist` — confirm `git status` shows ONLY the Makefile and `.gitignore` changes (the `public/.gitkeep` copy keeps `dist/.gitkeep` stable; if `.gitkeep` shows as deleted, the Task 2 vite-copy premise failed — stop and fix that instead of committing).

```bash
git add Makefile .gitignore
git commit -m "feat: make release-binaries cross-compiles the single-file artifacts"
```

---

### Task 7: Release workflow — build and attach binaries

**Files:**
- Modify: `.github/workflows/publish-release.yml` (new `build-binaries` job after `publish`; `create-github-release` job ~line 182)

**Interfaces:**
- Consumes: `needs.create-tag.outputs.version` (existing workflow output), artifact names from Task 6.

- [ ] **Step 1: Add the `build-binaries` job**

After the `publish` job, add:

```yaml
  build-binaries:
    name: Build Release Binaries
    needs: create-tag
    runs-on: ubuntu-latest
    steps:
      - name: Checkout tagged ref
        uses: actions/checkout@v4
        with:
          ref: ${{ needs.create-tag.outputs.version }}

      - name: Set up Node
        uses: actions/setup-node@v4
        with:
          node-version: 26

      # Mirrors the Dockerfile's frontend stage: the SPA build lands in
      # web/dist, where web/embed.go embeds it into the Go binaries below.
      - name: Build SPA
        env:
          ECONUMO_VERSION: ${{ needs.create-tag.outputs.version }}
        run: |
          npm install -g pnpm@11
          cd web
          pnpm install --frozen-lockfile
          pnpm run build

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - name: Build binaries
        env:
          VERSION: ${{ needs.create-tag.outputs.version }}
        run: |
          mkdir -p release-out
          for arch in amd64 arm64; do
            CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
              go build -trimpath \
              -ldflags "-s -w -X github.com/econumo/econumo/internal/version.Version=${VERSION}" \
              -o "release-out/econumo-linux-${arch}" ./cmd/econumo
          done
          cd release-out && sha256sum econumo-linux-amd64 econumo-linux-arm64 > SHA256SUMS

      - name: Upload artifact
        uses: actions/upload-artifact@v4
        with:
          name: release-binaries
          path: release-out/
          if-no-files-found: error
```

- [ ] **Step 2: Attach the binaries to the GitHub release**

In `create-github-release`: change `needs: [create-tag, publish]` to `needs: [create-tag, publish, build-binaries]`, and immediately after the `npx changelogithub` step (the release must exist before upload) add:

```yaml
      - name: Download release binaries
        uses: actions/download-artifact@v4
        with:
          name: release-binaries
          path: release-out

      - name: Attach binaries to the release
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          TAG: ${{ needs.create-tag.outputs.version }}
        run: |
          gh release upload "$TAG" \
            release-out/econumo-linux-amd64 \
            release-out/econumo-linux-arm64 \
            release-out/SHA256SUMS \
            --clobber
```

Also update the workflow's header comment: the first line "Publishes the Go backend image to ghcr.io/econumo/econumo ONLY." becomes "Publishes the Go backend image to ghcr.io/econumo/econumo and attaches single-file linux binaries (SPA embedded) to the GitHub release."

- [ ] **Step 3: Validate the YAML and commit**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/publish-release.yml'))" && echo OK`
Expected: `OK` (full behavioral verification happens on the next real release — the job mirrors `make release-binaries`, which Task 6 verified end-to-end).

```bash
git add .github/workflows/publish-release.yml
git commit -m "ci: build and attach linux release binaries to the GitHub release"
```

---

### Task 8: systemd unit + docs (README, .env.example, CLAUDE.md)

**Files:**
- Create: `deployment/systemd/econumo.service`
- Modify: `README.md` (new section between "Configuration" and "Localization", ~line 79)
- Modify: `.env.example` (SPA overrides area, ~line 30)
- Modify: `CLAUDE.md` (Project Overview artifact sentence; `ECONUMO_WEB_DIST` bullet; Deployment section)

**Interfaces:**
- Consumes: artifact names + `SHA256SUMS` (Task 6/7), `version` subcommand (Task 1), selection rule (Task 4).

- [ ] **Step 1: Write the unit file**

Create `deployment/systemd/econumo.service`:

```ini
# Reference unit for running the single-file Econumo binary without Docker.
# Assumes: binary at /opt/econumo/econumo, env at /etc/econumo/env (modeled on
# .env.example; at minimum DATABASE_URL and PORT), a dedicated system user
# `econumo`, and SQLite data under /var/lib/econumo. See the README section
# "Run without Docker (single binary)" for the setup walkthrough.

[Unit]
Description=Econumo personal finance server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=econumo
Group=econumo
WorkingDirectory=/var/lib/econumo
EnvironmentFile=/etc/econumo/env
ExecStart=/opt/econumo/econumo serve
Restart=on-failure
RestartSec=5

# Hardening: the process only ever writes its database directory.
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
ReadWritePaths=/var/lib/econumo

[Install]
WantedBy=multi-user.target
```

- [ ] **Step 2: Add the README section**

In `README.md`, after the "Configuration" section (the `user:create` docker-exec example) and before "### Localization", insert:

~~~markdown
### Run without Docker (single binary)

Every release also ships self-contained Linux binaries — the web UI is
embedded, so one file is the whole app. Grab the binary for your
architecture plus `SHA256SUMS` from the
[latest release](https://github.com/econumo/econumo/releases/latest) and
verify it:

```console
$ curl -LO https://github.com/econumo/econumo/releases/latest/download/econumo-linux-amd64
$ curl -LO https://github.com/econumo/econumo/releases/latest/download/SHA256SUMS
$ sha256sum --check --ignore-missing SHA256SUMS
```

A reference systemd unit lives in
[`deployment/systemd/econumo.service`](deployment/systemd/econumo.service):

```console
$ sudo useradd --system --home-dir /var/lib/econumo --shell /usr/sbin/nologin econumo
$ sudo mkdir -p /opt/econumo /var/lib/econumo /etc/econumo
$ sudo chown econumo:econumo /var/lib/econumo
$ sudo install -m 0755 econumo-linux-amd64 /opt/econumo/econumo
$ sudoedit /etc/econumo/env
$ sudo cp deployment/systemd/econumo.service /etc/systemd/system/
$ sudo systemctl daemon-reload && sudo systemctl enable --now econumo
```

A minimal `/etc/econumo/env` (all other settings from
[`.env.example`](.env.example) work here too):

```
DATABASE_URL=sqlite:///var/lib/econumo/db.sqlite
PORT=8181
```

**Upgrades:** replace `/opt/econumo/econumo` with the new release binary and
`sudo systemctl restart econumo` — database migrations run on boot, exactly
as in the Docker image. `/opt/econumo/econumo version` prints the installed
version.

**Management commands** need the same environment as the service:

```console
$ sudo -u econumo sh -c 'set -a; . /etc/econumo/env; exec /opt/econumo/econumo user:create "Name" user@example.com password'
```

**Back up** `/var/lib/econumo` (the SQLite database is the only state).
~~~

- [ ] **Step 3: Document the override in .env.example**

In `.env.example`, in the optional web-UI overrides area (after the `API_URL` block, ~line 30 — read the file to place it cleanly), add:

```bash
# Serve the web UI from a directory on disk instead of the SPA embedded in
# the binary (dev override / separately-built frontend). Unset = embedded.
# ECONUMO_WEB_DIST=web/dist
```

- [ ] **Step 4: Update CLAUDE.md**

Three targeted edits:

a) Project Overview: change "The production artifact is a single self-contained Go binary in a distroless image" sentence to also say "(SPA embedded via `go:embed` — `web/embed.go`); releases additionally attach standalone `econumo-linux-{amd64,arm64}` binaries + `SHA256SUMS` for Docker-free hosting."

b) Replace the `ECONUMO_WEB_DIST` config bullet with:

```markdown
- `ECONUMO_WEB_DIST` — disk path to a built SPA, overriding the build embedded
  in the binary (`web/embed.go`). Unset (default) = serve the embedded SPA,
  falling back to `web/dist` on disk when no build is embedded (source
  checkout). The binary logs the chosen source at boot (`spa_source`).
```

c) Deployment section: add a bullet after the image description:

```markdown
- Docker-free: each release attaches single-file linux binaries (SPA embedded)
  + `SHA256SUMS`; reference systemd unit in `deployment/systemd/econumo.service`,
  walkthrough in the README ("Run without Docker"). `make release-binaries`
  builds the same artifacts locally.
```

- [ ] **Step 5: Validate + commit**

Run: `make go-test` (guards nothing new here, but confirms the tree is green)
Manually verify the systemd unit parses: `systemd-analyze verify deployment/systemd/econumo.service 2>&1 | grep -v "Unit is bound" || true` (on a systemd host; warnings about the missing econumo user/binary are expected and fine — only syntax errors matter).

```bash
git add deployment/systemd/econumo.service README.md .env.example CLAUDE.md
git commit -m "docs: systemd unit + run-without-docker walkthrough for the single binary"
```

---

## Final Verification (after all tasks)

- [ ] `make go-test` — full smoke tier green.
- [ ] `make test` — full tier (engine comparison + frontend suite) green.
- [ ] `make release-binaries VERSION=v0.0.0-rc && cd release-out && sha256sum -c SHA256SUMS` — artifacts reproducible.
- [ ] Boot `econumo-linux-amd64` from an empty scratch dir (no `.env`, no `web/dist`): SPA loads, `/health` OK, `version` prints the stamp, boot log says `spa_source=embedded`.
- [ ] `docker build` the image and confirm the SPA serves with no `/app/web` in the image (`docker run --rm --entrypoint /app/econumo <img> version` works; SPA + `/econumo-config.js` merge verified per Task 5 Step 3).
- [ ] `git status` clean after a `make web-bundle` (the `.gitkeep` round-trip).
- [ ] The real workflow run is verified at the next release (publish-release skill).
