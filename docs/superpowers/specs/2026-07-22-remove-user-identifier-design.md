# Remove the `users.identifier` column

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
replace `identifier`. The column is redundant.

## Goal

Delete `users.identifier`. Make `lower(email)` the unique + lookup key. Keep the
observable wire contract byte-identical.

Non-goals: the change-email feature itself (separate spec), any change to email
encryption/`Encode`/`Decode`, any change to how other tables reference `users.id`.

## Key facts (from codebase investigation)

- `identifier` is **never on the wire**: no `json:"identifier"` tag exists in any
  DTO/response; `model.User.Identifier` has no JSON tag. So REST/MCP goldens
  (`apiparity`, `mcpparity`) must not change.
- `identifier` is **not a foreign-key target**: every FK to users targets
  `users(id)`. `identifier` is only a standalone `UNIQUE` column/index on `users`.
- Only **two repo methods** read it: `GetByIdentifier`, `ExistsByIdentifier`
  (`internal/user/repo/repo.go:123,134`). All other use is the
  `s.encode.Hash(lower(email))` derivation at the call sites below.
- The stored `email` column holds ciphertext/plaintext of the **original-case**
  (trimmed) email. Lowercasing happens today only at the `Hash` call sites, never
  before storage. Therefore the replacement uniqueness must be an **expression
  index on `lower(email)`**, not a plain unique index on the column.
- Expression indexes are supported by both engines (SQLite ≥ 3.9, PostgreSQL).

## Design

### 1. Schema migration (new dated file, both engines)

Append-only — do **not** edit historical migrations (they have run in production;
new installs replay old→new in order).

For `internal/infra/storage/migrations/{sqlite,pgsql}`:

1. `CREATE UNIQUE INDEX <name> ON users (lower(email));`
2. Drop the identifier unique index (`UNIQ_1483A5E9772E836A`) / `UNIQUE (identifier)`
   constraint and the `identifier` column.

**Migration-ordering caveat (must be documented in the upgrade note / CLAUDE.md):**
the `lower(email)` scheme requires *plaintext* emails. An instance still holding
salted/encrypted emails is *already* login-broken by design (mismatched
identifiers), so this is no regression — but such an instance must run
`data:remove-salt` to log in after upgrading. The boot migration itself does not
depend on the salt state (it does not read `identifier`).

### 2. Queries / repository (regenerate sqlc)

`internal/infra/storage/sqlc/query/{sqlite,pgsql}/users.sql`:

- `GetUserByIdentifier` → `GetUserByEmail` — `WHERE lower(email) = lower(?)`
  (`$1` for pgsql).
- `ExistsUserByIdentifier` → `ExistsUserByEmail` — same predicate.
- Remove `identifier` from `CreateUser` and `SyncUser` (insert columns + upsert
  `identifier = excluded.identifier` clause).
- Regenerate `sqlc/gen/{sqlite,pgsql}` + `querier.go`.

`internal/user/repo/`:

- `repo.go`: `GetByIdentifier`/`ExistsByIdentifier` → `GetByEmail(ctx, email)` /
  `ExistsByEmail(ctx, email)`; drop `Identifier` from `Save` params and `toModel`
  scan; drop the field from the row struct.
- `sqlite.go` / `pgsql.go` adapters: rename to `GetUserByEmail` /
  `ExistsUserByEmail`; interface in `repo.go` updated.
- `internal/user/repository.go`: interface method rename.

### 3. Service / model

`internal/model/user.go`:

- Remove the `Identifier` field.
- `NewUser(...)` loses the `identifier` argument.
- `UpdateEmail(email, now)` loses the `identifier` argument (was
  `UpdateEmail(identifier, encryptedEmail, now)`).

Service call sites — replace `s.encode.Hash(lower(email))` + `GetByIdentifier`
with `GetByEmail(email)` (the query lowercases; pass the raw trimmed email):

- `internal/user/login.go:24-25` — primary auth lookup.
- `internal/user/register.go:50-52` — dup-check via `ExistsByEmail`.
- `internal/user/admin.go:159-160` — `userByEmail` helper.
- `internal/user/admin.go:37-55` — `AdminChangeEmail`: drop the new-identifier
  derivation; uniqueness via `ExistsByEmail(newEmail)`; `UpdateEmail(newEmail, now)`.
- `internal/user/password.go:88,127` — reset/change flows.
- `internal/user/verify_email.go:47,131` — verification lookups.

**Storage casing:** keep storing the trimmed **original-case** email (preserves the
`get-user-data` display value). The `lower(email)` expression index owns
uniqueness, so stored case does not matter for correctness.

**`EncodeService.Hash`:** becomes dead code (nothing references it after this
change) — remove the method and its golden vectors in
`internal/infra/auth/crypto_golden_test.go`. `Encode`/`Decode` remain (used by
`data:remove-salt`).

### 4. `data:remove-salt` migration (`internal/user/migrate.go`)

Keep the CLI command. Update `MigrateRemoveDataSalt`:

- Drop the `saltFree.Hash(...)` identifier re-derivation.
- Still decrypt each `email` ciphertext → plaintext (via the salted encoder) and
  persist with `UpdateEmail(plain, now)`.

Its role — produce plaintext emails — is now precisely what the `lower(email)`
index depends on.

## Wire contract

Unchanged. `identifier` was never serialized; login/register/reset/verify behavior
is byte-identical. `apiparity` and `mcpparity` goldens must not change — if they
do, something regressed.

## Testing

- Update every identifier-asserting test to email-based lookups:
  `internal/test/fixture/entities.go:59` (fixture builder),
  `internal/user/migrate_test.go`, `internal/user/admin_integration_test.go`,
  `internal/user/trial_integration_test.go`,
  `internal/user/avatar_update_integration_test.go`,
  `internal/user/verify_email_test.go`,
  `internal/user/repo/repo_integration_test.go`
  (`TestUserRepo_GetByIdentifier`/`ExistsByIdentifier` → email variants),
  `internal/model/user_test.go`, `internal/model/user_activate_test.go`,
  `internal/infra/auth/crypto_golden_test.go` (drop Hash vectors),
  `internal/infra/storage/migrations/email_verified_backfill_test.go` (raw
  `INSERT INTO users (... identifier ...)`).
- `make go-test` (smoke: build, vet, gofmt, sqlite unit/integration, apiparity +
  mcpparity goldens unchanged, i18n guards, coverage gate).
- `make test-repo-pgsql` + the `enginecompare` suite — the expression index and
  `lower(email)` lookups must behave identically on SQLite and PostgreSQL.

## Docs

- CLAUDE.md: the "Wire & data contract" identifier notes, the `data:remove-salt`
  description, and the "Salt-free everywhere" / user-identifier references.
- Explanatory comments at `internal/server/server.go:114` and
  `internal/config/config.go:24`.
- Upgrade note documenting the `data:remove-salt` prerequisite for salted
  instances.

## Accepted edge

Non-ASCII email casing: Go's Unicode `strings.ToLower` (old identifier input) vs
SQL `lower()` (ASCII in SQLite) can differ for non-ASCII local parts. Emails are
ASCII in practice — accepted.

## Follow-on: change-email (separate spec)

Once this ships, the change-email feature simplifies: its pending-change table
needs no `new_identifier` column, and request/confirm uniqueness is a plain
`ExistsByEmail` check.
