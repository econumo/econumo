> **Later decision (2026-11-04):** the first-party share client and the admin status route
> (`GET /admin/user-status`, `ECONUMO_ADMIN_SHARE_TOKEN`) described below were removed. Any OAuth
> client now gets the `read` scope by asking for it; see CLAUDE.md, "The `read` scope".

# Shared reports — econumo core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the share app (share.econumo.com) everything it needs from core: a read-only OAuth scope for one pre-registered client, dated exchange rates, referral capture and counts, an owner-status admin read, and the SPA entry points.

**Architecture:** The `read` scope rides the existing MCP OAuth server (`internal/authserver`): the share client is a config-seeded, first-party row in `oauth_clients`; codes and grants record their scope; `oauth` access tokens carry it; the shared auth middleware admits a `read` token only on a fixed GET allowlist. Referral is a nullable `users.referral` written once inside the new-user transaction on both the password and the provider path (carried on the `oauth_states` row). The remaining pieces are additive reads (rates by date range, referral counts, `/admin/user-status`) and SPA links gated on a new served `SHARE_URL` key.

**Tech Stack:** Go (stdlib net/http, sqlc per engine, SQLite + PostgreSQL), React 19 + Vite + TanStack Query, vitest, react-i18next.

**Spec:** `/home/dmitry/dev/econumo/share/docs/superpowers/specs/2026-11-01-shared-reports-design.md` — section 9 and Appendix A. Appendix A is a fixed contract the share team codes against; nothing here may change it.

## Global Constraints

- Read the repo `CLAUDE.md` first; its frozen wire contracts, dependency rule (features never import features; cross-feature adapters live in `internal/server/glue_*.go`) and comment rules apply to every task.
- Go: `PATH=/usr/local/go/bin:$PATH go …` (add `GOTOOLCHAIN=go1.27.1` if the local toolchain refuses). sqlc: `(cd internal/infra/storage/sqlc && ~/go/bin/sqlc generate)`. OpenAPI: `make swagger`.
- Every new or changed SQL query exists in BOTH `query/sqlite` and `query/pgsql`; every migration exists in BOTH `migrations/sqlite` and `migrations/pgsql` with the same version. `.sql` files ASCII-only.
- Every changed API response regenerates goldens with `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/` (and `./internal/test/mcpparity/` when MCP output changes), then the diff is INSPECTED; never hand-edit a golden.
- Every new i18n key goes into all 11 `locales/<lang>.json` (de en es fr it nl pl pt ru uk zh), translated, not English copies; error codes go into `internal/shared/errs/codes.go` (`AllCodes`) and `errors.*`.
- Every new user-facing SPA action fires a `METRICS` event at its success point.
- Error/field messages in `en` are exact strings asserted by tests; copy them verbatim from this plan.
- Contract values (verbatim from Appendix A): client display name `Econumo Share`; scope string `read`; referral pattern `^[A-Za-z0-9_-]{1,32}$`; referral-stats cap 100; rate span cap 366 days; date format `YYYY-MM-DD`; 401 message `Invalid access token`; served key `SHARE_URL` always present, `""` default.
- `read` allowlist (GET only, exact paths): `/api/v1/user/get-user-data`, `/api/v1/account/get-account-list`, `/api/v1/category/get-category-list`, `/api/v1/tag/get-tag-list`, `/api/v1/label/get-label-list`, `/api/v1/currency/get-currency-list`, `/api/v1/currency/get-currency-rate-list`, `/api/v1/transaction/get-account-transaction-list`, `/api/v1/transaction/get-transaction-list`, `/api/v1/user/get-referral-stats`.
- Before each commit: `make go-lint` passes (build, vet, gofmt, OpenAPI fresh). Before the final task's commit: `make go-test` and `make web-test` and `make web-lint` pass.
- Branch: `feature/share-reports-core` (already checked out). Commit messages end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

### Decisions this plan makes (where the spec left a builder's call)

