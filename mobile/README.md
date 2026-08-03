# Econumo mobile (Capacitor)

A thin native shell around the web SPA (`web/`). See
`docs/superpowers/specs/2026-08-03-mobile-app-wrapper-design.md`.

## Build

    make mobile-install   # once: pnpm install here
    make mobile-sync      # build web/ and copy into both native projects
    make mobile-ios       # sync + open in Xcode
    make mobile-android   # sync + open in Android Studio

Release builds label the UI with the app version:

    ECONUMO_VERSION=v1.0.0 make mobile-sync

## Version compatibility handshake

The app and the server check each other in both directions; each side owns
one hard floor. **Both floors live in a single file, `compat/versions.json`**
(embedded by the Go backend, imported by the SPA — the same shared-file
pattern as `locales/`), so there is exactly one place to bump:

- **App-side floor** (`minServerVersion`): the oldest server the bundled app
  can talk to — bump when the SPA starts depending on newer API behavior. A
  server older than this hard-blocks the app ("update your server").
- **Server-side floor** (`minAppVersion`): the oldest app build the backend
  accepts, served as `MIN_APP_VERSION` in `econumo-config.js` — bump only
  when a release breaks compatibility with older app builds. An app older
  than this hard-blocks itself ("update the app"). Servers predating the key
  never block the app — the app-side floor governs them.
- **Soft tier:** compatible-but-outdated pairs get dismissable nudges. A
  server older than the app shows the `ServerVersionNotice` banner. An
  outdated app is covered by the pre-existing release notice (`UpdateNotice`
  in the sidebar): in app mode `getVersion()` is the app build's version, so
  the "Econumo {version} is out" block with its release-notes link fires
  whenever the app trails the latest published release — no app-specific
  surface needed.

Both hard cases render `AppUpdateBlock` (a full-screen gate over every
route). Version checks only fire on strict `vX.Y.Z` values, so `dev` builds
never block.

## Plain-HTTP self-hosted servers

Cleartext traffic is deliberately enabled on both platforms
(`android:usesCleartextTraffic="true"` in the Android manifest,
`NSAppTransportSecurity` / `NSAllowsArbitraryLoads` in `Info.plist`) so
user-configured self-hosted servers on a LAN work without TLS, with zero
configuration. For App Store submission, justify `NSAllowsArbitraryLoads` in
App Review notes as: "the app connects to user-configured self-hosted
servers" — standard for self-hosting clients.

## Store submission checklist

- App id `com.econumo.app` on both stores.
- Privacy labels: declare PostHog product analytics (disabled instance-wide
  when the server sets `ECONUMO_ANALYTICS=false`); no ad tracking.
- Review notes: include a demo account on Econumo Cloud so reviewers can
  sign in without registering.
- Release builds: `ECONUMO_VERSION=vX.Y.Z make mobile-sync`, then archive
  from Xcode (iOS) / build an AAB from Android Studio.
