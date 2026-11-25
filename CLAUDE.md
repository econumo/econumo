# Repository Guidelines

Guidance for AI agents working in this repository. Keep this file compact: rules, invariants
and pointers. Feature-level detail lives in the code and in `docs/superpowers/specs/`.

## Project Overview

Econumo is a self-hosted personal finance and budgeting application.

- **Backend**: Go (JSON API + embedded SPA server), vertical feature packages; the Go module is
  the repo root.
- **Frontend**: React 19 + Vite + TypeScript SPA (Tailwind 4 / shadcn, TanStack Query, Zustand)
  in `web/`.
- **Database**: SQLite (default) or PostgreSQL, selected at runtime by `DATABASE_URL`.

The production artifact is one self-contained Go binary in a distroless image
(`ghcr.io/econumo/econumo`) that serves the API and the SPA (`go:embed`, `web/embed.go`) and
runs migrations on boot. Releases also attach `econumo-linux-{amd64,arm64}` + `SHA256SUMS`.
There is a single edition.

## Development Commands

```bash
make go-test         # SMOKE: build + vet + gofmt + OpenAPI-docs-fresh + sqlite tests + coverage gate (GO_COVER_MIN, 80)
make test            # FULL: go-test + sqlite-vs-PostgreSQL suites + release upgrade suite + frontend suite
make test-repo-pgsql # repo/unit suite against PostgreSQL
make test-upgrade    # upgrade databases written by published release binaries to this checkout
make ai-eval         # AI prompts against a real model (needs ECONUMO_AI_DSN; never part of go-test)
make go-build        # compile ./econumo (regenerates OpenAPI docs first)
make go-run          # run the server locally (reads .env)
make go-lint         # build + vet + gofmt + OpenAPI-docs-fresh check
make swagger         # regenerate the committed OpenAPI docs

go test ./...
go run ./cmd/econumo serve
go run ./cmd/econumo user:create "Name" user@example.test secret

make web-install | web-run | web-test | web-lint | web-bundle   # pnpm install | dev (:9000, proxies /api to :8181) | vitest | oxlint | build
```

`make test` auto-provisions PostgreSQL through the compose stack, or set
`DATABASE_TEST_PGSQL_URL`.

### Publishing

- `make publish-dev`: the `dev` channel on the private R2 bucket. `dev` and `edge` are R2
  channels only; neither has a docker image.
- **Edge**: every push to `main` that changes the binary runs
  `.github/workflows/publish-edge.yml` (pushes touching only `docs/`, `.claude/` or Markdown
  outside the embedded `prompts/` publish nothing) and moves the `edge` R2 channel;
  production follows `edge`. No tag, no GitHub release. An edge build reports the
  highest published GitHub release as its `VERSION` (`.github/scripts/latest-release.sh`), so
  What's new, the update notice and new-account seeding behave as on that release; only
  `VERSION_REF` (the commit) tells edge builds apart. A release of the newest version
  re-dispatches Publish Edge, so `edge` is always main's head.
- **Releases** (`latest` + `vX.Y.Z`) are cut only by `.github/workflows/publish-release.yml`;
  see the `publish-release` skill. A release from `main` auto-creates `release/vX.Y.Z`;
  hotfixes dispatch from a `release/*` branch copied by hand. `latest` moves only for the
  highest version.
