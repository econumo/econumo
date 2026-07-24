# Design: `data:import-sqlite` — SQLite → PostgreSQL data migration

Date: 2026-07-23
Status: Approved (pending spec review)

## Problem

A self-hoster running Econumo on the default SQLite engine wants to move to
PostgreSQL. There is currently no supported path to carry existing data across.
The two engines have genuinely different column types (Postgres `BOOLEAN`,
`NUMERIC`, `UUID`, `TIMESTAMP` vs SQLite's `TEXT`/`INTEGER` affinity), so a naive
SQL dump does not import cleanly (bare integer `0` will not go into a Postgres
`BOOLEAN` column).

## Chosen approach

A new CLI subcommand under the existing `internal/cli` `resource:action`
convention:

```bash
# DATABASE_URL points at the (fresh) Postgres target
econumo data:import-sqlite /path/to/db.sqlite
```

It reads the source SQLite file and writes the configured Postgres database. A Go
command is chosen over a bash/SQL-text generator because the binary already links
both `modernc.org/sqlite` and `pgx` drivers: it reads real typed rows and writes
them through `pgx`, so type coercion (booleans, decimals, timestamps, UUIDs) is
handled by the drivers instead of fragile string substitution. It also fits the
existing CLI pattern and is testable against the `enginecompare` Postgres
provisioning.

### Rejected alternatives

- **Bash script emitting a `.sql` file** (the original idea): fragile boolean
  handling, needs per-column schema knowledge in `sed`/`awk`, and must reproduce
  the schema + `schema_migrations` by hand. The Go command sidesteps all of it.
- **Data-only command assuming a pre-migrated target**: simpler command, but
  needs a separate "boot the app once / run migrate" step first. Rejected in
  favour of a true one-command flow (see Schema setup).

## Command behaviour

### 1. Guards

- **Target must be Postgres.** If `cfg.DatabaseDriver` is not the Postgres
  engine, abort with a clear message (importing SQLite→SQLite is a foot-gun/no-op).
- **Source must exist.** Open the SQLite path read-only via the registered
  `sqlite` backend (`backend.Get("sqlite").Open(ctx, "sqlite:///<abs-path>")`),
  so the same pragmas (`foreign_keys`, busy timeout) apply. A missing/unreadable
  file aborts with a clear message. The path argument is required; no arg → usage
  error (exit 2), matching the other CLI commands.

### 2. Schema setup — the command runs migrations on the target

Before copying, run the normal migrate runner against the target:
`migrate.Run(ctx, targetDB, pgsqlBackend.Migrations())`. This makes a bare
`createdb`'d Postgres work in a single command: it creates the schema and fully
populates `schema_migrations`, so a later app boot runs nothing. Migrations also
insert their seed rows (the default USD currency `dffc2a06-…`, backfills) — these
are removed and replaced by the source's own rows during the copy (see Overwrite
policy), so no primary-key clash occurs.

### 3. Overwrite policy — abort by default, `--force` to replace

- If the target already holds real data (`SELECT count(*) FROM users > 0`), the
  command **aborts** with a message telling the user to pass `--force`. This
  prevents overwriting a live database.
- With `--force`, the command truncates all app tables it is about to copy
  (`TRUNCATE ... RESTART IDENTITY CASCADE`) inside the copy transaction, then
  copies. The freshly-migrated seed rows (USD currency) are removed by this
  truncate and replaced by the source rows, giving a faithful replica.
- A freshly-migrated empty target (only seed rows, zero users) proceeds without
  `--force`.

### 4. Copy — one transaction, FK-topological order, schema-derived

All copying happens in a single Postgres transaction (all-or-nothing). The
command is schema-driven — it introspects the **target** `information_schema`
rather than hardcoding anything, so it does not drift as migrations evolve:

1. **Table list.** All base tables in the `public` schema, minus the exclusion
   set (below).
2. **Column names + types** per table from `information_schema.columns`. A column
   with `data_type = 'boolean'` marks a boolean column.
