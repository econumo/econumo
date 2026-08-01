# Hidden Currencies + Migration Consolidation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `users_hidden_currencies` replaces `is_archived`; reads lose their USD fallbacks behind a migration-established invariant; the branch's four currency migrations squash into one final-shape `20260730000000` made safe by runner-hoisted `PRAGMA foreign_keys` statements — per `docs/superpowers/specs/2026-08-01-hidden-currencies-consolidation-design.md`.

**Architecture:** The runner detects `PRAGMA foreign_keys` statements, runs them on a pinned `sql.Conn` outside the migration transaction, and asserts `PRAGMA foreign_key_check` INSIDE the transaction (violations roll everything back). The squashed migration rebuilds only `currencies` to its final shape (no `is_archived`, ever), creates `users_hidden_currencies`, and rewrites the profile option to ids with USD-normalization + missing-row seeding. Archive endpoints/UI die; hide extends to own customs; strict reads error on violated invariants.

**Tech Stack:** Go stdlib + sqlc v1.30.0, apiparity, vitest.

## Global Constraints

- Wire: `isArchived` leaves the currency list item (pre-release); `isHidden` is the one flag, computed for own rows too (shared rows stay 0). Frozen strings: `"This currency cannot be hidden"` (now also the own-default refusal), `"The base currency cannot be modified"`. The `currency.cannot_archive` code/message/locales (added earlier today) are retired.
- Route guard floor: 101 → 99 (archive + unarchive removed; commented).
- Hidden is a pure preference: `EnsureUsable` = "global, or the caller's own custom" — no hidden check.
- Strict reads: absent/dangling profile-currency values are ERRORS, not USD. The invariant is the migration's job.
- sqlc regen pinned v1.30.0; `make swagger` after handler deletions; gofmt + conventional commits per task.
- Demo/beta DBs are hand-realigned once (Task 6); no released DB exists.

---

### Task 1: Runner — hoisted `PRAGMA foreign_keys` + in-tx FK check

**Files:** `internal/infra/storage/migrate/migrate.go`, `internal/infra/storage/migrate/migrate_test.go` (extend)

**Interfaces (produced):** unchanged public API. Behavior: `applyMigration` splits statements; `PRAGMA foreign_keys` statements (case-insensitive prefix match) are hoisted: OFF → executed on a dedicated `db.Conn(ctx)` BEFORE `BeginTx`; all non-pragma statements run in the tx; when an OFF was hoisted, `PRAGMA foreign_key_check` runs as a QUERY inside the tx and any returned row fails (and rolls back) the migration; the version INSERT stays inside the tx; after commit (or rollback) the conn executes `PRAGMA foreign_keys = ON` (deferred — the connection returns to the pool re-enabled no matter what) and closes. Migrations without pragmas keep today's `db.BeginTx` path byte-for-byte.

- [ ] **Step 1 (RED):** runner tests: (a) a two-table parent/child schema + a pragma-wrapped migration that renames/recreates the parent — children survive and `foreign_key_check` is clean (without hoisting this cascade-deletes, which is the pre-validated failure); (b) a pragma-wrapped migration that leaves a dangling child FK — `Run` returns an error mentioning `foreign_key_check` AND the version is NOT recorded AND the migration's changes rolled back; (c) a pragma-less migration behaves exactly as before (existing tests keep passing).
- [ ] **Step 2 (GREEN):** implement hoisting in `applyMigration` (signature gains nothing; internally branch on `hasFKPragma(stmts)`); pgsql unaffected (its files never contain the pragma — the detection is engine-neutral but inert).
- [ ] **Step 3:** `go test ./internal/infra/storage/migrate/ -count=1` + full `./internal/...`; commit `feat(migrate): hoist PRAGMA foreign_keys around the migration transaction`.

---

### Task 2: The squashed migration `20260730000000`

**Files:**
- Rewrite: `internal/infra/storage/migrations/sqlite/20260730000000.sql`, `internal/infra/storage/migrations/pgsql/20260730000000.sql`
- Delete: `{sqlite,pgsql}/20260801000000.sql`, `{sqlite,pgsql}/20260801100000.sql`, `migration_20260730_test.go`, `migration_20260801_test.go`, `migration_20260801100000_test.go`
- Create: `internal/infra/storage/migrations/migration_20260730_test.go` (consolidated)

sqlite content (exact):

