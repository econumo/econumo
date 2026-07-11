# Auth Brute-Force Protection (Rate Limiting) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Per-username attempt limiting (plus a per-endpoint global backstop) on login-user, reset-password, remind-password, and register-user, returning HTTP 429 with the standard error envelope; limits configurable via `ECONUMO_RATE_LIMIT_*` env vars.

**Architecture:** New in-memory `internal/infra/ratelimit` limiter (mutex + map of sliding-window timestamps, `port.Clock`-driven), consumed by the `user` feature through an `AttemptLimiter` interface in `internal/user/ports.go` and wired in `server.BuildAPI` from six new `config.Config` fields. New `errs.TooManyRequestsError` maps to 429 in `httpx.WriteError`.

**Tech Stack:** Go stdlib only (sync, time, maps). No new dependencies.

**Spec:** `docs/superpowers/specs/2026-07-09-auth-rate-limiting-design.md`

## Global Constraints

- **Frozen wire contract:** every EXISTING response must stay byte-identical. The 429 path is new; its message `"Too many attempts. Try again later."` becomes frozen once shipped. Envelope: `{"success":false,"message":"Too many attempts. Try again later.","code":429,"errors":{}}`.
- **Dependency rule:** `internal/user` consumes the limiter via its own interface; `internal/shared/*` may not import features or infra. `internal/infra/ratelimit` may import only `internal/shared/*`. (Enforced by `internal/test/archtest`.)
- **No PII in logs or limiter output:** the limiter never logs; keys (usernames/emails) must not appear in any log line.
- **Comments policy (CLAUDE.md):** comment only non-obvious rationale; keep swag `@…` blocks intact.
- **Formatting/verification:** `gofmt` clean; `make go-test` green (includes OpenAPI-freshness check + coverage gate 78%); regenerate goldens ONLY for the new scenario and inspect the diff; never hand-edit a golden.
- Env defaults (from the spec): LOGIN=5, RESET=5, REMIND=3, REGISTER=5, WINDOW=15m, GLOBAL=60/min; `0` disables that check.
- Run all commands from the worktree root: `/home/dmitry/dev/econumo/econumo/.claude/worktrees/graceful-soaring-pony`.

---

### Task 1: `errs.TooManyRequestsError`

**Files:**
- Modify: `internal/shared/errs/errs.go`
- Test: `internal/shared/errs/errs_test.go` (create if absent)

**Interfaces:**
- Consumes: nothing new.
- Produces: `errs.NewTooManyRequests(msg string) *TooManyRequestsError`, `errs.AsTooManyRequests(err error) (*TooManyRequestsError, bool)` — used by Tasks 2 and 3.

- [ ] **Step 1: Write the failing test**

Create `internal/shared/errs/errs_test.go` (if a file with this name already exists, append the test function):

```go
package errs_test

import (
	"fmt"
	"testing"

	"github.com/econumo/econumo/internal/shared/errs"
)

func TestTooManyRequests(t *testing.T) {
	e := errs.NewTooManyRequests("Too many attempts. Try again later.")
	if e.Error() != "Too many attempts. Try again later." {
		t.Fatalf("Error() = %q", e.Error())
	}
	if _, ok := errs.AsTooManyRequests(e); !ok {
		t.Fatal("AsTooManyRequests(e) = false, want true")
	}
	if _, ok := errs.AsTooManyRequests(fmt.Errorf("wrap: %w", e)); !ok {
		t.Fatal("AsTooManyRequests(wrapped) = false, want true")
	}
	if _, ok := errs.AsTooManyRequests(fmt.Errorf("other")); ok {
		t.Fatal("AsTooManyRequests(other) = true, want false")
	}
	empty := &errs.TooManyRequestsError{}
	if empty.Error() != "too many requests" {
		t.Fatalf("zero-value Error() = %q, want %q", empty.Error(), "too many requests")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/shared/errs/ -run TestTooManyRequests -v`
Expected: FAIL (compile error: `NewTooManyRequests` undefined)

- [ ] **Step 3: Implement**

Append to `internal/shared/errs/errs.go` (style mirrors `UnauthorizedError`):

```go
// TooManyRequestsError maps to HTTP 429 (a rate-limited auth attempt).
type TooManyRequestsError struct {
	Msg string
}

func (e *TooManyRequestsError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return "too many requests"
}

// NewTooManyRequests builds a TooManyRequestsError.
func NewTooManyRequests(msg string) *TooManyRequestsError { return &TooManyRequestsError{Msg: msg} }

// AsTooManyRequests reports whether err is (or wraps) a *TooManyRequestsError.
func AsTooManyRequests(err error) (*TooManyRequestsError, bool) {
	var v *TooManyRequestsError
	if errors.As(err, &v) {
		return v, true
	}
	return nil, false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/shared/errs/ -run TestTooManyRequests -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/shared/errs/
git commit -m "feat(errs): add TooManyRequestsError (HTTP 429 taxonomy entry)"
```

---

### Task 2: `httpx.WriteError` 429 mapping

**Files:**
- Modify: `internal/web/httpx/errors.go` (insert into `WriteError`, after the `AsUnauthorized` block at line ~42)
- Test: `internal/web/httpx/envelope_test.go` (extend `TestWriteError_StatusMappingMatrix`)

**Interfaces:**
- Consumes: `errs.AsTooManyRequests` (Task 1).
- Produces: any handler returning `*errs.TooManyRequestsError` now yields `429 {"success":false,"message":<msg>,"code":429,"errors":{}}`. Tasks 5–7 rely on this.

- [ ] **Step 1: Write the failing test**

In `internal/web/httpx/envelope_test.go`, find `TestWriteError_StatusMappingMatrix` (line ~149) and add a case following the existing table's row shape (read the existing rows first and copy their exact field names). The new row:

```go
{
	name:       "too many requests -> 429 envelope",
	err:        errs.NewTooManyRequests("Too many attempts. Try again later."),
	wantStatus: http.StatusTooManyRequests,
	wantBody:   `{"success":false,"message":"Too many attempts. Try again later.","code":429,"errors":{}}` + "\n",
},
```

