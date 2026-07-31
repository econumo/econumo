# Retire the `users.identifier` column

**Date:** 2026-07-22
**Status:** Approved (design)
**Follow-on:** enables the simplified "change email" feature (separate spec).

## Problem

The `users` table carries an `identifier` column — `hex(md5(lower(email)))` — that
serves as the unique constraint and the primary lookup key for authentication
(login, register dup-check, password reset, email verification, admin
change-email).

That column only exists because emails *used to* be AES-encrypted with a random
IV, so the ciphertext was neither uniquely indexable nor directly searchable. The
deterministic `identifier` hash was the stand-in.

The app now runs **salt-free with plaintext emails** (`ECONUMO_DATA_SALT` is
deprecated/ignored; `data:remove-salt` decrypts existing data to plaintext). With
plaintext emails, a unique index on `lower(email)` plus direct email lookups fully
replace `identifier`. The column's derivation is redundant.

## Goal

Make `lower(email)` the unique + lookup key. Stop deriving `identifier` from email
anywhere in the code, and remove `User.Identifier` from the domain model.

**Keep the physical column** (populated with the row's own `id` as a unique,
non-null placeholder). We do NOT drop it, because SQLite cannot drop a
`NOT NULL UNIQUE` column without a full `users` table rebuild, and migrations here
run in a single transaction under `foreign_keys = ON` (which can't be toggled
mid-transaction) — a bespoke rebuild of the FK-heavy `users` parent is the one
delicate, data-loss-capable step we choose to avoid. Retiring the column in place
achieves the functional goal at near-zero migration risk on both engines.

Keep the observable wire contract byte-identical.

Non-goals: dropping the physical column; the change-email feature (separate spec);
any change to email encryption / `Encode` / `Decode`; any change to how other
tables reference `users.id`.

## Key facts (from codebase investigation)

- `identifier` is **never on the wire**: no `json:"identifier"` tag in any DTO;
  `model.User.Identifier` has no JSON tag. REST/MCP goldens (`apiparity`,
  `mcpparity`) must not change.
- `identifier` is **not a foreign-key target**: every FK to users targets
  `users(id)`.
- Only **two repo methods** read it: `GetByIdentifier`, `ExistsByIdentifier`
  (`internal/user/repo/repo.go:123,134`). All other use is the
  `s.encode.Hash(lower(email))` derivation (login, register, admin, password,
  verify-email).
- The stored `email` column holds ciphertext/plaintext of the **original-case**
  (trimmed) email. Lowercasing happens today only at the `Hash` call sites.
  Therefore the replacement uniqueness must be an **expression index on
  `lower(email)`**, supported by both engines (SQLite ≥ 3.9, PostgreSQL).
- The `identifier` column is `NOT NULL` and `UNIQUE` (both a table constraint,
  baseline `20260101000000.sql:71`, and a named index `UNIQ_1483A5E9772E836A`,
  `:435`). A repeated constant would violate the constraint; the placeholder must
  stay unique → use each row's `id`.
- Migrations run one-file-per-transaction with `foreign_keys = ON`
  (`internal/infra/storage/migrate/migrate.go:178` `applyMigration`).

## Design

### 1. Schema migration (new dated file `20260722000000.sql`, both engines)

Append-only. Two statements, identical intent on both engines:

```sql
CREATE UNIQUE INDEX users_email_lower_uniq ON users (lower(email));
UPDATE users SET identifier = id;
```

- The expression unique index makes `lower(email)` the enforced key.
- The `UPDATE` retires the email-derived values (sets each to the row's own unique
  id). The pre-existing `NOT NULL UNIQUE` constraint on `identifier` is left in
  place; `id` satisfies both. No column drop, no table rebuild → safe on both
  engines.
- **Precondition (documented):** requires plaintext emails. A still-salted
  instance is *already* login-broken by design; after `data:remove-salt` the
  emails are plaintext and the `lower(email)` index/lookups work. If two existing
  rows share a `lower(email)` (shouldn't happen — the old identifier uniqueness
  prevented it), index creation fails loudly rather than silently.

### 2. Queries / repository (regenerate sqlc)

`internal/infra/storage/sqlc/query/{sqlite,pgsql}/users.sql`:

- Add `GetUserByEmail` / `ExistsUserByEmail` — `WHERE lower(email) = lower(?)`
  (`lower($1)` for pgsql).
- Remove `GetUserByIdentifier` / `ExistsUserByIdentifier`.
- Drop `identifier` from the `GetUserByID` / `GetUserByEmail` SELECT column lists
  (the column stays in the table; it is simply no longer read into the domain).
