# Change email (self-service) — Design

**Date:** 2026-07-22
**Status:** Draft for review
**Depends on:** the retire-`identifier` work (PR #141, merged) — this design targets the email-only model (`lower(email)` is the user key; `GetByEmail`/`ExistsByEmail`; no identifier derivation).

## Problem

There is no way for a signed-in user to change their own email. Email is the login username and the password-reset target, so changing it is a sensitive account operation that must prove ownership of the new address before it takes effect.

## Approved decisions (from brainstorming)

- **Verify the NEW email with a 6-digit code**, gate the request on the **current password**, and **notify the OLD email**.
- On confirm, **revoke other sessions** (keep the current one) — mirrors `update-password`.
- Store the pending change in a **new `users_email_change_requests` table**.
- Include a **resend-code** endpoint.
- Code verification is **always required**, independent of `ECONUMO_EMAIL_VERIFICATION` (that flag only gates the registration/login path).

## Flow

```
ProfilePage → "Change email" → ChangeEmailPage
  Step 1 (request):  new email + current password
     POST /api/v1/user/request-email-change   { newEmail, password }   [authenticated]
       → verify current password (hasher.Verify against stored hash+salt)
       → normalize newEmail (trim); reject if it equals the current email
       → reject if already taken: ExistsByEmail(newEmail) → CodeUserAlreadyExists
       → generate 6-digit code; store pending change (replace any prior), 10-min TTL
       → email the code to the NEW address
       → email a change-requested notice to the OLD address (naming the new address)
       → rate-limited; 200 with an empty success payload
  Step 2 (confirm):  enter code
     POST /api/v1/user/confirm-email-change    { code }               [authenticated]
       → load pending change for the user; generic invalid-code error (anti-enumeration)
       → compare HashResetCode(code); check expiry
       → re-check ExistsByEmail(newEmail) at commit time (race guard)
       → commit: u.UpdateEmail(Encode(newEmail), now) + MarkEmailVerified(now); delete pending row
       → revoke OTHER sessions (keep presenting token); PATs untouched
       → 200 returning the refreshed CurrentUserResult (SPA updates the shown email)
  Resend:
     POST /api/v1/user/resend-email-change-code                        [authenticated]
       → rate-limit/cooldown → 200 (+ Retry-After); re-send the code to the pending new address
       → silent no-op if there is no pending change (anti-enumeration)
```

All three endpoints are **authenticated** (unlike registration's public `confirm-email`) — the whole flow happens inside a signed-in profile session, and the code is delivered to the new inbox.

## Backend

### Data — new table (both engines)

`users_email_change_requests`, mirroring `users_email_verifications`:

| column | sqlite / pgsql | notes |
|---|---|---|
| `id` | TEXT / UUID | UUIDv7 |
| `user_id` | TEXT / UUID | FK → `users(id)` ON DELETE CASCADE, **UNIQUE** (one pending change per user) |
| `new_email` | VARCHAR(255) | the proposed email, stored via `Encode` (salt-free passthrough = plaintext) |
| `code` | TEXT / VARCHAR(64) | `HashResetCode(code)` sha256 hex, **UNIQUE** |
| `created_at`, `updated_at`, `expired_at` | DATETIME / TIMESTAMP(0) | 10-min TTL, frozen `Y-m-d H:i:s` |

No `new_identifier` column (email-only model). Uniqueness of the target is enforced against `users` via `ExistsByEmail`, not by a constraint on this table.

sqlc queries `query/{sqlite,pgsql}/email_change_request.sql`: `Insert`, `GetByUser`, `DeleteByUser`. Repo `internal/user/repo/emailchangerequest*.go` (interface in canonical sqlite types + `_sqlite.go`/`_pgsql.go` adapters); a new `EmailChangeRequests` port interface in `internal/user/repository.go` (`GetByUser`/`Save`/`DeleteByUser`), wired in `server.BuildAPI`.

### Model

`internal/model/user_email_change_request.go`: `EmailChangeRequest{ID, UserID, NewEmail, Code, CreatedAt, UpdatedAt, ExpiredAt}`, `NewEmailChangeRequest(...)` (10-min TTL constant), `IsExpired(now)`, `RetryAfter(now)`, and a resend-gap constant (60s) — same shape as `user_email_verification.go`.

### Use cases (`internal/user/change_email.go`, methods on the user `Service`)

- `RequestEmailChange(ctx, userID, req)` — load user by id; `hasher.Verify` the current password (mirror `UpdatePassword`); normalize + validate `newEmail`; reject if it equals the current email (a dedicated "same as current email" coded error) or `ExistsByEmail` is true (`CodeUserAlreadyExists`); generate the code; in a tx `DeleteByUser` then `Save`; send the code to the new address and the notice to the old address. Rate-limited by the caller (per user id).
- `ConfirmEmailChange(ctx, userID, req)` — `GetByUser`; generic invalid-code error on missing/mismatch/expired (anti-enumeration), failed attempts counted; re-check `ExistsByEmail(newEmail)` (race); in a tx `u.UpdateEmail(Encode(newEmail), now)` + `MarkEmailVerified(now)` + `repo.Save` + `DeleteByUser`; then revoke other sessions (reuse the `update-password` cascade with the presenting token). Returns the refreshed `CurrentUserResult`.
- `ResendEmailChangeCode(ctx, userID)` — unconditional rate-limit → `Retry-After`; regenerate + replace the pending code and re-send to the new address; silent no-op if no pending row. Returns a `time.Duration` cooldown.

### DTOs (`internal/model/user_dto.go`)

- `RequestEmailChangeRequest{NewEmail, Password}` + `Validate()` (reuse `validateEmailField`; non-blank password).
- `ConfirmEmailChangeRequest{Code}` + `Validate()` (non-blank / length like `ConfirmEmailRequest`).
- Results: `ConfirmEmailChange` returns `CurrentUserResult`; request/resend return the standard empty success payload.

### API (`internal/user/api/change_email.go` + `routes.go`)

- `RequestEmailChange`, `ConfirmEmailChange` via `endpoint.Handle` (auth + JSON body + `Validate()`).
- `ResendEmailChangeCode` hand-written like `ResendVerificationCode` (sets `Retry-After`).
- Routes (authenticated, under `/api/v1/user/`): `request-email-change`, `confirm-email-change`, `resend-email-change-code`.

### Mailer (`internal/infra/mailer/change_email.go`)

- `SendEmailChangeCode(ctx, to, name, code)` — to the NEW address (renders `emails.change_email.{subject,body}`).
- `SendEmailChangeNotice(ctx, to, name, newEmail)` — to the OLD address (renders `emails.change_email_notice.{subject,body}`, naming the new address so the owner can spot an unwanted change).
- Register the four keys in `EmailKeys` (the i18ntest guard asserts each exists per language).

### Config / rate limits (`internal/user/ports.go`, `internal/config/config.go`, `internal/server/server.go`)

New scopes keyed by **user id**: `RateScopeRequestEmailChange` (every attempt), `RateScopeConfirmEmailChange` (failed only, cleared on success), `RateScopeEmailChangeSent` (timestamp channel for the resend cooldown). New env vars, mirroring the verify-email ones:
- `ECONUMO_RATE_LIMIT_REQUEST_EMAIL_CHANGE` (default 3)
- `ECONUMO_RATE_LIMIT_CONFIRM_EMAIL_CHANGE` (default 5)
- (resend reuses the sent-timestamp gap, 60s; window is the shared `ECONUMO_RATE_LIMIT_WINDOW`)

Documented in CLAUDE.md + `.env.example`.

## Frontend (`web/`)

- `web/src/features/settings/ChangeEmailPage.tsx` — two-phase (request form → code form), mirroring `ChangePasswordPage` (SettingsShell, LoadingDialog, success/error ResponsiveDialogs). The resend button reuses the existing `Retry-After` cooldown handling from the verification client.
- `ProfilePage.tsx` — the currently-disabled email field gains a "Change" affordance routing to the new page; add `RouterPage.SETTINGS_CHANGE_EMAIL` + route.
- `web/src/api/user.ts` + `dto/user.ts`: `requestEmailChange`, `confirmEmailChange`, `resendEmailChangeCode` (the last reads `retry-after`, like `resendVerificationCode`).
- Hooks in `web/src/features/user/queries.ts`; confirm `onSuccess` invalidates `useUserData`.
- **Analytics:** new `METRICS` key (e.g. `appChangeEmail`) fired on confirm success (satisfies `metrics-coverage.test.ts`).

## i18n (`locales/{en,ru}.json`, `internal/test/i18ntest`)

- `emails.change_email.{subject,body}` (uses `{name}`, `{code}`), `emails.change_email_notice.{subject,body}` (uses `{name}`, `{email}`) — added to both catalogues and to `EmailKeys`.
- New `settings.*` UI strings for the page.
- Any new `errors.*` code (e.g. a "same as current email" message) added to `errs` registry (`internal/shared/errs/codes.go`) so the two-way `errs.AllCodes` ↔ `errors.*` guard passes.

## Testing

- **apiparity:** scenarios for all three routes + goldens (`UPDATE_GOLDEN=1`, then inspect the diff); guard counts updated; `handlerGlobs` already covers `internal/user/api`.
- **Unit/integration (sqlite + pgsql via `make test-repo-pgsql`):** request→confirm happy path; wrong/expired code; duplicate-email reject at request AND at confirm (race); password-wrong reject; same-as-current reject; resend cooldown (`Retry-After`); pending-row replacement (a second request supersedes the first); the session-revocation cascade (other sessions revoked, presenting token + PATs survive).
- **i18n guards** (key parity, `{var}` parity, `errs.AllCodes` ↔ `errors.*`, `EmailKeys` ↔ `emails.*`) run in `make go-test`.
- **Frontend:** `web-test` for the page's request/confirm/resend flows and the analytics event; `pnpm exec tsc -b` type-check.
- **MCP:** no MCP surface for this profile action (matches `update-password`/`update-avatar`), so no `mcpparity` change — confirm during implementation.

## Wire & contract notes

- Datetimes `Y-m-d H:i:s`; error envelope + coded messages rendered in the caller's language via the existing `errs`/i18n path.
- Codes: 6-digit numeric via `generatePasswordCode`, only the `HashResetCode` sha256 hex stored; 10-min TTL; 60s resend gap — identical to the reset/verify flows.
- The notice email reveals the proposed new address to the OLD inbox (the account owner), so an unwanted change is noticeable. Accepted.

## Out of scope

- Changing email for other users (that is the existing CLI `user:change-email` / admin path — unchanged).
- Undo/rollback of a completed change (the notice email is the safeguard; a user who loses access uses password-reset on the new address).
