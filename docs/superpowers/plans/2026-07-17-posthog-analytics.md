# PostHog Analytics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Send the SPA's existing ~100 product events to PostHog Cloud US as anonymous, cookieless events, with an `ECONUMO_ANALYTICS` env-var opt-out (default enabled) surfaced to the SPA by templating `/econumo-config.js`.

**Architecture:** A hand-rolled batching capture client (`web/src/lib/analytics.ts`, transport only) becomes a second consumer inside the existing `trackEvent()` choke point (`metrics.ts` builds the whitelisted properties; the dataLayer/liltag push stays byte-identical). The Go SPA handler appends one `window.econumoConfig.ANALYTICS = <bool>;` line to the served config file, reflecting the env var.

**Tech Stack:** Go stdlib (config + SPA handler), TypeScript/React SPA, vitest, no new dependencies (no posthog-js).

Spec: `docs/superpowers/specs/2026-07-17-posthog-analytics-design.md`.

## Global Constraints

- PostHog host: `https://us.i.posthog.com`, batch endpoint path `/batch/`.
- Project API key (public by design): `phc_nsMAM8nZ2N9Xh4PmCTuUpihHC2tHJnkMKKPdHxSHwDEk`.
- Every captured event carries `$process_person_profile: false`.
- `distinct_id`: `crypto.randomUUID()` once per page load, module memory only — NEVER cookies/localStorage/sessionStorage.
- Complete property whitelist: `domain`, `selfHosted`, `locale`, `version`, `page` (UUID segments → `:id`). Nothing else, ever.
- `domain` = real hostname only for `econumo.com`/`*.econumo.com`, else literal `"self-hosted"`; `selfHosted` = `domain === 'self-hosted'`. Do NOT use the legacy `selfHosted()` localStorage flag for analytics.
- Failure is silence: dropped batches, no retries, no console noise. Analytics must never break the app.
- The existing `window.dataLayer` push in `trackEvent()` stays byte-identical.
- `ECONUMO_ANALYTICS` default `true`; malformed value FAILS AT BOOT (strict parse — deliberate deviation from lenient `getBool`, because a typo while disabling analytics must not silently leave it on).
- SPA-side gate fails OPEN: absent/unknown `window.econumoConfig.ANALYTICS` = enabled.
- Design note: the spec's ~100-event queue cap is structurally unnecessary — `flush()` empties the queue synchronously at 10 events regardless of fetch outcome, so the queue can never exceed 10. Do not add dead-code capping.
- Repo rules: comments only for non-obvious rationale; `make go-test` must pass (gofmt, vet, coverage gate); frontend done = `pnpm test` + `pnpm lint` + `pnpm exec tsc -b` all green.

---

### Task 1: Backend config — `ECONUMO_ANALYTICS`

**Files:**
- Modify: `internal/config/config.go` (field near `CheckUpdates` ~line 27; parse in `Load` ~line 80; new `getBoolStrict` next to `getBool` ~line 199)
- Test: `internal/config/config_test.go` (after `TestLoad_CheckUpdates`, ~line 218)

**Interfaces:**
- Produces: `Config.Analytics bool` (default `true`) — consumed by Task 2's router wiring.

- [ ] **Step 1: Write the failing test**

Append to `internal/config/config_test.go`:

```go
func TestLoad_Analytics(t *testing.T) {
	t.Setenv("DATABASE_URL", "sqlite:///tmp/x.sqlite")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.Analytics {
		t.Fatal("Analytics default = false, want true")
	}
	t.Setenv("ECONUMO_ANALYTICS", "false")
	c, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Analytics {
		t.Fatal("Analytics with ECONUMO_ANALYTICS=false = true, want false")
	}
	// Strict parse: a typo while trying to disable analytics fails at boot
	// rather than silently leaving it enabled.
	t.Setenv("ECONUMO_ANALYTICS", "flase")
	if _, err = Load(); err == nil {
		t.Fatal("Load with ECONUMO_ANALYTICS=flase: err = nil, want boot error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoad_Analytics -v`
Expected: FAIL (compile error: `c.Analytics` undefined)

