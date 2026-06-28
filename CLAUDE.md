# Repository Guidelines

This file provides guidance to AI agents when working with code in this repository.

## Project Overview

Econumo is a self-hosted personal finance and budgeting application. It consists of:
- **Backend**: Go (HTTP API + static SPA server) with hexagonal architecture, in `go/`.
- **Frontend**: Vue 3 + Quasar 2 SPA with TypeScript, in `web/`.
- **Database**: SQLite (default) or PostgreSQL — selected at runtime by `DATABASE_URL`.

> History: the backend was originally a Symfony 5.4 (PHP) app. It has been fully
> replaced by the Go backend and the PHP code has been removed. A single "cloud"
> edition is shipped (there is no separate "ce" edition).

The production artifact is a single self-contained Go binary in a distroless image
(`ghcr.io/econumo/econumo`) that serves both the JSON API and the built SPA, and
runs database migrations on boot.

## Development Commands

### Go backend (in `go/`)

The Go module lives in `go/` (`go.work` at the repo root ties it together). Tests
run with the standard toolchain — no Docker required for the smoke tier.

```bash
make go-test         # SMOKE: build + vet + gofmt + sqlite unit/integration + coverage gate
make go-regression   # REGRESSION: go-test + the sqlite-vs-PostgreSQL engine-comparison suite
make go-test-fast    # Just the fast sqlite tests (no lint/coverage)
make go-image        # Build the Go backend Docker image locally (single-arch, --load)

# Inside go/ directly:
cd go && go test ./...                        # all tests
cd go && go build ./...                        # build everything
cd go && go run ./cmd/econumo serve            # run the server (reads go/.env)
cd go && go run ./cmd/econumo app:create-user "Name" user@example.test secret
```

The regression suite needs a PostgreSQL; `make go-regression` auto-provisions one
via the compose stack, or set `DATABASE_TEST_PGSQL_URL` to an existing database.

### Frontend (Vue/Quasar) — in `web/`

```bash
make install   # cd web && pnpm install
make dev       # cd web && npm run dev      (dev server)
make bundle    # cd web && npm run build    (production SPA build)
make lint      # cd web && npm run lint
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
go/
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

Tests live alongside the Go code in `go/`:
- `*_test.go` unit/integration tests per package (sqlite via `internal/test/dbtest`).
- `internal/test/enginecompare/` — the strongest contract: runs the REAL production
  handler (`server.BuildAPI`) on BOTH SQLite and PostgreSQL and asserts byte-identical
  responses (build tag `enginecompare`).
- `internal/test/{fixture,testkeys}` — shared fixture builder + embedded JWT keypair.

Coverage gate: `make go-test` enforces a cross-package minimum (`GO_COVER_MIN`,
default 64). CI surfaces the coverage % in the Actions job summary plus an HTML
artifact (`.github/workflows/go-tests.yml`).

## Configuration

The Go server reads its environment from `go/.env` (see `go/.env.example`). Key vars:

- `DATABASE_URL` — `sqlite:///abs/path/db.sqlite` or `postgres://…`. Selects the engine.
- `PORT` — HTTP listen port (required).
- `JWT_SECRET_KEY` / `JWT_PUBLIC_KEY` / `JWT_PASSPHRASE` — RS256 keypair (paths may use
  the Symfony-style `%kernel.project_dir%` placeholder, which is expanded to the cwd).
  Generate with `app:generate-jwt-keypair`. A committed dev keypair lives in `config/jwt/`.
- `ECONUMO_DATA_SALT` — AES key + identifier-hash salt (must match the data it reads).
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
- Migrations live in `go/internal/infra/storage/migrations/{sqlite,pgsql}` and run on boot.
- After changing a query: edit `query/{sqlite,pgsql}/*.sql` and regenerate with
  `sqlc generate` (config at `go/internal/infra/storage/sqlc/sqlc.yaml`).

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
```

The distroless image creates a `bin/console` symlink to the binary, so legacy
`bin/console app:…` invocations keep working.

## Authentication

- **Method**: JWT (RS256) via the `golang-jwt/jwt` library, in `internal/infra/auth`.
- Login lives under `internal/ui/handler/user` (`/api/v1/user/login-user`).
- Token refresh is not implemented (clients re-authenticate).

## Deployment

- Image: `ghcr.io/econumo/econumo` (GitHub Container Registry only).
  - `:dev` — published locally via `make publish`.
  - `:latest` + `:vX.Y.Z` — published by the GitHub release workflow (latest only from `main`).
- Self-hosting: see `deployment/docker-compose/` (`docker-compose.yml` + `.env.example`)
  and the README quick-start. The Dockerfile is `deployment/docker/go/Dockerfile`.

## Code Quality Tools

- `gofmt` (formatting), `go vet` (static analysis), the coverage gate (`make go-test`).
- Frontend: ESLint (`make lint`).
