# MCP OAuth (browser sign-in for MCP clients) — design

Date: 2026-10-03
Branch: `feature/mcp-oauth` (based on `v1.6-dev`)

## Goal

Let a user add `https://<instance>/mcp` to Claude (claude.ai custom connectors,
desktop/mobile, Claude Code) or Codex and connect by signing in to Econumo in
the browser and approving the client — instead of creating and pasting a
personal access token.

Success: the client is added by URL alone, a browser opens Econumo, the user
approves, MCP tools work, the client refreshes silently for months, and
revoking it in Settings cuts it off immediately.

## Decisions

| Topic | Decision |
|---|---|
| Protocol | MCP authorization spec: OAuth 2.1 authorization code + PKCE (S256 only), RFC 9728 protected-resource metadata, RFC 8414 AS metadata, RFC 7591 dynamic client registration, RFC 8707 `resource` |
| Implementation | Hand-written, stdlib-first authorization server in a new feature package `internal/authserver`; no OAuth library, no external IdP |
| Token lifetime | 1-hour access tokens + rotating refresh tokens, grant expires 90 days after its last refresh |
| Scope | One scope, `mcp` = full access (everything a PAT can do on `/mcp`) |
| Client registration | Open DCR; public clients (`none`) and confidential (`client_secret_post` / `client_secret_basic`) |
| Enablement | On whenever `ECONUMO_URL` is set (it is the issuer and the resource origin); no extra flag. Unset = every endpoint below 404s and `/mcp` is unchanged |
| REST | Unchanged. OAuth-issued tokens are rejected on REST; sessions and PATs keep working on `/mcp` |

Out of scope (YAGNI): Client ID Metadata Documents (Claude falls back to DCR;
Codex uses DCR only), token introspection/revocation endpoints, read-only or
per-feature scopes, OpenID Connect (`/.well-known/openid-configuration`,
ID tokens).

## Package layout

`internal/authserver` is a feature package like the others (it imports no
other feature):

- `authserver/` — use cases: `register.go` (DCR), `authorize.go` (validate
  request, approve, decline), `token.go` (code exchange, refresh),
  `grants.go` (list, revoke, reclaim sweep), `redirect.go` (redirect-URI
  rules), `pkce.go`, `repository.go`, `ports.go`.
- `authserver/repo/` — engine-adapter repo (sqlite/pgsql adapters over sqlc).
- `authserver/api/` — authenticated `/api/v1/authserver/*` handlers (envelope).
- `authserver/oauthhttp/` — the public OAuth endpoints on the root mux
  (OAuth JSON errors, not the envelope) and the well-known documents.

It is distinct from `internal/oauth`, which is Econumo acting as a *client* of
Google / Apple / OIDC. Model types live in `internal/model/authserver.go` and
`authserver_dto.go`.

Ports (consumer-side interfaces, wired in `internal/server` via `glue_*.go`):

- `authserver` → user feature: `TokenIssuer` — mint an `oauth` access token
  fenced by credentials generation, revoke a grant's tokens; `UserLocker` —
  take the user row lock inside the caller's transaction and return the
  user's current credentials generation.
- user feature → `authserver`: `GrantRevoker` — revoke every grant of a user
  inside the caller's transaction (used by the reclaim and `user:deactivate`,
  same pattern as `OAuthReclaimer`).

## Data model

New migration (sqlite + pgsql; `.sql` files ASCII-only).

**`oauth_clients`**

| column | notes |
|---|---|
| `id` TEXT PK | the `client_id` (UUIDv7) |
| `name` TEXT NOT NULL | `client_name` from DCR, trimmed, 1–100 chars (default `"MCP client"` when absent) |
| `redirect_uris` TEXT NOT NULL | JSON array, 1–10 entries |
| `secret_hash` TEXT NULL | sha256 hex of the issued secret; NULL for public clients |
| `created_at` DATETIME NOT NULL | |
| `last_used_at` DATETIME NULL | set at first approval; NULL rows older than 30 days are purged (opportunistically on each registration) |

**`oauth_authorization_codes`**

| column | notes |
|---|---|
| `code_hash` TEXT PK | sha256 hex of the raw code (32 random bytes, base64url) |
| `client_id`, `user_id` | FKs, `ON DELETE CASCADE` |
| `redirect_uri` TEXT | exactly as presented at authorize |
| `code_challenge` TEXT | S256 challenge |
| `resource` TEXT | always the canonical `<ECONUMO_URL>/mcp` |
| `credentials_generation` INTEGER | user's generation at approval |
| `created_at`, `expires_at` DATETIME | 2-minute lifetime |

