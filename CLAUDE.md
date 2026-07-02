# Repository Guidelines

This file provides guidance to AI agents when working with code in this repository.

## Project Overview

Econumo is a self-hosted personal finance and budgeting application. It consists of:
- **Backend**: Go (HTTP API + static SPA server) with hexagonal architecture (the Go module is the repo root).
- **Frontend**: Vue 3 + Quasar 2 SPA with TypeScript, in `web/`.
- **Database**: SQLite (default) or PostgreSQL — selected at runtime by `DATABASE_URL`.

> History: the backend was originally a Symfony 5.4 (PHP) app. It has been fully
> replaced by the Go backend and the PHP code has been removed. A single "cloud"
> edition is shipped (there is no separate "ce" edition).

The production artifact is a single self-contained Go binary in a distroless image
(`ghcr.io/econumo/econumo`) that serves both the JSON API and the built SPA, and
runs database migrations on boot.

## Development Commands

### Go backend

The Go module is the repo root. Tests run with the standard toolchain — no Docker
required for the smoke tier.

```bash
make test            # SMOKE: build + vet + gofmt + OpenAPI-docs-fresh + sqlite unit/integration + coverage gate
make regression      # REGRESSION: test + the sqlite-vs-PostgreSQL engine-comparison suite
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

The regression suite needs a PostgreSQL; `make regression` auto-provisions one
via the compose stack, or set `DATABASE_TEST_PGSQL_URL` to an existing database.

### Frontend (Vue/Quasar) — in `web/`

```bash
make web-install   # cd web && pnpm install
make web-dev       # cd web && npm run dev      (dev server)
make web-bundle    # cd web && npm run build    (production SPA build)
make web-lint      # cd web && npm run lint
```

### Publishing

```bash
make publish   # build + push the multi-arch `dev` image to ghcr.io/econumo/econumo:dev
```

Releases (`latest` + `vX.Y.Z`) are cut by the GitHub release workflow
(`.github/workflows/publish-release.yml`), not locally. Everything publishes to
`ghcr.io/econumo/econumo` only.

## Architecture

### Layered architecture (Hexagonal + DDD)

The backend follows a strict layered architecture with dependency inversion. The
dependency rule points inward: **ui → app → domain**; `infra` implements domain
interfaces. The app layer never imports `ui` or `infra`.

```
. (repo root = the Go module; web/ and deployment/ live alongside)
├── cmd/econumo/main.go ............ binary entrypoint; dispatches serve / healthcheck / resource:action commands
├── internal/
│   ├── shared/ .................... dependency-free kernel: vo (value objects), errs (domain error taxonomy),
│   │                                datetime (frozen wire/persistence layouts), jwt (RS256 issue/verify + keypair gen)
│   ├── reqctx/ .................... request-scoped values carried via context (e.g. caller timezone)
│   ├── domain/ .................... legacy: entities, value objects, repository INTERFACES, domain services (pure)
│   ├── app/ ....................... legacy: use-case services + request/result DTOs (depends on domain only)
│   ├── infra/ .................... implementations: sqlc repos, auth, mailer, storage
│   │   ├── repo/<feature>/ ....... repository implementations (engine-adapter pattern, see below)
│   │   ├── operation/ ............ shared row-based idempotency guard (operation_requests_ids) for
│   │   │                          client-supplied operation ids on create endpoints
│   │   ├── storage/sqlc/ ......... sqlc config + per-engine queries (query/{sqlite,pgsql}) and generated code (gen/{sqlite,pgsql})
│   │   ├── storage/migrations/ ... SQL migrations per engine ({sqlite,pgsql}); run on boot
│   │   ├── auth/ ................ password hashing + AES email encryption + user-identifier hashing
│   │   └── mailer/ .............. transactional email; transport from MAILER_DSN (console stdout | Resend API)
│   ├── ui/ ...................... HTTP edge: handlers, middleware, router, response envelope (httpx), SPA + apidoc
│   ├── server/ .................. composition root: server.BuildAPI wires every module (used by the binary AND tests)
│   ├── cli/ ..................... the resource:action management commands (stdlib dispatch; no cobra)
│   ├── config/ .................. environment configuration loading
│   └── test/ .................... shared test support: dbtest, fixture, testkeys, enginecompare, archtest
```

### Dependency rule

Features never import features (they stay decoupled via consumer-side ports
wired in `internal/server`); shared leaves (`shared`, `reqctx`, `ui`, `infra`)
never import a feature; the kernel (`internal/shared`, `internal/reqctx`)
imports nothing internal outside itself. This is enforced by
`internal/test/archtest`, which auto-detects feature packages (any
`internal/<top>` not in its infrastructure set) so newly moved features come
under enforcement without edits to the test. The legacy `internal/domain` and
`internal/app` layers are exempt from the feature-isolation rules until Phase 2
dissolves them into feature packages.

### The engine-adapter (sqlc) pattern

sqlc generates a SEPARATE package per engine (`gen/sqlite`, `gen/pgsql`) with
distinct Go types, and the two dialects differ (`?` vs `$N` placeholders, float vs
NUMERIC aggregates, date formats). A repo therefore:

- declares a small `querier` interface in the **canonical (sqlite-generated) types**,
- has two thin adapters — `sqlite.go` (native passthrough) and `pgsql.go` (whole-struct
  conversion shim) — selected ONCE in the constructor by `cfg.DatabaseDriver`,
- writes every method ONCE against the interface, so method bodies carry no
  per-driver branching.

Reference repos: `internal/infra/repo/{tag,user,currency,passwordrequest}`. The one
exception is `internal/infra/repo/budget/read.go`, which is hand-built dynamic SQL
(variadic `IN` lists, real per-engine value/date handling) and branches explicitly
by design.

### API handler pattern

HTTP handlers are thin adapters under `internal/ui/handler/<resource>/`: decode +
tier-1 `Validate()`, pull the user id from the JWT context, call the app service,
and emit the frozen response envelope via `httpx`. Routes are registered per module
in `routes.go` (`RegisterAPI`). Request/result DTOs live in `internal/app/<feature>/dto.go`.

### Frontend architecture (Vue 3 + Quasar)

Directory structure in `web/src/`: `pages/` (routes), `components/`, `composables/`,
`modules/api/v1/` (typed API clients), `stores/` (Pinia), `router/`, `i18n/`.
Build-time config comes from `web/.env.dist` (copied to `.env` at image build); the
version label shown in the UI is `ECONUMO_VERSION` (same name as the Docker build arg
that overrides it per build).

## Testing

Tests live alongside the Go code:
- `*_test.go` unit/integration tests per package (sqlite via `internal/test/dbtest`;
  dbtest applies production pragmas, e.g. `foreign_keys = ON`).
- `internal/test/apiparity/` — the shared API scenario catalogue: every registered
  route is replayed against the REAL production handler (`server.BuildAPI`).
  Two consumers: the untagged **smoke suite** (every `make test`) diffs each
  scenario's responses against committed golden files in `testdata/golden/`
  (normalized: generated UUIDs, datetimes, JWTs redacted), and the build-tagged
  parity suite below. Guard tests enforce that every route has a scenario, the
  scenario count and scanned-route count never shrink, and no golden is orphaned.
  Regenerate goldens with `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`,
  then INSPECT the diff — a golden change means observable behavior changed;
  never hand-edit a golden. If route-registration files move, update
  `handlerGlobs` in `guard_test.go`.
- `internal/test/enginecompare/` — the strongest contract: replays the same
  catalogue on BOTH SQLite and PostgreSQL and asserts byte-identical
  responses (build tag `enginecompare`).
- `internal/test/{fixture,testkeys}` — shared fixture builder + embedded JWT keypair.

Coverage gate: `make test` enforces a cross-package minimum (`GO_COVER_MIN`,
default 72). CI surfaces the coverage % in the Actions job summary plus an HTML
artifact (`.github/workflows/go-tests.yml`).

## Configuration

The Go server reads its environment from `.env` (see `.env.example`). Key vars:

- `DATABASE_URL` — `sqlite:///abs/path/db.sqlite` or `postgres://…`. Selects the engine.
  Required, and sourced from `.env` only — it is NOT baked into the image.