- [ ] **Step 3: Implement**

In `internal/config/config.go`:

(a) Add the field directly under `CheckUpdates`:

```go
	Analytics bool // ECONUMO_ANALYTICS: SPA sends anonymous product events to PostHog (default true)
```

(b) In `Load`, after the `c.MailProvider, ... = ...` line (before the rate-limit block):

```go
	// Strict parse (unlike the lenient getBool): a typo while trying to
	// DISABLE analytics must fail at boot, not silently leave it enabled.
	analytics, err := getBoolStrict("ECONUMO_ANALYTICS", true)
	if err != nil {
		return Config{}, err
	}
	c.Analytics = analytics
```

(c) Add next to `getBool`:

```go
func getBoolStrict(key string, def bool) (bool, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	}
	return false, fmt.Errorf("%s: invalid boolean %q", key, v)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestLoad_Analytics -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): ECONUMO_ANALYTICS opt-out flag (default true, strict parse)"
```

---

### Task 2: Backend SPA — templated `/econumo-config.js`

**Files:**
- Modify: `internal/web/spa/spa.go` (Handler signature + new serveRuntimeConfig)
- Modify: `internal/web/router/router.go:115` (pass the flag)
- Test: `internal/web/spa/spa_test.go` (two existing `Handler(...)` call sites at lines 40 and 79, plus new tests)

**Interfaces:**
- Consumes: `Config.Analytics` from Task 1 (via `deps.Cfg.Analytics`).
- Produces: `spa.Handler(dir string, analytics bool) http.Handler`; `GET /econumo-config.js` returns the dist file + `\nwindow.econumoConfig.ANALYTICS = true;\n` (or `false`).

- [ ] **Step 1: Write the failing tests**

In `internal/web/spa/spa_test.go`, change both existing constructors to `Handler(newSPADir(t), true)`, and append:

```go
func TestSPA_RuntimeConfigOverride(t *testing.T) {
	for _, tc := range []struct {
		name      string
		analytics bool
		want      string
	}{
		{"enabled", true, "window.econumoConfig.ANALYTICS = true;"},
		{"disabled", false, "window.econumoConfig.ANALYTICS = false;"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := Handler(newSPADir(t), tc.analytics)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/econumo-config.js", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			body := rec.Body.String()
			if !strings.HasPrefix(body, "window.econumoConfig={}") {
				t.Fatalf("body does not start with the dist config: %q", body)
			}
			if !strings.Contains(body, tc.want) {
				t.Fatalf("body missing %q: %q", tc.want, body)
			}
			if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
				t.Fatalf("Cache-Control = %q, want %q", got, "no-cache")
			}
			if got := rec.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
				t.Fatalf("Content-Type = %q", got)
			}
		})
	}
}

func TestSPA_RuntimeConfigMissingFile(t *testing.T) {
	h := Handler(t.TempDir(), true)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/econumo-config.js", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/web/spa/ -v`
Expected: FAIL (compile error: too many arguments to `Handler`)

- [ ] **Step 3: Implement**

In `internal/web/spa/spa.go`: add `"fmt"` to imports; change the signature to `func Handler(dir string, analytics bool) http.Handler`; insert after the `isReservedPath` block (before the `fileExists` check):

```go
		// The runtime config is the one templated response: the dist file plus
		// a server-controlled override line, so the instance's .env genuinely
		// controls the shipped SPA.
		if cleaned == "/econumo-config.js" {
			serveRuntimeConfig(w, r, dir, analytics)
			return
		}
```

And add:

```go
func serveRuntimeConfig(w http.ResponseWriter, r *http.Request, dir string, analytics bool) {
	content, err := os.ReadFile(filepath.Join(dir, "econumo-config.js"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprintf(w, "%s\nwindow.econumoConfig.ANALYTICS = %t;\n", content, analytics)
}
```

In `internal/web/router/router.go` line 115:

```go
	root.Handle("/", spa.Handler(deps.Cfg.SPADir, deps.Cfg.Analytics))
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/web/... ./internal/server/... -count=1`
Expected: PASS (including the existing "runtime config revalidates" cache case)

