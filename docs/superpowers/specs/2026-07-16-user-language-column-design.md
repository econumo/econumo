# Persist last selected language on the user (write-only) — Design

**Date:** 2026-07-16
**Status:** Approved for planning
**Builds on:** `2026-07-15-i18n-design.md` (shared catalogues, Accept-Language middleware, `reqctx.Language`)

## Goal

Store each user's last selected UI language in the `users` table so future
background jobs (emails sent outside a request context) can render in the
user's language. Write-only for now: nothing reads the column yet.

## Decisions

1. **Capture paths: explicit endpoint + login.** A new
   `POST /api/v1/user/update-language` (called by the SPA when an
   authenticated user switches language) plus a persist-on-login step (covers
   a language picked on the login page before any token exists). No hot-path
   (per-request) writes.
2. **Column default is `'en'`** — the app was English-only until now, so
   existing rows are correctly backfilled as English. No "unknown" state;
   future consumers use the stored value directly.
3. **Server-side resolution.** Both writes store the value the middleware
   resolved from `Accept-Language` (endpoint: the validated request body) —
   always a member of `i18n.Supported`.
4. **No read path.** The column appears in no SELECT, DTO, or API response.
   `get-user-data` and every existing route's golden stay byte-identical.

## Schema

New migration pair (`internal/infra/storage/migrations/{sqlite,pgsql}`):

```sql
ALTER TABLE users ADD COLUMN language TEXT NOT NULL DEFAULT 'en';
```

## Backend

- **Endpoint** `POST /api/v1/user/update-language`, body
  `{"language": "ru"}` — follows the existing `update-*` family exactly
  (named handler with swag annotations delegating to `endpoint.Handle`,
  route in `internal/user/api/routes.go`, DTO + `Validate()` in
  `internal/model/user_dto.go`). `Validate()` rejects values outside
  `i18n.Supported` with a coded validation error (`user.language_invalid`;
  catalogue entries in en + ru). Success returns the family's standard
  envelope.
- **Login capture**: after a successful login, `Login` persists
  `reqctx.Language(ctx)` for the user — one unconditional `UPDATE` alongside
  login's existing writes.
- **Repository**: `UpdateLanguage(ctx, userID, lang)` on the user repo —
  new `UpdateUserLanguage` query in both dialect files + `sqlc generate`,
  method written once against the `querier` interface (engine-adapter
  pattern, no driver branching).

## Frontend

### Initial language detection (browser locale, clamped to supported)

`locale()` today can return an UNSUPPORTED tag (e.g. `de` from
`navigator.language`): i18next then falls back to English for rendering, but
the raw tag leaks into `<html lang>`, `pluralPick`, and the `Accept-Language`
header. Fix the resolution so `locale()` always returns a supported tag:

1. stored localStorage value, if it is in the supported set (stale or
   tampered values are ignored, not deleted);
2. otherwise the first entry in `navigator.languages` (falling back to
   `navigator.language`) whose primary subtag is supported;
3. otherwise `'en'`.

The supported set comes from `getLocaleOptions()` — one source of truth on
the web side. Selecting a language explicitly still persists it exactly as
today.

### Persisting the choice

`applyLocale(value)` (the shared path behind the selector dropdown and the
Settings dialog) additionally fires a fire-and-forget
`update-language` request when an access token is present; failures are
ignored (login capture self-corrects later). The login-page selector makes
no call — login capture covers it.

## Testing

- Repo test for `UpdateLanguage` (runs on both engines via the existing
  pgsql rerun).
- Endpoint tests: happy path (column updated), invalid language → 400 with
  `user.language_invalid` in `errorCodes`.
- Login-capture test: login with `Accept-Language: ru` → column is `ru`;
  no header → column is `en`.
- apiparity: new scenario + golden for `update-language` (guard requires
  every route to have one); all existing goldens unchanged.
- i18ntest guards cover the new error code's catalogue entries.
- SPA test: `applyLocale` posts `update-language` when a token is present,
  and does not when logged out.
- SPA detection tests: unsupported stored value ignored; `navigator.languages`
  `['de-DE', 'ru-RU']` → `ru`; all-unsupported / undetectable → `en`.

## Out of scope

- Reading or displaying the stored value anywhere.
- The background email sender that will eventually consume it.
- Migrating the reset-password email off request-scoped Accept-Language.