- **R2 (ship)**: the same workflows publish the linux binaries, `deploy/ship/econumo.service`
  and `deploy/ship/econumo.env.example` with [ship](https://github.com/dmtrkzntsv/ship):
  `make ship-dist` stages `dist/ship/` (`SHIP_SKIP_BUILD=1` reuses `release-out/`),
  `make ship-publish VERSION=… CHANNELS=…` publishes. The unit is a TEMPLATE: only ship's
  placeholders (`{{SERVICE}} {{USER}} {{BIN}} {{BIN_DIR}} {{ENV_FILE}} {{DATA_DIR}} {{RUN_DIR}}
  {{UNIT}}`), and no `DynamicUser=`/`StateDirectory=`/`RuntimeDirectory=`. Credentials:
  `SHIP_R2_ENDPOINT`, `SHIP_R2_ACCESS_KEY_ID`, `SHIP_R2_SECRET_ACCESS_KEY`.

### Branch naming

- `feature/<slug>`: the default for every change that is not a bug fix (chores, docs,
  refactors, CI included).
- `bug/<slug>`: bug fixes.
- `release/vX.Y.Z`: reserved for releases, never for development.

## Rules for every change

- **Regression plan**: any change to user-observable behavior updates the affected items of
  `docs/regression-test-plan.md` in the same PR (verifiable outcomes, accurate 📱 markers).
- **Analytics**: every new user-facing feature/action fires an event: add a key to `METRICS`
  (`web/src/lib/metrics.ts`, `app`-prefixed camelCase) and call `trackEvent` at the success
  point, preferably in the shared hook. `metrics-coverage.test.ts` fails on a key never fired.
- **Offline**: a new write control reads `useOnline` (or is listed with a reason in
  `OFFLINE_EXEMPT` / `OFFLINE_VIA_SHARED`, `web/src/lib/offline-coverage.test.ts`); a routed
  page shows `OfflineNotice` or is listed in `offline-notice-coverage.test.ts`.
- **Attention items** go to the Inbox (`web/src/features/inbox`, `useInbox()`), never a new
  top banner (reserved for subscription, server-version and install notices).
- **i18n**: every catalogue in `locales/` carries every key (`en` is the reference). Plurals
  are one pipe-joined value read by `pluralPick`, not i18next suffixes.
- **Env vars**: a new variable is documented in `docs/full.env.example` (and `.env.example`
  only if most setups need it). App-owned config is prefixed `ECONUMO_`.
- **Goldens**: regenerate with `UPDATE_GOLDEN=1 go test ./internal/test/{apiparity,mcpparity}/`,
  then inspect the diff. Never hand-edit a golden.
- **Transaction writes** bump the account revision and assign sort keys (see Database);
  `revisionguard` fails otherwise.
- **AI prompts** obey the data allowlist (see AI integration); each prompt builder has a guard
  test on its exact key set.
- **Comments**: only exceptional scenarios and non-obvious business or frozen-contract
  rationale (the why). No comments restating a name or signature, no section dividers. Leave
  Swagger `// @…` blocks and `// Code generated … DO NOT EDIT.` markers intact.

## Architecture

### Backend layout

```
cmd/econumo/main.go      entrypoint: serve / healthcheck / resource:action commands
internal/
  shared/                dependency-free kernel: vo, errs, datetime, port, reqctx, sortkey
  model/                 every feature's entities, value objects and request/result DTOs
                         (<feature>.go, <feature>_dto.go); imports only shared
  <feature>/             behavior only: <verb>.go use cases on a Service, repository.go
                         (interfaces), ports.go (capabilities other features provide)
    repo/                repository implementation (engine-adapter pattern)
    api/                 HTTP handlers + routes.go (RegisterAPI)
    mcp/                 MCP tools (ten features)
  infra/                 storage (sqlc, migrations), operation, idempotency, auth, clock, ai,
                         mailer, i18n, ratelimit, oidc, instance, txrevision, txorder, handoff
  web/                   HTTP edge infra: middleware, router, httpx (envelope), endpoint, spa,
                         apidoc, mcp
  server/                composition root: server.BuildAPI wires every feature (binary AND
                         tests); glue_*.go hold the cross-feature adapters
  cli/ config/ logging/ version/ test/
```

Features (18): `account`, `admin`, `authserver`, `budget`, `category`, `connection`,
`currency`, `debt`, `imports`, `label`, `oauth`, `onboarding`, `payee`, `recurring`, `system`,
`tag`, `transaction`, `user`. `authserver` is Econumo as an OAuth SERVER for MCP clients;
`oauth` is Econumo as a CLIENT of Google/Apple/OIDC. `imports` (not `import`, a keyword) keeps
each provider in a subpackage (`applewallet`, `simplefin`) registered in
`internal/server/server.go`. Reports are hosted by `budget` (`report_*.go`).

**Dependency rule**: features never import features. The consuming feature declares a small
interface in its own package (typed in `model`) and `internal/server` wires an adapter
(`glue_<feature>_<purpose>.go`). Shared leaves never import a feature; `model` depends only on
`shared`. Enforced by `internal/test/archtest`, which auto-detects feature packages.

**Engine-adapter (sqlc) pattern**: sqlc generates one package per engine (`gen/sqlite`,
`gen/pgsql`). A repo declares a `querier` interface in the canonical (sqlite) types, has two
thin adapters (`sqlite.go` passthrough, `pgsql.go` conversion shim) chosen once in the
constructor, and writes each method once with no per-driver branching. Reference repos:
`internal/{tag,user,currency}/repo`. Exception: `internal/budget/repo/read.go` is hand-built
dynamic SQL and branches by design.

**API handlers** are thin named methods (the name carries the swag block) calling
`internal/web/endpoint`: `Handle` (auth + JSON body), `HandleNoBody`, `HandlePublic`, which do
require-user/decode/`Validate()`/call/envelope in a frozen order. Hand-written exceptions: CSV
export, multipart import, query-param reads, login's raw `{token,user}`.

**Design decisions**: one binary with both engines linked in; SQLite is the reference engine
and PostgreSQL must match byte-for-byte; stdlib-first (`net/http.ServeMux`, plain middleware,
hand-written `Validate()`, stdlib CLI); no assembler layer (services return result DTOs).

### MCP endpoint and OAuth server

`/mcp` is a second HTTP edge on the root mux, outside `/api`: the official Go SDK's Streamable
HTTP handler in stateless JSON-response mode, same bearer auth as REST. Shared infra in
`internal/web/mcp/`; features register tools from `internal/<feature>/mcp/`. Its golden suite
is `internal/test/mcpparity/`.

`internal/authserver` is a hand-written OAuth 2.1 server so MCP clients connect by URL (guide
`docs/mcp-setup.md`, spec `2026-10-03-mcp-oauth-design.md`):

- Enabled only when `ECONUMO_URL` is an `https://` origin with no path (plain http only on
  loopback). Otherwise the five public routes answer a JSON 404 and already-issued OAuth
  tokens stop being admitted.
- Dynamic client registration (`POST /oauth/register`), PKCE S256 only, consent at the SPA
  route `/oauth/authorize` (the SPA calls `get-authorization-request` then
  `approve-authorization`/`decline-authorization`). Token errors are RFC 6749 JSON, not the
  envelope.
- Scopes: requested scopes containing `read` and not `mcp` grant `read`; anything else grants
  `mcp`. `mcp` tokens are admitted only on `/mcp`; `read` tokens only on `GET` of the nine
  paths in `middleware.ReadScopePaths`. Everything else gets the frozen 401.
- Access tokens are `eco_oat_` rows, fixed 1 h; refresh tokens rotate, a replay of an old one
  revokes the grant (the most recent one within 60 s is only refused). A new code exchange
  replaces the user's other grants for that client, except for a global app, whose grants
  for one user coexist (two Claude accounts on one Econumo account) up to
  `MaxGlobalGrantsPerUser`, beyond which the least recently used one is revoked.
- A self-registered app belongs to its first approver (`authserver.app_not_yours` for anyone
  else, checked before the redirect URI); global apps come from the CLI's `app:register`.
- The reclaim (`reset-password`, `user:change-password`) and `user:deactivate` revoke every
  grant; `update-password` keeps them, like PATs.

### Frontend

`web/src/`: `pages/` (routes), `features/`, `components/` (shadcn-style), `api/`, `hooks/`,
`app/` (providers, router, i18n), `lib/`, `test/`. The shadcn CLI is not a dependency: add
components with `pnpm dlx shadcn add`. Lint is oxlint, tests are vitest.

- **Runtime config** (`window.econumoConfig`, `/econumo-config.js`): the Go server generates
  the whole document (`internal/web/router`, written by `internal/web/spa`);
  `web/public/econumo-config.js` is only the fallback for `pnpm dev` without a backend.
  `VERSION` is the real running version and has no env override; `VERSION_LABEL`
  (`ECONUMO_VERSION`) is display text only; `VERSION_REF` is the source revision. Persisted
  caches are keyed by `getCacheBuster()` = `VERSION+VERSION_REF`. A change to the stored shape
  that could be read by a build sharing that key bumps `FORMAT_VERSION` in
  `transactionStore.ts`.
- **Transactions live in IndexedDB**, not the localStorage query cache: database `econumo`
  (`web/src/lib/db.ts`) with stores `pendingWrites`, `importKeys`, `accountTransactions`,
  `meta`. `transactionStore.ts` is the only reader/writer; errors never surface, and an epoch
  bumped on login/logout fences in-flight syncs. The `['transactions']` query is the
  revision-driven sync (`web/src/features/transactions/{sync.ts,revisionTracking.ts}`): it
  compares stored bucket revisions with `transactionsRevision` from `get-account-list` and
  downloads only stale accounts through `get-account-transaction-list`. Spec
  `2026-10-11-transactions-indexeddb-sync-design.md`.
- **Offline writes** (`web/src/features/pendingWrites`, spec
  `2026-11-08-offline-writes-design.md`): add/edit/delete a transaction and set a budget limit
  work offline. Operations queue in IndexedDB with one `Idempotency-Key` each, one sender
  (`sender.ts`, under a Web Lock), entry point `submit.ts`. Caches keep server rows only;
  waiting changes are overlaid at read time (`overlay.ts`). Refused changes show in the
  Inbox's "Not saved" section. Every other write is disabled offline through `useOnline`.
- **Service worker** (`web/sw/sw.js`, production only): precaches the shell so the PWA starts
  offline; never intercepts `/api`, `/mcp`, OAuth routes, `/.well-known`, `/health`; caches
  nothing user-specific.
- **App lock** (`web/src/features/appLock`, spec `2026-11-11-app-lock-design.md`): a per-device
  PIN or pattern, frontend only, never sent to the server, no biometrics. The gate is in
  `RequireAuth`, which renders `LockScreen` instead of its outlet.
- **Onboarding / Getting started** (`web/src/features/onboarding`, `internal/onboarding`, spec
  `2026-11-01-onboarding-day-0-design.md`): steps `import`, `categories`, `accounts`, `plan`,
  then client-side `home_screen` and `connect`. The client is thin: each of
  `save-categories`, `save-accounts`, `save-plan`, `finish-import` is ONE server write in one
  transaction (user row lock, then the operation-id claim), idempotent on its `id`. Progress
  is the `onboarding` user option; `completed` is terminal and written only by
  `complete-onboarding`. Onboarding never creates debts.
- **Analytics** (`web/src/lib/analytics.ts`, the only file naming the vendor): identified
  only (nothing is sent without a session), hashed user id, synthetic `$host` so a
  self-hosted hostname never appears in a payload. Gated by the per-user `analytics` option
  (`update-analytics`). The per-user `ai` option (`update-ai`) is the AI opt-out.
- **Other surfaces**: Reports (`/reports`, net worth), the sidebar "What's new" card
  (`features/whatsNew`, driven by `last_seen_version`), the feedback form
  (`features/feedback`, posted from the browser).

### i18n

Catalogues `locales/<lang>.json` (11: `de en es fr it nl pl pt ru uk zh`) are shared by both
stacks: Go embeds them (`locales/embed.go`), the SPA imports them. Keys are namespaced like
`web/src/features/<name>` plus `common`, `errors` (the error-code catalogue) and `emails`.
`{var}` placeholders in both stacks. The backend (`internal/infra/i18n`) renders emails and
coded error messages; language = `Accept-Language` → stored `users.language` → `en`. To add a
language use the `add-language` skill. Guards in `internal/test/i18ntest`: key and placeholder
parity, frontend `t()` coverage, two-way coverage with `errs.AllCodes`, mail keys.

## Testing

- `*_test.go` per package on sqlite through `internal/test/dbtest`. Always open test SQLite
  through `dbtest` (the production opener), never a raw `sql.Open`. `dbtest.New(t)` picks the
  engine by `DBTEST_ENGINE`; raw SQL in tests uses `db.Rebind`, and decimals compare as
  normalized `vo.Decimal`.
- `internal/test/apiparity/`: every registered route replayed against the real
  `server.BuildAPI` handler and diffed against goldens in `testdata/golden/`. Guards: every
  route has a scenario, counts never shrink, no orphan golden. If route files move, update
  `handlerGlobs` in `guard_test.go`.
- `internal/test/mcpparity/`: the same for `/mcp`.
- `internal/test/enginecompare/` (tag `enginecompare`): the catalogues on SQLite and
  PostgreSQL must answer byte-identically.
- `internal/test/revisionguard/`: fails when an account's transaction fingerprint changes
  without its revision moving, or a row is stored without sort keys.
- `internal/test/upgradetest/` (tag `upgrade`, linux): boots each minor line's released
  binary, seeds it, then upgrades to this checkout and requires a schema equal to a fresh
  install's and the same reads. Intended behavior changes are listed in `intended`.
- AI evals (tag `aieval`): `internal/{imports,onboarding}/aieval_test.go` drive the production
  prompt builders against a real model, in English and Russian; an area fails under
  `aieval.PassThreshold` (0.90). Fix the prompt or the case, never lower the threshold.
- Helpers: `internal/test/fixture`, `authstub` (the bearer token IS the user id),
  `internal/infra/oidc/oidctest` (in-process fake OpenID provider).

## Configuration

The server reads `.env`. `docs/full.env.example` documents every setting; `.env.example` is
the quick-start subset. Non-obvious points:

- `DATABASE_URL` (`sqlite:///abs/path` or `postgres://…`) and `PORT` are required and come
  from `.env`. Bare names are reserved for ecosystem standards (`PORT`, `DATABASE_URL`,
  `MAILER_DSN`, `SQLITE_BUSY_TIMEOUT`, `OPEN_EXCHANGE_RATES_TOKEN`).
- Config parsing is strict: a malformed value fails at boot. Boolean fields whose default is
  on are stored inverted (`PasswordLoginDisabled`, `SimpleFINDisabled`, `FeedbackDisabled`)
  so a zero `config.Config` in tests keeps them on.
- `ECONUMO_URL`: the public URL. Appended to transactional emails, required by every sign-in
  provider slot (callbacks derive from it), and the issuer of the MCP OAuth server.
- `ECONUMO_PASSWORD_LOGIN=false` refuses every password flow with
  `user.password_login_disabled` and requires at least one provider slot.
- `ECONUMO_TRIAL`: trial days for self-registered users (`0` = permanent full access); after
  it they are `readonly` (HTTP 402 on writes).
- `ECONUMO_ADMIN_PORT` + `ECONUMO_ADMIN_TOKEN`: a second, private listener for the payment
  portal (`/admin/set-access`, `/admin/user-context`, `/admin/affiliate-users`), never on the public mux. The token
  (random, at least 32 chars) is also the HMAC key for billing links (`ECONUMO_BILLING_URL`)
  and `econumo://` AI gateway tokens.
- `ECONUMO_AFFILIATES`: comma-separated `program:cookie` pairs; on new accounts the first
  program whose cookie holds a valid code is copied into the write-once, server-only user
  options `affiliate` (program) and `affiliate_code`, only when `ECONUMO_BILLING_URL` is also
  set; billing reads them on the admin listener (`/admin/affiliate-users?since=…`, optional `program=…`).
- Provider slots `ECONUMO_OAUTH_GOOGLE_*`, `ECONUMO_OAUTH_APPLE_*`, `ECONUMO_OIDC_*` are each
  all-or-nothing. Apple's key is a file path (`_PRIVATE_KEY_FILE`).
  `ECONUMO_OIDC_TRUST_EMAIL` treats the custom issuer's email as verified. See
  `docs/oidc-setup.md`.
- `MAILER_DSN`: empty = console transport (stdout); `resend://<key>`, `smtp://` (STARTTLS
  required), `smtps://`. Parse errors never echo the DSN.
- `ECONUMO_AI_DSN`: `openai://`, `anthropic://` or `econumo://` (gateway, signed with the
  admin token) `…?model=<model>[&json=1]`. Unset = AI off.
- `ECONUMO_IMPORT_SIMPLEFIN` (default on) switches the SimpleFIN provider; Apple Wallet import
  is always on. `ECONUMO_IMPORT_ALLOW_PRIVATE_HOSTS` lifts the SimpleFIN SSRF guard.
- Rate limits (`ECONUMO_RATE_LIMIT_*`) are in-memory. **No limit may be shared by unrelated
  callers**: email-keyed limits count per address AND client IP under a per-address ceiling;
  `ECONUMO_RATE_LIMIT_IP` is per endpoint per client IP. The client IP comes from
  `ECONUMO_CLIENT_IP_HEADER` / `ECONUMO_CLIENT_IP_TRUSTED_PROXIES` (`middleware.ClientIP`) and
  is never logged. Over-limit is the frozen 429 envelope.
- `ECONUMO_DATA_SALT` and `ECONUMO_ANALYTICS` are deprecated: each is read only by its
  one-time migration (`data:remove-salt`, `migration:seed-analytics-option`).
- `X-Timezone` header: the caller's IANA timezone for day-boundary math; persisted to
  `users.timezone`, which `/mcp` (only) falls back to.

### Logging

Structured `log/slog`, static messages, details as fields; UUIDs only, never PII (no emails,
bodies, query strings, client IPs). One operation line per request (message = operation name,
INFO/WARN/ERROR by status, with `request_id`, `status`, `route`, `user_id`, `timezone`) plus a
DEBUG transport line. Enrich the operation line with `reqctx.AddLogAttr(ctx, key, value)`.
`ECONUMO_LOG_LEVEL`, or `-v`/`-vv`/`-vvv`/`-q` on any command.

## Database

- SQLite via pure-Go `modernc.org/sqlite` (CGO off); PostgreSQL via `pgx/v5` stdlib, simple
  protocol.
- **Queries**: edit `internal/infra/storage/sqlc/query/{sqlite,pgsql}/*.sql`, then
  `sqlc generate` (config `internal/infra/storage/sqlc/sqlc.yaml`). Keep `.sql` files
  ASCII-only, comments included: a multibyte character silently truncates the generated SQL
  and fails at runtime as `incomplete input`.
- **Migrations** live in `internal/infra/storage/migrations/{sqlite,pgsql}` and run on boot;
  both engines keep the same version set. A new migration needs a version above every
  RELEASED one; on a `release/*` hotfix branch, above every migration on `main` (production
  runs `main`). A dropped SQLite column is repaired with a table rebuild.
- **Command steps** (`migrations.RegisterCommand(version, "migration:<slug>")`) run a CLI
  command between SQL files with no surrounding transaction, so they must be idempotent. They
  run TODAY's code against the schema at their version: number a step above every migration
  its code depends on, and when a later migration changes what it writes, move the step to a
  new version and leave a `SELECT 1;` no-op at the old one.
- **SQLite datetimes** are bare `Y-m-d H:i:s` UTC text; every connection is opened with
  `_time_format=datetime&_timezone=UTC` (`sqlite.WithFrozenTimeFormat`).
  `currencies_rates.published_at` is the one DATE column (`Y-m-d`, bound as text).
- **Transaction revisions**: `accounts.transactions_revision` moves by exactly +1 per
  committed DB transaction that changes anything the account's downloaded transaction list
  carries. The bump lives inside the repository method doing the write, through
  `internal/infra/txrevision` (`Bump`, or `BumpWhere` BEFORE rows change or go), flushed once
  before COMMIT in sorted id order.
- **Transaction order**: within a day, rows are ordered by fractional keys
  (`internal/shared/sortkey`), never time of day: `transactions.sort_key` on the source
  account, `recipient_sort_key` on a transfer's recipient. Order is day DESC, the listed
  account's key DESC, then `id`. Keys are assigned in the storage layer only
  (`internal/infra/txorder`); `move-transaction` writes one key and renumbers nothing else.
- **`idempotency_keys`**: one row per `(user_id, idempotency_key)`, owned by
  `internal/infra/idempotency`, purged after 90 days.
- **Currency deletion is always soft** (`currencies.is_deleted`): accounts and transactions
  cascade from the row.

## CLI / management commands

`cmd/econumo/main.go` dispatches `serve`, `healthcheck`, and `resource:action` commands in
`internal/cli` (in the image: `docker exec <container> /app/econumo user:create …`):

```
user:create|change-email|change-password|activate|deactivate|verify-email|set-access|show
currency:update-rates|show-rate-gaps|backfill-rates|add
token:purge [days]
data:remove-salt            # decrypt legacy salted emails; run with the old salt still set
data:import-sqlite [--force] <sqlite-path>   # copy a SQLite database into PostgreSQL
app:register|list|show|update|make-global|rotate-secret|disconnect|delete   # MCP OAuth clients
migration:*                 # idempotent boot steps, also runnable by hand
```

`user:change-password` is the account reclaim (see Authentication). `app:register` creates a
global app and prints its secret once. CLI-created users never get a trial.

## API conventions

- **Two methods**: `GET` for reads, `POST` for every write, deletes included. One POST read:
  `import/list-external-accounts` (the bridge access URL must never reach a query string).
- **Paths**: `/api/v1/{module}/{action}-{subject}`, kebab-case, verb first, lists end in
  `-list` (`get-account-list`, `create-category`, `set-limit`).
- **Auth**: `Authorization: Bearer <token>`. Public routes: login, register, remind-password,
  reset-password, confirm-email, resend-verification-code, `/api/doc`, `/api/doc.json`, and
  the six public `oauth` routes.
- **Read-only access**: a `readonly` caller gets HTTP 402 on any `POST` outside the
  middleware's allowlist (account security actions, `update-analytics`, `update-ai`,
  `update-ai-suggestions`, `update-last-seen-version`, declining or approving a `read` authorization). `GET` is never
  restricted.