3. **FK topological order** from `information_schema` referential constraints:
   parents before children, so inserts satisfy FK constraints without disabling
   them. (The app schema has no table-level FK cycles.)

For each table, in order: `SELECT <cols> FROM <table>` on the SQLite source; for
each row, insert into Postgres with the same column list. Value handling:

- **Boolean columns:** SQLite `0`/`1` (int64) → Go `bool`.
- **Everything else** (TEXT ids/UUIDs, NUMERIC decimals stored as text,
  `"2006-01-02 15:04:05"` timestamps, SMALLINT) passes through as its scanned
  value; Postgres coerces from the text/number exactly. Decimal and timestamp
  fidelity is preserved because the source stores them as canonical text already.
- **NULLs** pass through as `NULL`.

Inserts are batched (prepared statement reused, or `pgx.CopyFrom` as an
optimisation) — dataset sizes for personal finance are modest (thousands of
transactions), so correctness is prioritised over throughput; `CopyFrom` is a
noted optional optimisation, not required for v1.

### 5. Excluded tables

- `messenger_messages` — dead Symfony leftover, unused, and the only `BIGSERIAL`
  table (avoids sequence handling entirely).
- `schema_migrations` — owned by the migrate runner (populated in step 2).
- `migration_versions` — legacy external bookkeeping table; not part of the app
  schema. (It only exists on very old installs; the runner reads it for drop-in
  imports but the app never creates it.)

Everything else is copied, **including**:

- `access_tokens` — sessions and PATs survive the move (no forced re-login).
- `currencies` — the seeded USD row is replaced by the source copy (same id).
- `operation_requests_ids` — transient idempotency cache; copied for completeness
  (harmless).

### 6. Output

- A per-table line: table name → rows copied.
- A final summary: total rows, elapsed time.
- Exit non-zero on any failure; the transaction rolls back so a failed import
  never leaves a partial database.

## Files

- `internal/cli/data_commands.go` (new) — `dataCommands()` returning the
  `data:import-sqlite` command; add `dataCommands()...` to `commandList()` in
  `cli.go`. (The existing `data:remove-salt` command lives in
  `user_commands.go`; leave it there — moving it is out of scope for this change.)
- The copy engine (introspection + topo-sort + row copy) lives in a small
  focused unit — e.g. `internal/cli/sqliteimport/` or an unexported helper in
  `internal/infra/storage/` — so it is unit-testable independent of CLI plumbing.
  Final placement decided in the implementation plan.
- `internal/cli/container.go` — the command needs the raw target `*sql.DB`
  (already on `container.db`) plus the ability to open the source SQLite and
  fetch the pgsql backend's migrations; both via the `backend` registry already
  imported.
- Help text / usage in `cli.go` updated to list the new command.
- `CLAUDE.md` — add `data:import-sqlite <sqlite-path>` to the CLI commands list.

## Testing

- **Integration test** (under the `enginecompare` tag, which provisions
  Postgres): build a source SQLite fixture with `dbtest` + `fixture` (users,
  accounts, transactions covering boolean/decimal/timestamp/NULL columns), run
  the import into a fresh Postgres schema, then assert:
  - per-table row counts match the source;
  - a spot-check of representative rows round-trips byte-identically (a boolean
    `is_deleted`, a `NUMERIC` amount, a timestamp, a NULL FK);
  - the `--force`/abort guard: a second import without `--force` aborts; with
    `--force` it replaces cleanly.
- **Unit test** for the FK topological sort and boolean-column detection against
  a synthetic `information_schema` shape (no DB needed), so the ordering logic is
  covered cheaply.
- No new golden files (this is a CLI/data path, not an HTTP route), so `apiparity`
  is untouched.

## Out of scope

- PostgreSQL → SQLite (reverse direction).
- Live/zero-downtime migration or incremental sync — this is a one-shot,
  offline copy into a fresh target.
- Any schema change to either engine.