- `PORT` — HTTP listen port (required by the server), from `.env`. The image keeps a
  fallback `PORT=80` (compose maps `8181:80`); for a host `go run`, set a non-privileged
  port (e.g. `8181`) in `.env`.
- `ECONUMO_JWT_PRIVATE_KEY_PATH` / `ECONUMO_JWT_PUBLIC_KEY_PATH` / `ECONUMO_JWT_PASSPHRASE` —
  RS256 keypair (paths may use the Symfony-style `%kernel.project_dir%` placeholder, which is
  expanded to the cwd). Defaults to `var/jwt/{private,public}.pem` and is auto-generated on first
  boot if missing (no keys are committed or baked into the image). `jwt:generate`
  generates one explicitly. In the image these resolve under `/app/var/jwt`; persist
  the `/app/var` volume (db + keys) to keep data and tokens valid.
- `ECONUMO_DATA_SALT` — **Deprecated and IGNORED by the API/repositories**, which always run
  salt-free (plaintext emails, `md5(lower(email))` identifiers). It is consumed by exactly one
  code path, the `data:remove-salt` migration (below), which reads it to decrypt existing data.
  Set it to your old salt, run that command, then unset it. Until you migrate, a still-salted
  database has unreadable emails / mismatched identifiers, so those users cannot log in (the
  intended push to migrate); `serve` logs a WARN at boot while it is set.
