# Repository Guidelines

This file provides guidance to AI agents when working with code in this repository.

## Project Overview

Econumo is a self-hosted personal finance and budgeting application. It consists of:
- **Backend**: Go (HTTP API + static SPA server) with hexagonal architecture (the Go module is the repo root).
- **Frontend**: React 19 + Vite SPA with TypeScript (Tailwind 4 / shadcn, TanStack Query, Zustand), in `web/`.
- **Database**: SQLite (default) or PostgreSQL — selected at runtime by `DATABASE_URL`.

> History: the backend was originally a Symfony 5.4 (PHP) app. It has been fully
> replaced by the Go backend and the PHP code has been removed. A single "cloud"
> edition is shipped (there is no separate "ce" edition).

The production artifact is a single self-contained Go binary in a distroless image
(`ghcr.io/econumo/econumo`) that serves both the JSON API and the built SPA (embedded
via `go:embed` — `web/embed.go`) and runs database migrations on boot. Releases
additionally attach standalone `econumo-linux-{amd64,arm64}` binaries + `SHA256SUMS`
for Docker-free hosting.

## Development Commands

### Go backend

The Go module is the repo root. Tests run with the standard toolchain — no Docker
required for the smoke tier.

```bash
make go-test         # SMOKE: build + vet + gofmt + OpenAPI-docs-fresh + sqlite unit/integration + coverage gate
make test            # FULL: go-test + the sqlite-vs-PostgreSQL engine-comparison suite + the frontend suite
make go-build        # Compile the binary to ./econumo (regenerates OpenAPI docs first)
make go-run          # Run the server locally (reads .env; regenerates OpenAPI docs first)
make go-lint         # build + vet + gofmt + OpenAPI-docs-fresh check
make swagger         # Regenerate the committed OpenAPI docs (swag, pinned to go.mod)

# Or directly with the go toolchain (run from the repo root):
go test ./...                        # all tests
go build ./...                        # build everything
go run ./cmd/econumo serve            # run the server (reads .env)
go run ./cmd/econumo user:create "Name" user@example.test secret
```

The full suite needs a PostgreSQL; `make test` auto-provisions one via the
compose stack, or set `DATABASE_TEST_PGSQL_URL` to an existing database.

### Frontend (React/Vite) — in `web/`

```bash
make web-install   # cd web && pnpm install
make web-run       # cd web && pnpm dev       (dev server, port 9000, proxies /api to :8181)
make web-test      # cd web && pnpm test      (vitest)
make web-lint      # cd web && pnpm lint      (oxlint)
make web-bundle    # cd web && pnpm build     (production SPA build -> web/dist)
```

### Publishing

```bash
make publish-dev   # build + push the multi-arch `dev` image to ghcr.io/econumo/econumo:dev
```

Releases (`latest` + `vX.Y.Z`) are cut by the GitHub release workflow
(`.github/workflows/publish-release.yml`), not locally; its "Publish Dev"
checkbox can also move the `dev` tag. The workflow's `branch` input selects
the source branch (default `main`; a `release/*` branch for hotfixes of older
versions). A release from `main` auto-creates `release/vX.Y.Z` at the released
commit as the base for future hotfixes; a hotfix dispatches from a new
`release/*` branch copied by hand from the fixed version's branch, and the
workflow then creates no branch. `latest` only moves for the highest version
released so far. See `.claude/skills/publish-release` for the full process.
Everything publishes to `ghcr.io/econumo/econumo` only.

### Branch naming

- `feature/<slug>` — feature work, and the default for EVERY change that is
  not a bug fix (chores, docs, refactors, CI, dependency bumps).
- `bug/<slug>` — bug fixes.
- `release/vX.Y.Z` — reserved for releases (auto-created when releasing from
  `main`; copied by hand from the previous release branch of the line for
  hotfixes); never used for development work.

## Architecture

### Feature packages (vertical slices)

The backend is organized as vertical feature packages rather than horizontal
layers. Each of the eleven features (`account`, `admin`, `budget`, `category`, `connection`,
`currency`, `payee`, `system`, `tag`, `transaction`, `user`) is a single `internal/<feature>`
tree holding its own use cases, persistence, and HTTP edge; the entities and
DTOs those use cases operate on live in the shared `internal/model` package
(below), so a feature package is behavior-only:

```
. (repo root = the Go module; web/ and deployment/ live alongside)
├── cmd/econumo/main.go ............ binary entrypoint; dispatches serve / healthcheck / resource:action commands
├── internal/
│   ├── shared/ .................... dependency-free kernel: vo (value objects), errs (domain error taxonomy),
│   │                                datetime (frozen wire/persistence layouts),
│   │                                port (Clock/TxRunner/OperationGuard seams), reqctx (request-scoped
│   │                                context values, e.g. caller timezone + log attrs)
│   ├── model/ ...................... the shared type universe: every feature's entities (with their
│   │                                invariant-preserving mutators), value objects, and request/result DTOs,
│   │                                one file per feature (account.go, account_dto.go, ...); imports only
│   │                                the shared kernel; part of the archtest kernel alongside `shared`
│   ├── <feature>/ ................. one package per feature (account, budget, category, connection,
│   │   │                            currency, payee, system, tag, transaction, user); root package holds only
│   │   │                            behavior — the entities/DTOs it operates on live in `internal/model`:
│   │   │   <verb>.go .............   one file per use case or a closely related group (create.go,
│   │   │                             update.go, delete.go, read.go, ...), naming a package-level `Service`
│   │   │                             (or `ReadService`/`WriteService`) method per use case
│   │   │   repository.go .........   the repository INTERFACE(s), in `model` types; account and budget
│   │   │                             split theirs into role interfaces + a composite for wiring
│   │   │   ports.go ..............   consumer-side interfaces for capabilities OTHER features provide
│   │   ├── repo/ ..................  repository implementation (engine-adapter pattern, see below)
│   │   ├── api/ ...................  HTTP edge: handlers + route registration (see API handler pattern below)
│   │   └── mcp/ ...................  MCP edge (all nine features have one): tool registration, mirroring
│   │                                 api/ (see MCP endpoint below; prompts live in internal/web/mcp)
│   ├── infra/ .................... engine-agnostic infrastructure shared by every feature:
│   │   ├── storage/sqlc/ ......... sqlc config + per-engine queries (query/{sqlite,pgsql}) and generated code (gen/{sqlite,pgsql})
│   │   ├── storage/migrations/ ... SQL migrations per engine ({sqlite,pgsql}); run on boot
│   │   ├── operation/ ............ shared row-based idempotency guard (operation_requests_ids) for
│   │   │                          client-supplied operation ids on create endpoints
│   │   ├── auth/ ................ password hashing + AES email encryption
│   │   ├── clock/ ................ time source abstraction
│   │   └── mailer/ .............. transactional email; transport from MAILER_DSN (console stdout | Resend API)
│   ├── web/ ..................... HTTP-edge infrastructure shared by every feature (the Go server edge —
│   │                              distinct from the repo-root web/, the React SPA): middleware, router,
│   │                              response envelope (httpx), SPA server, apidoc — no feature logic
│   ├── server/ .................. composition root: server.BuildAPI wires every feature (used by the
│   │                              binary AND tests); glue_*.go files hold the cross-feature adapters
│   │                              (see below)
│   ├── cli/ ..................... the resource:action management commands (stdlib dispatch; no cobra)
│   ├── config/ .................. environment configuration loading
│   └── test/ .................... shared test support: dbtest, fixture, testkeys, enginecompare, archtest
```

Not every feature has a `repository.go`/`ports.go` — e.g. `currency` has no
per-user persistence shape (it's rates + conversion + admin lookups), so it
keeps `read.go`/`admin.go`/`convertor.go` but no `repository.go`; `system` is
similar — it's in-memory poller state only (no persistence at all), so it has
no `repository.go` either.

### Dependency rule

Features never import features. When one feature needs another's data (e.g.
account needs a currency lookup, or budget needs to resolve a user), the
*consuming* feature declares a small interface in its own package (typed in
`model`), and `internal/server` wires a concrete adapter — usually a thin one
implemented in a `glue_<feature>_<purpose>.go` file (e.g.
`glue_account_currencylookup.go`, `glue_budget_userlookup.go`) — over the
*providing* feature's public API at composition time. This keeps every
feature package independently buildable and testable without pulling in its
siblings.

Shared leaves (`shared`, `model`, `web`, `infra`) never import a feature; the
kernel (`internal/shared` + `internal/model`) imports nothing internal outside
itself — `model` may only depend on `shared`. This is enforced by
`internal/test/archtest`, which auto-detects feature packages (any
`internal/<top>` not in its infrastructure set) so newly moved features come
under enforcement without edits to the test.

### The engine-adapter (sqlc) pattern

sqlc generates a SEPARATE package per engine (`gen/sqlite`, `gen/pgsql`) with
distinct Go types, and the two dialects differ (`?` vs `$N` placeholders, float vs
NUMERIC aggregates, date formats). A repo therefore:

- declares a small `querier` interface in the **canonical (sqlite-generated) types**,
- has two thin adapters — `sqlite.go` (native passthrough) and `pgsql.go` (whole-struct
  conversion shim) — selected ONCE in the constructor by `cfg.DatabaseDriver`,
- writes every method ONCE against the interface, so method bodies carry no
  per-driver branching.

Reference repos: `internal/{tag,user,currency}/repo` (the `user` repo also
covers password-reset requests, in `passwordrequest*.go` alongside the user
queries). The one exception is `internal/budget/repo/read.go`, which is
hand-built dynamic SQL (variadic `IN` lists, real per-engine value/date
handling) and branches explicitly by design.

### API handler pattern

HTTP handlers are thin named methods under `internal/<feature>/api/` (the
name carries the swag `@` annotation block); the body is one call into the
generic combinators in `internal/web/endpoint`: `endpoint.Handle` (auth + JSON
body), `endpoint.HandleNoBody` (auth, no body), `endpoint.HandlePublic`
(no auth) — each doing require-user/decode/`Validate()`/call/envelope in the
frozen order. Per-endpoint extras (e.g. `reqctx.AddLogAttr`) live in a small
closure. A handful of endpoints stay hand-written because their shape is
special: CSV export, multipart import, query-param reads, and login's raw
`{token,user}` response. Routes are registered in
`internal/<feature>/api/routes.go` (`RegisterAPI`). Request/result DTOs live
in `internal/model/<feature>_dto.go`.

### MCP endpoint

`/mcp` is a second, independent HTTP edge — mounted on the root mux next to
`/health`, outside `/api`, so the REST `apiparity` scanner never sees it (its
own golden suite, `internal/test/mcpparity/`, owns coverage instead). It's the
official Go SDK's Streamable HTTP handler in **stateless, JSON-response**
mode (no SSE, no server-held sessions — every tool call is a sub-second DB
round trip). Auth is the same bearer-token middleware as REST (PATs are the
intended credential for MCP clients). Shared edge infra lives in
`internal/web/mcp/`; each feature that exposes MCP surface registers its own
tools/prompts from an `internal/<feature>/mcp/` package, composed at
`server.BuildAPI` exactly like `RegisterAPI`.

