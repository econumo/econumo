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
make test            # SMOKE: build + vet + gofmt + sqlite unit/integration + coverage gate
make regression      # REGRESSION: test + the sqlite-vs-PostgreSQL engine-comparison suite
make test-fast       # Just the fast sqlite tests (no lint/coverage)

# Or directly with the go toolchain (run from the repo root):
go test ./...                        # all tests
go build ./...                        # build everything
go run ./cmd/econumo serve            # run the server (reads .env)
go run ./cmd/econumo app:create-user "Name" user@example.test secret
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
├── cmd/econumo/main.go ............ binary entrypoint; dispatches serve / healthcheck / app:* commands
├── internal/
│   ├── domain/ .................... entities, value objects, repository INTERFACES, domain services (pure)
│   ├── app/ ...................... use-case services + request/result DTOs (depends on domain only)
│   │   └── reqctx/ .............. request-scoped values carried via context (e.g. caller timezone)
│   ├── infra/ ................... implementations: sqlc repos, auth/JWT, mailer, storage
│   │   ├── repo/<feature>/ ...... repository implementations (engine-adapter pattern, see below)
│   │   ├── storage/sqlc/ ........ sqlc config + per-engine queries (query/{sqlite,pgsql}) and generated code (gen/{sqlite,pgsql})
│   │   ├── storage/migrations/ .. SQL migrations per engine ({sqlite,pgsql}); run on boot
│   │   ├── auth/ ............... JWT (RS256) + password hashing + AES email encryption
│   │   └── mailer/ ............. transactional email via the Resend API
│   ├── ui/ ..................... HTTP edge: handlers, middleware, router, response envelope (httpx), SPA + apidoc
│   ├── server/ ................. composition root: server.BuildAPI wires every module (used by the binary AND tests)
│   ├── cli/ ................... the `app:*` management commands (stdlib dispatch; no cobra)
│   ├── config/ ............... environment configuration loading
│   └── test/ ................. shared test support: dbtest, fixture, testkeys, enginecompare
```

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
version label shown in the UI is `ECONUMO_EDITION_LABEL`, overridden per build by
the `ECONUMO_VERSION` build arg.

## Testing

Tests live alongside the Go code:
- `*_test.go` unit/integration tests per package (sqlite via `internal/test/dbtest`).
- `internal/test/enginecompare/` — the strongest contract: runs the REAL production
  handler (`server.BuildAPI`) on BOTH SQLite and PostgreSQL and asserts byte-identical
  responses (build tag `enginecompare`).
- `internal/test/{fixture,testkeys}` — shared fixture builder + embedded JWT keypair.

Coverage gate: `make test` enforces a cross-package minimum (`GO_COVER_MIN`,
default 64). CI surfaces the coverage % in the Actions job summary plus an HTML
artifact (`.github/workflows/go-tests.yml`).

## Configuration

The Go server reads its environment from `.env` (see `.env.example`). Key vars:

- `DATABASE_URL` — `sqlite:///abs/path/db.sqlite` or `postgres://…`. Selects the engine.
- `PORT` — HTTP listen port (required by the server). Not kept in `.env`: the image
  hardcodes `80` (map it to any host port in compose) and `make run` sets it (default
  8181, override `RUN_PORT`). Set it yourself only for a bare `go run`.
- `JWT_SECRET_KEY` / `JWT_PUBLIC_KEY` / `JWT_PASSPHRASE` — RS256 keypair (paths may use
  the Symfony-style `%kernel.project_dir%` placeholder, which is expanded to the cwd).
  Defaults to `var/jwt/{private,public}.pem` and is auto-generated on first boot if
  missing (no keys are committed or baked into the image). `app:generate-jwt-keypair`
  generates one explicitly. In the image these resolve under `/app/var/jwt`; persist
  the `/app/var` volume (db + keys) to keep data and tokens valid.
- `ECONUMO_DATA_SALT` — AES key + identifier-hash salt (must match the data it reads).
  **Deprecated — will be removed in a future version.** Migrate to plaintext with
  `app:remove-data-salt` (below) and leave it unset.
- `ECONUMO_ALLOW_REGISTRATION` — enable/disable the register endpoint.
- `ECONUMO_CURRENCY_BASE` — base currency (default `USD`).
- `RESEND_API_KEY` + `ECONUMO_FROM_EMAIL` (+ optional `ECONUMO_REPLY_TO_EMAIL`) — enable
  password-reset email via Resend; without them the code is still written but no mail is sent.
- `OPEN_EXCHANGE_RATES_TOKEN` — currency-rate updates.
- `ECONUMO_SPA_DIR` — path to the built SPA the binary serves.
- `X-Timezone` request header — the caller's IANA timezone, used for day-boundary math
  (e.g. an account's "balance as of end of today"); the tz database is embedded in the binary.

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
app:create-user <name> <email> <password>
app:change-user-email <old> <new>
app:change-user-password <email> <password>
app:activate-user <email>
app:deactivate-users --date=YYYY-MM-DD
app:update-currency-rates [date]
app:add-currency <code> [name] [fraction-digits]
app:generate-jwt-keypair
app:remove-data-salt
```

`app:remove-data-salt` is a one-off migration that decrypts every user's email
back to plaintext and re-derives the identifier as `md5(lower(email))` (no salt),
so `ECONUMO_DATA_SALT` can be removed. Run it **while the old salt is still set**
(it needs it to decrypt), then unset `ECONUMO_DATA_SALT` and restart. It refuses
to run with an empty salt, and is idempotent (already-plaintext rows are skipped).
Back up the DB first — the decryption is one-way in practice.

In the distroless image these run via the binary directly, e.g.
`docker exec <container> /app/econumo app:create-user …`.

## Authentication

- **Method**: JWT (RS256) via the `golang-jwt/jwt` library, in `internal/infra/auth`.
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
- Exception (500): `{"success": false, "message": <string>, "code": 0, "exceptionType": <string>}` — no `errors` key; `stackTrace` only when `APP_ENV=dev`.
- Not implemented (501): `{"success": false, "message": <string>, "code": 0, "errors": []}` — here `errors` is an array `[]` (the lone exception to the object rule).
- JSON is encoded with HTML escaping disabled (`/`, `<`, `>` appear literally).

### Auth crypto (`internal/infra/auth/`)
- **Password hash**: sha512, 500 iterations, base64 (88 chars). Salt merged as `password{salt}`; `digest = sha512(salted)` then 499 rounds of `sha512(digest || salted)`. Verify rejects len≠88 or a `$`, constant-time.
- **User identifier**: `hex(md5(lower(email) || ECONUMO_DATA_SALT))` — 32-char hex; the primary user lookup key.
- **Email encryption**: AES-128-CBC, key = raw `ECONUMO_DATA_SALT` (exactly 16 bytes); layout `base64(iv[16] || hmac_sha256[32] || ciphertext)`, PKCS#7, random IV, HMAC verified (constant-time) before decrypt. Empty salt → passthrough.
- **Empty-salt (plaintext) mode**: an unset `ECONUMO_DATA_SALT` is fully supported — the email column holds plaintext and the identifier is `md5(lower(email))` (no salt). To move an existing salted database into this mode, run `app:remove-data-salt` (above) before unsetting the salt.

### JWT (`internal/infra/auth/jwt.go`)
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
