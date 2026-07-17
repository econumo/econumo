# Release Notifications — Design

**Date:** 2026-07-17
**Status:** Approved

## Overview

Notify users of a self-hosted Econumo instance when a new release is available,
linking to the release page on the main website (`https://econumo.com/releases/<version>/`),
not GitHub.

Architecture (approved as "Approach A"): the website publishes a machine-readable
"latest release" JSON feed; the Go backend polls it and caches the result in
memory; the SPA reads the cached info through a small authenticated API endpoint,
compares against its own build version, and renders the notification surfaces.

Why backend-proxied rather than browser-direct: one phone-home per instance
(server → econumo.com) instead of one per browser — no per-user IP leakage —
and the env opt-out works naturally because the backend owns the check. The SPA
does the version comparison, so the Go binary never needs to know its own
version (today only the SPA has one, Vite-inlined at image build).

## Decisions

- **Audience:** all users of the instance see the notification.
- **Opt-out:** `ECONUMO_CHECK_UPDATES` (default `true`); `false` disables
  polling entirely — standard self-hosted etiquette.
- **Data source:** a new JSON feed on the website (`econumo/website` repo,
  Astro on GitHub Pages). GitHub Pages already serves everything with
  `Access-Control-Allow-Origin: *` and CDN caching.
- **`dev` / non-semver builds:** never show anything. A `dev` image is
  typically ahead of the latest tagged release, and there is no meaningful
  ordering between `dev` and a semver. The backend still polls; the SPA
  simply ignores the data until it runs a tagged build.
- **Failure is silence:** feed unreachable, invalid payload, check disabled,
  dev build, SPA query error — all render exactly nothing. No error states,
  no retry UI.

## Part 1 — Website: the release feed (`econumo/website` repo)

New Astro endpoint `src/pages/releases/latest.json.ts` (same pattern as the
existing `search-data.json.ts`), served at
`https://econumo.com/releases/latest.json`:

```json
{ "version": "v1.0.2", "date": "2026-07-16", "url": "https://econumo.com/releases/v1.0.2/" }
```

Generated at build time from the releases content collection — newest by
semver. No other website changes. **Ships first**; the app feature is inert
without it.

## Part 2 — Backend: poller + API endpoint

New feature package `internal/system` (auto-covered by archtest; imports only
`model`/`shared`/`web` — no sibling features).

### Service

`system.Service` holds the latest-release info in memory (mutex-guarded):

- **Poller** — `StartPolling(ctx)`, launched from the `serve` command only
  (the first in-process background job; CLI commands and tests never start it,
  so test responses stay deterministic). Fetches the feed on boot, then every
  24 h, with a 10 s HTTP timeout. A failed or invalid fetch logs at DEBUG and
  keeps the previous value.
- **Read use case** — returns the cached info; empty when disabled, never
  fetched, or nothing valid received yet.

### Feed validation (trust boundary)

The feed is remote input rendered as a trusted link in every instance's UI.
A payload is accepted only if:

- `version` matches `^v\d+\.\d+\.\d+$`, and
- `url` starts with `https://econumo.com/`.

Anything else is dropped (previous value retained). This prevents a
compromised feed from injecting an arbitrary phishing URL as an "update" link.

### Endpoint

`GET /api/v1/system/get-update-info` — authenticated, standard success
envelope:

```json
{ "success": true, "message": "", "data": { "version": "v1.0.2", "url": "https://econumo.com/releases/v1.0.2/" } }
```

`version` and `url` are empty strings when nothing is known. Request/result
DTOs live in `internal/model/system_dto.go`; routes in
`internal/system/api/routes.go` per the standard handler pattern.

### Config

- `ECONUMO_CHECK_UPDATES` — bool, default `true`; parsed in `config.Load` via
  the repo's lenient `getBool` (malformed values fall back to the default
  `true`, consistent with `ECONUMO_DEBUG` / `ECONUMO_ALLOW_REGISTRATION` and
  the repo's other boolean flags — it does not fail at boot).
- The feed URL is a constant; tests override it via the service constructor
  (no env knob).

## Part 3 — Frontend: query, comparison, surfaces

### Data

- `web/src/api/system.ts` — typed client for `get-update-info`.
- `useUpdateInfo` TanStack Query hook — fetched once per session (long
  `staleTime`, no window-focus refetch).
- `isNewerVersion(latest, current)` — pure helper; returns `true` only when
  **both** strings match the existing `SEMVER` pattern and `latest` is
  semver-greater than `current`. `dev`, empty strings, and malformed input
  all yield `false`.

### Surfaces (all driven by the same hook)

1. **Settings gear badge** — a small dot on the Settings gear in the sidebar
   footer (`ApplicationLayout`), visible on desktop and on mobile home.
   Persistent while an update is pending; not dismissible.
2. **Sidebar notice (dismissible)** — a compact one-liner above the sidebar
   footer: "Econumo v1.0.2 is out", linking to the release URL (new tab),
   with a close button. Dismissal stores the version in `localStorage`
   (`econumo.dismissed-update-version`); the notice stays hidden for that
   version and reappears for the next one.
3. **Settings page row** — a primary-tinted card row at the top of the
   `SettingsPage` menu (between the user card and the "Finances" group):
   "New version v1.0.2 available", linking to the release URL (new tab).
   Persistent, not dismissible.

### i18n

New keys in `locales/{en,ru}.json` (e.g. `settings.update.available` with a
`{version}` placeholder). The i18ntest guards enforce en/ru key parity,
placeholder parity, and `t()`-call coverage automatically.

## Testing

- **Go:** unit tests for the poller against an `httptest` feed (valid,
  malformed JSON, bad version format, wrong-origin URL, non-2xx); handler
  test; apiparity scenario + golden for the new route (deterministic empty
  response, since tests never start the poller); `make go-test` green.
- **Web:** vitest for `isNewerVersion` and the dismissal logic; component
  tests for the three surfaces (shown when newer, hidden when current / dev /
  dismissed); `pnpm exec tsc -b` must pass.
- **Website:** build the site and verify the generated
  `releases/latest.json` output.

## Rollout order

1. `econumo/website`: add and deploy the feed.
2. This repo: backend package + endpoint, then frontend surfaces (one branch,
   one PR).