### Frontend architecture (React 19 + Vite)

Directory structure in `web/src/`: `pages/` (routes), `features/`, `components/`
(shadcn-style UI), `api/` (typed API clients), `hooks/`, `app/` (providers,
router, i18n setup), `lib/`, `locales/`, `test/`. Runtime config is read from
`public/econumo-config.js` (`window.econumoConfig`); the UI version label is
`ECONUMO_VERSION`, inlined by Vite at build time (the Docker build arg of the
same name sets it per image build, default `dev`). Lint is oxlint, tests are
vitest (`pnpm test`).

**Product analytics rule:** every new user-facing feature/action MUST fire an
analytics event — add a key to `METRICS` (`web/src/lib/metrics.ts`, frozen
`app`-prefixed camelCase names; the PostHog snake_case name derives
automatically) and call `trackEvent` at the action's success point (mutation
`onSuccess`; or inside `mutationFn` after the API call for hooks with a dedupe
short-circuit, e.g. the classification creates). Prefer the shared hook/store
choke point over per-page call sites so every surface (pages, dialogs, inline
creates) is covered once. `web/src/lib/metrics-coverage.test.ts` fails the
suite if a `METRICS` key is never fired; a catalogue key may only be excused
via its documented `NOT_WIRED` list.

### i18n (`locales/`, `internal/infra/i18n`, `web/src/app/i18n`)

Translations live in two catalogues, `locales/{en,ru}.json`, shared by both
stacks — no per-stack duplication. The Go side embeds them (`package locales`,
`go:embed *.json` in `locales/embed.go`, read via `locales.FS`); the SPA
imports the same files through Vite JSON imports for `react-i18next`. Keys are
namespaced to mirror `web/src/features/<name>` plus three cross-cutting
namespaces: `common`, `errors` (error-code catalogue, see the envelope section
above), and `emails` (backend-rendered mail). `{var}` placeholders use the same
`{name}` syntax in both stacks. Adding a language means a new
`locales/<lang>.json` plus entries in `i18n.Supported`
(`internal/infra/i18n/i18n.go`), registering the catalogue in `resources`
(`web/src/app/i18n.ts`), `getLocaleOptions()` (`web/src/lib/config.ts`),
and the `languages` list in `internal/test/i18ntest`.
- **Backend runtime**: `internal/infra/i18n` (`i18n.T(lang, key, params)`) translates
  server-rendered text — the password-reset email, and the error `message`/`errors`
  strings on both edges (REST envelope and MCP tool errors) for errors that carry a
  catalogue code. The `Language` middleware resolves `Accept-Language` to a supported
  two-letter tag and stashes it in `reqctx`; the middleware chain is
  `requestid -> accesslog -> recover -> cors -> timezone -> language -> [auth]`
  (`internal/web/middleware/middleware.go`).
- **Frontend runtime**: `web/src/app/i18n` wires `react-i18next`; UI language
  choice persists in `localStorage` (`locale()` in `web/src/lib/config.ts`) and
  is applied via `LanguageSelector` (`web/src/components/LanguageSelector.tsx`,
  surfaced on auth pages and in Settings). `apiErrorMessage`
  (`web/src/lib/apiError.ts`) renders a failed response from the envelope's
  server-translated strings: the first field error when present (the top-level
  message is then the generic form label), else `message`; `apiFieldErrors`
  serves inline per-field form errors.
  `pluralPick` (`web/src/lib/plural.ts`) reads pipe-delimited catalogue values
  (`"one | many"`, Russian `"one | few | many"`) and picks the variant via
  `Intl.PluralRules` — i18next's own plural suffixes are not used, so all
  plural strings are authored as a single pipe-joined value. The selected
  language is also persisted server-side (`users.language`, default `en` —
  written by `update-language` and on login from `Accept-Language`; read back
  as the stored-language fallback for header-less authenticated requests on
  both edges, and reserved for future background email rendering).
- **Guards** (`internal/test/i18ntest`, run inside `make go-test`): catalogue
  key parity between `en`/`ru`, `{var}` placeholder-set parity per key,
  frontend-source `t()`-call key coverage against the catalogue, two-way
  coverage between `errs.AllCodes` and the `errors.*` catalogue keys (every
  registered code has a translation and vice versa), and that every mailer
  email key has an `emails.*` entry in every language.

## Testing

Tests live alongside the Go code:
- `*_test.go` unit/integration tests per package (sqlite via `internal/test/dbtest`;
  dbtest applies production pragmas, e.g. `foreign_keys = ON`).
- `internal/test/apiparity/` — the shared API scenario catalogue: every registered
  route is replayed against the REAL production handler (`server.BuildAPI`).
  Two consumers: the untagged **smoke suite** (every `make go-test`) diffs each
  scenario's responses against committed golden files in `testdata/golden/`
  (normalized: generated UUIDs, datetimes, bearer tokens redacted), and the build-tagged
  parity suite below. Guard tests enforce that every route has a scenario, the
  scenario count and scanned-route count never shrink, and no golden is orphaned.
  Regenerate goldens with `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`,
  then INSPECT the diff — a golden change means observable behavior changed;
  never hand-edit a golden. If route-registration files move, update
  `handlerGlobs` in `guard_test.go`.
