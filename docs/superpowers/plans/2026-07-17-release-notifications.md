# Release Notifications Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Notify users when a new Econumo release is available, linking to `https://econumo.com/releases/<version>/`.

**Architecture:** The website publishes `releases/latest.json`; the Go backend polls it (daily, in-memory cache, env opt-out `ECONUMO_CHECK_UPDATES`) and exposes it via `GET /api/v1/system/get-update-info`; the SPA compares against its own build version and renders a settings-gear badge, a dismissible sidebar notice, and a Settings-page row.

**Tech Stack:** Go stdlib (net/http, sync, regexp), new feature package `internal/system`; React 19 + TanStack Query + react-i18next; Astro (website repo).

**Spec:** `docs/superpowers/specs/2026-07-17-release-notifications-design.md`

## Global Constraints

- API envelope, routes, and method rules are frozen contract (see CLAUDE.md): success envelope `{"success": true, "message": "", "data": ...}`; only GET (reads) and POST (writes); route shape `/api/v1/{module}/{action}-{subject}`.
- Env naming: app-owned config is prefixed `ECONUMO_` → the opt-out is `ECONUMO_CHECK_UPDATES` (default `true`).
- Feed validation (trust boundary): accept only `version` matching `^v\d+\.\d+\.\d+$` AND `url` starting with `https://econumo.com/`. Invalid payloads are dropped, previous value retained.
- Failure is silence: no error states or retry UI anywhere; `dev`/non-semver builds never show anything.
- i18n: every new key must exist in BOTH `locales/en.json` and `locales/ru.json` with identical `{var}` placeholder sets (enforced by `internal/test/i18ntest`, part of `make go-test`).
- Golden files: regenerate with `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`, inspect the diff, never hand-edit.
- Comments: sparse, only non-obvious rationale. Swagger `// @…` blocks are required on handlers.
- Frontend done-gate: `pnpm exec tsc -b` must pass in `web/` (vitest+oxlint do not type-check).
- Work in this worktree (`/home/dmitry/dev/econumo/econumo/.claude/worktrees/hashed-greeting-hanrahan`); never cd to the main checkout. `git stash` is forbidden (shared stack).
- Known pre-existing failure on main: `web/src/features/settings/ImportCsvDialog.test.tsx` — not caused by this work; ignore it in verification.

---

### Task 1: Website release feed (`econumo/website` repo — separate clone + PR)

The app feature is inert without this feed. Work in a scratchpad clone; this repo is untouched.

**Files (in the website clone):**
- Create: `src/pages/releases/latest.json.ts`

**Interfaces:**
- Produces: `https://econumo.com/releases/latest.json` → `{"version": "v1.0.2", "date": "2026-07-16", "url": "https://econumo.com/releases/v1.0.2/"}`. Task 3's poller consumes this exact shape (`date` is informational; the backend ignores it).

- [ ] **Step 1: Clone and branch**

```bash
git clone git@github.com:econumo/website.git /tmp/claude-1000/-home-dmitry-dev-econumo-econumo/d70d203b-4c91-4deb-bb5f-7d8d8fcf8f70/scratchpad/website
cd /tmp/claude-1000/-home-dmitry-dev-econumo-econumo/d70d203b-4c91-4deb-bb5f-7d8d8fcf8f70/scratchpad/website
git checkout -b feat/latest-release-feed
```

- [ ] **Step 2: Create the endpoint**

The repo already has `getReleases()` (newest-first, draft-filtered) and `releaseSlug()` in `src/lib/content.ts`, and `SITE.url` in `src/lib/site.ts` — reuse them (mirror `src/pages/releases/index.xml.ts`). Sort by semver, not date, so same-day releases can't misorder.

Create `src/pages/releases/latest.json.ts`:

```ts
import type { APIRoute } from 'astro';
import { getReleases, releaseSlug } from '@/lib/content';
import { SITE } from '@/lib/site';

const SEMVER = /^v(\d+)\.(\d+)\.(\d+)$/;

const parts = (slug: string): number[] => SEMVER.exec(slug)!.slice(1).map(Number);

// The latest published release as machine-readable JSON, consumed by the
// self-hosted app's update check. Shape is a contract: {version, date, url}.
export const GET: APIRoute = async () => {
  const releases = (await getReleases()).filter((r) => SEMVER.test(releaseSlug(r)));
  releases.sort((a, b) => {
    const [aMaj, aMin, aPat] = parts(releaseSlug(a));
    const [bMaj, bMin, bPat] = parts(releaseSlug(b));
    return bMaj - aMaj || bMin - aMin || bPat - aPat;
  });
  const latest = releases[0];
  const body = latest
    ? {
        version: releaseSlug(latest),
        date: latest.data.date.toISOString().slice(0, 10),
        url: `${SITE.url}/releases/${releaseSlug(latest)}/`,
      }
    : {};
  return new Response(JSON.stringify(body), {
    headers: { 'Content-Type': 'application/json; charset=utf-8' },
  });
};
```

