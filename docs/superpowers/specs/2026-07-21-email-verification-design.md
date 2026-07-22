# Email Verification After Registration — Design

**Date:** 2026-07-21
**Status:** Approved

## Problem

Registration accepts any email address without proof of ownership. On the cloud
instance this means unreachable users (typos, throwaway addresses) and no
reliable channel for billing or account communication. Users must prove they
own their email before they can use the application.

## Decisions (settled during brainstorming)

1. **Opt-in via env var.** New `ECONUMO_EMAIL_VERIFICATION` flag, default
   `false`. Self-hosted instances (whose default mailer is `console://`) are
   unaffected unless they opt in.
2. **Code only, no link.** The user types a short emailed code into a dialog.
   This mirrors the existing password-reset flow (`RecoveryDialog` +
   `users_password_requests`); the SPA has no deep-link/token-in-URL machinery
   and this design adds none.
3. **Email sent at blocked login, not at registration.** Registration is
   untouched. The first login attempt by an unverified user triggers the code
   email.
4. **Dedicated public endpoints for confirm and resend** (revised 2026-07-22
   after implementation review; the first iteration folded `code`/`resend`
   into the login request). Login keeps only the 403 "verification required"
   signal and the auto-send of the first code; confirming and resending are
   their own endpoints, mirroring the remind-password / reset-password pair:
   - `POST /api/v1/user/confirm-email` — `{username, code}`, no password.
     Unknown user, no outstanding code, and a wrong code all collapse into
     one generic "The confirmation code is not valid." validation error
     (anti-enumeration); an expired code gets its own message, like reset.
     Success is an empty envelope; the SPA then re-submits login.
     Failed attempts count under a new `confirm-email` rate scope
     (`ECONUMO_RATE_LIMIT_CONFIRM_EMAIL`, default 5, cleared on success) —
     the 12-hex code is the sole secret on this route, so it gets reset-grade
     brute-force protection.
   - `POST /api/v1/user/resend-verification-code` — `{username}` only,
     always returns success (anti-enumeration, like remind-password): unknown
     or already-verified users are a silent no-op; an unverified user gets a
     fresh code (replacing the old one), counted under the `verify-email`
     send scope.

## Data model

Migration (sqlite + pgsql, run on boot):

- `ALTER TABLE users ADD COLUMN email_verified BOOLEAN NOT NULL DEFAULT '1'`
  — the default backfills every existing row as **verified**; no existing
  user is ever locked out.
- New table `users_email_verifications`, mirroring `users_password_requests`:
  `id` (UUIDv7 TEXT PK), `user_id` (FK to users, UNIQUE — one outstanding
  code per user), `code` (sha256 hex of the plaintext code), `created_at`,
  `updated_at`, `expired_at`. Codes are 12-char hex, generated and hashed
  exactly like password-reset codes, 10-minute TTL. Only the hash is stored.

## Configuration

- `ECONUMO_EMAIL_VERIFICATION` — boolean, default `false`. Strict parse
  (malformed values fail at boot, like `ECONUMO_ANALYTICS`).
- `ECONUMO_RATE_LIMIT_VERIFY_EMAIL` — max verification emails per user per
  window (default `3`, every send counts, like remind). Shares
  `ECONUMO_RATE_LIMIT_WINDOW`.
- `ECONUMO_RATE_LIMIT_CONFIRM_EMAIL` — max FAILED confirm-email attempts per
  username per window (default `5`, cleared on success, like reset). Shares
  `ECONUMO_RATE_LIMIT_WINDOW`.
- `serve` logs a WARN at boot when verification is enabled while the mailer
  is the console transport (verification emails would go to stdout only).

## Behavior

### Registration (`register-user`)

- Flag **on**: the new user is written with `email_verified = false`.
- Flag **off**: `email_verified = true` (so flipping the flag on later does
  not strand users registered while it was off).
- No email is sent at registration. Response shape unchanged.
- Trial access (`ECONUMO_TRIAL`) is granted at registration exactly as today.
- CLI `user:create` and the admin path always create verified users.

### Login (`login-user`)

