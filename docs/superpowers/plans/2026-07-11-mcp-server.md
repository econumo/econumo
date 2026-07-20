# MCP Server Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Embed an MCP server (Streamable HTTP, stateless JSON) in the Econumo binary at `/mcp`, behind the existing bearer-token auth: 7 resources, 5 tools, 2 prompts, plus a stored `users.timezone` fallback so MCP requests get correct day-boundary math.

**Architecture:** Official MCP Go SDK mounted as a plain `http.Handler` on the root mux; per-feature `internal/<feature>/mcp/` edge packages (mirroring `api/`) register capabilities through a seam in `internal/web/mcp`; composition in `server.BuildAPI`. Spec: `docs/superpowers/specs/2026-07-11-mcp-server-design.md`.

**Tech Stack:** Go 1.25, `github.com/modelcontextprotocol/go-sdk` **v1.6.1**, existing stack (sqlc, net/http, slog).

## Global Constraints

- Branch: `feat/mcp-server` (exists, has the spec; draft PR #78). Commit per task.
- Wire contract is FROZEN for everything under `/api` — this plan must not change any REST response byte. MCP responses are NEW contract: goldens created here freeze them.
- SDK version exactly `v1.6.1`. Import alias convention: `sdk "github.com/modelcontextprotocol/go-sdk/mcp"` and `webmcp "github.com/econumo/econumo/internal/web/mcp"`.
- sqlc `.sql` files: **ASCII-only comments** (an em dash mangles sqlite codegen in sqlc v1.30). Regenerate with `cd internal/infra/storage/sqlc && go run github.com/sqlc-dev/sqlc/cmd/sqlc generate` (or `sqlc generate` if installed) — check the Makefile for the pinned invocation first.
- Comments: sparingly, why-not-what, no references to removed PHP code (CLAUDE.md policy).
- Datetime wire format everywhere: `"2006-01-02 15:04:05"` (`datetime.Layout`); date-only `"2006-01-02"` (`datetime.DateLayout`).
- Every task ends green: `go build ./... && go vet ./...` plus the named tests; Tasks 5, 7, 14, 15 also run the full `make go-test` (coverage gate `GO_COVER_MIN=78` — the smoke suite must stay above it).
- Two deliberate deviations from the spec text (amended in Task 15): (1) infrastructure errors in tool handlers surface as a **generic tool error** `"Internal error"` (typed SDK handlers cannot emit JSON-RPC errors), details logged server-side; (2) the operation-log message for MCP requests is `mcp` (path-derived by AccessLog), with `tool`/`resource`/`prompt` names as log attrs — not the JSON-RPC method as the message.

---

### Task 1: reqctx — explicit-location flag

**Files:**
- Modify: `internal/shared/reqctx/reqctx.go`
- Test: `internal/shared/reqctx/reqctx_test.go`

**Interfaces:**
- Consumes: existing `WithLocation(ctx, *time.Location)`, `Location(ctx)`.
- Produces: `func WithExplicitLocation(ctx context.Context, loc *time.Location) context.Context` (sets the location AND marks it caller-supplied) and `func IsLocationExplicit(ctx context.Context) bool`. Tasks 2, 5, 7 rely on these exact names.

- [ ] **Step 1: Write the failing test** (append to `reqctx_test.go`; match the file's existing test style)

```go
func TestExplicitLocation(t *testing.T) {
	ctx := context.Background()
	if reqctx.IsLocationExplicit(ctx) {
		t.Fatal("empty ctx must not be explicit")
	}
	// Plain WithLocation (the Timezone middleware's UTC default) is NOT explicit.
	ctx = reqctx.WithLocation(ctx, time.UTC)
	if reqctx.IsLocationExplicit(ctx) {
		t.Fatal("WithLocation must not mark explicit")
	}
	loc, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatal(err)
	}
	ctx = reqctx.WithExplicitLocation(ctx, loc)
	if !reqctx.IsLocationExplicit(ctx) {
		t.Fatal("WithExplicitLocation must mark explicit")
	}
	if got := reqctx.Location(ctx); got.String() != "Europe/Amsterdam" {
		t.Fatalf("Location = %s, want Europe/Amsterdam", got)
	}
}
```

- [ ] **Step 2: Run it — must fail to compile** — `go test ./internal/shared/reqctx/` → `undefined: reqctx.WithExplicitLocation`.

- [ ] **Step 3: Implement.** In `reqctx.go`: extend the key iota and add the two funcs.

```go
const (
	locationKey ctxKey = iota
	logAttrsKey
	explicitLocationKey
)

// WithExplicitLocation is WithLocation for a timezone the CALLER supplied
// (X-Timezone header) rather than the UTC default; the distinction lets the
// timezone-persist path ignore defaulted requests.
func WithExplicitLocation(ctx context.Context, loc *time.Location) context.Context {
	return context.WithValue(WithLocation(ctx, loc), explicitLocationKey, true)
}

func IsLocationExplicit(ctx context.Context) bool {
	v, _ := ctx.Value(explicitLocationKey).(bool)
	return v
}
```

- [ ] **Step 4: Run** `go test ./internal/shared/reqctx/` → PASS.
- [ ] **Step 5: Commit** — `git add internal/shared/reqctx && git commit -m "feat(reqctx): distinguish caller-supplied timezone from UTC default"`

---

### Task 2: Timezone middleware marks explicit locations

**Files:**
- Modify: `internal/web/middleware/middleware.go` (the `Timezone` middleware, ~line 174)
- Test: `internal/web/middleware/middleware_test.go`

**Interfaces:**
- Consumes: `reqctx.WithExplicitLocation` (Task 1).
- Produces: behavior only — a valid `X-Timezone` header now yields `reqctx.IsLocationExplicit(ctx) == true`; missing/invalid header keeps the non-explicit UTC default. No signature changes.

- [ ] **Step 1: Write the failing test** (append to `middleware_test.go`)

```go
func TestTimezone_MarksExplicit(t *testing.T) {
	cases := []struct {
		name, header string
		wantExplicit bool
		wantLoc      string
	}{
		{"valid header", "Europe/Amsterdam", true, "Europe/Amsterdam"},
		{"no header", "", false, "UTC"},
		{"garbage header", "Not/AZone", false, "UTC"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotExplicit bool
			var gotLoc string
			h := middleware.Timezone(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotExplicit = reqctx.IsLocationExplicit(r.Context())
				gotLoc = reqctx.Location(r.Context()).String()
			}))
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			if tc.header != "" {
				req.Header.Set("X-Timezone", tc.header)
			}
			h.ServeHTTP(httptest.NewRecorder(), req)
			if gotExplicit != tc.wantExplicit || gotLoc != tc.wantLoc {
				t.Fatalf("explicit=%v loc=%s, want %v/%s", gotExplicit, gotLoc, tc.wantExplicit, tc.wantLoc)
			}
		})
	}
}
```

- [ ] **Step 2: Run** `go test ./internal/web/middleware/ -run TestTimezone_MarksExplicit` → FAIL (explicit=false for valid header).

- [ ] **Step 3: Implement.** Rework the `Timezone` body so the valid-header branch uses the explicit variant:

```go
func Timezone(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		loc := time.UTC
		explicit := false
		if tz := r.Header.Get("X-Timezone"); tz != "" {
			if l, err := time.LoadLocation(tz); err == nil {
				loc, explicit = l, true
			}
		}
		if explicit {
			ctx = reqctx.WithExplicitLocation(ctx, loc)
		} else {
			ctx = reqctx.WithLocation(ctx, loc)
		}
		// Record the resolved timezone as a log dimension. Timezone runs inside
		// AccessLog, so the pointer accumulator carries it back out to both lines.
		reqctx.AddLogAttr(ctx, "timezone", loc.String())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
```

- [ ] **Step 4: Run** `go test ./internal/web/middleware/` → PASS (all existing tests too).
- [ ] **Step 5: Commit** — `git commit -am "feat(middleware): mark caller-supplied timezones as explicit"`

---

### Task 3: `users.timezone` migration + sqlc queries

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260712000000.sql`
- Create: `internal/infra/storage/migrations/pgsql/20260712000000.sql`
- Modify: `internal/infra/storage/sqlc/query/sqlite/users.sql`, `internal/infra/storage/sqlc/query/pgsql/users.sql`
- Generated: `internal/infra/storage/sqlc/gen/{sqlite,pgsql}/*` (via sqlc)
- Test: `internal/infra/storage/migrations/migrations_test.go` (or the package's existing migration-test file — find it with `ls internal/infra/storage/migrations/*_test.go` and append there)

**Interfaces:**
- Produces: generated methods `GetUserTimezone(ctx, db, id string) (string, error)` and `UpdateUserTimezone(ctx, db, arg UpdateUserTimezoneParams) error` in BOTH `sqlitegen` and `pgsqlgen`; `sqlitegen.User` gains a `Timezone string` field. Task 4 consumes them.

- [ ] **Step 1: Write both migration files** (identical content; both engines accept it):

```sql
ALTER TABLE users ADD COLUMN timezone VARCHAR(64) NOT NULL DEFAULT '';
```

- [ ] **Step 2: Extend the user queries.** In `query/sqlite/users.sql`: add `timezone` to the SELECT column lists of `GetUserByID` and `GetUserByIdentifier` (keeping them equal to the full column set, so sqlc keeps returning the `User` table struct and the repo's `userRow` alias still compiles). Do NOT touch `UpsertUser` — unrelated user saves must never overwrite a persisted timezone. Append (ASCII comments only):

```sql
-- name: GetUserTimezone :one
SELECT timezone FROM users WHERE id = ?;

-- name: UpdateUserTimezone :exec
UPDATE users SET timezone = ? WHERE id = ?;
```

Mirror in `query/pgsql/users.sql` with `$1`/`$2` placeholders and the same SELECT-list additions.

- [ ] **Step 3: Regenerate** — run sqlc per the Makefile's pinned invocation; then `go build ./...`. Expected: `sqlitegen.User`/`pgsqlgen.User` gain `Timezone`; new methods + `UpdateUserTimezoneParams{Timezone, ID string}` appear in both packages; everything compiles (if `internal/user/repo` fails to compile because a query row type changed shape, the SELECT lists in Step 2 are wrong — fix there, don't touch the repo).

- [ ] **Step 4: Write the roundtrip test** (in the migrations test file found above; if none exists, create `internal/infra/storage/migrations/timezone_test.go` using `dbtest`):

```go
func TestUsersTimezoneColumn(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	id := f.User(fixture.User{})
	var tz string
	if err := db.Raw.QueryRow(db.Rebind("SELECT timezone FROM users WHERE id = ?"), id).Scan(&tz); err != nil {
		t.Fatal(err)
	}
	if tz != "" {
		t.Fatalf("default timezone = %q, want empty", tz)
	}
	if _, err := db.Raw.Exec(db.Rebind("UPDATE users SET timezone = ? WHERE id = ?"), "Europe/Amsterdam", id); err != nil {
		t.Fatal(err)
	}
	if err := db.Raw.QueryRow(db.Rebind("SELECT timezone FROM users WHERE id = ?"), id).Scan(&tz); err != nil {
		t.Fatal(err)
	}
	if tz != "Europe/Amsterdam" {
		t.Fatalf("timezone = %q", tz)
	}
}
```

- [ ] **Step 5: Run** `go test ./internal/infra/storage/... ./internal/user/...` → PASS.
- [ ] **Step 6: Commit** — `git add -A && git commit -m "feat(user): users.timezone column + sqlc queries"`

---

### Task 4: user repo + service timezone methods

**Files:**
- Modify: `internal/user/repository.go` (the `Repository` interface, line ~15)
- Modify: `internal/user/repo/repo.go` (querier interface + `Repo` methods), `internal/user/repo/sqlite.go`, `internal/user/repo/pgsql.go` (adapters — follow the existing per-method passthrough/shim style in those files exactly)
- Create: `internal/user/timezone.go`
- Test: `internal/user/timezone_test.go` (service-level, via dbtest; check how existing `internal/user/*_test.go` construct the Service and copy that setup)

**Interfaces:**
- Consumes: generated `GetUserTimezone`/`UpdateUserTimezone` (Task 3).
- Produces (Tasks 5 and 7 consume these exact signatures):
  - `user.Repository` gains `UpdateTimezone(ctx context.Context, id vo.Id, tz string) error` and `GetTimezone(ctx context.Context, id vo.Id) (string, error)`
  - `func (s *Service) PersistTimezone(ctx context.Context, userID vo.Id, tz string) error`
  - `func (s *Service) GetTimezone(ctx context.Context, userID vo.Id) (string, error)`

- [ ] **Step 1: Write the failing service test**

```go
func TestTimezonePersistAndGet(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	svc := newTestService(t, db) // reuse/extract the Service constructor helper from the existing user tests

	uid, err := vo.ParseId(userID)
	if err != nil {
		t.Fatal(err)
	}
	// Fresh user: empty.
	tz, err := svc.GetTimezone(context.Background(), uid)
	if err != nil || tz != "" {
		t.Fatalf("GetTimezone = %q, %v; want empty, nil", tz, err)
	}
	// Persist a valid IANA name.
	if err := svc.PersistTimezone(context.Background(), uid, "Europe/Amsterdam"); err != nil {
		t.Fatal(err)
	}
	tz, _ = svc.GetTimezone(context.Background(), uid)
	if tz != "Europe/Amsterdam" {
		t.Fatalf("persisted timezone = %q", tz)
	}
	// Invalid names are silently ignored (never fail the request path).
	if err := svc.PersistTimezone(context.Background(), uid, "Not/AZone"); err != nil {
		t.Fatal(err)
	}
	tz, _ = svc.GetTimezone(context.Background(), uid)
	if tz != "Europe/Amsterdam" {
		t.Fatalf("invalid name overwrote timezone: %q", tz)
	}
}
```

- [ ] **Step 2: Run** `go test ./internal/user/ -run TestTimezonePersist` → compile FAIL.

- [ ] **Step 3: Implement.**
  - `repository.go`: add the two methods to the `Repository` interface.
  - `repo/repo.go`: extend the `querier` interface —

```go
	GetUserTimezone(ctx context.Context, db backend.DBTX, id string) (string, error)
	UpdateUserTimezone(ctx context.Context, db backend.DBTX, p sqlitegen.UpdateUserTimezoneParams) error
```

  and add the `Repo` methods (follow the file's existing tx/error style; `sql.ErrNoRows` → `errs.NewNotFound("User not found")` like the neighbors):

```go
func (r *Repo) GetTimezone(ctx context.Context, id vo.Id) (string, error) {
	tz, err := r.q.GetUserTimezone(ctx, r.tx.DB(ctx), id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return "", errs.NewNotFound("User not found")
	}
	return tz, err
}

func (r *Repo) UpdateTimezone(ctx context.Context, id vo.Id, tz string) error {
	return r.q.UpdateUserTimezone(ctx, r.tx.DB(ctx), sqlitegen.UpdateUserTimezoneParams{Timezone: tz, ID: id.String()})
}
```

  (If the existing repo methods obtain the DBTX differently — e.g. `r.tx.DB(ctx)` is named otherwise — copy the neighboring methods' exact style.)
  - `repo/sqlite.go`: passthrough adapter methods; `repo/pgsql.go`: shim converting `sqlitegen.UpdateUserTimezoneParams` → `pgsqlgen.UpdateUserTimezoneParams` — both copying the file's existing per-method pattern.
  - `internal/user/timezone.go`:

```go
package user

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// PersistTimezone stores a caller-observed IANA timezone. It is called from
// the request hot path, so invalid names are dropped silently rather than
// surfaced: a bad header must never fail an otherwise-valid request.
func (s *Service) PersistTimezone(ctx context.Context, userID vo.Id, tz string) error {
	if tz == "" || tz == "Local" {
		return nil
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return nil
	}
	return s.repo.UpdateTimezone(ctx, userID, tz)
}

func (s *Service) GetTimezone(ctx context.Context, userID vo.Id) (string, error) {
	return s.repo.GetTimezone(ctx, userID)
}
```

- [ ] **Step 4: Run** `go test ./internal/user/...` → PASS.
- [ ] **Step 5: Commit** — `git add -A && git commit -m "feat(user): timezone persistence on the user aggregate"`

---

### Task 5: timezone-tracking authenticator + BuildAPI wiring

**Files:**
- Create: `internal/server/glue_timezone.go`
- Modify: `internal/server/server.go` (BuildAPI, lines ~191-208: pass the decorator instead of `userSvc` to every `RegisterAPI`)
- Test: `internal/server/glue_timezone_test.go`

**Interfaces:**
- Consumes: `middleware.TokenAuthenticator`, `reqctx.IsLocationExplicit`/`Location` (Task 1), `(*user.Service).PersistTimezone` (Task 4).
- Produces: `func NewTimezoneTrackingAuthenticator(inner middleware.TokenAuthenticator, users *appuser.Service) middleware.TokenAuthenticator`. Also `timezoneFallback(users *appuser.Service) middleware.Middleware` in the same file (consumed by Task 7 — defined here so this file owns both halves of the mechanism).

- [ ] **Step 1: Write the failing test**

```go
func TestTimezoneTrackingAuthenticator(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	userSvc := ... // build the real user.Service the way server_test.go / user tests do
	authn := NewTimezoneTrackingAuthenticator(authstub.Authenticator{}, userSvc)

	loc, _ := time.LoadLocation("Europe/Amsterdam")
	uid, _ := vo.ParseId(userID)

	// Non-explicit ctx (UTC default): no persist.
	ctx := reqctx.WithLocation(context.Background(), time.UTC)
	if _, _, err := authn.Authenticate(ctx, userID); err != nil {
		t.Fatal(err)
	}
	if tz, _ := userSvc.GetTimezone(context.Background(), uid); tz != "" {
		t.Fatalf("persisted %q for non-explicit request", tz)
	}

	// Explicit ctx: persisted.
	ctx = reqctx.WithExplicitLocation(context.Background(), loc)
	if _, _, err := authn.Authenticate(ctx, userID); err != nil {
		t.Fatal(err)
	}
	if tz, _ := userSvc.GetTimezone(context.Background(), uid); tz != "Europe/Amsterdam" {
		t.Fatalf("timezone = %q, want Europe/Amsterdam", tz)
	}

	// Failed auth: no persist, error passes through.
	if _, _, err := authn.Authenticate(ctx, "not-a-user-id"); err == nil {
		t.Fatal("want auth error")
	}
}
```

(The authstub token IS the user-id string, so `Authenticate(ctx, userID)` succeeds and yields `uid`.)

- [ ] **Step 2: Run** `go test ./internal/server/ -run TestTimezoneTracking` → compile FAIL.

- [ ] **Step 3: Implement `glue_timezone.go`:**

```go
package server

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	appuser "github.com/econumo/econumo/internal/user"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/middleware"
)

// timezoneTrackingAuthenticator decorates the per-request authenticator to
// opportunistically persist the caller's X-Timezone so header-less clients
// (MCP) can fall back to it. The in-memory last-seen cache keeps writes to
// ~one per user per boot/change; persist failures never fail the request.
type timezoneTrackingAuthenticator struct {
	inner middleware.TokenAuthenticator
	users *appuser.Service
	seen  sync.Map // vo.Id -> string (last persisted IANA name)
}

func NewTimezoneTrackingAuthenticator(inner middleware.TokenAuthenticator, users *appuser.Service) middleware.TokenAuthenticator {
	return &timezoneTrackingAuthenticator{inner: inner, users: users}
}

func (a *timezoneTrackingAuthenticator) Authenticate(ctx context.Context, token string) (vo.Id, vo.Id, error) {
	userID, tokenID, err := a.inner.Authenticate(ctx, token)
	if err != nil || !reqctx.IsLocationExplicit(ctx) {
		return userID, tokenID, err
	}
	tz := reqctx.Location(ctx).String()
	if prev, ok := a.seen.Load(userID); ok && prev.(string) == tz {
		return userID, tokenID, nil
	}
	if perr := a.users.PersistTimezone(ctx, userID, tz); perr != nil {
		slog.WarnContext(ctx, "timezone persist failed", slog.Any("err", perr))
	} else {
		a.seen.Store(userID, tz)
	}
	return userID, tokenID, nil
}

// timezoneFallback installs the stored timezone for requests that carried no
// X-Timezone header. Applied to /mcp only; REST keeps header-or-UTC.
func timezoneFallback(users *appuser.Service) middleware.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if !reqctx.IsLocationExplicit(ctx) {
				if userID, ok := middleware.UserIDFromCtx(ctx); ok {
					if tz, err := users.GetTimezone(ctx, userID); err == nil && tz != "" {
						if loc, lerr := time.LoadLocation(tz); lerr == nil {
							ctx = reqctx.WithLocation(ctx, loc)
							reqctx.AddLogAttr(ctx, "timezone", loc.String())
							r = r.WithContext(ctx)
						}
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 4: Wire into BuildAPI.** In `server.go`, right before the `router.Compose(...)` block: `authn := NewTimezoneTrackingAuthenticator(userSvc, userSvc)`, then replace `userSvc` with `authn` in ALL nine `RegisterAPI(...)` calls.

- [ ] **Step 5: Run** `go test ./internal/server/... && make go-test` → PASS (goldens unchanged: the decorator adds no wire behavior).
- [ ] **Step 6: Commit** — `git add -A && git commit -m "feat(server): opportunistic user-timezone tracking on authenticated requests"`

---

### Task 6: SDK dependency + `internal/web/mcp` core

**Files:**
- Modify: `go.mod`/`go.sum` (`go get github.com/modelcontextprotocol/go-sdk@v1.6.1`)
- Create: `internal/web/mcp/mcp.go`, `internal/web/mcp/helpers.go`
- Test: `internal/web/mcp/mcp_test.go`

**Interfaces:**
- Produces (all later tasks consume these exact names):
  - `type Register func(*sdk.Server)` and `func Compose(fns ...Register) Register`
  - `func NewHandler(register Register) http.Handler` — builds the `sdk.Server` (name `econumo`), applies `register`, returns the stateless/JSON Streamable HTTP handler.
  - `func UserID(ctx context.Context) (vo.Id, error)` — auth-context accessor for handlers.
  - `func JSONText(v any) (string, error)` — HTML-escape-free JSON.
  - `func MapErr(ctx context.Context, err error) error` — domain errs pass through (message-bearing tool error); anything else logs and returns `errors.New("Internal error")`.
  - `func AddJSONResource[T any](s *sdk.Server, uri, name, description string, load func(ctx context.Context, userID vo.Id) (T, error))`

- [ ] **Step 1: Add the dependency** — `go get github.com/modelcontextprotocol/go-sdk@v1.6.1 && go mod tidy`.

- [ ] **Step 2: Write the failing test** — spin the handler up via `httptest` and drive raw JSON-RPC (stateless mode needs no initialize before other calls; `Accept` must contain both content types):

```go
package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

func rpc(t *testing.T, url string, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("non-JSON response %q: %v", raw, err)
		}
	}
	return resp.StatusCode, out
}

type pingIn struct {
	Msg string `json:"msg" jsonschema:"message to echo"`
}
type pingOut struct {
	Echo string `json:"echo"`
}

func TestNewHandler_InitializeAndToolError(t *testing.T) {
	register := webmcp.Compose(func(s *sdk.Server) {
		sdk.AddTool(s, &sdk.Tool{Name: "ping", Description: "echo"},
			func(ctx context.Context, req *sdk.CallToolRequest, in pingIn) (*sdk.CallToolResult, pingOut, error) {
				if in.Msg == "boom-domain" {
					return nil, pingOut{}, webmcp.MapErr(ctx, errs.NewValidation("Category name must be 3-64 characters"))
				}
				if in.Msg == "boom-infra" {
					return nil, pingOut{}, webmcp.MapErr(ctx, errors.New("pq: connection refused on 10.0.0.5"))
				}
				return nil, pingOut{Echo: in.Msg}, nil
			})
	})
	ts := httptest.NewServer(webmcp.NewHandler(register))
	defer ts.Close()

	status, out := rpc(t, ts.URL, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	if status != 200 {
		t.Fatalf("initialize status %d", status)
	}
	if !bytes.Contains(mustJSON(t, out), []byte(`"name":"econumo"`)) {
		t.Fatalf("serverInfo missing econumo: %v", out)
	}

	_, out = rpc(t, ts.URL, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ping","arguments":{"msg":"hi"}}}`)
	if !bytes.Contains(mustJSON(t, out), []byte(`"echo":"hi"`)) {
		t.Fatalf("tool call: %v", out)
	}

	// Domain error: isError tool result carrying the exact message.
	_, out = rpc(t, ts.URL, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ping","arguments":{"msg":"boom-domain"}}}`)
	if s := string(mustJSON(t, out)); !strings.Contains(s, `"isError":true`) || !strings.Contains(s, "Category name must be 3-64 characters") {
		t.Fatalf("domain error: %s", s)
	}

	// Infra error: isError with the sanitized message only.
	_, out = rpc(t, ts.URL, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"ping","arguments":{"msg":"boom-infra"}}}`)
	if s := string(mustJSON(t, out)); !strings.Contains(s, "Internal error") || strings.Contains(s, "10.0.0.5") {
		t.Fatalf("infra error leaked or missing: %s", s)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestJSONTextNoHTMLEscaping(t *testing.T) {
	got, err := webmcp.JSONText(map[string]string{"a": "x<y>/z"})
	if err != nil || got != `{"a":"x<y>/z"}` {
		t.Fatalf("JSONText = %q, %v", got, err)
	}
}

func TestUserIDMissing(t *testing.T) {
	if _, err := webmcp.UserID(context.Background()); err == nil {
		t.Fatal("want error on missing user")
	}
	_ = vo.Id{}
}
```

- [ ] **Step 3: Run** `go test ./internal/web/mcp/` → compile FAIL.

- [ ] **Step 4: Implement `mcp.go`:**

```go
// Package mcp is the MCP edge shared by every feature: it builds the SDK
// server, mounts the Streamable HTTP transport (stateless + JSON responses:
// tools are sub-second DB calls with nothing to stream, and statelessness
// keeps /mcp restart-safe and proxy-friendly), and defines the seam through
// which feature packages register their tools/resources/prompts.
package mcp

import (
	"net/http"
	"runtime/debug"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type Register func(*sdk.Server)

func Compose(fns ...Register) Register {
	return func(s *sdk.Server) {
		for _, fn := range fns {
			if fn != nil {
				fn(s)
			}
		}
	}
}

const instructions = "Econumo personal-finance server. Read reference data from the econumo:// " +
	"resources (accounts, categories, tags, payees, currencies, budgets, user); query monthly " +
	"budget state with get_budget and transactions with list_transactions; log changes with " +
	"create_transaction / update_transaction / delete_transaction. Amounts are decimal strings; " +
	"ids are UUIDs from the resources."

func NewHandler(register Register) http.Handler {
	srv := sdk.NewServer(
		&sdk.Implementation{Name: "econumo", Version: serverVersion()},
		&sdk.ServerOptions{Instructions: instructions},
	)
	register(srv)
	return sdk.NewStreamableHTTPHandler(
		func(*http.Request) *sdk.Server { return srv },
		&sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
}

func serverVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}
```

- [ ] **Step 5: Implement `helpers.go`:**

```go
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/middleware"
)

// UserID returns the authenticated caller. /mcp sits behind the auth
// middleware, so absence is a programming error, not a client condition.
func UserID(ctx context.Context) (vo.Id, error) {
	id, ok := middleware.UserIDFromCtx(ctx)
	if !ok {
		return vo.Id{}, errors.New("Internal error")
	}
	return id, nil
}

// JSONText marshals for MCP payloads with the same HTML-escaping-off policy
// as the REST envelope.
func JSONText(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

// MapErr shapes a use-case error for the model: domain errors keep their
// message (typed SDK handlers turn any returned error into an isError tool
// result the model can read and self-correct on); everything else is
// infrastructure — logged here, replaced by a static message so no internals
// leak.
func MapErr(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errs.AsValidation(err); ok {
		return err
	}
	if _, ok := errs.AsNotFound(err); ok {
		return err
	}
	if _, ok := errs.AsAccessDenied(err); ok {
		return err
	}
	slog.ErrorContext(ctx, "mcp internal error", slog.Any("err", err))
	return errors.New("Internal error")
}

// AddJSONResource registers a per-user JSON resource with the shared
// load-marshal-wrap plumbing.
func AddJSONResource[T any](s *sdk.Server, uri, name, description string, load func(ctx context.Context, userID vo.Id) (T, error)) {
	s.AddResource(
		&sdk.Resource{URI: uri, Name: name, Description: description, MIMEType: "application/json"},
		func(ctx context.Context, req *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
			reqctx.AddLogAttr(ctx, "resource", uri)
			userID, err := UserID(ctx)
			if err != nil {
				return nil, err
			}
			v, err := load(ctx, userID)
			if err != nil {
				return nil, MapErr(ctx, err)
			}
			text, err := JSONText(v)
			if err != nil {
				return nil, MapErr(ctx, err)
			}
			return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{
				{URI: req.Params.URI, MIMEType: "application/json", Text: text},
			}}, nil
		})
}
```

- [ ] **Step 6: Run** `go test ./internal/web/mcp/` → PASS.
- [ ] **Step 7: Commit** — `git add -A && git commit -m "feat(mcp): MCP edge core - SDK server, streamable handler, registration seam"`

---

### Task 7: mount `/mcp` — router + BuildAPI

**Files:**
- Modify: `internal/web/router/router.go` (add `MCP http.Handler` to `Deps`; mount)
- Modify: `internal/server/server.go` (build the MCP handler chain, pass to router)
- Test: `internal/web/router/router_test.go` (mount presence), `internal/server/mcp_test.go` (end-to-end auth)

**Interfaces:**
- Consumes: `webmcp.NewHandler`/`Compose` (Task 6), `timezoneFallback` (Task 5), `middleware.Auth`.
- Produces: `router.Deps.MCP http.Handler` (nil = not mounted); `/mcp` serves the MCP endpoint behind auth in the production handler. Tasks 8-14 build on the live endpoint.

- [ ] **Step 1: Failing router test** (append to `router_test.go`):

```go
func TestRouter_MountsMCP(t *testing.T) {
	var hit bool
	h := router.New(router.Deps{Cfg: config.Config{}, MCP: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
	})})
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
	h.ServeHTTP(httptest.NewRecorder(), req)
	if !hit {
		t.Fatal("POST /mcp did not reach the MCP handler")
	}
	// Without a handler the SPA fallback answers; just assert no panic.
	h = router.New(router.Deps{Cfg: config.Config{}})
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/mcp", nil))
}
```

- [ ] **Step 2: Run** → FAIL. **Implement in `router.go`:** add to `Deps`:

```go
	// MCP is the fully-wrapped MCP endpoint handler (auth + timezone fallback
	// applied by the composition root). Nil = endpoint not mounted.
	MCP http.Handler
```

and in `New`, after the API subtree mount:

```go
	// MCP endpoint. Mounted at the root (outside /api: JSON-RPC, not the REST
	// contract, so the apiparity machinery must not scan it) but inside the
	// same global chain.
	if deps.MCP != nil {
		root.Handle("/mcp", global(deps.MCP))
	}
```

- [ ] **Step 3: Wire in `server.go`** (before the `router.New` return):

```go
	mcpRegister := webmcp.Compose(
	// Feature registrations land here task by task.
	)
	mcpHandler := middleware.Chain(
		middleware.Auth(authn, cfg.IsDev()),
		timezoneFallback(userSvc),
	)(webmcp.NewHandler(mcpRegister))
```

and add `MCP: mcpHandler` to the `router.Deps` literal. Import `webmcp "github.com/econumo/econumo/internal/web/mcp"`.

- [ ] **Step 4: Failing end-to-end test** (`internal/server/mcp_test.go`; build cfg/db the way existing server tests or `apiparity.NewHarness` do — dbtest + fixture user + fixture.AccessToken with a known raw token hashed via `appuser.HashAccessToken`):

```go
func TestMCP_AuthGate(t *testing.T) {
	handler, rawToken := buildTestAPIWithToken(t) // helper: BuildAPI over dbtest + one fixture user + one session token
	ts := httptest.NewServer(handler)
	defer ts.Close()

	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`

	// No token: the standard 401 envelope, MCP never runs.
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(initBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 401 || !strings.Contains(string(body), `"Access token not found"`) {
		t.Fatalf("unauth: %d %s", resp.StatusCode, body)
	}

	// Valid token: initialize succeeds with serverInfo name econumo.
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(initBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+rawToken)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), `"name":"econumo"`) {
		t.Fatalf("auth init: %d %s", resp.StatusCode, body)
	}
}
```

- [ ] **Step 5: Run** `go test ./internal/web/router/ ./internal/server/ && make go-test` → PASS.
- [ ] **Step 6: Commit** — `git add -A && git commit -m "feat(mcp): mount /mcp behind bearer auth with stored-timezone fallback"`

---

### Task 8: dictionary resources — categories, tags, payees

**Files:**
- Create: `internal/category/mcp/mcp.go`, `internal/tag/mcp/mcp.go`, `internal/payee/mcp/mcp.go`
- Modify: `internal/server/server.go` (add the three registrations to `webmcp.Compose`)
- Test: `internal/category/mcp/mcp_test.go` (exemplar; tag/payee tests are the same shape)

**Interfaces:**
- Consumes: `webmcp.AddJSONResource` (Task 6); `(*category.ReadService).GetCategoryList(ctx, userID) (*model.GetCategoryListResult, error)`; same-shape `GetTagList`/`GetPayeeList` on the tag/payee `ReadService`s.
- Produces: `categorymcp.Register(read *category.ReadService) webmcp.Register` (same for tag/payee); resources `econumo://categories|tags|payees` whose JSON body is the result's `Items` slice.

- [ ] **Step 1: Failing test** (`internal/category/mcp/mcp_test.go`). Build the ReadService over dbtest the way `internal/category/read_test.go` does, seed one category via fixture, then drive the SDK in-memory (no HTTP):

```go
func TestCategoriesResource(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	f.Category(fixture.Category{UserID: userID, Name: "Groceries"}) // match the fixture builder's actual field names

	read := newReadService(t, db) // copy construction from the category read tests

	srv := sdk.NewServer(&sdk.Implementation{Name: "t", Version: "t"}, nil)
	categorymcp.Register(read)(srv)

	// Handlers read the user from the auth middleware's context key. In-memory
	// SDK sessions hand handlers the SERVER session's context, so the
	// user-carrying context (built by running the real Auth middleware over
	// authstub — see the mcptest helper below) goes to srv.Connect.
	ctx := mcptest.CtxWithUser(t, userID)

	st, ct := sdk.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()

	client := sdk.NewClient(&sdk.Implementation{Name: "c", Version: "t"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	res, err := cs.ReadResource(ctx, &sdk.ReadResourceParams{URI: "econumo://categories"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Contents) != 1 || !strings.Contains(res.Contents[0].Text, `"Groceries"`) {
		t.Fatalf("contents: %+v", res.Contents)
	}
}
```

**IMPORTANT implementation note for this test:** the server-side handler receives the context of the SERVER session (`srv.Connect(ctx, ...)`), not the client's. Pass the user-carrying context to `srv.Connect`, not (only) to the client. Build it with a shared helper — create `internal/test/mcptest/mcptest.go`:

```go
// Package mcptest provides the context plumbing feature mcp tests need: a
// context carrying the auth middleware's user id, produced by running the
// REAL middleware over a stub authenticator.
package mcptest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/econumo/econumo/internal/test/authstub"
	"github.com/econumo/econumo/internal/web/middleware"
)

func CtxWithUser(t testing.TB, userID string) context.Context {
	t.Helper()
	var ctx context.Context
	h := middleware.Auth(authstub.Authenticator{}, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx = r.Context()
	}))
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+userID)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if ctx == nil {
		t.Fatal("auth middleware rejected the stub token")
	}
	return ctx
}
```

- [ ] **Step 2: Run** → compile FAIL.

- [ ] **Step 3: Implement.** `internal/category/mcp/mcp.go`:

```go
// Package mcp is the category feature's MCP edge.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appcategory "github.com/econumo/econumo/internal/category"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

func Register(read *appcategory.ReadService) webmcp.Register {
	return func(s *sdk.Server) {
		webmcp.AddJSONResource(s, "econumo://categories", "categories",
			"The user's transaction categories: id, name, type (expense|income), isArchived (0/1).",
			func(ctx context.Context, userID vo.Id) ([]model.CategoryResult, error) {
				res, err := read.GetCategoryList(ctx, userID)
				if err != nil {
					return nil, err
				}
				return res.Items, nil
			})
	}
}
```

`internal/tag/mcp/mcp.go` and `internal/payee/mcp/mcp.go` are the same shape over `GetTagList` → `[]model.TagResult` (URI `econumo://tags`, description "The user's transaction tags: id, name, isArchived (0/1).") and `GetPayeeList` → `[]model.PayeeResult` (URI `econumo://payees`, description "The user's payees: id, name, isArchived (0/1).").

- [ ] **Step 4: Wire** — in `server.go`'s `webmcp.Compose(...)`: `categorymcp.Register(categoryReadSvc), tagmcp.Register(tagReadSvc), payeemcp.Register(payeeReadSvc)` (match the actual read-service variable names used in BuildAPI).
- [ ] **Step 5: Run** `go test ./internal/category/... ./internal/tag/... ./internal/payee/... ./internal/server/` → PASS. Also `go test ./internal/test/archtest/` (new feature mcp subpackages must pass the dependency rules).
- [ ] **Step 6: Commit** — `git add -A && git commit -m "feat(mcp): categories/tags/payees resources"`

---

### Task 9: accounts, currencies, budgets resources

**Files:**
- Create: `internal/account/mcp/mcp.go`, `internal/currency/mcp/mcp.go`, `internal/budget/mcp/mcp.go`
- Modify: `internal/server/server.go` (Compose additions)
- Test: `internal/account/mcp/mcp_test.go` (exemplar — includes the timezone-sensitive balance assertion)

**Interfaces:**
- Consumes: `(*account.Service).AccountListForUser(ctx, userID) ([]model.AccountResult, error)`; `(*currency.ReadService).GetCurrencyList` + `GetCurrencyRateList`; `(*budget.Service).GetBudgetList(ctx, userID) (*model.GetBudgetListResult, error)`; `mcptest.CtxWithUser` (Task 8).
- Produces: `accountmcp.Register(svc *account.Service) webmcp.Register`; `currencymcp.Register(read *currency.ReadService) webmcp.Register`; `budgetmcp.Register(svc *budget.Service) webmcp.Register`. Resource JSON shapes: accounts/budgets = the `Items` slice; currencies = `{"currencies": [...model.CurrencyResult], "rates": [...model.CurrencyRateResult]}`.

- [ ] **Step 1: Failing account test** — same harness shape as Task 8 (build `account.Service` the way `internal/account/read_test.go` does; seed user + account via fixture). Assert the resource text contains the account name and a `"balance"` key. Balance-timezone note: the resource handler just calls the service — `reqctx.Location(ctx)` flows from the request context, which Task 7's fallback middleware set; no timezone logic lives in this package (assert nothing timezone-specific here beyond the call succeeding with a bare context = UTC).

- [ ] **Step 2: Implement.** `internal/account/mcp/mcp.go`:

```go
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appaccount "github.com/econumo/econumo/internal/account"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

func Register(svc *appaccount.Service) webmcp.Register {
	return func(s *sdk.Server) {
		webmcp.AddJSONResource(s, "econumo://accounts", "accounts",
			"The user's accounts with current balances (as of end of the user's today): id, name, currency, balance, owner, sharedAccess.",
			func(ctx context.Context, userID vo.Id) ([]model.AccountResult, error) {
				return svc.AccountListForUser(ctx, userID)
			})
	}
}
```

`internal/currency/mcp/mcp.go`:

```go
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appcurrency "github.com/econumo/econumo/internal/currency"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

type currenciesDoc struct {
	Currencies []model.CurrencyResult     `json:"currencies"`
	Rates      []model.CurrencyRateResult `json:"rates"`
}

func Register(read *appcurrency.ReadService) webmcp.Register {
	return func(s *sdk.Server) {
		webmcp.AddJSONResource(s, "econumo://currencies", "currencies",
			"Known currencies plus the latest exchange rates against the instance base currency.",
			func(ctx context.Context, userID vo.Id) (currenciesDoc, error) {
				list, err := read.GetCurrencyList(ctx, userID)
				if err != nil {
					return currenciesDoc{}, err
				}
				rates, err := read.GetCurrencyRateList(ctx, userID)
				if err != nil {
					return currenciesDoc{}, err
				}
				return currenciesDoc{Currencies: list.Items, Rates: rates.Items}, nil
			})
	}
}
```

`internal/budget/mcp/mcp.go` — `econumo://budgets`, description "The user's budgets (id, name, currency); pass a budget id and month to the get_budget tool for monthly state.", loader returns `res.Items` (`[]model.MetaResult`) from `svc.GetBudgetList`.

- [ ] **Step 3: Wire** the three registrations into `webmcp.Compose` in `server.go`.
- [ ] **Step 4: Run** `go test ./internal/account/... ./internal/currency/... ./internal/budget/... ./internal/server/ ./internal/test/archtest/` → PASS.
- [ ] **Step 5: Commit** — `git add -A && git commit -m "feat(mcp): accounts/currencies/budgets resources"`

---

### Task 10: user resource (profile + connections)

**Files:**
- Create: `internal/user/mcp/mcp.go`
- Modify: `internal/server/server.go` (Compose addition — pass `connectionSvc` directly; its `GetConnectionList` already satisfies the port)
- Test: `internal/user/mcp/mcp_test.go`

**Interfaces:**
- Consumes: `(*user.ReadService).GetUserData(ctx, userID) (*model.GetUserDataResult, error)`; the connection feature's `(*connection.Service).GetConnectionList(ctx, userID) (*model.GetConnectionListResult, error)`.
- Produces: `usermcp.Register(read *user.ReadService, connections usermcp.ConnectionLister) webmcp.Register` with

```go
type ConnectionLister interface {
	GetConnectionList(ctx context.Context, userID vo.Id) (*model.GetConnectionListResult, error)
}
```

Resource `econumo://user` JSON: `{"user": <CurrentUserResult>, "connections": [<ConnectionResult>]}`.

- [ ] **Step 1: Failing test** — harness as in Task 8; user feature ReadService construction copied from `internal/user/read_test.go`; a stub `ConnectionLister` returning one fixed `model.ConnectionResult`. Assert the text contains the user's email and the connection user's name.

- [ ] **Step 2: Implement:**

```go
// Package mcp is the user feature's MCP edge.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	appuser "github.com/econumo/econumo/internal/user"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

// ConnectionLister is the consumer-side port for the connection feature
// (features never import features; server wires the concrete service, whose
// GetConnectionList satisfies this directly).
type ConnectionLister interface {
	GetConnectionList(ctx context.Context, userID vo.Id) (*model.GetConnectionListResult, error)
}

type userDoc struct {
	User        model.CurrentUserResult  `json:"user"`
	Connections []model.ConnectionResult `json:"connections"`
}

func Register(read *appuser.ReadService, connections ConnectionLister) webmcp.Register {
	return func(s *sdk.Server) {
		webmcp.AddJSONResource(s, "econumo://user", "user",
			"The authenticated user's profile (id, name, email, avatar, base currency) and connected users with shared-account access.",
			func(ctx context.Context, userID vo.Id) (userDoc, error) {
				u, err := read.GetUserData(ctx, userID)
				if err != nil {
					return userDoc{}, err
				}
				conns, err := connections.GetConnectionList(ctx, userID)
				if err != nil {
					return userDoc{}, err
				}
				return userDoc{User: u.User, Connections: conns.Items}, nil
			})
	}
}
```

- [ ] **Step 3: Wire** — `usermcp.Register(userReadSvc, connectionSvc)` in Compose (match actual variable names).
- [ ] **Step 4: Run** `go test ./internal/user/... ./internal/server/ ./internal/test/archtest/` → PASS.
- [ ] **Step 5: Commit** — `git add -A && git commit -m "feat(mcp): user resource with connections"`

---

### Task 11: transaction tools

**Files:**
- Create: `internal/transaction/mcp/mcp.go`
- Modify: `internal/server/server.go` (Compose addition)
- Test: `internal/transaction/mcp/mcp_test.go`

**Interfaces:**
- Consumes: `(*transaction.Service)` methods `CreateTransaction`, `UpdateTransaction`, `DeleteTransaction`, `GetTransactionList` (signatures per `internal/transaction/{create,update,delete,read}.go`); `vo.NewId()`; `vo.FlexString`; `datetime.Layout`/`DateLayout`.
- Produces: `transactionmcp.Register(svc *transaction.Service) webmcp.Register` adding tools `list_transactions`, `create_transaction`, `update_transaction`, `delete_transaction`. Output types are the existing model result DTOs (their JSON tags are the frozen REST shapes — reused verbatim).

- [ ] **Step 1: Failing test** — dbtest + fixture (user, account, category — copy construction from `internal/transaction/create_test.go`), SDK in-memory session via `mcptest.CtxWithUser`. Exercise: `create_transaction` (expense, date `"2024-04-02"`, amount `"12.50"`) → result's `structuredContent.item.id` non-empty and `type":"expense"`; `list_transactions` (no filters) → contains the created id; `update_transaction` (change amount) → ok; `delete_transaction` → ok; then `list_transactions` → empty. Error path: `create_transaction` with `category_id` of a non-existent UUID → `IsError` true, message text non-empty, no stack/driver strings.

- [ ] **Step 2: Implement `internal/transaction/mcp/mcp.go`:**

```go
// Package mcp is the transaction feature's MCP edge: the write tools plus
// the filtered list read.
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	apptransaction "github.com/econumo/econumo/internal/transaction"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

type listInput struct {
	AccountID   string `json:"account_id,omitempty" jsonschema:"filter by account id (UUID)"`
	PeriodStart string `json:"period_start,omitempty" jsonschema:"inclusive start, YYYY-MM-DD or 'YYYY-MM-DD HH:MM:SS'"`
	PeriodEnd   string `json:"period_end,omitempty" jsonschema:"inclusive end, YYYY-MM-DD or 'YYYY-MM-DD HH:MM:SS'"`
}

type txFields struct {
	Type               string `json:"type" jsonschema:"expense, income or transfer"`
	Amount             string `json:"amount" jsonschema:"decimal string, e.g. 12.50"`
	AccountID          string `json:"account_id" jsonschema:"source account id (UUID)"`
	Date               string `json:"date" jsonschema:"YYYY-MM-DD or 'YYYY-MM-DD HH:MM:SS'"`
	CategoryID         string `json:"category_id,omitempty" jsonschema:"category id (UUID); required unless type is transfer"`
	AccountRecipientID string `json:"account_recipient_id,omitempty" jsonschema:"transfer target account id (UUID)"`
	AmountRecipient    string `json:"amount_recipient,omitempty" jsonschema:"received amount for cross-currency transfers"`
	Description        string `json:"description,omitempty"`
	PayeeID            string `json:"payee_id,omitempty" jsonschema:"payee id (UUID)"`
	TagID              string `json:"tag_id,omitempty" jsonschema:"tag id (UUID)"`
}

type createInput struct{ txFields }

type updateInput struct {
	ID string `json:"id" jsonschema:"transaction id (UUID)"`
	txFields
}

type deleteInput struct {
	ID string `json:"id" jsonschema:"transaction id (UUID)"`
}

// expand widens a date-only value to the wire datetime; end=true lands on the
// last second of the day so date-only ranges are inclusive.
func expand(s string, end bool) string {
	if len(s) == len(datetime.DateLayout) {
		if end {
			return s + " 23:59:59"
		}
		return s + " 00:00:00"
	}
	return s
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func flexPtr(s string) *vo.FlexString {
	if s == "" {
		return nil
	}
	f := vo.FlexString(s)
	return &f
}

func (f txFields) toRequestFields() (typ string, amount vo.FlexString, accountID, date string,
	categoryID, accountRecipientID *string, amountRecipient *vo.FlexString, description, payeeID, tagID *string) {
	return f.Type, vo.FlexString(f.Amount), f.AccountID, expand(f.Date, false),
		strPtr(f.CategoryID), strPtr(f.AccountRecipientID), flexPtr(f.AmountRecipient),
		strPtr(f.Description), strPtr(f.PayeeID), strPtr(f.TagID)
}

func Register(svc *apptransaction.Service) webmcp.Register {
	return func(s *sdk.Server) {
		sdk.AddTool(s, &sdk.Tool{Name: "list_transactions",
			Description: "List the user's transactions, optionally filtered by account and/or period."},
			func(ctx context.Context, req *sdk.CallToolRequest, in listInput) (*sdk.CallToolResult, model.GetTransactionListResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "list_transactions")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.GetTransactionListResult{}, err
				}
				res, err := svc.GetTransactionList(ctx, userID, model.TransactionListRequest{
					AccountId:   in.AccountID,
					PeriodStart: expand(in.PeriodStart, false),
					PeriodEnd:   expand(in.PeriodEnd, true),
				})
				if err != nil {
					return nil, model.GetTransactionListResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "create_transaction",
			Description: "Record a new expense, income or transfer. Look up account/category/payee/tag ids in the econumo:// resources first."},
			func(ctx context.Context, req *sdk.CallToolRequest, in createInput) (*sdk.CallToolResult, model.CreateTransactionResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "create_transaction")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.CreateTransactionResult{}, err
				}
				typ, amount, accountID, date, categoryID, accountRecipientID, amountRecipient, description, payeeID, tagID := in.toRequestFields()
				res, err := svc.CreateTransaction(ctx, userID, model.CreateTransactionRequest{
					Id:                 vo.NewId().String(), // operation id, minted server-side for MCP
					Type:               typ,
					Amount:             amount,
					AccountId:          accountID,
					AccountRecipientId: accountRecipientID,
					AmountRecipient:    amountRecipient,
					CategoryId:         categoryID,
					Date:               date,
					Description:        description,
					PayeeId:            payeeID,
					TagId:              tagID,
				})
				if err != nil {
					return nil, model.CreateTransactionResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "update_transaction",
			Description: "Update an existing transaction; send the full new field set (type, amount, account_id, date, ...)."},
			func(ctx context.Context, req *sdk.CallToolRequest, in updateInput) (*sdk.CallToolResult, model.UpdateTransactionResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "update_transaction")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.UpdateTransactionResult{}, err
				}
				typ, amount, accountID, date, categoryID, accountRecipientID, amountRecipient, description, payeeID, tagID := in.toRequestFields()
				res, err := svc.UpdateTransaction(ctx, userID, model.UpdateTransactionRequest{
					Id:                 in.ID,
					Type:               typ,
					Amount:             amount,
					AccountId:          accountID,
					AccountRecipientId: accountRecipientID,
					AmountRecipient:    amountRecipient,
					CategoryId:         categoryID,
					Date:               date,
					Description:        description,
					PayeeId:            payeeID,
					TagId:              tagID,
				})
				if err != nil {
					return nil, model.UpdateTransactionResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})

		sdk.AddTool(s, &sdk.Tool{Name: "delete_transaction",
			Description: "Delete a transaction by id."},
			func(ctx context.Context, req *sdk.CallToolRequest, in deleteInput) (*sdk.CallToolResult, model.DeleteTransactionResult, error) {
				reqctx.AddLogAttr(ctx, "tool", "delete_transaction")
				userID, err := webmcp.UserID(ctx)
				if err != nil {
					return nil, model.DeleteTransactionResult{}, err
				}
				res, err := svc.DeleteTransaction(ctx, userID, model.DeleteTransactionRequest{Id: in.ID})
				if err != nil {
					return nil, model.DeleteTransactionResult{}, webmcp.MapErr(ctx, err)
				}
				return nil, *res, nil
			})
	}
}
```

**Check against the actual DTO field names** (`internal/model/transaction_dto.go:36-133`) before compiling — in particular `UpdateTransactionRequest` fields must be filled the same way. If `UpdateTransactionRequest` lacks a field used above, drop it there.

- [ ] **Step 3: Wire** — `transactionmcp.Register(transactionSvc)` in Compose.
- [ ] **Step 4: Run** `go test ./internal/transaction/... ./internal/server/ ./internal/test/archtest/` → PASS.
- [ ] **Step 5: Commit** — `git add -A && git commit -m "feat(mcp): transaction tools (list/create/update/delete)"`

---

### Task 12: budget tool — `get_budget`

**Files:**
- Modify: `internal/budget/mcp/mcp.go` (package exists since Task 9 — add the tool inside the existing `Register`, alongside the budgets resource; add `time` and `errs` imports)
- Test: extend `internal/budget/mcp/mcp_test.go`

**Interfaces:**
- Consumes: `(*budget.Service).GetBudget(ctx, userID, model.GetBudgetRequest{Id, Date string}) (*model.GetBudgetResult, error)` — `Date` may be `"YYYY-MM-DD"`; empty defaults to caller-local current month.
- Produces: tool `get_budget` with input `{budget_id string, month string?}` (month `YYYY-MM`), output `model.GetBudgetResult`.

- [ ] **Step 1: Failing test** — seed a budget the way `internal/budget/read_test.go` does (or create one through the service), call the tool via in-memory session; assert `structuredContent.item.meta.id` matches and a bad `month` value (`"junk"`) yields `IsError` (validation).

- [ ] **Step 2: Implement** — add inside `Register` (same func as the budgets resource):

```go
type getBudgetInput struct {
	BudgetID string `json:"budget_id" jsonschema:"budget id (UUID), from econumo://budgets"`
	Month    string `json:"month,omitempty" jsonschema:"YYYY-MM; defaults to the current month"`
}

sdk.AddTool(s, &sdk.Tool{Name: "get_budget",
	Description: "Full monthly budget state: folders, envelopes, categories, tags, limits, spent and available amounts."},
	func(ctx context.Context, req *sdk.CallToolRequest, in getBudgetInput) (*sdk.CallToolResult, model.GetBudgetResult, error) {
		reqctx.AddLogAttr(ctx, "tool", "get_budget")
		userID, err := webmcp.UserID(ctx)
		if err != nil {
			return nil, model.GetBudgetResult{}, err
		}
		date := ""
		if in.Month != "" {
			if _, perr := time.Parse("2006-01", in.Month); perr != nil {
				return nil, model.GetBudgetResult{}, errs.NewValidation("month must be YYYY-MM")
			}
			date = in.Month + "-01"
		}
		res, err := svc.GetBudget(ctx, userID, model.GetBudgetRequest{Id: in.BudgetID, Date: date})
		if err != nil {
			return nil, model.GetBudgetResult{}, webmcp.MapErr(ctx, err)
		}
		return nil, *res, nil
	})
```

- [ ] **Step 3: Run** `go test ./internal/budget/...` → PASS.
- [ ] **Step 4: Commit** — `git add -A && git commit -m "feat(mcp): get_budget tool"`

---

### Task 13: prompts

**Files:**
- Create: `internal/web/mcp/prompts.go`
- Modify: `internal/web/mcp/mcp.go` (`NewHandler` calls `addPrompts(srv)` after `register`)
- Test: extend `internal/web/mcp/mcp_test.go`

**Interfaces:**
- Produces: prompts `log-expense` (required arg `description`) and `budget-review` (optional arg `month`). Static templates — no feature imports (this package must never import a feature).

- [ ] **Step 1: Failing test** — via the Task 6 `rpc` helper: `prompts/list` returns both names; `prompts/get log-expense` with `{"description":"5 coffee"}` returns one user message whose text contains `5 coffee` and `create_transaction`; `prompts/get budget-review` without arguments contains `get_budget`.

- [ ] **Step 2: Implement `prompts.go`:**

```go
package mcp

import (
	"context"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/econumo/econumo/internal/shared/reqctx"
)

func addPrompts(s *sdk.Server) {
	s.AddPrompt(&sdk.Prompt{
		Name:        "log-expense",
		Description: "Log a transaction in Econumo from a free-text description.",
		Arguments: []*sdk.PromptArgument{{
			Name: "description", Description: "free text, e.g. '27.50 groceries at Lidl yesterday'", Required: true,
		}},
	}, func(ctx context.Context, req *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
		reqctx.AddLogAttr(ctx, "prompt", "log-expense")
		text := fmt.Sprintf(`Log this in my Econumo finance tracker: %s

Follow these steps:
1. Read econumo://accounts, econumo://categories and econumo://payees.
2. Work out the type (expense unless clearly income or a transfer), the amount (decimal string), the date (default: today), and the best-matching account, category and payee (payee may be omitted).
3. Call create_transaction.
4. Confirm in one line what you logged: amount, currency, category, account, date.
If the amount or the account is ambiguous, ask me before creating anything.`,
			req.Params.Arguments["description"])
		return &sdk.GetPromptResult{Messages: []*sdk.PromptMessage{
			{Role: "user", Content: &sdk.TextContent{Text: text}},
		}}, nil
	})

	s.AddPrompt(&sdk.Prompt{
		Name:        "budget-review",
		Description: "Review the monthly budget: limits vs spending, overspends, notable items.",
		Arguments: []*sdk.PromptArgument{{
			Name: "month", Description: "YYYY-MM; defaults to the current month", Required: false,
		}},
	}, func(ctx context.Context, req *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
		reqctx.AddLogAttr(ctx, "prompt", "budget-review")
		month := req.Params.Arguments["month"]
		if month == "" {
			month = "the current month"
		}
		text := fmt.Sprintf(`Review my Econumo budget for %s.

Follow these steps:
1. Read econumo://budgets; if I have more than one budget, ask which one.
2. Call get_budget with the budget_id (and the month, if not current).
3. Compare limits against spending per envelope/category: flag anything overspent, and anything above 90%% of its limit.
4. If something looks unusual, sample the underlying activity with list_transactions.
5. Reply with a short structured review in my language: overall position, top overspends, notable items, one concrete suggestion.`,
			month)
		return &sdk.GetPromptResult{Messages: []*sdk.PromptMessage{
			{Role: "user", Content: &sdk.TextContent{Text: text}},
		}}, nil
	})
}
```

In `NewHandler`, after `register(srv)`: `addPrompts(srv)`.

- [ ] **Step 3: Run** `go test ./internal/web/mcp/` → PASS.
- [ ] **Step 4: Commit** — `git add -A && git commit -m "feat(mcp): log-expense and budget-review prompts"`

---

### Task 14: golden-file scenario suite (`mcpparity`) + engine comparison

**Files:**
- Modify: `internal/test/apiparity/harness.go` — add `func (h *Harness) URL() string { return h.srv.URL }`
- Create: `internal/test/mcpparity/mcpparity.go` (runner), `internal/test/mcpparity/catalogue.go` (scenarios), `internal/test/mcpparity/smoke_test.go`, `internal/test/mcpparity/enginecompare_test.go`, `internal/test/mcpparity/testdata/golden/` (generated)

**Interfaces:**
- Consumes: `apiparity.NewHarness/Seed/OwnerID/OwnerToken/Call` + `apiparity.NormalizeGolden/NormalizeParity`; the full `/mcp` surface (Tasks 7-13).
- Produces: frozen MCP wire goldens; `Catalogue()` shared by smoke + enginecompare.

- [ ] **Step 1: Runner (`mcpparity.go`)** — a scenario is an ordered list of steps; REST steps reuse `h.Call` for seeding, MCP steps post JSON-RPC:

```go
// Package mcpparity freezes the MCP endpoint's wire behavior with golden
// files, mirroring apiparity: same harness, same normalization, sqlite
// goldens in the smoke tier and a build-tagged sqlite-vs-pgsql comparison.
package mcpparity

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/test/apiparity"
)

type Step struct {
	Label string
	// REST seeding step (Method != ""): replayed via the apiparity harness.
	Method, Path string
	Body         any
	// MCP step (RPC != ""): the JSON-RPC request body posted to /mcp.
	RPC string
	// NoAuth sends the MCP request without a bearer token.
	NoAuth bool
	// CaptureID extracts data.item.id from a REST step's response for
	// fmt.Sprintf-style substitution (%s) into later RPC bodies.
	CaptureID bool
}

type Scenario struct {
	Name  string
	Steps []Step
}

var catalogue []Scenario

func register(s Scenario) { catalogue = append(catalogue, s) }

func Catalogue() []Scenario { return catalogue }

// Run replays a scenario and returns one normalized transcript block per step.
func Run(t *testing.T, h *apiparity.Harness, s Scenario) []string {
	t.Helper()
	var out []string
	var captured string
	for _, st := range s.Steps {
		if st.Method != "" {
			status, body := h.Call(t, st.Method, st.Path, apiparity.OwnerToken, st.Body)
			if st.CaptureID {
				captured = extractItemID(t, body)
			}
			out = append(out, fmt.Sprintf("== %s %s %s -> %d\n%s", st.Label, st.Method, st.Path, status, apiparity.NormalizeGolden(body)))
			continue
		}
		rpcBody := st.RPC
		if strings.Contains(rpcBody, "%s") {
			rpcBody = fmt.Sprintf(rpcBody, captured)
		}
		token := apiparity.OwnerToken
		if st.NoAuth {
			token = ""
		}
		status, body := postMCP(t, h.URL(), token, rpcBody)
		out = append(out, fmt.Sprintf("== %s POST /mcp -> %d\n%s", st.Label, status, apiparity.NormalizeGolden(body)))
	}
	return out
}

func postMCP(t *testing.T, baseURL, token, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, raw
}
```

`extractItemID`: copy the shape of `apiparity.extractItemID` (unexported there — reimplement locally: unmarshal `{"data":{"item":{"id":...}}}`, `t.Fatal` on absence).

- [ ] **Step 2: Catalogue (`catalogue.go`)** — scenarios via `init()`. Cover, at minimum (JSON-RPC `id` fixed integers; REST paths from the routes files):
  1. `lifecycle` — `initialize`, `tools/list`, `resources/list`, `prompts/list`.
  2. `unauthorized` — initialize with `NoAuth: true` (expect the 401 envelope golden).
  3. `resources` — REST-seed one category (`POST /api/v1/category/create-category`), one tag, one payee, one account (copy body shapes from the apiparity catalogue files — e.g. `internal/test/apiparity/*.go` scenario definitions); then `resources/read` for all seven URIs.
  4. `budget` — REST `POST /api/v1/budget/create-budget` (+CaptureID), MCP `get_budget` with `%s` and month `2024-04`, and a `get_budget` with `"month":"junk"` (isError golden).
  5. `transactions` — REST-seed account + category (CaptureID on account); MCP `create_transaction` / `list_transactions` / `delete_transaction` flow; `create_transaction` with a bogus `category_id` UUID (isError golden). NOTE: `create_transaction`'s response contains the new id only inside normalized-away UUIDs — for `delete_transaction`, list first and take the id via a REST `GET /api/v1/transaction/get-transaction-list` CaptureID variant if needed, or simply end the scenario after the error-path (keep the scenario deterministic; the in-memory feature tests already cover update/delete).
  6. `prompts` — `prompts/get log-expense` with a description; `prompts/get budget-review` without arguments.

- [ ] **Step 3: `smoke_test.go`** — mirror `apiparity/smoke_test.go`: sqlite harness, one golden per scenario at `testdata/golden/<name>.golden`, `UPDATE_GOLDEN=1` writes. Also a floor guard: `if len(Catalogue()) < 6 { t.Fatal("mcp catalogue shrank") }`.

- [ ] **Step 4: `enginecompare_test.go`** — `//go:build enginecompare`; run every scenario on `dbtest.NewSQLite` and `dbtest.NewPostgres` harnesses, assert per-step `apiparity.NormalizeParity` equality (copy the loop shape from `internal/test/enginecompare/apiparity_test.go`).

- [ ] **Step 5: Generate goldens** — `UPDATE_GOLDEN=1 go test ./internal/test/mcpparity/`, then **inspect every golden by eye**: no leaked internals in error texts, `isError` where expected, stable ordering. Then `go test ./internal/test/mcpparity/` → PASS, and `make go-test` → PASS.
- [ ] **Step 6: Commit** — `git add -A && git commit -m "test(mcp): golden-file scenario suite + engine comparison"`

---

### Task 15: docs + spec amendment

**Files:**
- Modify: `README.md` (new "MCP" section), `CLAUDE.md` (architecture + testing notes), `docs/superpowers/specs/2026-07-11-mcp-server-design.md` (the two deviations)

- [ ] **Step 1: README section** (place near the API/self-hosting docs): what `/mcp` is, that any bearer token works and a PAT is the right credential (`Settings → Personal access tokens`), stateless Streamable HTTP, plus config snippets:

```jsonc
// Claude Code (.mcp.json) / Claude Desktop / Cursor — remote server with a static header:
{
  "mcpServers": {
    "econumo": {
      "type": "http",
      "url": "https://your-econumo.example.com/mcp",
      "headers": { "Authorization": "Bearer eco_pat_..." }
    }
  }
}
```

  List the 7 resources, 5 tools, 2 prompts in one compact table each. Note: claude.ai web custom connectors need OAuth and are not supported yet.

- [ ] **Step 2: CLAUDE.md** — add to the feature-package tree line for `mcp/` edges; document `/mcp` (stateless Streamable HTTP, PAT auth, outside `/api` so apiparity does not scan it; mcpparity owns its goldens); document `users.timezone` (opportunistic persist via the authenticator decorator; `/mcp` fallback; header always wins) under the timezone/header notes; add `internal/test/mcpparity/` to the Testing section (golden regen command `UPDATE_GOLDEN=1 go test ./internal/test/mcpparity/`).

- [ ] **Step 3: Spec amendment** — in the Error handling section, replace the infrastructure-errors bullet with: infra errors surface as a generic `"Internal error"` **tool error** (typed SDK handlers cannot emit JSON-RPC errors; nothing leaks, details logged at ERROR); in the Logging section, note the operation message is `mcp` with `tool`/`resource`/`prompt` attrs.

- [ ] **Step 4: Verify** `make go-test && make test` (full tier: needs the compose PostgreSQL or `DATABASE_TEST_PGSQL_URL`; this also runs the enginecompare suite incl. mcpparity).
- [ ] **Step 5: Commit** — `git add -A && git commit -m "docs: MCP endpoint documentation + spec amendments"`. Push and mark PR #78 ready for review if all green.