- [ ] **Step 3: Build and verify the output**

```bash
npm ci && npm run build
cat dist/releases/latest.json
```

Expected: valid JSON naming the newest release (v1.0.2 or later), e.g. `{"version":"v1.0.2","date":"2026-07-16","url":"https://econumo.com/releases/v1.0.2/"}`. If `npm run build` is not the build script, check `package.json`/`Makefile` for the build target and use that.

- [ ] **Step 4: Commit, push, PR**

```bash
git add src/pages/releases/latest.json.ts
git commit -m "feat: latest-release JSON feed for the app update check"
git push -u origin feat/latest-release-feed
gh pr create --repo econumo/website --title "feat: latest-release JSON feed" --body "Adds /releases/latest.json ({version, date, url}, semver-newest) for the self-hosted app's update notifications. Consumed by econumo/econumo (release-notifications feature)."
```

---

### Task 2: Backend config flag `ECONUMO_CHECK_UPDATES`

**Files:**
- Modify: `internal/config/config.go` (Config struct ~line 22-26, Load ~line 64-77)
- Modify: `internal/config/config_test.go`
- Modify: `.env.example`

**Interfaces:**
- Produces: `config.Config.CheckUpdates bool` (default `true`) — consumed by Task 4's `serve` wiring.

- [ ] **Step 1: Write the failing test** (append to `internal/config/config_test.go`):

```go
func TestLoad_CheckUpdates(t *testing.T) {
	t.Setenv("DATABASE_URL", "sqlite:///tmp/x.sqlite")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.CheckUpdates {
		t.Fatal("CheckUpdates default = false, want true")
	}
	t.Setenv("ECONUMO_CHECK_UPDATES", "false")
	c, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.CheckUpdates {
		t.Fatal("CheckUpdates with ECONUMO_CHECK_UPDATES=false = true, want false")
	}
}
```

- [ ] **Step 2: Run it — expect FAIL**

Run: `go test ./internal/config/ -run TestLoad_CheckUpdates -v`
Expected: compile error `c.CheckUpdates undefined`.

- [ ] **Step 3: Implement**

In the `Config` struct, after the `SQLiteBusyTimeout int` line:

```go
	CheckUpdates      bool   // ECONUMO_CHECK_UPDATES: poll econumo.com for the latest release (default true)
```

In `Load()`, after the `SQLiteBusyTimeout:` line:

```go
		CheckUpdates:           getBool("ECONUMO_CHECK_UPDATES", true),
```

- [ ] **Step 4: Run test — expect PASS**

Run: `go test ./internal/config/ -run TestLoad_CheckUpdates -v`

- [ ] **Step 5: Document in `.env.example`** — add near the `ECONUMO_DEBUG` block:

```
# Optional: set ECONUMO_CHECK_UPDATES=false to disable the daily check for new
# Econumo releases (a single request from the server to econumo.com).
# ECONUMO_CHECK_UPDATES=false
```

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go .env.example
git commit -m "feat(config): ECONUMO_CHECK_UPDATES flag (default true)"
```

---

### Task 3: `internal/system` service — feed poll, validation, cache

**Files:**
- Create: `internal/model/system_dto.go`
- Create: `internal/system/updates.go`
- Create: `internal/system/updates_test.go`

**Interfaces:**
- Consumes: `model.GetUpdateInfoResult` (defined here), `vo.Id` from `internal/shared/vo`.
- Produces (Task 4 relies on these exact signatures):
  - `system.DefaultFeedURL` (const string)
  - `system.NewService(enabled bool, feedURL string) *system.Service`
  - `(*Service).GetUpdateInfo(ctx context.Context, userID vo.Id) (model.GetUpdateInfoResult, error)` — matches `endpoint.HandleNoBody`'s `call` shape
  - `(*Service).StartPolling(ctx context.Context)` — non-blocking; no-op when disabled

- [ ] **Step 1: DTO** — create `internal/model/system_dto.go`:

```go
// Result DTO for the system read endpoint (get-update-info).
package model

// GetUpdateInfoResult is the get-update-info response: the latest release
// published on econumo.com, or empty strings when unknown (check disabled,
// not fetched yet, or the feed unreachable). The SPA compares version against
// its own build version; the server never does.
type GetUpdateInfoResult struct {
	Version string `json:"version"`
	Url     string `json:"url"`
}
```

- [ ] **Step 2: Write the failing tests** — create `internal/system/updates_test.go`:

```go
package system

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/econumo/econumo/internal/shared/vo"
)

func feedServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func info(t *testing.T, s *Service) (string, string) {
	t.Helper()
	res, err := s.GetUpdateInfo(context.Background(), vo.Id(""))
	if err != nil {
		t.Fatal(err)
	}
	return res.Version, res.Url
}

func TestFetchValidPayload(t *testing.T) {
	srv := feedServer(t, 200, `{"version":"v9.9.9","date":"2026-07-16","url":"https://econumo.com/releases/v9.9.9/"}`)
	s := NewService(true, srv.URL)
	s.fetch(context.Background())
	v, u := info(t, s)
	if v != "v9.9.9" || u != "https://econumo.com/releases/v9.9.9/" {
		t.Fatalf("info = %q %q, want v9.9.9 + release url", v, u)
	}
}

func TestFetchRejectsInvalidPayloads(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
	}{
		"malformed json":   {200, `{"version": `},
		"bad version":      {200, `{"version":"latest","url":"https://econumo.com/releases/latest/"}`},
		"wrong url origin": {200, `{"version":"v9.9.9","url":"https://evil.example/phish/"}`},
		"non-2xx":          {500, `{"version":"v9.9.9","url":"https://econumo.com/releases/v9.9.9/"}`},
		"empty payload":    {200, `{}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := feedServer(t, tc.status, tc.body)
			s := NewService(true, srv.URL)
			s.fetch(context.Background())
			if v, u := info(t, s); v != "" || u != "" {
				t.Fatalf("info = %q %q, want empty (payload must be dropped)", v, u)
			}
		})
	}
}

func TestFetchFailureKeepsPreviousValue(t *testing.T) {
	good := feedServer(t, 200, `{"version":"v9.9.9","url":"https://econumo.com/releases/v9.9.9/"}`)
	s := NewService(true, good.URL)
	s.fetch(context.Background())

	bad := feedServer(t, 500, ``)
	s.feedURL = bad.URL
	s.fetch(context.Background())
	if v, _ := info(t, s); v != "v9.9.9" {
		t.Fatalf("version after failed refetch = %q, want v9.9.9 retained", v)
	}
}

func TestStartPollingDisabledIsNoop(t *testing.T) {
	srv := feedServer(t, 200, `{"version":"v9.9.9","url":"https://econumo.com/releases/v9.9.9/"}`)
	s := NewService(false, srv.URL)
	s.StartPolling(context.Background())
	if v, _ := info(t, s); v != "" {
		t.Fatalf("disabled service has version %q, want empty", v)
	}
}
```

- [ ] **Step 3: Run — expect FAIL**

Run: `go test ./internal/system/ -v`
Expected: compile errors (package/`NewService` undefined).

- [ ] **Step 4: Implement** — create `internal/system/updates.go`:

```go
// Package system reports instance-level information; currently the latest
// published Econumo release, polled from the econumo.com feed.
package system

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

const DefaultFeedURL = "https://econumo.com/releases/latest.json"

// The feed is remote input rendered as a trusted link in every instance's UI:
// only well-formed versions and econumo.com URLs are ever accepted.
const allowedURLPrefix = "https://econumo.com/"

var versionPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

const pollInterval = 24 * time.Hour

type Service struct {
	enabled bool
	feedURL string
	client  *http.Client

	mu   sync.RWMutex
	info model.GetUpdateInfoResult
}

func NewService(enabled bool, feedURL string) *Service {
	return &Service{
		enabled: enabled,
		feedURL: feedURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// GetUpdateInfo returns the cached latest-release info; empty strings when the
// check is disabled, hasn't run yet, or nothing valid was ever received. The
// SPA does the version comparison, so failure here is silence, never an error.
func (s *Service) GetUpdateInfo(_ context.Context, _ vo.Id) (model.GetUpdateInfoResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.info, nil
}

// StartPolling launches the background feed poll (boot + every 24h). Only the
// serve command calls this; CLI commands and tests never do, so their
// responses stay deterministic and hermetic.
func (s *Service) StartPolling(ctx context.Context) {
	if !s.enabled {
		return
	}
	go func() {
		s.fetch(ctx)
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.fetch(ctx)
			}
		}
	}()
}

func (s *Service) fetch(ctx context.Context) {
	if err := s.fetchOnce(ctx); err != nil {
		slog.Debug("release feed check failed", "err", err)
	}
}

func (s *Service) fetchOnce(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.feedURL, nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("feed returned status %d", resp.StatusCode)
	}
	var feed struct {
		Version string `json:"version"`
		Url     string `json:"url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&feed); err != nil {
		return err
	}
	if !versionPattern.MatchString(feed.Version) {
		return fmt.Errorf("feed version %q is not vX.Y.Z", feed.Version)
	}
	if !strings.HasPrefix(feed.Url, allowedURLPrefix) {
		return fmt.Errorf("feed url %q is outside %s", feed.Url, allowedURLPrefix)
	}
	s.mu.Lock()
	s.info = model.GetUpdateInfoResult{Version: feed.Version, Url: feed.Url}
	s.mu.Unlock()
	return nil
}
```

- [ ] **Step 5: Run — expect PASS (and race-clean)**

Run: `go test ./internal/system/ -race -v`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/model/system_dto.go internal/system/
git commit -m "feat(system): latest-release feed poller with validation and in-memory cache"
```

