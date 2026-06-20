# sqlc query layer

Compile-time-checked SQL for Econumo. Queries are written by hand in `.sql`
files under `query/`; [sqlc](https://sqlc.dev) generates typed Go so a wrong
column or argument fails at build time instead of at request time.

## Layout

```
sqlc/
  sqlc.yaml          # v2 config: one generation target per engine
  gen.go             # hosts the //go:generate sqlc generate directive
  query/             # hand-written .sql (added per module; *.sql by engine where SQL diverges)
  gen/
    sqlite/          # package sqlitegen — generated, do not edit
    pgsql/           # package pgsqlgen  — generated, do not edit
```

Schema input is the embedded migration SQL — **not** a separate file:

- SQLite schema: `go/migrations/sqlite/`
- PostgreSQL schema: `go/migrations/pgsql/`

One schema source feeds two consumers (the migration runner at boot and sqlc at
codegen), so schema and queries can't drift: a query against a dropped/renamed
column fails `sqlc generate`.

## Regenerating

`sqlc generate` is wired to `go generate` via the directive in `gen.go`:

```go
//go:generate sqlc generate
```

Run from the module root (`go/`):

```sh
go generate ./internal/infra/storage/sqlc/...
# or regenerate everything:
go generate ./...
```

`sqlc` must be on `PATH` (install: `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`).
The generated `gen/sqlite` and `gen/pgsql` packages are committed; CI regenerates
and diffs to fail on stale output.

## Two engines, one Querier

`emit_interface: true` makes both engines emit a `Querier` interface that their
concrete `*Queries` satisfies:

- `gen/sqlite` → package `sqlitegen`, `sqlitegen.Querier`, `sqlitegen.New(DBTX)`
- `gen/pgsql`  → package `pgsqlgen`, `pgsqlgen.Querier`, `pgsqlgen.New(DBTX)`

The two interfaces share the same method set (same query names, same Go types,
thanks to the type overrides), so the repo layer programs against that shape and
the selected `Backend` decides which engine's `New(...)` is called at startup.
Per-engine placeholder and type differences (`$1` vs `?`, uuid storage, upsert
syntax) are resolved at codegen — there is no runtime `Rebind`/dialect layer.

## Type overrides

- `uuid` / `CHAR(36)` → `string` (adapted to domain `Id` / `Identifier` in repos)
- `NUMERIC(19,8)` → `string` (adapted to `shopspring/decimal` at scale 8 to match
  PHP bcmath)

## Binding a Querier to the current context

The repo layer never holds a long-lived `*Queries`. Instead it obtains the
executor for the current unit of work and wraps it:

```go
qx := txManager.Querier(ctx)        // *sql.Tx if inside WithTx, else *sql.DB
q := sqlitegen.New(qx)              // (or pgsqlgen.New(qx), chosen by Backend)
row, err := q.GetUserByIdentifier(ctx, identifier)
```

Because `TxManager.Querier(ctx)` returns the active `*sql.Tx` when a transaction
is in flight (production nesting *or* the test harness's outer tx) and the pooled
`*sql.DB` otherwise, the same repo code runs transactionally or not without
knowing which — and savepoint-aware `WithTx` (see `backend/tx.go`) makes nesting
and per-test rollback work uniformly on both engines.