1. A self-registered client asking for `scope=read` is granted `mcp`, exactly as today (no `invalid_scope`). The configured share client is ALWAYS granted `read`, whatever it requests; it can never obtain `mcp`. Discovery metadata keeps advertising only `mcp`.
2. `ECONUMO_SHARE_CLIENT_ID` must be a UUID (the `oauth_clients.id` column is `UUID` on PostgreSQL). `ECONUMO_SHARE_CLIENT_SECRET` must be at least 32 characters.
3. The share client row is upserted at boot with `first_party = true`. A first-party row that is not the currently configured share client (config removed or id changed) is treated as an unknown client everywhere (authorize, token, refresh) so its secret cannot buy an `mcp` grant.
4. The authorization `resource` rule is unchanged: absent or `<ECONUMO_URL>/mcp`. The share app omits it.
5. Turning the share client off (unset its env) stops admitting already-issued `read` tokens (a gate in the server's authenticator decorator), mirroring how disabling OAuth stops `mcp` tokens.
6. A `readonly` user may approve a `read`-scope request (approving writes no user data; spec §4 wants readonly owners to reach their existing reports). `approve-authorization` joins `ReadonlyAllowedPaths`, and the service refuses an `mcp` approval for a non-full caller with the same 402 envelope the middleware used to send, so the existing readonly golden stays byte-identical.
7. Dated rates: in the ranged form, a custom currency's fixed rate and the synthetic base/base row are dated `from` (they hold on every day of the range); in the unranged form a custom rate is dated by the currency's creation day and the synthetic base row by the newest real rate's day. Validation: `from`/`to` both or neither (missing one → `common.is_blank` on it), not `YYYY-MM-DD` → `common.invalid_format`, `to` < `from` → new `currency.rate_range_order` on `to`, inclusive day count > 366 → new `currency.rate_range_too_long` on `to`.
8. `get-referral-stats`: `refs` absent or blank → `{"items":[]}`; more than 100 comma-separated entries → field error `refs` / `common.too_long`; invalid entries are dropped; duplicates collapse to the first occurrence; order follows the request. Counts include every user whose `referral` equals the ref (active or not).
9. `ECONUMO_ADMIN_SHARE_TOKEN` requires the admin pair (`ECONUMO_ADMIN_PORT` + `ECONUMO_ADMIN_TOKEN`) and must differ from `ECONUMO_ADMIN_TOKEN`; both fail at boot otherwise. `/admin/user-status` answers in the standard envelope like every admin route (`{"success":true,"message":"","data":{"userId":…,"active":…}}`).
10. `SHARE_URL` follows the `mcpUrl()` rule in the SPA: on the web it describes the instance serving the page, so it reads `""` when the SPA talks to a custom backend on another origin; the native app does not merge it (its bundled fallback is `""`).

## Review Focus

1. A `read` token on anything outside the allowlist — a write (`POST /api/v1/tag/create-tag`), a non-allowlisted read (`GET /api/v1/user/get-session-list`), `HEAD` on an allowlisted path, `/mcp`, an ingest route — must get the frozen 401 `Invalid access token`, never 402/200. Pinned in Task 2 (middleware table) and Task 3 (apiparity `read_scope` scenario through the real handler).
2. A first-party client row left behind after the share config is removed or its id changes must not authorize, exchange or refresh (otherwise the share secret buys an `mcp` grant). Pinned in Task 3 (`TestStaleFirstPartyClientIsUnknown`).
3. A `read` grant refreshed many times must keep issuing `read` tokens and answer `scope: "read"`, and a self-registered client sending `scope=read` must still get `mcp`. Pinned in Task 3 (`TestShareClientRefreshKeepsReadScope`, `TestDynamicClientAskingReadGetsMCP`).
4. Referral on the provider path: an oversized, malformed or absent `ref`, a `ref` on `start-link`, and an EXISTING user signing in with a `ref` must all leave `users.referral` untouched. Pinned in Task 5 (`TestStartLoginReferral*`, `TestProvisionStoresReferral_ExistingUserUnaffected`).
5. Dated rates around the window edge: a currency whose only rates predate `from` still appears once (its latest earlier rate); a rate published exactly on `from` counts as in-range and the earlier one is still included; a range of exactly 366 inclusive days is accepted, 367 refused. Pinned in Task 4 (`TestGetCurrencyRateHistory_*`, `TestCurrencyRateListRequest_Validate`).

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/config/config.go` | 5 new env vars + validation |
| `internal/model/token.go` | `TokenScopeRead` |
| `internal/model/authserver.go`, `authserver_dto.go` | client `FirstParty`, code/grant `Scope`, consent result `scope`/`isFirstParty` |
| `internal/model/referral.go` (new) | `NormalizeReferral`, stats request/result DTOs |
| `internal/model/currency_dto.go`, `currency_view.go` | rate `date`, `CurrencyRateListRequest` |
| `internal/model/admin*.go` | `AdminUserStatusResult` |
| `internal/web/middleware/auth.go` | `ReadScopePaths`, read-scope case, `AccessLevelFromCtx`, approve in readonly allowlist |
| `internal/web/middleware/adminauth.go` | share-token-aware `AdminAuth` |
| `internal/authserver/share.go` (new) | share client config, seeding, granted-scope rule |
| `internal/authserver/{authorize,token,repository}.go`, `repo/*` | scope on codes/grants, first-party handling, readonly approve rule |
| `internal/server/glue_readscope.go` (new) | read-scope gate decorator |
| `internal/currency/read.go`, `repo/read.go`, `api/currency.go` | dated rates |
| `internal/user/register.go`, `external.go`, `read.go`, `api/referral.go` (new) | referral write + stats |
| `internal/oauth/start.go`, `callback.go`, `ports.go`, `repo/*` | referral through state row |
| `internal/admin/*`, `internal/server/glue_admin.go` | user-status |
| `internal/web/router/router.go` | `SHARE_URL` served key |
| `internal/infra/storage/migrations/{sqlite,pgsql}/20261101000000.sql`, `20261101000001.sql` | oauth scope columns; referral columns |
| `web/src/lib/config.ts`, `web/public/econumo-config.js` | `shareUrl()` |
| `web/src/features/share/shareLinks.ts` (new) | deep links + analytics |
| `web/src/lib/referral.ts` (new) | `?ref=` reading |
| `web/src/features/{classifications/TagsPage,budgets/classificationMenu,settings/SettingsPage,auth/*,authserver/ConsentPage}.tsx` | entry points, ref passing, consent wording |

---

### Task 1: Configuration

**Files:**
- Modify: `internal/config/config.go`, `.env.example`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces on `config.Config`:
  - `ShareURL string` (ECONUMO_SHARE_URL, trailing `/` trimmed)
  - `ShareClientID, ShareClientSecret, ShareRedirectURI string`
  - `AdminShareToken string` (ECONUMO_ADMIN_SHARE_TOKEN)
  - `func (c Config) ShareClientEnabled() bool` (= `ShareClientID != ""`)

- [ ] **Step 1: Write the failing tests** — table test `TestLoad_ShareConfig` using `t.Setenv` on top of the minimum env the existing `TestLoad_*` tests set:

```go
// ok cases
{"share url https", env{"ECONUMO_SHARE_URL": "https://share.econumo.com/"}, wantShareURL: "https://share.econumo.com"},
{"share url loopback http", env{"ECONUMO_SHARE_URL": "http://localhost:8282"}, wantShareURL: "http://localhost:8282"},
{"client triple", env{"ECONUMO_URL": "https://app.example.test", "ECONUMO_SHARE_CLIENT_ID": "0b9c2a52-6a8e-4f1e-9d33-6d6c1b2b7f10",
  "ECONUMO_SHARE_CLIENT_SECRET": strings.Repeat("s", 32), "ECONUMO_SHARE_REDIRECT_URI": "https://share.example.test/auth/callback"}, wantClient: true},
{"admin share token", env{"ECONUMO_ADMIN_PORT": "9090", "ECONUMO_ADMIN_TOKEN": strings.Repeat("a", 32), "ECONUMO_ADMIN_SHARE_TOKEN": strings.Repeat("b", 32)}},
// error cases (wantErr substring)
{"share url http remote", env{"ECONUMO_SHARE_URL": "http://share.example.test"}, wantErr: "ECONUMO_SHARE_URL"},
{"share url relative", env{"ECONUMO_SHARE_URL": "share.example.test"}, wantErr: "ECONUMO_SHARE_URL"},
{"share url query", env{"ECONUMO_SHARE_URL": "https://share.example.test/?x=1"}, wantErr: "ECONUMO_SHARE_URL"},
{"client half set", env{"ECONUMO_SHARE_CLIENT_ID": "0b9c2a52-…"}, wantErr: "ECONUMO_SHARE_CLIENT_SECRET"},
{"client id not uuid", env{…triple with ID "econumo-share"}, wantErr: "ECONUMO_SHARE_CLIENT_ID"},
{"client secret short", env{…triple with 31-char secret}, wantErr: "ECONUMO_SHARE_CLIENT_SECRET"},
{"client without app url", env{…triple, no ECONUMO_URL}, wantErr: "ECONUMO_URL"},
{"redirect not https", env{…triple, REDIRECT "http://share.example.test/cb"}, wantErr: "ECONUMO_SHARE_REDIRECT_URI"},
{"admin share token without admin pair", env{"ECONUMO_ADMIN_SHARE_TOKEN": strings.Repeat("b", 32)}, wantErr: "ECONUMO_ADMIN_SHARE_TOKEN"},
{"admin share token short", env{admin pair, share token 31 chars}, wantErr: "ECONUMO_ADMIN_SHARE_TOKEN"},
{"admin share token equals admin token", env{admin pair, share token == admin token}, wantErr: "ECONUMO_ADMIN_SHARE_TOKEN"},
```
Unset everything → `ShareURL == ""`, `ShareClientEnabled() == false`, `AdminShareToken == ""`.

- [ ] **Step 2: Run, expect FAIL** — `go test ./internal/config/ -run TestLoad_ShareConfig -v` → compile error (fields missing).

- [ ] **Step 3: Implement** in `Load()` after the `ECONUMO_URL` block (so `c.AppURL` is known) a `loadShare(&c) error` helper:
  - Share URL: `url.Parse`; scheme `https`, or `http` only when `isLoopbackHost(u.Hostname())`; non-empty host; no user, query, fragment; store `strings.TrimRight(v, "/")`.
  - Client triple all-or-nothing (error names the first missing variable, like `loadOAuth`); `vo.ParseId` on the id; `len(secret) >= 32`; `ECONUMO_URL` required; redirect URI: absolute, `https` or loopback `http`, ASCII host (the full check is repeated by `authserver.ValidateRedirectURI` at Build).
  - `ECONUMO_ADMIN_SHARE_TOKEN`: requires `c.AdminToken != ""` (message names `ECONUMO_ADMIN_TOKEN`), `len >= 32`, `!= c.AdminToken`.
  - Add the five variables, commented out with one-line explanations, to `.env.example` next to `ECONUMO_URL`.

- [ ] **Step 4: Run, expect PASS** — same command; then `go test ./internal/config/`.

- [ ] **Step 5: Commit** — `git commit -m "feat: Add share-app configuration"`.

---

### Task 2: The `read` token scope and its route allowlist

**Files:**
- Modify: `internal/model/token.go`, `internal/web/middleware/auth.go`, `internal/user/oauth_tokens.go`, `internal/user/repo/accesstoken.go`, `internal/authserver/ports.go`, `internal/authserver/token.go` (call sites only), every fake of `authserver.Credentials` in `internal/authserver/*_test.go`
- Test: `internal/web/middleware/auth_scope_test.go`, `internal/user/oauth_tokens_test.go`, `internal/model/token_test.go`

**Interfaces:**
- Produces:
  - `model.TokenScopeRead TokenScope = "read"`; `model.ParseTokenScope` still rejects it.
  - `middleware.ReadScopePaths map[string]bool` — exactly the 10 allowlist paths (Global Constraints).
  - `middleware.AccessLevelFromCtx(ctx context.Context) (model.AccessLevel, bool)` — the authenticated principal's effective level, stored by `AuthWith` next to user id and token id.
  - `authserver.Credentials.IssueOAuthAccessToken(ctx context.Context, userID, grantID vo.Id, name string, scope model.TokenScope, generation int64, ttl time.Duration) (string, bool, error)`; `*user.Service` implements it. Callers in `token.go` pass `model.TokenScopeMCP` for now (Task 3 passes the grant's scope).

- [ ] **Step 1: Write the failing tests**
  - In `TestAuth_ScopeGate` add rows (all via `middleware.Auth`):
    ```go
    {"read on allowlisted GET", model.TokenScopeRead, http.MethodGet, "/api/v1/tag/get-tag-list", http.StatusOK},
    {"read on referral stats", model.TokenScopeRead, http.MethodGet, "/api/v1/user/get-referral-stats", http.StatusOK},
    {"read on non-allowlisted GET", model.TokenScopeRead, http.MethodGet, "/api/v1/user/get-session-list", http.StatusUnauthorized},
    {"read POST on allowlisted path", model.TokenScopeRead, http.MethodPost, "/api/v1/tag/get-tag-list", http.StatusUnauthorized},
    {"read POST write", model.TokenScopeRead, http.MethodPost, "/api/v1/tag/create-tag", http.StatusUnauthorized},
    {"read HEAD", model.TokenScopeRead, http.MethodHead, "/api/v1/tag/get-tag-list", http.StatusUnauthorized},
    {"read on ingest", model.TokenScopeRead, http.MethodPost, "/api/v1/import/ingest-apple-wallet", http.StatusUnauthorized},
    ```
    The existing assertion that every 401 body carries `"message":"Invalid access token"` covers the frozen text.
  - In `TestAuthWith_MCPScopeNeedsAllowMCPScope` add `{"read token on /mcp, allowed", model.TokenScopeRead, "/mcp", true, http.StatusUnauthorized}`.
  - `TestReadScopePaths_ExactContract`: `len(middleware.ReadScopePaths) == 10` and each of the 10 contract paths is present.
  - `TestAuth_StoresAccessLevel`: a `scopedAuthn` variant returning `Level: model.AccessLevelReadonly`, GET request; the handler reads `middleware.AccessLevelFromCtx` and gets `(model.AccessLevelReadonly, true)`.
  - `internal/user/oauth_tokens_test.go`: `TestIssueOAuthAccessToken_ReadScope` — issue with `model.TokenScopeRead`, then `Authenticate` the raw token returns `Principal.Scope == model.TokenScopeRead`; issuing with `model.TokenScopeFull` returns an error (repo guard).
  - `internal/model/token_test.go`: `ParseTokenScope("read")` errors.

- [ ] **Step 2: Run, expect FAIL** — `go test ./internal/web/middleware/ ./internal/user/ ./internal/model/ -run 'Scope|AccessLevel|IssueOAuth' -v`.

- [ ] **Step 3: Implement**
  - `auth.go`: add `case p.Scope == model.TokenScopeRead && r.Method == http.MethodGet && ReadScopePaths[r.URL.Path]:` to the scope switch; update the switch comment to "four known scopes"; put `level` into ctx under a new private key and add `AccessLevelFromCtx`. Export comment on `ReadScopePaths` says the apiparity guard asserts each path is a registered GET route (Task 3 adds that guard).
  - `user/oauth_tokens.go`: the new `scope` parameter replaces the hard-coded `model.TokenScopeMCP`.
  - `user/repo/accesstoken.go` `InsertOAuthIfGeneration`: accept `t.Scope` of `TokenScopeMCP` or `TokenScopeRead`, error otherwise.
  - `authserver/ports.go` + `token.go` call sites + test fakes: new signature, pass `model.TokenScopeMCP`.

- [ ] **Step 4: Run, expect PASS** — same command, then `go test ./internal/authserver/... ./internal/user/... ./internal/web/...`.

- [ ] **Step 5: Commit** — `git commit -m "feat: Add the read token scope and its GET allowlist"`.

---

### Task 3: The share client in the authorization server

**Files:**
- Create: `internal/authserver/share.go`, `internal/server/glue_readscope.go`, `internal/infra/storage/migrations/{sqlite,pgsql}/20261101000000.sql`
- Modify: `internal/model/authserver.go`, `internal/authserver/{repository,authorize,token}.go`, `internal/authserver/repo/{repo,sqlite,pgsql}.go`, `internal/infra/storage/sqlc/query/{sqlite,pgsql}/oauth_server.sql` (+ regenerate), `internal/server/server.go`, `internal/test/fixture/*` (oauth client `FirstParty`, codes/grants `Scope`), `internal/test/apiparity/{harness,fixture,catalogue_authserver,guard_test}.go`
- Test: `internal/authserver/share_test.go`, `internal/authserver/repo/repo_integration_test.go`, `internal/server/glue_readscope_test.go`, apiparity goldens

**Interfaces:**
- Consumes: Task 1 config fields; Task 2 `model.TokenScopeRead`, `IssueOAuthAccessToken(…, scope, …)`.
- Produces:
  - `model.OAuthClient.FirstParty bool`; `model.OAuthAuthorizationCode.Scope model.TokenScope`; `model.OAuthGrant.Scope model.TokenScope`.
  - `authserver.ScopeRead = "read"` (existing `authserver.Scope = "mcp"` unchanged); `authserver.ShareClientName = "Econumo Share"`.
  - `type authserver.ShareClient struct { ID vo.Id; Secret, RedirectURI string }`
  - `func (s *Service) ConfigureShareClient(ctx context.Context, sc ShareClient) error` — error when `!s.Enabled()` ("ECONUMO_SHARE_CLIENT_ID requires the MCP OAuth server: ECONUMO_URL must be an https origin with no path") or when `ValidateRedirectURI(sc.RedirectURI)` fails; otherwise upserts the row and remembers `sc.ID`.
  - `func (s *Service) ShareClientEnabled() bool`
  - `func (s *Service) grantedScope(c *model.OAuthClient) model.TokenScope` — `TokenScopeRead` for the configured share client, `TokenScopeMCP` otherwise.
  - `Repository.UpsertFirstPartyClient(ctx context.Context, c *model.OAuthClient) error` — sqlc `UpsertFirstPartyOAuthClient`: INSERT (… `first_party` true, `last_used_at` = `created_at` so the unused-client purge never takes it) ON CONFLICT (id) DO UPDATE SET name, redirect_uris, secret_hash, first_party.
  - `func server.NewReadScopeGate(inner middleware.TokenAuthenticator, enabled bool) middleware.TokenAuthenticator` — refuses a `read` principal with `errs.NewUnauthorized("Invalid access token")` when `!enabled`; passes everything else through untouched; forwards `middleware.StoredLanguageResolver` to `inner` when `inner` implements it.

- [ ] **Step 1: Migration** `20261101000000.sql`, both engines:
  ```sql
  -- The read scope (the share app): a first-party client is seeded from config,
  -- and each code and grant records the scope its access tokens carry.
  ALTER TABLE oauth_clients ADD COLUMN first_party BOOLEAN DEFAULT '0' NOT NULL;      -- pgsql: DEFAULT false
  ALTER TABLE oauth_authorization_codes ADD COLUMN scope TEXT NOT NULL DEFAULT 'mcp';
  ALTER TABLE oauth_grants ADD COLUMN scope TEXT NOT NULL DEFAULT 'mcp';
  ```
  Add `first_party` to `InsertOAuthClient`/`GetOAuthClient`, `scope` to `InsertOAuthCode`/`ConsumeOAuthCode` RETURNING, and to every grant SELECT/INSERT in `oauth_server.sql` (both engines); add `UpsertFirstPartyOAuthClient`; regenerate sqlc; map the new fields in `repo.go` and the pgsql shim.

- [ ] **Step 2: Write the failing tests** (`internal/authserver/share_test.go`, using the helpers in `helpers_test.go`; service built with an https issuer)
  - `TestConfigureShareClient_RequiresEnabledServer`: issuer `http://app.example.test` (disabled) → error containing `ECONUMO_URL`.
  - `TestConfigureShareClient_RejectsBadRedirect`: `http://share.example.test/cb` → error.
  - `TestConfigureShareClient_Upserts`: called twice with a changed secret and redirect → one row, `FirstParty`, `Name == "Econumo Share"`, new secret hash, `LastUsedAt != nil`.
  - `TestShareClientFlow_GrantsRead`: approve (scope param `mcp` on purpose) → exchange with `client_secret` → `TokenResponse.Scope == "read"`; the access token authenticates with `Principal.Scope == model.TokenScopeRead`; the stored grant has `Scope == read`.
  - `TestShareClientRequiresSecret`: exchange without/with wrong `client_secret` → `invalid_client` (401).
  - `TestShareClientRefreshKeepsReadScope`: three refreshes in a row each return `scope == "read"` and a `read` token.
  - `TestDynamicClientAskingReadGetsMCP`: a `Register`ed public client with `scope=read` on authorize → token `scope == "mcp"`, principal scope `mcp`.
  - `TestStaleFirstPartyClientIsUnknown`: seed a first-party row with id A, then build a service whose configured share client is B (and one with none): `DescribeAuthorization` for A → `CodeAuthServerClientNotFound` validation error; `Token` (both grant types) for A → `invalid_client`.
  - Repo integration (`repo_integration_test.go`): upsert/get round-trip of `FirstParty`; code and grant `Scope` round-trip; `PurgeUnusedClients` with a far-future cutoff does NOT delete the seeded first-party row.
  - `internal/server/glue_readscope_test.go`: `TestReadScopeGate` — enabled=false refuses a `read` principal with the frozen message and passes `full`/`mcp`; enabled=true passes `read`; `var _ middleware.StoredLanguageResolver` holds for the gate when the inner implements it and the call is forwarded.

- [ ] **Step 3: Run, expect FAIL** — `go test ./internal/authserver/... ./internal/server/ -run 'Share|FirstParty|ReadScope|DynamicClient|Stale' -v`.

- [ ] **Step 4: Implement**
  - `share.go`: `ShareClient`, `ConfigureShareClient`, `ShareClientEnabled`, `grantedScope`, plus `func (s *Service) usableClient(c *model.OAuthClient) bool` (false for a first-party row whose id is not the configured share client id); `Service` gains `share *vo.Id`.
  - `authorize.go` `validate` and `token.go` `authenticateClient`: after `GetClient`, `!s.usableClient(c)` behaves exactly like not-found (`clientNotFound()` / `errBadClient`).
  - `ApproveAuthorization`: `Scope: s.grantedScope(c)` on the code; `Resource` stays `s.ResourceURL()`.
  - `exchange`: grant `Scope: code.Scope`; `IssueOAuthAccessToken(…, code.Scope, …)`; `tokenResponse(access, refresh, string(code.Scope))`.
  - `refresh`: `IssueOAuthAccessToken(…, g.Scope, …)`; `tokenResponse(…, string(g.Scope))`. `tokenResponse` gains the `scope string` parameter.
  - `server.go`: right after `authSrv` is built, `if cfg.ShareClientEnabled() { id, _ := vo.ParseId(cfg.ShareClientID); if err := authSrv.ConfigureShareClient(context.Background(), appauthserver.ShareClient{…}); err != nil { return nil, nil, nil, err } }`; then `authn := NewReadScopeGate(NewTimezoneTrackingAuthenticator(userSvc, userSvc), authSrv.ShareClientEnabled())`.

- [ ] **Step 5: apiparity**
  - Harness `cfg`: `ShareClientID: "e1000000-0000-0000-0000-0000000000a1"`, `ShareClientSecret: "share-secret-0123456789abcdef0123456789"`, `ShareRedirectURI: "https://share.example.test/auth/callback"`.
  - Fixture: export `ShareClientID` and `ReadToken`/`ReadTokenID`; seed an `oauth`-kind access token for `OwnerID` with scope `read` (no grant), live expiry like the owner session. Harness auth switch gains `case "read": tok = ReadToken`; document `"read"` in the `Call.Auth` comment.
  - New scenario `read_scope` in `catalogue_authserver.go`: `"read"`-auth GETs of the 9 allowlisted paths that already exist (`get-referral-stats` and the ranged rate call are added by Tasks 6 and 4), then `err:` calls: `GET /api/v1/user/get-session-list`, `POST /api/v1/tag/create-tag` (valid body), `POST /api/v1/user/update-name`, `GET /api/v1/budget/get-budget-list`.
  - Guard in `guard_test.go`: `TestReadScopePathsAreRegisteredGETRoutes` — every key of `middleware.ReadScopePaths` is a scanned `GET` route (mirror the existing `ReadonlyAllowedPaths` guard). It fails until Task 6 adds `get-referral-stats`; mark that single path with a `t.Skip`-free allow-list entry `pendingReadScopePaths = map[string]bool{"/api/v1/user/get-referral-stats": true}` that Task 6 deletes.
  - `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`; inspect: only the new `read_scope` golden appears; no existing golden changes.

- [ ] **Step 6: Run** — `go test ./internal/authserver/... ./internal/server/... ./internal/test/apiparity/ ./internal/infra/storage/migrations/...` → PASS.

- [ ] **Step 7: Commit** — `git commit -m "feat: Seed the share client and issue read-scoped tokens"`.

---

### Task 3b: Consent shows the scope; readonly users may approve `read`

**Files:**
- Modify: `internal/model/authserver_dto.go`, `internal/authserver/authorize.go`, `internal/authserver/api/authserver.go`, `internal/web/middleware/auth.go` (`ReadonlyAllowedPaths`), `web/src/api/authserver.ts`, `web/src/features/authserver/ConsentPage.tsx`, `locales/*.json`, `internal/test/apiparity/catalogue_authserver.go`
- Test: `internal/authserver/authorize_test.go`, `web/src/features/authserver/ConsentPage.test.tsx`, apiparity goldens

**Interfaces:**
- Consumes: Task 2 `middleware.AccessLevelFromCtx`; Task 3 `grantedScope`.
- Produces:
  - `model.AuthorizationRequestResult` gains `Scope string \`json:"scope"\`` (`"mcp"` | `"read"`) and `IsFirstParty bool \`json:"isFirstParty"\``.
  - `func (s *Service) ApproveAuthorization(ctx context.Context, userID, tokenID vo.Id, fullAccess bool, req model.AuthorizationRequest) (model.AuthorizationDecisionResult, error)` — when `!fullAccess && s.grantedScope(c) != model.TokenScopeRead` returns `&errs.PaymentRequiredError{Msg: "Read-only access. Write operations are disabled.", Code: errs.CodeReadonlyAccess}` before any write.
  - SPA `AuthorizationRequestResultDto` gains `scope: 'mcp' | 'read'`, `isFirstParty: boolean`.
  - i18n (en): `authserver.consent.wantsRead` = "wants to read your Econumo data."; `authserver.consent.readAccess` = "It will be able to see your accounts, transactions, categories, tags, labels and currencies. It can't change anything."; `authserver.consent.firstParty` = "Made by Econumo".

- [ ] **Step 1: Write the failing tests**
  - Go: `TestDescribeAuthorization_ReportsScope` — share client → `Scope == "read"`, `IsFirstParty == true`; dynamic client → `"mcp"`, `false`; `TestApprove_ReadonlyMayApproveRead` — share client, `fullAccess=false` → redirect URL with `code=`; `TestApprove_ReadonlyRefusedForMCP` — dynamic client, `fullAccess=false` → `*errs.PaymentRequiredError` with `Code == errs.CodeReadonlyAccess`, and no code row inserted.
  - Vitest `ConsentPage.test.tsx`: with a `get-authorization-request` mock returning `scope: 'read', isFirstParty: true, clientName: 'Econumo Share'` the page shows "wants to read your Econumo data.", the read-access sentence and "Made by Econumo", and NOT "It will be able to read and change all of your Econumo data."; with `scope: 'mcp', isFirstParty: false` the existing texts show and "Made by Econumo" does not.

- [ ] **Step 2: Run, expect FAIL** — `go test ./internal/authserver/ -run 'ReportsScope|Readonly' -v`; `cd web && pnpm vitest run src/features/authserver/ConsentPage.test.tsx`.

- [ ] **Step 3: Implement**
  - `DescribeAuthorization` fills `Scope`/`IsFirstParty`. Handler `ApproveAuthorization`: `level, _ := middleware.AccessLevelFromCtx(ctx); fullAccess := level == model.AccessLevelFull` (fail closed on an unknown level, as the middleware does).
  - Add `"/api/v1/authserver/approve-authorization": true` to `ReadonlyAllowedPaths`, extending its doc comment with one sentence: the service itself refuses every non-`read` approval for a restricted caller with the same 402.
  - ConsentPage: `wantsRead`/`readAccess` replace `wantsAccess`/`fullAccess` when `result.scope === 'read'`; a small muted "Made by Econumo" line under the client name when `result.isFirstParty`. The existing 402 branch (driven by the approve response) needs no change.
  - Translate the three keys into all 11 catalogues.

- [ ] **Step 4: apiparity** — in `authserver_consent` add `get-authorization-request-share-client` (owner) and `approve-authorization-share-client-readonly` (readonly, share client, redirect `https://share.example.test/auth/callback`). `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`; inspect: existing describe responses gain only `"scope":"mcp","isFirstParty":false`; `err:approve-authorization-readonly` is byte-identical; the readonly share approval succeeds with a redacted code.

- [ ] **Step 5: Run** — `go test ./internal/authserver/... ./internal/web/middleware/ ./internal/test/apiparity/ ./internal/test/i18ntest/`; `cd web && pnpm vitest run src/features/authserver`; `make swagger`.

- [ ] **Step 6: Commit** — `git commit -m "feat: Show the requested scope on the consent page"`.

---

### Task 4: Exchange rates by date range

**Files:**
- Modify: `internal/model/currency_dto.go`, `internal/model/currency_view.go`, `internal/currency/read.go`, `internal/currency/repo/read.go` (+ pgsql shim), `internal/infra/storage/sqlc/query/{sqlite,pgsql}/currency_read.sql` (+ regenerate), `internal/currency/api/currency.go`, `internal/shared/errs/codes.go`, `locales/*.json`, `internal/test/apiparity/catalogue.go` (or a new `catalogue_currency_rates.go`), `internal/test/apiparity/catalogue_authserver.go` (`read_scope` gains the ranged call)
- Test: `internal/model/currency_dto_test.go`, `internal/currency/read_scoping_test.go` (or new `read_history_test.go`), `internal/currency/repo/lookup_read_integration_test.go`, `internal/currency/api/currency_endpoints_test.go`, apiparity + mcpparity goldens

**Interfaces:**
- Produces:
  - `model.CurrencyRateResult.Date string \`json:"date"\`` (`YYYY-MM-DD`); `model.CurrencyRateViewRow.Date string`.
  - `type model.CurrencyRateListRequest struct { From, To string; HasFrom, HasTo bool }` with `Validate() error` and `Range() (from, to time.Time, ok bool)` (`ok` false when neither is given); `model.MaxCurrencyRateRangeDays = 366`.
  - `errs.CodeCurrencyRateRangeOrder = "currency.rate_range_order"` (en "The end date must not be before the start date"), `errs.CodeCurrencyRateRangeTooLong = "currency.rate_range_too_long"` (en "The date range must be at most {max} days", params `max: 366`).
  - `currency.ReadModel.CurrencyRateRangeView(ctx context.Context, from, to time.Time) ([]model.CurrencyRateViewRow, error)`.
  - `func (s *ReadService) GetCurrencyRateHistory(ctx context.Context, userID vo.Id, from, to time.Time) (*model.GetCurrencyRateListResult, error)`.
  - sqlc `GetCurrencyRateRangeView :many` — rows with `published_at` in `[from, to]` UNION the per-(currency, base) row with the greatest `published_at < from`, ordered `currency_id, base_currency_id, published_at`. SQLite compares with `date(published_at)` against `'Y-m-d'` strings (like `GetAverageCurrencyRates`); PostgreSQL with `$n::date`.

- [ ] **Step 1: Write the failing tests**
  - `TestCurrencyRateListRequest_Validate` (table):
    - neither → nil, `Range()` ok=false;
    - `from` only → field `to`, code `common.is_blank`, message "This value should not be blank."; `to` only → field `from`, same;
    - `from=2026-13-01` → field `from`, `common.invalid_format`, "This value is not valid."; `from=2026-1-5` (not zero-padded) → same;
    - `from=2026-03-02&to=2026-03-01` → field `to`, `currency.rate_range_order`;
    - `2024-01-01..2024-12-31` (366 inclusive days) → nil; `2023-01-01..2024-01-01` (366) → nil; `2023-01-01..2024-01-02` (367) → field `to`, `currency.rate_range_too_long`;
    - `from == to` → nil.
  - Service (`ReadModel` fake) `TestGetCurrencyRateHistory_*`:
    - `_IncludesLatestBeforeFrom`: EUR rates on 2026-02-20, 2026-02-27, 2026-03-03; range 2026-03-01..2026-03-31 → EUR items dated `2026-02-27` and `2026-03-03` only.
    - `_OnlyOlderRates`: GBP only on 2026-01-10 → one GBP item dated `2026-01-10`.
    - `_RateOnFrom`: rates on 2026-02-27 and 2026-03-01, range from 2026-03-01 → both present.
    - `_FiltersInvisible`: a rate for a currency not in the user's visible list is dropped.
    - `_CustomAndBaseDatedFrom`: an own custom currency (rate 2.5) → one item, `date == from`, `rate == "2.5"`; no stored base row → synthetic base item rate `"1"`, `date == from`.
  - `TestGetCurrencyRateList_DateField`: unranged form — each stored rate's `Date` equals its `UpdatedAt[:10]`; a custom currency's `Date` is its creation day.
  - Repo integration (both engines via `dbtest.New`): `CurrencyRateRangeView` over seeded rows returns the expected `(currency, date)` pairs for the `_IncludesLatestBeforeFrom` data.
  - Endpoint test: `GET …/get-currency-rate-list?from=2026-03-01&to=2026-03-31` → 200 with `date` on every item; `?from=2026-03-01` → 400 envelope with `errors.to`.

- [ ] **Step 2: Run, expect FAIL** — `go test ./internal/model/ ./internal/currency/... -run 'RateList|RateHistory|RateRange|DateField' -v`.

- [ ] **Step 3: Implement**
  - DTO + validation; date parse with `time.Parse("2006-01-02", s)` and a `t.Format` round-trip equality check (rejects `2026-1-5`); inclusive days = `int(to.Sub(from).Hours()/24) + 1`.
  - Repo: format `Date` as `published_at` `Y-m-d`; also populate `Date` in `LatestCurrencyRateListView`.
  - Service: `GetCurrencyRateList` sets `Date` (stored rows, customs by `CreatedAt`, synthetic base by `latest[:10]`); `GetCurrencyRateHistory` reuses the visibility filter and the custom/base synthesis with `date = from.Format("2006-01-02")` and `updatedAt = from` formatted with `datetime.Layout`.
  - Handler: hand-written (query params), following `transaction/api/accounttransactionlist.go`: build the request from `q.Has("from")`/`q.Has("to")`, `Validate`, branch on `Range()`. Swag: add `@Param from query string false "Start day (YYYY-MM-DD); requires to"` and `@Param to …`, `@Failure 400`, update `@Description`.
  - Codes into `AllCodes`; translate both keys in all 11 catalogues.

- [ ] **Step 4: Goldens** — add apiparity `currency_rate_range` scenario (owner): plain call, a 31-day range, `err:` cases from-only, bad format, reversed, 367 days; add the ranged call to `read_scope`. `UPDATE_GOLDEN=1` for apiparity AND mcpparity; inspect: existing rate items gain only `"date"`; `list_currencies` MCP golden gains only `"date"`.

- [ ] **Step 5: Run** — `go test ./internal/currency/... ./internal/model/ ./internal/test/apiparity/ ./internal/test/mcpparity/ ./internal/test/i18ntest/`; `make swagger`. If `DATABASE_TEST_PGSQL_URL` is set, also `make test-repo-pgsql` for the repo test.

- [ ] **Step 6: Commit** — `git commit -m "feat: Serve exchange rates for a date range"`.

---

### Task 5: Referral captured at sign-up (password and provider paths)
> **Moved to the separate referral-program PR (user decision 2026-11-01).** Not implemented on this branch; kept here as the starting design for that PR (WIP parked on branch `feature/referral-program`).


**Files:**
- Create: `internal/model/referral.go`, `internal/infra/storage/migrations/{sqlite,pgsql}/20261101000001.sql`
- Modify: `internal/model/user_dto.go` (`RegisterRequest.Ref`), `internal/model/oauth_dto.go` (`StartOAuthRequest.Ref`), `internal/model/oauth.go` (`OAuthState.Referral`), `internal/user/{register,external,repository}.go`, `internal/user/repo/*` + `users.sql` (both engines), `internal/oauth/{start,callback,ports}.go`, `internal/oauth/repo/*` + `oauth.sql` (both engines), `internal/server/glue_oauth_users.go`, `internal/user/api/user.go` + `internal/oauth/api/*.go` (swag annotations only)
- Test: `internal/model/referral_test.go`, `internal/user/register_test.go`, `internal/oauth/service_test.go`, apiparity

**Interfaces:**
- Produces:
  - `func model.NormalizeReferral(raw string) string` — returns `raw` when it matches `^[A-Za-z0-9_-]{1,32}$`, else `""` (no trimming).
  - `RegisterRequest.Ref string \`json:"ref"\``; `StartOAuthRequest.Ref string \`json:"ref"\``; neither `Validate` looks at it.
  - `model.OAuthState.Referral string` (column `oauth_states.referral TEXT NOT NULL DEFAULT ''`).
  - `user.Repository.SetReferral(ctx context.Context, id vo.Id, ref string) error` — sqlc `SetUserReferral`: `UPDATE users SET referral = ? WHERE id = ? AND referral IS NULL`.
  - `func (s *Service) ProvisionExternalUser(ctx context.Context, name, email, referral string) (*model.User, error)`; `oauth.Users.ProvisionExternal(ctx, name, email, referral string)`.
  - `persistNewUser` gains a trailing `referral string`; it calls `SetReferral` inside the same `WithTx` as `Save` when non-empty. `createUser` (CLI path) passes `""`.
- Migration `20261101000001.sql` (both engines):
  ```sql
  -- Referral: the share report a user signed up from (an opaque public report
  -- id), written once at registration. The oauth state carries it across the
  -- provider round trip.
  ALTER TABLE users ADD COLUMN referral TEXT DEFAULT NULL;
  CREATE INDEX IDX_users_referral ON users (referral);
  ALTER TABLE oauth_states ADD COLUMN referral TEXT NOT NULL DEFAULT '';
  ```

- [ ] **Step 1: Write the failing tests**
  - `TestNormalizeReferral`: `"abcDEF_12-x"` → same; `""`, `" abc"`, `"abc!"`, `"a/b"`, `strings.Repeat("a", 33)`, `"ref%20x"`, `"рус"` → `""`; `strings.Repeat("a", 32)` → same.
  - `internal/user/register_test.go`:
    - `TestRegisterStoresReferral`: register with `Ref: "Abc_123"` → `SELECT referral FROM users WHERE id = ?` (via `db.Rebind`) = `"Abc_123"`.
    - `TestRegisterIgnoresInvalidReferral`: `Ref: "bad ref"` → registration succeeds, `referral IS NULL`.
    - `TestRegisterDuplicateEmailLeavesReferral`: existing user with referral `"first"`, second register for the same email with `Ref: "second"` → "User already exists", stored value still `"first"`.
    - `TestAdminCreateUserHasNoReferral`: CLI path → `referral IS NULL`.
  - `internal/oauth/service_test.go` (fake IdP harness already there):
    - `TestStartLoginReferralStoredOnState`: `StartLogin` with `Ref: "r1"` → state row `Referral == "r1"`; with `Ref: strings.Repeat("x", 33)` → `""`; with `Ref: "a b"` → `""`; absent → `""`.
    - `TestStartLinkIgnoresReferral`: `StartLink` with `Ref: "r1"` → state `Referral == ""`.
    - `TestProvisionStoresReferral`: full start-login → callback for a new email → new user's `referral == "r1"`.
    - `TestProvisionStoresReferral_ExistingUserUnaffected`: same flow for an email/identity that already exists (passwordless user with referral NULL) → still NULL.
    - `TestForgedStateCarriesNoReferral`: callback with an unknown state → error redirect, no user created.
  - apiparity: in `catalogue_user.go` add `register-user-with-ref` (`ref: "rep0000001"`) and `register-user-with-invalid-ref` (`ref: "not valid!"`), both 200; in `catalogue_oauth.go` add `start-login-with-ref` and `start-login-with-oversized-ref` (both 200).

- [ ] **Step 2: Run, expect FAIL** — `go test ./internal/model/ ./internal/user/ ./internal/oauth/ -run 'Referral' -v`.

- [ ] **Step 3: Implement** — migration; sqlc queries (`SetUserReferral`; `referral` in `InsertOAuthState`/`GetOAuthState`; both engines) and regenerate; `start()` stores `model.NormalizeReferral(req.Ref)` only when `intent == model.OAuthIntentLogin`; `callback.go` step 7 passes `st.Referral`; `Register` passes `model.NormalizeReferral(req.Ref)`; glue passes through. Swag: the request structs are already the documented bodies; regenerate.

- [ ] **Step 4: Goldens** — `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`; inspect: only the new calls appear (responses identical in shape to the existing register/start-login ones).

- [ ] **Step 5: Run** — `go test ./internal/user/... ./internal/oauth/... ./internal/test/apiparity/ ./internal/infra/storage/migrations/... ./internal/infra/storage/sqliteimport/...`; `make swagger`.

- [ ] **Step 6: Commit** — `git commit -m "feat: Record the referral a user signed up from"`.

---

### Task 6: `get-referral-stats`
> **Moved to the separate referral-program PR (user decision 2026-11-01).** Not implemented on this branch; kept here as the starting design for that PR (WIP parked on branch `feature/referral-program`).


**Files:**
- Create: `internal/user/api/referral.go`
- Modify: `internal/model/referral.go`, `internal/user/read.go`, `internal/user/repo/read*.go` + `user_read.sql` (both engines), `internal/user/api/routes.go`, `internal/test/apiparity/{catalogue_user,catalogue_authserver,guard_test}.go`
- Test: `internal/model/referral_test.go`, `internal/user/read_test.go`, `internal/user/api/user_endpoints_test.go`, apiparity

**Interfaces:**
- Consumes: Task 5 `NormalizeReferral`, `users.referral`.
- Produces:
  - `type model.ReferralStatsRequest struct { Refs string; Provided bool }`, `Validate()` (more than `model.MaxReferralStatsRefs = 100` comma-separated entries → field `refs`, `common.too_long`, "This value is too long."), `Valid() []string` (normalized, invalid dropped, deduped, request order).
  - `type model.ReferralStatsItem struct { Ref string \`json:"ref"\`; Signups int64 \`json:"signups"\` }`; `type model.ReferralStatsResult struct { Items []ReferralStatsItem \`json:"items"\` }` (`items` never `null`).
  - `user.ReadModel.CountUsersByReferral(ctx context.Context, ref string) (int64, error)` — sqlc `CountUsersByReferral :one` (`SELECT COUNT(*) FROM users WHERE referral = ?`; one indexed query per ref, at most 100).
  - `func (s *ReadService) GetReferralStats(ctx context.Context, req model.ReferralStatsRequest) (*model.ReferralStatsResult, error)`.
  - Route `GET /api/v1/user/get-referral-stats` (auth), hand-written handler `GetReferralStats`.

- [ ] **Step 1: Write the failing tests**
  - `TestReferralStatsRequest`: absent → `Valid()` empty; `"a,b,a, ,bad!,c"` → `["a","b","c"]`; 101 entries → `Validate` error on `refs`; 100 entries → nil.
  - `TestGetReferralStats`: users with referral `r1` ×2, `r2` ×1 (one of them inactive) → request `r1,r2,r3` → `[{r1,2},{r2,1},{r3,0}]`; absent → `{"items":[]}` (assert JSON).
  - Endpoint: `?refs=` (blank) → 200 `{"items":[]}`; 101 refs → 400 with `errors.refs`.
  - apiparity: `referral_stats` scenario (owner): after a `register-user` with `ref: "rep0000001"`, `get-referral-stats?refs=rep0000001,unknown0001,bad!` → `[{"ref":"rep0000001","signups":1},{"ref":"unknown0001","signups":0}]`; `err:` 101 refs; add `get-referral-stats` to the `read_scope` scenario; delete `pendingReadScopePaths` from the Task 3 guard.

- [ ] **Step 2: Run, expect FAIL** — `go test ./internal/model/ ./internal/user/... -run 'ReferralStats' -v`.

- [ ] **Step 3: Implement** — DTOs, query (both engines, regenerate), service, handler (modelled on `accounttransactionlist.go`; `reqctx.AddLogAttr(ctx, "refs", len(valid))`), route, swag block (`@Param refs query string false "Comma-separated report ids, at most 100"`).

- [ ] **Step 4: Goldens + run** — `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`, inspect (only new calls); `go test ./internal/user/... ./internal/test/apiparity/`; `make swagger`.

- [ ] **Step 5: Commit** — `git commit -m "feat: Add referral sign-up counts"`.

---

### Task 7: `/admin/user-status` and the share admin token

**Files:**
- Modify: `internal/web/middleware/adminauth.go`, `internal/admin/{ports,service}.go` (+ new `status.go`), `internal/admin/api/{handler,routes}.go`, `internal/model/admin*.go`, `internal/server/{glue_admin,server}.go`, `internal/test/apiparity/admin_reachability_test.go`
- Test: `internal/web/middleware/adminauth_test.go`, `internal/admin/admin_test.go`, `internal/admin/api/handler_test.go`, `internal/server/glue_admin_test.go`

**Interfaces:**
- Consumes: Task 1 `cfg.AdminShareToken`.
- Produces:
  - `func middleware.AdminAuth(token, shareToken string) Middleware` — the main token on every route; `shareToken` (when non-empty) only for `GET` on `middleware.AdminShareStatusPath = "/admin/user-status"`; anything else → 401 `Invalid access token`. Constant-time compares for both.
  - `admin.UserRecord.IsActive bool` (filled from `model.User.IsActive` in `glue_admin.go`'s `record`).
  - `type model.AdminUserStatusResult struct { UserID string \`json:"userId"\`; Active bool \`json:"active"\` }`.
  - `func (s *admin.Service) UserStatus(ctx context.Context, userID vo.Id) (*model.AdminUserStatusResult, error)` — `errs.NotFound` passes through (the handler maps it to a real 404); logs `user_id`.
  - Route `GET /admin/user-status?userId=<uuid>`; handler `UserStatus` mirrors `UserContext`'s blank/malformed `userId` validation.

- [ ] **Step 1: Write the failing tests**
  - `adminauth_test.go` `TestAdminAuthShareToken` (table, main `M`, share `S`): `S` + GET `/admin/user-status` → 200; `S` + GET `/admin/user-context` → 401; `S` + POST `/admin/set-access` → 401; `S` + POST `/admin/user-status` → 401; `M` + GET `/admin/user-status` → 200; share token `""` configured and bearer `""`/missing → 401; every 401 body has `"message":"Invalid access token"`.
  - `TestUserStatus`: active user → `{UserID, Active: true}`; after deactivate → `Active: false`; unknown id → not-found error.
  - Handler: unknown user → 404; missing `userId` → 400; OK → envelope `{"success":true,"message":"","data":{"userId":"…","active":true}}` (assert exact JSON).
  - `admin_reachability_test.go`: add `GET /admin/user-status?userId=` + `OwnerID` and `/api/v1/admin/user-status` to both lists.

- [ ] **Step 2: Run, expect FAIL** — `go test ./internal/web/middleware/ ./internal/admin/... ./internal/test/apiparity/ -run 'Admin|UserStatus' -v`.

- [ ] **Step 3: Implement** — middleware, record field, service, handler, route; `server.go` passes `middleware.AdminAuth(cfg.AdminToken, cfg.AdminShareToken)`. Audit line: the access log already records the operation; add `reqctx.AddLogAttr(ctx, "active", res.Active)`.

- [ ] **Step 4: Run, expect PASS**; then `go test ./internal/admin/... ./internal/server/... ./internal/web/...`.

- [ ] **Step 5: Commit** — `git commit -m "feat: Add the owner-status admin read for the share app"`.

---

### Task 8: `SHARE_URL` and the SPA entry points

**Files:**
- Create: `web/src/features/share/shareLinks.ts`, `web/src/features/share/shareLinks.test.ts`
- Modify: `internal/web/router/router.go`, `internal/web/router/router_test.go`, `web/public/econumo-config.js`, `web/src/lib/config.ts`, `web/src/lib/metrics.ts`, `web/src/features/classifications/TagsPage.tsx`, `web/src/features/budgets/classificationMenu.tsx`, `web/src/features/settings/SettingsPage.tsx`, `locales/*.json`
- Test: `router_test.go`, `shareLinks.test.ts`, `TagsPage.test.tsx`, `SettingsPage.test.tsx`, a budget menu test next to the existing `classificationMenu` coverage (e.g. `BudgetTable.test.tsx`)

**Interfaces:**
- Consumes: Task 1 `cfg.ShareURL`.
- Produces:
  - Served key `"SHARE_URL": deps.Cfg.ShareURL` (always present).
  - `EconumoConfig.SHARE_URL?: string`; `export function shareUrl(): string` in `config.ts` — same origin rule as `mcpUrl()` (`""` when `backendHost()` is another origin; native app returns the raw value, which is the bundled `''`).
  - `shareLinks.ts`: `shareTripUrl(tagId: string): string` → `` `${shareUrl()}/new?kind=trip&tag=${encodeURIComponent(tagId)}` `` (`''` when no share URL); `sharedReportsUrl(): string` → `` `${shareUrl()}/app` ``; `openShareTrip(tagId: string, source: 'tags' | 'budget'): void` and `openSharedReports(): void` — `window.open(url, '_blank', 'noopener,noreferrer')` then `trackEvent`.
  - `METRICS.SHARE_TRIP_OPEN = 'appShareTripOpen'` (eventData `{ source }`), `METRICS.SHARED_REPORTS_OPEN = 'appSharedReportsOpen'`.
  - i18n (en): `share.tag_action` = "Share as trip page"; `share.settings_row` = "Shared reports".

- [ ] **Step 1: Write the failing tests**
  - `router_test.go`: the exact served-document assertion gains `"SHARE_URL":""` (default) — and a second case with `Cfg.ShareURL = "https://share.example.test"` contains `"SHARE_URL":"https://share.example.test"`.
  - `shareLinks.test.ts`: with `window.econumoConfig.SHARE_URL = 'https://share.example.test'` → `shareTripUrl('a b')` = `'https://share.example.test/new?kind=trip&tag=a%20b'`, `sharedReportsUrl()` = `'https://share.example.test/app'`; `openShareTrip('t1','budget')` calls `window.open` with the trip URL and `trackEvent(METRICS.SHARE_TRIP_OPEN, { source: 'budget' })`; with `SHARE_URL: ''` both URLs are `''`; with a custom backend on another origin `shareUrl()` is `''`.
  - `TagsPage.test.tsx`: a tag row's ⋮ menu shows "Share as trip page" only when `SHARE_URL` is set and only for tags (not labels); selecting it opens the trip URL for that tag id.
  - Budget: a tag line's ⋮ menu shows "Share as trip page" (enabled for own and shared tags) when `SHARE_URL` is set; absent otherwise; category lines never show it.
  - `SettingsPage.test.tsx`: "Shared reports" row in the Data group only when `SHARE_URL` is set; click opens `…/app` and fires `SHARED_REPORTS_OPEN`.

- [ ] **Step 2: Run, expect FAIL** — `go test ./internal/web/router/ -v`; `cd web && pnpm vitest run src/features/share src/features/classifications/TagsPage.test.tsx src/features/settings/SettingsPage.test.tsx src/features/budgets`.

- [ ] **Step 3: Implement**
  - Router key with a one-line comment (present even when empty, like `BILLING_URL`); `econumo-config.js` fallback `SHARE_URL: ''`.
  - TagsPage `extraActions`: append `{ label: t('share.tag_action'), onSelect: () => openShareTrip(row.id, 'tags') }` when `row.kind === 'tag' && shareTripUrl(row.id) !== ''`.
  - `classificationMenu.tsx` `actionsFor`: append the same action (source `'budget'`) for `ref.kind === 'tag'`, never disabled.
  - SettingsPage Data group: `MenuRow` "Shared reports" with an `ExternalLink` trailing icon, `onClick={openSharedReports}`, rendered when `sharedReportsUrl() !== ''`.
  - Translate both keys into all 11 catalogues.

- [ ] **Step 4: Run, expect PASS** — the commands above plus `cd web && pnpm vitest run src/lib/metrics-coverage.test.ts` and `go test ./internal/test/i18ntest/`.

- [ ] **Step 5: Commit** — `git commit -m "feat: Link tags and Settings to shared reports"`.

---

### Task 9: SPA passes `?ref=` on register and provider sign-in
> **Moved to the separate referral-program PR (user decision 2026-11-01).** Not implemented on this branch; kept here as the starting design for that PR (WIP parked on branch `feature/referral-program`).


**Files:**
- Create: `web/src/lib/referral.ts`, `web/src/lib/referral.test.ts`
- Modify: `web/src/api/user.ts`, `web/src/api/oauth.ts`, `web/src/features/auth/queries.ts`, `web/src/features/auth/oauthQueries.ts`, `web/src/features/auth/RegistrationPage.tsx`, `web/src/features/auth/ProviderButtons.tsx`
- Test: `referral.test.ts`, `RegistrationPage.test.tsx`, `ProviderButtons.test.tsx`, `LoginPage.test.tsx`

**Interfaces:**
- Consumes: Task 5 request fields `ref`.
- Produces:
  - `referral.ts`: `export function referralFrom(params: URLSearchParams): string | undefined` — the `ref` value when it matches `/^[A-Za-z0-9_-]{1,32}$/`, else `undefined`.
  - `register(email, password, name, ref?: string)` sends `ref` only when defined; `startLogin(provider, client, ref?: string)` likewise.
  - `useRegister` variables gain `ref?: string`; `useStartOAuth` variables gain `ref?: string` (used only for `intent === 'login'`).

- [ ] **Step 1: Write the failing tests**
  - `referralFrom`: `?ref=rep_01` → `'rep_01'`; `?ref=bad!`, 33 chars, absent → `undefined`.
  - RegistrationPage at `/register?ref=rep0000001&utm_source=share`: submitting posts `{email, password, name, ref: 'rep0000001'}` (msw request-body assertion); at `/register` the body has no `ref` key.
  - ProviderButtons on `/register?ref=rep0000001` and `/login?ref=rep0000001`: clicking a provider posts `start-login` with `ref: 'rep0000001'`; with `intent="link"` (Settings) no `ref` is sent.

- [ ] **Step 2: Run, expect FAIL** — `cd web && pnpm vitest run src/lib/referral.test.ts src/features/auth`.

- [ ] **Step 3: Implement** — `RegistrationPage` and `ProviderButtons` read `useSearchParams()` and pass `referralFrom(params)`. No new metric: registration and provider login already fire `USER_REGISTRATION` / `OAUTH_LOGIN_COMPLETED`.

- [ ] **Step 4: Run, expect PASS**; `cd web && pnpm lint`.

- [ ] **Step 5: Commit** — `git commit -m "feat: Pass the share referral through sign-up"`.

---

### Task 10: Documentation and full verification

**Files:**
- Modify: `CLAUDE.md`, `docs/regression-test-plan.md`, `docs/mcp-setup.md` (one short "First-party share client" note), `.env.example` (verify Task 1 entries)

- [ ] **Step 1: CLAUDE.md** — update, in place and in the existing style:
  - MCP endpoint / MCP OAuth: the `read` scope, the share client (config-seeded first-party row, always granted `read`, stale first-party rows treated as unknown, self-registered clients asking `read` get `mcp`), the `ReadScopePaths` allowlist, the read-scope gate when the share client is off, readonly users may approve `read`.
  - Authentication: token scopes list gains `read` (`oauth` tokens of the share client; GET allowlist only; frozen 401 elsewhere).
  - Configuration: `ECONUMO_SHARE_URL`, `ECONUMO_SHARE_CLIENT_ID`/`_SECRET`/`_REDIRECT_URI`, `ECONUMO_ADMIN_SHARE_TOKEN` (rules from Task 1); admin listener bullet gains `GET /admin/user-status` and the share token's single-route scope.
  - Web UI config: `SHARE_URL` joins the always-present keys; `shareUrl()` origin rule.
  - API conventions: `get-currency-rate-list?from&to` (rules from Decision 7) and `get-referral-stats`.
  - Registration / provider sign-in: `ref` on `register-user` and `start-login`, stored once in `users.referral`, carried on `oauth_states.referral`.
- [ ] **Step 2: Regression plan** — add verifiable items: §2 register via `/register?ref=…` and provider sign-up from `/login?ref=…` record the referral (check with `get-referral-stats`); §7 Tags page ⋮ "Share as trip page" opens `SHARE_URL/new?kind=trip&tag=<id>` in a new tab, hidden without `ECONUMO_SHARE_URL` 📱; §9 budget tag line ⋮ same action 📱; §12a consent for the share client shows "wants to read your Econumo data." and "Made by Econumo", a readonly user can approve it, the app appears in Connected apps and revoking it stops the share app; §13 Settings → Data "Shared reports" row.
- [ ] **Step 3: Full verification**
  - `make go-test` → PASS (includes OpenAPI freshness, goldens, i18n guards, coverage gate).
  - If PostgreSQL is available (`DATABASE_TEST_PGSQL_URL`), `make test-repo-pgsql` and `go test -tags enginecompare ./internal/test/enginecompare/ ./internal/test/mcpparity/` → PASS.
  - `make web-test` and `make web-lint` → PASS.
- [ ] **Step 4: Commit** — `git commit -m "docs: Document the share-reports core changes"`.
