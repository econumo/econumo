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

## Signing (one-time)

The App target uses automatic signing; the Apple Developer Team ID is
deliberately NOT committed. On a new machine:

1. Sign into the Apple ID in **Xcode → Settings → Accounts** — automatic
   signing then creates the iOS development certificate and provisioning
   profiles on demand (`-allowProvisioningUpdates` does the same for
   command-line builds).
2. Put your Team ID into the git-ignored `mobile/ios/local.xcconfig`
   (used by Debug/device builds via `debug.xcconfig`'s optional include):

       DEVELOPMENT_TEAM = XXXXXXXXXX

3. TestFlight archives take the team from the `APPLE_TEAM_ID` environment
   variable instead (Release builds don't read `debug.xcconfig`).

## Install on your iPhone (cable)

1. Connect the phone via USB and tap **Trust** on it.
2. `make mobile-ios`, select the phone as the run destination, press Run.
3. First run only: the phone asks to enable **Developer Mode**
   (Settings → Privacy & Security → Developer Mode, then reboot), and the
   app must be trusted under Settings → General → VPN & Device Management.

A development-signed install expires after 7 days on a free Apple ID; with
the paid Developer Program it lasts a year — or use TestFlight below and
forget about expiry.

## TestFlight

One-time setup:

1. Xcode signed into the Apple ID (see Signing above).
2. Create the app record once in [App Store Connect](https://appstoreconnect.apple.com/)
   → My Apps → **+** → New App: platform iOS, bundle ID `com.econumo.app`
   (automatic signing registers the identifier on the developer portal during
   the first device build; if it is not listed yet, add it under
   Identifiers on developer.apple.com), any SKU, name "Econumo".
3. In the app's TestFlight tab, add yourself (and any teammates) as internal
   testers — internal testing needs no Apple review.

Each upload:

    APPLE_TEAM_ID=XXXXXXXXXX make mobile-testflight APP_VERSION=1.0.0 BUILD=1

This archives with the given marketing version + build number (BUILD must be
unique per upload — bump it every time), stamps the SPA's version label to
match, and uploads straight to App Store Connect
(`ExportOptions.plist`: method `app-store-connect`, destination `upload`).
After a few minutes of processing the build appears in TestFlight and
installs on the phone through the TestFlight app.

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
- Privacy labels: declare Twillingate product analytics, identified (a hashed
  user id and a per-instance group) — each user can opt out in Settings →
  Profile; no ad tracking.
- Review notes: include a demo account on Econumo Cloud so reviewers can
  sign in without registering.
- Release builds: `ECONUMO_VERSION=vX.Y.Z make mobile-sync`, then archive
  from Xcode (iOS) / build an AAB from Android Studio.

## Sign-in with Google / Apple / SSO

OAuth sign-in opens the backend's authorization URL in the system browser
(via `@capacitor/browser`). The browser performs the OAuth handshake with the
provider (Google, Apple, etc.) and the backend redirects back to the app over
one of two return shapes:

- **The verified app link** `https://<backend>/oauth/app-return…` — preferred,
  and the only shape a look-alike app cannot claim. The backend emits it when
  its `ECONUMO_APP_LINKS_IOS` / `ECONUMO_APP_LINKS_ANDROID` are set (they also
  make it serve the two association documents:
  `/.well-known/apple-app-site-association` and
  `/.well-known/assetlinks.json`).
- **The private URL scheme** `com.econumo.app://oauth…` — the fallback when the
  backend has no app links configured.

Both carry the same parameters: `handoff` / `linkHandoff` in the fragment,
`error` / `linkError` in the query. The SPA dispatch handler
(`web/src/lib/deepLinks.ts`) accepts either shape and resolves the handoff into
a session or renders the error.

Claiming the app link needs one step per platform:

- **iOS** — Xcode → the App target → Signing & Capabilities → **+ Capability**
  → **Associated Domains**, then add `applinks:app.econumo.com`. The entitlement
  lives in the Xcode project, not in `Info.plist`; iOS fetches the AASA through
  Apple's CDN, so allow a few minutes after a backend change. The Team ID +
  bundle id pair (`ABCDE12345.com.econumo.app`) must be listed in the backend's
  `ECONUMO_APP_LINKS_IOS`.
- **Android** — `AndroidManifest.xml` already carries the `autoVerify` VIEW
  intent filter for `https` host `app.econumo.com`, path prefix
  `/oauth/app-return`. Verification matches the app's **signing** certificate,
  so the backend's `ECONUMO_APP_LINKS_ANDROID` must list the SHA-256 fingerprint
  Play App Signing shows (Play Console → Setup → App integrity), plus the upload
  certificate's fingerprint if you also want locally built/uploaded APKs to
  verify. Check a device with `adb shell pm get-app-links com.econumo.app`.

The private scheme stays registered in both projects (iOS `CFBundleURLTypes`,
Android a second VIEW intent filter with scheme `com.econumo.app` host `oauth`)
because a **self-hosted** backend can never be an association domain of the
store app: its host is not in the app's entitlement, so app links cannot work
there and the scheme is the only way back. Residual risk, documented: on the
scheme any app installed on the device may register the same URL and receive the
redirect. The handoff is single-use, short-lived and only exchangeable by the
client that began the flow (it must present the flow secret), which keeps the
damage to a denied sign-in rather than a stolen session.

When neither shape reaches the app (app not installed, or an unverified
association), the browser simply lands on the SPA page `/oauth/app-return`,
which offers a single "Open the app" link on the private scheme.

The backend's OAuth callback URL is always its own `/api/v1/oauth/callback-<provider>`
(Google, Apple, etc.) — no per-backend configuration needed. App Store rule 4.8
requires the cloud backend to enable Apple sign-in whenever Google is enabled.