- **`Idempotency-Key`** (`internal/web/middleware/idempotency.go`): the SPA sends a UUIDv7 on
  every `POST`. The server checks the table BEFORE the handler and records the key inside the
  request's own DB transaction. A replay answers HTTP 409 with `common.operation_locked` and
  an additive `resourceId`. A non-2xx releases the key unless the handler pinned it
  (`reqctx.PinIdempotency`: create/update/delete-transaction, `set-limit`, accept-suggestion).
  Absent key = no guard (MCP, other clients).
- **Errors** are rendered in the caller's language when they carry a catalogue code
  (`internal/shared/errs/codes.go`); code-less errors keep literal English.
- Responses are gzipped (`middleware.Compress`) except small or already-compressed bodies.
- The private admin surface returns a real 404 for an unknown user; the public API does not.

## Authentication

- Opaque bearer tokens, stored as `hex(sha256(token))` in `access_tokens`. Kinds: `session`
  (sliding 30 days, touched at most every 5 minutes), `personal` (PAT, fixed optional expiry,
  shown once), `oauth` (MCP OAuth server). Scopes: `full`, `ingest` (PAT that may only call
  `/api/v1/import/ingest-*`), `mcp`, `read`. There is no token refresh for sessions.
- The `user` feature owns authentication; the middleware seam is
  `middleware.TokenAuthenticator`.