```sql
-- Per-user custom currencies, in one step to the final shape: currencies
-- gains user_id (NULL = global) and rate (the custom's fixed rate);
-- UNIQUE(code) is replaced by two partial unique indexes; new table
-- users_hidden_currencies stores per-user hidden currencies; the profile
-- currency option becomes a currency ID with a USD-normalized invariant
-- (every user ends up holding a live currency id).
--
-- SQLite cannot drop a table-level UNIQUE, so currencies is rebuilt — a
-- SINGLE-table rebuild: the runner hoists the pragmas below outside its
-- transaction, so renaming does not rewrite children's FK clauses, and the
-- runner's in-transaction foreign_key_check guards the result.
PRAGMA foreign_keys = OFF;

CREATE TABLE currencies_new
(
    id         TEXT    NOT NULL
    , code       TEXT     NOT NULL
    , symbol     VARCHAR(12) NOT NULL
    , created_at DATETIME    NOT NULL
    , name VARCHAR(64) DEFAULT NULL
    , fraction_digits SMALLINT DEFAULT '2' NOT NULL
    , user_id TEXT DEFAULT NULL
    , rate NUMERIC(19, 8) DEFAULT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
INSERT INTO currencies_new (id, code, symbol, created_at, name, fraction_digits)
SELECT id, code, symbol, created_at, name, fraction_digits FROM currencies;
DROP TABLE currencies;
ALTER TABLE currencies_new RENAME TO currencies;

CREATE UNIQUE INDEX UNIQ_currencies_code_global ON currencies (code) WHERE user_id IS NULL;
CREATE UNIQUE INDEX UNIQ_currencies_user_code ON currencies (user_id, code) WHERE user_id IS NOT NULL;
CREATE INDEX IDX_currencies_user_id ON currencies (user_id);

CREATE TABLE users_hidden_currencies
(
    user_id     TEXT NOT NULL
    , currency_id TEXT NOT NULL
    , created_at  DATETIME NOT NULL
    , PRIMARY KEY (user_id, currency_id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE CASCADE
);
CREATE INDEX IDX_users_hidden_currencies_currency_id ON users_hidden_currencies (currency_id);

-- Profile currency option: code -> id. Values already matching an id stay;
-- resolvable codes map own-first then global; anything else normalizes to
-- the global USD row (baseline-seeded, undeletable) so reads never need a
-- fallback. Users without the option row get one, same invariant.
UPDATE users_options SET value = COALESCE(
    (SELECT c.id FROM currencies c WHERE c.code = users_options.value AND c.user_id = users_options.user_id),
    (SELECT c.id FROM currencies c WHERE c.code = users_options.value AND c.user_id IS NULL),
    (SELECT c.id FROM currencies c WHERE c.code = 'USD' AND c.user_id IS NULL)
)
WHERE name = 'currency'
  AND NOT EXISTS (SELECT 1 FROM currencies c2 WHERE c2.id = users_options.value);
INSERT INTO users_options (id, user_id, name, value, created_at, updated_at)
SELECT lower(hex(randomblob(16))), u.id, 'currency',
       (SELECT c.id FROM currencies c WHERE c.code = 'USD' AND c.user_id IS NULL),
       CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM users u
WHERE NOT EXISTS (SELECT 1 FROM users_options o WHERE o.user_id = u.id AND o.name = 'currency');

PRAGMA foreign_keys = ON;
```

pgsql content: `ALTER TABLE currencies ADD COLUMN user_id UUID DEFAULT NULL` + FK constraint, `ADD COLUMN rate NUMERIC(19, 8) DEFAULT NULL`, `ALTER COLUMN name TYPE VARCHAR(64)`, `DROP CONSTRAINT currencies_code_key`, `DROP INDEX UNIQ_37C4469377153098`, both partial uniques + `IDX_currencies_user_id`, the hidden table + index (UUID/TIMESTAMP(0) types), the option UPDATE with `::text` casts + the USD COALESCE arm, and the missing-row INSERT using `gen_random_uuid()::text`. No pragmas.

- [ ] **Step 1 (RED):** consolidated `migration_20260730_test.go` (keeps `openFK`/`toRunList` helpers — they lived in the deleted file; move them here): rebuild preserves seeded accounts/transactions/rates rows + clean `foreign_key_check`; partial-unique matrix (same custom code two owners OK, dup per owner rejected, dup global rejected); user-delete cascades to customs + hidden rows; option matrix (own-first / global / already-id / unresolvable→USD id / missing-row seeded with USD id).
- [ ] **Step 2:** delete the four old migration files + three tests; write the two new files; GREEN; `go test ./internal/... -count=1` (fixtures unchanged EXCEPT `fixture.Currency.IsArchived` still exists until Task 3 — the fixture INSERT references `is_archived`, which no longer exists → **Tasks 2+3 must land as ONE commit**; proceed directly).

