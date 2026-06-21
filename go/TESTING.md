# Testing

The Go backend has two test suites: a **fast** one (default) and a separate
**engine-comparison** one. Both run with `CGO_ENABLED=0` (the production build).

## Fast suite (default)

Pure Go, SQLite-only, no external dependencies. This is what you run constantly
while developing.

```sh
make go-test          # run it
make go-test-cover    # run it + enforce the coverage gate (fails below GO_COVER_MIN)
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

## Engine-comparison suite (sqlite vs PostgreSQL)

Build-tagged (`enginecompare`) so it never slows the fast suite. Each scenario
runs the **same** repository operations against both a migrated SQLite DB and a
real PostgreSQL DB and asserts the results are **identical** — this is what
guards against drift between the two sqlc engine adapters (placeholders, types,
datetime + decimal handling, upsert syntax).

```sh
# Without Postgres: the pgsql half SKIPS, the sqlite half still runs the scenarios.
make go-test-engines

# With Postgres: the full comparison. Point it at any throwaway database
# (each test gets its own freshly-migrated schema and drops it afterwards).
make go-test-engines \
  DATABASE_TEST_PGSQL_URL='postgres://econumo:econumo@localhost:5432/econumo_test?sslmode=disable'
```

Scenarios live in `internal/enginecompare`. Add one by writing a `scenario`
closure (seed via the portable `seed(...)` helper, call repos, return a
deterministic snapshot string) and passing it to `runOnBoth(t, ...)`.

## CI

`.github/workflows/go-tests.yml` runs on every push/PR touching `go/`:

- **test** job: build, vet, gofmt, and `make go-test-cover` (the coverage gate).
- **engine-compare** job: spins up a `postgres:17` service container and runs
  `make go-test-engines` against it (full sqlite-vs-pgsql comparison).