- **Revocation**: `update-password` rotates the credential and revokes the OTHER sessions,
  keeping PATs and linked identities. `user:deactivate` revokes sessions and PATs.
  `reset-password` and CLI `user:change-password` are the account RECLAIM
  (`user.reclaimCredentials`): they revoke every session and PAT, drop pending grants, and
  unlink every external identity whose provider vouches for a different address. Data sharing
  (connections, budget grants) is never touched.
- **Credentials fence** (`users.credentials_generation`): any flow that reads evidence and
  writes later captures the generation and presents it at write time; session and identity
  inserts are conditional on it INSIDE the SQL (`…IfGeneration`). Checking in Go would be the
  race. It is paired with the user row lock (`Repository.LockRow`): every write to an existing
  user row and every credential mint takes it before reading. Reset, verification and
  email-change codes are issued and consumed under the same lock.
- **Provider sign-in** (`internal/oauth`): state-carrying redirect, verified ID token, then a
  one-time 60-second handoff code exchanged for a session, bound to a flow secret held by the
  client that started it. Auto-link by verified email happens only into a PASSWORDLESS
  account; a match on an account with a password is refused (`account_exists_password`). An
  unverified email is refused for every intent. A link started from Settings writes nothing
  on the callback: the authenticated `complete-link` does. Identities are keyed by
  `(provider, issuer, subject)`.