---

### Task 3: Backend — archive out, hide extends to own customs, strict reads

**Files (deletions/edits driven by the compiler after regen):**
- sqlc: drop `SetCurrencyArchived`; remove `is_archived` from `GetCurrencyRecord`/`InsertUserCurrency`/`GetUserCurrencyListView` in BOTH engines; regenerate.
- `internal/model`: `CurrencyRecord`/`CurrencyViewRow` lose `IsArchived`; `CurrencyListItem` loses the `IsArchived` wire field; DTOs `Archive*/Unarchive*` deleted.
- `internal/currency`: `manage_lifecycle.go` archive/unarchive use cases + `setArchived` + the default-guard block deleted; `EnsureUsable` (repo/lookup.go) drops the archived branch; `visibility.go` HideCurrency allows `*rec.UserID == caller` (foreign stays refused; base + profile-default guards unchanged); read service computes `isHidden` for own rows (hiddenSet applies to scope own too).
- `internal/currency/api`: archive handlers + routes deleted; `make swagger`.
- `internal/shared/errs` + locales: retire `CodeCurrencyCannotArchive` / `errors.currency.cannot_archive` (both catalogues).
- `internal/test/fixture`: `Currency.IsArchived` field + insert column go.
- apiparity: guard floor 99; scenario rework — archive calls and the `set-default-pts`/`err:archive-default`/`set-default-usd` trio replaced by: `hide-own-currency` (happy, previously uncoverable), `show-own-currency`, `err:hide-foreign` (guest hides owner's custom → 400), `set-default-pts` + `err:hide-default` (own custom as default → frozen hidden message) + `set-default-usd`; `read-after-hide` asserting `isHidden:1` on the own row.
- Strict reads: `internal/user/read.go` + `usecase.go` — absent/dangling → `fmt.Errorf("user %s: currency option violates the id invariant", ...)`-style internal error (500), USD fallback code paths deleted; `register.go` propagates the seed-resolution error; glue `CurrencyID` + budget `DefaultCurrencyID` error on absent; `budget/create.go` drops the `model.DefaultCurrency` fallback branch.
- Tests: currency manage/api/read tests (archive tests deleted, hide-own tests added incl. default-guard + foreign refusal), strict-read tests (dangling id errors), user tests updated, `manage_test.go` fakes lose archived bits.

- [ ] TDD where behavior is NEW (hide-own allowed + guards; strict-read errors): write those failing tests first, then the mechanical deletions, then GREEN across `./internal/...` with `UPDATE_GOLDEN=1` for apiparity/mcpparity (inspect: isArchived gone, hide-own steps added, archive steps gone) and enginecompare on PostgreSQL.
- [ ] Commit (Tasks 2+3): `feat(currency)!: users_hidden_currencies replaces archival; squashed final-shape migration; strict currency reads`.

---

### Task 4: Frontend

**Files:** `web/src/api/dto/currency.ts` (drop `isArchived`), `web/src/api/currency.ts` + `queries.ts` (drop archive fns/hooks), `web/src/features/currencies/CurrenciesPage.tsx` (own rows get the SAME hide/show switch — locked when the row is the profile default, reusing `locked_profile`; archived caption/greyed handling for currencies gone), `selectable.ts` (own filter by `isHidden`), tests reworked (unified switch, no archive), locales: retire currency archive strings IF now unused (grep `t()` first).

- [ ] RED (rework page tests: own-row switch posts hide/show; own default row disabled; no archive requests) → GREEN → `pnpm test` + `pnpm lint`; commit `feat(web): one visible/hidden switch for every currency row`.

---

### Task 5: Full verification

- [ ] `make test` (smoke + engines + web); goldens only intentionally changed; coverage ≥ 80.

### Task 6: Demo/beta realignment + browser check

- [ ] demo.sqlite surgical alignment (schema already matches except `is_archived`): `ALTER TABLE currencies DROP COLUMN is_archived;` + `DELETE FROM schema_migrations WHERE version IN ('20260801000000','20260801100000');` (any archived customs would first copy to hidden rows — verify count is 0). Boot the rebuilt binary, verify: currencies page shows the unified switch on PTS; hiding PTS while it is NOT the default works and removes it from the transfer picker; hiding it while default is refused. Revert all demo state; revoke the session.
- [ ] Commit anything outstanding. (Superseded plan/spec files stay as-is — they are historical records.)
