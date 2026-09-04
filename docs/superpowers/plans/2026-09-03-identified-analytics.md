# Identified Product Analytics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move Twillingate analytics from anonymous to identified — hashed user id, per-instance identity and group, richer attributes — and replace the instance-wide `ECONUMO_ANALYTICS` switch with a per-user preference stored in `users_options`.

**Architecture:** The browser stays the only thing that talks to the collector. Identity values are hashed before they leave (`sha256` in the SPA for the user, in Go for the instance), the instance digest reaches the SPA through the existing `econumo-config.js` merge, and the opt-out is a fifth `users_options` row served by a new `update-analytics` endpoint and mirrored to localStorage so it applies before `get-user-data` resolves.

**Tech Stack:** Go (stdlib `net/http`, sqlc, `modernc.org/sqlite` + `pgx`), React 19 + TypeScript + TanStack Query + vitest, Twillingate MCP for collector config.

**Spec:** `docs/superpowers/specs/2026-09-03-identified-analytics-design.md` — read it first; this plan implements it and does not repeat its reasoning.

## Global Constraints

- **Never adopt the Twillingate JS SDK.** Its `identified` mode persists a visitor id in `localStorage`, which is consent-relevant under ePrivacy and would invalidate the no-banner position. All transport stays hand-written in `web/src/lib/analytics.ts`.
- **Never use `crypto.subtle`.** It is secure-context-only and therefore absent on self-hosted instances served over `http://<lan-ip>`. Same reason `analytics.ts` already uses `uuid.v4` instead of `crypto.randomUUID`.
- **Never send `$user_name`.** `$group_name` is sent and is the `host` value (never a real self-hosted hostname).
- Digest prefixes are exact: `"econumo:analytics:v1:"` for users, `"econumo:instance:v1:"` for instances. User digest is 32 hex chars, instance digest is 12.
- Attribute names are exact: `host`, `deployment`, `mode`, `locale`, `access_state`, `current_url`, `signup_year`, `signup_month`, `connections`, `accounts`, `accounts_hidden`, `categories`, `categories_archived`, `payees`, `payees_archived`, `tags`, `tags_archived`. Reserved keys `$user_id`, `$group_id`, `$group_name`, `$install_id`, `$app_version`, `$platform` are batch-level.
- The custom `version` and `self_hosted` attributes are removed, not kept alongside their replacements.
- Comments follow CLAUDE.md: only non-obvious rationale, never restatements of the code.
- Every user-facing action fires an analytics event (`METRICS` + `trackEvent`); `web/src/lib/metrics-coverage.test.ts` enforces it.
- Go coverage gate is `make go-test` (`GO_COVER_MIN`, default 80).

## File Structure

**Created**
- `web/src/lib/analyticsId.ts` — synchronous SHA-256 and the user-digest helper.
- `web/src/lib/analyticsProfile.ts` — account-profile counts read from the TanStack query cache.
- `web/src/lib/analyticsPreference.ts` — the opt-out gate and its localStorage mirror.
- `internal/infra/instance/instance.go` — instance digest derived from `schema_migrations`.
- `internal/user/api/analytics.go` — the `update-analytics` handler.

**Modified**
- `web/src/lib/analytics.ts` — batch identity attributes, identity reset.
- `web/src/lib/metrics.ts` — host/deployment/platform/version attributes, preference gate.
- `web/src/lib/config.ts` — `getInstanceId()`, remove `analyticsEnabled()` and the `ANALYTICS` key.
- `web/src/lib/appConfig.ts` — `MERGED_KEYS`: drop `ANALYTICS`, add `INSTANCE_ID`.
- `web/src/api/user.ts`, `web/src/features/auth/LogoutPage.tsx` — identity wiring.
- `web/src/features/user/queries.ts`, `web/src/features/settings/ProfilePage.tsx`, `web/src/api/dto/user.ts` — the toggle.
- `internal/model/user.go`, `internal/model/user_dto.go` — the option and the DTOs.
- `internal/user/{usecase,profile,register,repository,migrate}.go`, `internal/user/repo/{repo,sqlite,pgsql}.go`, `internal/user/api/routes.go`.
- `internal/web/middleware/auth.go` — read-only allowlist.
- `internal/web/router/router.go`, `internal/server/server.go` — `INSTANCE_ID` in, `ANALYTICS` out.
- `internal/cli/migration_commands.go`, `internal/infra/storage/migrations/commands.go`.
- `internal/infra/storage/sqlc/query/{sqlite,pgsql}/users_options.sql`.
- `locales/*.json` (11), `.env.example`, `CLAUDE.md`, `docs/regression-test-plan.md`.

**Task order:** T1–T2 are independent. T3→T4→T5 and T6→T7 are backend chains. T8 unblocks the frontend attribute work. T9–T13 are frontend, in order. T14–T15 last.

---

### Task 1: Synchronous SHA-256 and the user digest

**Files:**
- Create: `web/src/lib/analyticsId.ts`
- Test: `web/src/lib/analyticsId.test.ts`

**Interfaces:**
- Consumes: nothing.
- Produces: `sha256Hex(input: string): string` (64 lowercase hex chars), `analyticsUserId(userId: string): string` (32 hex chars).

- [ ] **Step 1: Write the failing test**

```ts
// web/src/lib/analyticsId.test.ts
import { describe, expect, it } from 'vitest'
import { analyticsUserId, sha256Hex } from './analyticsId'

async function webcrypto(input: string): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(input))
  return Array.from(new Uint8Array(digest), (b) => b.toString(16).padStart(2, '0')).join('')
}

describe('sha256Hex', () => {
  it('matches the NIST vectors', () => {
    expect(sha256Hex('')).toBe('e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855')
    expect(sha256Hex('abc')).toBe('ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad')
  })

  // Lengths either side of the 64-byte block and the 55/56-byte padding edge.
  it.each([0, 1, 55, 56, 63, 64, 65, 119, 120, 200])('matches webcrypto at %i bytes', async (n) => {
    const input = 'x'.repeat(n)
    expect(sha256Hex(input)).toBe(await webcrypto(input))
  })

  it('hashes multi-byte characters as UTF-8', async () => {
    expect(sha256Hex('日本語 café')).toBe(await webcrypto('日本語 café'))
  })
})

describe('analyticsUserId', () => {
  it('is the first 32 chars of the domain-separated digest', () => {
    const id = '018f1e5c-0000-7000-8000-000000000001'
    expect(analyticsUserId(id)).toBe(sha256Hex(`econumo:analytics:v1:${id}`).slice(0, 32))
    expect(analyticsUserId(id)).toHaveLength(32)
  })

  it('is stable and distinct per user', () => {
    expect(analyticsUserId('a')).toBe(analyticsUserId('a'))
    expect(analyticsUserId('a')).not.toBe(analyticsUserId('b'))
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm test -- analyticsId`
Expected: FAIL — cannot resolve `./analyticsId`.

- [ ] **Step 3: Write the implementation**