Consumed with a row-counted `DELETE ... WHERE code_hash = ?`: zero rows = already
used / never existed. Expired rows are purged opportunistically on exchange.

**`oauth_grants`**

| column | notes |
|---|---|
| `id` TEXT PK | UUIDv7 |
| `user_id`, `client_id` | FKs, `ON DELETE CASCADE` |
| `refresh_token_hash` TEXT UNIQUE | current refresh token |
| `prev_refresh_token_hash` TEXT NULL | the one it replaced (reuse detection) |
| `rotated_at` DATETIME NULL | when `prev_` was replaced |
| `created_at`, `last_used_at` DATETIME | |
| `expires_at` DATETIME | `last_used_at + 90d`, slides on refresh |
| `revoked_at` DATETIME NULL | |

Index on `prev_refresh_token_hash` and `user_id`. Dead grants (revoked/expired
> 30 days), dead `oauth` access tokens (same rule, one set-based DELETE) and
expired codes are purged best-effort after each successful code exchange.

**`access_tokens`** — new `kind = 'oauth'` (`model.TokenKindOAuth`), scope `mcp`, raw prefix
`eco_oat_`, `expires_at = created_at + 1h` fixed (never slides — `Touch` keeps
the existing session-only slide), `name` = the client name, and a new nullable
`grant_id` column (indexed) so a grant revoke can revoke its live access tokens
in one statement.

## Flows

### Discovery

- `GET /.well-known/oauth-protected-resource` and
  `GET /.well-known/oauth-protected-resource/mcp` (RFC 9728):
  `{"resource": "<URL>/mcp", "authorization_servers": ["<URL>"], "scopes_supported": ["mcp"], "bearer_methods_supported": ["header"]}`.
- `GET /.well-known/oauth-authorization-server` (RFC 8414):
  `issuer` = `<URL>`, `authorization_endpoint` = `<URL>/oauth/authorize`,
  `token_endpoint` = `<URL>/oauth/token`, `registration_endpoint` =
  `<URL>/oauth/register`, `response_types_supported: ["code"]`,
  `grant_types_supported: ["authorization_code","refresh_token"]`,
  `code_challenge_methods_supported: ["S256"]`,
  `token_endpoint_auth_methods_supported: ["none","client_secret_post","client_secret_basic"]`,
  `scopes_supported: ["mcp"]`.
- Unauthenticated / invalid-token `401` on `/mcp` adds
  `WWW-Authenticate: Bearer resource_metadata="<URL>/.well-known/oauth-protected-resource/mcp", scope="mcp"`.
  The body stays the existing frozen envelope. REST `401`s are unchanged.

`<URL>` is `ECONUMO_URL` with any trailing slash removed.

### Registration — `POST /oauth/register`

- JSON body per RFC 7591. Accepted fields: `client_name`, `redirect_uris`
  (required), `grant_types` (subset of `authorization_code`, `refresh_token`),
  `response_types` (`code`), `token_endpoint_auth_method`. Unknown fields are
  ignored.
- Redirect-URI rules (`redirect.go`): absolute, no fragment; either `https://`
  with any host, or `http://` on a loopback host (`127.0.0.1`, `[::1]`,
  `localhost`). Everything else → `400 invalid_redirect_uri`.
- `token_endpoint_auth_method` `none` (default) → no secret; `client_secret_post`
  / `client_secret_basic` → a 32-byte secret returned once, stored hashed.
- Response `201` with `client_id`, `client_id_issued_at`, `client_name`,
  `redirect_uris`, `grant_types`, `response_types`,
  `token_endpoint_auth_method`, and `client_secret` (+ `client_secret_expires_at: 0`)
  when issued.
- Behind the global per-endpoint rate limiter (`ECONUMO_RATE_LIMIT_GLOBAL`).

### Authorize + consent

`/oauth/authorize` is an SPA route (the Go catch-all serves `index.html`; the
global security headers already send `X-Frame-Options: DENY` /
`frame-ancestors 'none'`, so the consent page cannot be framed).

1. Signed out → the SPA routes to login preserving the full `/oauth/authorize?…`
   URL as the return target, and returns there after any sign-in path
   (password, provider sign-in, email-verification step).
