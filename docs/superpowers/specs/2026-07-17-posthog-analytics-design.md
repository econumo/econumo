# PostHog Product Analytics — Design

**Date:** 2026-07-17
**Status:** Approved

## Overview

Wire the SPA's existing product-event stream to PostHog Cloud (US) so feature
usage can be understood across BOTH the cloud edition and self-hosted installs
— anonymously, with no cookies or browser storage, no consent banner, and a
single instance-level opt-out env var for self-hosters (enabled by default).

The instrumentation already exists: `web/src/lib/metrics.ts` defines ~100
product events and `trackEvent()` is called throughout the features, pushing
GTM-style objects into `window.dataLayer` (consumed by the liltag tag loader —
an empty tag list on self-hosted installs, so those events currently go
nowhere). This design adds a second consumer inside `trackEvent()`: a tiny
hand-rolled PostHog capture client. The dataLayer/liltag path is left exactly
as-is.

Approved as "Approach A": a ~60-line batching sender owned by this repo, not
the `posthog-js` SDK. PostHog ingestion is a plain HTTP endpoint; since only
anonymous custom events are needed, a hand-rolled client means no cookies and
no PII **by construction** — only the whitelisted payload built here ever
leaves the browser — with no SDK bundle weight (~50 KB gzip) and no risk of an
SDK upgrade or config drift re-enabling autocapture in a finance app.

## Decisions

- **Granularity: anonymous events.** Random in-memory `distinct_id` per page
  load; never linked to a real user, never persisted. No personal data → no
  consent banner, no cookie machinery. Events carry
  `$process_person_profile: false` (PostHog anonymous mode: no person
  profiles, cheaper per-event billing).
- **Delivery: browser → PostHog directly** (`https://us.i.posthog.com`).
  Accepted trade-offs: end-user browsers contact posthog.com, and adblockers
  will eat a share of events. Mitigation for IPs: the PostHog project setting
  "Discard client IP data" (see checklist).
- **Opt-out: instance-level env var only.** `ECONUMO_ANALYTICS` (default
  `true`), same pattern as `ECONUMO_CHECK_UPDATES`. No per-user UI toggle, no
  DNT handling.
- **Editions: both, one pipeline.** PostHog is THE product-analytics path for
  cloud and self-hosted alike, segmented by a `domain` property. liltag/GTM
  stays untouched for whatever the cloud edition does with tags.
- **Edition detection is derived, not the legacy flag.** The existing
  `selfHosted()` in `config.ts` is a localStorage value set by the "use my own
  server" toggle on the login/registration pages — on a normal bundled install
  nobody flips it, so it reports `false`. Analytics instead derives from the
  serving origin: hostname is `econumo.com` or a subdomain → cloud, anything
  else → self-hosted.
- **PostHog project: existing PostHog Cloud US project.** The public project
  API key (`phc_…`) is baked into the source as a constant (it is public by
  design). Key to be supplied at implementation time.

## Part 1 — Frontend: capture client (`web/src/lib/analytics.ts`, new)

The only file that knows PostHog exists.

- Constants: `POSTHOG_HOST = 'https://us.i.posthog.com'`, `POSTHOG_KEY =
  'phc_…'`.
- `capture(event, properties)`: appends to an in-memory queue. Flush when the
  queue reaches 10 events or on a ~10 s timer, as one `fetch` POST to
  `${POSTHOG_HOST}/batch/` with `keepalive: true`. On
  `visibilitychange → hidden`, flush the remainder via
  `navigator.sendBeacon` so tab-closes don't lose the tail.
- `distinct_id`: `crypto.randomUUID()`, generated once per page load, held in
  a module variable. Never written to cookie / localStorage / sessionStorage —
  no cross-visit tracking, hence no persistent online identifier.
- Every event carries `$process_person_profile: false`.
- Queue capped at ~100 events (oldest dropped); a failed flush is dropped
  silently — no retries, no console noise, analytics can never break the app.
- Entirely disabled when `import.meta.env.DEV` (local `pnpm dev` must not
  pollute production data).

### Edition / domain derivation (in `analytics.ts`)

- `domain`: `window.location.hostname` when it is `econumo.com` or a
  subdomain (`hostname === 'econumo.com' || hostname.endsWith('.econumo.com')`);
  otherwise the literal string `"self-hosted"`. A real hostname is only ever
  sent when it is an econumo.com domain; self-hosters' hostnames never leave
  the browser.
- `selfHosted`: boolean, `domain === 'self-hosted'` — kept alongside as the
  natural PostHog filter/breakdown property.

## Part 2 — Frontend: wiring (`metrics.ts`, `config.ts`)

- `trackEvent()` keeps its `window.dataLayer` push byte-identical, and
  additionally calls `capture(metric, context)` when `analyticsEnabled()`.