(Adapt field names to the existing table struct; the existing rows show whether bodies are compared as full strings or decoded. If the matrix asserts on decoded fields, assert: status 429, `success` false, `message` exact, `code` 429, `errors` `{}`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/web/httpx/ -run TestWriteError_StatusMappingMatrix -v`
Expected: FAIL — the 429 case currently falls through to the 500 exception envelope.

- [ ] **Step 3: Implement**

In `internal/web/httpx/errors.go`, insert after the `AsUnauthorized` block (before `AsNotFound`):

```go
	if v, ok := errs.AsTooManyRequests(err); ok {
		Err(w, v.Error(), http.StatusTooManyRequests, nil, http.StatusTooManyRequests)
		return
	}
```

Also extend the mapping list in the `WriteError` doc comment with:
`//   - *errs.TooManyRequestsError -> 429, code 429, empty errors`

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/web/httpx/ -v`
Expected: PASS (all envelope tests)

- [ ] **Step 5: Commit**

```bash
git add internal/web/httpx/
git commit -m "feat(httpx): map TooManyRequestsError to the 429 error envelope"
```

---

### Task 3: `internal/infra/ratelimit` limiter

**Files:**
- Create: `internal/infra/ratelimit/ratelimit.go`
- Test: `internal/infra/ratelimit/ratelimit_test.go`

**Interfaces:**
- Consumes: `port.Clock` (`internal/shared/port`), `errs.NewTooManyRequests` (Task 1).
- Produces (used by Task 9's wiring; satisfies Task 4's `user.AttemptLimiter`):
  - `ratelimit.Config{Limits map[string]int; Window time.Duration; Global int; MaxKeys int}`
  - `ratelimit.New(cfg Config, clk port.Clock) *Limiter`
  - `(*Limiter).Allow(scope, key string) error`, `(*Limiter).Fail(scope, key string)`, `(*Limiter).Clear(scope, key string)`
  - `ratelimit.Message = "Too many attempts. Try again later."`

- [ ] **Step 1: Write the failing tests**

Create `internal/infra/ratelimit/ratelimit_test.go`:

```go
package ratelimit_test

import (
	"sync"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/ratelimit"
	"github.com/econumo/econumo/internal/shared/errs"
)

// fakeClock is a mutable clock so tests can slide the window.
type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time { return c.t }

func newLimiter(cfg ratelimit.Config) (*ratelimit.Limiter, *fakeClock) {
	clk := &fakeClock{t: time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)}
	return ratelimit.New(cfg, clk), clk
}

func cfgLogin(limit int) ratelimit.Config {
	return ratelimit.Config{Limits: map[string]int{"login": limit}, Window: 15 * time.Minute}
}

func mustAllowed(t *testing.T, l *ratelimit.Limiter, scope, key string) {
	t.Helper()
	if err := l.Allow(scope, key); err != nil {
		t.Fatalf("Allow(%s,%s) = %v, want nil", scope, key, err)
	}
}

func mustBlocked(t *testing.T, l *ratelimit.Limiter, scope, key string) {
	t.Helper()
	err := l.Allow(scope, key)
	if err == nil {
		t.Fatalf("Allow(%s,%s) = nil, want TooManyRequestsError", scope, key)
	}
	if _, ok := errs.AsTooManyRequests(err); !ok {
		t.Fatalf("Allow(%s,%s) = %T, want *errs.TooManyRequestsError", scope, key, err)
	}
	if err.Error() != ratelimit.Message {
		t.Fatalf("message = %q, want %q", err.Error(), ratelimit.Message)
	}
}

func TestAllow_UnderLimit(t *testing.T) {
	l, _ := newLimiter(cfgLogin(3))
	for i := 0; i < 2; i++ {
		mustAllowed(t, l, "login", "user@example.test")
		l.Fail("login", "user@example.test")
	}
	mustAllowed(t, l, "login", "user@example.test") // 2 failures < 3
}

func TestAllow_BlocksAtLimit(t *testing.T) {
	l, _ := newLimiter(cfgLogin(3))
	for i := 0; i < 3; i++ {
		mustAllowed(t, l, "login", "user@example.test")
		l.Fail("login", "user@example.test")
	}
	mustBlocked(t, l, "login", "user@example.test")
}

func TestAllow_KeysAreIndependent(t *testing.T) {
	l, _ := newLimiter(cfgLogin(1))
	l.Fail("login", "a@example.test")
	mustBlocked(t, l, "login", "a@example.test")
	mustAllowed(t, l, "login", "b@example.test")
}

func TestAllow_ScopesAreIndependent(t *testing.T) {
	l, _ := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 1, "reset": 1}, Window: 15 * time.Minute,
	})
	l.Fail("login", "u@example.test")
	mustBlocked(t, l, "login", "u@example.test")
	mustAllowed(t, l, "reset", "u@example.test")
}

func TestAllow_WindowSlides(t *testing.T) {
	l, clk := newLimiter(cfgLogin(2))
	l.Fail("login", "u@example.test")
	clk.t = clk.t.Add(10 * time.Minute)
	l.Fail("login", "u@example.test")
	mustBlocked(t, l, "login", "u@example.test")
	clk.t = clk.t.Add(6 * time.Minute) // first failure now 16m old -> expired
	mustAllowed(t, l, "login", "u@example.test")
	clk.t = clk.t.Add(10 * time.Minute) // second failure now expired too
	mustAllowed(t, l, "login", "u@example.test")
}

func TestClear_ResetsCounter(t *testing.T) {
	l, _ := newLimiter(cfgLogin(1))
	l.Fail("login", "u@example.test")
	mustBlocked(t, l, "login", "u@example.test")
	l.Clear("login", "u@example.test")
	mustAllowed(t, l, "login", "u@example.test")
}

func TestZeroLimit_Disables(t *testing.T) {
	l, _ := newLimiter(cfgLogin(0))
	for i := 0; i < 100; i++ {
		l.Fail("login", "u@example.test")
		mustAllowed(t, l, "login", "u@example.test")
	}
}

func TestUnknownScope_Disabled(t *testing.T) {
	l, _ := newLimiter(cfgLogin(1))
	for i := 0; i < 10; i++ {
		l.Fail("nope", "u@example.test")
		mustAllowed(t, l, "nope", "u@example.test")
	}
}

func TestGlobalCap_BlocksAcrossKeys(t *testing.T) {
	l, _ := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 100}, Window: 15 * time.Minute, Global: 3,
	})
	mustAllowed(t, l, "login", "a@example.test")
	mustAllowed(t, l, "login", "b@example.test")
	mustAllowed(t, l, "login", "c@example.test")
	mustBlocked(t, l, "login", "d@example.test") // 4th request, distinct key
}

func TestGlobalCap_SlidesOnItsOwnMinuteWindow(t *testing.T) {
	l, clk := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 100}, Window: 15 * time.Minute, Global: 2,
	})
	mustAllowed(t, l, "login", "a@example.test")
	mustAllowed(t, l, "login", "b@example.test")
	mustBlocked(t, l, "login", "c@example.test")
	clk.t = clk.t.Add(61 * time.Second)
	mustAllowed(t, l, "login", "c@example.test")
}

func TestGlobalCap_ZeroDisables(t *testing.T) {
	l, _ := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 100}, Window: 15 * time.Minute, Global: 0,
	})
	for i := 0; i < 200; i++ {
		mustAllowed(t, l, "login", "u@example.test")
	}
}

// A key already over its per-key limit must be rejected WITHOUT consuming the
// global budget — an attack on one account must not starve everyone else.
func TestBlockedKey_DoesNotConsumeGlobal(t *testing.T) {
	l, _ := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 1}, Window: 15 * time.Minute, Global: 3,
	})
	l.Fail("login", "victim@example.test")
	for i := 0; i < 50; i++ {
		mustBlocked(t, l, "login", "victim@example.test")
	}
	// Global budget (3) untouched by the blocked calls above.
	mustAllowed(t, l, "login", "a@example.test")
	mustAllowed(t, l, "login", "b@example.test")
	mustAllowed(t, l, "login", "c@example.test")
	mustBlocked(t, l, "login", "d@example.test")
}

