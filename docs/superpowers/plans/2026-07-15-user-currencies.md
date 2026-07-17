# User-Managed Currencies & Rates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Per-user custom currencies with owner-set dated rates, per-user show/hide of global currencies in dropdowns, and a Settings → Currencies page — per the approved spec `docs/superpowers/specs/2026-07-14-user-currencies-design.md`.

**Architecture:** Extend the `currencies` table with a nullable owner (`user_id`) and `is_archived`; replace `UNIQUE(code)` with two partial unique indexes; add `users_hidden_currencies`. New `ManageService` in `internal/currency` drives 8 new POST endpoints; the read side becomes per-user. Account/budget writes gain a "currency usable by caller" check. Frontend adds a Currencies settings page and filters all currency pickers.

**Tech Stack:** Go stdlib + sqlc (sqlite/pgsql engine-adapter pattern), React 19 + TanStack Query + vitest/msw.

## Global Constraints

- Work in `/home/dmitry/dev/econumo/econumo/.claude/worktrees/curious-cuddling-music` on branch `feat/user-currencies`. Every subagent's FIRST command: `cd /home/dmitry/dev/econumo/econumo/.claude/worktrees/curious-cuddling-music && git branch --show-current` (must print `feat/user-currencies`).
- sqlc `.sql` comments must be ASCII-only — NO em dashes or any multi-byte characters (sqlc v1.30 sqlite codegen corrupts queries; see the NOTE in `internal/infra/storage/sqlc/query/sqlite/currency_write.sql`).
- Frozen wire strings introduced by this feature (exact, asserted by tests):
  - `"CurrencyCode is incorrect"` (field `code`), `"Currency already exists"` (field `code`), `"Currency name must be 1-64 characters"` (field `name`), `"Currency symbol must be 1-12 characters"` (field `symbol`), `"Fraction digits must be between 0 and 8"` (field `fractionDigits`), `"Rate must be a positive number"` (field `rate`), `"Date is not valid"` (field `date`), `"Currency is not available"` (field `currencyId`).
  - Fieldless (message-level): `"Currency is in use and cannot be deleted"`, `"The base currency cannot be modified"`, `"This currency cannot be hidden"`, and the existing `"Operation is locked"`.
  - Ownership failures: `errs.NewAccessDenied("")` (empty message, HTTP 403) — same as payee/category.
- New wire fields on `CurrencyResult` (additive): `"scope"` (`"global"|"own"|"shared"`), `"isArchived"` (int 0/1), `"isHidden"` (int 0/1).
- New routes (all under auth): `POST /api/v1/currency/{create-currency,update-currency,archive-currency,unarchive-currency,delete-currency,set-currency-rate,hide-currency,show-currency}`.
- **Spec deviation (intentional):** `create-currency` uses the house convention — request field `id` is the client-generated operation id (idempotency key, required); the entity gets a fresh server UUIDv7. The spec's `operationId?` is superseded.
- Datetimes on the wire: `datetime.Layout` = `"2006-01-02 15:04:05"`; dates: `datetime.DateLayout` = `"2006-01-02"`.
- Rate storage direction: `rate` = units of the currency per 1 unit of the instance base currency (`cfg.CurrencyBase`, default `USD`).
- Commit after every task with a conventional-commits message; run `gofmt` before committing Go code.
- Seeded USD currency id in tests/goldens: `dffc2a06-6f29-4704-8575-31709adee926` (global, from the baseline migration).

## Pre-validated technical decisions (do NOT re-litigate)

These were probed empirically before planning:

1. **SQLite migration MUST use the full FK-closure rebuild** (rename → recreate → copy → drop, exactly like `internal/infra/storage/migrations/sqlite/20260101000000.sql`). A single-table rebuild with `PRAGMA legacy_alter_table` was probed and **cascade-deletes child rows** (the pragma is a no-op inside the migration transaction; the rename rewrites children's FK clauses to `currencies__old`, and the drop's implicit DELETE cascades). The closure rebuild pattern was probed and preserves all rows with clean `PRAGMA foreign_key_check`.
2. **sqlc v1.30 parses everything needed**: partial unique indexes (`CREATE UNIQUE INDEX ... WHERE user_id IS NULL`), the rename/rebuild migration, `ALTER TABLE ... DROP CONSTRAINT`, and the new table. Generated `Currency` struct is byte-identical across engines: `UserID *string`, `IsArchived bool` — so whole-struct pgsql shim casts keep working.
3. Partial-index semantics verified: same custom code for two users OK; duplicate `(user_id, code)` rejected; duplicate global code rejected; `ON DELETE CASCADE` from `users` to custom currencies works.

---

### Task 1: Database migration (both engines) + fixture support

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260715000000.sql`
- Create: `internal/infra/storage/migrations/pgsql/20260715000000.sql`
- Create: `internal/infra/storage/migrations/migration_20260715_test.go`
- Modify: `internal/test/fixture/entities.go` (Currency struct + new HiddenCurrency helper)

**Interfaces:**
- Consumes: existing schema (see `sqlite/20260101000000.sql` for closure-table DDL).
- Produces: `currencies` with `user_id TEXT/UUID NULL` (FK → users, CASCADE) and `is_archived BOOLEAN NOT NULL DEFAULT 0`; partial unique indexes `UNIQ_currencies_code_global` / `UNIQ_currencies_user_code`; index `IDX_currencies_user_id`; table `users_hidden_currencies(user_id, currency_id, created_at, PK(user_id, currency_id))` with index `IDX_users_hidden_currencies_currency_id`. Fixture: `fixture.Currency{..., UserID string, IsArchived bool}`, `Builder.HiddenCurrency(userID, currencyID string)`.

- [ ] **Step 1: Write the failing migration test**

Create `internal/infra/storage/migrations/migration_20260715_test.go`. It applies all migrations EXCEPT the new one to a fresh FK-ON sqlite DB, seeds a user/currency/account/transaction/rate/budget/element chain, then applies the new migration and asserts data survival, FK integrity, and the new uniqueness rules:

```go
package migrations_test

// Verifies the 20260715000000 sqlite migration: the currencies FK-closure
// rebuild must preserve data under PRAGMA foreign_keys = ON (a naive rebuild
// cascade-deletes children), and the new partial unique indexes must enforce
// per-owner code uniqueness.

import (
	"context"
	"database/sql"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	_ "modernc.org/sqlite"
)

const newVersion = "20260715000000"

func openFK(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.ExecContext(context.Background(), "PRAGMA foreign_keys = ON;"); err != nil {
		t.Fatal(err)
	}
	return db
}

func toRunList(src []migrations.Migration) []migrate.Migration {
	out := make([]migrate.Migration, 0, len(src))
	for _, m := range src {
		out = append(out, migrate.Migration{Version: m.Version, SQL: m.SQL})
	}
	return out
}

func TestMigration20260715_DataSurvivesRebuild(t *testing.T) {
	db := openFK(t)
	ctx := context.Background()
	all := toRunList(migrations.SQLite())
	var before, target []migrate.Migration
	for _, m := range all {
		if m.Version == newVersion {
			target = append(target, m)
			continue
		}
		if m.Version < newVersion {
			before = append(before, m)
		}
	}
	if len(target) != 1 {
		t.Fatalf("migration %s not found in embed", newVersion)
	}
	if err := migrate.Run(ctx, db, before); err != nil {
		t.Fatalf("pre-migrations: %v", err)
	}
	seed := []string{
		`INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
		 VALUES ('u1', 'ident1', 'a@b.c', 'A', 'face:sky', 'x', 's', 'argon2id', '2026-01-01 00:00:00', '2026-01-01 00:00:00', 1)`,
		`INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at)
		 VALUES ('a1', 'dffc2a06-6f29-4704-8575-31709adee926', 'u1', 'Cash', 2, 'wallet', 0, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`,
		`INSERT INTO transactions (id, user_id, account_id, amount, type, spent_at, created_at, updated_at)
		 VALUES ('t1', 'u1', 'a1', '10.00000000', 0, '2026-01-02 00:00:00', '2026-01-02 00:00:00', '2026-01-02 00:00:00')`,
		`INSERT INTO currencies_rates (id, currency_id, base_currency_id, published_at, rate)
		 VALUES ('r1', 'dffc2a06-6f29-4704-8575-31709adee926', 'dffc2a06-6f29-4704-8575-31709adee926', '2026-01-01', '1.00000000')`,
		`INSERT INTO budgets (id, currency_id, user_id, name, started_at, created_at, updated_at)
		 VALUES ('b1', 'dffc2a06-6f29-4704-8575-31709adee926', 'u1', 'Budget', '2026-01-01 00:00:00', '2026-01-01 00:00:00', '2026-01-01 00:00:00')`,
		`INSERT INTO budgets_elements (id, budget_id, currency_id, external_id, type, created_at, updated_at, position)
		 VALUES ('be1', 'b1', 'dffc2a06-6f29-4704-8575-31709adee926', 'x1', 1, '2026-01-01 00:00:00', '2026-01-01 00:00:00', 0)`,
	}
	for _, s := range seed {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("seed: %v\n%s", err, s)
		}
	}
	if err := migrate.Run(ctx, db, all); err != nil {
		t.Fatalf("target migration: %v", err)
	}
	for table, want := range map[string]int{
		"accounts": 1, "transactions": 1, "currencies_rates": 1,
		"budgets": 1, "budgets_elements": 1,
	} {
		var n int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != want {
			t.Errorf("%s rows = %d, want %d (rebuild lost data)", table, n, want)
		}
	}
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign_key_check reported violations after migration")
	}
}