- [ ] **Step 5: Run the full smoke tier**

Run: `make go-test`
Expected: PASS (no apiparity/golden churn — no API routes changed)

- [ ] **Step 6: Commit**

```bash
git add internal/web/spa/spa.go internal/web/spa/spa_test.go internal/web/router/router.go
git commit -m "feat(spa): template ANALYTICS flag into served econumo-config.js"
```

---

### Task 3: Frontend — capture client `analytics.ts`

**Files:**
- Create: `web/src/lib/analytics.ts`
- Test: `web/src/lib/analytics.test.ts`

**Interfaces:**
- Produces (consumed by Task 5):
  - `capture(event: string, properties?: Record<string, unknown>): void`
  - `analyticsDomain(hostname?: string): string`

- [ ] **Step 1: Write the failing tests**

Create `web/src/lib/analytics.test.ts`:

```ts
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

type AnalyticsModule = typeof import('./analytics')

let analytics: AnalyticsModule
let fetchMock: ReturnType<typeof vi.fn>

beforeEach(async () => {
  vi.useFakeTimers()
  fetchMock = vi.fn(() => Promise.resolve(new Response()))
  vi.stubGlobal('fetch', fetchMock)
  vi.resetModules()
  analytics = await import('./analytics')
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

function sentPayload(call = 0): { api_key: string; batch: Array<Record<string, any>> } {
  return JSON.parse(fetchMock.mock.calls[call][1].body as string)
}

describe('analyticsDomain', () => {
  it.each([
    ['econumo.com', 'econumo.com'],
    ['app.econumo.com', 'app.econumo.com'],
    ['myeconumo.com', 'self-hosted'],
    ['budget.example.org', 'self-hosted'],
    ['localhost', 'self-hosted'],
  ])('%s -> %s', (hostname, expected) => {
    expect(analytics.analyticsDomain(hostname)).toBe(expected)
  })
})

describe('capture', () => {
  it('batches and flushes on the timer with the PostHog payload shape', () => {
    analytics.capture('appTransactionCreate', { locale: 'en' })
    expect(fetchMock).not.toHaveBeenCalled()
    vi.advanceTimersByTime(10_000)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('https://us.i.posthog.com/batch/')
    expect(init.method).toBe('POST')
    expect(init.keepalive).toBe(true)
    const payload = sentPayload()
    expect(payload.api_key).toMatch(/^phc_/)
    expect(payload.batch).toHaveLength(1)
    expect(payload.batch[0].event).toBe('appTransactionCreate')
    expect(payload.batch[0].timestamp).toBeTruthy()
    expect(payload.batch[0].properties).toMatchObject({
      locale: 'en',
      $process_person_profile: false,
    })
  })

  it('uses one in-memory distinct_id per page load', () => {
    analytics.capture('a')
    analytics.capture('b')
    vi.advanceTimersByTime(10_000)
    const { batch } = sentPayload()
    expect(batch[0].distinct_id).toBe(batch[1].distinct_id)
    expect(batch[0].distinct_id).toMatch(/^[0-9a-f-]{36}$/)
    expect(document.cookie).toBe('')
    expect(localStorage.length).toBe(0)
  })

  it('flushes immediately at 10 queued events', () => {
    for (let i = 0; i < 10; i++) {
      analytics.capture(`event-${i}`)
    }
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(sentPayload().batch).toHaveLength(10)
    // The timer was cleared by the size-triggered flush: nothing further goes out.
    vi.advanceTimersByTime(10_000)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('drops the batch silently when fetch rejects', async () => {
    fetchMock.mockImplementation(() => Promise.reject(new TypeError('blocked')))
    analytics.capture('a')
    vi.advanceTimersByTime(10_000)
    await vi.runAllTimersAsync()
    // No unhandled rejection, and the queue restarts empty.
    analytics.capture('b')
    vi.advanceTimersByTime(10_000)
    expect(sentPayload(1).batch).toHaveLength(1)
  })

  it('sends the tail via sendBeacon when the tab hides', () => {
    const beacon = vi.fn(() => true)
    vi.stubGlobal('navigator', { ...window.navigator, sendBeacon: beacon })
    analytics.capture('a')
    Object.defineProperty(document, 'visibilityState', { value: 'hidden', configurable: true })
    document.dispatchEvent(new Event('visibilitychange'))
    expect(beacon).toHaveBeenCalledTimes(1)
    const [url, body] = beacon.mock.calls[0]
    expect(url).toBe('https://us.i.posthog.com/batch/')
    expect(JSON.parse(body as string).batch).toHaveLength(1)
    expect(fetchMock).not.toHaveBeenCalled()
    Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true })
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && pnpm test src/lib/analytics.test.ts`
Expected: FAIL (cannot resolve `./analytics`)