func TestEviction_CapsKeyCount(t *testing.T) {
	l, clk := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 5}, Window: 15 * time.Minute, MaxKeys: 3,
	})
	l.Fail("login", "a@example.test")
	clk.t = clk.t.Add(time.Second)
	l.Fail("login", "b@example.test")
	clk.t = clk.t.Add(time.Second)
	l.Fail("login", "c@example.test")
	clk.t = clk.t.Add(time.Second)
	// Map full (3 keys); inserting a 4th evicts the stalest key (a).
	l.Fail("login", "d@example.test")
	// a's counter was evicted: 5 fresh failures needed again to block it.
	mustAllowed(t, l, "login", "a@example.test")
	// d's failure was recorded.
	for i := 0; i < 4; i++ {
		l.Fail("login", "d@example.test")
	}
	mustBlocked(t, l, "login", "d@example.test")
}

func TestEviction_PrefersExpiredEntries(t *testing.T) {
	l, clk := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 2}, Window: time.Minute, MaxKeys: 2,
	})
	l.Fail("login", "old@example.test")
	clk.t = clk.t.Add(2 * time.Minute) // old@ expires
	l.Fail("login", "fresh@example.test")
	l.Fail("login", "fresh@example.test")
	// Map at cap; the expired old@ entry is dropped, fresh@ keeps its 2 failures.
	l.Fail("login", "new@example.test")
	mustBlocked(t, l, "login", "fresh@example.test")
	mustAllowed(t, l, "login", "new@example.test")
}

// go test -race exercises the mutex.
func TestConcurrentAccess(t *testing.T) {
	l, _ := newLimiter(ratelimit.Config{
		Limits: map[string]int{"login": 3}, Window: 15 * time.Minute, Global: 1000,
	})
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			keys := []string{"a@example.test", "b@example.test", "c@example.test"}
			for i := 0; i < 200; i++ {
				k := keys[(g+i)%len(keys)]
				_ = l.Allow("login", k)
				l.Fail("login", k)
				if i%10 == 0 {
					l.Clear("login", k)
				}
			}
		}(g)
	}
	wg.Wait()
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/infra/ratelimit/ -v`
Expected: FAIL (package does not exist / does not compile)

- [ ] **Step 3: Implement**

Create `internal/infra/ratelimit/ratelimit.go`:

```go
// Package ratelimit is the in-memory brute-force limiter for the public auth
// endpoints: per-key sliding-window attempt counters plus a per-scope global
// backstop. State is process-local and resets on restart by design (single
// binary; see the 2026-07-09 auth-rate-limiting spec).
package ratelimit

import (
	"strings"
	"sync"
	"time"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
)

// Message is the frozen 429 envelope message.
const Message = "Too many attempts. Try again later."

const (
	defaultMaxKeys = 10000
	defaultWindow  = 15 * time.Minute
	// The global backstop counts every request in its own fixed window so a
	// spray of unique keys is caught quickly, independent of the per-key window.
	globalWindow = time.Minute
)

// Config holds the limiter policy. A limit of 0 (or an absent scope) disables
// that check.
type Config struct {
	Limits  map[string]int // per-scope max attempts per key per Window
	Window  time.Duration  // per-key sliding window (default 15m)
	Global  int            // per-scope cap per minute across ALL keys; 0 disables
	MaxKeys int            // per-key map size cap (default 10000)
}

type Limiter struct {
	mu       sync.Mutex
	cfg      Config
	clock    port.Clock
	attempts map[string][]time.Time
}

func New(cfg Config, clk port.Clock) *Limiter {
	if cfg.MaxKeys <= 0 {
		cfg.MaxKeys = defaultMaxKeys
	}
	if cfg.Window <= 0 {
		cfg.Window = defaultWindow
	}
	return &Limiter{cfg: cfg, clock: clk, attempts: map[string][]time.Time{}}
}

// Key layouts: per-key entries are "k\x00<scope>\x00<key>", global counters are
// "g\x00<scope>". The prefixes keep a caller-supplied key from ever colliding
// with a global counter.
func attemptKey(scope, key string) string { return "k\x00" + scope + "\x00" + key }
func globalKey(scope string) string       { return "g\x00" + scope }

// Allow reports whether another attempt may proceed. The per-key check runs
// first and rejects without consuming the global budget, so an attack on one
// key cannot starve every other caller of the endpoint.
func (l *Limiter) Allow(scope, key string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock.Now()

	if limit := l.cfg.Limits[scope]; limit > 0 {
		k := attemptKey(scope, key)
		hits := prune(l.attempts[k], now, l.cfg.Window)
		l.store(k, hits)
		if len(hits) >= limit {
			return errs.NewTooManyRequests(Message)
		}
	}
	if l.cfg.Global > 0 {
		k := globalKey(scope)
		hits := prune(l.attempts[k], now, globalWindow)
		if len(hits) >= l.cfg.Global {
			l.attempts[k] = hits
			return errs.NewTooManyRequests(Message)
		}
		l.attempts[k] = append(hits, now)
	}
	return nil
}

// Fail records a failed attempt against the key.
func (l *Limiter) Fail(scope, key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cfg.Limits[scope] <= 0 {
		return
	}
	now := l.clock.Now()
	k := attemptKey(scope, key)
	if _, exists := l.attempts[k]; !exists && len(l.attempts) >= l.cfg.MaxKeys {
		l.evict(now)
	}
	l.attempts[k] = append(prune(l.attempts[k], now, l.cfg.Window), now)
}

// Clear wipes the key's counter (called after a successful attempt).
func (l *Limiter) Clear(scope, key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, attemptKey(scope, key))
}

func (l *Limiter) store(k string, hits []time.Time) {
	if len(hits) == 0 {
		delete(l.attempts, k)
		return
	}
	l.attempts[k] = hits
}

// prune drops timestamps older than the window. Slices are append-only, so
// they are already sorted ascending.
func prune(hits []time.Time, now time.Time, window time.Duration) []time.Time {
	cutoff := now.Add(-window)
	i := 0
	for ; i < len(hits); i++ {
		if hits[i].After(cutoff) {
			break
		}
	}
	return hits[i:]
}

