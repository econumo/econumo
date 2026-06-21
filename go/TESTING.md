# Testing

The Go backend has two test tiers, both run with `CGO_ENABLED=0` (the production
build):

| Tier | Command | What it runs | Dependencies |
|------|---------|--------------|--------------|
| **Smoke** | `make go-test` | unit + sqlite tests + lint + coverage gate | none |
| **Regression** | `make go-regression` | everything in smoke **+** sqlite-vs-PostgreSQL comparison | PostgreSQL |

Run **smoke** constantly / before every commit. Run **regression** before
merging or releasing.

## Smoke (unit + sqlite, no dependencies)

Pure Go, SQLite-only. `make go-test` = `go-lint` + `go-test-cover`:

```sh
make go-test          # lint + sqlite tests + coverage gate (the everyday command)

# or the building blocks individually:
make go-test-fast     # just the fast sqlite tests, no lint/coverage
make go-test-cover    # + enforce the coverage gate (fails below GO_COVER_MIN)
make go-lint          # build + go vet + gofmt check
```

It covers, in layers:

- **Domain unit tests** (`internal/domain/**`) — entities + value objects, pure
  and dependency-free: validation rules, state transitions, the "bump updatedAt
  only on real change" invariants, boundary values.
- **Repository integration tests** (`internal/infra/repo/**`) — every repo against
  a real migrated in-memory SQLite via `internal/testutil`: CRUD round-trips,
  NotFound paths, own-vs-shared list queries, datetime/decimal boundaries,
  idempotency.
- **HTTP/handler tests** (`internal/ui/handler/**`) — the real `net/http` stack
  (router + middleware + service + repo) via `httptest`, asserting the exact wire
  envelope, status codes, validation messages, and resulting DB state after
  mutations.
- **Crypto golden vectors** (`internal/infra/auth`) — password hashing, AES
  encode/decode, JWT — pinned against values captured from the PHP backend.
- **Middleware / httpx / router** — CORS, recover, JWT, timezone, the response
  envelope + error→HTTP mapping, 404/405, SPA fallback.

### Shared test helper

`internal/testutil` centralizes DB setup so tests don't duplicate it:

```go
db := testutil.NewSQLite(t)            // fresh migrated in-memory DB, auto-closed
repo := categoryrepo.NewRepo(db.Engine, db.TX)
db.Exec(t, `INSERT INTO ...`, args...) // seed fixture rows
```

### Coverage gate

`make go-test-cover` measures **true cross-package coverage** of `./internal/...`
(handler tests exercise the app + repo layers, which own-package coverage
under-reports) and fails below `GO_COVER_MIN` (default 64). Raise the floor as
coverage grows:

```sh
make go-test-cover GO_COVER_MIN=70
```

## Regression (smoke + sqlite-vs-PostgreSQL)

`make go-regression` = smoke + the engine-comparison suite. The comparison is
build-tagged (`enginecompare`) so it never slows the smoke tier. Each scenario
runs the **same** repository operations against both a migrated SQLite DB and a
real PostgreSQL DB and asserts the results are **identical** — guarding against
drift between the two sqlc engine adapters (placeholders, types, datetime +
decimal handling, upsert syntax).

```sh
# Full regression. Auto-provisions a throwaway econumo_test DB in the compose
# `postgres` service (start it first: make go-up  — or  docker compose up -d postgres).
make go-regression

# Point it at any external Postgres instead:
make go-regression \
  DATABASE_TEST_PGSQL_URL='postgres://user:pass@host:5432/dbname?sslmode=disable'
```

Just the comparison (without re-running smoke):

```sh
make go-test-engines   # uses DATABASE_TEST_PGSQL_URL; pgsql half SKIPS if unreachable,
                       # sqlite half still runs the scenarios
```

Each test creates its own freshly-migrated schema and drops it afterwards, so
the suite is safe to re-run against the same database repeatedly.

Scenarios live in `internal/enginecompare`. Add one by writing a `scenario`
closure (seed via the portable `seed(...)` helper, call repos, return a
deterministic snapshot string) and passing it to `runOnBoth(t, ...)`.

## CI

`.github/workflows/go-tests.yml` runs on every push/PR touching `go/`:

- **smoke** job: `make go-test` (build, vet, gofmt, sqlite tests, coverage gate).
- **regression** job (needs smoke): spins up a `postgres:17` service container
  and runs `make go-test-engines` against it (the sqlite-vs-pgsql comparison).