- [ ] **Step 3: Implement**

Create `web/src/lib/analytics.ts`:

```ts
// PostHog capture transport — the only file that knows PostHog exists.
// Anonymous by construction: the distinct_id is a random per-page-load value
// held in memory (never persisted anywhere), and only the whitelisted
// properties built by the caller ever leave the browser.
// See docs/superpowers/specs/2026-07-17-posthog-analytics-design.md.

const POSTHOG_HOST = 'https://us.i.posthog.com'
// A PostHog project API key is public by design (it can only ingest events).
const POSTHOG_KEY = 'phc_nsMAM8nZ2N9Xh4PmCTuUpihHC2tHJnkMKKPdHxSHwDEk'
const FLUSH_AT = 10
const FLUSH_INTERVAL_MS = 10_000

interface CapturedEvent {
  event: string
  distinct_id: string
  timestamp: string
  properties: Record<string, unknown>
}

const distinctId = crypto.randomUUID()
let queue: CapturedEvent[] = []
let timer: ReturnType<typeof setTimeout> | null = null

export function analyticsDomain(hostname: string = window.location.hostname): string {
  if (hostname === 'econumo.com' || hostname.endsWith('.econumo.com')) {
    return hostname
  }
  return 'self-hosted'
}

export function capture(event: string, properties: Record<string, unknown> = {}): void {
  if (import.meta.env.MODE === 'development') {
    return
  }
  queue.push({
    event,
    distinct_id: distinctId,
    timestamp: new Date().toISOString(),
    properties: { ...properties, $process_person_profile: false },
  })
  if (queue.length >= FLUSH_AT) {
    flush()
    return
  }
  timer ??= setTimeout(flush, FLUSH_INTERVAL_MS)
}

function takeBatch(): string | null {
  if (timer) {
    clearTimeout(timer)
    timer = null
  }
  if (queue.length === 0) {
    return null
  }
  const body = JSON.stringify({ api_key: POSTHOG_KEY, batch: queue })
  queue = []
  return body
}

function flush(): void {
  const body = takeBatch()
  if (!body) {
    return
  }
  void fetch(`${POSTHOG_HOST}/batch/`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    keepalive: true,
    body,
  }).catch(() => {
    // Dropped on purpose: analytics must never break or noisy-log the app.
  })
}

// Flush the tail when the tab hides; sendBeacon survives page teardown.
document.addEventListener('visibilitychange', () => {
  if (document.visibilityState !== 'hidden') {
    return
  }
  const body = takeBatch()
  if (!body) {
    return
  }
  try {
    navigator.sendBeacon(`${POSTHOG_HOST}/batch/`, body)
  } catch {
    // Same policy as flush: drop silently.
  }
})
```

Note: `import.meta.env.MODE === 'development'` (not `.DEV`) so `pnpm dev` is excluded while vitest (`MODE === 'test'`) can exercise the client; a production build is `'production'`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && pnpm test src/lib/analytics.test.ts`
Expected: PASS (6 tests)

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/analytics.ts web/src/lib/analytics.test.ts
git commit -m "feat(web): anonymous PostHog batch capture client"
```

---

### Task 4: Frontend — `analyticsEnabled()` gate + config defaults

