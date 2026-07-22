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
4. **Verification is folded into the login endpoint** (no separate
   `verify-email` route). The login request gains optional `code` and `resend`
   fields; HTTP 403 signals "verification required". The code is only ever
   processed after the password check passes, so there is no new public
   surface to enumerate or brute-force independently.

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

New block in `Service.Login`, placed **after** the existing password +
`is_active` check and active only when the flag is on and the user is
unverified:

- **No `code` in the request:** ensure an outstanding code exists — if none
  or expired, generate one and send the email (rate-limited under the new
  verify-email scope; over-limit → the standard 429 envelope). Respond
  **HTTP 403** with the standard error envelope and catalogue code
  `user.email_verification_required`.
- **`resend: true`:** force a fresh code + email (same rate limit), then the
  same 403.
- **`code` present:** compare sha256 against the outstanding row.
  - Valid and unexpired → set `email_verified = true`, delete the code row,
    and continue into the normal session mint. The response is the ordinary
    frozen `{token, user}` — verify and login complete in one round trip.
  - Invalid or expired → **403** with catalogue code
    `user.verification_code_invalid`, counted as a failed attempt under the
    existing login rate limiter.

Wire notes: the login request DTO gains two **optional** fields (`code`
string, `resend` bool) — additive, existing clients unaffected. 403 is
currently unused on this route, so the SPA distinguishes the verification
state by status alone; the error envelope itself is unchanged. Both new
catalogue codes are registered in `errs.AllCodes` with `errors.*` entries in
`en` and `ru` (enforced by i18ntest).

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
  "We sent a code to {email}", a code input, a **Verify** button that
  re-submits login with the code using the credentials still held in form
  state (success lands the user in the app in one step), and a **Resend
  code** button (login with `resend: true`). Wrong-code / throttle errors
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
- **apiparity / enginecompare:** no new route, so route guards and existing
  goldens are untouched (default config = flag off). Add flag-on scenarios if
  the catalogue supports per-scenario server config; otherwise the
  feature-level tests above carry that coverage.
- **i18ntest:** new catalogue keys, placeholder parity, and the two error
  codes are enforced automatically.
- **Vitest:** VerifyEmailDialog flow — opens on 403, verify re-submits with
  code, resend fires, inline errors render; metrics coverage.