```ts
// web/src/lib/analyticsId.ts
// Hand-rolled SHA-256 rather than crypto.subtle: subtle is secure-context-only,
// so it is missing on a self-hosted instance served over plain http://<lan-ip>,
// where a module-level call would take the whole SPA down.

const K = new Uint32Array([
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
  0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
  0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
  0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
  0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
  0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
  0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
  0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
])

function rotr(x: number, n: number): number {
  return ((x >>> n) | (x << (32 - n))) >>> 0
}

export function sha256Hex(input: string): string {
  const bytes = new TextEncoder().encode(input)
  const padded = new Uint8Array((((bytes.length + 9 + 63) / 64) | 0) * 64)
  padded.set(bytes)
  padded[bytes.length] = 0x80
  const view = new DataView(padded.buffer)
  const bitLength = bytes.length * 8
  view.setUint32(padded.length - 8, Math.floor(bitLength / 0x100000000), false)
  view.setUint32(padded.length - 4, bitLength >>> 0, false)

  const h = new Uint32Array([
    0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19,
  ])
  const w = new Uint32Array(64)

  for (let offset = 0; offset < padded.length; offset += 64) {
    for (let i = 0; i < 16; i++) {
      w[i] = view.getUint32(offset + i * 4, false)
    }
    for (let i = 16; i < 64; i++) {
      const s0 = rotr(w[i - 15], 7) ^ rotr(w[i - 15], 18) ^ (w[i - 15] >>> 3)
      const s1 = rotr(w[i - 2], 17) ^ rotr(w[i - 2], 19) ^ (w[i - 2] >>> 10)
      w[i] = (w[i - 16] + s0 + w[i - 7] + s1) >>> 0
    }
    let a = h[0], b = h[1], c = h[2], d = h[3], e = h[4], f = h[5], g = h[6], hh = h[7]
    for (let i = 0; i < 64; i++) {
      const s1 = rotr(e, 6) ^ rotr(e, 11) ^ rotr(e, 25)
      const ch = (e & f) ^ (~e & g)
      const t1 = (hh + s1 + ch + K[i] + w[i]) >>> 0
      const s0 = rotr(a, 2) ^ rotr(a, 13) ^ rotr(a, 22)
      const maj = (a & b) ^ (a & c) ^ (b & c)
      const t2 = (s0 + maj) >>> 0
      hh = g
      g = f
      f = e
      e = (d + t1) >>> 0
      d = c
      c = b
      b = a
      a = (t1 + t2) >>> 0
    }
    h[0] = (h[0] + a) >>> 0
    h[1] = (h[1] + b) >>> 0
    h[2] = (h[2] + c) >>> 0
    h[3] = (h[3] + d) >>> 0
    h[4] = (h[4] + e) >>> 0
    h[5] = (h[5] + f) >>> 0
    h[6] = (h[6] + g) >>> 0
    h[7] = (h[7] + hh) >>> 0
  }
  return Array.from(h, (x) => x.toString(16).padStart(8, '0')).join('')
}

// Domain-separated and truncated: the analytics store never holds a value that
// is also a primary key in the application database.
export function analyticsUserId(userId: string): string {
  return sha256Hex(`econumo:analytics:v1:${userId}`).slice(0, 32)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm test -- analyticsId`
Expected: PASS, all cases.

- [ ] **Step 5: Lint and commit**

```bash
cd web && pnpm lint
git add web/src/lib/analyticsId.ts web/src/lib/analyticsId.test.ts
git commit -m "feat(web): add synchronous sha256 and the analytics user digest"
```

---

### Task 2: `createdAt` on the current-user payload

**Files:**
- Modify: `internal/model/user_dto.go` (`CurrentUserResult`), `internal/user/usecase.go` (`toCurrentUserWithEmail`), `internal/user/read.go` (`ReadService.currentUser`), `internal/model/user_view.go`, `internal/user/repo/read.go`, `internal/infra/storage/sqlc/query/{sqlite,pgsql}/user_read.sql` (+ `sqlc generate`)
- Test: `internal/user/read_test.go` (add a case; create the file only if absent)

**Both DTO paths must be populated.** `login-user`/`register-user` go through the write service's `toCurrentUserWithEmail`, but `get-user-data` — the endpoint the SPA caches and Task 11 reads — is served by the separate CQRS `ReadService.currentUser`, backed by the `GetUserView` query. Changing only the write path ships `createdAt: ""` on the endpoint that matters.

**Interfaces:**
- Consumes: nothing.
- Produces: `CurrentUserResult.CreatedAt string` with JSON key `createdAt`, formatted `2006-01-02 15:04:05` UTC via `datetime.FormatOrEmpty`.

- [ ] **Step 1: Write the failing test**

```go
// in internal/user/read_test.go
func TestCurrentUserCarriesCreatedAt(t *testing.T) {
	h := newHarness(t) // the package's existing service harness
	u := h.registerUser(t, "created@example.test")

	got, err := h.svc.GetUserData(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetUserData: %v", err)
	}
	if got.User.CreatedAt == "" {
		t.Fatal("createdAt is empty")
	}
	if _, perr := time.Parse("2006-01-02 15:04:05", got.User.CreatedAt); perr != nil {
		t.Fatalf("createdAt %q is not the frozen layout: %v", got.User.CreatedAt, perr)
	}
}
```

If the package's harness helpers are named differently, use whatever `internal/user/*_test.go` already uses to build a service and a user — do not invent a second harness.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/user/ -run TestCurrentUserCarriesCreatedAt`
Expected: FAIL — `got.User.CreatedAt` undefined.

- [ ] **Step 3: Add the field and populate it**

In `internal/model/user_dto.go`, add to `CurrentUserResult` after `Avatar`:

```go
	CreatedAt    string         `json:"createdAt"`
```

In `internal/user/usecase.go`, inside the returned `model.CurrentUserResult{...}` in `toCurrentUserWithEmail`:

```go
		CreatedAt:    datetime.FormatOrEmpty(&u.CreatedAt),
```

`datetime` is already imported in that file. If `FormatOrEmpty` takes a `*time.Time`, pass `&u.CreatedAt`; if the entity field is already a pointer, pass it directly.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/user/ ./internal/model/`
Expected: PASS.

- [ ] **Step 5: Regenerate goldens and docs, then inspect the diff**

```bash
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/
UPDATE_GOLDEN=1 go test ./internal/test/mcpparity/
make swagger
git diff --stat internal/test/apiparity/testdata internal/test/mcpparity/testdata
git diff internal/test/apiparity/testdata | head -40
```

Expected: every golden carrying a user payload gains exactly one `createdAt` line. **If anything else changed, stop and investigate** — a golden diff means observable behavior changed.

- [ ] **Step 6: Commit**

```bash
make go-test
git add -A
git commit -m "feat(user): expose createdAt on the current-user payload"
```

---

### Task 3: The `analytics` user option in the model

**Files:**
- Modify: `internal/model/user.go`
- Test: `internal/model/user_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `model.OptionAnalytics = "analytics"`; `func (u *User) AnalyticsEnabled() bool`; `func (u *User) SetAnalytics(enabled bool, id vo.Id, now time.Time)`.

- [ ] **Step 1: Write the failing test**

```go
// in internal/model/user_test.go
func TestAnalyticsOption(t *testing.T) {
	now := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	u := &model.User{}

	if !u.AnalyticsEnabled() {
		t.Fatal("absent option must read as enabled")
	}

	// Missing row: SetAnalytics appends one, so users predating the option work.
	u.SetAnalytics(false, vo.NewId(), now)
	if len(u.Options) != 1 || u.Options[0].Name != model.OptionAnalytics {
		t.Fatalf("options = %+v, want one analytics row", u.Options)
	}
	if got := *u.Options[0].Value; got != "0" {
		t.Fatalf("value = %q, want \"0\"", got)
	}
	if u.AnalyticsEnabled() {
		t.Fatal("AnalyticsEnabled = true after opting out")
	}

	// Existing row: updated in place, no second row, no new id.
	id := u.Options[0].ID
	later := now.Add(time.Hour)
	u.SetAnalytics(true, vo.NewId(), later)
	if len(u.Options) != 1 {
		t.Fatalf("options = %d, want 1", len(u.Options))
	}
	if u.Options[0].ID != id {
		t.Fatal("existing option id was replaced")
	}
	if !u.AnalyticsEnabled() {
		t.Fatal("AnalyticsEnabled = false after opting in")
	}
	if !u.Options[0].UpdatedAt.Equal(later) {
		t.Fatalf("UpdatedAt = %v, want %v", u.Options[0].UpdatedAt, later)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run TestAnalyticsOption`
Expected: FAIL — `OptionAnalytics` undefined.

- [ ] **Step 3: Implement**

In the option-name const block in `internal/model/user.go`:

```go
	OptionAnalytics    = "analytics"
```

Do **not** add it to `persistedOptions`: registration seeds it explicitly (Task 5) through the same setter used here, so one code path creates the row.

Then, next to `ReportPeriod()` and `UpdateReportPeriod`:

```go
// AnalyticsEnabled reports the user's product-analytics preference. Absent or
// empty reads as enabled: rows predating the option, and any user the backfill
// migration has not reached, default to the shipped behaviour.
func (u *User) AnalyticsEnabled() bool {
	o := u.Option(OptionAnalytics)
	if o == nil || o.Value == nil || *o.Value == "" {
		return true
	}
	return *o.Value != "0"
}

// SetAnalytics writes the preference, appending the option row when the user
// has none — unlike the other option setters, which no-op on a missing row.
// id is consumed only in that case.
func (u *User) SetAnalytics(enabled bool, id vo.Id, now time.Time) {
	v := "0"
	if enabled {
		v = "1"
	}
	if o := u.Option(OptionAnalytics); o != nil {
		o.setValue(&v, now)
		return
	}
	u.Options = append(u.Options, NewUserOption(id, OptionAnalytics, &v, now))
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/model/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/model/user.go internal/model/user_test.go
git commit -m "feat(model): add the analytics user option"
```