2. Signed in → `GET /api/v1/authserver/get-authorization-request?<query>`
   validates: `client_id` exists; `redirect_uri` matches a registered URI
   (exact, except loopback URIs match ignoring the port, RFC 8252 §7.3);
   `response_type=code`; `code_challenge` present with
   `code_challenge_method=S256`; `resource` absent or equal to `<URL>/mcp`
   (trailing slash tolerated). Any requested `scope` is accepted and `mcp` is
   granted (clients invent scope names; refusing them only breaks the flow).
   - Unknown client or unmatched redirect URI → coded error; the SPA shows an
     error page and **never redirects**.
   - Any other failure → the response carries an error `redirectUrl`
     (`error=invalid_request|unsupported_response_type|invalid_target`, `state`
     echoed unless the state itself was rejected as too long);
     the SPA navigates there.
   - Success → `{clientName, redirectHost, isLoopback}`.
3. Consent page shows the client name, where the user will be sent (redirect
   host, or "an app on this computer" for loopback — the anti-phishing signal,
   since DCR lets anyone pick any name), a "full access to your Econumo data"
   statement, the signed-in email with "Not you? Switch account", Allow / Deny.
4. Allow → `POST /api/v1/authserver/approve-authorization` (same parameters)
   re-validates everything, then in one transaction takes the user row lock,
   checks the presenting session is still unrevoked (a reclaim committing after
   the auth middleware accepted it must not let the approval through: refused
   with the frozen 401), stores a code with the generation read under the lock
   and sets the client's `last_used_at`; returns
   `{redirectUrl}` = `redirect_uri?code=…&state=…&iss=<URL>`.
   Deny → `POST /api/v1/authserver/decline-authorization` returns the
   `error=access_denied` redirect. The SPA navigates to `redirectUrl`.
5. A `readonly` user gets the existing `402` on approve (consistent with `/mcp`
   being 402 for them); the page explains it.

### Token — `POST /oauth/token`

`application/x-www-form-urlencoded`. Client authentication: public clients send
`client_id`; confidential clients send `client_secret_post` or HTTP Basic, and
the secret is compared in constant time. Errors are RFC 6749 JSON
(`invalid_request`, `invalid_client` (401), `invalid_grant`,
`unsupported_grant_type`), `Cache-Control: no-store` on every response.

**`grant_type=authorization_code`** — `code`, `redirect_uri`, `code_verifier`,
optional `resource`:

1. Consume the code (row-counted delete); missing/expired → `invalid_grant`.
2. Check client match, `redirect_uri` equality, PKCE
   (`base64url(sha256(verifier)) == challenge`, constant-time), `resource`
   (absent or canonical).
3. In one transaction under the user row lock: create the grant (new refresh
   token), insert the access token via `InsertAccessTokenIfGeneration` with the
   code's captured generation. Zero rows (a reclaim landed after approval) →
   roll back, `invalid_grant`. Every other unrevoked grant the user holds for
   the same client is revoked with its access tokens, so re-authorizing an app
   replaces its connection instead of adding one.
4. Respond `{access_token, token_type: "Bearer", expires_in: 3600, refresh_token, scope: "mcp"}`.

**`grant_type=refresh_token`** — `refresh_token`, optional `resource` (must be
canonical if present) and `scope` (ignored; the grant stays `mcp`):

1. Look up by `refresh_token_hash`. Found, live, client matches → in one
   transaction under the user row lock (re-read the grant under the lock; it
   must still be unrevoked and still carry that hash): rotate (`prev_` ← current,
   new current, `rotated_at` = now), slide `expires_at`, insert the access token
   fenced by the generation read under the lock. Respond as above.
2. Not found by current hash but found by `prev_refresh_token_hash`:
   - within 60 s of `rotated_at` → `invalid_grant`, grant untouched (concurrent
     refresh from two processes sharing credentials, or a retried request whose
     response was lost);
   - otherwise → treat as theft: revoke the grant and its access tokens, log
     it, `invalid_grant`.
3. Otherwise → `invalid_grant`.

### Authentication on `/mcp` and REST

OAuth-issued access tokens carry the new token scope `mcp`
(`model.TokenScopeMCP`, not user-selectable for PATs). The auth middleware's
existing scope allowlist admits `mcp` only on the `/mcp` path; everywhere else
it is the frozen `401 Invalid access token`. The token was issued for the
`<URL>/mcp` resource, so REST must not honour it.

### Cascades

