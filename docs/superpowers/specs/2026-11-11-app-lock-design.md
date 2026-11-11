# App lock (PIN + biometrics) — design

Date: 2026-11-11. Status: approved for implementation. **Revision 1 (end of file) overrides
anything above it that it contradicts.**

## Problem

Someone who picks up the owner's unlocked phone (or sits at their unlocked
computer) can open Econumo and read every balance and transaction. The owner
wants a lock in front of the app: a PIN, optionally Face ID / Touch ID /
fingerprint.

## Threat model and non-goals

- **In scope:** a person with physical access to an unlocked device who uses
  the app's UI (taps, types). The lock must hide all financial data from them
  and must not be bypassable through the UI (reload, back button, deep links,
  opening a new tab, the app switcher preview as far as the platform allows).
- **Out of scope:** an attacker with developer tools or file access to the
  browser profile. The lock is frontend-only: the session token, the
  persisted query cache (localStorage) and the transaction store (IndexedDB)
  stay unencrypted, and the lock's own configuration lives in localStorage
  where such an attacker can delete it. Encrypting local data with a
  PIN/passkey-derived key is a possible follow-up, not part of this change.
- No server changes. No API, migration, or Go code. The lock is a per-device
  preference and is never sent to the server.

## User-visible behaviour

### Settings

Profile settings (`/settings/profile`) get a new row **App lock** (after
"Sessions"/before "Personal tokens" in the security list) showing `On`/`Off`,
linking to a new page `/settings/profile/app-lock` (`RouterPage.SETTINGS_APP_LOCK`,
`AppLockPage`, inside `SettingsShell` with `editableOffline` — every control is
local, so it all works offline).

The page:

- **App lock** switch. Turning it on opens the *Set PIN* flow: enter a PIN of
  4–8 digits, then enter it again to confirm (mismatch: inline error, start
  over). Cancelling leaves the lock off. Turning it off asks for the current
  PIN (or a biometric check when biometrics are on) first.
