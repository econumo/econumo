# Opaque access tokens: DB-backed sessions + personal access tokens

Date: 2026-07-10

## Problem

Authentication today is a stateless RS256 JWT with a fixed 30-day TTL. Nothing
is stored server-side, so tokens cannot be listed or revoked: a leaked token
stays valid until it expires, a password change does not invalidate existing
logins, and there is no way to see "where am I logged in". We also want
personal access tokens (PATs) for integrations, which requires server-side
token storage anyway.

## Decision summary

- **Opaque tokens everywhere.** Both login sessions and PATs become random
  opaque strings stored (hashed) in a new `access_tokens` table. The JWT
  machinery — `internal/shared/jwt`, the RSA keypair, `jwt:generate`,
  `EnsureKeypair`, the `ECONUMO_JWT_*` config — is removed entirely.
- **Sliding session TTL.** A session expires 30 days after its *last use*
  (throttled touch), not 30 days after login.
- **PATs** are full-access (no scopes), named, with an optional expiry
  (30/90/365 days, custom date, or never). Created and revoked in the UI;
  the raw token is shown exactly once.
- **Sessions UI.** The profile page gets a "Security" section: change
  password (existing), a sessions sub-page (list + revoke + "sign out other
  devices"), and a personal-tokens sub-page.
- **Password change revokes sessions** (all but the current one; the email
  reset flow and the CLI revoke all). PATs survive password changes.
- **Hard cutover.** After deploying, every existing JWT gets a 401 and the
  SPA redirects to login. Users log in once; no data migration beyond the new
  table.

## Token format & crypto

- Session token: `eco_ses_<base62>`; personal token: `eco_pat_<base62>`.
  The random part encodes 32 random bytes with `base64.RawURLEncoding`
  (43 chars, alphabet `[A-Za-z0-9_-]`, 256-bit entropy).
  Prefixes make tokens identifiable to humans and secret scanners; the
  authoritative kind still comes from the DB row.
- The DB stores only `hex(sha256(full token string))` under a unique index.
  SHA-256 (not argon2/bcrypt) is deliberate: with 256 bits of entropy,
  brute-forcing hashes is infeasible, and verification must be one cheap
  indexed lookup per API request.
- The raw token is never persisted: the session token goes to the client in
  the login response; a PAT is returned once from `create-personal-token`.

## Schema

One new migration per engine (`internal/infra/storage/migrations/{sqlite,pgsql}`):

```
access_tokens
  id            TEXT PRIMARY KEY        -- UUIDv7
  user_id       TEXT NOT NULL REFERENCES users(id)
  kind          TEXT NOT NULL           -- 'session' | 'personal'
  token_hash    TEXT NOT NULL UNIQUE    -- hex(sha256(token))
  name          TEXT NULL               -- PAT only: user-given label
  user_agent    TEXT NULL               -- session only: User-Agent at login
  created_at    TEXT NOT NULL           -- frozen "Y-m-d H:i:s" layout
  last_used_at  TEXT NOT NULL
  expires_at    TEXT NULL               -- session: always set (last_used_at+30d);
                                        -- PAT: chosen date or NULL (never expires)
  revoked_at    TEXT NULL
```

Plus an index on `user_id` for the list/revoke-cascade queries. Datetimes use
the frozen `Y-m-d H:i:s` layout like every other table. IP address is
deliberately NOT stored: honest client IPs behind a reverse proxy require
trusted `X-Forwarded-For` handling (its own config topic); user-agent + last
activity is enough for the sessions list, and the column can be added later
without breaking anything.

## Lifecycle

- **Valid** = `revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now)`.
- **Sliding window (sessions):** on an authenticated request, if
  `now - last_used_at >= 5 minutes`, one UPDATE sets `last_used_at = now` and
  `expires_at = now + 30 days`. The 5-minute throttle keeps SQLite (single
  writer) from taking a write on every request; "last active" in the UI is
  accurate to ±5 minutes. PATs get the same throttled `last_used_at` touch but
  their `expires_at` is never extended.
- **Revocation** sets `revoked_at` (UPDATE, not DELETE).
- **Purge:** on login, the user's dead rows — those whose expiry or revocation
  happened more than 30 days ago — are deleted opportunistically; no cron, no
  background goroutine. Operators can additionally run `token:purge [days]`
  (CLI), a single global DELETE backed by indexes on revoked_at/expires_at.
- Constants (no new env vars): session TTL 30 days, touch throttle 5 minutes.

## Verification (middleware)

`middleware.TokenVerifier` (`Verify(token) (jwt.Claims, error)`) becomes:

```go
type TokenAuthenticator interface {
    Authenticate(ctx context.Context, token string) (vo.Id, error)
}
```

The implementation lives in the `user` feature (approach: extend `user`, no
new feature package — sessions are auth artifacts of users, and the
password-request precedent already lives there): sha256 the presented token →
SELECT by `token_hash` → validity check → throttled touch → return `user_id`.
`server.BuildAPI` wires the user service into the middleware instead of
`jwt.JWT`. No JOIN on `users.is_active` per request: deactivating a user
revokes all their tokens instead (see cascades).

401 messages change to token-neutral wording (goldens regenerated
deliberately): `"Access token not found"` (missing/malformed header) and
`"Invalid access token"` (unknown, expired, or revoked token). Envelope shape
is unchanged.

## API

Login (`POST /api/v1/user/login-user`) keeps its exact `{token, user}`
response; the token is now opaque. Login creates the session row (capturing
`User-Agent`) inside the existing flow; rate limiting is untouched.
`POST /api/v1/user/logout-user` (already registered, currently a stateless
no-op) now revokes the current session.

New endpoints, all authenticated, module `user`, standard envelope +
`endpoint.Handle`/`HandleNoBody` combinators, swag annotations:

| Route | Behavior |
|---|---|
| `GET  /api/v1/user/get-session-list` | Live sessions only: `id`, `userAgent`, `createdAt`, `lastUsedAt`, `isCurrent` (bool; matched by presented token's hash) |
| `POST /api/v1/user/revoke-session` `{id}` | Revoke one session; not-yours/unknown → the project's domain-not-found envelope (HTTP 400, "Session not found"); revoking the current one is allowed (= logout) |
| `POST /api/v1/user/revoke-other-sessions` | No body; revokes all the user's sessions except the current one |
| `GET  /api/v1/user/get-personal-token-list` | `id`, `name`, `createdAt`, `lastUsedAt`, `expiresAt` (null = never) |
| `POST /api/v1/user/create-personal-token` `{name, expiresAt?}` | Name 1–64 chars; `expiresAt` an explicit datetime in the frozen layout (UI computes it from 30/90/365/custom/never), omitted/null = never;
a provided `expiresAt` must lie in the future. Response includes the full token — the only time it is ever shown. Server generates the id (UUIDv7); no client operation id / idempotency guard |
| `POST /api/v1/user/revoke-personal-token` `{id}` | Revoke one PAT; not-yours/unknown → the domain-not-found envelope (HTTP 400, "Token not found") |

Sessions and PATs are separate lists — PATs never appear in the sessions list.

## Revocation cascades

| Trigger | Effect |
|---|---|
| `update-password` (UI) | Revoke all sessions EXCEPT the current one |
| `reset-password` (email flow) | Revoke ALL sessions (no current session in that flow) |
| CLI `user:change-password` | Revoke ALL sessions |
| CLI `user:deactivate` | Revoke ALL sessions AND all PATs |
| Any password change | PATs are NOT revoked (integrations keep working; GitHub-style) |

## JWT removal

Deleted: `internal/shared/jwt`, the `jwt:generate` CLI command, the
`EnsureKeypair` boot step, `ECONUMO_JWT_PRIVATE_KEY_PATH` /
`ECONUMO_JWT_PUBLIC_KEY_PATH` / `ECONUMO_JWT_PASSPHRASE` config, and the
committed test keypair (`internal/test/testkeys`). Stale env vars in existing
`.env` files are simply ignored — boot does not fail. The `/app/var` volume
now holds only the database.

## Frontend (`web/`)

`ProfilePage` gains a **Security** section — a group of navigation rows in the
existing settings anatomy:

- **Change password** → existing `/settings/profile/change-password`
  (unchanged, just grouped under the new heading).
- **Sessions** → new `/settings/profile/sessions` (`SessionsPage.tsx`):
  live sessions with a light user-agent heuristic ("browser + OS", raw string
  as fallback, no new dependency), created / last-active times, a "Current"
  badge, per-row revoke, and a "Sign out other devices" action. Revoking the
  current session logs out and redirects to login.
- **API tokens** → new `/settings/profile/tokens` (`PersonalTokensPage.tsx`):
  PAT list (name, created, last used, expires or "never"), a create dialog
  (name + expiry: 30/90/365 days / custom date / never), a show-once modal
  with the token, a copy button and a "you won't see this again" warning, and
  revoke with confirmation.

Both new pages use `SettingsShell` (crumbs `Settings → Profile → …`, mobile
back), mirroring `ChangePasswordPage`. Typed clients in `web/src/api`,
TanStack Query hooks, vitest coverage for pages and hooks, i18n strings in
`locales/`. The SPA logout flow must call `logout-user` before dropping the
token (verify whether it already does; wire it if not).

## Testing

- Repo + use-case tests alongside the code (sqlite via `dbtest`; the pgsql
  adapters are exercised by `make test-repo-pgsql` automatically). sqlc
  queries in both dialects; migrations for both engines.
- Middleware tests move to the new `TokenAuthenticator` interface.
- **apiparity:** scenarios for every new/changed route (the guard tests force
  this — route and scenario counts grow); goldens regenerated and the diff
  inspected. The golden normalizer currently redacts JWTs — it must learn to
  redact `eco_ses_*` / `eco_pat_*` tokens.
- **enginecompare:** the new scenarios replay byte-identical on SQLite and
  PostgreSQL for free once they're in the shared catalogue.
- Behavior scenarios: revoked token → 401; expired session → 401; sliding
  window extends on use; password change kills other sessions but not the
  current one and not PATs; reset kills all sessions; `user:deactivate` kills
  everything; PAT with `expiresAt` stops working after it passes; show-once
  (token never appears in any list response).

## Documentation

- CLAUDE.md: replace the frozen "JWT" contract section with the opaque-token
  contract (format, 401 wording, new routes); drop `ECONUMO_JWT_*` from the
  config list; update the auth section.
- README + `.env.example`: remove JWT vars; note the breaking change
  ("after upgrading, everyone logs in again"); `/app/var` volume note.
- OpenAPI regenerated via `make swagger`.

## Out of scope (deliberate)

- Token scopes / read-only PATs (schema doesn't block adding later).
- Storing client IPs (reverse-proxy trust configuration).
- Refresh tokens; configurable TTLs via env.
- Renaming the 401 envelope beyond the two message strings.
