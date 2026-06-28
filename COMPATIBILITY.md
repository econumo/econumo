# Compatibility constraints

This service is a drop-in replacement for an existing HTTP API. The behaviors
documented here are **frozen**: they must stay byte-compatible with what is
already deployed, because three things outlive any single release of the server:

- **Existing API clients** — the web SPA and the iOS app parse exact JSON
  shapes, field names, encodings, and error messages.
- **Existing database data** — stored password hashes, encrypted emails, user
  identifiers, and row id formats were written by the prior implementation.
- **Already-issued auth tokens** — JWTs in the wild must continue to verify
  until they expire (up to 30 days after the cutover).

When in doubt, do not "clean up" any of the specs below. Each one has a
one-line reason it is frozen. Changing it breaks live clients, locks users out,
or makes existing data unreadable.

---

## Response envelope

All API responses use one of four shapes. See `internal/ui/httpx/envelope.go`.

- **Success** (HTTP 200):
  `{"success": true, "message": "", "data": <payload>}`
- **Error** (validation / handled HTTP errors, default HTTP 400):
  `{"success": false, "message": <string>, "code": <int>, "errors": <object>}`
  - `errors` is an **object**, mapping field name → array of message strings,
    e.g. `{"name": ["Category name must be 3-64 characters"]}`.
  - `errors` is **always present**, serialized as `{}` when there are none.
- **Exception** (unhandled, HTTP 500):
  `{"success": false, "message": <string>, "code": 0, "exceptionType": <string>}`
  - The `errors` key is **omitted** entirely here.
  - `stackTrace` is added **only** when running in dev (`APP_ENV=dev`).
- **Not implemented** (HTTP 501):
  `{"success": false, "message": <string>, "code": 0, "errors": []}`
  - Note: here `errors` is an **array** (`[]`), not an object — the lone
    exception to the object rule above.

JSON is encoded with HTML escaping disabled, so `/`, `<`, `>` etc. appear
literally (not as `/`).

*Why frozen:* clients branch on `success`, read `errors` as a field→messages
map, and the 501/exception variants differ structurally from the normal error
shape — any deviation breaks client-side error handling.

---

## Auth crypto

See `internal/infra/auth/` (`password.go`, `encode.go`).

### Password hash
- Algorithm: **sha512**, **500 iterations**, output **base64** (standard,
  padded → 88 chars).
- Salt is merged into the password as `password{salt}` (literal braces); an
  empty salt leaves the password unchanged.
- Round structure: `digest = sha512(salted)`, then 499 more rounds of
  `digest = sha512(digest || salted)`.
- Verify rejects any stored hash whose length ≠ 88 or that contains `$`, then
  compares in constant time.

*Why frozen:* stored hashes were written this way; any change means no existing
user can log in.

### User identifier
- `identifier = hex(md5(lowercase(email) || ECONUMO_DATA_SALT))` — a 32-char
  hex string (CHAR(32)).
- The caller lowercases the email before hashing.

*Why frozen:* it is the primary lookup key for existing user rows.

### Email encryption (reversible)
- AES-128-CBC; the key is the raw `ECONUMO_DATA_SALT` bytes (must be exactly 16
  bytes for AES-128).
- On-the-wire layout: `base64( iv[16] || hmac_sha256[32] || ciphertext )`.
- Padding: PKCS#7. IV is random per encryption. Decode verifies the HMAC
  (constant time) before decrypting.
- Empty salt → passthrough (value stored/returned as-is).

*Why frozen:* existing encrypted email columns must remain decryptable.

---

## JWT

See `internal/infra/auth/jwt.go`.

- Signing: **RS256** only. Issuing and verification both reject any other
  algorithm (defends against `none`/HS256 alg-confusion).
- Keys: the existing RSA keypair. Public key (SPKI PEM) is always loaded;
  private key (encrypted PKCS#8 PEM, PBES2) is loaded only when present, so a
  verify-only deployment needs no signing key.
- Claims:
  - `iat` — issued-at Unix seconds
  - `exp` — `iat + 2592000` (**30-day TTL**)
  - `roles` — `["ROLE_USER"]`
  - `username` — the decoded **plaintext email**
  - `id` — the user UUID string
  - No `nbf`, `iss`, `sub`, `aud`, or `baseCurrency` claim.

*Why frozen:* tokens already issued (up to 30 days before cutover) must keep
verifying, and clients read `id`/`username` out of the token.

---

## Datetime wire format

- All API datetimes are formatted as `"2006-01-02 15:04:05"` — space separator,
  no timezone, no fractional seconds.

*Why frozen:* clients parse this exact layout; an ISO-8601 `T`/`Z` form would
not parse.

---

## Field encodings

- `isArchived` is serialized as an **int** `0`/`1`, not a JSON boolean.
- Category `type` is serialized as an **alias string**: `"expense"` or
  `"income"` (internally backed by a small integer).
- Nullable fields follow the existing schema; empty string is used where the
  prior implementation used empty-string-for-NULL.

*Why frozen:* clients read these exact JSON types/values.

---

## Validation messages

Error message strings must match exactly, character for character. Examples:

- Category name out of range → `"Category name must be 3-64 characters"`
  (field key `name`).
- Login failure → `"Invalid credentials."` (HTTP 401).
- Blank required field → `"This value should not be blank."`
  (code `IS_BLANK_ERROR`).

*Why frozen:* clients (and tests) assert on these literal strings.

---

## Routes

Exact paths and methods are part of the contract (e.g.
`POST /api/v1/user/login-user`, `POST /api/v1/user/register-user`, and the
`/api/v1/...` resource routes such as `POST /api/v1/category/create-category`).

- **Login** (`/api/v1/user/login-user`): request body uses `username` (the
  email) + `password`; response is `{"token": ..., "user": ...}`.
- **Register** (`/api/v1/user/register-user`): returns the created user
  **without** a token (distinct from login).
- Public routes: login, register, remind-password, reset-password, plus
  `/api/doc` and `/api/doc.json`. Everything else requires a valid JWT.

*Why frozen:* clients call these exact URLs and depend on login-vs-register
returning different shapes.

---

## API documentation paths

- Swagger UI served at `/api/doc`; raw OpenAPI spec at `/api/doc.json`. Both
  public; the frontend fetches the spec from `/api/doc.json`.

*Why frozen:* the documentation URLs are linked/fetched as-is.

---

## Database schema

- The schema is the **existing** one: ids are stored as `TEXT`, etc. The Go
  code reads and writes the same tables/columns/types as before.
- The legacy migration mechanism still reads the actual `migration_versions`
  table to determine schema state.

*Why frozen:* this is live production data; the running schema is shared with
the data already in it.

---

## ID generation

- **New** ids are **UUIDv7** (time-ordered).
- **Existing** ids are never rewritten — they are JWT claim values, foreign-key
  targets, and held by clients.

*Why frozen:* an existing id is referenced from tokens, related rows, and client
state; only freshly created entities get new (v7) ids.