- Every change to the set of sign-in methods emails the owner a notice, CC'd to the addresses
  the linked providers reported. Code emails are never CC'd.

## Wire & data contract (frozen)

Clients parse exact JSON and the production database holds data in these formats. Do not
clean them up.

- **Envelope** (`internal/web/httpx/envelope.go`):
  - 200: `{"success": true, "message": "", "data": <payload>}`
  - handled error (default 400): `{"success": false, "message", "code": <int>, "errors": {field: [messages]}}`,
    `errors` always present.
  - 500: `{"success": false, "message", "code": 0, "exceptionType"}`, no `errors`; detail goes
    to logs only.
  - 501: `errors` is the array `[]`. 429: message `"Too many attempts. Try again later."`.
  - JSON is encoded with HTML escaping disabled.
- **401 messages** are exact: `"Access token not found"` (missing/malformed header) and
  `"Invalid access token"` (anything else).
- **Tokens**: `eco_ses_` / `eco_pat_` / `eco_oat_` + `base64.RawURLEncoding` of 32 random bytes.
- **Passwords** are versioned by `users.algorithm`: `sha512` (legacy: 500 iterations, base64,
  salt merged as `password{salt}`), `argon2id` (every new hash, PHC string), `none`
  (passwordless). Unknown values fail closed.