func TestMigration20260715_PartialUniqueAndNewTable(t *testing.T) {
	db := openFK(t)
	ctx := context.Background()
	if err := migrate.Run(ctx, db, toRunList(migrations.SQLite())); err != nil {
		t.Fatal(err)
	}
	mustExec := func(q string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	mustFail := func(q, why string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q); err == nil {
			t.Fatalf("expected failure (%s):\n%s", why, q)
		}
	}
	mustExec(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
	          VALUES ('u1', 'i1', 'a@b.c', 'A', 'face:sky', 'x', 's', 'argon2id', '2026-01-01 00:00:00', '2026-01-01 00:00:00', 1)`)
	mustExec(`INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
	          VALUES ('u2', 'i2', 'b@b.c', 'B', 'face:sky', 'x', 's', 'argon2id', '2026-01-01 00:00:00', '2026-01-01 00:00:00', 1)`)
	mustExec(`INSERT INTO currencies (id, code, symbol, created_at, user_id) VALUES ('p1', 'PTS', 'pts', '2026-01-01 00:00:00', 'u1')`)
	mustExec(`INSERT INTO currencies (id, code, symbol, created_at, user_id) VALUES ('p2', 'PTS', 'pts', '2026-01-01 00:00:00', 'u2')`)
	mustFail(`INSERT INTO currencies (id, code, symbol, created_at, user_id) VALUES ('p3', 'PTS', 'pts', '2026-01-01 00:00:00', 'u2')`, "duplicate (user, code)")
	mustFail(`INSERT INTO currencies (id, code, symbol, created_at) VALUES ('x1', 'USD', '$', '2026-01-01 00:00:00')`, "duplicate global code")
	mustExec(`INSERT INTO users_hidden_currencies (user_id, currency_id, created_at) VALUES ('u1', 'dffc2a06-6f29-4704-8575-31709adee926', '2026-01-01 00:00:00')`)
	mustFail(`INSERT INTO users_hidden_currencies (user_id, currency_id, created_at) VALUES ('u1', 'dffc2a06-6f29-4704-8575-31709adee926', '2026-01-01 00:00:00')`, "duplicate hidden PK")
	mustExec(`DELETE FROM users WHERE id = 'u1'`)
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM currencies WHERE user_id = 'u1'").Scan(&n); err != nil || n != 0 {
		t.Fatalf("user-delete cascade to custom currencies: n=%d err=%v", n, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users_hidden_currencies WHERE user_id = 'u1'").Scan(&n); err != nil || n != 0 {
		t.Fatalf("user-delete cascade to hidden rows: n=%d err=%v", n, err)
	}
}
```

Note: check `internal/infra/storage/migrations/embed.go` for the exported type name (`migrations.Migration` with `Version`/`SQL` fields is the expected shape — adapt `toRunList` to the actual exported API; `migrate.Migration` is in `internal/infra/storage/migrate/migrate.go`). Check the `users` seed column list against the CURRENT users schema (it has `algorithm` and `avatar` after the 2026 migrations) and the `transactions` column list against `sqlite/20260101000000.sql` — adjust the INSERT column lists so they are valid, keeping the assertions identical.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestMigration20260715 ./internal/infra/storage/migrations/ -v`
Expected: FAIL with `migration 20260715000000 not found in embed`.

- [ ] **Step 3: Write the sqlite migration (FK-closure rebuild)**

Create `internal/infra/storage/migrations/sqlite/20260715000000.sql`. Structure (this is the exact statement order; the un-shown table DDL and index DDL MUST be copied **verbatim** from `internal/infra/storage/migrations/sqlite/20260101000000.sql` — the closure tables were not touched by any later migration):

The FK closure of `currencies` (15 tables, all renamed → recreated → copied → dropped):
`currencies, accounts, budgets, budgets_elements, currencies_rates, accounts_access, accounts_folders, accounts_options, budgets_excluded_accounts, transactions, budgets_access, budgets_envelopes, budgets_folders, budgets_elements_limits, budgets_envelopes_categories`.
(NOT in the closure — leave untouched: `users, users_options, users_connections, users_connections_invites, users_password_requests, access_tokens, categories, tags, payees, folders, operation_requests_ids`.)

```sql
-- Per-user currencies: currencies gains user_id (NULL = global) and
-- is_archived; UNIQUE(code) is replaced by two partial unique indexes
-- (global codes instance-unique, custom codes unique per owner); new table
-- users_hidden_currencies stores which global currencies a user hid.
--
-- SQLite cannot drop a table-level UNIQUE, so currencies is rebuilt. Under
-- PRAGMA foreign_keys = ON, renaming currencies rewrites the FK clauses of
-- every table that references it, so the ENTIRE FK closure of currencies is
-- rebuilt in one pass (the same pattern as 20260101000000.sql). Rename all,
-- recreate all, copy parents before children, drop old children before old
-- parents.

ALTER TABLE currencies RENAME TO currencies__old;
ALTER TABLE accounts RENAME TO accounts__old;
ALTER TABLE accounts_access RENAME TO accounts_access__old;
ALTER TABLE accounts_folders RENAME TO accounts_folders__old;
ALTER TABLE accounts_options RENAME TO accounts_options__old;
ALTER TABLE transactions RENAME TO transactions__old;
ALTER TABLE currencies_rates RENAME TO currencies_rates__old;
ALTER TABLE budgets RENAME TO budgets__old;
ALTER TABLE budgets_access RENAME TO budgets_access__old;
ALTER TABLE budgets_folders RENAME TO budgets_folders__old;
ALTER TABLE budgets_elements RENAME TO budgets_elements__old;
ALTER TABLE budgets_elements_limits RENAME TO budgets_elements_limits__old;
ALTER TABLE budgets_envelopes RENAME TO budgets_envelopes__old;
ALTER TABLE budgets_envelopes_categories RENAME TO budgets_envelopes_categories__old;
ALTER TABLE budgets_excluded_accounts RENAME TO budgets_excluded_accounts__old;

CREATE TABLE currencies
(
    id         TEXT    NOT NULL
    , code       TEXT     NOT NULL
    , symbol     VARCHAR(12) NOT NULL
    , created_at DATETIME    NOT NULL
    , name VARCHAR(36) DEFAULT NULL
    , fraction_digits SMALLINT DEFAULT '2' NOT NULL
    , user_id TEXT DEFAULT NULL
    , is_archived BOOLEAN DEFAULT '0' NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

-- ... CREATE TABLE for the other 14 closure tables, copied VERBATIM from
-- 20260101000000.sql (same columns, constraints, FK clauses) ...

INSERT INTO currencies (id, code, symbol, created_at, name, fraction_digits)
SELECT id, code, symbol, created_at, name, fraction_digits FROM currencies__old;
INSERT INTO accounts SELECT * FROM accounts__old;
INSERT INTO currencies_rates SELECT * FROM currencies_rates__old;
INSERT INTO accounts_access SELECT * FROM accounts_access__old;
INSERT INTO accounts_folders SELECT * FROM accounts_folders__old;
INSERT INTO accounts_options SELECT * FROM accounts_options__old;
INSERT INTO transactions SELECT * FROM transactions__old;
INSERT INTO budgets SELECT * FROM budgets__old;
INSERT INTO budgets_access SELECT * FROM budgets_access__old;
INSERT INTO budgets_folders SELECT * FROM budgets_folders__old;
INSERT INTO budgets_elements SELECT * FROM budgets_elements__old;
INSERT INTO budgets_elements_limits SELECT * FROM budgets_elements_limits__old;
INSERT INTO budgets_envelopes SELECT * FROM budgets_envelopes__old;
INSERT INTO budgets_envelopes_categories SELECT * FROM budgets_envelopes_categories__old;
INSERT INTO budgets_excluded_accounts SELECT * FROM budgets_excluded_accounts__old;

DROP TABLE budgets_envelopes_categories__old;
DROP TABLE budgets_elements_limits__old;
DROP TABLE budgets_excluded_accounts__old;
DROP TABLE budgets_elements__old;
DROP TABLE budgets_envelopes__old;
DROP TABLE budgets_folders__old;
DROP TABLE budgets_access__old;
DROP TABLE transactions__old;
DROP TABLE accounts_access__old;
DROP TABLE accounts_folders__old;
DROP TABLE accounts_options__old;
DROP TABLE currencies_rates__old;
DROP TABLE accounts__old;
DROP TABLE budgets__old;
DROP TABLE currencies__old;

-- Recreate every index of the rebuilt tables (verbatim from
-- 20260101000000.sql), EXCEPT: do NOT recreate UNIQ_37C4469377153098
-- (the old unique code index); add the three new currencies indexes.
-- Full index list to recreate:
--   accounts: IDX_CAC89EAC38248176, IDX_CAC89EACA76ED395 (check exact names
--     in 20260101000000.sql), is_deleted_idx_accounts, user_id_is_deleted_idx_accounts
--   accounts_access: IDX on account_id + IDX on user_id
--   accounts_folders / accounts_options: their IDX_* entries
--   transactions: IDX_EAA81A4C* (6 of them), spent_idx_transactions,
--     type_idx_transactions, account_id_spent_at_idx_transactions,
--     account_recipient_id_spent_at_idx_transactions,
--     category_id_account_id_spent_at_idx_transactions,
--     tag_id_account_id_spent_at_idx_transactions
--   currencies_rates: identifier_uniq_currencies_rates,
--     base_currency_id_published_at_idx_currencies_rates,
--     currency_id_published_at_idx_currencies_rates,
--     published_at_idx_currencies_rates, IDX_5AA604E03101778E, IDX_5AA604E038248176
--   budgets: IDX_DCAA954838248176, IDX_DCAA9548A76ED395
--   budgets_access / budgets_folders / budgets_elements (incl.
--     identifier_uniq_budgets_elements, external_id_idx_budgets_elements) /
--     budgets_elements_limits (element_period_uniq_budgets_elements_limits,
--     period_idx_budgets_elements_limits) / budgets_envelopes /
--     budgets_envelopes_categories / budgets_excluded_accounts: their entries

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
```

To enumerate the exact table DDL and index set mechanically: open `sqlite/20260101000000.sql`, copy each closure table's `CREATE TABLE` and every `CREATE [UNIQUE] INDEX ... ON <closure-table>` statement. The authoritative index list is also recoverable by running the migrations and dumping `SELECT name, sql FROM sqlite_master WHERE type='index' AND name NOT LIKE 'sqlite_%'` (a probe during planning confirmed the set matches 20260101000000.sql).

- [ ] **Step 4: Write the pgsql migration (in-place ALTERs)**

Create `internal/infra/storage/migrations/pgsql/20260715000000.sql`:

```sql
-- Per-user currencies (PostgreSQL variant): in-place ALTERs. The inline
-- UNIQUE (code) constraint is auto-named currencies_code_key; the baseline
-- also created a redundant UNIQ_37C4469377153098 index on code. Both go.
ALTER TABLE currencies ADD COLUMN user_id UUID DEFAULT NULL;
ALTER TABLE currencies ADD COLUMN is_archived BOOLEAN DEFAULT FALSE NOT NULL;
ALTER TABLE currencies ADD CONSTRAINT fk_currencies_user_id FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE;
ALTER TABLE currencies DROP CONSTRAINT currencies_code_key;
DROP INDEX UNIQ_37C4469377153098;
CREATE UNIQUE INDEX UNIQ_currencies_code_global ON currencies (code) WHERE user_id IS NULL;
CREATE UNIQUE INDEX UNIQ_currencies_user_code ON currencies (user_id, code) WHERE user_id IS NOT NULL;
CREATE INDEX IDX_currencies_user_id ON currencies (user_id);

CREATE TABLE users_hidden_currencies
(
    user_id     UUID NOT NULL
    , currency_id UUID NOT NULL
    , created_at  TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (user_id, currency_id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE CASCADE
);
CREATE INDEX IDX_users_hidden_currencies_currency_id ON users_hidden_currencies (currency_id);
```

If `DROP CONSTRAINT currencies_code_key` fails in the pgsql CI job, find the real name with `SELECT conname FROM pg_constraint WHERE conrelid = 'currencies'::regclass AND contype = 'u';` against a scratch DB and fix — but `currencies_code_key` is the standard auto-name for an inline `UNIQUE (code)`.

- [ ] **Step 5: Run the migration tests**

Run: `go test -run TestMigration20260715 ./internal/infra/storage/migrations/ -v`
Expected: both tests PASS. If `DataSurvivesRebuild` reports 0 rows in any child table, the closure rebuild is wrong (a rename leaked an `__old` reference) — do not work around; fix the statement order.

Also run the whole existing suite to catch schema regressions: `go test ./internal/... 2>&1 | tail -20`
Expected: all pre-existing tests still PASS (sqlc gen code is not yet regenerated, so nothing else changed).

- [ ] **Step 6: Extend the fixture builder**

In `internal/test/fixture/entities.go`, extend the `Currency` struct and add a hidden-currency helper (follow the existing `b.insert`/`b.orNewID`/`b.now()` style; check the existing `Currency` builder at its current location for the exact insert helper signature):

```go
// Currency gains optional ownership and archival.
type Currency struct {
	ID             string
	Code           string
	Symbol         string
	Name           string
	FractionDigits *int
	UserID         string // empty = global (NULL)
	IsArchived     bool
}
```

Update `Builder.Currency` to include the two new columns:

```go
func (b *Builder) Currency(c Currency) string {
	id := b.orNewID(c.ID)
	code := c.Code
	if code == "" {
		code = "USD"
	}
	symbol := c.Symbol
	if symbol == "" {
		symbol = "$"
	}
	digits := 2
	if c.FractionDigits != nil {
		digits = *c.FractionDigits
	}
	b.insert(`INSERT INTO currencies (id, code, symbol, name, fraction_digits, user_id, is_archived, created_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, code, symbol, nullable(c.Name), digits, nullable(c.UserID), c.IsArchived, b.now())
	return id
}

// HiddenCurrency marks a global currency hidden for the user.
func (b *Builder) HiddenCurrency(userID, currencyID string) {
	b.insert(`INSERT INTO users_hidden_currencies (user_id, currency_id, created_at) VALUES (?, ?, ?)`,
		userID, currencyID, b.now())
}
```

(`nullable` already exists in the package; if `b.insert` differs in signature, match the neighbors. `IsArchived bool` binds fine on both engines.)

- [ ] **Step 7: Run the full test suite and commit**

Run: `go build ./... && go test ./internal/... 2>&1 | tail -5`
Expected: PASS.

```bash
git add internal/infra/storage/migrations/ internal/test/fixture/entities.go
git commit -m "feat(currency): migration for per-user currencies + hidden-currencies table"
```

---

### Task 2: sqlc queries + ManageRepo (write side)

**Files:**
- Modify: `internal/infra/storage/sqlc/query/sqlite/currency_write.sql` and `internal/infra/storage/sqlc/query/pgsql/currency_write.sql`
- Regenerate: `internal/infra/storage/sqlc/gen/{sqlite,pgsql}` (run `sqlc generate` from `internal/infra/storage/sqlc/`)
- Create: `internal/currency/repo/manage.go`, `internal/currency/repo/manage_integration_test.go`
- Modify: `internal/model/currency.go` (add `CurrencyRecord`)

**Interfaces:**
- Consumes: generated `sqlitegen`/`pgsqlgen` code; `backend.TxManager`; `internal/test/dbtest` + `internal/test/fixture`.
- Produces (used by Task 4/5 services):
  - `model.CurrencyRecord{ID, Code, Symbol string; Name *string; FractionDigits int; UserID *string; IsArchived bool; CreatedAt time.Time}`
  - `currencyrepo.NewManageRepo(driver string, tx *backend.TxManager) *ManageRepo` implementing (checked in Task 4 by `var _ appcurrency.ManageModel = (*ManageRepo)(nil)` at the wiring site):
    - `GetCurrencyRecord(ctx, id string) (model.CurrencyRecord, error)` (`sql.ErrNoRows` → `errs.NewNotFound("Currency not found")`)
    - `GlobalCodeExists(ctx, code string) (bool, error)`
    - `OwnerCodeExists(ctx, userID, code string) (bool, error)`
    - `InsertUserCurrency(ctx, c model.CurrencyRecord) error`
    - `UpdateCurrencyDetails(ctx, id, name, symbol string, fractionDigits int) error`
    - `SetCurrencyArchived(ctx, id string, archived bool) error`
    - `DeleteCurrency(ctx, id string) error`
    - `CountCurrencyUsage(ctx, id, code string) (int64, error)`
    - `GetGlobalIDByCode(ctx, code string) (string, error)` (`sql.ErrNoRows` → `errs.NewNotFound(...)`)
    - `UpsertRate(ctx, r model.RateRow) error` (reuse the existing generated `UpsertCurrencyRate`, date truncated to midnight UTC like `write.go`)
    - `HideCurrency(ctx, userID, currencyID string, now time.Time) error` (idempotent upsert)
    - `ShowCurrency(ctx, userID, currencyID string) error` (idempotent delete)

- [ ] **Step 1: Add the queries (both engines)**

Append to `query/sqlite/currency_write.sql` (ASCII-only comments!):

```sql
-- User currency management (per-user custom currencies). Global currencies
-- have user_id NULL; custom currencies carry their owner id.

-- name: GetCurrencyRecord :one
SELECT id, code, symbol, name, fraction_digits, user_id, is_archived, created_at
FROM currencies WHERE id = ?;

-- name: GlobalCurrencyCodeExists :one
SELECT COUNT(*) FROM currencies WHERE code = ? AND user_id IS NULL;

-- name: OwnerCurrencyCodeExists :one
SELECT COUNT(*) FROM currencies WHERE code = ? AND user_id = ?;

-- name: InsertUserCurrency :exec
INSERT INTO currencies (id, code, symbol, name, fraction_digits, user_id, is_archived, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateCurrencyDetails :exec
UPDATE currencies SET name = ?, symbol = ?, fraction_digits = ? WHERE id = ?;

-- name: SetCurrencyArchived :exec
UPDATE currencies SET is_archived = ? WHERE id = ?;

-- name: DeleteCurrency :exec
DELETE FROM currencies WHERE id = ?;

-- name: GetGlobalCurrencyIDByCode :one
SELECT id FROM currencies WHERE code = ? AND user_id IS NULL;

-- Usage census for delete protection: accounts (including soft-deleted ones,
-- they still hold the FK), budgets, budget elements, and any user whose
-- profile currency option stores this code.
-- name: CountCurrencyUsage :one
SELECT (SELECT COUNT(*) FROM accounts WHERE currency_id = ?)
     + (SELECT COUNT(*) FROM budgets WHERE currency_id = ?)
     + (SELECT COUNT(*) FROM budgets_elements WHERE currency_id = ?)
     + (SELECT COUNT(*) FROM users_options WHERE name = 'currency' AND value = ?) AS usage_count;

-- name: InsertHiddenCurrency :exec
INSERT INTO users_hidden_currencies (user_id, currency_id, created_at)
VALUES (?, ?, ?)
ON CONFLICT (user_id, currency_id) DO NOTHING;

-- name: DeleteHiddenCurrency :exec
DELETE FROM users_hidden_currencies WHERE user_id = ? AND currency_id = ?;
```

pgsql variants in `query/pgsql/currency_write.sql`: identical SQL with `$N` placeholders; for `CountCurrencyUsage` use `$1` for all three currency-id positions and `$2` for the code (the pgsql adapter passes two args; the sqlite adapter passes the id three times via the generated `CurrencyID`, `CurrencyID_2`, `CurrencyID_3` fields — this asymmetry has precedent in `tag_read.sql`).

Also scope the existing CLI-serving queries to globals only, in BOTH engine files (custom currencies must never leak into `currency:add` / `currency:update-rates` / OXR symbol lists):

```sql
-- name: ListCurrencyCodes :many
SELECT id, code FROM currencies WHERE user_id IS NULL;

-- name: GetCurrencyByCode :one
SELECT id, code, symbol, name, fraction_digits, created_at FROM currencies WHERE code = ? AND user_id IS NULL;
```

And in `query/{sqlite,pgsql}/currencies.sql` (the user-module lookup — profile currency, budget default; per-user resolution comes in Task 6, this keeps the global behavior explicit):

```sql
-- name: GetCurrencyIDByCode :one
SELECT id FROM currencies WHERE code = ? AND user_id IS NULL;

-- name: GetCurrencyIDByCodeForUser :one
SELECT id FROM currencies
WHERE code = ? AND (user_id IS NULL OR user_id = ?)
ORDER BY (user_id IS NULL) ASC
LIMIT 1;
```

(pgsql: `$1`, `$2`. The ORDER BY prefers the user's own custom over a same-code global; a custom cannot normally collide with a global — creation forbids it — but an admin may later add a colliding global.)

- [ ] **Step 2: Regenerate sqlc and build**

Run: `cd internal/infra/storage/sqlc && sqlc generate && cd - && go build ./...`
Expected: build passes. The `Currency` model struct in both `gen/sqlite/models.go` and `gen/pgsql/models.go` now has `UserID *string` and `IsArchived bool` (verified identical across engines during planning).

- [ ] **Step 3: Write the failing repo integration test**

Create `internal/currency/repo/manage_integration_test.go` (mirror the style of `lookup_read_integration_test.go`):

```go
package repo_test

import (
	"context"
	"testing"
	"time"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

const usdID = "dffc2a06-6f29-4704-8575-31709adee926"

func newManage(t *testing.T) (*currencyrepo.ManageRepo, *dbtest.DB, *fixture.Builder) {
	t.Helper()
	db := dbtest.New(t)
	return currencyrepo.NewManageRepo(db.Engine, db.TX), db, fixture.New(t, db)
}

func TestManageRepo_InsertGetUpdateArchiveDelete(t *testing.T) {
	r, _, f := newManage(t)
	ctx := context.Background()
	uid := f.User(fixture.User{Name: "A"})
	name := "Points"
	rec := model.CurrencyRecord{
		ID: fixture.NewID(), Code: "PTS", Symbol: "pts", Name: &name,
		FractionDigits: 0, UserID: &uid, CreatedAt: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
	}
	if err := r.InsertUserCurrency(ctx, rec); err != nil {
		t.Fatal(err)
	}
	got, err := r.GetCurrencyRecord(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Code != "PTS" || got.UserID == nil || *got.UserID != uid || got.IsArchived {
		t.Fatalf("unexpected record: %+v", got)
	}
	if err := r.UpdateCurrencyDetails(ctx, rec.ID, "Kid points", "kp", 2); err != nil {
		t.Fatal(err)
	}
	if err := r.SetCurrencyArchived(ctx, rec.ID, true); err != nil {
		t.Fatal(err)
	}
	got, _ = r.GetCurrencyRecord(ctx, rec.ID)
	if got.Name == nil || *got.Name != "Kid points" || got.Symbol != "kp" || got.FractionDigits != 2 || !got.IsArchived {
		t.Fatalf("update/archive not persisted: %+v", got)
	}
	if err := r.DeleteCurrency(ctx, rec.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.GetCurrencyRecord(ctx, rec.ID); err == nil {
		t.Fatal("expected NotFound after delete")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("want NotFound, got %v", err)
	}
}

func TestManageRepo_CodeExistence(t *testing.T) {
	r, _, f := newManage(t)
	ctx := context.Background()
	uid := f.User(fixture.User{Name: "A"})
	f.Currency(fixture.Currency{Code: "PTS", UserID: uid})
	if ok, _ := r.GlobalCodeExists(ctx, "USD"); !ok {
		t.Fatal("USD should exist globally")
	}
	if ok, _ := r.GlobalCodeExists(ctx, "PTS"); ok {
		t.Fatal("PTS is custom, not global")
	}
	if ok, _ := r.OwnerCodeExists(ctx, uid, "PTS"); !ok {
		t.Fatal("owner PTS should exist")
	}
	if ok, _ := r.OwnerCodeExists(ctx, fixture.NewID(), "PTS"); ok {
		t.Fatal("other user should not own PTS")
	}
}

func TestManageRepo_UsageCount(t *testing.T) {
	r, _, f := newManage(t)
	ctx := context.Background()
	uid := f.User(fixture.User{Name: "A"})
	cid := f.Currency(fixture.Currency{Code: "PTS", UserID: uid})
	if n, _ := r.CountCurrencyUsage(ctx, cid, "PTS"); n != 0 {
		t.Fatalf("fresh currency usage = %d, want 0", n)
	}
	f.Account(fixture.Account{UserID: uid, CurrencyID: cid, Name: "Kid"})
	f.Option(uid, "currency", "PTS")
	if n, _ := r.CountCurrencyUsage(ctx, cid, "PTS"); n != 2 {
		t.Fatalf("usage = %d, want 2 (account + profile option)", n)
	}
}

func TestManageRepo_HideShowIdempotent(t *testing.T) {
	r, _, f := newManage(t)
	ctx := context.Background()
	uid := f.User(fixture.User{Name: "A"})
	now := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	if err := r.HideCurrency(ctx, uid, usdID, now); err != nil {
		t.Fatal(err)
	}
	if err := r.HideCurrency(ctx, uid, usdID, now); err != nil {
		t.Fatal(err) // idempotent
	}
	if err := r.ShowCurrency(ctx, uid, usdID); err != nil {
		t.Fatal(err)
	}
	if err := r.ShowCurrency(ctx, uid, usdID); err != nil {
		t.Fatal(err) // idempotent
	}
}
```

(Adapt `f.Option`/`f.Account` argument shapes to the real fixture API — `Option(userID, name, value)` exists per the fixture inventory; if the value parameter is a pointer, adjust.)

- [ ] **Step 4: Run to verify failure**

Run: `go test -run TestManageRepo ./internal/currency/repo/ -v`
Expected: FAIL — `NewManageRepo` undefined.

- [ ] **Step 5: Add `model.CurrencyRecord` and implement `ManageRepo`**

Append to `internal/model/currency.go`:

```go
// CurrencyRecord is one currencies row with ownership and archival state.
// UserID nil = global (admin-managed); non-nil = custom, owned by that user.
type CurrencyRecord struct {
	ID             string
	Code           string
	Symbol         string
	Name           *string
	FractionDigits int
	UserID         *string
	IsArchived     bool
	CreatedAt      time.Time
}
```

Create `internal/currency/repo/manage.go` following the exact engine-adapter pattern of `internal/tag/repo` (canonical sqlite-generated types, `manageQuerier` interface, `sqliteManageQuerier` passthrough, `pgsqlManageQuerier` conversion shim, `NewManageRepo(driver, tx)` switch that panics on unknown driver). Canonical aliases:

```go
type currencyRecordRow = sqlitegen.GetCurrencyRecordRow
type insertUserCurrencyP = sqlitegen.InsertUserCurrencyParams
type updateCurrencyDetailsP = sqlitegen.UpdateCurrencyDetailsParams
type setCurrencyArchivedP = sqlitegen.SetCurrencyArchivedParams
type usageP = sqlitegen.CountCurrencyUsageParams
type hideP = sqlitegen.InsertHiddenCurrencyParams
type unhideP = sqlitegen.DeleteHiddenCurrencyParams
```

Method sketches (repeat the `db := r.tx.Querier(ctx)` + interface-call body style of `write.go`):

```go
func (r *ManageRepo) GetCurrencyRecord(ctx context.Context, id string) (model.CurrencyRecord, error) {
	row, err := r.q.GetCurrencyRecord(ctx, r.db(ctx), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.CurrencyRecord{}, errs.NewNotFound("Currency not found")
		}
		return model.CurrencyRecord{}, err
	}
	return model.CurrencyRecord{
		ID: row.ID, Code: row.Code, Symbol: row.Symbol, Name: row.Name,
		FractionDigits: int(row.FractionDigits), UserID: row.UserID,
		IsArchived: row.IsArchived, CreatedAt: row.CreatedAt,
	}, nil
}

func (r *ManageRepo) CountCurrencyUsage(ctx context.Context, id, code string) (int64, error) {
	return r.q.CountCurrencyUsage(ctx, r.db(ctx), usageP{
		CurrencyID: id, CurrencyID_2: id, CurrencyID_3: id, Value: &code,
	})
}
```

Notes: the generated field names for `CountCurrencyUsageParams` and the `Value` type (`*string` because `users_options.value` is nullable) must be checked against `gen/sqlite/currency_write.sql.go` after generation — match whatever sqlc produced. `UpsertRate` copies the body of `WriteRepo.UpsertRate` (midnight-UTC date truncation). The pgsql shim converts params with whole-struct casts where the generated types are field-identical, and field-by-field otherwise (the pgsql `CountCurrencyUsage` takes only `(id, code)` — its shim ignores `CurrencyID_2/3`).

- [ ] **Step 6: Run tests, gofmt, commit**

Run: `gofmt -l internal/ | grep -v /gen/ ; go test ./internal/currency/... ./internal/infra/... -count=1 2>&1 | tail -5`
Expected: no gofmt output, tests PASS.

```bash
git add internal/infra/storage/sqlc/ internal/currency/repo/manage.go internal/currency/repo/manage_integration_test.go internal/model/currency.go
git commit -m "feat(currency): manage repo + sqlc queries for per-user currencies"
```

---

### Task 3: Per-user read path (list scoping + latest-rate-per-currency)

**Files:**
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/currency_read.sql`
- Regenerate sqlc; Modify: `internal/model/currency_view.go`, `internal/currency/repo/read.go`, `internal/currency/read.go`
- Modify: `internal/currency/repo/lookup_read_integration_test.go` and any test broken by the `ReadModel` signature change
- Test: `internal/currency/read_scoping_test.go` (new, service-level with a fake), extend `internal/currency/repo/lookup_read_integration_test.go`

**Interfaces:**
- Consumes: Task 1 schema, fixtures (`Currency{UserID}`, `HiddenCurrency`, `AccountAccess`, `BudgetAccess`, `BudgetElement`).
- Produces:
  - `model.CurrencyViewRow` gains `UserID *string; IsArchived bool`.
  - `currency.ReadModel` becomes:
    ```go
    type ReadModel interface {
        UserCurrencyListView(ctx context.Context, userID string) ([]model.CurrencyViewRow, error)
        HiddenCurrencyIDs(ctx context.Context, userID string) ([]string, error)
        LatestCurrencyRateListView(ctx context.Context) ([]model.CurrencyRateViewRow, error)
    }
    ```
  - `model.CurrencyResult` gains `Scope string \`json:"scope"\``, `IsArchived int \`json:"isArchived"\``, `IsHidden int \`json:"isHidden"\`` (in `internal/model/currency_dto.go`).
  - `ReadService.GetCurrencyList` returns globals + own + shared-reachable rows with the three new fields; `GetCurrencyRateList` returns the latest rate per currency, filtered to the same visible set.

- [ ] **Step 1: Replace the read queries (both engines)**

In `query/sqlite/currency_read.sql`, replace `GetCurrencyListView` and `GetLatestCurrencyRateListView`, and add the hidden-ids query:

```sql
-- name: GetUserCurrencyListView :many
-- Per-user visible currencies: all globals, the user's own customs (archived
-- included, the settings page needs them), and foreign customs reachable via
-- accounts or budgets shared to the user (budget currency and element
-- currencies). Codes can repeat across owners, so id breaks ties.
SELECT c.id, c.code, c.symbol, c.name, c.fraction_digits, c.user_id, c.is_archived
FROM currencies c
WHERE c.user_id IS NULL
   OR c.user_id = ?
   OR c.id IN (
       SELECT a.currency_id FROM accounts a
       JOIN accounts_access aa ON aa.account_id = a.id
       WHERE aa.user_id = ?
   )
   OR c.id IN (
       SELECT b.currency_id FROM budgets b
       JOIN budgets_access ba ON ba.budget_id = b.id
       WHERE ba.user_id = ?
   )
   OR c.id IN (
       SELECT be.currency_id FROM budgets_elements be
       JOIN budgets_access ba ON ba.budget_id = be.budget_id
       WHERE ba.user_id = ? AND be.currency_id IS NOT NULL
   )
ORDER BY c.code ASC, c.id ASC;

-- name: GetHiddenCurrencyIDs :many
SELECT currency_id FROM users_hidden_currencies WHERE user_id = ?;

-- name: GetLatestCurrencyRateListView :many
-- Latest rate row per (currency, base) pair. The previous single-latest-date
-- form dropped any currency whose newest rate predates the newest OXR batch,
-- which breaks backdated custom rates.
SELECT cr.id, cr.currency_id, cr.base_currency_id, cr.published_at, cr.rate
FROM currencies_rates cr
WHERE cr.published_at = (
    SELECT MAX(cr2.published_at) FROM currencies_rates cr2
    WHERE cr2.currency_id = cr.currency_id AND cr2.base_currency_id = cr.base_currency_id
)
ORDER BY cr.currency_id ASC, cr.base_currency_id ASC;
```

Keep `GetCurrencyByIDView`, `GetAverageCurrencyRates`, `GetLatestCurrencyRateDate` unchanged. Delete the now-unused `GetCurrencyListView` (or leave it if the user-module still consumes it — grep first; the currency ReadRepo is the only consumer). pgsql variant: same SQL with `$1` reused for all four user positions (single-param generated func, precedent in `tag_read.sql`); ORDER BY identical.

- [ ] **Step 2: Regenerate sqlc**

Run: `cd internal/infra/storage/sqlc && sqlc generate && cd - && go build ./... 2>&1 | head`
Expected: compile errors ONLY in `internal/currency/repo/read.go` (removed/renamed generated funcs) — that's the next step.

- [ ] **Step 3: Update view row, repo, service**

`internal/model/currency_view.go`:

```go
type CurrencyViewRow struct {
	ID             string
	Code           string
	Symbol         string
	Name           *string
	FractionDigits int16
	UserID         *string
	IsArchived     bool
}
```

`internal/currency/repo/read.go`: canonical alias becomes `type currencyRow = sqlitegen.GetUserCurrencyListViewRow`; `readQuerier` gains the user-id params and `GetHiddenCurrencyIDs`; `ReadRepo` methods:

```go
func (r *ReadRepo) UserCurrencyListView(ctx context.Context, userID string) ([]model.CurrencyViewRow, error) {
	rows, err := r.q.GetUserCurrencyListView(ctx, r.db(ctx), userID)
	if err != nil {
		return nil, err
	}
	out := make([]model.CurrencyViewRow, 0, len(rows))
	for _, c := range rows {
		out = append(out, model.CurrencyViewRow{
			ID: c.ID, Code: c.Code, Symbol: c.Symbol, Name: c.Name,
			FractionDigits: c.FractionDigits, UserID: c.UserID, IsArchived: c.IsArchived,
		})
	}
	return out, nil
}

func (r *ReadRepo) HiddenCurrencyIDs(ctx context.Context, userID string) ([]string, error) {
	return r.q.GetHiddenCurrencyIDs(ctx, r.db(ctx), userID)
}
```

The sqlite adapter passes `sqlitegen.GetUserCurrencyListViewParams{UserID: userID, UserID_2: userID, UserID_3: userID, UserID_4: userID}` (verify generated field names); the pgsql adapter passes the single `userID` and converts rows with `currencyRow(c)` per-field if the struct shapes differ.

`internal/currency/read.go` — the service:

```go
const (
	ScopeGlobal = "global"
	ScopeOwn    = "own"
	ScopeShared = "shared"
)

func (s *ReadService) GetCurrencyList(ctx context.Context, userID vo.Id) (*model.GetCurrencyListResult, error) {
	rows, err := s.read.UserCurrencyListView(ctx, userID.String())
	if err != nil {
		return nil, err
	}
	hidden, err := s.read.HiddenCurrencyIDs(ctx, userID.String())
	if err != nil {
		return nil, err
	}
	hiddenSet := make(map[string]bool, len(hidden))
	for _, id := range hidden {
		hiddenSet[id] = true
	}
	items := make([]model.CurrencyResult, 0, len(rows))
	for _, r := range rows {
		scope := ScopeGlobal
		if r.UserID != nil {
			if *r.UserID == userID.String() {
				scope = ScopeOwn
			} else {
				scope = ScopeShared
			}
		}
		archived := 0
		if r.IsArchived {
			archived = 1
		}
		isHidden := 0
		if scope == ScopeGlobal && hiddenSet[r.ID] {
			isHidden = 1
		}
		items = append(items, model.CurrencyResult{
			Id: r.ID, Code: r.Code, Name: currencyName(r), Symbol: r.Symbol,
			FractionDigits: int(r.FractionDigits),
			Scope:          scope, IsArchived: archived, IsHidden: isHidden,
		})
	}
	return &model.GetCurrencyListResult{Items: items}, nil
}

func (s *ReadService) GetCurrencyRateList(ctx context.Context, userID vo.Id) (*model.GetCurrencyRateListResult, error) {
	visible, err := s.read.UserCurrencyListView(ctx, userID.String())
	if err != nil {
		return nil, err
	}
	visibleSet := make(map[string]bool, len(visible))
	for _, v := range visible {
		visibleSet[v.ID] = true
	}
	rows, err := s.read.LatestCurrencyRateListView(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]model.CurrencyRateResult, 0, len(rows))
	for _, r := range rows {
		if !visibleSet[r.CurrencyID] {
			continue
		}
		items = append(items, model.CurrencyRateResult{
			CurrencyId: r.CurrencyID, BaseCurrencyId: r.BaseCurrencyID,
			Rate: r.Rate, UpdatedAt: r.UpdatedAt,
		})
	}
	return &model.GetCurrencyRateListResult{Items: items}, nil
}
```

`internal/model/currency_dto.go` — extend `CurrencyResult` (keep existing fields in place, append):

```go
type CurrencyResult struct {
	Id             string `json:"id"`
	Code           string `json:"code"`
	Name           string `json:"name"`
	Symbol         string `json:"symbol"`
	FractionDigits int    `json:"fractionDigits"`
	Scope          string `json:"scope"`
	IsArchived     int    `json:"isArchived"`
	IsHidden       int    `json:"isHidden"`
}
```

- [ ] **Step 4: Write scoping integration tests**

Extend `internal/currency/repo/lookup_read_integration_test.go` (or a new `read_scoping_integration_test.go` in the same package) with a repo-level test:

```go
func TestReadRepo_UserCurrencyListScoping(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	read := currencyrepo.NewReadRepo(db.Engine, db.TX)
	ctx := context.Background()

	alice := f.User(fixture.User{Name: "Alice"})
	bob := f.User(fixture.User{Name: "Bob"})
	carol := f.User(fixture.User{Name: "Carol"})
	ptsAlice := f.Currency(fixture.Currency{Code: "PTS", UserID: alice, Name: "Points"})
	ptsBob := f.Currency(fixture.Currency{Code: "PTS", UserID: bob, Name: "Bob points"})
	gemCarol := f.Currency(fixture.Currency{Code: "GEM", UserID: carol, Name: "Gems"})

	// Bob shares an account denominated in his PTS with Alice.
	acc := f.Account(fixture.Account{UserID: bob, CurrencyID: ptsBob, Name: "Kid"})
	f.AccountAccess(acc, alice, 1)
	// Carol shares a budget with Alice; one element uses Carol's GEM.
	bud := f.Budget(fixture.Budget{UserID: carol})
	f.BudgetElement(fixture.BudgetElement{BudgetID: bud, CurrencyID: gemCarol, ExternalID: "e1", Type: 1})
	f.BudgetAccess(bud, alice, 1, true)

	rows, err := read.UserCurrencyListView(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, r := range rows {
		ids[r.ID] = true
	}
	if !ids[usdID] {
		t.Error("global USD missing")
	}
	if !ids[ptsAlice] {
		t.Error("own custom missing")
	}
	if !ids[ptsBob] {
		t.Error("shared-account custom missing")
	}
	if !ids[gemCarol] {
		t.Error("shared-budget element custom missing")
	}
	// Bob does NOT see Alice's or Carol's customs.
	rows, _ = read.UserCurrencyListView(ctx, bob)
	for _, r := range rows {
		if r.ID == ptsAlice || r.ID == gemCarol {
			t.Errorf("bob sees foreign custom %s", r.Code)
		}
	}
}

func TestReadRepo_LatestRatePerCurrency(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	read := currencyrepo.NewReadRepo(db.Engine, db.TX)
	ctx := context.Background()
	uid := f.User(fixture.User{Name: "A"})
	pts := f.Currency(fixture.Currency{Code: "PTS", UserID: uid})
	f.Rate(fixture.Rate{CurrencyID: pts, Rate: "100.00000000", PublishedAt: "2026-07-01"})
	f.Rate(fixture.Rate{CurrencyID: usdID, Rate: "1.00000000", PublishedAt: "2026-07-10"})
	rows, err := read.LatestCurrencyRateListView(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byCur := map[string]string{}
	for _, r := range rows {
		byCur[r.CurrencyID] = r.Rate
	}
	if byCur[pts] == "" {
		t.Fatal("backdated custom rate dropped by latest-rate query")
	}
	if byCur[usdID] == "" {
		t.Fatal("usd rate missing")
	}
}
```

(Adapt fixture builder shapes — `BudgetElement`/`BudgetAccess` signatures per `internal/test/fixture/entities.go`. `usdID` const from Task 2's test file — share it in one file per package.)

Also add a service-level test with an in-package fake `ReadModel` verifying scope/isArchived/isHidden mapping and the rate-list visibility filter (`internal/currency/read_scoping_test.go`), covering: own archived row → `scope:"own", isArchived:1`; hidden global → `isHidden:1`; foreign custom → `scope:"shared"`; rate of an invisible currency filtered out.

- [ ] **Step 5: Run, fix broken neighbors, commit**

Run: `go build ./... && go test ./internal/currency/... -count=1`
Then the full suite: `go test ./internal/... -count=1 2>&1 | tail -5` — the apiparity smoke suite will FAIL on the `currency_reads` golden (new fields, per-user list, rate-list order). That failure moves to Task 7 (goldens regenerate after the write endpoints land); run `go test $(go list ./internal/... | grep -v apiparity)` to confirm everything else is green. If other user-feature tests consume `GetCurrencyListView`, update them to the new interface.
Expected: everything except `internal/test/apiparity` PASS.

```bash
git add -A internal/ && git commit -m "feat(currency): per-user currency list + latest-rate-per-currency read path"
```

---

### Task 4: DTOs + ManageService lifecycle use cases (create/update/archive/unarchive/delete)

**Files:**
- Modify: `internal/model/currency_dto.go` (request/result DTOs + Validate)
- Create: `internal/currency/manage.go` (service + create/update use cases), `internal/currency/manage_lifecycle.go` (archive/unarchive/delete), `internal/currency/manage_test.go`
- Modify: `internal/currency/admin.go` (extract `validateCode` for reuse with a caller-chosen field key)

**Interfaces:**
- Consumes: Task 2 `ManageModel` methods, `port.TxRunner`, `port.OperationGuard`, `port.Clock`.
- Produces (consumed by Task 5/7):
  ```go
  type ManageModel interface { // internal/currency/manage.go, typed in model
      GetCurrencyRecord(ctx context.Context, id string) (model.CurrencyRecord, error)
      GlobalCodeExists(ctx context.Context, code string) (bool, error)
      OwnerCodeExists(ctx context.Context, userID, code string) (bool, error)
      InsertUserCurrency(ctx context.Context, c model.CurrencyRecord) error
      UpdateCurrencyDetails(ctx context.Context, id, name, symbol string, fractionDigits int) error
      SetCurrencyArchived(ctx context.Context, id string, archived bool) error
      DeleteCurrency(ctx context.Context, id string) error
      CountCurrencyUsage(ctx context.Context, id, code string) (int64, error)
      GetGlobalIDByCode(ctx context.Context, code string) (string, error)
      UpsertRate(ctx context.Context, r model.RateRow) error
      HideCurrency(ctx context.Context, userID, currencyID string, now time.Time) error
      ShowCurrency(ctx context.Context, userID, currencyID string) error
  }
  type ProfileCurrency interface { // consumer-side port, satisfied by glue in Task 7
      CurrencyCode(ctx context.Context, userID string) (string, error)
  }
  func NewManageService(repo ManageModel, tx port.TxRunner, ops port.OperationGuard, clock port.Clock, profile ProfileCurrency, baseCode string) *ManageService
  ```
  Use-case methods (all `func (s *ManageService) X(ctx, userID vo.Id, req model.XRequest) (*model.XResult, error)`): `CreateCurrency`, `UpdateCurrency`, `ArchiveCurrency`, `UnarchiveCurrency`, `DeleteCurrency` (this task); `SetCurrencyRate`, `HideCurrency`, `ShowCurrency` (Task 5).

- [ ] **Step 1: Write the DTOs**

Append to `internal/model/currency_dto.go`:

```go
// create-currency. Id is the client-generated operation id (idempotency key);
// the entity gets a fresh server id.
type CreateCurrencyRequest struct {
	Id             string  `json:"id"`
	Code           string  `json:"code"`
	Name           string  `json:"name"`
	Symbol         *string `json:"symbol"`
	FractionDigits *int    `json:"fractionDigits"`
	Rate           *string `json:"rate"`
}

func (r CreateCurrencyRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.Id) == "" {
		fields = append(fields, errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if strings.TrimSpace(r.Code) == "" {
		fields = append(fields, errs.FieldError{Key: "code", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if strings.TrimSpace(r.Name) == "" {
		fields = append(fields, errs.FieldError{Key: "name", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

type CreateCurrencyResult struct {
	Item CurrencyResult `json:"item"`
}

// update-currency: full replace of the mutable fields; code is immutable.
type UpdateCurrencyRequest struct {
	Id             string `json:"id"`
	Name           string `json:"name"`
	Symbol         string `json:"symbol"`
	FractionDigits int    `json:"fractionDigits"`
}

func (r UpdateCurrencyRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.Id) == "" {
		fields = append(fields, errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if strings.TrimSpace(r.Name) == "" {
		fields = append(fields, errs.FieldError{Key: "name", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if strings.TrimSpace(r.Symbol) == "" {
		fields = append(fields, errs.FieldError{Key: "symbol", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

type UpdateCurrencyResult struct {
	Item CurrencyResult `json:"item"`
}

type ArchiveCurrencyRequest struct {
	Id string `json:"id"`
}

func (r ArchiveCurrencyRequest) Validate() error { return validateBlankId(r.Id) }

type ArchiveCurrencyResult struct{}

type UnarchiveCurrencyRequest struct {
	Id string `json:"id"`
}

func (r UnarchiveCurrencyRequest) Validate() error { return validateBlankId(r.Id) }

type UnarchiveCurrencyResult struct{}

type DeleteCurrencyRequest struct {
	Id string `json:"id"`
}

func (r DeleteCurrencyRequest) Validate() error { return validateBlankId(r.Id) }

type DeleteCurrencyResult struct{}

type SetCurrencyRateRequest struct {
	CurrencyId string  `json:"currencyId"`
	Rate       string  `json:"rate"`
	Date       *string `json:"date"`
}

func (r SetCurrencyRateRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.CurrencyId) == "" {
		fields = append(fields, errs.FieldError{Key: "currencyId", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if strings.TrimSpace(r.Rate) == "" {
		fields = append(fields, errs.FieldError{Key: "rate", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

type SetCurrencyRateResult struct{}

type HideCurrencyRequest struct {
	Id string `json:"id"`
}

func (r HideCurrencyRequest) Validate() error { return validateBlankId(r.Id) }

type HideCurrencyResult struct{}

type ShowCurrencyRequest struct {
	Id string `json:"id"`
}

func (r ShowCurrencyRequest) Validate() error { return validateBlankId(r.Id) }

type ShowCurrencyResult struct{}

func validateBlankId(id string) error {
	if strings.TrimSpace(id) == "" {
		return errs.NewValidation("Validation failed",
			errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"})
	}
	return nil
}
```

(If `internal/model/currency_dto.go` lacks the `strings`/`errs` imports, add them. If a `validateBlankId`-equivalent helper already exists in the model package, use it instead.)

- [ ] **Step 2: Write failing service tests**

Create `internal/currency/manage_test.go` with an in-package fake `ManageModel` (map-backed) + fake `ProfileCurrency` + the `port` fakes used elsewhere in the package's tests (check `admin_integration_test.go` for the existing fake clock/tx style; a no-op TxRunner `func(ctx, fn) error { return fn(ctx) }` and a fixed clock suffice). Cover at minimum:

```go
// create
TestCreateCurrency_HappyPath                 // entity persisted with owner, symbol defaults to code, digits default 2; result Item has scope "own"
TestCreateCurrency_WithInitialRate           // rate row upserted in same flow with today's date
TestCreateCurrency_BadCode                   // "pt" and "P!S" -> validation, field "code", "CurrencyCode is incorrect"
TestCreateCurrency_DuplicateOwnCode          // -> "Currency already exists" (field "code")
TestCreateCurrency_CollidesWithGlobalCode    // "USD" -> "Currency already exists"
TestCreateCurrency_NameLength                // 65 chars -> "Currency name must be 1-64 characters"
TestCreateCurrency_SymbolLength              // 13 chars -> "Currency symbol must be 1-12 characters"
TestCreateCurrency_FractionDigitsRange       // -1 and 9 -> "Fraction digits must be between 0 and 8"
TestCreateCurrency_BadRate                   // "0", "-1", "abc" -> "Rate must be a positive number"
TestCreateCurrency_DuplicateOperation        // second Claim -> "Operation is locked"
// update
TestUpdateCurrency_HappyPath
TestUpdateCurrency_NotOwner                  // global and foreign-custom targets -> AccessDenied("")
TestUpdateCurrency_NotFound
// archive/unarchive
TestArchiveCurrency_OwnerOnly
TestUnarchiveCurrency
// delete
TestDeleteCurrency_RefusesWhenUsed           // usage > 0 -> "Currency is in use and cannot be deleted"
TestDeleteCurrency_HappyPath
TestDeleteCurrency_NotOwner
```

Assertion style: `errs.AsValidation(err)` / `errs.AsAccessDenied(err)` + exact message/field-key equality.

- [ ] **Step 3: Run to verify failure**

Run: `go test -run 'TestCreateCurrency|TestUpdateCurrency|TestArchiveCurrency|TestUnarchiveCurrency|TestDeleteCurrency' ./internal/currency/ -v 2>&1 | head`
Expected: FAIL to compile — `NewManageService` undefined.

- [ ] **Step 4: Implement the service**

`internal/currency/manage.go`:

```go
// User-facing currency management: per-user custom currencies. Global
// currencies (user_id NULL) stay admin/CLI territory; every mutation here
// requires the caller to own the target currency.
package currency

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

type ManageService struct {
	repo     ManageModel
	tx       port.TxRunner
	ops      port.OperationGuard
	clock    port.Clock
	profile  ProfileCurrency
	baseCode string
	nextID   func() vo.Id
}

func NewManageService(repo ManageModel, tx port.TxRunner, ops port.OperationGuard, clock port.Clock, profile ProfileCurrency, baseCode string) *ManageService {
	return &ManageService{repo: repo, tx: tx, ops: ops, clock: clock, profile: profile, baseCode: baseCode, nextID: vo.NewId}
}

// rateShape: scale-8 positive decimal string, no sign, no exponent.
var rateShape = regexp.MustCompile(`^[0-9]{1,11}(\.[0-9]{1,8})?$`)

func validateRate(rate string) error {
	bad := errs.NewValidation("Validation failed",
		errs.FieldError{Key: "rate", Message: "Rate must be a positive number"})
	if !rateShape.MatchString(rate) {
		return bad
	}
	if strings.Trim(strings.ReplaceAll(rate, ".", ""), "0") == "" {
		return bad // all zeros
	}
	return nil
}

func validateName(name string) (string, error) {
	n := strings.TrimSpace(name)
	if len(n) < 1 || len(n) > 64 {
		return "", errs.NewValidation("Validation failed",
			errs.FieldError{Key: "name", Message: "Currency name must be 1-64 characters"})
	}
	return n, nil
}

func validateSymbol(symbol string) (string, error) {
	sym := strings.TrimSpace(symbol)
	if len(sym) < 1 || len(sym) > 12 {
		return "", errs.NewValidation("Validation failed",
			errs.FieldError{Key: "symbol", Message: "Currency symbol must be 1-12 characters"})
	}
	return sym, nil
}

func validateFractionDigits(d int) error {
	if d < 0 || d > 8 {
		return errs.NewValidation("Validation failed",
			errs.FieldError{Key: "fractionDigits", Message: "Fraction digits must be between 0 and 8"})
	}
	return nil
}

// ownedRecord loads a currency and enforces that the caller owns it. A global
// or foreign currency answers with the same AccessDenied as other features'
// ownership failures (no existence leak beyond what the list already shows).
func (s *ManageService) ownedRecord(ctx context.Context, id string, userID vo.Id) (model.CurrencyRecord, error) {
	rec, err := s.repo.GetCurrencyRecord(ctx, id)
	if err != nil {
		return model.CurrencyRecord{}, err
	}
	if rec.UserID == nil || *rec.UserID != userID.String() {
		return model.CurrencyRecord{}, errs.NewAccessDenied("")
	}
	return rec, nil
}

func toCurrencyResult(rec model.CurrencyRecord, scope string) model.CurrencyResult {
	name := rec.Code
	if rec.Name != nil && *rec.Name != "" {
		name = *rec.Name
	}
	archived := 0
	if rec.IsArchived {
		archived = 1
	}
	return model.CurrencyResult{
		Id: rec.ID, Code: rec.Code, Name: name, Symbol: rec.Symbol,
		FractionDigits: rec.FractionDigits, Scope: scope, IsArchived: archived, IsHidden: 0,
	}
}

func (s *ManageService) CreateCurrency(ctx context.Context, userID vo.Id, req model.CreateCurrencyRequest) (*model.CreateCurrencyResult, error) {
	opID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	code, err := validateCodeField(req.Code, "code")
	if err != nil {
		return nil, err
	}
	name, err := validateName(req.Name)
	if err != nil {
		return nil, err
	}
	symbol := code
	if req.Symbol != nil && *req.Symbol != "" {
		if symbol, err = validateSymbol(*req.Symbol); err != nil {
			return nil, err
		}
	}
	digits := 2
	if req.FractionDigits != nil {
		digits = *req.FractionDigits
	}
	if err := validateFractionDigits(digits); err != nil {
		return nil, err
	}
	if req.Rate != nil {
		if err := validateRate(*req.Rate); err != nil {
			return nil, err
		}
	}
	uid := userID.String()
	rec := model.CurrencyRecord{
		ID: s.nextID().String(), Code: code, Symbol: symbol, Name: &name,
		FractionDigits: digits, UserID: &uid,
	}
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		now := s.clock.Now()
		already, cerr := s.ops.Claim(ctx, opID, now)
		if cerr != nil {
			return cerr
		}
		if already {
			return errs.NewValidation("Operation is locked")
		}
		dupOwn, cerr := s.repo.OwnerCodeExists(ctx, uid, code)
		if cerr != nil {
			return cerr
		}
		dupGlobal, cerr := s.repo.GlobalCodeExists(ctx, code)
		if cerr != nil {
			return cerr
		}
		if dupOwn || dupGlobal {
			return errs.NewValidation("Validation failed",
				errs.FieldError{Key: "code", Message: "Currency already exists"})
		}
		rec.CreatedAt = now
		if serr := s.repo.InsertUserCurrency(ctx, rec); serr != nil {
			return serr
		}
		if req.Rate != nil {
			baseID, berr := s.repo.GetGlobalIDByCode(ctx, s.baseCode)
			if berr != nil {
				return berr
			}
			if serr := s.repo.UpsertRate(ctx, model.RateRow{
				ID: s.nextID().String(), CurrencyID: rec.ID, BaseCurrencyID: baseID,
				Date: todayIn(ctx, now), Rate: *req.Rate,
			}); serr != nil {
				return serr
			}
		}
		return s.ops.MarkHandled(ctx, opID, now)
	}); err != nil {
		return nil, err
	}
	return &model.CreateCurrencyResult{Item: toCurrencyResult(rec, ScopeOwn)}, nil
}

// todayIn resolves "today" in the caller's timezone (X-Timezone header via
// reqctx), truncated to a date, expressed in UTC for storage.
func todayIn(ctx context.Context, now time.Time) time.Time {
	local := now.In(reqctx.Location(ctx))
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}
```

(`validateCodeField` = a refactor of `validateCode` in `admin.go` that takes the field key: `validateCodeField(code, key string)`; keep `validateCode(code)` delegating with key `"currency"` so the CLI path's wire shape is untouched. `reqctx.Location` exists — see `internal/shared/reqctx/reqctx.go:32`.)

`internal/currency/manage_lifecycle.go`:

```go
package currency

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *ManageService) UpdateCurrency(ctx context.Context, userID vo.Id, req model.UpdateCurrencyRequest) (*model.UpdateCurrencyResult, error) {
	name, err := validateName(req.Name)
	if err != nil {
		return nil, err
	}
	symbol, err := validateSymbol(req.Symbol)
	if err != nil {
		return nil, err
	}
	if err := validateFractionDigits(req.FractionDigits); err != nil {
		return nil, err
	}
	var rec model.CurrencyRecord
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		r, lerr := s.ownedRecord(ctx, req.Id, userID)
		if lerr != nil {
			return lerr
		}
		if uerr := s.repo.UpdateCurrencyDetails(ctx, r.ID, name, symbol, req.FractionDigits); uerr != nil {
			return uerr
		}
		r.Name = &name
		r.Symbol = symbol
		r.FractionDigits = req.FractionDigits
		rec = r
		return nil
	}); err != nil {
		return nil, err
	}
	return &model.UpdateCurrencyResult{Item: toCurrencyResult(rec, ScopeOwn)}, nil
}

func (s *ManageService) ArchiveCurrency(ctx context.Context, userID vo.Id, req model.ArchiveCurrencyRequest) (*model.ArchiveCurrencyResult, error) {
	if err := s.setArchived(ctx, userID, req.Id, true); err != nil {
		return nil, err
	}
	return &model.ArchiveCurrencyResult{}, nil
}

func (s *ManageService) UnarchiveCurrency(ctx context.Context, userID vo.Id, req model.UnarchiveCurrencyRequest) (*model.UnarchiveCurrencyResult, error) {
	if err := s.setArchived(ctx, userID, req.Id, false); err != nil {
		return nil, err
	}
	return &model.UnarchiveCurrencyResult{}, nil
}

func (s *ManageService) setArchived(ctx context.Context, userID vo.Id, id string, archived bool) error {
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		rec, err := s.ownedRecord(ctx, id, userID)
		if err != nil {
			return err
		}
		if rec.IsArchived == archived {
			return nil
		}
		return s.repo.SetCurrencyArchived(ctx, rec.ID, archived)
	})
}

func (s *ManageService) DeleteCurrency(ctx context.Context, userID vo.Id, req model.DeleteCurrencyRequest) (*model.DeleteCurrencyResult, error) {
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		rec, err := s.ownedRecord(ctx, req.Id, userID)
		if err != nil {
			return err
		}
		used, err := s.repo.CountCurrencyUsage(ctx, rec.ID, rec.Code)
		if err != nil {
			return err
		}
		if used > 0 {
			return errs.NewValidation("Currency is in use and cannot be deleted")
		}
		return s.repo.DeleteCurrency(ctx, rec.ID)
	}); err != nil {
		return nil, err
	}
	return &model.DeleteCurrencyResult{}, nil
}
```

Note on the usage census by code: `CountCurrencyUsage` checks `users_options.value = code` for the OWNER'S code. A same-code custom of another user would also match — acceptable over-caution (delete refuses; it never false-negatives). Document with the one-line comment already in the SQL.

- [ ] **Step 5: Run tests to green, gofmt, commit**

Run: `gofmt -l internal/currency/ ; go test ./internal/currency/... -count=1 -v 2>&1 | tail -15`
Expected: all PASS.

```bash
git add internal/model/currency_dto.go internal/currency/
git commit -m "feat(currency): manage service lifecycle use cases (create/update/archive/delete)"
```

---

### Task 5: ManageService rates + hide/show use cases

**Files:**
- Create: `internal/currency/rates.go`, `internal/currency/visibility.go`
- Modify: `internal/currency/manage_test.go` (extend) or create `internal/currency/rates_visibility_test.go`

**Interfaces:**
- Consumes: Task 4 `ManageService`, `ProfileCurrency` port, `datetime.DateLayout`.
- Produces: `SetCurrencyRate`, `HideCurrency`, `ShowCurrency` methods (signatures per Task 4 Interfaces block).

- [ ] **Step 1: Write failing tests**

Extend the Task 4 fakes. Cases:

```go
TestSetCurrencyRate_HappyPathDefaultDate   // no date -> row upserted with today (fixed clock, UTC ctx)
TestSetCurrencyRate_ExplicitDate           // "2026-01-15" -> row with that date
TestSetCurrencyRate_BadDate                // "15/01/2026" -> "Date is not valid" (field "date")
TestSetCurrencyRate_BadRate                // "0" -> "Rate must be a positive number"
TestSetCurrencyRate_GlobalTarget           // USD -> AccessDenied("")
TestSetCurrencyRate_ForeignTarget          // other user's custom -> AccessDenied("")
TestHideCurrency_HappyPath                 // global, not base, not profile -> hidden row written
TestHideCurrency_CustomTarget              // own custom -> "This currency cannot be hidden"
TestHideCurrency_BaseCurrency              // base (USD) -> "The base currency cannot be modified"
TestHideCurrency_ProfileCurrency           // profile code EUR, hide EUR -> "This currency cannot be hidden"
TestShowCurrency_HappyPath                 // removes hidden row; idempotent when absent
```

- [ ] **Step 2: Run to verify failure**

Run: `go test -run 'TestSetCurrencyRate|TestHideCurrency|TestShowCurrency' ./internal/currency/ 2>&1 | head -5`
Expected: compile FAIL (methods undefined).

- [ ] **Step 3: Implement**

`internal/currency/rates.go`:

```go
package currency

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// SetCurrencyRate upserts one dated rate for an owned custom currency against
// the instance base currency. Date defaults to today in the caller's timezone.
func (s *ManageService) SetCurrencyRate(ctx context.Context, userID vo.Id, req model.SetCurrencyRateRequest) (*model.SetCurrencyRateResult, error) {
	if err := validateRate(req.Rate); err != nil {
		return nil, err
	}
	date := todayIn(ctx, s.clock.Now())
	if req.Date != nil && *req.Date != "" {
		parsed, perr := time.ParseInLocation(datetime.DateLayout, *req.Date, time.UTC)
		if perr != nil {
			return nil, errs.NewValidation("Validation failed",
				errs.FieldError{Key: "date", Message: "Date is not valid"})
		}
		date = parsed
	}
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		rec, err := s.ownedRecord(ctx, req.CurrencyId, userID)
		if err != nil {
			return err
		}
		baseID, err := s.repo.GetGlobalIDByCode(ctx, s.baseCode)
		if err != nil {
			return err
		}
		return s.repo.UpsertRate(ctx, model.RateRow{
			ID: s.nextID().String(), CurrencyID: rec.ID, BaseCurrencyID: baseID,
			Date: date, Rate: req.Rate,
		})
	}); err != nil {
		return nil, err
	}
	return &model.SetCurrencyRateResult{}, nil
}
```

(The base-currency case needs no special guard here: the base is global, so `ownedRecord` already answers AccessDenied.)

`internal/currency/visibility.go`:

```go
package currency

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// HideCurrency removes a GLOBAL currency from the caller's dropdowns. Custom
// currencies archive instead; the base currency and the caller's profile
// currency must stay visible.
func (s *ManageService) HideCurrency(ctx context.Context, userID vo.Id, req model.HideCurrencyRequest) (*model.HideCurrencyResult, error) {
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		rec, err := s.repo.GetCurrencyRecord(ctx, req.Id)
		if err != nil {
			return err
		}
		if rec.UserID != nil {
			return errs.NewValidation("This currency cannot be hidden")
		}
		if rec.Code == s.baseCode {
			return errs.NewValidation("The base currency cannot be modified")
		}
		profileCode, err := s.profile.CurrencyCode(ctx, userID.String())
		if err != nil {
			return err
		}
		if rec.Code == profileCode {
			return errs.NewValidation("This currency cannot be hidden")
		}
		return s.repo.HideCurrency(ctx, userID.String(), rec.ID, s.clock.Now())
	}); err != nil {
		return nil, err
	}
	return &model.HideCurrencyResult{}, nil
}

func (s *ManageService) ShowCurrency(ctx context.Context, userID vo.Id, req model.ShowCurrencyRequest) (*model.ShowCurrencyResult, error) {
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		rec, err := s.repo.GetCurrencyRecord(ctx, req.Id)
		if err != nil {
			return err
		}
		return s.repo.ShowCurrency(ctx, userID.String(), rec.ID)
	}); err != nil {
		return nil, err
	}
	return &model.ShowCurrencyResult{}, nil
}
```

- [ ] **Step 4: Run tests, commit**

Run: `go test ./internal/currency/... -count=1 2>&1 | tail -5`
Expected: PASS.

```bash
git add internal/currency/ && git commit -m "feat(currency): set-rate and hide/show use cases"
```

---

### Task 6: User-aware code resolution + denomination usability checks

**Files:**
- Modify: `internal/currency/repo/lookup.go` (add `GetIDByCodeForUser`, `EnsureUsable`)
- Modify: `internal/user/ports.go`, `internal/user/profile.go`, `internal/user/usecase.go`, `internal/user/read.go` (+ its read repo/query `user_read.sql` if it resolves currency by code — grep `CurrencyIDByCode` in `internal/user/` and `query/{sqlite,pgsql}/user_read.sql`)
- Modify: `internal/budget/ports.go`, `internal/budget/create.go`, `internal/budget/crud.go`, `internal/budget/accounts.go`
- Modify: `internal/account/ports.go`, `internal/account/create.go`, `internal/account/update.go`
- Modify: `internal/server/glue_account.go`, `internal/server/glue_transaction.go` (import code lookup), `internal/server/server.go` as needed
- Tests: extend `internal/currency/repo/lookup_read_integration_test.go`; feature tests in `internal/account/` and `internal/budget/` (extend the existing use-case test files alongside the code they test)

**Interfaces:**
- Consumes: Task 2 queries (`GetCurrencyIDByCodeForUser`, `GetCurrencyRecord`).
- Produces:
  - `currencyrepo.Lookup.GetIDByCodeForUser(ctx context.Context, userID, code string) (string, error)` — own custom first, then global; NotFound otherwise.
  - `currencyrepo.Lookup.EnsureUsable(ctx context.Context, userID, currencyID string) error` — nil when global, or own AND not archived; `errs.NewNotFound("Currency not found")` when missing; otherwise `errs.NewValidation("Currency is not available", errs.FieldError{Key: "currencyId", Message: "Currency is not available"})`.
  - Feature ports updated:
    - `user.CurrencyLookup.GetIDByCode(ctx, userID, code string)` (adds userID; `DefaultCode()` unchanged)
    - `budget.CurrencyLookup` gains the userID param on `GetIDByCode` AND a new `EnsureUsable(ctx, userID, currencyID string) error`
    - `account.CurrencyLookup` gains `EnsureUsable(ctx, userID, currencyID string) error`
  - Rule applied at: account create (always), account update (only when the currency **changes**), budget create with explicit currencyId, budget update (only when changed), change-element-currency (only when changed).

- [ ] **Step 1: Write failing lookup tests**

Extend `internal/currency/repo/lookup_read_integration_test.go`:

```go
func TestLookup_GetIDByCodeForUser(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	lk := currencyrepo.New(db.Engine, db.TX)
	ctx := context.Background()
	uid := f.User(fixture.User{Name: "A"})
	pts := f.Currency(fixture.Currency{Code: "PTS", UserID: uid})
	// Own custom resolves.
	got, err := lk.GetIDByCodeForUser(ctx, uid, "PTS")
	if err != nil || got != pts {
		t.Fatalf("own custom: got %q err %v", got, err)
	}
	// Global resolves for anyone.
	if got, err = lk.GetIDByCodeForUser(ctx, uid, "USD"); err != nil || got != usdID {
		t.Fatalf("global: got %q err %v", got, err)
	}
	// Foreign custom does NOT resolve.
	other := f.User(fixture.User{Name: "B"})
	if _, err = lk.GetIDByCodeForUser(ctx, other, "PTS"); err == nil {
		t.Fatal("foreign custom code must not resolve")
	}
}

func TestLookup_EnsureUsable(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	lk := currencyrepo.New(db.Engine, db.TX)
	ctx := context.Background()
	alice := f.User(fixture.User{Name: "Alice"})
	bob := f.User(fixture.User{Name: "Bob"})
	pts := f.Currency(fixture.Currency{Code: "PTS", UserID: alice})
	old := f.Currency(fixture.Currency{Code: "OLD", UserID: alice, IsArchived: true})
	if err := lk.EnsureUsable(ctx, alice, usdID); err != nil {
		t.Errorf("global should be usable: %v", err)
	}
	if err := lk.EnsureUsable(ctx, alice, pts); err != nil {
		t.Errorf("own custom should be usable: %v", err)
	}
	if err := lk.EnsureUsable(ctx, bob, pts); err == nil {
		t.Error("foreign custom must be rejected")
	} else if v, ok := errs.AsValidation(err); !ok || v.Msg != "Validation failed" || v.Fields[0].Message != "Currency is not available" {
		t.Errorf("wrong error: %v", err)
	}
	if err := lk.EnsureUsable(ctx, alice, old); err == nil {
		t.Error("own archived custom must be rejected")
	}
	if err := lk.EnsureUsable(ctx, alice, fixture.NewID()); err == nil {
		t.Error("missing currency must error")
	} else if _, ok := errs.AsNotFound(err); !ok {
		t.Errorf("want NotFound, got %v", err)
	}
}
```

Run: `go test -run 'TestLookup_GetIDByCodeForUser|TestLookup_EnsureUsable' ./internal/currency/repo/ -v` — expected compile FAIL.

- [ ] **Step 2: Implement the Lookup methods**

In `internal/currency/repo/lookup.go`, extend the `lookupQuerier` interface with `GetCurrencyIDByCodeForUser` and `GetCurrencyRecord` (share the generated funcs; add both engine adapters), then:

```go
// GetIDByCodeForUser resolves a code preferring the user's own custom
// currency, then a global one. Foreign customs never resolve.
func (l *Lookup) GetIDByCodeForUser(ctx context.Context, userID, code string) (string, error) {
	id, err := l.q.GetCurrencyIDByCodeForUser(ctx, l.db(ctx), code, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errs.NewNotFound("Currency " + code + " not found")
		}
		return "", err
	}
	return id, nil
}

// EnsureUsable reports whether the user may denominate new entities in the
// currency: global, or their own non-archived custom.
func (l *Lookup) EnsureUsable(ctx context.Context, userID, currencyID string) error {
	row, err := l.q.GetCurrencyRecord(ctx, l.db(ctx), currencyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.NewNotFound("Currency not found")
		}
		return err
	}
	if row.UserID == nil {
		return nil
	}
	if *row.UserID == userID && !row.IsArchived {
		return nil
	}
	return errs.NewValidation("Currency is not available",
		errs.FieldError{Key: "currencyId", Message: "Currency is not available"})
}
```

(Adapter arg order per the generated params struct. Note the fieldful validation error renders as `"Form validation error"` + `errors: {currencyId: ["Currency is not available"]}` on the wire — consistent with other field errors.)

- [ ] **Step 3: Update the consumer ports and call sites (TDD per feature)**

For EACH consumer, first extend that feature's existing use-case test file with a failing case, then wire the port. The features use consumer-side fakes in their tests — extend those fakes with the new methods.

**account** (`internal/account/ports.go`):

```go
type CurrencyLookup interface {
	GetByID(ctx context.Context, id string) (model.CurrencyView, error)
	EnsureUsable(ctx context.Context, userID, currencyID string) error
}
```

`internal/account/create.go` — right after the `vo.ParseId(req.CurrencyId)` block:

```go
if err := s.currency.EnsureUsable(ctx, userID.String(), currencyID.String()); err != nil {
	return nil, err
}
```

`internal/account/update.go` — the account is loaded before mutation; check only on change (keeps forms that resend an unchanged — possibly archived/foreign — currency working):

```go
if currencyID != nil && !currencyID.Equal(acct.CurrencyID) {
	if err := s.currency.EnsureUsable(ctx, userID.String(), currencyID.String()); err != nil {
		return nil, err
	}
}
```

(Place it where the loaded `acct` is in scope — inside the mutate/tx closure if that's where the load happens; match the file's actual structure. `vo.Id.Equal` exists — used in payee ownership checks.)

Tests (extend the account feature's create/update test files): creating an account in a foreign custom → error `"Currency is not available"`; in own non-archived custom → OK; updating an account's OTHER field while its currency is archived → OK; changing to an archived custom → error.

**budget** (`internal/budget/ports.go`):

```go
type CurrencyLookup interface {
	GetIDByCode(ctx context.Context, userID, code string) (string, error)
	EnsureUsable(ctx context.Context, userID, currencyID string) error
}
```

`internal/budget/create.go`: the fallback call becomes `s.currency.GetIDByCode(ctx, userID.String(), code)`; the explicit-id path gains `EnsureUsable(ctx, userID.String(), curID.String())` after the parse. `internal/budget/crud.go` (UpdateBudget): after parsing `req.CurrencyId`, check `EnsureUsable` only when it differs from the loaded budget's current currency id. `internal/budget/accounts.go` (ChangeElementCurrency): check only when the parsed id differs from the element's current currency (`el` is loaded there; compare against its currency pointer).

**user** (`internal/user/ports.go`): `GetIDByCode(ctx context.Context, userID, code string) (string, error)`. Update `profile.go:35` (`s.currency.GetIDByCode(ctx, userID.String(), code)`) and `usecase.go:134-145` (both calls — the fallback re-resolve passes the same userID). For `read.go:103` the read-side port `CurrencyIDByCode` also gains userID; update `query/{sqlite,pgsql}/user_read.sql`'s currency query to the ForUser form (same SQL as `GetCurrencyIDByCodeForUser` in Task 2) and regenerate.

**transaction import** (`internal/server/glue_transaction.go:107-132`): the `transactionImportCurrencyByCode` adapter now calls `GetIDByCodeForUser` — check how the importing user's id reaches that path; if the port is code-only, extend it with userID the same way (the import use case has the caller's userID).

**glue** (`internal/server/glue_account.go`): forward the new method:

```go
func (l *AccountCurrencyLookup) EnsureUsable(ctx context.Context, userID, currencyID string) error {
	return l.inner.EnsureUsable(ctx, userID, currencyID)
}
```

`budget`'s CurrencyLookup is satisfied directly by `*currencyrepo.Lookup` (structural) — the new methods land automatically; `user`'s likewise (verify with `go build ./...`).

- [ ] **Step 4: Run the full suite**

Run: `go build ./... && go test $(go list ./internal/... | grep -v apiparity) -count=1 2>&1 | tail -5`
Expected: PASS (apiparity still red until Task 7). Fix any test fakes that now miss interface methods.

- [ ] **Step 5: Commit**

```bash
git add -A internal/ && git commit -m "feat(currency): user-aware code resolution + denomination usability checks"
```

---

### Task 7: HTTP edge — handlers, routes, wiring, swagger, apiparity

**Files:**
- Modify: `internal/currency/api/handler.go`, `internal/currency/api/currency.go`, `internal/currency/api/routes.go`
- Create: `internal/server/glue_currency.go`
- Modify: `internal/server/server.go` (wire ManageService + glue)
- Modify: `internal/test/apiparity/catalogue.go` (new scenario), `internal/test/apiparity/guard_test.go` (`minRoutes` 85 → 93)
- Regenerate: OpenAPI docs (`make swagger`), apiparity goldens (`UPDATE_GOLDEN=1`)
- Test: `internal/currency/api/currency_endpoints_test.go` (extend, using the existing `harness_test.go` + `authstub` pattern)

**Interfaces:**
- Consumes: Tasks 4-5 `ManageService`; `endpoint.Handle`; `middleware.TokenAuthenticator`; user feature's public API for the profile-code glue.
- Produces: the 8 routes (Global Constraints list); `handlercurrency.NewHandlers(read *appcurrency.ReadService, manage *appcurrency.ManageService, dev bool)`; `server.NewCurrencyProfileCurrency(userSvc)` glue implementing `appcurrency.ProfileCurrency`.

- [ ] **Step 1: Write failing endpoint tests**

Extend `internal/currency/api/currency_endpoints_test.go` following the file's existing harness pattern (authstub bearer token = user id). Cover per endpoint: 200 happy path; 401 without token; one 4xx contract case (create duplicate code → 400 `"Currency already exists"`; update foreign → 403; delete in-use → 400; hide custom → 400; set-rate global → 403). Assert envelope shape (`success`, `data`) and exact messages.

- [ ] **Step 2: Handlers + routes**

`internal/currency/api/handler.go`:

```go
type Handlers struct {
	read   *appcurrency.ReadService
	manage *appcurrency.ManageService
	dev    bool
}

func NewHandlers(read *appcurrency.ReadService, manage *appcurrency.ManageService, dev bool) *Handlers {
	return &Handlers{read: read, manage: manage, dev: dev}
}
```

`internal/currency/api/currency.go` — add the 8 handlers. Full swag block on each (copy the annotation shape from `internal/category/api/category.go`, adjusting Tags to `Currency`, routes, and request/result models). Two examples; the remaining six follow the same two shapes:

```go
// CreateCurrency handles POST /api/v1/currency/create-currency (auth).
//
// @Summary     Create a custom currency
// @Description Creates a per-user custom currency. Idempotent on the request id.
// @Tags        Currency
// @Accept      json
// @Produce     json
// @Param       request body     model.CreateCurrencyRequest true "Create currency request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.CreateCurrencyResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/currency/create-currency [post]
func (h *Handlers) CreateCurrency(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, func(ctx context.Context, userID vo.Id, req model.CreateCurrencyRequest) (*model.CreateCurrencyResult, error) {
		reqctx.AddLogAttr(ctx, "currency_code", req.Code)
		return h.manage.CreateCurrency(ctx, userID, req)
	})
}

// ArchiveCurrency handles POST /api/v1/currency/archive-currency (auth).
// ... (same annotation block shape) ...
func (h *Handlers) ArchiveCurrency(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.dev, func(ctx context.Context, userID vo.Id, req model.ArchiveCurrencyRequest) (*model.ArchiveCurrencyResult, error) {
		reqctx.AddLogAttr(ctx, "currency_id", req.Id)
		return h.manage.ArchiveCurrency(ctx, userID, req)
	})
}
```

(`UpdateCurrency`, `UnarchiveCurrency`, `DeleteCurrency`, `HideCurrency`, `ShowCurrency` mirror ArchiveCurrency with their DTOs; `SetCurrencyRate` logs `currency_id` from `req.CurrencyId`.)

`internal/currency/api/routes.go` — append inside the registration func:

```go
mux.Handle("POST /api/v1/currency/create-currency", auth(h.CreateCurrency))
mux.Handle("POST /api/v1/currency/update-currency", auth(h.UpdateCurrency))
mux.Handle("POST /api/v1/currency/archive-currency", auth(h.ArchiveCurrency))
mux.Handle("POST /api/v1/currency/unarchive-currency", auth(h.UnarchiveCurrency))
mux.Handle("POST /api/v1/currency/delete-currency", auth(h.DeleteCurrency))
mux.Handle("POST /api/v1/currency/set-currency-rate", auth(h.SetCurrencyRate))
mux.Handle("POST /api/v1/currency/hide-currency", auth(h.HideCurrency))
mux.Handle("POST /api/v1/currency/show-currency", auth(h.ShowCurrency))
```

- [ ] **Step 3: Wiring + glue**

`internal/server/glue_currency.go`:

```go
// Currency glue: adapters for ports the currency feature declares.
package server

import (
	"context"

	appcurrency "github.com/econumo/econumo/internal/currency"
	appuser "github.com/econumo/econumo/internal/user"
)

// CurrencyProfileCurrency answers "what is this user's profile currency code"
// for the hide-currency guard, via the user feature's public API.
type CurrencyProfileCurrency struct {
	inner *appuser.Service
}

var _ appcurrency.ProfileCurrency = (*CurrencyProfileCurrency)(nil)

func NewCurrencyProfileCurrency(inner *appuser.Service) *CurrencyProfileCurrency {
	return &CurrencyProfileCurrency{inner: inner}
}

func (p *CurrencyProfileCurrency) CurrencyCode(ctx context.Context, userID string) (string, error) {
	return p.inner.CurrencyCode(ctx, userID)
}
```

Check what the user service actually exposes: `glue_budget.go`'s `BudgetUserLookup.CurrencyCode` (lines ~226-239) already reads the profile currency — reuse the SAME underlying call; if it's on a read service or repo rather than `appuser.Service`, mirror that exact dependency instead.

`internal/server/server.go` — in the currency section:

```go
currencyManageRepo := currencyrepo.NewManageRepo(cfg.DatabaseDriver, txm)
currencyManageSvc := appcurrency.NewManageService(currencyManageRepo, txm, opGuard, clk,
	NewCurrencyProfileCurrency(userSvc), cfg.CurrencyBase)
currencyHandlers := handlercurrency.NewHandlers(currencyReadSvc, currencyManageSvc, cfg.IsDev())
```

(Order note: `userSvc` must be constructed before this line — check the current construction order; the currency handlers are only used at registration time, so moving the currency block after the user block is safe if needed.)

- [ ] **Step 4: Swagger + endpoint tests green**

Run: `make swagger && go test ./internal/currency/... -count=1 2>&1 | tail -5`
Expected: docs regenerate without diff-check errors; endpoint tests PASS.

- [ ] **Step 5: apiparity scenario + goldens + guard**

In `internal/test/apiparity/catalogue.go` add (after the payee scenario, following its exact idiom — `CaptureIDInto` + pointer bodies):

```go
register(Scenario{Name: "currency_write_read", Calls: func() []Call {
	const opCreate = "cc000000-0000-0000-0000-0000000000f1"
	const opCreate2 = "cc000000-0000-0000-0000-0000000000f2"
	const usd = "dffc2a06-6f29-4704-8575-31709adee926"
	var curID string
	return []Call{
		{Label: "create-currency", Method: "POST", Path: "/api/v1/currency/create-currency", Auth: "owner",
			Body: map[string]any{"id": opCreate, "code": "PTS", "name": "Points", "symbol": "pts", "fractionDigits": 0, "rate": "100"}, CaptureIDInto: &curID},
		{Label: "read-after-create", Method: "GET", Path: "/api/v1/currency/get-currency-list", Auth: "owner", Body: map[string]any{}},
		{Label: "rates-after-create", Method: "GET", Path: "/api/v1/currency/get-currency-rate-list", Auth: "owner", Body: map[string]any{}},
		{Label: "err:create-duplicate-code", Method: "POST", Path: "/api/v1/currency/create-currency", Auth: "owner",
			Body: map[string]any{"id": opCreate2, "code": "PTS", "name": "Points again"}},
		{Label: "update-currency", Method: "POST", Path: "/api/v1/currency/update-currency", Auth: "owner",
			Body: map[string]any{"id": &curID, "name": "Kid points", "symbol": "kp", "fractionDigits": 2}},
		{Label: "err:update-foreign", Method: "POST", Path: "/api/v1/currency/update-currency", Auth: "guest",
			Body: map[string]any{"id": &curID, "name": "Hijack", "symbol": "x", "fractionDigits": 2}},
		{Label: "set-currency-rate", Method: "POST", Path: "/api/v1/currency/set-currency-rate", Auth: "owner",
			Body: map[string]any{"currencyId": &curID, "rate": "120.5", "date": "2026-01-15"}},
		{Label: "err:set-rate-global", Method: "POST", Path: "/api/v1/currency/set-currency-rate", Auth: "owner",
			Body: map[string]any{"currencyId": usd, "rate": "2"}},
		{Label: "archive-currency", Method: "POST", Path: "/api/v1/currency/archive-currency", Auth: "owner", Body: map[string]any{"id": &curID}},
		{Label: "read-after-archive", Method: "GET", Path: "/api/v1/currency/get-currency-list", Auth: "owner", Body: map[string]any{}},
		{Label: "unarchive-currency", Method: "POST", Path: "/api/v1/currency/unarchive-currency", Auth: "owner", Body: map[string]any{"id": &curID}},
		{Label: "hide-currency", Method: "POST", Path: "/api/v1/currency/hide-currency", Auth: "owner", Body: map[string]any{"id": usd}},
		{Label: "read-after-hide", Method: "GET", Path: "/api/v1/currency/get-currency-list", Auth: "owner", Body: map[string]any{}},
		{Label: "show-currency", Method: "POST", Path: "/api/v1/currency/show-currency", Auth: "owner", Body: map[string]any{"id": usd}},
		{Label: "err:hide-custom", Method: "POST", Path: "/api/v1/currency/hide-currency", Auth: "owner", Body: map[string]any{"id": &curID}},
		{Label: "delete-currency", Method: "POST", Path: "/api/v1/currency/delete-currency", Auth: "owner", Body: map[string]any{"id": &curID}},
		{Label: "read-after-delete", Method: "GET", Path: "/api/v1/currency/get-currency-list", Auth: "owner", Body: map[string]any{}},
	}
}})
```

Caveats to verify while wiring this: (a) the harness's base currency must be USD and the owner's profile currency must NOT be USD... if it IS USD, the hide-currency call must target a different seeded global (check what the apiparity fixture seeds — if only USD exists, seed/`currency:add`-style insert another global in the scenario setup or pick the guard error as the golden instead: hiding USD then asserts `"The base currency cannot be modified"`, which is also a fine contract to freeze; prefer adding an `err:` prefix label in that case). (b) `CaptureIDInto` extracts `data.item.id` — `CreateCurrencyResult.Item` provides it.

Bump the guard: in `guard_test.go`, `minRoutes = 85` → `minRoutes = 93` (85 + 8).

Regenerate goldens: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ -count=1`
Then: `git diff internal/test/apiparity/testdata/golden/ | head -100` — INSPECT: `currency_reads.golden` changes must be exactly (1) three new fields on every currency item, (2) possibly reordered/expanded rate rows (latest-per-currency + new ORDER BY). Any OTHER golden changing means an unintended behavior change — investigate before committing. New `currency_write_read.golden` appears.

- [ ] **Step 6: Full backend suite + commit**

Run: `make go-test 2>&1 | tail -15`
Expected: PASS including coverage gate and OpenAPI-docs-fresh check.

```bash
git add -A internal/ docs/ && git commit -m "feat(currency): HTTP endpoints for user currency management"
```

---

### Task 8: Backend cross-engine verification

**Files:** none new — this is a verification gate.

- [ ] **Step 1: PostgreSQL repo suite**

Run: `make test-repo-pgsql 2>&1 | tail -10` (auto-provisions Postgres via compose, or set `DATABASE_TEST_PGSQL_URL`).
Expected: PASS. Failures localize to: pgsql migration constraint names (Task 1 Step 4 note), pgsql query variants, or adapter conversion shims.

- [ ] **Step 2: Engine-comparison suite**

Run: `make test 2>&1 | tail -10` (runs go-test + enginecompare + web suite).
Expected: PASS; enginecompare replays the new `currency_write_read` scenario on both engines and asserts byte-identical responses. Known pre-existing failure: `web/src/features/transactions/ImportCsvDialog.test.tsx` fails on main (documented 2026-07-14) — that single failure is NOT caused by this work; everything else must pass.

- [ ] **Step 3: Commit anything the gates required, else proceed**

```bash
git status --short # expect clean; commit fixes if any were needed
```

---

### Task 9: Frontend API layer + queries + picker filtering

**Files:**
- Modify: `web/src/api/dto/currency.ts`, `web/src/api/currency.ts`
- Modify: `web/src/features/currencies/queries.ts`; Create: `web/src/features/currencies/selectable.ts`, `web/src/features/currencies/selectable.test.ts`
- Modify: `web/src/components/CurrencySelect.tsx`, `web/src/components/CurrencyPickerDialog.tsx`
- Modify: `web/src/test/fixtures.ts` (currency fixtures gain the new fields; add mutation handlers to `coreHandlers` if convenient)

**Interfaces:**
- Consumes: Task 7 endpoints.
- Produces:
  - `CurrencyDto` gains `scope: 'global' | 'own' | 'shared'; isArchived: 0 | 1; isHidden: 0 | 1`.
  - `api/currency.ts`: `createCurrency({id, code, name, symbol?, fractionDigits?, rate?}): Promise<CurrencyDto>`, `updateCurrency({id, name, symbol, fractionDigits}): Promise<void>`, `setCurrencyRate({currencyId, rate, date?}): Promise<void>`, `archiveCurrency(id)`, `unarchiveCurrency(id)`, `deleteCurrency(id)`, `hideCurrency(id)`, `showCurrency(id)` — all POST, modeled verbatim on `web/src/api/payee.ts`.
  - `selectableCurrencies(items: CurrencyDto[] | undefined, currentId?: string): CurrencyDto[]` in `web/src/features/currencies/selectable.ts`.
  - Mutation hooks in `features/currencies/queries.ts`: `useCreateCurrency`, `useUpdateCurrency`, `useSetCurrencyRate`, `useArchiveCurrency`, `useUnarchiveCurrency`, `useDeleteCurrency`, `useHideCurrency`, `useShowCurrency` — each invalidates `queryKeys.currencies`; create/set-rate also invalidate `queryKeys.currencyRates`.

- [ ] **Step 1: Write the failing selectable test**

`web/src/features/currencies/selectable.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import type { CurrencyDto } from '@/api/dto/currency'
import { selectableCurrencies } from './selectable'

const cur = (over: Partial<CurrencyDto>): CurrencyDto => ({
  id: 'x', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2,
  scope: 'global', isArchived: 0, isHidden: 0, ...over,
})

describe('selectableCurrencies', () => {
  it('keeps visible globals and own active customs', () => {
    const items = [
      cur({ id: 'usd' }),
      cur({ id: 'eur', code: 'EUR', isHidden: 1 }),
      cur({ id: 'pts', code: 'PTS', scope: 'own' }),
      cur({ id: 'old', code: 'OLD', scope: 'own', isArchived: 1 }),
      cur({ id: 'gem', code: 'GEM', scope: 'shared' }),
    ]
    expect(selectableCurrencies(items).map((c) => c.id)).toEqual(['usd', 'pts'])
  })
  it('keeps the current value even when filtered out', () => {
    const items = [cur({ id: 'usd' }), cur({ id: 'gem', code: 'GEM', scope: 'shared' })]
    expect(selectableCurrencies(items, 'gem').map((c) => c.id)).toEqual(['usd', 'gem'])
  })
  it('handles undefined', () => {
    expect(selectableCurrencies(undefined)).toEqual([])
  })
})
```

Run: `cd web && pnpm test -- --run src/features/currencies/selectable.test.ts` — expected FAIL (module missing).

- [ ] **Step 2: Implement DTO, helper, api functions, hooks**

`web/src/features/currencies/selectable.ts`:

```ts
import type { CurrencyDto } from '@/api/dto/currency'

// Dropdown-eligible currencies: visible globals plus the user's own active
// customs. Foreign (shared-visible) and archived/hidden entries stay out,
// except the entity's current value so an edit form cannot self-corrupt.
export function selectableCurrencies(items: CurrencyDto[] | undefined, currentId?: string): CurrencyDto[] {
  return (items ?? []).filter(
    (c) =>
      c.id === currentId ||
      (c.scope === 'global' && c.isHidden === 0) ||
      (c.scope === 'own' && c.isArchived === 0),
  )
}
```

`api/dto/currency.ts` — extend `CurrencyDto` with the three fields. `api/currency.ts` — add the 8 functions copying `payee.ts`'s envelope idiom (`const response = await api.post<Envelope<{ item: CurrencyDto }>>('/api/v1/currency/create-currency', body); return response.data.data.item` for create; void POSTs for the rest). `features/currencies/queries.ts` — add the hooks (follow `web/src/features/classifications/queries.ts` mutation shape; simple `invalidateQueries` is sufficient — no optimistic cache surgery needed for v1).

Update `web/src/test/fixtures.ts`: `fixtureUsd`/`fixtureEur` gain `scope: 'global', isArchived: 0, isHidden: 0`.

- [ ] **Step 3: Filter the pickers**

In `CurrencySelect.tsx` and `CurrencyPickerDialog.tsx`, wrap the source list: where options are computed from `useCurrencies()` data, apply `selectableCurrencies(currencies, value)` before the existing `fuzzyMatch` filter (`value` is each component's current-selection prop).

- [ ] **Step 4: Run web tests, lint, commit**

Run: `cd web && pnpm test -- --run 2>&1 | tail -10 && pnpm lint`
Expected: PASS except the pre-existing `ImportCsvDialog.test.tsx` failure; lint clean.

```bash
git add web/ && git commit -m "feat(web): currency api mutations + selectable-currencies picker filtering"
```

---

### Task 10: Frontend Settings → Currencies page

**Files:**
- Create: `web/src/features/currencies/CurrenciesPage.tsx`, `web/src/features/currencies/CurrencyDialog.tsx`, `web/src/features/currencies/RateDialog.tsx`, `web/src/features/currencies/CurrenciesPage.test.tsx`
- Modify: `web/src/app/router-pages.ts` (`SETTINGS_CURRENCIES: '/settings/currencies'`), `web/src/app/routes.tsx` (route entry), `web/src/features/settings/SettingsPage.tsx` (MenuRow in the classification group, after Payees), `web/src/locales/en-US.ts` (`modules.classifications.currencies.*`)

**Interfaces:**
- Consumes: Task 9 hooks + `useCurrencies()`/`useCurrencyRates()`; `SettingsShell`, `ResponsiveDialog`, `ConfirmDialog`, `Switch`, `DropdownMenu`, `CardField` (all existing); `useUserData()` + `userCurrencyId()` from `features/user/queries`.
- Produces: the page at `/settings/currencies`.

- [ ] **Step 1: Write failing page tests**

`CurrenciesPage.test.tsx` following `web/src/features/classifications/PayeesTagsPages.test.tsx` (renderPage helper, `mockViewport`, `coreHandlers` overrides). Cases:

```
renders My currencies and Global currencies sections; own customs show name+code+rate label
create flow: open "Create currency" dialog, fill code/name/rate, POST body includes uuidv7 id + code "PTS" (uppercased), list invalidated
archive toggle on an own custom posts archive-currency
delete flow: kebab -> Delete -> ConfirmDialog -> delete-currency posted; server 400 "Currency is in use and cannot be deleted" surfaces as visible error text
hide/show switch on a global posts hide-currency / show-currency
base currency row's visibility switch is disabled (derive base from rates' baseCurrencyId)
profile currency row's visibility switch is disabled (from userCurrencyId(user))
set-rate dialog posts {currencyId, rate, date?}
```

Run: `cd web && pnpm test -- --run src/features/currencies/CurrenciesPage.test.tsx` — expected FAIL.

- [ ] **Step 2: Implement the page + dialogs**

`CurrenciesPage.tsx` structure (inside `SettingsShell title/heading/backTo={RouterPage.SETTINGS}` with a "Create currency" action button, like the classification pages):

- Data: `const { data: currencies } = useCurrencies()`, `const { data: rates } = useCurrencyRates()`, `const { data: user } = useUserData()`.
- Derivations: `own = currencies?.filter(c => c.scope === 'own') ?? []`; `globals = currencies?.filter(c => c.scope === 'global') ?? []`; `baseId = rates?.[0]?.baseCurrencyId`; `profileId = userCurrencyId(user)`; `rateFor(id) = rates?.find(r => r.currencyId === id)`.
- **My currencies** section: one row per own custom — name, `code · symbol`, rate caption (`1 {baseCode} = {rate} {code}` when a rate exists), archive `Switch` (`aria-label={'archive ' + c.name}`), kebab `DropdownMenu` (`aria-label={'actions ' + c.name}`) with Edit / Set rate / destructive Delete. Empty state paragraph: t(`...empty_state`).
- **Global currencies** section: one row per global — name/code, visibility `Switch` (`checked={c.isHidden === 0}`, `aria-label={'show ' + c.name}`), `disabled={c.id === baseId || c.id === profileId}` with a `title` tooltip.
- Dialogs: `CurrencyDialog` (create/edit; fields code [create only, uppercased on input, maxLength 3], name, symbol, fractionDigits [number 0-8], rate [create only, optional]) modeled on `CategoryDialog.tsx` (`ResponsiveDialog` + form id + footer submit). `RateDialog` (rate + optional date input `type="date"`). Delete via existing `ConfirmDialog`. Create submits `{ id: uuidv7(), code, name, symbol: symbol || undefined, fractionDigits, rate: rate || undefined }`.
- Mutation error surfacing: `onError` of delete/create shows the API envelope `message` (check how classification pages surface errors — reuse that mechanism; if none, render the mutation error text near the section).

Routing/menu/i18n:

```ts
// router-pages.ts
SETTINGS_CURRENCIES: '/settings/currencies',
// routes.tsx (settings children)
{ path: '/settings/currencies', element: <CurrenciesPage /> },
```

```tsx
// SettingsPage.tsx classification group, after Payees
<MenuRow label={t('modules.classifications.currencies.pages.settings.menu_item')} to={RouterPage.SETTINGS_CURRENCIES} />
```

`en-US.ts` under `modules.classifications` (sibling of `payees`):

```ts
'currencies': {
  'pages': {
    'settings': {
      'menu_item': 'Currencies',
      'header': 'Currencies',
      'create_currency': 'Create currency',
      'my_currencies': 'My currencies',
      'global_currencies': 'Global currencies',
      'archived_item': 'Archived',
      'empty_state': 'Create your own currency, like "Points", set its exchange rate, and use it for accounts and budgets.',
      'rate_caption': '1 {base} = {rate} {code}',
      'locked_base': 'The base currency is always visible',
      'locked_profile': 'Your profile currency is always visible',
    },
  },
  'modals': {
    'create': { 'header': 'New currency' },
    'edit': { 'header': 'Edit currency' },
    'rate': { 'header': 'Set exchange rate', 'submit': 'Save rate' },
    'delete': { 'title': 'Delete currency?', 'question': 'Are you sure you want to delete "{name}"?' },
  },
  'forms': {
    'currency': {
      'code': { 'label': 'Code' },
      'name': { 'label': 'Name', 'validation': { 'required_field': 'Required field' } },
      'symbol': { 'label': 'Symbol' },
      'fraction_digits': { 'label': 'Decimal places' },
      'rate': { 'label': 'Exchange rate' },
      'date': { 'label': 'Date' },
    },
  },
},
```

- [ ] **Step 3: Run tests + lint + build, commit**

Run: `cd web && pnpm test -- --run 2>&1 | tail -10 && pnpm lint && pnpm build 2>&1 | tail -3`
Expected: tests PASS (modulo the pre-existing ImportCsvDialog failure), lint clean, build succeeds.

```bash
git add web/ && git commit -m "feat(web): Settings -> Currencies page (custom currencies, rates, global visibility)"
```

---

### Task 11: Final verification + PR update

- [ ] **Step 1: Full suite**

Run: `make test 2>&1 | tail -15`
Expected: green across go-test, pgsql repo suite, enginecompare, and the web suite (pre-existing ImportCsvDialog failure excepted).

- [ ] **Step 2: End-to-end sanity (manual, scripted)**

Boot the server against a scratch sqlite DB and drive the happy path with curl: register/login (or `user:create` + login), `create-currency` (PTS, rate 100), `get-currency-list` (PTS present, scope `own`), create an account in PTS, `hide-currency` on a non-base global, verify `get-currency-list` marks it hidden, `delete-currency` on PTS → expect `"Currency is in use and cannot be deleted"`. This validates the wiring the test doubles can't.

- [ ] **Step 3: Push and update PR #92**

```bash
git push origin feat/user-currencies
gh pr view 92 --json title # PR already exists for the spec; the implementation lands on the same branch
```

Add a PR comment summarizing the implementation and the golden-file diffs (which encode the observable behavior changes: new currency fields, per-currency-latest rates).

---

## Plan self-review notes (already applied)

- Spec coverage: every spec section maps to a task (data model → 1; API → 2/4/5/7; read path → 3; conversion → no-op by design; denomination validation → 6; frontend → 9/10; error handling → 4/5/6 exact strings; testing → per-task + 8/11).
- The spec's `operationId?` became the house-convention `id` field (flagged in Global Constraints).
- The spec's "usable = global or own non-archived" is enforced in `EnsureUsable`; change-only checks on update paths implement the spec's "existing references are never re-validated".
- Type consistency: `ManageModel`/`ProfileCurrency`/`EnsureUsable`/`GetIDByCodeForUser` names match across Tasks 2/4/5/6/7; `CurrencyRecord` fields match the generated row shape; frontend `CurrencyDto` fields match `CurrencyResult` JSON tags.