- `ECONUMO_ALLOW_REGISTRATION` — enable/disable the register endpoint.
- `ECONUMO_CORS_ALLOW_ORIGIN` — comma-separated cross-origin allowlist. Empty (default) = same-domain
  only (no `Access-Control-Allow-Origin` emitted; the bundled SPA and API share an origin so it
  just works). A configured origin is reflected back with `Vary: Origin`; `*` allows any origin.
- `ECONUMO_CURRENCY_BASE` — base currency (default `USD`).
- `ECONUMO_DEBUG` — `true` exposes 500 stack traces (default `false`). Replaces the former `APP_ENV`.
- `MAILER_DSN` — mail transport for password-reset email; the scheme selects the provider, exactly
  as `DATABASE_URL`'s scheme selects the DB engine. Empty (default) = the **console** transport (renders
  each email to stdout — a dev aid that never silently drops mail); `resend://<api_key>` sends via Resend.
  From / Reply-To fold in as query params: `resend://<key>?from=…&reply_to=…` (also accepted by
  `console://`/`log://`). Parsed once in `config.Load` (a bad scheme fails at boot). Replaces the former
  `RESEND_API_KEY` / `ECONUMO_MAIL_FROM` / `ECONUMO_MAIL_REPLY_TO`.
- `OPEN_EXCHANGE_RATES_TOKEN` — currency-rate updates.
- `SQLITE_BUSY_TIMEOUT` — SQLite `busy_timeout` PRAGMA in ms (default `0`); bare name mirrors the engine pragma.
- `ECONUMO_WEB_DIST` — path to the built SPA the binary serves.
- `ECONUMO_LOG_LEVEL` — base slog level `debug|info|warn|error` (default `info`). Every command
  (`serve` and all resource:action commands) also accepts `-v`/`-vv`/`-vvv` (force DEBUG; `-vvv` adds source)
  and `-q` (quiet); flags override `ECONUMO_LOG_LEVEL`. Resolution lives in `internal/logging`.

  > **Env naming convention:** app-owned config is prefixed `ECONUMO_`; bare names are reserved for
  > ecosystem standards (`PORT`, `DATABASE_URL`, `MAILER_DSN`) and names the engine/vendor owns
  > (`SQLITE_BUSY_TIMEOUT`, `OPEN_EXCHANGE_RATES_TOKEN`).
