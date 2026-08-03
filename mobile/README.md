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