---

### Task 4: API edge — handler, route, wiring, swagger, apiparity golden

**Files:**
- Create: `internal/system/api/handler.go`
- Create: `internal/system/api/system.go`
- Create: `internal/system/api/routes.go`
- Modify: `internal/server/server.go` (Seams struct ~line 57, BuildAPI wiring ~line 190-203)
- Modify: `cmd/econumo/main.go` (`run`, ~line 206)
- Modify: `internal/test/apiparity/catalogue.go` (read-scenario section, after `budget_reads` ~line 75)
- Modify: `CLAUDE.md` (config var list; the two "nine features" mentions in the feature-packages section)
- Generated: OpenAPI docs via `make swagger`; golden via `UPDATE_GOLDEN=1`

**Interfaces:**
- Consumes: `system.NewService/StartPolling/GetUpdateInfo`, `system.DefaultFeedURL` (Task 3); `cfg.CheckUpdates` (Task 2); `endpoint.HandleNoBody`, `middleware.Auth`, `router.RegisterAPI` (existing).
- Produces: `GET /api/v1/system/get-update-info` (auth) → `data: {"version": "", "url": ""}` until the poller has data. Frontend Task 5 consumes this route. `server.Seams.Updates *system.Service` — nil (all tests) means a never-started, disabled service.

- [ ] **Step 1: Handler package** — create `internal/system/api/handler.go`:

```go
// Package api wires the system module's HTTP edge.
package api

import (
	appsystem "github.com/econumo/econumo/internal/system"
	"github.com/econumo/econumo/internal/web/apidoc"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	svc *appsystem.Service
	dev bool
}

func NewHandlers(svc *appsystem.Service, dev bool) *Handlers {
	return &Handlers{svc: svc, dev: dev}
}
```

- [ ] **Step 2: Endpoint** — create `internal/system/api/system.go`:

```go
package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

// _ keeps the apidoc and model import aliases visible to swag's annotation
// parser (a type reference's leading identifier must resolve to an import alias).
var (
	_ = apidoc.JsonResponseError{}
	_ = model.GetUpdateInfoResult{}
)

// GetUpdateInfo handles GET /api/v1/system/get-update-info (auth). No request
// body; returns the latest release published on econumo.com, or empty strings
// when unknown (update checks disabled or the feed not fetched yet).
//
// @Summary     Get the latest published release
// @Description Returns the latest Econumo release published on econumo.com (version + release-page URL), or empty strings when unknown. The client compares against its own version.
// @Tags        System
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetUpdateInfoResult}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/system/get-update-info [get]
func (h *Handlers) GetUpdateInfo(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.dev, h.svc.GetUpdateInfo)
}
```