| Event | Grants | `oauth` access tokens |
|---|---|---|
| `reset-password`, CLI `user:change-password` (the reclaim) | all revoked, same transaction | all revoked |
| `user:deactivate` | all revoked, same transaction | all revoked |
| `update-password` | kept (integrations survive, like PATs) | kept |
| `revoke-connected-app` | that grant revoked | that grant's revoked |
| refresh-token theft detected | that grant revoked | that grant's revoked |

The reclaim and deactivate already bump the credentials generation, so codes
approved before them fail at exchange, and refreshes take the user row lock
the reclaim holds — no refresh can slip past a sweep.

### Settings API (authenticated, envelope)

- `GET /api/v1/authserver/get-connected-app-list` → live grants:
  `{id, clientName, redirectHost, isLoopback, createdAt, lastUsedAt}`.
- `POST /api/v1/authserver/revoke-connected-app` `{id}` — the caller's own
  grant only (others → not found). Added to `ReadonlyAllowedPaths`: revoking
  access is a security action.

## Logging

Operation lines with ids only (never client names, redirect URIs, codes or
tokens): `oauth-register` (`client_id`), `approve-authorization`
(`user_id`, `client_id`), `oauth-token` (`grant_type`, `client_id`,
`grant_id`, `user_id` on success), `oauth-refresh-reuse` (WARN, `grant_id`,
`revoked` bool), `revoke-connected-app` (`grant_id`).

## Frontend

`web/src/features/authserver/`:

- `ConsentPage` at `/oauth/authorize` — a small card usable on phone and
  desktop, states: loading, consent, invalid request (no redirect), readonly,
  submitting.
- Login return-to for the authorize URL across every sign-in path (reuse an
  existing mechanism if one exists, otherwise add one restricted to same-origin
  relative paths).
- Settings → "Connected apps" beside Personal tokens: list (name, redirect
  host, connected, last used) with Revoke; empty state explaining how to add
  Econumo to Claude / Codex with the `<URL>/mcp` address and a copy button.
- Metrics: `appConnectedAppApprove`, `appConnectedAppRevoke`, fired from the
  mutation hooks' `onSuccess`.
- i18n: `authserver.*` keys in all 11 catalogues.

## Error codes

New `errs` codes with `errors.*` catalogue entries (11 languages):
`authserver.disabled` (consent endpoints when `ECONUMO_URL` is unset — the
`/api/v1/authserver/*` routes are always registered),
`authserver.client_not_found`, `authserver.redirect_uri_mismatch`,
`authserver.grant_not_found`.

The consent API carries the OAuth parameters under camelCase names
(`clientId`, `redirectUri`, `responseType`, `codeChallenge`,
`codeChallengeMethod`, `resource`, `scope`, `state`) per the API conventions;
the SPA maps them from the snake_case `/oauth/authorize` query.

## Testing

- Unit: redirect-URI validation and matching (https exact, loopback any port,
  reject other schemes, fragments, userinfo); PKCE; refresh rotation, 60 s
  grace, theft revocation; DCR input validation.
- Integration (sqlite + `make test-repo-pgsql`): register → authorize →
  approve → exchange → `/mcp` tool call → refresh → revoke; code reuse,
  expired code, wrong verifier, wrong redirect URI, wrong resource, wrong
  client, bad secret; `oauth` token rejected on REST; reset-password between
  approval and exchange fails closed; reclaim and deactivate revoke grants;
  `update-password` keeps them; stale-client purge.
- `apiparity` scenarios + goldens for the four `/api/v1/authserver/*` routes.
- Root-mux OAuth endpoints: table-driven `httptest` suite (like `applinks`),
  including the `ECONUMO_URL`-unset 404s.
- `mcpparity`: golden for the `WWW-Authenticate` header on an unauthenticated
  `/mcp` request.
- `archtest` passes with the new feature package.
- Frontend vitest: ConsentPage (allow, deny, invalid, readonly, login
  return-to), Connected apps (list, revoke, empty state).
- Manual acceptance on a public URL: claude.ai custom connector, Claude Code
  (`claude mcp add --transport http econumo <URL>/mcp`), Codex
  (`codex mcp add` + `codex mcp login`).

## Docs

- `CLAUDE.md`: MCP endpoint section gains the authorization flow; `ECONUMO_URL`
  notes what it now enables; feature list gains `authserver`; token kinds gain
  `oauth`.
- New `docs/mcp-setup.md` (connecting Claude and Codex; PAT alternative).
- `docs/regression-test-plan.md`: consent page (📱), connected apps, revoke.
- Swagger annotations on the new `/api/v1/authserver/*` handlers.