- **Emails**: `users.email` holds `model.NormalizeEmail(email)`, computed in Go, unique. The
  legacy `identifier` column holds the row's own id. Emails are stored as plaintext.
- **Encodings**: datetimes `"2006-01-02 15:04:05"` (no zone); `isArchived` and similar flags
  are int `0`/`1`; category `type` is `"expense"`/`"income"`; ids are TEXT, new ones UUIDv7,
  existing ones never rewritten; `avatar` is `"<icon>:<color>"`.
- Route paths and methods are contract. Login takes `username` + `password` and returns
  `{"token","user"}`; register returns the user without a token.
- Validation strings are exact and asserted in `en`.

## Domain behaviours

### Budgets

- **Membership** is explicit (`budgets_accounts`); soft-deleted members keep counting. An
  account leaves only while it has no transactions between the budget start and the caller's
  current month (`budget.account_not_removable`). Deleting an account writes a correction so
  its balance is zero, and that invariant self-heals on later edits.
- **Lifecycle**: optional end month (`ended_at`) and an archived flag; archived = hidden and
  read-only (403 `budget.archived`). `clone-budget` is an owner-or-admin deep copy.
- **Period start day** (1-28): a period keeps its key `YYYY-MM-01`; only the date window
  shifts. One helper per stack: `model.PeriodWindow`/`model.PeriodOf`,
  `web/src/features/budgets/period.ts`. Day-1 budgets stay byte-identical.
