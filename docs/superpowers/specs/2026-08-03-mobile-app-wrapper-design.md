# Mobile app wrapper (Capacitor) — design

**Date:** 2026-08-03
**Status:** approved design, pending implementation plan

## Goal

Ship the existing React SPA to the Apple App Store and Google Play as a
native-packaged app, as an additional distribution channel for Econumo. One
frontend codebase: `web/` stays the single SPA; the app is a thin Capacitor
shell around its production build. The app connects to Econumo Cloud by
default and to any self-hosted instance via the existing custom-server
toggle on the login page.

## Non-goals

- No native rewrite. The SwiftUI prototype on the `ios` branch is superseded;
  the branch is deleted as part of this work.
- No service worker / PWA hardening (worthwhile later, independent of this).
- No push notifications in v1.
- No changes to the Go backend. The wrapper works against released backends
  as they are.

## Architecture

A new top-level `mobile/` directory holds the Capacitor project:

```
mobile/
├── package.json          # Capacitor CLI + plugins (pnpm, like web/)
├── capacitor.config.ts   # appId, webDir: ../web/dist
├── ios/                  # generated native project (committed)
└── android/              # generated native project (committed)
```

- The native projects are committed, per Capacitor convention (they carry
  icons, splash assets, entitlements, and store metadata).
- `webDir` points at `web/dist`; `npx cap sync` copies the built SPA into
  both native projects.
- Makefile targets: `mobile-install` (pnpm install), `mobile-sync`
  (web build + `cap sync`), `mobile-ios` / `mobile-android` (sync + open in
  Xcode / Android Studio).
- App id: `com.econumo.app` on both platforms.

## Platform detection

`web/src/lib/platform.ts` exposes `isNativeApp(): boolean`, implemented by
probing the `window.Capacitor` global (`isNativePlatform()`), which the
Capacitor runtime injects. No new dependency in `web/`; the web build's
behavior is unchanged byte-for-byte. Every app-specific behavior below
branches on this one helper.

## Backend connection

In app mode:

- **Custom API is always allowed.** `isCustomApiAllowed()`
  (`web/src/lib/config.ts`) returns `true` when `isNativeApp()`, regardless
  of config. The login page's existing custom-server section appears for
  every app user.
- **Default host is Econumo Cloud.** `backendHost()`'s fallback becomes
  `https://app.econumo.com` instead of `window.location.origin` (which is a
  meaningless `capacitor://localhost` inside the WebView). Self-hosted users
  flip the existing toggle and enter their URL, exactly as on the web.
- **Networking goes through CapacitorHttp.** The `CapacitorHttp` plugin
  option patches `XMLHttpRequest`, so axios requests execute on the native
  HTTP stack. Requests then carry no browser origin, which removes CORS from
  the picture entirely — self-hosted servers need zero configuration
  (`ECONUMO_CORS_ALLOW_ORIGIN` stays irrelevant).

## Server-owned config at runtime

On the web, the Go server merges instance truth into the served
`/econumo-config.js`. The app bundles a static copy, so in app mode the SPA
fetches `${backendHost()}/econumo-config.js` at startup and after a server
switch, and applies a fixed allowlist of keys:

- Merged: `ALLOW_REGISTRATION`, `ANALYTICS`, `VERSION`.
- Every other key keeps its bundled default.

The fetch is non-fatal: on failure (offline boot, old backend) the app keeps
bundled defaults and cached data. `VERSION` here is the **server's** version,
used for the skew warning below; the UI's own version label stays the app
bundle's.

## Native adaptations

Each sits behind `isNativeApp()` at an existing choke point:

- **Durable credentials.** WKWebView may evict `localStorage` under storage
  pressure. The token and backend host (the keys whose loss logs the user
  out or strands them on the wrong server) are mirrored to the Capacitor
  Preferences plugin (native storage) on every write, and restored into
  `localStorage` before first render. The splash screen covers the async
  restore. `web/src/lib/storage.ts` is the single write path today, so the
  mirror is one adapter.