The login request keeps its frozen `{username, password}` shape — no new
fields. One block in `Service.Login`, placed **after** the existing password
+ `is_active` check and active only when the flag is on and the user is
unverified: ensure an outstanding code exists — if none or expired, generate
one and send the email (rate-limited under the verify-email scope;
over-limit → the standard 429 envelope) — then respond **HTTP 403** with the
standard error envelope and catalogue code `user.email_verification_required`.
403 is otherwise unused on this route, so the SPA distinguishes the
verification state by status alone.

### Confirm (`confirm-email`) and resend (`resend-verification-code`)

Two public routes mirroring remind/reset (registered in the public group,
`endpoint.HandlePublic`, empty-object success envelopes):

- `ConfirmEmail(ctx, {username, code})`: rate-gate under `confirm-email`
  (failed attempts only); resolve the identifier; unknown user, missing row,
  or hash mismatch → generic validation error `user.verification_code_invalid`
  (indistinguishable, anti-enumeration); expired →
  `user.verification_code_expired`; success → mark verified + delete the code
  row in one transaction, clear the attempt counter, return `{}`.
- `ResendVerificationCode(ctx, {username})`: always returns `{}`; unknown or
  already-verified user is a silent no-op; otherwise force a fresh code +
  email under the `verify-email` send scope (over-limit → 429, like remind).

All three catalogue codes (`user.email_verification_required`,
`user.verification_code_invalid`, `user.verification_code_expired`) are
registered in `errs.AllCodes` with `errors.*` entries in `en` and `ru`
(enforced by i18ntest).

### Password reset

A successful `reset-password` also sets `email_verified = true` — completing
the reset proves mailbox ownership.

### Email

New mailer sender mirroring `internal/infra/mailer/reset.go`, with catalogue
keys `emails.verify.subject` / `emails.verify.body` (`{name}`, `{code}`
placeholders) in `locales/{en,ru}.json`, rendered in the caller's language
via the existing `reqctx.Language` resolution.

### CLI

- `user:show` additionally displays the email-verified state.
- New `user:verify-email <email>` command marks a user verified (support /
  self-host rescue hatch).

### Out of scope / unaffected

- MCP edge: unaffected — PATs are minted from an already-logged-in session.
- No change to remind-password, registration response, token formats, or the
  402 access-level middleware.
- No verification of email *changes* (`user:change-email`) — the CLI is an
  operator tool; out of scope.

## SPA

- The login mutation surfaces the HTTP status; `LoginPage` opens a new
  `VerifyEmailDialog` when login fails with 403 (no other 403 exists on the
  route).
- `VerifyEmailDialog`, modeled on `RecoveryDialog` (ResponsiveDialog):
  "We sent a code to {email}", a code input, a **Verify** button that calls
  `confirm-email` and, on success, silently re-submits login with the
  credentials still held in form state (the user lands in the app in one
  visible step), and a **Resend code** button calling
  `resend-verification-code` with the username. Wrong-code / throttle errors
  render inline via `apiErrorMessage`.
- New `auth.verify_email.*` strings in both catalogues.
- Analytics (house rule): `METRICS` keys `appEmailVerificationCompleted`
  (fired on the successful verify-login) and `appEmailVerificationResent`
  (fired on resend), both at the mutation choke point so
  `metrics-coverage.test.ts` passes.

## Testing

- **Go unit/integration** (sqlite via dbtest; pgsql via `make test-repo-pgsql`):
  flag off → behavior identical to today; flag on → unverified login returns
  403 and sends exactly one email while a code is outstanding; valid code →
  `{token, user}` + `email_verified` flipped + row deleted; invalid/expired
  code → 403 + counts toward the login limiter; resend throttled at the
  verify-email cap; reset-password flips `email_verified`; migration backfill
  leaves existing users verified; CLI `user:verify-email` and `user:show`.
- **apiparity / enginecompare:** the two new public routes get catalogue
  scenarios (route guard requires one per route): `confirm-email` with a bad
  code → the generic invalid-code envelope; `resend-verification-code` for a
  verified user → the empty success envelope (flag-off default config, so
  no email side effect). Existing goldens stay byte-identical; only new
  golden files are added. Flag-on behavior stays covered by the
  feature-level service tests.
- **i18ntest:** new catalogue keys, placeholder parity, and the two error
  codes are enforced automatically.
- **Vitest:** VerifyEmailDialog flow — opens on 403, verify re-submits with
  code, resend fires, inline errors render; metrics coverage.
