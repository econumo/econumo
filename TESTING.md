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
  a real migrated in-memory SQLite via `internal/test/dbtest`: CRUD round-trips,
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

`internal/test/dbtest` centralizes DB setup so tests don't duplicate it:

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
build-tagged (`enginecompare`) so it never slows the smoke tier. It asserts that
SQLite and PostgreSQL produce **identical** output (SQLite is the reference /
target engine) at two levels:

- **Repository level** — runs the same repository operation on both engines and
  compares a deterministic snapshot. Narrow and fast; pinpoints which query
  diverges.
- **API level (comprehensive)** — stands up the **real production HTTP handler**
  (`internal/server.BuildAPI`, the identical router `cmd/econumo` serves) over
  each engine from an **identical seed**, replays a broad catalogue of requests
  (every read endpoint plus a write→read sequence per mutating module), and
  compares the **raw response bytes**. This is the strongest parity contract —
  it exercises middleware, JWT, the per-engine sqlc adapters, decimal/datetime
  handling, and envelope serialization end-to-end. Server-generated UUIDv7 ids
  (which legitimately differ per run) are redacted before comparison; all other
  bytes are compared strictly.

Both guard against drift between the two sqlc engine adapters (placeholders,
types, datetime + decimal handling, upsert syntax, result ordering).

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

Scenarios live in `internal/test/enginecompare`:

- **Repository scenarios** (`scenarios_test.go`): write a `scenario` closure (seed
  via the portable `seed(...)` helper, call repos, return a deterministic snapshot
  string) and pass it to `runOnBoth(t, ...)`.
- **API scenarios** (`apiparity_test.go`): add an `apiScenario` returning an
  ordered `[]apiCall` (method, path, which seeded user's token, JSON body) and
  pass it to `runAPIOnBoth(t, name, ...)`. The shared fixture lives in
  `apiparity_harness_test.go` (`seedAPIFixture`); extend it when a new scenario
  needs more seed data, then bump `apiCatalogueSize()`.

## CI

`.github/workflows/go-tests.yml` runs on every push/PR touching `go/`:

- **smoke** job: `make go-test` (build, vet, gofmt, sqlite tests, coverage gate).
- **regression** job (needs smoke): spins up a `postgres:17` service container
  and runs `make go-test-engines` against it (the sqlite-vs-pgsql comparison).
