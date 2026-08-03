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

## Minimum supported server version

**v1.3.0** — mirrored by `MIN_SERVER_VERSION` in `web/src/lib/appConfig.ts`.
Older servers show a non-blocking warning banner in the app. Bump both
together when the bundled SPA starts depending on newer API behavior.

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