**Files:**
- Modify: `web/src/lib/config.ts` (add `ANALYTICS` to `EconumoConfig` ~line 16; new function near `isRegistrationAllowed` ~line 118)
- Modify: `web/public/econumo-config.js`
- Test: `web/src/lib/config.test.ts`

**Interfaces:**
- Produces (consumed by Task 5): `analyticsEnabled(): boolean` — `true` unless `window.econumoConfig.ANALYTICS` is `false`/`'false'` (fails open).

- [ ] **Step 1: Write the failing tests**

Append to `web/src/lib/config.test.ts` (it already imports `* as config` — match the file's existing import style):

```ts
describe('analyticsEnabled', () => {
  it.each([
    [undefined, true],
    [true, true],
    ['true', true],
    [false, false],
    ['false', false],
    ['garbage', true], // unknown fails OPEN: enabled-by-default contract
  ])('ANALYTICS=%s -> %s', (value, expected) => {
    window.econumoConfig = { ANALYTICS: value as boolean | string | undefined }
    expect(config.analyticsEnabled()).toBe(expected)
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && pnpm test src/lib/config.test.ts`
Expected: FAIL (`analyticsEnabled` is not a function)

- [ ] **Step 3: Implement**

In `web/src/lib/config.ts` — add to the `EconumoConfig` interface:

```ts
  ANALYTICS?: boolean | string
```

Add below `isRegistrationAllowed`:

```ts
export function analyticsEnabled(): boolean {
  const analytics = window.econumoConfig?.ANALYTICS
  if (typeof analytics === 'boolean') {
    return analytics
  }
  // Absent or unrecognized fails OPEN (enabled): a stale hand-hosted config
  // file keeps the enabled-by-default contract.
  return analytics !== 'false'
}
```

In `web/public/econumo-config.js`, add after `PAYWALL_ENABLED`:

```js
  ANALYTICS: true,
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && pnpm test src/lib/config.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/config.ts web/src/lib/config.test.ts web/public/econumo-config.js
git commit -m "feat(web): analyticsEnabled() runtime-config gate"
```

---

### Task 5: Frontend — wire `trackEvent()` to PostHog

**Files:**
- Modify: `web/src/lib/metrics.ts` (imports + `scrubbedPage` + capture call in `trackEvent`)
- Test: `web/src/lib/metrics.test.ts`

**Interfaces:**
- Consumes: `capture`/`analyticsDomain` (Task 3), `analyticsEnabled` (Task 4), existing `locale`/`getVersion` from `config.ts`.
- Produces: `scrubbedPage(pathname: string): string` (exported for tests); unchanged `trackEvent` signature.

- [ ] **Step 1: Write the failing tests**

Rewrite `web/src/lib/metrics.test.ts` as:

```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { METRICS, scrubbedPage, trackEvent } from './metrics'
import { capture } from './analytics'

vi.mock('./analytics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./analytics')>()
  return { ...actual, capture: vi.fn() }
})

beforeEach(() => {
  vi.clearAllMocks()
  window.econumoConfig = {}
  window.dataLayer = []
  window.history.replaceState({}, '', '/')
})

it('pushes the event with context to the dataLayer', () => {
  trackEvent(METRICS.TRANSACTION_CREATE, { a: 1 })
  expect(window.dataLayer).toHaveLength(1)
  const entry = window.dataLayer[0] as Record<string, unknown>
  expect(entry.event).toBe('appTransactionCreate')
  expect(entry.eventData).toEqual({ a: 1 })
  expect(entry.eventContext).toMatchObject({ selfHosted: false, locale: 'en' })
})

describe('PostHog capture', () => {
  it('captures with the whitelisted properties only', () => {
    window.history.replaceState({}, '', '/budgets/01980e2c-1111-7000-8000-123456789abc/details')
    trackEvent(METRICS.TRANSACTION_CREATE, { secret: 'never-sent' })
    expect(capture).toHaveBeenCalledTimes(1)
    const [event, props] = vi.mocked(capture).mock.calls[0]
    expect(event).toBe('appTransactionCreate')
    expect(props).toEqual({
      domain: 'self-hosted', // jsdom runs on localhost
      selfHosted: true,
      locale: 'en',
      version: 'dev',
      page: 'budgets/:id/details',
    })
  })

  it('is gated by ANALYTICS=false but the dataLayer push survives', () => {
    window.econumoConfig = { ANALYTICS: false }
    trackEvent(METRICS.TRANSACTION_CREATE)
    expect(capture).not.toHaveBeenCalled()
    expect(window.dataLayer).toHaveLength(1)
  })
})

describe('scrubbedPage', () => {
  it.each([
    ['/accounts', 'accounts'],
    ['/budgets/01980e2c-1111-7000-8000-123456789abc', 'budgets/:id'],
    [
      '/budgets/01980E2C-1111-7000-8000-123456789ABC/tags/01980e2c-2222-7000-8000-123456789abc',
      'budgets/:id/tags/:id',
    ],
    ['/', ''],
  ])('%s -> %s', (path, expected) => {
    expect(scrubbedPage(path)).toBe(expected)
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && pnpm test src/lib/metrics.test.ts`
Expected: FAIL (`scrubbedPage` not exported; `capture` not called)

- [ ] **Step 3: Implement**

In `web/src/lib/metrics.ts` — replace the imports line with:

```ts
import { analyticsDomain, capture } from './analytics'
import { analyticsEnabled, getVersion, locale, selfHosted } from './config'
```

Add above `trackEvent`:

```ts
const UUID_RE = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi

// Route with UUID segments templated to ":id": no instance data may ride
// along on an analytics event.
export function scrubbedPage(pathname: string): string {
  return pathname.substring(1).replace(UUID_RE, ':id')
}
```

Append inside `trackEvent`, after the existing `window.dataLayer.push({...})` call (which stays byte-identical):

```ts
  if (analyticsEnabled()) {
    const domain = analyticsDomain()
    capture(metric, {
      domain,
      selfHosted: domain === 'self-hosted',
      locale: locale(),
      version: getVersion(),
      page: scrubbedPage(window.location.pathname),
    })
  }
```

- [ ] **Step 4: Run the full frontend suite**

Run: `cd web && pnpm test && pnpm lint && pnpm exec tsc -b`
Expected: all PASS (tsc catches any type slip vitest/oxlint miss)

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/metrics.ts web/src/lib/metrics.test.ts
git commit -m "feat(web): send product events to PostHog from trackEvent"
```

---

### Task 6: Docs + final verification

**Files:**
- Modify: `.env.example` (after the `ECONUMO_CHECK_UPDATES` block)
- Modify: `CLAUDE.md` (Configuration section, after the `ECONUMO_CHECK_UPDATES` line)

**Interfaces:** none (docs only).

- [ ] **Step 1: Document the env var**

In `.env.example`, after the `ECONUMO_CHECK_UPDATES` block:

```
# Optional: anonymous product analytics (PostHog). Each event carries only the
# event name, app version, UI language, a UUID-scrubbed route, and a
# "self-hosted" marker — no account data, no hostnames, no cookies, and client
# IPs are discarded at ingestion. Set false to disable for the whole instance.
# ECONUMO_ANALYTICS=false
```

In `CLAUDE.md`, after the `ECONUMO_CHECK_UPDATES` bullet:

```
- `ECONUMO_ANALYTICS` — anonymous product analytics from the SPA to PostHog (default `true`).
  `false` disables it instance-wide; the server surfaces the flag by appending a
  `window.econumoConfig.ANALYTICS = <bool>;` line to the served `/econumo-config.js`.
  Malformed values fail at boot (strict parse, unlike the other booleans).
```

- [ ] **Step 2: Full verification**

Run: `make go-test && cd web && pnpm test && pnpm lint && pnpm exec tsc -b`
Expected: all PASS

- [ ] **Step 3: Commit**

```bash
git add .env.example CLAUDE.md
git commit -m "docs: ECONUMO_ANALYTICS env var"
```

---

## Out of scope (manual, PostHog project side)

Recorded in the spec (Part 7): enable "Discard client IP data", keep autocapture/session replay off. Nothing in this repo enforces those.