- **Savings** is a per-budget membership flag (`budgets_accounts.is_savings`), not an account
  type; only the account's owner sets it. Savings rows are `structure.savings`, never in
  `structure.elements`. "Saved" is the net of everyday↔savings transfers only.
- **Plans**: a plan of 0 is stored and is a plan; `set-limit` with `amount: null` removes it.
  `get-budget` distinguishes them with `isBudgeted`.
- **Visibility**: an element shows when it has spending or a limit.
- **Upcoming money** (projected recurring templates and real rows dated after the caller's
  today) is a HINT on the plan number (`upcoming` fields), never part of spent, Available or
  any total. Figures are money dated up to today (`internal/budget/dated_read.go`).
- **Account balance** "as of end of today" uses the caller's timezone.
- **Rates**: conversions use each month's average rate; a month without one borrows the
  latest earlier rate, never face value. The import converter alone reads unfilled rates.
- **Category type matches transaction type** (`transaction.category_type_mismatch`), except
  on an update keeping the stored category and type.
- **Recurring** templates advance with month-end clamping from a persisted scheduled day;
  posting is manual and idempotent on the client transaction id.

### Debts (`internal/debt`, `web/src/features/debts`)

Specs: `2026-11-13-debts-on-elements-design.md` (the current model),
`2026-11-13-debt-rate-changes-design.md`, `2026-11-14-debts-without-fixed-payments-design.md`.

- A debt is a loan tracked on ONE expense element of ONE budget: a top-level expense category
  or an envelope. It is not an account and not a budget row; the element keeps its own plans,
  spending and totals. Name, icon and currency are the element's. One active debt per element
  plus paid-off ones whose lives never overlap.
- Access is the budget's: no accepted access = not found, readers read, owner/admin/user
  write, archived budget = 403.
- **Payments are exactly the money the element's Spent counts** (untagged expenses in its
  categories on member accounts, within the debt's life). No rules or filters. Splits
  (`debts_payments`) are materialized by the debt sync on read; stored splits are never
  recomputed.
- **Terms** are the contract's facts. The setup trio (amount borrowed, start date, last
  payment date) makes the origin snapshot; without it the debt is not set up (`isSetUp` 0, no
  figures). Statements (`reconcile-debt`) append snapshots and keep the loan's implied rate.
  A correction rewrites the origin in place and drops the stored splits.