---

### Task 4: `update-analytics` endpoint

**Files:**
- Create: `internal/user/api/analytics.go`
- Modify: `internal/model/user_dto.go`, `internal/user/profile.go`, `internal/user/api/routes.go`, `internal/web/middleware/auth.go`, `internal/test/apiparity/catalogue.go`
- Test: `internal/user/api/user_endpoints_test.go`, `internal/web/middleware/middleware_test.go`

**Interfaces:**
- Consumes: `model.OptionAnalytics`, `User.SetAnalytics`, `User.AnalyticsEnabled` (Task 3); `Service.mutate`, `Service.toCurrentUser`, `Repository.NextIdentity`.
- Produces: `POST /api/v1/user/update-analytics`; `model.UpdateAnalyticsRequest{Enabled *bool}`; `model.UpdateAnalyticsResult{User CurrentUserResult}`; `(*Service).UpdateAnalytics(ctx, userID vo.Id, req model.UpdateAnalyticsRequest) (*model.UpdateAnalyticsResult, error)`; `(*Handlers).UpdateAnalytics`.

- [ ] **Step 1: Write the failing tests**

```go
// in internal/user/api/user_endpoints_test.go
func TestUpdateAnalytics(t *testing.T) {
	h := newAPIHarness(t) // whatever the file already uses
	rec := h.post(t, "/api/v1/user/update-analytics", `{"enabled":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `{"name":"analytics","value":"0"}`) {
		t.Fatalf("analytics option missing from response: %s", rec.Body.String())
	}

	rec = h.post(t, "/api/v1/user/update-analytics", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing enabled: status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "This value should not be blank.") {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}