- Keep `identifier` in `InsertUser` / `UpsertUser` (the column is still
  `NOT NULL`); the repo supplies the row `id` as its value.

`internal/user/repo/`:

- `repo.go`: `GetByIdentifier`/`ExistsByIdentifier` → `GetByEmail(ctx, email)` /
  `ExistsByEmail(ctx, email)`. In `Save`, set the upsert param
  `Identifier: u.ID.String()`. Drop `Identifier` from `userRow` and from
  `hydrate`. Update the `querier` interface + `sqlite.go`/`pgsql.go` adapters.
- `internal/user/repository.go`: interface method rename.

### 3. Service / model

`internal/model/user.go`:

- Remove the `Identifier` field.
- `NewUser(...)` loses the `identifier` argument.
- `UpdateEmail(email, now)` loses the `identifier` argument (was
  `UpdateEmail(identifier, encryptedEmail, now)`).

Service call sites — replace `s.encode.Hash(lower(email))` + `GetByIdentifier`
with `GetByEmail(email)` (the query lowercases; pass the trimmed email):

- `internal/user/login.go:24-25`
- `internal/user/register.go:49-52` — dup-check via `ExistsByEmail`
- `internal/user/admin.go:158-160` — `userByEmail`
- `internal/user/admin.go:37-55` — `AdminChangeEmail`: drop new-identifier
  derivation; uniqueness via `ExistsByEmail(newEmail)`; `UpdateEmail(newEmail, now)`
- `internal/user/password.go:88,127`
- `internal/user/verify_email.go:47,131`

**Storage casing:** keep storing trimmed **original-case** email; the
`lower(email)` index owns uniqueness.

**`EncodeService.Hash`:** becomes dead code — remove the method and its golden
vectors (`internal/infra/auth/crypto_golden_test.go`). `Encode`/`Decode` remain.

### 4. `data:remove-salt` migration (`internal/user/migrate.go`)

Keep the command. Drop the `saltFree.Hash(...)` identifier re-derivation; still
decrypt each `email` ciphertext → plaintext and persist with
`UpdateEmail(plain, now)`. The repo writes `id` into the `identifier` column
automatically on save.

## Wire contract

Unchanged. `identifier` was never serialized; login/register/reset/verify behavior
is byte-identical. `apiparity` and `mcpparity` goldens must not change.

## Testing

- Update every identifier-asserting test to email-based lookups /
  no-Identifier-field:
  `internal/test/fixture/entities.go:59`, `internal/user/migrate_test.go`,
  `internal/user/admin_integration_test.go`,
  `internal/user/trial_integration_test.go`,
  `internal/user/avatar_update_integration_test.go`,
  `internal/user/verify_email_test.go`,
  `internal/user/repo/repo_integration_test.go`
  (`GetByIdentifier`/`ExistsByIdentifier` → email variants),
  `internal/model/user_test.go`, `internal/model/user_activate_test.go`,
  `internal/infra/auth/crypto_golden_test.go` (drop Hash vectors),
  `internal/infra/storage/migrations/email_verified_backfill_test.go`.
- `make go-test` (smoke: build, vet, gofmt, sqlite unit/integration, apiparity +
  mcpparity goldens unchanged, i18n guards, coverage gate).
- `make test-repo-pgsql` + the `enginecompare` suite — `lower(email)` lookups and
  the expression index must behave identically on SQLite and PostgreSQL.

## Docs

- CLAUDE.md: the "Wire & data contract" identifier notes, the `data:remove-salt`
  description, the "Salt-free everywhere" / user-identifier references (note the
  column is retained as a vestigial `= id` placeholder).
- `internal/infra/storage/sqlc/sqlc.yaml` comment mentioning the Identifier VO
  mapping; `internal/server/server.go:114`; `internal/config/config.go:24`.
- Upgrade note: `data:remove-salt` prerequisite for salted instances.

## Accepted edges

- The `identifier` column is retained (vestigial, `= id`) rather than dropped, to
  avoid a SQLite table rebuild. A future dedicated migration can drop it if ever
  desired.
- Non-ASCII email casing: Go's Unicode `strings.ToLower` (old identifier input) vs
  SQL `lower()` (ASCII in SQLite) can differ for non-ASCII local parts. Emails are
  ASCII in practice — accepted.

## Follow-on: change-email (separate spec)

Once this ships, the change-email feature simplifies: request/confirm uniqueness
is a plain `ExistsByEmail` check and no identifier derivation is involved.