- **Rate changes** (`debts_rate_changes`) set a yearly rate and payment from a date.
  Schedule `none` ("Return by a date") has payment 0, no dues, and refuses rate changes.
- **Paid off is a status**, set only by `mark-debt-paid-off` and undone by `reopen-debt`,
  never by the balance reaching 0. `isPaidOff` is a display flag from the balance.
- A planned extra is the element's plan above the scheduled payment. Elements carry an
  additive `debt` object on `get-budget` / `get-budget-plan`.
- **Lock order**: the debt's budget row (`LockBudget`) before any element row, budgets in id
  order, everywhere. Every debt write runs its sync, write and presenting read in ONE
  transaction (`inOneTx`). `Views`/`Timelines` are read-only so budget builders never invert
  the lock.
- A category with an active debt cannot join an envelope (`budget.element_has_debt`); use
  `move-debt`. Category deletes and merges are never refused: they go through category's
  `DebtUsage` port (`Prepare`/`Release`/`Reassign`).
- SPA: reached from the element's row menu; the only way in is `DebtReviewScreen` (tabs
  Overview, Balance, Settings, Danger zone).

### Transaction import (`internal/imports`)

Spec `2026-08-15-transaction-import-design.md`. 33 routes under `/api/v1/import/`; no MCP
surface.

- One `import_sources` row per user per provider. Apple Wallet is a push provider (scope
  `ingest` PATs): a card starts `unmapped` and its events queue until mapped to an OWNED
  account or ignored. Ingest never fails the request on a bad transaction.
- SimpleFIN is a pull provider: the browser encrypts the access URL with a
  passphrase-wrapped key; the server stores only ciphertext. The plaintext URL travels in
  request bodies and is never persisted, logged or put in an error.
- The matcher (a pure function of event, candidates and `ECONUMO_IMPORT_*` thresholds)
  adopts an existing transaction before creating one. Deleting an imported transaction leaves
  a tombstone so it never comes back; `reimport-link` undoes that.
- **Rules** belong to the account OWNER, not the importer. `classify` rules only fill empty
  fields; `skip` rules win. A rule's category reaches only transactions of its own kind.
- **AI rule suggestions** (spec `2026-11-11-ai-import-rule-suggestions-design.md`): after a
  live import leaves rows unclassified, a background job proposes a rule, landing in the
  owner's Inbox. One open suggestion per (owner, merchant, type); one owner's jobs run one at
  a time. The same AI pass also ASSIGNS the category and payee to those transactions that
  still have NO category (a rule- or hand-set category means nothing is assigned, even an
  empty payee; empty fields only, under a transaction row lock, like a rule's write; the link
  then reads `assigned_by = 'ai'`, which keeps an agreeing rule suggestion listed). The link's
  `assigned_by` (`rule`/`ai`/NULL) is rewritten by every write of its applied snapshot; a path
  that leaves the transaction's classification alone keeps it. A missing payee is created only for a merchant the AI flags `wellKnown`; a rule
  suggestion may carry such a name (`payee_name`), created when the user saves the rule. Spec
  `2026-11-22-ai-import-suggestion-settings-design.md`. User options `ai_suggest_rules` and
  `ai_suggest_fill` (assign; `update-ai-suggestions`, absent = on); both off = no AI call.

### AI integration (`internal/infra/ai`)

Consumers: onboarding (savings detection, icons for imported or newly named categories and
accounts) and imports (rule suggestions, automatic category/payee assignment, `suggest-match-text`). System prompts are Markdown
files in the top-level `prompts/`; reply schemas live in `internal/{imports,onboarding}/prompts.go`.

- **Data allowlist, every prompt**: category names and types; payee, tag, label and account
  names; currency codes; language; timezone; a transaction's type and the bank's payee text;
  opaque per-request keys (`a1`, `c1`, `p1`). NEVER amounts, balances, plans, dates,
  descriptions/memos, entity ids, user names or emails.
- A user with the `ai` option off sends nothing; the option is re-read right before each
  provider call.
- AI never fails a request: a failure, timeout or unusable reply changes nothing and logs a
  WARN with ids and `err_type` only. Each job has its own per-user rate-limit scope.
- Icons are kept only from the shared catalogue `icons/icons.json` and applied with a
  conditional update, so an icon the user picked is never repainted.

## Deployment

- Image `ghcr.io/econumo/econumo`: `:latest` + `:vX.Y.Z`, releases only. `dev` and `edge`
  (main's head, production) exist only as R2 channels. Dockerfile:
  `deployment/docker/Dockerfile`.
- Self-hosting: root `docker-compose.yml` + `.env.example`; Docker-free:
  `docs/run-without-docker.md` (`make release-binaries` builds the same artifacts).