- `dbtest.New(t)` selects the engine by `DBTEST_ENGINE` (default sqlite; `pgsql`
  under `-tags enginecompare` → Postgres, each test in its own schema). `make
  test-repo-pgsql` reruns the whole repo/unit suite against PostgreSQL so the
  pgsql adapters + generated queries are exercised, not just the parity
  catalogue; it is part of `make test` and the CI regression job. Raw SQL
  in a test must use `db.Rebind(query)` for `?`→`$N`, and decimal assertions
  compare normalized `vo.Decimal` values (sqlite text vs pgsql NUMERIC differ).
- `internal/test/enginecompare/` — the strongest contract: replays the same
  catalogue on BOTH SQLite and PostgreSQL and asserts byte-identical
  responses (build tag `enginecompare`).
- `internal/test/fixture` — shared fixture builder (users, accounts, access tokens, ...);
  `internal/test/authstub` — a stub `middleware.TokenAuthenticator` for feature api tests
  (the bearer token IS the user id string).
- `internal/test/mcpparity/` — the MCP counterpart to `apiparity`: a golden-file JSON-RPC
  scenario catalogue (`initialize`, `tools/list`, `prompts/list`, each tool/prompt)
  replayed against the real `server.BuildAPI` handler over `/mcp`,
  normalized the same way as the REST goldens. Runs in the smoke tier and, under
  `-tags enginecompare`, against both engines. Regenerate goldens with
  `UPDATE_GOLDEN=1 go test ./internal/test/mcpparity/`, then INSPECT the diff — same rule
  as `apiparity`.

Coverage gate: `make go-test` enforces a cross-package minimum (`GO_COVER_MIN`,
default 78). CI surfaces the coverage % in the Actions job summary plus an HTML
artifact (`.github/workflows/go-tests.yml`).

## Configuration

The Go server reads its environment from `.env` (see `.env.example`). Key vars:

- `DATABASE_URL` — `sqlite:///abs/path/db.sqlite` or `postgres://…`. Selects the engine.
  Required, and sourced from `.env` only — it is NOT baked into the image.
- `PORT` — HTTP listen port (required by the server), from `.env`. The image keeps a
  fallback `PORT=80` (compose maps `8181:80`); for a host `go run`, set a non-privileged
  port (e.g. `8181`) in `.env`.
- Auth tokens are stored in the database (`access_tokens`) — there are no signing keys and
  no token-related configuration. Persist the `/app/var` volume (the db) to keep data and
  logins valid. Leftover `ECONUMO_JWT_*` variables from older builds are ignored.
- `ECONUMO_DATA_SALT` — **Deprecated and IGNORED by the API/repositories**, which always run
  salt-free (plaintext emails, `lower(email)` as the lookup key). It is consumed by exactly one
  code path, the `data:remove-salt` migration (below), which reads it to decrypt existing data.
  Set it to your old salt, run that command, then unset it. Until you migrate, a still-salted
  database has unreadable emails, so those users cannot log in (the intended push to migrate);
  `serve` logs a WARN at boot while it is set.
- `ECONUMO_ALLOW_REGISTRATION` — enable/disable the register endpoint.
- `ECONUMO_TRIAL` — access granted to a newly registered user: `none` (default, no
  access grant) or `end-of-next-month` (full access until the first of the month
  after next). Malformed values fail at boot. See `user:set-access` / `user:show`
  below and the 402 rule in API conventions for how access is enforced afterward.
- `ECONUMO_EMAIL_VERIFICATION` — require newly registered users to confirm an emailed
  code at login before their first session (default `false`; strict boolean, malformed
  fails at boot). The code email is sent at the first blocked login attempt, not at
  registration; `serve` WARNs at boot when enabled with the console mail transport.
  Existing rows and CLI/admin-created users are always verified.
- `ECONUMO_ADMIN_PORT` / `ECONUMO_ADMIN_TOKEN` — the private admin listener the payment
  portal talks to (`POST /admin/set-access`, `GET /admin/user-context`). A **second**
  `http.Server`, started by `serve` only when BOTH are set, so a self-hosted instance
  never serves those routes and they sit on no public mux at all (enforced by
  `TestAdminRoutesAreNotOnThePublicMux`). Auth is `Authorization: Bearer <ECONUMO_ADMIN_TOKEN>`
  compared in constant time; the same token is the HMAC key for billing-handoff tokens
  (minimum 32 characters, and it must be RANDOM — authenticated users see HMAC samples in
  their billing links, so a guessable passphrase is offline-attackable; generate with
  `openssl rand -hex 32`). A half-configured pair fails at boot. Every admin action is
  audit-logged on its operation line: `set-access` carries `user_id`, the written
  `access_level`/`access_until` AND the `old_*` pair (logged even when the write fails, so
  attempts are recorded); `user-context` carries `user_id` + `connections`; CLI
  `user:set-access` emits the same `set-access` line. Ids and levels only — never emails. The port accepts a
  host-qualified value (`127.0.0.1:9090`) to pin the listener to loopback/an internal
  interface on bare-host deployments; a bare port binds all interfaces (the container
  default, where compose controls exposure). Unlike the public API,
  this surface returns a real 404 for an unknown user: its consumer is a machine.
- `ECONUMO_BILLING_URL` — payment portal URL (**https required** except loopback hosts —
  billing links carry signed identity tokens in the query string). Empty (default) means
  `POST /api/v1/user/create-billing-link` returns 400 and the SPA shows no billing UI.
  Merged into the served `econumo-config.js` as `BILLING_URL`, so one variable drives
  both halves. Requires `ECONUMO_ADMIN_TOKEN` (the signing key).