// evict makes room under MaxKeys: drop fully-expired per-key entries; if the
// map is still full, drop the entry with the oldest most-recent attempt.
// Global counters (a handful, one per scope) are never evicted.
func (l *Limiter) evict(now time.Time) {
	var stalest string
	var stalestAt time.Time
	for k, hits := range l.attempts {
		if strings.HasPrefix(k, "g\x00") {
			continue
		}
		pruned := prune(hits, now, l.cfg.Window)
		if len(pruned) == 0 {
			delete(l.attempts, k)
			continue
		}
		l.attempts[k] = pruned
		newest := pruned[len(pruned)-1]
		if stalest == "" || newest.Before(stalestAt) {
			stalest, stalestAt = k, newest
		}
	}
	if len(l.attempts) >= l.cfg.MaxKeys && stalest != "" {
		delete(l.attempts, stalest)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass (with race detector)**

Run: `go test ./internal/infra/ratelimit/ -race -v`
Expected: PASS, all tests

- [ ] **Step 5: Verify the arch rule still holds**

Run: `go test ./internal/test/archtest/`
Expected: PASS (ratelimit imports only shared packages)

- [ ] **Step 6: Commit**

```bash
git add internal/infra/ratelimit/
git commit -m "feat(ratelimit): in-memory sliding-window attempt limiter"
```

---

### Task 4: `user.AttemptLimiter` seam (interface + Service wiring, no enforcement yet)

**Files:**
- Modify: `internal/user/ports.go` (interface + scope constants)
- Modify: `internal/user/usecase.go` (Service field, `NewService` param, nil-safe helpers)
- Modify: `internal/server/server.go:73` (pass `nil` for now; real wiring in Task 9)
- Modify: `internal/cli/container.go:74` (pass `nil` — CLI commands are not rate-limited)
- Modify: `internal/user/api/harness_test.go:110` (thread a limiter param, default `nil`)
- Modify: `internal/user/admin_integration_test.go:33`, `internal/user/migrate_test.go:37` (pass `nil`)

**Interfaces:**
- Consumes: nothing from Task 3 (interface-only; structural typing matches later).
- Produces (used by Tasks 5–7 and 9):
  - `user.AttemptLimiter` interface: `Allow(scope, key string) error; Fail(scope, key string); Clear(scope, key string)`
  - Scope constants: `user.RateScopeLogin = "login"`, `user.RateScopeReset = "reset"`, `user.RateScopeRemind = "remind"`, `user.RateScopeRegister = "register"`
  - `NewService(repo, tx, encode, hasher, jwtSvc, currency, budgets, passwordRequests, mailer, clock, limiter, allowRegistration)` — limiter inserted between `clock` and `allowRegistration`
  - Unexported nil-safe helpers on `Service`: `allowAttempt(scope, key string) error`, `failAttempt(scope, key string)`, `clearAttempt(scope, key string)`
  - Test helper `newHarnessWithLimiter(t *testing.T, limiter appuser.AttemptLimiter) *harness` in `internal/user/api/harness_test.go`

- [ ] **Step 1: Add the interface and scope constants to `internal/user/ports.go`**

Append:

```go
// AttemptLimiter is the brute-force-protection seam for the public auth use
// cases. Keys are the lowercased+trimmed submitted username/email; scopes are
// the RateScope* constants. A nil limiter disables protection (CLI, tests).
type AttemptLimiter interface {
	// Allow reports whether another attempt may proceed; over-limit yields an
	// *errs.TooManyRequestsError (HTTP 429).
	Allow(scope, key string) error
	// Fail records a failed attempt.
	Fail(scope, key string)
	// Clear wipes the key's failure counter after a successful attempt.
	Clear(scope, key string)
}

// Rate-limit scopes; the same strings key the limiter config in internal/server.
const (
	RateScopeLogin    = "login"
	RateScopeReset    = "reset"
	RateScopeRemind   = "remind"
	RateScopeRegister = "register"
)
```

- [ ] **Step 2: Thread the field through `internal/user/usecase.go`**

Add `limiter AttemptLimiter` to the `Service` struct (after `clock`), add the parameter to `NewService` between `clock port.Clock` and `allowRegistration bool`, set it in the constructor body, and append the nil-safe helpers at the end of the file:

```go
// allowAttempt / failAttempt / clearAttempt guard the optional limiter (nil in
// the CLI and most tests), mirroring the nil-mailer pattern in RemindPassword.
func (s *Service) allowAttempt(scope, key string) error {
	if s.limiter == nil {
		return nil
	}
	return s.limiter.Allow(scope, key)
}

func (s *Service) failAttempt(scope, key string) {
	if s.limiter != nil {
		s.limiter.Fail(scope, key)
	}
}

func (s *Service) clearAttempt(scope, key string) {
	if s.limiter != nil {
		s.limiter.Clear(scope, key)
	}
}
```

- [ ] **Step 3: Update every `NewService` call site**

All six call sites gain an argument in position 11 (between the clock and the bool):

- `internal/server/server.go:73-76`: pass `nil` for now (Task 9 replaces it with the real limiter).
- `internal/cli/container.go:74`: pass `nil` (management commands are trusted local operations).
- `internal/user/admin_integration_test.go:33` and `internal/user/migrate_test.go:37`: pass `nil`.
- `internal/user/api/harness_test.go`: split the constructor —

```go
func newHarness(t *testing.T) *harness { return newHarnessWithLimiter(t, nil) }

// newHarnessWithLimiter lets rate-limit tests inject a tight limiter; every
// other test keeps the nil (disabled) default.
func newHarnessWithLimiter(t *testing.T, limiter appuser.AttemptLimiter) *harness {
	// ... existing newHarness body, with the svc line becoming:
	// svc := appuser.NewService(repo, txm, encode, hasher, jwtSvc, currency, budgets, passwordReqs, resetMailer, clk, limiter, cfg.AllowRegistration)
}
```

- [ ] **Step 4: Build + run the affected packages**

Run: `go build ./... && go test ./internal/user/... ./internal/server/... ./internal/cli/...`
Expected: PASS — behavior is unchanged (nil limiter everywhere).

- [ ] **Step 5: Commit**

```bash
git add internal/user/ internal/server/ internal/cli/
git commit -m "feat(user): AttemptLimiter seam on the user service (nil = disabled)"
```

---

### Task 5: Enforce in Login + API-level tests

**Files:**
- Modify: `internal/user/login.go`
- Test: create `internal/user/api/ratelimit_test.go`

**Interfaces:**
- Consumes: `s.allowAttempt`/`s.failAttempt`/`s.clearAttempt`, `user.RateScopeLogin` (Task 4); `ratelimit.New` (Task 3) in tests; 429 envelope (Task 2).
- Produces: rate-limited `Login`. The test file's `newLimitedHarness` helper is reused by Tasks 6–7.

- [ ] **Step 1: Write the failing tests**

Create `internal/user/api/ratelimit_test.go`:

```go
package api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/ratelimit"
	appuser "github.com/econumo/econumo/internal/user"
)

// tight per-key limits so a handful of calls trips the 429; global generous so
// only the per-key path is under test here.
func newLimitedHarness(t *testing.T) *harness {
	t.Helper()
	limiter := ratelimit.New(ratelimit.Config{
		Limits: map[string]int{
			appuser.RateScopeLogin:    2,
			appuser.RateScopeReset:    2,
			appuser.RateScopeRemind:   2,
			appuser.RateScopeRegister: 2,
		},
		Window: 15 * time.Minute,
		Global: 1000,
	}, realClock{})
	return newHarnessWithLimiter(t, limiter)
}

// realClock: the limiter needs a monotonic-ish clock; the harness fixedClock
// would also work (all attempts land in one window), but real time matches
// production wiring.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func assert429(t *testing.T, status int, env envelope) {
	t.Helper()
	if status != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 (body: %s)", status, env.raw)
	}
	if env.Success || env.Code != 429 || env.Message != "Too many attempts. Try again later." {
		t.Fatalf("envelope = %+v, want success=false code=429 frozen message", env)
	}
	if string(env.Errors) != "{}" {
		t.Fatalf("errors = %s, want {}", env.Errors)
	}
}

func login(h *harness, t *testing.T, username, password string) (int, envelope) {
	return h.do(t, "POST", "/api/v1/user/login-user", "", map[string]any{
		"username": username, "password": password,
	})
}

func TestLogin_RateLimitedAfterFailures(t *testing.T) {
	h := newLimitedHarness(t)
	for i := 0; i < 2; i++ {
		if status, _ := login(h, t, seedEmail, "wrong-pw"); status != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, status)
		}
	}
	// Third attempt is blocked even with the CORRECT password: lockout is by
	// attempt count, not credential.
	status, env := login(h, t, seedEmail, seedPassword)
	assert429(t, status, env)
}

func TestLogin_UnknownUserAlsoRateLimited(t *testing.T) {
	h := newLimitedHarness(t)
	for i := 0; i < 2; i++ {
		if status, _ := login(h, t, "ghost@example.test", "x"); status != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, status)
		}
	}
	status, env := login(h, t, "ghost@example.test", "x")
	assert429(t, status, env) // anti-enumeration: same behavior as an existing user
}

func TestLogin_SuccessClearsCounter(t *testing.T) {
	h := newLimitedHarness(t)
	if status, _ := login(h, t, seedEmail, "wrong-pw"); status != http.StatusUnauthorized {
		t.Fatal("expected 401")
	}
	if status, _ := login(h, t, seedEmail, seedPassword); status != http.StatusOK {
		t.Fatal("expected 200 (1 failure < limit 2)")
	}
	// Counter was cleared: two more failures allowed before a block.
	if status, _ := login(h, t, seedEmail, "wrong-pw"); status != http.StatusUnauthorized {
		t.Fatal("expected 401")
	}
	if status, _ := login(h, t, seedEmail, "wrong-pw"); status != http.StatusUnauthorized {
		t.Fatal("expected 401")
	}
	status, env := login(h, t, seedEmail, seedPassword)
	assert429(t, status, env)
}

func TestLogin_KeyIsCaseAndSpaceInsensitive(t *testing.T) {
	h := newLimitedHarness(t)
	if status, _ := login(h, t, seedEmail, "wrong-pw"); status != http.StatusUnauthorized {
		t.Fatal("expected 401")
	}
	if status, _ := login(h, t, "  "+upperFirst(seedEmail)+" ", "wrong-pw"); status != http.StatusUnauthorized {
		t.Fatal("expected 401")
	}
	// Both failures hit the same bucket -> third attempt blocked.
	status, env := login(h, t, seedEmail, seedPassword)
	assert429(t, status, env)
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]-32) + s[1:] // seedEmail starts with a lowercase ASCII letter
}

func TestLogin_NilLimiterUnlimited(t *testing.T) {
	h := newHarness(t) // nil limiter
	for i := 0; i < 10; i++ {
		if status, _ := login(h, t, seedEmail, "wrong-pw"); status != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401 (never 429)", i+1, status)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/user/api/ -run "TestLogin_RateLimited|TestLogin_UnknownUser|TestLogin_SuccessClears|TestLogin_KeyIs|TestLogin_NilLimiter" -v`
Expected: the RateLimited/UnknownUser/SuccessClears/KeyIs tests FAIL (429 never returned); NilLimiter passes.

- [ ] **Step 3: Implement in `internal/user/login.go`**

Replace the `Login` body's opening and the two failure paths:

```go
func (s *Service) Login(ctx context.Context, req model.LoginRequest, now time.Time) (*model.LoginResult, error) {
	limitKey := strings.ToLower(strings.TrimSpace(req.Username))
	if err := s.allowAttempt(RateScopeLogin, limitKey); err != nil {
		return nil, err
	}
	identifier := s.encode.Hash(strings.ToLower(req.Username))
	u, err := s.repo.GetByIdentifier(ctx, identifier)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			s.failAttempt(RateScopeLogin, limitKey)
			return nil, errs.NewUnauthorized("Invalid credentials.")
		}
		return nil, err
	}
	if !u.IsActive || !s.hasher.Verify(u.Password, req.Password, u.Salt) {
		s.failAttempt(RateScopeLogin, limitKey)
		return nil, errs.NewUnauthorized("Invalid credentials.")
	}

	email, derr := s.encode.Decode(u.Email)
	if derr != nil {
		return nil, derr
	}
	token, terr := s.jwt.Issue(u.ID.String(), email, now)
	if terr != nil {
		return nil, terr
	}
	cur, cerr := s.toCurrentUserWithEmail(ctx, u, email)
	if cerr != nil {
		return nil, cerr
	}
	s.clearAttempt(RateScopeLogin, limitKey)
	return &model.LoginResult{Token: token, User: cur}, nil
}
```

(The identifier hash deliberately keeps the frozen un-trimmed `strings.ToLower(req.Username)`; only the limiter key trims.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/user/... -v`
Expected: PASS — including all pre-existing login/user tests (nil limiter).

- [ ] **Step 5: Commit**

```bash
git add internal/user/
git commit -m "feat(user): rate-limit login attempts per username"
```

---

### Task 6: Enforce in RemindPassword + ResetPassword + tests

**Files:**
- Modify: `internal/user/password.go` (`RemindPassword` line ~55, `ResetPassword` line ~91)
- Test: append to `internal/user/api/ratelimit_test.go`

**Interfaces:**
- Consumes: Task 4 helpers + `RateScopeRemind`/`RateScopeReset`; `newLimitedHarness` (Task 5).
- Produces: rate-limited remind/reset.

- [ ] **Step 1: Write the failing tests**

Append to `internal/user/api/ratelimit_test.go`:

```go
func remind(h *harness, t *testing.T, username string) (int, envelope) {
	return h.do(t, "POST", "/api/v1/user/remind-password", "", map[string]any{"username": username})
}

func reset(h *harness, t *testing.T, username, code, password string) (int, envelope) {
	return h.do(t, "POST", "/api/v1/user/reset-password", "", map[string]any{
		"username": username, "code": code, "password": password,
	})
}

// Remind counts EVERY request (each one sends an email), success included.
func TestRemindPassword_RateLimited(t *testing.T) {
	h := newLimitedHarness(t)
	for i := 0; i < 2; i++ {
		if status, _ := remind(h, t, seedEmail); status != http.StatusOK {
			t.Fatalf("attempt %d: expected 200", i+1)
		}
	}
	status, env := remind(h, t, seedEmail)
	assert429(t, status, env)
}

// Anti-enumeration holds under limiting: unknown users return the same 200s
// then the same 429.
func TestRemindPassword_UnknownUserSameShape(t *testing.T) {
	h := newLimitedHarness(t)
	for i := 0; i < 2; i++ {
		if status, _ := remind(h, t, "ghost@example.test"); status != http.StatusOK {
			t.Fatalf("attempt %d: expected 200 (anti-enumeration)", i+1)
		}
	}
	status, env := remind(h, t, "ghost@example.test")
	assert429(t, status, env)
}

func TestResetPassword_RateLimitedOnBadCodes(t *testing.T) {
	h := newLimitedHarness(t)
	for i := 0; i < 2; i++ {
		if status, _ := reset(h, t, seedEmail, "000000000000", "new-pw-123"); status != http.StatusBadRequest {
			t.Fatalf("attempt %d: expected 400", i+1)
		}
	}
	status, env := reset(h, t, seedEmail, "000000000000", "new-pw-123")
	assert429(t, status, env)
}

// A successful reset clears the counter; look up the real code in the DB the
// same way internal/user/api/reset_password_test.go does (read that file and
// reuse its query helper/pattern verbatim).
func TestResetPassword_SuccessClearsCounter(t *testing.T) {
	h := newLimitedHarness(t)
	if status, _ := reset(h, t, seedEmail, "000000000000", "new-pw-123"); status != http.StatusBadRequest {
		t.Fatal("expected 400")
	}
	if status, _ := remind(h, t, seedEmail); status != http.StatusOK {
		t.Fatal("expected 200 remind")
	}
	code := fetchResetCode(t, h)
	if status, _ := reset(h, t, seedEmail, code, "new-pw-123"); status != http.StatusOK {
		t.Fatal("expected 200 reset")
	}
	// Counter cleared: two more bad attempts before a block.
	if status, _ := reset(h, t, seedEmail, "000000000000", "x-pw-123"); status != http.StatusBadRequest {
		t.Fatal("expected 400")
	}
	if status, _ := reset(h, t, seedEmail, "000000000000", "x-pw-123"); status != http.StatusBadRequest {
		t.Fatal("expected 400")
	}
	status, env := reset(h, t, seedEmail, "000000000000", "x-pw-123")
	assert429(t, status, env)
}
```

Also add the code-lookup helper (same DB read `reset_password_test.go:25` does inline):

```go
// fetchResetCode reads the emailed reset code for the seed user straight from
// the DB (the test mailer is a no-op).
func fetchResetCode(t *testing.T, h *harness) string {
	t.Helper()
	var code string
	if err := h.db.QueryRow(`SELECT code FROM users_password_requests WHERE user_id = ?`, seedUserID).Scan(&code); err != nil {
		t.Fatalf("read reset code: %v", err)
	}
	return code
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/user/api/ -run "TestRemindPassword_|TestResetPassword_" -v`
Expected: new tests FAIL (no 429); pre-existing reset tests still PASS.

- [ ] **Step 3: Implement in `internal/user/password.go`**

`RemindPassword` — the lowered username already exists at the top; add limiting immediately after it (every request counts, before the user lookup so anti-enumeration is airtight):

```go
	lowered := strings.ToLower(strings.TrimSpace(req.Username))
	if err := s.allowAttempt(RateScopeRemind, lowered); err != nil {
		return nil, err
	}
	s.failAttempt(RateScopeRemind, lowered) // every remind sends an email, so every request counts
```

`ResetPassword` — add at the top (after `lowered := …`), then `failAttempt` on each of the three failure paths and `clearAttempt` on success:

```go
	lowered := strings.ToLower(strings.TrimSpace(req.Username))
	if err := s.allowAttempt(RateScopeReset, lowered); err != nil {
		return nil, err
	}
	u, err := s.repo.GetByIdentifier(ctx, s.encode.Hash(lowered))
	if err != nil {
		if isNotFound(err) {
			s.failAttempt(RateScopeReset, lowered)
			return nil, errs.NewValidation("Reset password error")
		}
		return nil, err
	}

	pr, err := s.passwordRequests.GetByUserAndCode(ctx, u.ID, strings.TrimSpace(req.Code))
	if err != nil {
		if isNotFound(err) {
			s.failAttempt(RateScopeReset, lowered)
			return nil, errs.NewValidation("Reset password error")
		}
		return nil, err
	}
	if pr.IsExpired(s.clock.Now()) {
		s.failAttempt(RateScopeReset, lowered)
		return nil, errs.NewValidation("The code is expired")
	}
```

…and just before the final `return &model.ResetPasswordResult{}, nil`:

```go
	s.clearAttempt(RateScopeReset, lowered)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/user/... -v`
Expected: PASS (new + all pre-existing).

- [ ] **Step 5: Commit**

```bash
git add internal/user/
git commit -m "feat(user): rate-limit remind-password and reset-password"
```

---

### Task 7: Enforce in Register + tests

**Files:**
- Modify: `internal/user/register.go:16`
- Test: append to `internal/user/api/ratelimit_test.go`

**Interfaces:**
- Consumes: Task 4 helpers + `RateScopeRegister`; `newLimitedHarness` (Task 5).
- Produces: rate-limited `Register`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/user/api/ratelimit_test.go`:

```go
func registerUser(h *harness, t *testing.T, email string) (int, envelope) {
	return h.do(t, "POST", "/api/v1/user/register-user", "", map[string]any{
		"email": email, "password": "brand-new-pw", "name": "New User",
	})
}

// Register counts EVERY attempt (mass-creation protection), success included.
func TestRegister_RateLimited(t *testing.T) {
	h := newLimitedHarness(t)
	if status, _ := registerUser(h, t, "fresh@example.test"); status != http.StatusOK {
		t.Fatal("expected 200 first register")
	}
	if status, _ := registerUser(h, t, "fresh@example.test"); status != http.StatusBadRequest {
		t.Fatal("expected 400 duplicate")
	}
	status, env := registerUser(h, t, "fresh@example.test")
	assert429(t, status, env)
}

func TestRegister_KeysIndependent(t *testing.T) {
	h := newLimitedHarness(t)
	for i := 0; i < 2; i++ {
		registerUser(h, t, "a@example.test")
	}
	if status, env := registerUser(h, t, "a@example.test"); status != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for exhausted key, got %d: %s", status, env.raw)
	}
	if status, _ := registerUser(h, t, "b@example.test"); status != http.StatusOK {
		t.Fatal("expected 200 for a different email")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/user/api/ -run "TestRegister_" -v`
Expected: FAIL (third attempt returns 400, not 429).

- [ ] **Step 3: Implement in `internal/user/register.go`**

Add at the very top of `Register` (before the `allowRegistration` gate, so even disabled-registration probing is counted; needs `"strings"` added to the imports):

```go
	limitKey := strings.ToLower(strings.TrimSpace(req.Email))
	if err := s.allowAttempt(RateScopeRegister, limitKey); err != nil {
		return nil, err
	}
	s.failAttempt(RateScopeRegister, limitKey) // every attempt counts toward the cap
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/user/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/user/
git commit -m "feat(user): rate-limit register-user attempts per email"
```

---

### Task 8: Config env vars + `.env.example`

**Files:**
- Modify: `internal/config/config.go` (six fields, strict parse helpers, `Load` wiring)
- Modify: `.env.example`
- Test: `internal/config/config_test.go` (append)

**Interfaces:**
- Consumes: nothing new.
- Produces (used by Task 9): `Config.RateLimitLogin`, `.RateLimitReset`, `.RateLimitRemind`, `.RateLimitRegister` (int), `.RateLimitWindow` (`time.Duration`), `.RateLimitGlobal` (int).

- [ ] **Step 1: Write the failing tests**

Open `internal/config/config_test.go` first and match its existing style for setting env vars (likely `t.Setenv`) and for the minimum valid environment (`DATABASE_URL` is required). Append:

```go
func TestLoad_RateLimitDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "sqlite:///tmp/x.sqlite")
	c, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.RateLimitLogin != 5 || c.RateLimitReset != 5 || c.RateLimitRemind != 3 || c.RateLimitRegister != 5 {
		t.Fatalf("per-endpoint defaults = %d/%d/%d/%d, want 5/5/3/5",
			c.RateLimitLogin, c.RateLimitReset, c.RateLimitRemind, c.RateLimitRegister)
	}
	if c.RateLimitWindow != 15*time.Minute {
		t.Fatalf("window = %v, want 15m", c.RateLimitWindow)
	}
	if c.RateLimitGlobal != 60 {
		t.Fatalf("global = %d, want 60", c.RateLimitGlobal)
	}
}

func TestLoad_RateLimitOverridesAndDisable(t *testing.T) {
	t.Setenv("DATABASE_URL", "sqlite:///tmp/x.sqlite")
	t.Setenv("ECONUMO_RATE_LIMIT_LOGIN", "10")
	t.Setenv("ECONUMO_RATE_LIMIT_RESET", "0") // 0 = disabled
	t.Setenv("ECONUMO_RATE_LIMIT_REMIND", "7")
	t.Setenv("ECONUMO_RATE_LIMIT_REGISTER", "8")
	t.Setenv("ECONUMO_RATE_LIMIT_WINDOW", "1h30m")
	t.Setenv("ECONUMO_RATE_LIMIT_GLOBAL", "0")
	c, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.RateLimitLogin != 10 || c.RateLimitReset != 0 || c.RateLimitRemind != 7 || c.RateLimitRegister != 8 {
		t.Fatalf("overrides not applied: %+v", c)
	}
	if c.RateLimitWindow != 90*time.Minute || c.RateLimitGlobal != 0 {
		t.Fatalf("window/global overrides not applied: %v / %d", c.RateLimitWindow, c.RateLimitGlobal)
	}
}

func TestLoad_RateLimitBadValuesFailBoot(t *testing.T) {
	cases := map[string]string{
		"ECONUMO_RATE_LIMIT_LOGIN":  "five",
		"ECONUMO_RATE_LIMIT_GLOBAL": "-1",
		"ECONUMO_RATE_LIMIT_WINDOW": "15minutes",
	}
	for key, bad := range cases {
		t.Run(key, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "sqlite:///tmp/x.sqlite")
			t.Setenv(key, bad)
			if _, err := config.Load(); err == nil {
				t.Fatalf("Load() with %s=%q succeeded, want boot error", key, bad)
			}
		})
	}
}
```

(Add `"time"` to the test file's imports if absent.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run TestLoad_RateLimit -v`
Expected: FAIL (fields undefined).

- [ ] **Step 3: Implement in `internal/config/config.go`**

Add to the `Config` struct (after `SQLiteBusyTimeout`):

```go
	// Auth brute-force protection (see the 2026-07-09 auth-rate-limiting spec).
	// Counts are attempts per key per RateLimitWindow; 0 disables a check.
	RateLimitLogin    int           // ECONUMO_RATE_LIMIT_LOGIN: failed logins per username
	RateLimitReset    int           // ECONUMO_RATE_LIMIT_RESET: failed reset attempts per username
	RateLimitRemind   int           // ECONUMO_RATE_LIMIT_REMIND: remind requests per username
	RateLimitRegister int           // ECONUMO_RATE_LIMIT_REGISTER: register attempts per email
	RateLimitWindow   time.Duration // ECONUMO_RATE_LIMIT_WINDOW: sliding window (Go duration)
	RateLimitGlobal   int           // ECONUMO_RATE_LIMIT_GLOBAL: per-endpoint cap per minute
```

Add `"time"` to the imports. In `Load`, after the MAILER_DSN block:

```go
	// Rate-limit values fail at boot on a malformed value (unlike the lenient
	// getInt), because a typo here would silently disable brute-force protection.
	for _, p := range []struct {
		dst *int
		key string
		def int
	}{
		{&c.RateLimitLogin, "ECONUMO_RATE_LIMIT_LOGIN", 5},
		{&c.RateLimitReset, "ECONUMO_RATE_LIMIT_RESET", 5},
		{&c.RateLimitRemind, "ECONUMO_RATE_LIMIT_REMIND", 3},
		{&c.RateLimitRegister, "ECONUMO_RATE_LIMIT_REGISTER", 5},
		{&c.RateLimitGlobal, "ECONUMO_RATE_LIMIT_GLOBAL", 60},
	} {
		n, err := getIntStrict(p.key, p.def)
		if err != nil {
			return Config{}, err
		}
		*p.dst = n
	}
	window, werr := getDurationStrict("ECONUMO_RATE_LIMIT_WINDOW", 15*time.Minute)
	if werr != nil {
		return Config{}, werr
	}
	c.RateLimitWindow = window
```

Add the helpers next to `getInt`:

```go
// getIntStrict is getInt with a hard failure on malformed or negative input,
// for settings where a silent fallback would be a security downgrade.
func getIntStrict(key string, def int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s %q is not a non-negative integer", key, v)
	}
	return n, nil
}

func getDurationStrict(key string, def time.Duration) (time.Duration, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s %q is not a positive Go duration (e.g. \"15m\")", key, v)
	}
	return d, nil
}
```

- [ ] **Step 4: Add to `.env.example`**

Read `.env.example` first and match its comment style. Append near the other `ECONUMO_*` behavior vars:

```bash
# Brute-force protection for the public auth endpoints. Counts are attempts
# per username/email per window; 0 disables a check. Defaults shown.
#ECONUMO_RATE_LIMIT_LOGIN=5
#ECONUMO_RATE_LIMIT_RESET=5
#ECONUMO_RATE_LIMIT_REMIND=3
#ECONUMO_RATE_LIMIT_REGISTER=5
#ECONUMO_RATE_LIMIT_WINDOW=15m
#ECONUMO_RATE_LIMIT_GLOBAL=60
```

(If the file lists vars uncommented with defaults, follow that convention instead.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/config/ .env.example
git commit -m "feat(config): ECONUMO_RATE_LIMIT_* env vars (strict-parsed, 0 disables)"
```

---

### Task 9: Wire in BuildAPI + swagger annotations + apiparity 429 scenario

**Files:**
- Modify: `internal/server/server.go` (construct the limiter, replace Task 4's `nil`)
- Modify: `internal/user/api/user.go` (four `@Failure 429` annotations)
- Modify: `internal/test/apiparity/harness.go:72` (production-default limits in the harness cfg)
- Create: `internal/test/apiparity/catalogue_ratelimit.go`
- Create (generated): `internal/test/apiparity/testdata/golden/auth_rate_limit.golden`
- Regenerate: `internal/web/apidoc/docs/*` via `make swagger`

**Interfaces:**
- Consumes: `ratelimit.New`/`Config` (Task 3), `user.RateScope*` (Task 4), config fields (Task 8).
- Produces: production wiring; frozen 429 golden.

- [ ] **Step 1: Wire the limiter in `internal/server/server.go`**

Above the `userSvc := appuser.NewService(` call insert:

```go
	authLimiter := ratelimit.New(ratelimit.Config{
		Limits: map[string]int{
			appuser.RateScopeLogin:    cfg.RateLimitLogin,
			appuser.RateScopeReset:    cfg.RateLimitReset,
			appuser.RateScopeRemind:   cfg.RateLimitRemind,
			appuser.RateScopeRegister: cfg.RateLimitRegister,
		},
		Window: cfg.RateLimitWindow,
		Global: cfg.RateLimitGlobal,
	}, clk)
```

Replace the `nil` limiter argument with `authLimiter`, and add `"github.com/econumo/econumo/internal/infra/ratelimit"` to the imports.

- [ ] **Step 2: Add swagger annotations**

In `internal/user/api/user.go`, add to the `@Failure` blocks of `LoginUser`, `RegisterUser`, `RemindPassword`, and `ResetPassword` (after the 401 line in each):

```go
// @Failure     429     {object} apidoc.JsonResponseError
```

Then regenerate: `make swagger` — the committed docs under `internal/web/apidoc/docs/` change; commit them (the `make go-test` freshness check requires it).

- [ ] **Step 3: Set explicit limits in the apiparity harness**

In `internal/test/apiparity/harness.go`, extend the `cfg := config.Config{…}` literal (add `"time"` to imports if absent):

```go
		// Production-default auth rate limits: existing auth scenarios stay far
		// under them (1 bad login / 1 remind / 1 bad reset per fresh-DB scenario),
		// and the auth_rate_limit scenario deliberately exceeds them to freeze the
		// 429 envelope.
		RateLimitLogin:    5,
		RateLimitReset:    5,
		RateLimitRemind:   3,
		RateLimitRegister: 5,
		RateLimitWindow:   15 * time.Minute,
		RateLimitGlobal:   60,
```

- [ ] **Step 4: Add the scenario**

Create `internal/test/apiparity/catalogue_ratelimit.go`:

```go
package apiparity

import "fmt"

// Auth brute-force protection: the harness wires the production-default limits
// (5 login / 5 reset / 3 remind / 5 register per 15m window), so this scenario
// freezes the over-limit 429 envelope for each protected endpoint. Each
// scenario runs on a fresh DB + fresh in-memory limiter, keeping counts
// deterministic; total calls (22) stay under the global 60/min backstop.
func init() {
	register(Scenario{Name: "auth_rate_limit", Calls: func() []Call {
		var calls []Call
		for i := 1; i <= 5; i++ {
			calls = append(calls, Call{Label: fmt.Sprintf("err:login-bad-%d", i), Method: "POST", Path: "/api/v1/user/login-user", Auth: "",
				Body: map[string]any{"username": OwnerEmail, "password": "wrong"}})
		}
		// Correct password, but over the failure limit: lockout is by attempt
		// count, not credential.
		calls = append(calls, Call{Label: "err:login-limited", Method: "POST", Path: "/api/v1/user/login-user", Auth: "",
			Body: map[string]any{"username": OwnerEmail, "password": SeedPassword}})

		for i := 1; i <= 5; i++ {
			calls = append(calls, Call{Label: fmt.Sprintf("err:reset-bad-%d", i), Method: "POST", Path: "/api/v1/user/reset-password", Auth: "",
				Body: map[string]any{"username": GuestEmail, "code": "00000000-0000-0000-0000-000000000000", "password": "irrelevant-pw"}})
		}
		calls = append(calls, Call{Label: "err:reset-limited", Method: "POST", Path: "/api/v1/user/reset-password", Auth: "",
			Body: map[string]any{"username": GuestEmail, "code": "00000000-0000-0000-0000-000000000000", "password": "irrelevant-pw"}})

		// Remind counts every request (each sends an email via the console
		// transport), so three 200s then a 429.
		for i := 1; i <= 3; i++ {
			calls = append(calls, Call{Label: fmt.Sprintf("remind-%d", i), Method: "POST", Path: "/api/v1/user/remind-password", Auth: "",
				Body: map[string]any{"username": GuestEmail}})
		}
		calls = append(calls, Call{Label: "err:remind-limited", Method: "POST", Path: "/api/v1/user/remind-password", Auth: "",
			Body: map[string]any{"username": GuestEmail}})

		// Register counts every attempt: one success, four duplicate-email 400s,
		// then the cap.
		calls = append(calls, Call{Label: "register-1", Method: "POST", Path: "/api/v1/user/register-user", Auth: "",
			Body: map[string]any{"email": "ratelimit@example.test", "password": SeedPassword, "name": "RL"}})
		for i := 2; i <= 5; i++ {
			calls = append(calls, Call{Label: fmt.Sprintf("err:register-dup-%d", i), Method: "POST", Path: "/api/v1/user/register-user", Auth: "",
				Body: map[string]any{"email": "ratelimit@example.test", "password": SeedPassword, "name": "RL"}})
		}
		calls = append(calls, Call{Label: "err:register-limited", Method: "POST", Path: "/api/v1/user/register-user", Auth: "",
			Body: map[string]any{"email": "ratelimit@example.test", "password": SeedPassword, "name": "RL"}})
		return calls
	}})
}
```

- [ ] **Step 5: Generate the new golden, then verify NOTHING else changed**

```bash
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ -run "TestSmoke_Catalogue/auth_rate_limit" -v
git status --short internal/test/apiparity/testdata/
```

Expected: exactly ONE new file `testdata/golden/auth_rate_limit.golden`, zero modified goldens. INSPECT the new golden: every `*-limited` call must show `-> 429` with body `{"success":false,"message":"Too many attempts. Try again later.","code":429,"errors":{}}`; the five `login-bad` calls show the frozen 401; `remind-*` show 200 success envelopes.

If any EXISTING golden shows as modified: STOP — a production response changed; find and fix the regression before proceeding.

- [ ] **Step 6: Run the full apiparity + server suites**

Run: `go test ./internal/test/apiparity/ ./internal/server/... -v 2>&1 | tail -20`
Expected: PASS, including the guard tests (scenario count grew by 1; no orphaned goldens).

- [ ] **Step 7: Commit**

```bash
git add internal/server/ internal/user/api/user.go internal/web/apidoc/ internal/test/apiparity/
git commit -m "feat(server): wire auth rate limiter; freeze the 429 envelope in apiparity"
```

---

### Task 10: Docs + full verification

**Files:**
- Modify: `CLAUDE.md` (Configuration section)
- Verify: whole repo

- [ ] **Step 1: Document the env vars in CLAUDE.md**

In the Configuration list (after the `SQLITE_BUSY_TIMEOUT` entry), add:

```markdown
- `ECONUMO_RATE_LIMIT_LOGIN` / `ECONUMO_RATE_LIMIT_RESET` / `ECONUMO_RATE_LIMIT_REMIND` /
  `ECONUMO_RATE_LIMIT_REGISTER` — brute-force protection for the public auth endpoints:
  max attempts per username/email per window (defaults 5/5/3/5; login and reset count
  only FAILED attempts and clear on success, remind and register count every request).
  `ECONUMO_RATE_LIMIT_WINDOW` — sliding window (Go duration, default `15m`).
  `ECONUMO_RATE_LIMIT_GLOBAL` — per-endpoint cap per minute across all keys (default `60`).
  `0` disables a check. Over-limit requests get HTTP 429 with the standard error envelope
  (message `"Too many attempts. Try again later."`, frozen). State is in-memory (resets on
  restart); a malformed value fails at boot.
```

Also add one line to the "Wire & data contract (frozen)" envelope list:

```markdown
- Rate-limited (429): `{"success": false, "message": "Too many attempts. Try again later.", "code": 429, "errors": {}}` — same shape as the handled-error envelope.
```

- [ ] **Step 2: Full smoke verification**

Run: `make go-test`
Expected: PASS — build + vet + gofmt + OpenAPI-freshness + all tests + coverage gate (≥78%; the new packages are heavily tested, so coverage should not drop).

- [ ] **Step 3: Engine-comparison + frontend tier (if Docker/PostgreSQL available)**

Run: `make test`
Expected: PASS — the enginecompare suite replays `auth_rate_limit` on both engines byte-identically. If Docker is unavailable, note it in the final report instead of skipping silently.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: document ECONUMO_RATE_LIMIT_* configuration and the 429 envelope"
```