- **CSV export.** `downloadBlob()` (`web/src/lib/download.ts`) routes to
  Filesystem (write to cache dir) + the native Share sheet in app mode;
  anchor-click blob downloads do nothing in WKWebView.
- **External links.** `target="_blank"` anchors and `window.open` calls open
  the system browser via the Capacitor Browser plugin instead of navigating
  the app WebView. A small helper wraps the few call sites (settings links,
  docs links, update notice).
- **401 handling.** The axios interceptor's
  `window.location.assign('/login?reason=expired')` becomes SPA router
  navigation. Full page loads to non-root paths are fragile in a packaged
  WebView, and router navigation is strictly better on the web too.
- **Android back button.** The App plugin maps hardware back to router
  back-navigation, minimizing the app at the navigation root.
- **Status bar & splash.** Standard StatusBar + SplashScreen plugins; the
  SPA already draws edge-to-edge with `env(safe-area-inset-*)` padding, so
  no layout work is expected.

## Version skew

The store app freezes a SPA build while users' backends vary — the first
time frontend and backend versions can diverge (they ship atomically in the
server binary today).

- The persisted-query-cache buster incorporates the app bundle's version, so
  an app update never restores a stale-shaped cache.

**Two-way handshake (amended 2026-08-03):** the app and the server check each
other in both directions, one hard floor per side:

- The app carries `MIN_SERVER_VERSION` (`web/src/lib/appConfig.ts`) — the
  oldest server it can talk to. An older server is incompatible and
  hard-blocks the app with an "update your server" gate.
- The server publishes `MIN_APP_VERSION` (constant
  `version.MinAppVersion`, merged into the served `econumo-config.js`) —
  the oldest app build it accepts. An older app hard-blocks itself with an
  "update the app" gate. Servers predating the key never block the app;
  the app-side floor governs them.
- Compatible-but-different versions get soft, dismissable banners instead:
  "update the server" when the server is older than the app, "update the
  app" when the server is newer.

The hard gate (`AppUpdateBlock`) mounts outside the router so it covers
every route, login included. Comparisons only fire on strict `vX.Y.Z`
values, so `dev` builds never block. Floors are documented in
`mobile/README.md` and bumped together with their constants.

## Store packaging & review

- Icons and splash screens generated from the existing icon set via
  `@capacitor/assets`.
- Privacy labels declare the PostHog product analytics (which continue to
  honor the server's `ANALYTICS` flag and the instance-wide opt-out, as on
  the web).
- Review notes include a demo account on Econumo Cloud so reviewers can
  exercise the app without registering.
- Guideline 4.2 (minimum functionality) is addressed by bundled assets plus
  the native integrations above — this is a packaged app, not a wrapped
  website.

## Testing

- Vitest units for the new seams, with mocked Capacitor globals: platform
  detection, the storage mirror/restore, the download bridge, the runtime
  config fetch and its allowlist.
- The existing web suite must pass unchanged — app-mode branches are dead
  code on the web.
- Manual smoke on the iOS Simulator and an Android emulator against both
  Econumo Cloud and a local backend (login, browse cached data offline,
  create a transaction, CSV export, server switch, Android back).

## Phasing

1. **Foundation** — `mobile/` scaffold, platform detection, cloud-default
   host, CapacitorHttp, storage mirror, runtime config fetch. Exit: the app
   runs end-to-end on both platforms against cloud and self-hosted.
2. **Native UX** — external links, CSV export/share, Android back, status
   bar/splash, 401 router navigation.
3. **Store packaging** — assets, privacy labels, demo account, TestFlight /
   internal testing tracks, submissions.

## Repo notes

- Delete the local `ios` branch (superseded native prototype; never pushed).
- Add a `mobile/` section to `CLAUDE.md` (structure, build commands, the
  app-mode config-allowlist rule) once implementation lands.