- `ECONUMO_CORS_ALLOW_ORIGIN` — comma-separated cross-origin allowlist. Empty (default) = same-domain
  only (no `Access-Control-Allow-Origin` emitted; the bundled SPA and API share an origin so it
  just works). A configured origin is reflected back with `Vary: Origin`; `*` allows any origin.
- `ECONUMO_CURRENCY_BASE` — base currency (default `USD`).
- `ECONUMO_CHECK_UPDATES` — daily check for new releases against `econumo.com/releases/latest.json` (single server-side request; result served to the SPA via `get-update-info`). `false` disables it.
- `ECONUMO_ANALYTICS` — anonymous product analytics from the SPA to PostHog (default `true`).
  `false` disables it instance-wide. Malformed values fail at boot (strict parse, unlike
  the other booleans). Server-owned SPA config keys reach the frontend via an
  `Object.assign(window.econumoConfig, …)` line the SPA handler appends to the served
  `/econumo-config.js`; the embedded dist file's static values are the fallback when a
  key is not overridden. `ANALYTICS` and `ALLOW_REGISTRATION` are always merged
  (server truth); `ECONUMO_ALLOW_CUSTOM_API` merges `ALLOW_CUSTOM_API` only when set
  (unset = keep the dist value).
- `MAILER_DSN` — mail transport for password-reset email; the scheme selects the provider, exactly
  as `DATABASE_URL`'s scheme selects the DB engine. Empty (default) = the **console** transport (renders
  each email to stdout — a dev aid that never silently drops mail); `resend://<api_key>` sends via Resend.
  From / Reply-To fold in as query params: `resend://<key>?from=…&reply_to=…` (also accepted by
  `console://`/`log://`). Parsed once in `config.Load` (a bad scheme fails at boot). Replaces the former
  `RESEND_API_KEY` / `ECONUMO_MAIL_FROM` / `ECONUMO_MAIL_REPLY_TO`.
- `OPEN_EXCHANGE_RATES_TOKEN` — currency-rate updates.
- `SQLITE_BUSY_TIMEOUT` — SQLite `busy_timeout` PRAGMA in ms (default `0`); bare name mirrors the engine pragma.
- `ECONUMO_RATE_LIMIT_LOGIN` / `ECONUMO_RATE_LIMIT_RESET` / `ECONUMO_RATE_LIMIT_REMIND` /
  `ECONUMO_RATE_LIMIT_REGISTER` — brute-force protection for the public auth endpoints:
  max attempts per username/email per window (defaults 5/5/3/5; login and reset count
  only FAILED attempts and clear on success, remind and register count every request).
  `ECONUMO_RATE_LIMIT_ACCEPT_INVITE` — cap on `connection/accept-invite` attempts per
  authenticated user per window (default `10`; every attempt counts), guarding the short
  invite code against online brute force.
  `ECONUMO_RATE_LIMIT_VERIFY_EMAIL` — verification-code emails per username per window (default `3`; every send counts).
  `ECONUMO_RATE_LIMIT_CONFIRM_EMAIL` — failed confirm-email attempts per username per window (default `5`; cleared on success).
  `ECONUMO_RATE_LIMIT_REQUEST_EMAIL_CHANGE` — change-email code sends per user per window (default `3`; every send counts).
  `ECONUMO_RATE_LIMIT_CONFIRM_EMAIL_CHANGE` — failed confirm-email-change attempts per user per window (default `5`; cleared on success).
  `ECONUMO_RATE_LIMIT_WINDOW` — sliding window (Go duration, default `15m`).
  `ECONUMO_RATE_LIMIT_GLOBAL` — per-endpoint cap per minute across all keys (default `60`).
  `0` on a count disables that check (the window must be positive). Over-limit requests get HTTP 429 with the standard error envelope
  (message `"Too many attempts. Try again later."`, frozen). State is in-memory (resets on
  restart); a malformed value fails at boot.