```

```go
// in internal/web/middleware/middleware_test.go, next to the billing-link case
// A restricted user must still be able to withdraw from analytics: an objection
// that only works while you are paying is not an objection.
func TestReadonlyMayUpdateAnalytics(t *testing.T) {
	rec, ran := authRequest(t, http.MethodPost, "/api/v1/user/update-analytics", readonlyStub())
	if !ran || rec.Code == http.StatusPaymentRequired {
		t.Fatalf("status = %d, ran = %v, want the handler to run", rec.Code, ran)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/user/api/ ./internal/web/middleware/ -run 'Analytics'`
Expected: FAIL — 404 from the router, and `UpdateAnalytics` undefined.

- [ ] **Step 3: Add the DTOs**

In `internal/model/user_dto.go`, after the `update-language` block:

```go
// ---------------------------------------------------------------------------
// update-analytics
// ---------------------------------------------------------------------------

// UpdateAnalyticsRequest is the update-analytics request body. Enabled is a
// pointer so an absent field is a validation error rather than a silent false.
type UpdateAnalyticsRequest struct {
	Enabled *bool `json:"enabled"`
}

func (r UpdateAnalyticsRequest) Validate() error {
	if r.Enabled == nil {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "enabled", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return nil
}

// UpdateAnalyticsResult is the update-analytics response.
type UpdateAnalyticsResult struct {
	User CurrentUserResult `json:"user"`
}
```

- [ ] **Step 4: Add the use case**

In `internal/user/profile.go`, after `UpdateReportPeriod`:

```go
// UpdateAnalytics writes the caller's product-analytics preference and returns
// the refreshed current user.
func (s *Service) UpdateAnalytics(ctx context.Context, userID vo.Id, req model.UpdateAnalyticsRequest) (*model.UpdateAnalyticsResult, error) {
	u, err := s.mutate(ctx, userID, func(u *model.User, now time.Time) error {
		u.SetAnalytics(*req.Enabled, s.repo.NextIdentity(), now)
		return nil
	})
	if err != nil {
		return nil, err
	}
	cur, err := s.toCurrentUser(ctx, u)
	if err != nil {
		return nil, err
	}
	return &model.UpdateAnalyticsResult{User: cur}, nil
}
```

- [ ] **Step 5: Add the handler and route**

```go
// internal/user/api/analytics.go
package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

var _ = model.UpdateAnalyticsResult{}

// UpdateAnalytics handles POST /api/v1/user/update-analytics (auth).
//
// @Summary     Update analytics preference
// @Description Enables or disables product analytics for the authenticated user and returns the refreshed user.
// @Tags        User
// @Accept      json
// @Produce     json
// @Param       request body     model.UpdateAnalyticsRequest true "Update analytics request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UpdateAnalyticsResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/user/update-analytics [post]
func (h *Handlers) UpdateAnalytics(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.UpdateAnalytics)
}
```

Note there is no `@Failure 402` line: the route is read-only-allowed.

In `internal/user/api/routes.go`, beside `update-language`:

```go
		mux.Handle("POST /api/v1/user/update-analytics", auth(h.UpdateAnalytics))
```

In `internal/web/middleware/auth.go`, add to `ReadonlyAllowedPaths`:

```go
	"/api/v1/user/update-analytics":         true,
```

and extend that block's doc comment with one sentence: withdrawing from analytics is a privacy right, not a paid feature.

- [ ] **Step 6: Add the parity scenario**

In `internal/test/apiparity/catalogue.go`, beside the `update-language` entry:

```go
			{Label: "update-analytics", Method: "POST", Path: "/api/v1/user/update-analytics", Auth: "owner", Body: map[string]any{"enabled": false}},
```

- [ ] **Step 7: Run tests, regenerate goldens, inspect**

```bash
go test ./internal/user/... ./internal/web/middleware/
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/
make swagger
git diff internal/test/apiparity/testdata | head -60
```

Expected: one new golden for `update-analytics`; the scenario's user payload carries `{"name":"analytics","value":"0"}`. No other golden changes.

- [ ] **Step 8: Commit**

```bash
make go-test
git add -A
git commit -m "feat(user): add the update-analytics endpoint"
```

---

### Task 5: Registration seeds the option from config

**Files:**
- Modify: `internal/user/usecase.go` (`Service` field + `NewService`), `internal/user/register.go`, `internal/cli/container.go`, `internal/server/server.go`
- Test: `internal/user/register_test.go`

**Interfaces:**
- Consumes: `User.SetAnalytics` (Task 3).
- Produces: `NewService(..., analyticsDefault bool)` — a new trailing parameter after `emailVerification`; every construction site must pass `cfg.Analytics`.

- [ ] **Step 1: Write the failing test**

```go
// in internal/user/register_test.go
func TestRegisterSeedsAnalyticsOptionFromConfig(t *testing.T) {
	for _, tc := range []struct{ name string; def bool; want string }{
		{"default on", true, "1"},
		{"default off", false, "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarnessWithAnalyticsDefault(t, tc.def) // extend the file's existing harness
			u := h.registerUser(t, "seed@example.test")

			o := u.Option(model.OptionAnalytics)
			if o == nil || o.Value == nil {
				t.Fatal("analytics option not seeded")
			}
			if *o.Value != tc.want {
				t.Fatalf("value = %q, want %q", *o.Value, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/user/ -run TestRegisterSeedsAnalyticsOptionFromConfig`
Expected: FAIL — option not seeded.

- [ ] **Step 3: Thread the default through the service**

In `internal/user/usecase.go`, add to the `Service` struct beside `emailVerification`:

```go
	analyticsDefault    bool
```

Add the matching trailing parameter to `NewService` and assign it. In `internal/user/register.go`, immediately after the existing `SeedDefaultOptions` call:

```go
	u.SetAnalytics(s.analyticsDefault, s.repo.NextIdentity(), now)
```

Use whatever the surrounding code names the user variable and the clock value; do not introduce a second `now`.

- [ ] **Step 4: Update both construction sites**

`internal/cli/container.go` around line 101 and the `appuser.NewService(` call in `internal/server/server.go`: append `cfg.Analytics` as the final argument. Compile errors point at any site missed.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/user/... ./internal/cli/ ./internal/server/`
Expected: PASS.

- [ ] **Step 6: Regenerate goldens and inspect**

```bash
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/
UPDATE_GOLDEN=1 go test ./internal/test/mcpparity/
git diff internal/test/apiparity/testdata | head -40
```

Expected: fixtures registered through the service now carry `{"name":"analytics","value":"1"}` in their options array. Nothing else.

- [ ] **Step 7: Commit**

```bash
make go-test
git add -A
git commit -m "feat(user): seed the analytics option at registration"
```

---

### Task 6: Repository query for users missing an option

**Files:**
- Modify: `internal/infra/storage/sqlc/query/sqlite/users_options.sql`, `internal/infra/storage/sqlc/query/pgsql/users_options.sql`, `internal/user/repo/{repo,sqlite,pgsql}.go`, `internal/user/repository.go`
- Test: `internal/user/repo/repo_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `Repository.ListUserIDsMissingOption(ctx context.Context, name string) ([]vo.Id, error)` and its `*Repo` implementation.

- [ ] **Step 1: Write the failing test**

```go
// in internal/user/repo/repo_test.go
func TestListUserIDsMissingOption(t *testing.T) {
	db := dbtest.New(t)
	r := repo.NewRepo(db.Driver, db.TX)
	ctx := context.Background()

	with := fixture.User(t, db)    // whatever the package's fixture helpers are
	without := fixture.User(t, db)

	u, err := r.GetByID(ctx, with.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	u.SetAnalytics(true, r.NextIdentity(), time.Now().UTC())
	if err := r.Save(ctx, u); err != nil {
		t.Fatalf("Save: %v", err)
	}

	ids, err := r.ListUserIDsMissingOption(ctx, model.OptionAnalytics)
	if err != nil {
		t.Fatalf("ListUserIDsMissingOption: %v", err)
	}
	if len(ids) != 1 || ids[0] != without.ID {
		t.Fatalf("ids = %v, want [%v]", ids, without.ID)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/user/repo/ -run TestListUserIDsMissingOption`
Expected: FAIL — method undefined.

- [ ] **Step 3: Add the SQL**

Append to `internal/infra/storage/sqlc/query/sqlite/users_options.sql`:

```sql
-- name: ListUserIDsMissingOption :many
-- Ordered by id so the backfill is deterministic across engines and reruns.
SELECT u.id
FROM users u
WHERE NOT EXISTS (
    SELECT 1 FROM users_options o WHERE o.user_id = u.id AND o.name = ?
)
ORDER BY u.id;
```

And to the pgsql file, identical but with `$1`:

```sql
-- name: ListUserIDsMissingOption :many
-- Ordered by id so the backfill is deterministic across engines and reruns.
SELECT u.id
FROM users u
WHERE NOT EXISTS (
    SELECT 1 FROM users_options o WHERE o.user_id = u.id AND o.name = $1
)
ORDER BY u.id;
```

Do **not** put a bare `;` on its own line — it truncates the generated SQL.

- [ ] **Step 4: Regenerate sqlc**

```bash
sqlc generate -f internal/infra/storage/sqlc/sqlc.yaml
git status --short internal/infra/storage/sqlc/gen
```

Expected: both `gen/sqlite` and `gen/pgsql` gain `ListUserIDsMissingOption`.

- [ ] **Step 5: Wire the querier, both adapters, and the repo**

`internal/user/repo/repo.go`, in the `querier` interface beside `ListUserIDs`:

```go
	ListUserIDsMissingOption(ctx context.Context, db backend.DBTX, name string) ([]string, error)
```

`internal/user/repo/sqlite.go`:

```go
func (sqliteQuerier) ListUserIDsMissingOption(ctx context.Context, db backend.DBTX, name string) ([]string, error) {
	return sqlitegen.New(db).ListUserIDsMissingOption(ctx, name)
}
```

`internal/user/repo/pgsql.go`:

```go
func (pgsqlQuerier) ListUserIDsMissingOption(ctx context.Context, db backend.DBTX, name string) ([]string, error) {
	return pgsqlgen.New(db).ListUserIDsMissingOption(ctx, name)
}
```

The method on `*Repo`, beside the other list methods:

```go
func (r *Repo) ListUserIDsMissingOption(ctx context.Context, name string) ([]vo.Id, error) {
	rows, err := r.q.ListUserIDsMissingOption(ctx, r.db(ctx), name)
	if err != nil {
		return nil, err
	}
	ids := make([]vo.Id, 0, len(rows))
	for _, raw := range rows {
		id, perr := vo.ParseId(raw)
		if perr != nil {
			return nil, perr
		}
		ids = append(ids, id)
	}
	return ids, nil
}
```

Use whatever id parser the file already uses for the same job (`vo.ParseId`, `vo.IdFromString`, …) — match the neighbours rather than introducing a new one.

Finally add the method to the `Repository` interface in `internal/user/repository.go`, with a one-line comment saying it exists for the analytics backfill.

- [ ] **Step 6: Run tests on both engines**

```bash
go test ./internal/user/...
DBTEST_ENGINE=pgsql go test -tags enginecompare ./internal/user/repo/ -run TestListUserIDsMissingOption
```

Expected: PASS on both. Skip the second command if no PostgreSQL is available and say so in the handoff.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat(user): query users missing a given option"
```

---

### Task 7: The backfill migration command

**Files:**
- Create: nothing
- Modify: `internal/user/migrate.go`, `internal/cli/migration_commands.go`, `internal/infra/storage/migrations/commands.go`
- Test: `internal/user/migrate_test.go`, `internal/cli/commands_test.go`

**Interfaces:**
- Consumes: `Repository.ListUserIDsMissingOption` (Task 6), `User.SetAnalytics` (Task 3), `Service.analyticsDefault` (Task 5).
- Produces: `(*Service).SeedAnalyticsOption(ctx context.Context) (int, error)` returning the number of rows written; CLI command `migration:seed-analytics-option`; migration step `20260903000000`.

- [ ] **Step 1: Write the failing test**

```go
// in internal/user/migrate_test.go
func TestSeedAnalyticsOption(t *testing.T) {
	h := newHarnessWithAnalyticsDefault(t, false)
	a := h.insertUserWithoutOptions(t, "a@example.test") // raw repo insert, no analytics row
	b := h.insertUserWithoutOptions(t, "b@example.test")

	n, err := h.svc.SeedAnalyticsOption(context.Background())
	if err != nil {
		t.Fatalf("SeedAnalyticsOption: %v", err)
	}
	if n != 2 {
		t.Fatalf("seeded = %d, want 2", n)
	}
	for _, id := range []vo.Id{a.ID, b.ID} {
		u, gerr := h.repo.GetByID(context.Background(), id)
		if gerr != nil {
			t.Fatalf("GetByID: %v", gerr)
		}
		if u.AnalyticsEnabled() {
			t.Fatalf("user %s: analytics enabled, want the config default (off)", id)
		}
	}

	// Idempotent: a rerun writes nothing and leaves values alone.
	again, err := h.svc.SeedAnalyticsOption(context.Background())
	if err != nil {
		t.Fatalf("rerun: %v", err)
	}
	if again != 0 {
		t.Fatalf("rerun seeded = %d, want 0", again)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/user/ -run TestSeedAnalyticsOption`
Expected: FAIL — `SeedAnalyticsOption` undefined.

- [ ] **Step 3: Implement the use case**

In `internal/user/migrate.go`, beside `MigrateRemoveDataSalt`:

```go
// SeedAnalyticsOption writes the analytics preference row for every user that
// has none, using the deprecated ECONUMO_ANALYTICS value as the seed so an
// operator who disabled analytics before the per-user preference existed does
// not silently start sending events again. Idempotent: users that already hold
// the option are skipped, so a rerun is a no-op.
func (s *Service) SeedAnalyticsOption(ctx context.Context) (int, error) {
	var seeded int
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		ids, err := s.repo.ListUserIDsMissingOption(ctx, model.OptionAnalytics)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		for _, id := range ids {
			u, gerr := s.repo.GetByID(ctx, id)
			if gerr != nil {
				return gerr
			}
			u.SetAnalytics(s.analyticsDefault, s.repo.NextIdentity(), now)
			if serr := s.repo.Save(ctx, u); serr != nil {
				return serr
			}
			seeded++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return seeded, nil
}
```

- [ ] **Step 4: Register the CLI command and the migration step**

In `internal/cli/migration_commands.go`, add to the returned slice:

```go
		{
			name:    "migration:seed-analytics-option",
			summary: "write the per-user analytics preference for users that have none, seeded from ECONUMO_ANALYTICS (idempotent)",
			run: func(ctx context.Context, c *container, args []string) error {
				n, err := c.user.SeedAnalyticsOption(ctx)
				if err != nil {
					return err
				}
				fmt.Printf("seeded %d analytics preference(s)\n", n)
				return nil
			},
		},
```

In `internal/infra/storage/migrations/commands.go`, inside `init()`:

```go
	// No SQL file: the value comes from the deprecated ECONUMO_ANALYTICS, which
	// SQL cannot read, and each row needs a UUIDv7 id SQLite cannot mint.
	RegisterCommand("20260903000000", "migration:seed-analytics-option")
```

- [ ] **Step 5: Add the CLI test**

Mirror the existing `migration:zero-deleted-accounts` cases in `internal/cli/commands_test.go`: run the command twice through `Run([]string{"migration:seed-analytics-option"})` asserting exit code 0 both times, and once through the `MigrationCommandRunner` runner.

- [ ] **Step 6: Run tests**

```bash
go test ./internal/user/ ./internal/cli/ ./internal/infra/storage/...
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
make go-test
git add -A
git commit -m "feat(cli): backfill the analytics preference from ECONUMO_ANALYTICS"
```

---

### Task 8: Instance digest and `INSTANCE_ID` in the served config

**Files:**
- Create: `internal/infra/instance/instance.go`, `internal/infra/instance/instance_test.go`
- Modify: `internal/web/router/router.go`, `internal/server/server.go`
- Test: `internal/web/router/router_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `instance.ID(ctx context.Context, db *sql.DB) (string, error)` — 12 lowercase hex chars, `""` when `schema_migrations` is empty; `router.Deps.InstanceID string`; config key `INSTANCE_ID`. Removes config key `ANALYTICS`.

- [ ] **Step 1: Write the failing test**

```go
// internal/infra/instance/instance_test.go
package instance_test

func TestIDIsStableAndDerivedFromTheEarliestMigration(t *testing.T) {
	db := dbtest.New(t) // migrations already applied

	got, err := instance.ID(context.Background(), db.DB)
	if err != nil {
		t.Fatalf("ID: %v", err)
	}
	if len(got) != 12 {
		t.Fatalf("id = %q, want 12 hex chars", got)
	}
	if !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(got) {
		t.Fatalf("id = %q, want lowercase hex", got)
	}

	again, err := instance.ID(context.Background(), db.DB)
	if err != nil {
		t.Fatalf("ID rerun: %v", err)
	}
	if again != got {
		t.Fatalf("id changed between calls: %q then %q", got, again)
	}
}

func TestIDIsEmptyWithoutMigrations(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version VARCHAR(191) NOT NULL PRIMARY KEY, applied_at TIMESTAMP NOT NULL)`); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := instance.ID(context.Background(), db)
	if err != nil {
		t.Fatalf("ID: %v", err)
	}
	if got != "" {
		t.Fatalf("id = %q, want empty", got)
	}
}
```

Use the dbtest accessor the repo already exposes for the raw `*sql.DB`; if `dbtest.New(t)` returns a wrapper, take its DB field rather than opening a second connection.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/infra/instance/`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement**

```go
// internal/infra/instance/instance.go

// Package instance derives a stable, per-deployment identifier for product
// analytics. It anchors on the earliest schema_migrations row, which every
// deployment stamps on its first boot of the Go binary and never rewrites —
// so the value exists before the first user, needs no storage of its own, and
// reveals nothing about the instance's hostname.
package instance

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/econumo/econumo/internal/shared/datetime"
)

// ID returns the 12-hex-character instance digest, or "" when the bookkeeping
// table holds no rows yet (a database created but never migrated).
func ID(ctx context.Context, db *sql.DB) (string, error) {
	var version string
	var appliedAt any
	row := db.QueryRowContext(ctx,
		`SELECT version, applied_at FROM schema_migrations ORDER BY applied_at ASC, version ASC LIMIT 1`)
	if err := row.Scan(&version, &appliedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("instance: read schema_migrations: %w", err)
	}
	stamp, err := normalize(appliedAt)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte("econumo:instance:v1:" + version + "|" + stamp))
	return hex.EncodeToString(sum[:])[:12], nil
}

// normalize collapses the two drivers' timestamp representations to the frozen
// layout, so the same instance hashes identically whichever engine it runs on
// and whichever way its driver round-trips a TIMESTAMP.
func normalize(v any) (string, error) {
	switch t := v.(type) {
	case time.Time:
		return t.UTC().Format(datetime.Layout), nil
	case []byte:
		return normalizeString(string(t))
	case string:
		return normalizeString(t)
	default:
		return "", fmt.Errorf("instance: unsupported applied_at type %T", v)
	}
}

func normalizeString(s string) (string, error) {
	for _, layout := range []string{datetime.Layout, time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999999 -0700 MST"} {
		if parsed, err := time.Parse(layout, s); err == nil {
			return parsed.UTC().Format(datetime.Layout), nil
		}
	}
	return "", fmt.Errorf("instance: unparseable applied_at %q", s)
}
```

If the shared datetime package names its layout constant something other than `datetime.Layout`, use that name — grep `internal/shared/datetime` first.

- [ ] **Step 4: Wire it into the served config**

In `internal/web/router/router.go`, delete the `"ANALYTICS": deps.Cfg.Analytics,` line from `overrides`, and after the `VERSION` block add:

```go
	// Identifies the deployment in product analytics; empty on a database that
	// has not been migrated, in which case the SPA sends no instance.
	if deps.InstanceID != "" {
		overrides["INSTANCE_ID"] = deps.InstanceID
	}
```

Add the field to `router.Deps` with a comment saying where it comes from. In `internal/server/server.go`, resolve it next to `spaVersion` and pass it into `router.Deps`:

```go
	// Failure here must not stop the server: analytics are not load-bearing.
	instanceID, err := instance.ID(context.Background(), db)
	if err != nil {
		slog.Warn("instance id unavailable", "err", err)
		instanceID = ""
	}
```

Match the file's existing logging style; if `Build` already has a context in scope, use it instead of `context.Background()`.

- [ ] **Step 5: Update the router test**

In `internal/web/router/router_test.go`, replace any assertion expecting `ANALYTICS` in the served config with one asserting it is **absent**, and add one asserting `INSTANCE_ID` appears when `Deps.InstanceID` is set. `internal/web/spa/spa_test.go` passes its own override map and needs no change.

- [ ] **Step 6: Run tests**

```bash
go test ./internal/infra/instance/ ./internal/web/... ./internal/server/
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
make go-test
git add -A
git commit -m "feat(server): derive an instance id and serve it as INSTANCE_ID"
```

---

### Task 9: Batch identity in the transport

**Files:**
- Modify: `web/src/lib/analytics.ts`
- Test: `web/src/lib/analytics.test.ts`

**Interfaces:**
- Consumes: nothing (callers pass already-hashed values).
- Produces: `setAnalyticsUser(userId: string | null): void`, `setAnalyticsGroup(id: string, name: string): void`, `resetAnalyticsIdentity(): void`. The batch envelope's `attributes` gains `$user_id`, `$group_id`, `$group_name` when set.

- [ ] **Step 1: Write the failing test**

```ts
// in web/src/lib/analytics.test.ts
it('carries identity on the batch and drops it on reset', async () => {
  const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('', { status: 202 }))
  setAnalyticsUser('a'.repeat(32))
  setAnalyticsGroup('a3f19c02b7d4', 'selfhosted_a3f19c02b7d4')

  capture('test_event')
  await flushForTest() // use whatever the file already uses to force a flush

  const first = JSON.parse(fetchMock.mock.calls[0][1]!.body as string)
  expect(first.attributes.$user_id).toBe('a'.repeat(32))
  expect(first.attributes.$group_id).toBe('a3f19c02b7d4')
  expect(first.attributes.$group_name).toBe('selfhosted_a3f19c02b7d4')
  expect(first.attributes.$user_name).toBeUndefined()
  const installId = first.attributes.$install_id
  expect(installId).toBeTruthy()

  resetAnalyticsIdentity()
  capture('after_logout')
  await flushForTest()

  const second = JSON.parse(fetchMock.mock.calls[1][1]!.body as string)
  expect(second.attributes.$user_id).toBeUndefined()
  // A fresh install id so the next person on a shared browser is not linked.
  expect(second.attributes.$install_id).not.toBe(installId)
  // The group is the deployment, not the person, so it survives logout.
  expect(second.attributes.$group_id).toBe('a3f19c02b7d4')
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm test -- analytics.test`
Expected: FAIL — `setAnalyticsUser` is not exported.

- [ ] **Step 3: Implement**

In `web/src/lib/analytics.ts`, change `const installId = uuidv4()` to `let installId = uuidv4()` and add above `takeBatch`:

```ts
let userId: string | null = null
let groupId: string | null = null
let groupName: string | null = null

export function setAnalyticsUser(id: string | null): void {
  userId = id
}

export function setAnalyticsGroup(id: string, name: string): void {
  groupId = id
  groupName = name
}

// Called on logout. The install id is re-minted alongside so the next person on
// a shared browser inherits nothing; the group is the deployment, not the
// person, so it stays.
export function resetAnalyticsIdentity(): void {
  userId = null
  installId = uuidv4()
}
```

In `takeBatch`, replace the fixed attributes object with:

```ts
  const body = JSON.stringify({
    key: INGEST_KEY,
    attributes: {
      $install_id: installId,
      ...(userId ? { $user_id: userId } : {}),
      ...(groupId ? { $group_id: groupId, $group_name: groupName } : {}),
    },
    events: queue,
  })
```

Rewrite the file's header comment: it currently promises anonymity by construction. State instead that the transport sends a hashed user id and a per-instance group, writes nothing to the device, and never sends `$user_name`.

- [ ] **Step 4: Run tests**

Run: `cd web && pnpm test -- analytics.test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd web && pnpm lint
git add web/src/lib/analytics.ts web/src/lib/analytics.test.ts
git commit -m "feat(web): send hashed user and instance group on the analytics batch"
```

---

### Task 10: Host, deployment, platform and version attributes

**Files:**
- Modify: `web/src/lib/metrics.ts`, `web/src/lib/config.ts`, `web/src/lib/appConfig.ts`
- Test: `web/src/lib/metrics.test.ts`, `web/src/lib/config.test.ts`

**Interfaces:**
- Consumes: `INSTANCE_ID` from the served config (Task 8).
- Produces: `getInstanceId(): string` (config.ts); `isCloudHost(hostname: string): boolean`, `analyticsHost(hostname?: string): string`, `deploymentKind(hostname?: string): 'cloud' | 'self-hosted'`, `analyticsPlatform(): 'web' | 'ios' | 'android'` (metrics.ts); `setAnalyticsContext(attrs: Record<string, unknown>): void` (analytics.ts, spread into the batch attributes). Removes `analyticsDomain` and `analyticsEnabled`.

- [ ] **Step 1: Write the failing test**

```ts
// in web/src/lib/metrics.test.ts
describe('host attributes', () => {
  it('keeps cloud hostnames verbatim', () => {
    expect(analyticsHost('econumo.com')).toBe('econumo.com')
    expect(analyticsHost('app.econumo.com')).toBe('app.econumo.com')
    expect(deploymentKind('app.econumo.com')).toBe('cloud')
  })

  it('replaces every other hostname with the instance id', () => {
    window.econumoConfig = { ...window.econumoConfig, INSTANCE_ID: 'a3f19c02b7d4' }
    expect(analyticsHost('money.example.com')).toBe('selfhosted_a3f19c02b7d4')
    expect(analyticsHost('192.168.1.20')).toBe('selfhosted_a3f19c02b7d4')
    expect(deploymentKind('money.example.com')).toBe('self-hosted')
  })

  it('falls back to a bare marker when the server sent no instance id', () => {
    window.econumoConfig = { ...window.econumoConfig, INSTANCE_ID: '' }
    expect(analyticsHost('money.example.com')).toBe('selfhosted_unknown')
  })

  // A lookalike domain must not be treated as cloud.
  it('does not match a suffix impostor', () => {
    expect(isCloudHost('notdeconumo.com')).toBe(false)
    expect(isCloudHost('econumo.com.evil.test')).toBe(false)
  })
})
```

Add to the `trackEvent` test in the same file assertions that the captured attributes contain `deployment`, `$app_version`, `$platform`, and no `version` or `self_hosted` keys.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm test -- metrics.test`
Expected: FAIL — `analyticsHost` is not exported.

- [ ] **Step 3: Add `getInstanceId` and delete `analyticsEnabled`**

In `web/src/lib/config.ts`: add `INSTANCE_ID?: string` to `EconumoConfig`, remove `ANALYTICS`, delete `analyticsEnabled()` entirely, and add:

```ts
export function getInstanceId(): string {
  return window.econumoConfig?.INSTANCE_ID || ''
}
```

In `web/src/lib/appConfig.ts` change the allowlist to:

```ts
const MERGED_KEYS = ['ALLOW_REGISTRATION', 'INSTANCE_ID'] as const
```

Update its comment: the instance id is server truth, so an app pointed at a self-hosted backend reports that backend's instance rather than none.

- [ ] **Step 4: Rework the attributes in `metrics.ts`**

Replace `analyticsDomain` (delete it from `analytics.ts` too, along with its test) with:

```ts
export function isCloudHost(hostname: string = window.location.hostname): boolean {
  return hostname === 'econumo.com' || hostname.endsWith('.econumo.com')
}

// Self-hosted hostnames never leave the browser: the deployment is identified
// by the server-derived instance digest instead.
export function analyticsHost(hostname: string = window.location.hostname): string {
  if (isCloudHost(hostname)) {
    return hostname
  }
  return `selfhosted_${getInstanceId() || 'unknown'}`
}

export function deploymentKind(hostname: string = window.location.hostname): 'cloud' | 'self-hosted' {
  return isCloudHost(hostname) ? 'cloud' : 'self-hosted'
}

export function analyticsPlatform(): 'web' | 'ios' | 'android' {
  if (!isNativeApp()) {
    return 'web'
  }
  return /android/i.test(navigator.userAgent) ? 'android' : 'ios'
}
```

Import `isNativeApp` from `./platform` and `getInstanceId` from `./config`. Then in `trackEvent`, replace the capture block's attributes:

```ts
    const host = analyticsHost()
    capture(analyticsEventName(metric), {
      host,
      deployment: deploymentKind(),
      locale: locale(),
      mode: viewMode(),
      current_url: `https://${host}/${scrubbedPage(window.location.pathname)}`,
      ...(currentAccessState ? { access_state: currentAccessState } : {}),
    })
```

`$app_version` and `$platform` are batch-level, so set them once at module load in `analytics.ts` instead of per event — add to Task 9's batch attributes object, sourced from a new exported setter called once from `metrics.ts`:

```ts
// analytics.ts
let batchContext: Record<string, unknown> = {}

export function setAnalyticsContext(attrs: Record<string, unknown>): void {
  batchContext = attrs
}
```

and spread `...batchContext` into the batch `attributes`. In `metrics.ts`, call it at module scope:

```ts
setAnalyticsContext({ $app_version: getVersion(), $platform: analyticsPlatform() })
```

- [ ] **Step 5: Run tests**

Run: `cd web && pnpm test -- metrics.test config.test analytics.test`
Expected: PASS. Fix any other test that referenced `analyticsEnabled` or `analyticsDomain`.

- [ ] **Step 6: Commit**

```bash
cd web && pnpm lint && pnpm test
git add -A
git commit -m "feat(web): report instance host, deployment, platform and app version"
```

---

### Task 11: Account profile counts and signup cohort

**Files:**
- Create: `web/src/lib/analyticsProfile.ts`, `web/src/lib/analyticsProfile.test.ts`
- Modify: `web/src/lib/metrics.ts`, `web/src/app/` provider that owns the QueryClient
- Test: as above

**Interfaces:**
- Consumes: `queryKeys` from `@/app/queryKeys`; DTOs `AccountDto`, `FolderDto`, `CategoryDto`, `PayeeDto`, `TagDto`, `CurrentUserDto` (with `createdAt` from Task 2).
- Produces: `setAnalyticsQueryClient(client: QueryClient | null): void`, `profileAttributes(): Record<string, number>`.

- [ ] **Step 1: Write the failing test**

```ts
// web/src/lib/analyticsProfile.test.ts
import { QueryClient } from '@tanstack/react-query'
import { queryKeys } from '@/app/queryKeys'
import { profileAttributes, setAnalyticsQueryClient } from './analyticsProfile'

it('counts what the cache holds and omits what it does not', () => {
  const qc = new QueryClient()
  qc.setQueryData(queryKeys.folders, [
    { id: 'f1', name: 'Visible', position: 0, isVisible: 1 },
    { id: 'f2', name: 'Hidden', position: 1, isVisible: 0 },
  ])
  qc.setQueryData(queryKeys.accounts, [
    { item: { id: 'a1', folderId: 'f1' } },
    { item: { id: 'a2', folderId: 'f2' } },
    { item: { id: 'a3', folderId: null } },
  ])
  qc.setQueryData(queryKeys.categories, [
    { id: 'c1', isArchived: 0 },
    { id: 'c2', isArchived: 1 },
  ])
  qc.setQueryData(queryKeys.user, { id: 'u1', createdAt: '2026-02-17 08:30:00' })
  setAnalyticsQueryClient(qc)

  const attrs = profileAttributes()
  expect(attrs.accounts).toBe(3)
  expect(attrs.accounts_hidden).toBe(1) // unfoldered counts as visible
  expect(attrs.categories).toBe(1)
  expect(attrs.categories_archived).toBe(1)
  expect(attrs.signup_year).toBe(2026)
  expect(attrs.signup_month).toBe(2)
  // Never loaded, so never guessed at.
  expect('tags' in attrs).toBe(false)
  expect('connections' in attrs).toBe(false)
})

it('is empty with no client', () => {
  setAnalyticsQueryClient(null)
  expect(profileAttributes()).toEqual({})
})
```

Adjust the account fixture shape to whatever `useAccounts()` actually caches — check `web/src/features/accounts/queries.ts` for whether it stores `AccountItemDto[]` or `AccountDto[]` and match it exactly.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm test -- analyticsProfile`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement**

```ts
// web/src/lib/analyticsProfile.ts
// Profile counts are read from the query cache rather than fetched: the numbers
// are already in memory, and sourcing them server-side would mean five
// cross-feature interfaces plus COUNT queries on every get-user-data.

import type { QueryClient } from '@tanstack/react-query'
import { queryKeys } from '@/app/queryKeys'

let client: QueryClient | null = null

export function setAnalyticsQueryClient(qc: QueryClient | null): void {
  client = qc
}

function list<T>(key: readonly unknown[]): T[] | undefined {
  const data = client?.getQueryData(key)
  return Array.isArray(data) ? (data as T[]) : undefined
}

function archivedCounts(
  out: Record<string, number>,
  key: readonly unknown[],
  name: string,
): void {
  const rows = list<{ isArchived: 0 | 1 }>(key)
  if (!rows) {
    return
  }
  out[name] = rows.filter((r) => r.isArchived === 0).length
  out[`${name}_archived`] = rows.filter((r) => r.isArchived === 1).length
}

export function profileAttributes(): Record<string, number> {
  const out: Record<string, number> = {}
  if (!client) {
    return out
  }

  const accounts = list<{ item: { folderId: string | null } }>(queryKeys.accounts)
  const folders = list<{ id: string; isVisible: 0 | 1 }>(queryKeys.folders)
  if (accounts) {
    out.accounts = accounts.length
    if (folders) {
      // Accounts have no visibility of their own; a hidden folder hides them.
      const hidden = new Set(folders.filter((f) => f.isVisible === 0).map((f) => f.id))
      out.accounts_hidden = accounts.filter((a) => a.item.folderId !== null && hidden.has(a.item.folderId)).length
    }
  }

  archivedCounts(out, queryKeys.categories, 'categories')
  archivedCounts(out, queryKeys.payees, 'payees')
  archivedCounts(out, queryKeys.tags, 'tags')

  const connections = list<unknown>(queryKeys.connections)
  if (connections) {
    out.connections = connections.length
  }

  const user = client.getQueryData<{ createdAt?: string }>(queryKeys.user)
  if (user?.createdAt) {
    // "2006-01-02 15:04:05" UTC; a cohort label, so it never drifts.
    const [y, m] = user.createdAt.slice(0, 7).split('-')
    const year = Number(y)
    const month = Number(m)
    if (Number.isFinite(year) && Number.isFinite(month)) {
      out.signup_year = year
      out.signup_month = month
    }
  }
  return out
}
```

- [ ] **Step 4: Feed the counts into every batch**

In `metrics.ts`, extend the `setAnalyticsContext` call site so batch attributes are recomputed per capture rather than fixed at module load: replace the module-scope call with a call inside `trackEvent`, immediately before `capture(...)`:

```ts
    setAnalyticsContext({
      $app_version: getVersion(),
      $platform: analyticsPlatform(),
      ...profileAttributes(),
    })
```

Register the app's QueryClient once where it is created (the provider in `web/src/app/`): `setAnalyticsQueryClient(queryClient)`.

- [ ] **Step 5: Run tests**

Run: `cd web && pnpm test`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd web && pnpm lint
git add -A
git commit -m "feat(web): stamp account profile counts on analytics batches"
```

---

### Task 12: Identity wiring and the opt-out gate

**Files:**
- Create: `web/src/lib/analyticsPreference.ts`, `web/src/lib/analyticsPreference.test.ts`
- Modify: `web/src/api/user.ts`, `web/src/features/auth/LogoutPage.tsx`, `web/src/lib/metrics.ts`, `web/src/api/dto/user.ts`
- Test: `web/src/lib/metrics.test.ts`

**Interfaces:**
- Consumes: `analyticsUserId` (Task 1), `setAnalyticsUser`/`setAnalyticsGroup`/`resetAnalyticsIdentity` (Task 9), `getInstanceId`/`analyticsHost` (Task 10), the `analytics` option (Task 4).
- Produces: `analyticsAllowed(): boolean`, `rememberAnalyticsPreference(enabled: boolean): void`; `UserOptions.ANALYTICS = 'analytics'`.

- [ ] **Step 1: Write the failing test**

```ts
// web/src/lib/analyticsPreference.test.ts
import { analyticsAllowed, rememberAnalyticsPreference } from './analyticsPreference'

beforeEach(() => localStorage.clear())

it('defaults to allowed', () => {
  expect(analyticsAllowed()).toBe(true)
})

it('remembers an opt-out across reloads', () => {
  rememberAnalyticsPreference(false)
  expect(analyticsAllowed()).toBe(false)
  rememberAnalyticsPreference(true)
  expect(analyticsAllowed()).toBe(true)
})
```

```ts
// in web/src/lib/metrics.test.ts
it('sends nothing to either sink when opted out', () => {
  const captureSpy = vi.spyOn(analyticsModule, 'capture')
  window.dataLayer = []
  rememberAnalyticsPreference(false)

  trackEvent(METRICS.ACCOUNT_CREATE)

  expect(captureSpy).not.toHaveBeenCalled()
  expect(window.dataLayer).toHaveLength(0)
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && pnpm test -- analyticsPreference metrics.test`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement the preference mirror**

```ts
// web/src/lib/analyticsPreference.ts
// The server owns this preference; this is a mirror. get-user-data resolves
// after the first page view fires, so without a synchronous local copy an
// opted-out user would still emit one event per page load.
//
// Deliberately NOT cleared on logout: it is a device-level fail-safe, so an
// opted-out user's login-page events do not resume the moment they sign out.
// The next user's data corrects it on arrival.

import { getItem, setItem } from './storage'

const KEY = 'analyticsOptOut'

export function analyticsAllowed(): boolean {
  return getItem(KEY) !== true
}

export function rememberAnalyticsPreference(enabled: boolean): void {
  setItem(KEY, !enabled)
}
```

- [ ] **Step 4: Gate `trackEvent`**

In `metrics.ts`, wrap the whole body of `trackEvent` after the `if (!metric)` guard:

```ts
  if (!analyticsAllowed()) {
    return
  }
```

placed before the `window.dataLayer.push(...)`, so the third-party tag sink is silenced too. Delete the `analyticsEnabled()` condition from the capture block, keeping the `appUIModal` exclusion.

- [ ] **Step 5: Wire identity and preference at the user-data sites**

In `web/src/api/user.ts`, at both existing `setAnalyticsAccessState(...)` call sites (in `login` and `getUserData`) add:

```ts
  setAnalyticsUser(analyticsUserId(user.id))
  rememberAnalyticsPreference(user.options.find((o) => o.name === UserOptions.ANALYTICS)?.value !== '0')
```

Add `ANALYTICS: 'analytics'` to `UserOptions` in `web/src/api/dto/user.ts`, and `createdAt: string` to `CurrentUserDto`.

Set the group once, where the app boots (the same provider that registers the QueryClient in Task 11):

```ts
setAnalyticsGroup(getInstanceId(), analyticsHost())
```

Guard it so an empty instance id skips the call.

In `web/src/features/auth/LogoutPage.tsx`, add `resetAnalyticsIdentity()` immediately after `trackEvent(METRICS.USER_LOGOUT)` and before `removeToken()`, with a one-line comment saying the logout event must still carry the identity.

- [ ] **Step 6: Run tests**

Run: `cd web && pnpm test`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
cd web && pnpm lint
git add -A
git commit -m "feat(web): identify users and honour the analytics opt-out"
```

---

### Task 13: The Settings toggle

**Files:**
- Modify: `web/src/api/user.ts`, `web/src/features/user/queries.ts`, `web/src/features/settings/ProfilePage.tsx`, `web/src/lib/metrics.ts`, `locales/*.json` (all 11)
- Test: `web/src/features/settings/ProfilePage.test.tsx`

**Interfaces:**
- Consumes: `POST /api/v1/user/update-analytics` (Task 4), `rememberAnalyticsPreference` (Task 12), `userOption` (existing).
- Produces: `updateAnalytics(enabled: boolean): Promise<CurrentUserDto>` (api), `useUpdateAnalytics()` (queries), `METRICS.USER_UPDATE_ANALYTICS = 'appUserUpdateAnalytics'`.

- [ ] **Step 1: Write the failing test**

```tsx
// in web/src/features/settings/ProfilePage.test.tsx
it('toggles the analytics preference', async () => {
  const user = userEvent.setup()
  renderProfilePage() // the file's existing helper, user fixture opted in
  const toggle = await screen.findByRole('switch', { name: /analytics/i })
  expect(toggle).toBeChecked()

  await user.click(toggle)

  await waitFor(() => expect(updateAnalyticsSpy).toHaveBeenCalledWith(false))
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm test -- ProfilePage`
Expected: FAIL — no switch found.

- [ ] **Step 3: Add the API client and mutation**

```ts
// web/src/api/user.ts
export async function updateAnalytics(enabled: boolean): Promise<CurrentUserDto> {
  const response = await api.post<CurrentUserResponseDto>(apiUrl('/api/v1/user/update-analytics'), { enabled })
  return response.data.data.user
}
```

```ts
// web/src/features/user/queries.ts
export function useUpdateAnalytics() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (enabled: boolean) => {
      // Fired before the write so an opting-out user's last event still lands;
      // opting in is tracked after, once events are allowed again.
      if (!enabled) {
        trackEvent(METRICS.USER_UPDATE_ANALYTICS, { enabled })
      }
      const user = await userApi.updateAnalytics(enabled)
      rememberAnalyticsPreference(enabled)
      if (enabled) {
        trackEvent(METRICS.USER_UPDATE_ANALYTICS, { enabled })
      }
      return user
    },
    onSuccess: (user) => {
      queryClient.setQueryData(queryKeys.user, user)
    },
  })
}
```

Add `USER_UPDATE_ANALYTICS: 'appUserUpdateAnalytics',` to `METRICS` next to the other `USER_*` keys.

- [ ] **Step 4: Add the UI**

In `ProfilePage.tsx`, after the preferences group and before the security group:

```tsx
      <p className="px-1 pb-1 pt-4 text-xs font-medium uppercase text-muted-foreground">
        {t('user.page.settings.profile.groups.privacy')}
      </p>
      <div className="flex max-w-md items-center justify-between gap-4 rounded-lg bg-econumo-card px-4 py-3.5">
        <label htmlFor="profile-analytics" className="text-sm">
          {t('user.page.settings.profile.analytics.label')}
          <span className="mt-0.5 block text-xs text-muted-foreground">
            {t('user.page.settings.profile.analytics.description')}
          </span>
        </label>
        <Switch
          id="profile-analytics"
          checked={analyticsOn}
          onCheckedChange={(checked) => updateAnalytics.mutate(checked)}
        />
      </div>
```

with, above the return:

```tsx
  const updateAnalytics = useUpdateAnalytics()
  const analyticsOn = userOption(user, UserOptions.ANALYTICS) !== '0'
```

Import `Switch` from `@/components/ui/switch`, and `userOption`/`UserOptions` from their existing homes.

- [ ] **Step 5: Add the translations**

Add three keys to `locales/en.json` under the existing `user.page.settings.profile` structure — `groups.privacy`, `analytics.label`, `analytics.description` — then the same keys with real translations in the other ten catalogues (`de`, `es`, `fr`, `it`, `nl`, `pl`, `pt`, `ru`, `uk`, `zh`). English text:

- `groups.privacy`: `"Privacy"`
- `analytics.label`: `"Share usage data"`
- `analytics.description`: `"Helps improve Econumo. Never includes your financial data."`

- [ ] **Step 6: Run tests**

```bash
cd web && pnpm test && pnpm lint
go test ./internal/test/i18ntest/
```

Expected: PASS, including the catalogue key-parity guard and `metrics-coverage.test.ts`.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat(web): add the analytics opt-out toggle to settings"
```

---

### Task 14: Documentation

**Files:**
- Modify: `.env.example`, `CLAUDE.md`, `docs/regression-test-plan.md`
- Create: `docs/superpowers/specs/2026-09-03-privacy-policy-revision.md`

**Interfaces:** none — documentation only.

- [ ] **Step 1: Update `.env.example`**

Delete the `ECONUMO_ANALYTICS` entry.

- [ ] **Step 2: Update CLAUDE.md**

- In the configuration list, replace the `ECONUMO_ANALYTICS` bullet with one describing it as deprecated: ignored by the application, read once by `migration:seed-analytics-option` to seed existing users, in the same words as the `ECONUMO_DATA_SALT` bullet. Analytics are now a per-user preference, on by default.
- Under the product-analytics rule, note that events carry a hashed user id, a per-instance group, and account profile counts, and that the per-user preference gates both sinks.
- Add `migration:seed-analytics-option` to the CLI command list and `update-analytics` where user endpoints are enumerated.
- Update the web-UI config list: `ANALYTICS` is gone, `INSTANCE_ID` is always merged.

- [ ] **Step 3: Update the regression test plan**

Add items to `docs/regression-test-plan.md`: toggling analytics off in Settings → Profile persists across a reload and across devices; a read-only (lapsed) user can still toggle it; the toggle's state survives logout and login.

- [ ] **Step 4: Draft the privacy-policy revision**

Write `docs/superpowers/specs/2026-09-03-privacy-policy-revision.md` containing the proposed replacement wording for econumo.com's policy (that site is not in this repo). It must state: usage events are linked to your account through a pseudonymous identifier; what is collected (feature usage, counts of accounts/categories/payees/tags/connections, signup month, app version, deployment and instance identifier); that no financial data, names, or email addresses are sent; legitimate interest as the lawful basis; the in-app opt-out and how to reach it; and a new effective date.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "docs: identified analytics, deprecated ECONUMO_ANALYTICS, policy draft"
```

---

### Task 15: Collector configuration

**Files:** none in this repository.

**Interfaces:** Consumes the attribute names from Tasks 10–11.

- [ ] **Step 1: Probe on a throwaway project**

Create a temporary project via the Twillingate MCP tools, set `identity=identified`, post one batch carrying `$user_id`, `$group_id`, `$group_name` and the new attributes, and confirm through `query` that the values land unhashed and the group surfaces resolve. The deployed collector has lagged its source before, so this must be verified rather than assumed.

- [ ] **Step 2: Switch the real project**

```
update_project alias=app.econumo.com identity=identified
update_project alias=app.econumo.com attributes=[access_state, host, locale, mode, deployment, signup_year, signup_month, connections, accounts, accounts_hidden, categories, categories_archived, payees, payees_archived, tags, tags_archived]
```

`$app_version` and `$platform` must **not** appear in that list: `$`-prefixed keys roll up automatically and are ignored if declared.

- [ ] **Step 3: Archive the throwaway project and verify live data**

After the SPA ships, confirm through `product_events` and `identities kind=group` that events arrive with identities attached and that instance groups appear.

---

## Plan self-review

- **Spec coverage.** §3.1 → T1, T12. §3.2 → T8, T12. §3.3 → T9, T12. §4 host/`current_url` → T10. §4.1 versions → T10. §4.2 `deployment`, counts, cohort → T10, T11, T2. §5.1 storage → T3, T5. §5.2 endpoint + allowlist → T4. §5.3 UI → T13. §5.4 gate and mirror → T12. §6 removal → T8 (server), T10 (client). §7 migration → T6, T7. §8 collector → T15. §9 tests → folded into each task. §9 docs → T14.
- **Sequencing.** T2 must land before T11 (the cohort reads `createdAt`); T8 before T10 (the host reads `INSTANCE_ID`); T9 before T10 and T12; T3 before T4, T5, T6, T7; T6 before T7.
- **Known soft spots for the implementer.** Test-harness helper names in T2, T5 and T7 are described by role, not quoted, because they differ per file — read the neighbouring tests first and reuse what is there rather than adding a parallel harness. The account cache shape in T11 must be checked against `useAccounts()` before the fixture is written.
