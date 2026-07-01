# Runtime-configurable UI version label

**Date:** 2026-06-30
**Status:** Approved

## Problem

`ECONUMO_VERSION` (the version label shown in the SPA) is baked into the compiled
JS bundle at image-build time: the Dockerfile seds the build arg into `web/.env`,
Quasar inlines `process.env.ECONUMO_VERSION` via DefinePlugin, and
`web/src/modules/package.ts` reads it as a frozen constant. Changing the label on a
running container (e.g. a demo environment) therefore requires a full image rebuild.

## Goal

Allow the displayed version label to be overridden at runtime, without a rebuild, by
editing a file that ships alongside the SPA — while leaving the default behaviour
byte-identical for anyone who does not touch that file.

## Chosen approach

Reuse the existing runtime-config global `window.econumoConfig`, which is set by the
static, unbundled file `web/public/econumo-config.js` (copied verbatim to
`dist/spa/econumo-config.js` and loaded via `<script src="econumo-config.js">` in
`index.template.html` before the app boots). `config.ts` already reads
`ALLOW_REGISTRATION` and `PAYWALL_ENABLED` from this global, so a version field fits
the established pattern with no new fetch and no first-render race.

A separate `econumo-config.json` fetched at runtime was rejected: it adds a second
config mechanism and an async fetch that races with first render.

## Precedence

The version resolves as:

```
window.econumoConfig?.VERSION  ||  process.env.ECONUMO_VERSION
```

- `VERSION` absent, `null`, or empty → build-time value wins (current behaviour preserved).
- `VERSION` set in the served `econumo-config.js` → overrides live.

## Changes (frontend only)

1. **`web/src/modules/config.ts`**
   - Add `VERSION?: string` to the `EconumoConfig` interface.
   - Export `getVersion(): string` returning
     `window.econumoConfig?.VERSION || process.env.ECONUMO_VERSION`.

2. **`web/src/modules/package.ts`**
   - `getEditionLabel()` calls `config.getVersion()` instead of reading
     `process.env.ECONUMO_VERSION` directly.

3. **`web/public/econumo-config.js`**
   - Add `VERSION: null` as a documented placeholder. `null`/absent keeps the
     build-time fallback, so the default build behaves exactly as today.

No backend/Go changes. The `--build-arg ECONUMO_VERSION` path is unchanged and
continues to supply the fallback baked into the bundle.

## Operating it (the demo use case)

`econumo-config.js` is served from `/app/web/econumo-config.js`. To change the label
on a running distroless container (no shell, so copy a file in rather than edit in
place):

```bash
docker cp econumo-config.js <container>:/app/web/econumo-config.js
```

or bind-mount the file via compose. Then hard-refresh the browser.

## Caveat

`econumo-config.js` has no content-hash in its filename, so browsers/proxies may
cache it; a live edit may require a hard refresh / cache-bust to take effect. Out of
scope for this change. If it becomes a problem, add a `no-cache` response header for
that single file in the SPA handler (`internal/ui/spa/spa.go`).

## Testing

- No Go changes → no backend test impact.
- Manual: `make web-bundle`, then confirm that (a) setting `VERSION` in the built
  `econumo-config.js` changes the displayed label, and (b) removing/nulling `VERSION`
  falls back to the build-time `ECONUMO_VERSION`.