- `X-Timezone` request header — the caller's IANA timezone, used for day-boundary math
  (e.g. an account's "balance as of end of today"); the tz database is embedded in the binary.

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
The `AccessLog` middleware (`internal/ui/middleware/accesslog.go`) installs a request-scoped
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
currency:update-rates [date]
currency:add <code> [name] [fraction-digits]
jwt:generate
data:remove-salt
```

`data:remove-salt` is a one-off migration that decrypts every user's email
back to plaintext and re-derives the identifier as `md5(lower(email))` (no salt),
so `ECONUMO_DATA_SALT` can be removed. Run it **while the old salt is still set**
(it needs it to decrypt), then unset `ECONUMO_DATA_SALT` and restart. It refuses
to run with an empty salt, and is idempotent (already-plaintext rows are skipped).
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
- **Authentication is header-based:** send `Authorization: Bearer <token>` (RS256 JWT;
  the scheme is case-insensitive). The JWT middleware verifies the signature, rejects
  expired/invalid tokens with the 401 envelope, and puts the `id` claim (user UUID)
  into the request context for handlers. Public routes (login, register, remind-password,
  reset-password, `/api/doc`, `/api/doc.json`) need no header; everything else does.

## Authentication

- **Method**: JWT (RS256) via the `golang-jwt/jwt` library, in `internal/shared/jwt` (a
  self-contained package: token issue/verify, keypair generation, and the shared
  `EnsureKeypair` boot/CLI entry point; no `internal/*` dependency).
- Login lives under `internal/ui/handler/user` (`/api/v1/user/login-user`).
- Token refresh is not implemented (clients re-authenticate).

## Wire & data contract (frozen)

These behaviours are **frozen**: the web SPA and other clients parse exact JSON,
and the production database holds data written in these formats. Don't "clean
them up" — changing one breaks live clients, locks users out, or makes stored
data unreadable. Most are also asserted by the test suite.

### Response envelope (`internal/ui/httpx/envelope.go`)
- Success (200): `{"success": true, "message": "", "data": <payload>}`
- Error (handled, default 400): `{"success": false, "message": <string>, "code": <int>, "errors": <object>}` — `errors` maps field → `[messages]`, always present (`{}` when none).
- Exception (500): `{"success": false, "message": <string>, "code": 0, "exceptionType": <string>}` — no `errors` key; `stackTrace` only when `ECONUMO_DEBUG=true`.
- Not implemented (501): `{"success": false, "message": <string>, "code": 0, "errors": []}` — here `errors` is an array `[]` (the lone exception to the object rule).
- JSON is encoded with HTML escaping disabled (`/`, `<`, `>` appear literally).

### Auth crypto (`internal/infra/auth/`)
- **Password hash**: sha512, 500 iterations, base64 (88 chars). Salt merged as `password{salt}`; `digest = sha512(salted)` then 499 rounds of `sha512(digest || salted)`. Verify rejects len≠88 or a `$`, constant-time.
- **User identifier**: `hex(md5(lower(email)))` — 32-char hex; the primary user lookup key. (`EncodeService` still supports a salted form `hex(md5(lower(email) || salt))`, but only the `data:remove-salt` migration uses it — see below.)
- **Email encryption**: emails are stored as plaintext. `EncodeService` still implements AES-128-CBC (key = raw salt, 16 bytes; layout `base64(iv[16] || hmac_sha256[32] || ciphertext)`, PKCS#7, random IV, HMAC verified constant-time before decrypt), but the API constructs it with an empty salt, so Encode/Decode are passthrough. The salted path runs only inside `data:remove-salt`.
- **Salt-free everywhere**: the API and all CLI user commands construct `EncodeService` with `""` and ignore `ECONUMO_DATA_SALT` entirely (`server.BuildAPI`, `cli` container). The salt reaches code through one path only: `data:remove-salt` passes it into `MigrateRemoveDataSalt(ctx, salt)`, which builds a temporary salted encoder to decrypt legacy data and re-derive identifiers as `md5(lower(email))`.

### JWT (`internal/shared/jwt/jwt.go`)
- RS256 only (issue + verify reject any other alg — defends against `none`/HS256 confusion).
- Claims: `iat`; `exp = iat + 2592000` (30-day TTL); `roles = ["ROLE_USER"]`; `username` = plaintext email; `id` = user UUID. No `nbf`/`iss`/`sub`/`aud`.

### Encodings, messages, routes
- Datetimes: `"2006-01-02 15:04:05"` — space separator, no zone, no fractional seconds.
- `isArchived` → int `0`/`1` (not bool); category `type` → alias string `"expense"`/`"income"`; empty string for NULL where the schema does.
- Validation strings are exact and asserted by clients/tests, e.g. `"Category name must be 3-64 characters"` (field `name`), `"Invalid credentials."` (401), `"This value should not be blank."` (code `IS_BLANK_ERROR`).
- Exact route paths/methods are contract, e.g. `POST /api/v1/user/login-user`, `POST /api/v1/user/register-user`. Login takes `username` (email) + `password` and returns `{"token", "user"}`; register returns the created user **without** a token. Public routes: login, register, remind-password, reset-password, `/api/doc`, `/api/doc.json`; everything else needs a valid JWT.
- Data: ids are stored as `TEXT`. New ids are UUIDv7; existing ids are never rewritten (they're JWT claims, FK targets, and held by clients).

## Key design decisions

- **One binary, two engines, runtime-selected** — both DB backends are linked in and chosen by `DATABASE_URL`; no Go plugins.
- **sqlc for compile-checked SQL** — a wrong column/arg fails `go build`; per-engine query variants only where the dialects diverge (see the engine-adapter pattern).
- **stdlib-first** — `net/http.ServeMux` routing, `func(http.Handler) http.Handler` middleware, hand-written `Validate()` (no tag DSL), stdlib CLI. Third-party deps only where stdlib can't deliver (decimal, JWT, DB drivers, uuid, sqlc, Resend).
- **No assembler layer** — app services build and return the result DTOs directly.
- **SQLite is the reference engine** — it's the default; PostgreSQL must match it byte-for-byte (enforced by the `enginecompare` suite).

### Notable behaviours
- **Budget element visibility**: a tag/envelope/category appears in `get-budget` when it has spending **or** a limit (current or carried-over) — so a tag with a limit but no transactions stays visible.
- **Account balance day boundary**: "balance as of end of today" uses the **caller's** timezone (`X-Timezone` header), not the server's UTC day.

## Deployment

- Image: `ghcr.io/econumo/econumo` (GitHub Container Registry only).
  - `:dev` — published locally via `make publish`.
  - `:latest` + `:vX.Y.Z` — published by the GitHub release workflow (latest only from `main`).
- Self-hosting: see the root `docker-compose.yml` (+ `.env.example`, copied to `.env`)
  and the README quick-start. The Dockerfile is `deployment/docker/Dockerfile`.

## Code Quality Tools

- `gofmt` (formatting), `go vet` (static analysis), the coverage gate (`make test`).
- Frontend: ESLint (`make web-lint`).

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