- When on:
  - **Change PIN**: current PIN, then new PIN + confirm.
  - **Unlock with Face ID / fingerprint** switch (label
    "Use biometrics", hint "Face ID, Touch ID or fingerprint, when the device
    supports it"). Shown only when
    `PublicKeyCredential.isUserVerifyingPlatformAuthenticatorAvailable()`
    resolves `true`, and never in the native Capacitor app (WebAuthn is not
    available for the app's WebView origin). Turning it on registers a
    platform passkey (see Biometrics) — if the user cancels or it fails, the
    switch stays off with a toast. Turning it off just forgets the credential
    id locally (the passkey itself stays in the OS keychain; the hint says so:
    "You can delete the passkey in your device's password settings").
  - **Lock after**: Immediately / 1 minute (default) / 5 minutes / 15 minutes
    in the background.
- A short explanatory paragraph: the lock protects this device only; forgetting
  the PIN means signing in again; it doesn't encrypt data on the device.

The PIN entry used in settings is the same `PinPad` component as the lock
screen (dots + numeric keypad + physical keyboard digits/Backspace/Enter).

### When the app locks

The lock applies only when the device has a lock configuration **for the
signed-in user** (`config.userId === getSessionUserId()`) and a session token.

- **Cold start:** every full page load (opening the PWA, a reload, a new tab)
  starts locked.
- **Background:** when the page becomes hidden (`visibilitychange` →
  `hidden`), the time is recorded. When it becomes visible again and
  `now - hiddenAt >= lockAfter`, the app locks. "Immediately" (0) locks on
  hide.
- **Privacy cover:** whenever the page is hidden and the lock is configured,
  an opaque full-screen cover (logo only) is drawn over the app, *without*
  unmounting it, so the OS app-switcher snapshot is less likely to show data
  (best-effort: the platform may snapshot before the repaint). On visible, the
  cover goes away unless the app locked.
- Unlock state is per tab (in memory). Other tabs are not unlocked by
  unlocking one.

### Lock screen

While locked, `RequireAuth` renders `LockScreen` **instead of** its `<Outlet />`,
so the application layout, every page, dialog and toast is unmounted and no
financial data is in the DOM. (Unmounting is deliberate: an overlay would fight
Radix dialog focus traps. Unsaved forms are lost on a lock; the transaction
dialog's draft restore already covers the main case.) Background machinery
(query-cache restore, pending-writes sender) keeps running.

The lock screen (full screen, centered, works on phone and desktop):

- Econumo logo, the user's avatar + name from the cached `get-user-data`
  (omitted if not cached), heading "Enter PIN".
- `PinPad`: one dot per PIN digit (the stored length; the PIN is submitted
  automatically when that many digits are entered), a 3×4 keypad
  (1–9, biometric button or blank, 0, backspace). Physical keyboard works.
- Wrong PIN: dots shake, cleared, "Wrong PIN" with remaining attempts once
  fewer than 5 remain.
- **Biometric button** (when enabled): calls the WebAuthn check. The check is
  also attempted automatically once each time the lock screen appears while
  the page is visible; browsers that require a user gesture (iOS Safari)
  reject that silently and the user taps the button.
- **Forgot PIN?** link → confirm dialog: "Sign out to reset the PIN? You'll
  need to sign in again. Changes not yet saved to the server will be lost."
  (the pending-writes count is mentioned when > 0). Confirm → navigate to
  `/logout` (which is outside `RequireAuth`).

### Attempt limits

Failed PIN attempts are counted in the lock config (persisted, so a reload
doesn't reset them):

- Attempts 1–4: no delay.
- From the 5th failure on, a lockout before the next try: 5th → 30 s, 6th →
  1 min, 7th → 5 min, 8th → 15 min, 9th → 15 min. The keypad is disabled with
  a countdown ("Try again in 0:30"). The lockout end is stored as a timestamp.
- The **10th** failure signs out (navigate to `/logout`, wiping local data).
- A successful PIN or biometric unlock resets the counter.

### Sign-out and user changes

- `LogoutPage` removes the lock configuration, with the exceptions in Revision 1 (R9).
- A config whose `userId` differs from the session's user is ignored and
  deleted at boot / on login (shared computer: user A's PIN never locks user B).

## Technical design

All code under `web/src/features/appLock/`.

### `config.ts` — persisted configuration

localStorage key `econumo.appLock`, JSON:

```ts
interface AppLockConfig {
  v: 1
  userId: string
  pin: { salt: string; hash: string; iterations: number; length: number } // base64
  lockAfterMs: number            // 0 | 60_000 | 300_000 | 900_000
  biometric: { credentialId: string } | null // base64url
  failedAttempts: number
  lockedUntil: number | null     // epoch ms
}
```

`readConfig()` (validates shape; malformed → null), `writeConfig()`,
`clearConfig()`, `activeConfig()` = config whose userId matches
`getSessionUserId()` and a token exists. A `storage` event listener lets other
tabs react to changes (see Revision 1, R1: a storage event never unlocks).

### `pin.ts` — hashing

PBKDF2-SHA256 via `crypto.subtle`, 16-byte random salt, 150 000 iterations
(stored per config so it can change), 32-byte output, constant-time compare of
the derived bytes. `hashPin(pin)` / `verifyPin(pin, stored)`. PIN validation:
`/^\d{4,8}$/`. The hash avoids storing the PIN in clear; it is not a defence
against an offline attacker (a 4–8 digit space is small), which is outside the
threat model.

### `biometric.ts` — WebAuthn

- `biometricAvailable()`: not native app, `window.PublicKeyCredential` exists,
  secure context, and `isUserVerifyingPlatformAuthenticatorAvailable()` is true.
- `registerBiometric(user)`: `navigator.credentials.create` with
  `rp: { name: 'Econumo' }` (rp id defaults to the current host),
  `user: { id: random 16 bytes, name: user email or name, displayName: name }`,
  random 32-byte challenge, `pubKeyCredParams` ES256 (-7) and RS256 (-257),
  `authenticatorSelection: { authenticatorAttachment: 'platform',
  userVerification: 'required', residentKey: 'discouraged' }`,
  `attestation: 'none'`, timeout 60 s. Returns the credential id (base64url).
- `verifyBiometric(credentialId)`: `navigator.credentials.get` with a random
  challenge, `allowCredentials: [{ type: 'public-key', id, transports:
  ['internal', 'hybrid'] }]`, `userVerification: 'required'`, timeout 60 s.
  Success requires: the promise resolves; `clientDataJSON.type ===
  'webauthn.get'`; its `challenge` equals the one sent; its `origin` equals
  `location.origin`; the authenticator data's UV flag (bit 2 of byte 32) is
  set. No signature verification (no server; an attacker who can tamper with
  the JS is out of scope). Returns `'ok' | 'cancelled' | 'failed'`
  (`NotAllowedError`/`AbortError` → cancelled).

### `lockStore.ts` — runtime state (zustand)

`{ locked: boolean, obscured: boolean }` plus actions:

- Initial `locked = activeConfig() !== null` (cold start).
- `lock()`, `unlock()`, `startVisibilityTracking()` (idempotent; installed once
  from `main.tsx` before render): on `hidden` → remember `hiddenAt`, set
  `obscured = true` if configured, and if `lockAfterMs === 0` lock at once; on
  `visible` → lock if `now - hiddenAt >= lockAfterMs`, then `obscured = false`.
  Also handles `pagehide`/`pageshow` with `persisted` (bfcache) like
  hidden/visible.
- Enabling the lock in settings does **not** lock the current tab; disabling
  unlocks it.
- `useAppLock()` hook exposes `locked`, `obscured`, `config`.

### Components

- `PinPad.tsx` — controlled digit entry (length, onComplete, disabled,
  error/shake state, optional biometric slot), accessible: keypad buttons have
  labels, an `aria-live` region announces errors; a visually hidden
  `inputMode="numeric"` input is NOT used (the keypad is the input on phones);
  keyboard input via a `keydown` listener while mounted.
- `LockScreen.tsx` — described above.
- `PrivacyCover.tsx` — fixed, `z-[1000]`, opaque `bg-background`, logo; rendered
  from `RootLayout` while `obscured && !locked`.
- `AppLockPage.tsx`, `SetPinDialog.tsx` (set / change / confirm-current flows,
  phone-friendly sheet or dialog using existing UI primitives).
- Gate: `RequireAuth` renders `<LockScreen />` instead of `<Outlet />` when
  `locked`.

### Analytics (`METRICS`)

Fire at success points, through the settings page actions / lock screen:

- `APP_LOCK_ENABLED` (`appAppLockEnabled`), `APP_LOCK_DISABLED`,
- `APP_LOCK_BIOMETRIC_ENABLED`, `APP_LOCK_BIOMETRIC_DISABLED`,
- `APP_LOCK_PIN_CHANGED`, `APP_LOCK_TIMEOUT_CHANGED` (`{ lockAfter: seconds }`),
- `APP_LOCK_UNLOCKED` (`{ method: 'pin' | 'biometric' }`),
- `APP_LOCK_FORGOT_PIN` (fired before navigating to `/logout`),
- `APP_LOCK_ATTEMPTS_EXCEEDED`.

Plus an analytics profile fact `app_lock`: `off` | `pin` | `biometric`
(`analyticsProfile.ts`). Events are never sent with PIN data.

### i18n

New keys under `appLock.*` (lock screen, settings page and set-PIN flow; literal keys only), in all 11 catalogues (respect each catalogue's
register: de Sie, nl u, fr vous, pt European). Plural strings use the pipe
format with `pluralPick`.

### Tests (vitest)

- `pin.test.ts`: hash/verify round trip, wrong PIN, salt differs per hash,
  validation.
- `config.test.ts`: read/validate/write/clear, `activeConfig` user matching.
- `lockStore.test.ts`: cold start locked iff config; visibility timing for
  0 / 60 s (fake timers + `document.visibilityState` stub); obscured flag;
  storage event removal unlocks.
- `biometric.test.ts`: stubbed `navigator.credentials` — success, wrong
  challenge, UV flag missing, cancel; availability false in native app.
- `LockScreen.test.tsx`: correct PIN unlocks, wrong PIN error, lockout
  countdown after 5 failures, 10th failure navigates to `/logout`, forgot-PIN
  confirm navigates to `/logout`, biometric button.
- `RequireAuth.test.tsx`: renders lock screen instead of outlet when locked;
  ignores another user's config.
- `AppLockPage.test.tsx`: enable flow (mismatch, success), change PIN, disable
  requires PIN, lock-after select, biometric row hidden when unavailable.
- `LogoutPage.test.tsx`: config cleared.
- Existing guard suites must pass (`metrics-coverage`, `offline-coverage`,
  `offline-notice-coverage`, i18n `t()` key coverage via `go test
  ./internal/test/i18ntest/`).

### Docs

- `docs/regression-test-plan.md` §12: new "App lock" items (📱 markers for the
  phone/PWA ones: cold start, background timeout, app-switcher cover, Face ID).
- `CLAUDE.md`: a short "App lock" paragraph under Frontend architecture.

## Implementation tasks

1. **Core** — `config.ts`, `pin.ts`, `biometric.ts`, `lockStore.ts` + tests;
   `startVisibilityTracking()` called from `main.tsx`.
2. **Lock screen** — `PinPad`, `LockScreen`, `PrivacyCover`, `RequireAuth`
   gate, `RootLayout` cover, `LogoutPage` clear, lock-screen i18n keys (all
   11 catalogues), metrics for unlock/forgot/exceeded, tests.
3. **Settings** — `AppLockPage`, `SetPinDialog`, route + `RouterPage`, Profile
   row, settings i18n keys, remaining metrics + analytics profile fact, tests,
   regression plan, CLAUDE.md.

## Revision 1 — spec review fixes (overrides the text above)

- **R1 Cross-tab.** A `storage` event never unlocks. If, in another tab, the
  lock config disappears or changes owner, or `token`/`sessionUserId` changes,
  a tab that is locked (or holds data) does `location.reload()` (it then boots
  locked or lands on `/login`). Settings changes from another tab (lockAfter,
  biometric) just take effect; nothing is cached — see R3.
- **R2 Count before checking.** On a PIN attempt (lock screen AND every
  settings prompt for the current PIN), re-read storage, write
  `failedAttempts + 1` and the resulting `lockedUntil` first, then derive and
  compare; on success reset the counter. A reload mid-check cannot buy free
  attempts.
- **R3 Storage layout.** Two keys: `econumo.appLock` (config: v, userId, pin,
  lockAfterMs, biometric) and `econumo.appLock.attempts`
  (`{ failedAttempts, lockedUntil }`). Every check/write re-reads
  localStorage; the zustand store holds only `locked`/`obscured`. The lock
  screen reads the PIN hash at verify time.
- **R4 Owner matching, fail closed.** `activeConfig()` = config present AND
  token present AND (`sessionUserId === null` OR `sessionUserId ===
  config.userId`). A null `sessionUserId` locks and deletes nothing. The
  config (+ attempts) is deleted only in `setToken` (lib/storage.ts, the one
  choke point for every sign-in) when both ids are known and differ.
- **R5 Biometrics accept the device passcode.** `userVerification:
  'required'` is satisfied by the phone passcode / Windows Hello PIN. Say so
  in the biometrics hint ("Your device may also accept its passcode") and in
  the threat model.
- **R6 Ceremony in flight.** A module flag set while `create()`/`get()` is
  pending and for 2 s after; while set, hiding the page neither locks nor
  records `hiddenAt` (Android's passkey prompt is a separate activity), and
  the app is not obscured-then-locked mid-setup.
- **R7 One stable passkey.** `user.id` = SHA-256(userId) bytes (precomputed,
  so re-registering replaces the keychain entry), `user.name` =
  `"<email> (app lock)"`, `displayName` = `"Econumo app lock"`,
  `excludeCredentials` = the stored id if any (an `InvalidStateError` then
  means "already registered": treat the stored id as still valid). The passkey
  syncs through iCloud Keychain / Google Password Manager; the hint says so.
- **R8 Transports.** `allowCredentials.transports: ['internal']` only.
- **R9 Sign-out rules.** `LogoutPage` removes the token first, then the lock
  config. It keeps the config when router state has `keepPendingWrites:
  true` and no `resetAppLock` ("Set a password" re-sign-in). "Forgot PIN" and
  the attempts limit navigate to `/logout` with `{ keepPendingWrites: true,
  resetAppLock: true }`: the owner's offline queue survives (it is per user),
  the lock config is removed. The forgot-PIN dialog then doesn't warn about
  losing changes.
- **R10 User gesture.** `create()`/`get()` are called synchronously in the
  click handler: challenge, user id hash and credential id are prepared
  before (on mount / when the row renders); nothing is awaited before the
  call.
- **R11 Availability.** `biometricAvailable()` is false when not a secure
  context, in the native app, when `location.hostname` is an IP address
  (v4 or v6) and when the platform-authenticator probe is false.
- **R12 Insecure context.** When `!window.isSecureContext` (`crypto.subtle`
  missing — plain-http self-hosting), the App lock row is shown disabled with
  the hint "Needs HTTPS"; the lock never activates (activeConfig is null).
- **R13 Init timing.** No storage read at module load: `initAppLock()` runs in
  `main.tsx` inside `bootNativeApp().then(...)` before render; it sets the
  initial `locked` and installs visibility/storage/heartbeat listeners
  (idempotent).
- **R14 Native mirror.** Both keys are added to `MIRRORED_KEYS` and written
  through `mirrorWrite`.
- **R15 Clock.** A negative `now - hiddenAt` locks. A `lockedUntil` more than
  15 min in the future is clamped to now + 15 min.
- **R16 Heartbeat.** While configured and unlocked, a 15 s interval: if the
  wall-clock gap since the last tick ≥ max(lockAfterMs, 30 s), the device
  slept — lock. No idle-input timer in this change.
- **R17 Fresh sign-in is an unlock.** `setToken` sets a sessionStorage marker
  `econumo.appLock.fresh`; `initAppLock()` consumes it (start unlocked, reset
  attempts) — so password login's full reload doesn't ask for the PIN. A
  client-side sign-in (OAuth handoff, re-login after 401) calls
  `unlock()` through the same `setToken` path. A lock screen whose config
  vanished reloads instead of crashing.
- **R18 Gate placement.** Gate stays in `RequireAuth` (covers the consent page
  too) and unmounts `ApplicationLayout`; its hooks (budget-month keeper, sync
  button) pause while locked and unlock remounts and refetches — accepted.
  `LockScreen` reads the user with `queryClient.getQueryData(['user', …])`
  (the existing user query key), never a fetching hook.
- **R19 Auto biometric attempt.** Only when the lock screen appears with the
  page visible and focused, on a touch device (`(pointer: coarse)`), once per
  appearance (ref guard, StrictMode-safe), with an `AbortController` aborted
  on unmount; any rejection is silent.
- **R20 PinPad modes.** `verify` mode: dots = stored length, auto-submit.
  `create` mode: up to 8 dots fill as typed, a "Continue" button enabled at
  ≥4 digits. The keydown listener ignores events from inputs/textareas and
  only runs while its PinPad is the topmost interactive surface (the lock
  screen, or the open PIN dialog).
- **R21 Lockout and biometrics.** Biometric unlock is allowed during a
  lockout and resets the counter. Settings prompts share the counter (R2).
- **R22 Privacy cover outside React.** A static `<div id="privacy-cover">`
  appended to `<body>` by `initAppLock()`; the `visibilitychange`/`pagehide`
  handler toggles its `hidden` attribute synchronously, then syncs the store.
  Style: fixed inset-0, `z-index: 2147483647`, opaque background matching the
  theme (CSS var), logo. No `PrivacyCover` React component.
- **R23 Guards.** Add `AppLockPage` to `NO_NOTICE` in
  `offline-notice-coverage.test.ts` ("local device setting, works offline").