- Event properties (the complete whitelist — nothing else is ever sent):
  - `domain` / `selfHosted` (derived above),
  - `locale` (UI language),
  - `version` (`ECONUMO_VERSION`, Vite-inlined),
  - `page`: the current path with UUID-looking segments replaced by `:id`
    (e.g. `budgets/0198…` → `budgets/:id`) so no instance data rides along.
- `config.ts`: new `analyticsEnabled(): boolean` returning
  `window.econumoConfig.ANALYTICS !== false` — absent/unknown fails open to
  enabled (a stale hand-hosted config file keeps the default), and the
  `ANALYTICS` key joins `EconumoConfig` + `web/public/econumo-config.js`.

## Part 3 — Backend: opt-out plumbing

- `config.Load`: new `ECONUMO_ANALYTICS` env var, default `true`; malformed
  value fails at boot (consistent with the other booleans).
- SPA handler (`internal/web/spa`): `Handler` takes a generic map of
  server-owned config keys and serves `GET /econumo-config.js` as the dist
  file plus one merge line,
  `Object.assign(window.econumoConfig, {"ALLOW_REGISTRATION":…,"ANALYTICS":…});`
  (built once at construction; `encoding/json` sorts map keys, so the output
  is deterministic). The router wires `ANALYTICS` and `ALLOW_REGISTRATION` —
  the latter fixes a pre-existing drift where `ECONUMO_ALLOW_REGISTRATION=false`
  still showed the SPA's Register UI (the static file said `true`). Two more
  keys merge tri-state — `ECONUMO_API_URL` and `ECONUMO_ALLOW_CUSTOM_API`
  override `API_URL` / `ALLOW_CUSTOM_API` only when explicitly set, so a
  file-configured deployment is never clobbered by a default. Future
  server-owned keys are one map entry; keys the server does not own
  (`VERSION`, `PAYWALL_ENABLED`) stay whatever
  the dist file says. Existing `Cache-Control: no-cache` is kept, so a `.env`
  flip takes effect on the next page load. If the file is missing (broken
  image) the route 404s exactly as today.
- Anyone hosting the SPA outside the binary (separate static host + API_URL)
  sets `ANALYTICS: false` directly in their own `econumo-config.js`.

## Part 4 — Error handling

- `capture()` is fire-and-forget; flush wraps `fetch` in a swallow-all catch.
- Failed batches are dropped — no retry queue (no memory growth, no request
  storms behind corporate proxies).
- An adblocker blocking `us.i.posthog.com` is indistinguishable from a failed
  fetch: silently dropped, app unaffected. Accepted per the delivery decision.

## Part 5 — Testing

- **Frontend (vitest):**
  - `analytics.test.ts`: batch payload shape against the `/batch/` contract
    (project key, `distinct_id` stability within a page load,
    `$process_person_profile: false`); flush on threshold / timer / page-hide
    (fake timers, mocked `fetch` + `sendBeacon`); queue cap; silent failure on
    fetch rejection; disabled path short-circuits before queueing.
  - `metrics.test.ts` additions: UUID-scrubbed `page`; `domain`/`selfHosted`
    derivation for `econumo.com`, `app.econumo.com`, and a third-party host;
    `ANALYTICS: false` gates the PostHog call but not the dataLayer push.
- **Backend (Go):**
  - `config` tests: default true, explicit `false`, malformed value → boot
    error.
  - `spa_test.go`: `/econumo-config.js` response ends with the override line
    in both states, keeps `no-cache`, and still 404s when the file is absent.
  - No API route changes → no apiparity/golden churn.

## Part 6 — Docs & self-hoster communication

- `.env.example`: commented `# ECONUMO_ANALYTICS=false` block stating exactly
  what is collected (anonymous product events: event name, app version,
  language, scrubbed route, `"self-hosted"` marker — no account data, no
  hostnames, no IP retention) and that setting `false` disables it.
- README/docs privacy note with the same content, alongside the existing
  `ECONUMO_CHECK_UPDATES` documentation.

## Part 7 — PostHog project checklist (manual, one-time)

Settings on the PostHog side that code cannot enforce; recorded here so they
aren't tribal knowledge:

- Enable **"Discard client IP data"** on the project (transport IPs are never
  stored).
- Leave autocapture and session replay **off** for the project.
- Note the project API key and bake it into `analytics.ts`.

## Non-goals

- No per-user identification (`posthog.identify`), retention cohorts, or
  cross-visit journeys.
- No session replay, feature flags, surveys, or autocapture.
- No server-side event emission or relay.
- No removal of liltag / the dataLayer mechanism.
- No per-user opt-out UI, no DNT/GPC handling.