- [ ] **Step 3: Routes** — create `internal/system/api/routes.go` (the apiparity guard's `internal/*/api/routes.go` glob picks this up automatically):

```go
package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/middleware"
	"github.com/econumo/econumo/internal/web/router"
)

func RegisterAPI(h *Handlers, authn middleware.TokenAuthenticator, dev bool) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		authMw := middleware.Auth(authn, dev)
		auth := func(fn http.HandlerFunc) http.Handler { return authMw(fn) }

		mux.Handle("GET /api/v1/system/get-update-info", auth(h.GetUpdateInfo))
	}
}
```

- [ ] **Step 4: Wire into BuildAPI** — in `internal/server/server.go`:

Add imports (alphabetical among the app imports):

```go
	appsystem "github.com/econumo/econumo/internal/system"
	handlersystem "github.com/econumo/econumo/internal/system/api"
```

Extend `Seams` (the comment explains why it lives here):

```go
type Seams struct {
	Clock   port.Clock
	Avatars appuser.AvatarPicker
	// Updates is the release-check service. serve constructs an enabled one and
	// starts its poller; nil (every test and CLI path) wires a disabled service
	// that never polls, keeping responses deterministic and hermetic.
	Updates *appsystem.Service
}
```

In `BuildAPI`, next to the other handler constructions (after the `currencyHandlers` block, ~line 135):

```go
	updates := seams.Updates
	if updates == nil {
		updates = appsystem.NewService(false, appsystem.DefaultFeedURL)
	}
	systemHandlers := handlersystem.NewHandlers(updates, cfg.IsDev())
```

Register in `router.Compose(...)`, after `handlerbudget.RegisterAPI(...)`:

```go
		handlersystem.RegisterAPI(systemHandlers, userSvc, cfg.IsDev()),
```

- [ ] **Step 5: Start polling in serve** — in `cmd/econumo/main.go` `run()`, replace

```go
	handler := server.BuildAPI(cfg, db, server.Seams{})
```

with

```go
	updates := system.NewService(cfg.CheckUpdates, system.DefaultFeedURL)
	handler := server.BuildAPI(cfg, db, server.Seams{Updates: updates})
	updates.StartPolling(ctx)
```

and add the import `"github.com/econumo/econumo/internal/system"`.

- [ ] **Step 6: apiparity scenario** — in `internal/test/apiparity/catalogue.go`, after the `budget_reads` registration:

```go
	register(Scenario{Name: "system_reads", Calls: func() []Call {
		return []Call{
			{Label: "get-update-info", Method: "GET", Path: "/api/v1/system/get-update-info", Auth: "owner", Body: map[string]any{}},
		}
	}})
```

- [ ] **Step 7: Regenerate swagger docs and the golden**

```bash
make swagger
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/
git diff --stat internal/test/apiparity/testdata/
```

Expected: exactly ONE new golden file (`system_reads` scenario, body `{"success":true,"message":"","data":{"version":"","url":""}}`) and zero changes to existing goldens. Any changed existing golden means observable behavior changed — stop and investigate.

- [ ] **Step 8: Full backend gate**

Run: `make go-test`
Expected: PASS (build, vet, gofmt, docs-fresh, tests, coverage gate, i18ntest, apiparity guards).

- [ ] **Step 9: Update CLAUDE.md** — in the config section add one line:

```
- `ECONUMO_CHECK_UPDATES` — daily check for new releases against `econumo.com/releases/latest.json` (single server-side request; result served to the SPA via `get-update-info`). `false` disables it.
```

and update the feature-package enumeration: "nine features" → "ten features", adding `system` to the alphabetical list (`account`, `budget`, `category`, `connection`, `currency`, `payee`, `system`, `tag`, `transaction`, `user`); note `system` has no `repository.go` (in-memory state only, like `currency` has no per-user persistence).

- [ ] **Step 10: Commit**

```bash
git add internal/system/api/ internal/server/server.go cmd/econumo/main.go internal/test/apiparity/ internal/web/apidoc/ CLAUDE.md
git commit -m "feat(system): get-update-info endpoint + serve-time release poller"
```

(If `make swagger` writes docs elsewhere — check `git status` — add those paths too.)

---

### Task 5: Frontend — version helper, API client, hook

**Files:**
- Create: `web/src/lib/version.ts`
- Create: `web/src/lib/version.test.ts`
- Create: `web/src/api/dto/system.ts`
- Create: `web/src/api/system.ts`
- Create: `web/src/hooks/useAvailableUpdate.ts`
- Modify: `web/src/app/queryKeys.ts` (add key)
- Modify: `web/src/features/settings/SettingsPage.tsx:20` (delete local `SEMVER`, import from lib)

**Interfaces:**
- Consumes: `GET /api/v1/system/get-update-info` (Task 4); `getVersion()` from `@/lib/config`; `api`, `apiUrl` from `@/api/client`.
- Produces (Task 6 relies on these):
  - `SEMVER: RegExp` and `isNewerVersion(latest: string, current: string): boolean` from `@/lib/version`
  - `getDismissedUpdateVersion(): string | null` / `setDismissedUpdateVersion(version: string): void` from `@/lib/version`
  - `useAvailableUpdate(): { version: string; url: string } | null` from `@/hooks/useAvailableUpdate`

- [ ] **Step 1: Write the failing test** — create `web/src/lib/version.test.ts`:

```ts
import { isNewerVersion } from './version'

it.each([
  ['v1.0.2', 'v1.0.1', true],
  ['v1.1.0', 'v1.0.9', true],
  ['v2.0.0', 'v1.9.9', true],
  ['v1.0.2', 'v1.0.2', false],
  ['v1.0.1', 'v1.0.2', false],
  ['v1.0.10', 'v1.0.9', true],
  ['v1.0.2', 'dev', false],
  ['dev', 'v1.0.2', false],
  ['', 'v1.0.2', false],
  ['v1.0.2', '', false],
  ['1.0.3', 'v1.0.2', false],
])('isNewerVersion(%s, %s) -> %s', (latest, current, expected) => {
  expect(isNewerVersion(latest, current)).toBe(expected)
})

it('dismissed-update version round-trips through localStorage', async () => {
  const { getDismissedUpdateVersion, setDismissedUpdateVersion } = await import('./version')
  localStorage.removeItem('econumo.dismissed-update-version')
  expect(getDismissedUpdateVersion()).toBeNull()
  setDismissedUpdateVersion('v1.0.2')
  expect(getDismissedUpdateVersion()).toBe('v1.0.2')
})
```

- [ ] **Step 2: Run — expect FAIL**

Run: `cd web && pnpm vitest run src/lib/version.test.ts`
Expected: module `./version` not found.

- [ ] **Step 3: Implement** — create `web/src/lib/version.ts`:

```ts
export const SEMVER = /^v(\d+)\.(\d+)\.(\d+)$/

// True only when BOTH strings are strict vX.Y.Z and latest > current — so a
// `dev` image, an empty feed, or a custom build tag can never trigger the
// update surfaces.
export function isNewerVersion(latest: string, current: string): boolean {
  const l = SEMVER.exec(latest)
  const c = SEMVER.exec(current)
  if (!l || !c) {
    return false
  }
  for (let i = 1; i <= 3; i++) {
    const d = Number(l[i]) - Number(c[i])
    if (d !== 0) {
      return d > 0
    }
  }
  return false
}

const DISMISSED_KEY = 'econumo.dismissed-update-version'

export function getDismissedUpdateVersion(): string | null {
  return localStorage.getItem(DISMISSED_KEY)
}

export function setDismissedUpdateVersion(version: string): void {
  localStorage.setItem(DISMISSED_KEY, version)
}
```

- [ ] **Step 4: Run — expect PASS**

Run: `cd web && pnpm vitest run src/lib/version.test.ts`

- [ ] **Step 5: Deduplicate SEMVER** — in `web/src/features/settings/SettingsPage.tsx` delete line 20 (`const SEMVER = /^v\d+\.\d+\.\d+$/`) and add to the imports:

```ts
import { SEMVER } from '@/lib/version'
```

- [ ] **Step 6: API client + hook**

Create `web/src/api/dto/system.ts`:

```ts
export interface UpdateInfoDto {
  version: string
  url: string
}
```

Create `web/src/api/system.ts` (same envelope pattern as `currency.ts`):

```ts
import { api, apiUrl } from './client'
import type { UpdateInfoDto } from './dto/system'

interface Envelope<T> {
  data: T
}

export async function getUpdateInfo(): Promise<UpdateInfoDto> {
  const response = await api.get<Envelope<UpdateInfoDto>>(apiUrl('/api/v1/system/get-update-info'))
  return response.data.data
}
```

In `web/src/app/queryKeys.ts` add to `queryKeys`:

```ts
  updateInfo: ['updateInfo'] as const,
```

Create `web/src/hooks/useAvailableUpdate.ts`:

```ts
import { useQuery } from '@tanstack/react-query'
import { getUpdateInfo } from '@/api/system'
import { ONE_DAY, queryKeys } from '@/app/queryKeys'
import { getVersion } from '@/lib/config'
import { isNewerVersion } from '@/lib/version'

export interface AvailableUpdate {
  version: string
  url: string
}

// The latest published release when it is strictly newer than this build;
// null otherwise (current, dev build, check disabled, or feed unavailable).
export function useAvailableUpdate(): AvailableUpdate | null {
  const { data } = useQuery({
    queryKey: queryKeys.updateInfo,
    queryFn: getUpdateInfo,
    staleTime: ONE_DAY,
    refetchOnWindowFocus: false,
  })
  if (!data || !isNewerVersion(data.version, getVersion())) {
    return null
  }
  return { version: data.version, url: data.url }
}
```

- [ ] **Step 7: Type-check and full web tests**

Run: `cd web && pnpm exec tsc -b && pnpm test`
Expected: tsc clean; tests pass (ImportCsvDialog failure is pre-existing, ignore).

- [ ] **Step 8: Commit**

```bash
git add web/src/lib/version.ts web/src/lib/version.test.ts web/src/api/dto/system.ts web/src/api/system.ts web/src/hooks/useAvailableUpdate.ts web/src/app/queryKeys.ts web/src/features/settings/SettingsPage.tsx
git commit -m "feat(web): update-info client, semver compare, useAvailableUpdate hook"
```

---

### Task 6: Frontend — the three surfaces + i18n

**Files:**
- Create: `web/src/components/UpdateNotice.tsx`
- Create: `web/src/components/UpdateNotice.test.tsx`
- Modify: `web/src/app/layouts/ApplicationLayout.tsx` (both footers, ~lines 177-217)
- Modify: `web/src/features/settings/SettingsPage.tsx` (footer area, ~line 105)
- Modify: `locales/en.json`, `locales/ru.json`

**Interfaces:**
- Consumes: `useAvailableUpdate`, `getDismissedUpdateVersion`/`setDismissedUpdateVersion` (Task 5).
- Produces: i18n keys `common.update.notice` (`{version}`), `common.update.dismiss`, `settings.update.available` (`{version}`).

- [ ] **Step 1: i18n keys**

In `locales/en.json`, inside the `common` object add:

```json
"update": {
  "notice": "Econumo {version} is out",
  "dismiss": "Dismiss"
}
```

Inside the `settings` object add:

```json
"update": {
  "available": "New version {version} available"
}
```

In `locales/ru.json`, same paths:

```json
"update": {
  "notice": "Вышла Econumo {version}",
  "dismiss": "Скрыть"
}
```

```json
"update": {
  "available": "Доступна новая версия {version}"
}
```

- [ ] **Step 2: Write the failing component test** — create `web/src/components/UpdateNotice.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AvailableUpdate } from '@/hooks/useAvailableUpdate'
import { UpdateNotice } from './UpdateNotice'

const mockUpdate = vi.hoisted(() => ({ value: null as AvailableUpdate | null }))
vi.mock('@/hooks/useAvailableUpdate', () => ({
  useAvailableUpdate: () => mockUpdate.value,
}))

beforeEach(() => {
  localStorage.removeItem('econumo.dismissed-update-version')
  mockUpdate.value = { version: 'v9.9.9', url: 'https://econumo.com/releases/v9.9.9/' }
})

it('renders the release link when an update is available', () => {
  render(<UpdateNotice />)
  const link = screen.getByRole('link', { name: /v9\.9\.9/ })
  expect(link).toHaveAttribute('href', 'https://econumo.com/releases/v9.9.9/')
  expect(link).toHaveAttribute('target', '_blank')
})

it('renders nothing when no update is available', () => {
  mockUpdate.value = null
  const { container } = render(<UpdateNotice />)
  expect(container).toBeEmptyDOMElement()
})

it('dismisses per version and stays hidden', async () => {
  const user = userEvent.setup()
  render(<UpdateNotice />)
  await user.click(screen.getByRole('button', { name: /dismiss/i }))
  expect(screen.queryByRole('link')).not.toBeInTheDocument()
  expect(localStorage.getItem('econumo.dismissed-update-version')).toBe('v9.9.9')
})

it('reappears for a newer version after a dismissal', () => {
  localStorage.setItem('econumo.dismissed-update-version', 'v9.9.8')
  render(<UpdateNotice />)
  expect(screen.getByRole('link', { name: /v9\.9\.9/ })).toBeInTheDocument()
})
```

- [ ] **Step 3: Run — expect FAIL**

Run: `cd web && pnpm vitest run src/components/UpdateNotice.test.tsx`
Expected: `./UpdateNotice` not found.

- [ ] **Step 4: Implement** — create `web/src/components/UpdateNotice.tsx`:

```tsx
import { useState } from 'react'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useAvailableUpdate } from '@/hooks/useAvailableUpdate'
import { getDismissedUpdateVersion, setDismissedUpdateVersion } from '@/lib/version'

export function UpdateNotice() {
  const { t } = useTranslation()
  const update = useAvailableUpdate()
  const [dismissed, setDismissed] = useState<string | null>(getDismissedUpdateVersion)
  if (!update || dismissed === update.version) {
    return null
  }
  return (
    <div className="mx-3 mb-2 flex items-center gap-2 rounded-md bg-accent px-3 py-2 text-xs text-muted-foreground">
      <a
        href={update.url}
        target="_blank"
        rel="noreferrer"
        className="min-w-0 flex-1 truncate font-medium hover:text-foreground"
      >
        {t('common.update.notice', { version: update.version })}
      </a>
      <button
        type="button"
        aria-label={t('common.update.dismiss')}
        className="shrink-0 hover:text-foreground"
        onClick={() => {
          setDismissedUpdateVersion(update.version)
          setDismissed(update.version)
        }}
      >
        <X className="size-3.5" />
      </button>
    </div>
  )
}
```

- [ ] **Step 5: Run — expect PASS**

Run: `cd web && pnpm vitest run src/components/UpdateNotice.test.tsx`

- [ ] **Step 6: Mount in `ApplicationLayout.tsx`**

Add imports:

```tsx
import { UpdateNotice } from '@/components/UpdateNotice'
import { useAvailableUpdate } from '@/hooks/useAvailableUpdate'
```

In the component body (near the other hooks, ~line 104): `const update = useAvailableUpdate()`.

**Rail footer** (~line 179): wrap the Settings link contents with a dot —

```tsx
              <Link
                to={RouterPage.SETTINGS}
                title={t('settings.page.menu_item')}
                className="relative text-muted-foreground hover:text-foreground"
              >
                <Settings className="size-5" />
                {update ? (
                  <span className="absolute -top-0.5 -right-0.5 size-2 rounded-full bg-primary" data-testid="update-dot" />
                ) : null}
              </Link>
```

**Full footer** (~line 203): the Settings text link gets the dot —

```tsx
                <Link to={RouterPage.SETTINGS} className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground">
                  {t('settings.page.menu_item')}
                  {update ? <span className="size-1.5 rounded-full bg-primary" data-testid="update-dot" /> : null}
                </Link>
```

**Notice placement**: immediately BEFORE the `{rail ? (` footer conditional (~line 177), so it sits above the footer in both compact and full desktop modes but not in the icon rail:

```tsx
          {!rail ? <UpdateNotice /> : null}
```

- [ ] **Step 7: Settings page row** — in `web/src/features/settings/SettingsPage.tsx`:

Add imports: `import { useAvailableUpdate } from '@/hooks/useAvailableUpdate'` and in the body `const update = useAvailableUpdate()`.

Directly above the `<footer ...>` element add:

```tsx
      {update ? (
        <a
          href={update.url}
          target="_blank"
          rel="noreferrer"
          className="mx-auto pb-1 text-xs font-medium text-primary hover:underline"
        >
          {t('settings.update.available', { version: update.version })} →
        </a>
      ) : null}
```

- [ ] **Step 8: Verify everything**

```bash
cd web && pnpm exec tsc -b && pnpm lint && pnpm test
cd .. && make go-test
```

Expected: all green (`make go-test` includes the i18ntest guards that check the new keys' en/ru + placeholder parity and t()-coverage; ImportCsvDialog failure is pre-existing).

- [ ] **Step 9: Commit**

```bash
git add web/src/components/UpdateNotice.tsx web/src/components/UpdateNotice.test.tsx web/src/app/layouts/ApplicationLayout.tsx web/src/features/settings/SettingsPage.tsx locales/en.json locales/ru.json
git commit -m "feat(web): release-update surfaces — sidebar notice, settings badge and row"
```

---

### Task 7: End-to-end verification and PR

- [ ] **Step 1: Full suite**

Run: `make test` (needs the compose PostgreSQL; if unavailable, `make go-test` + `make web-test` and say so in the PR).

- [ ] **Step 2: Live check** — run the app and verify the flow end-to-end (use the `verify`/`run` skill flow): start the server with a stub feed to see the surfaces.

```bash
# Terminal A: a stub feed
python3 -c "
import http.server, json
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        body = json.dumps({'version': 'v99.0.0', 'date': '2026-07-17', 'url': 'https://econumo.com/releases/v99.0.0/'}).encode()
        self.send_response(200); self.send_header('Content-Type', 'application/json'); self.end_headers()
        self.wfile.write(body)
http.server.HTTPServer(('127.0.0.1', 9999), H).serve_forever()
"
```

Point the poller at it for the manual run ONLY by temporarily editing `DefaultFeedURL` (do not commit) — or verify against the real feed once the website PR from Task 1 is merged. Confirm: `GET /api/v1/system/get-update-info` returns the version; the SPA (`make web-run` against `make go-run`) shows the sidebar notice, the settings dot, and the Settings row; dismissing the notice hides it; the dot survives. Revert any temporary edit (`git diff` must be clean except committed work).

- [ ] **Step 3: Branch + PR** (superpowers:finishing-a-development-branch)

```bash
git branch -m feat/release-notifications
git push -u origin feat/release-notifications
gh pr create --title "feat: new-release notifications" --body "..."
```

PR body: summary of the three surfaces, the endpoint, `ECONUMO_CHECK_UPDATES`, the feed contract, a link to the econumo/website PR from Task 1, and the spec path.

---

## Self-Review Notes

- Spec coverage: feed (Task 1), opt-out config (2), poller+validation+cache (3), endpoint+wiring+goldens+docs (4), comparison+hook (5), three surfaces+i18n (6), rollout order (1 before 7). ✔
- Golden expectation: exactly one new file, zero changed — matches the spec's "additive only". ✔
- Type consistency: `GetUpdateInfoResult{Version,Url}` ↔ `{"version","url"}` ↔ `UpdateInfoDto` ↔ `AvailableUpdate` verified across Tasks 3-6. ✔