- **Web UI config** — the SPA is ALWAYS embedded in the binary (`web/embed.go`,
  `//go:embed all:dist`); there is no disk-serving mode. Instance-specific
  values reach the frontend by being merged into the served `econumo-config.js`
  at runtime (the `Object.assign(window.econumoConfig, …)` suffix in
  `internal/web/spa`). One rule: the backend value overwrites the embedded
  default when present. Each key maps to `ECONUMO_<KEY>`:
  `ECONUMO_ALLOW_CUSTOM_API`, `ECONUMO_LILTAG_CONFIG_URL` (load liltag config
  from a URL instead of the bundled `liltag-config.json`),
  `ECONUMO_LILTAG_CACHE_TTL`, and `ECONUMO_VERSION` (UI version label; defaults
  to the binary's `internal/version.Version`, overridable for demo/staging).
  Flags (`ANALYTICS`, `ALLOW_REGISTRATION`) and `BILLING_URL` are always merged
  (server truth); text/URL keys merge only when non-empty. The composition root
  resolves the FS (`web.DistFS`) and version once in `server.BuildAPI`.
- `ECONUMO_LOG_LEVEL` — base slog level `debug|info|warn|error` (default `info`). Every command
  (`serve` and all resource:action commands) also accepts `-v`/`-vv`/`-vvv` (force DEBUG; `-vvv` adds source)
  and `-q` (quiet); flags override `ECONUMO_LOG_LEVEL`. Resolution lives in `internal/logging`.

  > **Env naming convention:** app-owned config is prefixed `ECONUMO_`; bare names are reserved for
  > ecosystem standards (`PORT`, `DATABASE_URL`, `MAILER_DSN`) and names the engine/vendor owns
  > (`SQLITE_BUSY_TIMEOUT`, `OPEN_EXCHANGE_RATES_TOKEN`).
- `X-Timezone` request header — the caller's IANA timezone, used for day-boundary math
  (e.g. an account's "balance as of end of today"); the tz database is embedded in the binary.
  A `users.timezone` column opportunistically persists it: a decorator around
  `middleware.TokenAuthenticator` (wired once in `server.BuildAPI`) writes the header's
  value on every authenticated request where it differs from the last value seen for that
  user (throttled by an in-memory per-user cache, same pattern as the token
  `last_used_at` throttle). MCP clients send no header, so `/mcp` (only) falls back to the
  stored value when none is present; the header always wins when it is present, on both
  edges — REST behavior is unchanged.
- Error `message`/`errors` text is rendered in the caller's language on BOTH edges:
  `httpx.WriteError` (REST) and `internal/web/mcp/helpers.go` (`MapErr`) resolve
  `reqctx.Language(ctx)` and render any coded message/field message via
  `i18n.T(lang, "errors."+code, params)`; code-less errors keep their literal English.
  The language follows the same fallback pattern as the timezone above, but on both
  edges: explicit `Accept-Language` header → stored `users.language` → `en`. The stored
  fallback runs inside the auth middleware via the optional
  `middleware.StoredLanguageResolver` capability, implemented by the server's
  authenticator decorator (`StoredLanguage`, `internal/server/glue_language.go`;
  `reqctx.IsLanguageExplicit` mirrors `IsLocationExplicit`).

### Logging

Two tiers of structured `log/slog` output, both with **static messages** and details as
fields (custom dimensions) — UUIDs only, never PII (no emails, bodies, or query strings):

- **Operation result** — one line per request, message = the static operation name (e.g.
  `create-category`), at INFO (2xx) / WARN (4xx) / ERROR (5xx & recovered panics). Always
  carries `request_id`, `status`, `route`, the user dimensions `user_id` + `timezone`, and
  `err`/`err_type` on failure.
- **Edge/transport** — a DEBUG `"http request"` line with `method`, `route`, `status`,
  `duration_ms`.

`request_id` is a UUIDv7 minted in the `RequestID` middleware and echoed on `X-Request-Id`.
The `AccessLog` middleware (`internal/web/middleware/accesslog.go`) installs a request-scoped
accumulator (`reqctx.WithLogAttrs`) and emits both lines; any layer enriches the operation
line with operation-specific params via `reqctx.AddLogAttr(ctx, key, value)` (e.g.
`category_id`). `/health` logs the transport line only; `OPTIONS` preflight is skipped.

## Database

- **SQLite** (default): pure-Go `modernc.org/sqlite` driver (CGO off).
- **PostgreSQL**: `jackc/pgx/v5` (stdlib), simple protocol (PgBouncer-safe).
- Migrations live in `internal/infra/storage/migrations/{sqlite,pgsql}` and run on boot.
- After changing a query: edit `query/{sqlite,pgsql}/*.sql` and regenerate with
  `sqlc generate` (config at `internal/infra/storage/sqlc/sqlc.yaml`).

## CLI / management commands

The binary is subcommand-driven (`cmd/econumo/main.go`): `serve` runs the server,
`healthcheck` probes a running one, and everything else routes to `internal/cli`:

```
user:create <name> <email> <password>
user:change-email <old> <new>
user:change-password <email> <password>
user:activate <email>
user:deactivate <email>
user:verify-email <email>
user:set-access <email> <full|readonly> [YYYY-MM-DD]
user:show <email>
currency:update-rates [date]
currency:add <code> [name] [fraction-digits]
token:purge [days]
data:remove-salt
```

`data:remove-salt` is a one-off migration that decrypts every user's email
back to plaintext, so `ECONUMO_DATA_SALT` can be removed. `lower(email)` is the
unique lookup key (unique expression index `users_email_lower_uniq`), so a
still-salted instance MUST run this command after upgrading, before that index
is relied on — otherwise emails stay encrypted and those users can't log in
(they are already login-broken pre-migration, so this is not a regression).
Run it **while the old salt is still set** (it needs it to decrypt), then
unset `ECONUMO_DATA_SALT` and restart. It refuses to run with an empty salt,
and is idempotent (already-plaintext rows are skipped).
Back up the DB first — the decryption is one-way in practice.

In the distroless image these run via the binary directly, e.g.
`docker exec <container> /app/econumo user:create …`.

## API conventions

- **Methods — only two.** `GET` for reads; `POST` for every write — create, update,
  AND delete. There is no `PUT`/`PATCH`/`DELETE`; deletes are POSTs.
- **Path shape:** `/api/v1/{module}/{action}-{subject}`, all kebab-case, the action
  verb leading. List endpoints end in `-list`. Examples from the source:
  - Reads (`GET`): `/api/v1/account/get-account-list`, `/api/v1/budget/get-budget`,
    `/api/v1/category/get-category-list`, `/api/v1/user/get-user-data`.
  - Writes (`POST`): `/api/v1/category/create-category`, `/api/v1/account/update-account`,
    `/api/v1/category/delete-category`, `/api/v1/connection/generate-invite`,
    `/api/v1/budget/set-limit`, `/api/v1/payee/archive-payee`.
- **Authentication is header-based:** send `Authorization: Bearer <token>` (an opaque
  access token, `eco_ses_*`/`eco_pat_*`; the scheme is case-insensitive). The auth
  middleware resolves the token's sha256 against the `access_tokens` table, rejects
  unknown/expired/revoked tokens with the 401 envelope, and puts the user id AND the
  token row id into the request context (the latter is the "current session" for
  logout/revoke/isCurrent). Public routes (login, register, remind-password,
  reset-password, confirm-email, resend-verification-code, `/api/doc`,
  `/api/doc.json`) need no header; everything else does.
- **Read-only access is enforced at the edge:** a caller whose access level is
  `readonly` (trial ended, no access granted) gets HTTP 402 on any `POST` route not
  in the middleware's small allowlist (account security actions — logout, session/PAT
  revocation, password update); `GET` reads are never restricted.

## Authentication

- **Method**: opaque bearer tokens stored (sha256-hashed) in the `access_tokens` table.
  Two kinds: `session` (minted at login; sliding 30-day TTL — expiry renews on use, with
  last-used persistence throttled to once per 5 minutes) and `personal` (user-created
  PATs with an optional fixed expiry, full access, shown exactly once at creation).
- The `user` feature owns everything: `Authenticate` (the per-request hot path),
  session/PAT use cases, and the revocation cascades. The middleware seam is
  `middleware.TokenAuthenticator`, wired to the user service in `server.BuildAPI`.
- Revocation cascades: `update-password` revokes all sessions EXCEPT the presenting
  one; `reset-password` and CLI `user:change-password` revoke ALL sessions;
  `user:deactivate` revokes sessions AND PATs (which is why per-request auth needs no
  `is_active` join). PATs survive password changes.
- Dead rows (expired/revoked > 30 days ago) are purged opportunistically at login;
  `token:purge [days]` does the same globally in one indexed DELETE (the
  revoked_at/expires_at indexes exist for it).
- Sessions/PAT management endpoints: `get-session-list`, `revoke-session`,
  `revoke-other-sessions`, `get-personal-token-list`, `create-personal-token`,
  `revoke-personal-token` (all under `/api/v1/user/`).
- Login lives under `internal/user/api` (`/api/v1/user/login-user`).
- Token refresh is not implemented (sessions slide instead; clients re-authenticate
  after 30 days of inactivity).

## Wire & data contract (frozen)

These behaviours are **frozen**: the web SPA and other clients parse exact JSON,
and the production database holds data written in these formats. Don't "clean
them up" — changing one breaks live clients, locks users out, or makes stored
data unreadable. Most are also asserted by the test suite.

### Response envelope (`internal/web/httpx/envelope.go`)
- Success (200): `{"success": true, "message": "", "data": <payload>}`
- Error (handled, default 400): `{"success": false, "message": <string>, "code": <int>, "errors": <object>}` — `errors` maps field → `[messages]`, always present (`{}` when none).
- Exception (500): `{"success": false, "message": <string>, "code": 0, "exceptionType": <string>}` — no `errors` key; error detail (message, stack trace) goes to the server logs only, never the response body.
- Not implemented (501): `{"success": false, "message": <string>, "code": 0, "errors": []}` — here `errors` is an array `[]` (the lone exception to the object rule).
- Rate-limited (429): `{"success": false, "message": "Too many attempts. Try again later.", "code": 429, "errors": {}}` — same shape as the handled-error envelope.
- JSON is encoded with HTML escaping disabled (`/`, `<`, `>` appear literally).
- **Server-rendered error language** (handled-error envelope only): `message` and the per-field
  `errors` strings are rendered server-side in the caller's language when the underlying error
  carries a catalogue code (`errors.*` keys in `locales/{en,ru}.json`; registry:
  `internal/shared/errs/codes.go`, `AllCodes`); errors without a code keep their literal English
  text. The language is `reqctx.Language`: `Accept-Language` (the SPA sends its selected locale
  on every request) → the authenticated user's stored `users.language` (resolved in the auth
  middleware via `middleware.StoredLanguageResolver`) → `en`. The `en` catalogue text matches the
  historical strings, so English callers see the same wire bytes as before; the envelope carries
  no separate code/translation fields.

### Auth crypto (`internal/infra/auth/`)
- **Password hash**: versioned by `users.algorithm`. `sha512` (legacy, all pre-existing rows): sha512, 500 iterations, base64 (88 chars), salt merged as `password{salt}`; `digest = sha512(salted)` then 499 rounds of `sha512(digest || salted)`; verify rejects len≠88 or a `$`, constant-time. `argon2id` (every new hash: registration and all password changes): PHC string `$argon2id$v=19$m=19456,t=2,p=1$…$…` (OWASP params), salt embedded in the hash — the `salt` column persists for sha512 rows. Verification dispatches on the column; unknown values fail closed.
- **User lookup key**: `lower(email)` — enforced unique by the expression index `users_email_lower_uniq`; repo lookups are `GetByEmail`/`ExistsByEmail` (`WHERE lower(email) = lower(?)`). The legacy `identifier` column is retained (dropping a NOT NULL UNIQUE column would force a SQLite table rebuild) but retired: it now holds the row's own `id` as a unique non-null placeholder, no longer derived from email. `EncodeService.Hash` (the md5 identifier derivation) was removed — nothing computes it anymore.
- **Email encryption**: emails are stored as plaintext. `EncodeService` still implements AES-128-CBC (key = raw salt, 16 bytes; layout `base64(iv[16] || hmac_sha256[32] || ciphertext)`, PKCS#7, random IV, HMAC verified constant-time before decrypt), but the API constructs it with an empty salt, so Encode/Decode are passthrough. The salted path runs only inside `data:remove-salt`.
- **Salt-free everywhere**: the API and all CLI user commands construct `EncodeService` with `""` and ignore `ECONUMO_DATA_SALT` entirely (`server.BuildAPI`, `cli` container). The salt reaches code through one path only: `data:remove-salt` passes it into `MigrateRemoveDataSalt(ctx, salt)`, which builds a temporary salted encoder to decrypt legacy email data to plaintext (the repo writes the row `id` into `identifier` automatically, so no re-derivation is needed).

### Access tokens (`internal/user/token.go`)
- Format: `eco_ses_` (session) / `eco_pat_` (personal) + `base64.RawURLEncoding` of 32
  random bytes (43 chars, alphabet `[A-Za-z0-9_-]`). Only `hex(sha256(token))` is stored.
- 401 messages are exact: `"Access token not found"` (missing/malformed header) and
  `"Invalid access token"` (unknown/expired/revoked token, and any internal
  authenticator error — no internals leak).
- Sessions: sliding `expires_at = last_used_at + 30d`, touched at most every 5 minutes.
  PAT `expires_at` never moves (NULL = never expires).

### Encodings, messages, routes
- Datetimes: `"2006-01-02 15:04:05"` — space separator, no zone, no fractional seconds.
- `isArchived` → int `0`/`1` (not bool); category `type` → alias string `"expense"`/`"income"`; empty string for NULL where the schema does.
- Validation strings are exact per language and asserted by tests in `en` (the default with no `Accept-Language` and no stored preference), e.g. `"Category name must be 3-64 characters"` (field `name`), `"Invalid credentials."` (401), `"This value should not be blank."` (code `common.is_blank`); coded errors render from the `errors.*` catalogue in the caller's language (see the envelope section).
- Exact route paths/methods are contract, e.g. `POST /api/v1/user/login-user`, `POST /api/v1/user/register-user`. Login takes `username` (email) + `password` and returns `{"token", "user"}`; register returns the created user **without** a token. Public routes: login, register, remind-password, reset-password, confirm-email, resend-verification-code, `/api/doc`, `/api/doc.json`; everything else needs a valid access token.
- Data: ids are stored as `TEXT`. New ids are UUIDv7; existing ids are never rewritten (they're FK targets and held by clients).
- `avatar` (user embeds) → `"<icon>:<color>"`, e.g. `"face:fuchsia"` — a Material
  icon ligature name plus a color slug from the 7-slug allowlist in
  `internal/user/avatar.go` (mirrored by `web/src/lib/avatars.ts`). Set via
  `POST /api/v1/user/update-avatar`; new users get a random default; Gravatar
  is gone.

## Key design decisions

- **One binary, two engines, runtime-selected** — both DB backends are linked in and chosen by `DATABASE_URL`; no Go plugins.
- **sqlc for compile-checked SQL** — a wrong column/arg fails `go build`; per-engine query variants only where the dialects diverge (see the engine-adapter pattern).
- **stdlib-first** — `net/http.ServeMux` routing, `func(http.Handler) http.Handler` middleware, hand-written `Validate()` (no tag DSL), stdlib CLI. Third-party deps only where stdlib can't deliver (decimal, DB drivers, uuid, sqlc, Resend).
- **No assembler layer** — app services build and return the result DTOs directly.
- **SQLite is the reference engine** — it's the default; PostgreSQL must match it byte-for-byte (enforced by the `enginecompare` suite).

### Notable behaviours
- **Budget element visibility**: a tag/envelope/category appears in `get-budget` when it has spending **or** a limit (current or carried-over) — so a tag with a limit but no transactions stays visible.
- **Account balance day boundary**: "balance as of end of today" uses the **caller's** timezone (`X-Timezone` header), not the server's UTC day.

## Deployment

- Image: `ghcr.io/econumo/econumo` (GitHub Container Registry only).
  - `:dev` — published locally via `make publish-dev`, or by the release workflow's "Publish Dev" checkbox.
  - `:latest` + `:vX.Y.Z` — published by the GitHub release workflow (latest only from `main`).
- Docker-free: each release attaches single-file linux binaries (SPA embedded)
  + `SHA256SUMS`; reference systemd unit in `deployment/systemd/econumo.service`,
  walkthrough in `docs/run-without-docker.md` (linked from the README).
  `make release-binaries` builds the same artifacts locally.
- Self-hosting: see the root `docker-compose.yml` (+ `.env.example`, copied to `.env`)
  and the README quick-start. The Dockerfile is `deployment/docker/Dockerfile`.

## Code Quality Tools

- `gofmt` (formatting), `go vet` (static analysis), the coverage gate (`make go-test`).
- Frontend: oxlint (`make web-lint`) + vitest (`make web-test`).

## Comments — write sparingly

Comment only **exceptional scenarios** and **non-obvious business logic / frozen-contract
rationale** — the *why*, not the *what*. Do NOT add:

- godoc or inline comments that merely restate a symbol's name or signature
  (`// Id returns the id`, `// CreateCategory creates a category`), accessor docs;
- section-divider / scaffolding comments, or anything that paraphrases the next line;
- references to the former PHP/Symfony implementation. The backend was ported from a
  now-removed Symfony 5.4 (PHP) app; state the behavior or constraint directly in Go
  terms rather than naming the old code (e.g. "timestamps are bare `Y-m-d H:i:s` with no
  zone, so the clock must be UTC" — not "to match the PHP DatetimeService").

**Exempt:** Swagger `// @…` annotation blocks on handlers (they generate the OpenAPI
spec) and `// Code generated … DO NOT EDIT.` markers — leave both intact.
