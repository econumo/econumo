# Delete a custom currency (soft delete) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a user delete their own custom currency even when the only thing still referencing it is a soft-deleted account, by making currency deletion a soft delete.

**Architecture:** Add `currencies.is_deleted`. `DeleteCurrency` keeps refusing while a **live** entity uses the currency (accounts with `is_deleted = 0`, budgets, budget elements, profile defaults) and otherwise sets the flag instead of removing the row. The flag travels to clients as `isDeleted` and each consumer filters for itself — pickers and the settings list exclude it, while account hydration and the rate list deliberately do not.

**Tech Stack:** Go 1.x (stdlib `net/http`, sqlc, SQLite via `modernc.org/sqlite`, PostgreSQL via `pgx/v5`), React 19 + TanStack Query + vitest.

**Spec:** `docs/superpowers/specs/2026-08-02-delete-custom-currency-design.md`

## Global Constraints

- **Currencies are NEVER hard-deleted.** No code path may remove a row from `currencies`. `accounts.currency_id` is `ON DELETE CASCADE` and `transactions.account_id` cascades from it, so a `DELETE FROM currencies` destroys account and transaction history. Deletion always means `is_deleted = 1`.
- **A deleted currency must keep loading for entities that still hold it.** Never filter `is_deleted` inside `GetUserCurrencyListView` or any by-id read. `GetCurrencyRateList` gates its output on list-view membership, and clients treat a missing rate row as "fall back to 1:1" — filtering at the source silently produces wrong totals, not a missing label.
- **Only the owner's own customs are deletable.** Globals (`user_id IS NULL`) and foreign customs already get 403 from `ownedRecord` (`internal/currency/manage.go:111`). Do not weaken that.
- Every SQL change is made in **both** `query/sqlite/` and `query/pgsql/`, then `sqlc generate` is run. Never hand-edit files under `internal/infra/storage/sqlc/gen/`.
- **sqlc `.sql` comments must be ASCII only.** An em dash in a query comment corrupts sqlite codegen (byte-offset bug in sqlc v1.30). Use `--` or plain hyphens.
- Comments: only non-obvious rationale and frozen-contract reasoning. No godoc restating a name, no section dividers, no references to the former PHP implementation.
- Run tests from the repo root. `make go-test` is the smoke gate; `make web-test` and `pnpm exec tsc -b` gate the frontend.

---

### Task 1: Migration — `currencies.is_deleted` + the rewritten partial index

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260802000000.sql`
- Create: `internal/infra/storage/migrations/pgsql/20260802000000.sql`
- Modify: `internal/test/fixture/entities.go:146-179` (the `Currency` builder)
- Test: `internal/infra/storage/migrations/migration_20260802_test.go`

**Interfaces:**
- Produces: the column `currencies.is_deleted` (`BOOLEAN NOT NULL`, default false) on both engines; the index `UNIQ_currencies_user_code` re-created with the predicate `user_id IS NOT NULL AND is_deleted = 0`; `fixture.Currency{Deleted bool}` for later tasks to seed deleted rows.

- [ ] **Step 1: Write the failing migration test**

Create `internal/infra/storage/migrations/migration_20260802_test.go`. It reuses `openFK`, `toRunList` and `seedUser` from `migration_20260730_test.go` — same `migrations_test` package, so no imports of those helpers are needed.

```go
package migrations_test

// Verifies 20260802000000: the is_deleted column defaults to not-deleted for
// existing rows, and the rewritten partial unique index frees a deleted row's
// (user_id, code) pair while still rejecting duplicates among live rows.

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/migrate"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
	_ "modernc.org/sqlite"
)

const softDeleteVersion = "20260802000000"

func TestMigration20260802_BackfillsNotDeletedAndFreesDeletedCodes(t *testing.T) {
	db := openFK(t)
	ctx := context.Background()
	all := toRunList(migrations.SQLite())
	var before []migrate.Migration
	for _, m := range all {
		if m.Version < softDeleteVersion {
			before = append(before, m)
		}
	}
	if err := migrate.Run(ctx, db, before); err != nil {
		t.Fatalf("pre-migrations: %v", err)
	}
	seedUser(t, db, "u1")
	if _, err := db.ExecContext(ctx, `INSERT INTO currencies (id, code, symbol, name, fraction_digits, user_id, rate, created_at)
		VALUES ('c1', 'PTS', 'P', NULL, 2, 'u1', '10.00000000', '2026-01-01 00:00:00')`); err != nil {
		t.Fatalf("seed custom: %v", err)
	}

	if err := migrate.Run(ctx, db, all); err != nil {
		t.Fatalf("target migration: %v", err)
	}

	var deleted bool
	if err := db.QueryRowContext(ctx, `SELECT is_deleted FROM currencies WHERE id = 'c1'`).Scan(&deleted); err != nil {
		t.Fatalf("scan is_deleted: %v", err)
	}
	if deleted {
		t.Fatal("existing row came out deleted; the column must default to not-deleted")
	}

	// A live duplicate is still rejected.
	if _, err := db.ExecContext(ctx, `INSERT INTO currencies (id, code, symbol, name, fraction_digits, user_id, rate, created_at, is_deleted)
		VALUES ('c2', 'PTS', 'P', NULL, 2, 'u1', '10.00000000', '2026-01-01 00:00:00', 0)`); err == nil {
		t.Fatal("duplicate live (user_id, code) was accepted; the partial unique index is not enforcing")
	}

	// Once the original is deleted, the code is free again.
	if _, err := db.ExecContext(ctx, `UPDATE currencies SET is_deleted = 1 WHERE id = 'c1'`); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO currencies (id, code, symbol, name, fraction_digits, user_id, rate, created_at, is_deleted)
		VALUES ('c2', 'PTS', 'P', NULL, 2, 'u1', '10.00000000', '2026-01-01 00:00:00', 0)`); err != nil {
		t.Fatalf("re-creating a deleted code must succeed: %v", err)
	}

	// Two users may still share a code.
	seedUser(t, db, "u2")
	if _, err := db.ExecContext(ctx, `INSERT INTO currencies (id, code, symbol, name, fraction_digits, user_id, rate, created_at, is_deleted)
		VALUES ('c3', 'PTS', 'P', NULL, 2, 'u2', '10.00000000', '2026-01-01 00:00:00', 0)`); err != nil {
		t.Fatalf("cross-user same code must succeed: %v", err)
	}
}
```

- [ ] **Step 2: Run it to confirm it fails**

Run: `go test ./internal/infra/storage/migrations/ -run TestMigration20260802 -v`
Expected: FAIL — `no such column: is_deleted`.

- [ ] **Step 3: Write the SQLite migration**

Create `internal/infra/storage/migrations/sqlite/20260802000000.sql`:

```sql
-- Soft delete for currencies. Deletion never removes the row: accounts.currency_id
-- cascades and transactions.account_id cascades from there, so a DELETE would take
-- account and transaction history with it. ADD COLUMN with a constant default needs
-- no table rebuild, so no foreign_keys pragma juggling here.
ALTER TABLE currencies ADD COLUMN is_deleted BOOLEAN DEFAULT '0' NOT NULL;

-- The per-user code index excludes deleted rows so an owner can re-create a code
-- they deleted. The global index is deliberately left alone: re-adding a retired
-- global clears its flag instead of inserting a second row.
DROP INDEX UNIQ_currencies_user_code;
CREATE UNIQUE INDEX UNIQ_currencies_user_code
    ON currencies (user_id, code) WHERE user_id IS NOT NULL AND is_deleted = 0;
```

- [ ] **Step 4: Write the PostgreSQL migration**

Create `internal/infra/storage/migrations/pgsql/20260802000000.sql`:

```sql
-- Soft delete for currencies. See the sqlite sibling for the semantics.
ALTER TABLE currencies ADD COLUMN is_deleted BOOLEAN NOT NULL DEFAULT false;

DROP INDEX UNIQ_currencies_user_code;
CREATE UNIQUE INDEX UNIQ_currencies_user_code
    ON currencies (user_id, code) WHERE user_id IS NOT NULL AND is_deleted = false;
```

- [ ] **Step 5: Run the migration test to verify it passes**

Run: `go test ./internal/infra/storage/migrations/ -run TestMigration20260802 -v`
Expected: PASS.

- [ ] **Step 6: Teach the fixture builder to seed deleted currencies**

In `internal/test/fixture/entities.go`, add the field to the `Currency` struct (after `Rate`):

```go
	Rate           string // fixed rate for customs (e.g. "10.00000000"); empty -> NULL
	Deleted        bool   // seeds a soft-deleted currency row
```

Then in `func (b *Builder) Currency(c Currency) string`, replace the two `b.insert(...)` calls with a form that carries the flag. Both engines take `TRUE`/`FALSE` literals, matching how the `Account` builder handles `is_deleted`:

```go
	deleted := "FALSE"
	if c.Deleted {
		deleted = "TRUE"
	}
	now := b.now()
	if c.Name == "" {
		b.insert(`INSERT INTO currencies (id, code, symbol, name, fraction_digits, user_id, rate, created_at, is_deleted) VALUES (?, ?, ?, NULL, ?, ?, ?, ?, `+deleted+`)`,
			id, c.Code, c.Symbol, digits, nullable(c.UserID), nullable(c.Rate), now)
	} else {
		b.insert(`INSERT INTO currencies (id, code, symbol, name, fraction_digits, user_id, rate, created_at, is_deleted) VALUES (?, ?, ?, ?, ?, ?, ?, ?, `+deleted+`)`,
			id, c.Code, c.Symbol, c.Name, digits, nullable(c.UserID), nullable(c.Rate), now)
	}
	return id
```

- [ ] **Step 7: Verify the whole suite still builds and passes**

Run: `go build ./... && go test ./internal/infra/storage/... ./internal/test/...`
Expected: PASS. The fixture change is additive, so no existing test should need edits.

- [ ] **Step 8: Commit**

```bash
git add internal/infra/storage/migrations/sqlite/20260802000000.sql \
        internal/infra/storage/migrations/pgsql/20260802000000.sql \
        internal/infra/storage/migrations/migration_20260802_test.go \
        internal/test/fixture/entities.go
git commit -m "feat: add currencies.is_deleted and scope the per-user code index to live rows"
```

---

### Task 2: Repository layer — soft delete, live-only usage, deleted-aware lookups

**Files:**
- Modify: `internal/infra/storage/sqlc/query/sqlite/currency_write.sql:61-90`
- Modify: `internal/infra/storage/sqlc/query/pgsql/currency_write.sql:53-79`
- Modify: `internal/infra/storage/sqlc/query/sqlite/currency_read.sql:6-30`
- Modify: `internal/infra/storage/sqlc/query/pgsql/currency_read.sql:5-29`
- Modify: `internal/infra/storage/sqlc/query/sqlite/currencies.sql:4-8`
- Modify: `internal/infra/storage/sqlc/query/pgsql/currencies.sql:4-8`
- Modify: `internal/model/currency.go:58-69` (`CurrencyRecord`)
- Modify: `internal/model/currency_view.go:11-22` (`CurrencyViewRow`)
- Modify: `internal/currency/manage.go:20-35` (`ManageModel` port)
- Modify: `internal/currency/repo/manage.go` (querier interface + both adapters + `ManageRepo` methods)
- Modify: `internal/currency/repo/read.go` (map the new column)
- Test: `internal/currency/repo/manage_integration_test.go`

**Interfaces:**
- Consumes: `currencies.is_deleted` and `fixture.Currency{Deleted bool}` from Task 1.
- Produces:
  - `model.CurrencyRecord.IsDeleted bool`
  - `model.CurrencyViewRow.IsDeleted bool`
  - `ManageModel.SoftDeleteCurrency(ctx context.Context, id string) error` — replaces `DeleteCurrency`
  - `ManageModel.CountCurrencyUsage(ctx, id) (int64, error)` — unchanged signature, now counts LIVE references only

- [ ] **Step 1: Rewrite the existing delete assertion**

`TestManageRepo_InsertGetUpdateDelete` (`internal/currency/repo/manage_integration_test.go:22`) currently ends by asserting the record is NotFound after `DeleteCurrency`. That assertion encodes the behaviour this plan removes, so it changes rather than being preserved. Replace its final block:

```go
	if err := r.SoftDeleteCurrency(ctx, rec.ID); err != nil {
		t.Fatal(err)
	}
	got, err = r.GetCurrencyRecord(ctx, rec.ID)
	if err != nil {
		t.Fatalf("the row must survive a delete (currencies are never hard-deleted): %v", err)
	}
	if !got.IsDeleted {
		t.Fatal("IsDeleted = false after SoftDeleteCurrency")
	}
```

The `errs` import may become unused once `errs.AsNotFound` goes — check before committing.

- [ ] **Step 2: Write the failing repo integration tests**

Append to the same file. The package helper is `newManage(t)`, which returns `(*currencyrepo.ManageRepo, *dbtest.DB, *fixture.Builder)`; the existing tests build `ctx` and `uid` inline, so these follow suit.

```go
func TestManageRepo_UsageCountIgnoresDeletedAccounts(t *testing.T) {
	r, _, f := newManage(t)
	ctx := context.Background()
	uid := f.User(fixture.User{Name: "A"})
	cid := f.Currency(fixture.Currency{Code: "PTS", UserID: uid})
	f.Account(fixture.Account{UserID: uid, CurrencyID: cid, Name: "Dead", Deleted: true})

	if n, _ := r.CountCurrencyUsage(ctx, cid); n != 0 {
		t.Fatalf("usage = %d, want 0: a soft-deleted account is not live usage", n)
	}

	f.Account(fixture.Account{UserID: uid, CurrencyID: cid, Name: "Live"})
	if n, _ := r.CountCurrencyUsage(ctx, cid); n != 1 {
		t.Fatalf("usage = %d, want 1 for a live account", n)
	}
}

func TestManageRepo_OwnerCodeExistsIgnoresDeleted(t *testing.T) {
	r, _, f := newManage(t)
	ctx := context.Background()
	uid := f.User(fixture.User{Name: "A"})
	f.Currency(fixture.Currency{Code: "PTS", UserID: uid, Deleted: true})

	if ok, _ := r.OwnerCodeExists(ctx, uid, "PTS"); ok {
		t.Fatal("a deleted currency must not block re-creating its code")
	}
}
```

- [ ] **Step 3: Run them to confirm they fail**

Run: `go test ./internal/currency/repo/ -run 'TestManageRepo_(InsertGetUpdateDelete|UsageCountIgnoresDeleted|OwnerCodeExistsIgnoresDeleted)' -v`
Expected: FAIL to compile — `r.SoftDeleteCurrency` and `got.IsDeleted` are undefined.

- [ ] **Step 4: Update the SQLite queries**

In `internal/infra/storage/sqlc/query/sqlite/currency_write.sql`:

```sql
-- name: GetCurrencyRecord :one
SELECT id, code, symbol, name, fraction_digits, user_id, rate, created_at, is_deleted
FROM currencies WHERE id = ?;

-- name: GlobalCurrencyCodeExists :one
SELECT COUNT(*) FROM currencies WHERE code = ? AND user_id IS NULL;

-- Deleted customs release their code, so they must not block a re-create.
-- name: OwnerCurrencyCodeExists :one
SELECT COUNT(*) FROM currencies WHERE code = ? AND user_id = ? AND is_deleted = 0;
```

Replace the `DeleteCurrency` query outright:

```sql
-- Currencies are never removed: accounts.currency_id and transactions.account_id
-- both cascade, so a DELETE would destroy account and transaction history.
-- name: SoftDeleteCurrency :exec
UPDATE currencies SET is_deleted = 1 WHERE id = ?;
```

And scope the usage census to live references:

```sql
-- Usage census for delete protection. Only LIVE references count: a soft-deleted
-- account is unreachable and unrestorable, so it must not pin a currency forever.
-- name: CountCurrencyUsage :one
SELECT (SELECT COUNT(*) FROM accounts WHERE accounts.currency_id = ? AND accounts.is_deleted = 0)
     + (SELECT COUNT(*) FROM budgets WHERE budgets.currency_id = ?)
     + (SELECT COUNT(*) FROM budgets_elements WHERE budgets_elements.currency_id = ?)
     + (SELECT COUNT(*) FROM users_options WHERE users_options.name = 'currency' AND users_options.value = ?) AS usage_count;
```

In `internal/infra/storage/sqlc/query/sqlite/currency_read.sql`, add the column to the list view's SELECT (the WHERE clause is deliberately NOT filtered — see Global Constraints):

```sql
SELECT c.id, c.code, c.symbol, c.name, c.fraction_digits, c.user_id, c.rate, c.created_at, c.is_deleted
FROM currencies c
```

In `internal/infra/storage/sqlc/query/sqlite/currencies.sql`:

```sql
-- name: GetCurrencyIDByCodeForUser :one
-- Deleted rows are excluded: an owner may hold a live and a deleted currency with
-- the same code, and LIMIT 1 would otherwise pick between them arbitrarily.
SELECT id FROM currencies
WHERE code = ? AND (user_id IS NULL OR user_id = ?) AND is_deleted = 0
ORDER BY (user_id IS NULL) ASC
LIMIT 1;
```

- [ ] **Step 5: Mirror every change in the PostgreSQL queries**

Same edits in `query/pgsql/currency_write.sql`, `query/pgsql/currency_read.sql` and `query/pgsql/currencies.sql`, using `$N` placeholders and `false` for the boolean. The pgsql `CountCurrencyUsage` reuses `$1` throughout, so it keeps a single generated param:

```sql
-- name: SoftDeleteCurrency :exec
UPDATE currencies SET is_deleted = true WHERE id = $1;

-- name: CountCurrencyUsage :one
SELECT (SELECT COUNT(*) FROM accounts WHERE accounts.currency_id = $1 AND accounts.is_deleted = false)
     + (SELECT COUNT(*) FROM budgets WHERE budgets.currency_id = $1)
     + (SELECT COUNT(*) FROM budgets_elements WHERE budgets_elements.currency_id = $1)
     + (SELECT COUNT(*) FROM users_options WHERE users_options.name = 'currency' AND users_options.value = $1) AS usage_count;

-- name: OwnerCurrencyCodeExists :one
SELECT COUNT(*) FROM currencies WHERE code = $1 AND user_id = $2 AND is_deleted = false;

-- name: GetCurrencyIDByCodeForUser :one
SELECT id FROM currencies
WHERE code = $1 AND (user_id IS NULL OR user_id = $2) AND is_deleted = false
ORDER BY (user_id IS NULL) ASC
LIMIT 1;
```

- [ ] **Step 6: Regenerate sqlc**

Run: `sqlc generate -f internal/infra/storage/sqlc/sqlc.yaml`
Expected: `gen/sqlite` and `gen/pgsql` pick up `IsDeleted bool` on `GetCurrencyRecordRow` and `GetUserCurrencyListViewRow`, gain `SoftDeleteCurrency`, and drop `DeleteCurrency`.

- [ ] **Step 7: Add the model fields**

`internal/model/currency.go`, in `CurrencyRecord` after `CreatedAt`:

```go
	CreatedAt time.Time
	// IsDeleted marks a soft-deleted custom currency: it stays resolvable for
	// the entities that still reference it, but leaves every picker and list.
	IsDeleted bool
```

`internal/model/currency_view.go`, in `CurrencyViewRow` after `CreatedAt`:

```go
	CreatedAt time.Time
	IsDeleted bool
```

- [ ] **Step 8: Rename the port method and update the repo adapters**

In `internal/currency/manage.go`, in the `ManageModel` interface, replace the `DeleteCurrency` line:

```go
	SoftDeleteCurrency(ctx context.Context, id string) error
```

In `internal/currency/repo/manage.go`, rename in the `manageQuerier` interface:

```go
	SoftDeleteCurrency(ctx context.Context, db backend.DBTX, id string) error
```

Rename the `ManageRepo` method and both engine adapters' methods from `DeleteCurrency` to `SoftDeleteCurrency`, calling the regenerated `SoftDeleteCurrency` query. Map `IsDeleted` wherever `currencyRecordRow` is converted into `model.CurrencyRecord`, and in `internal/currency/repo/read.go` wherever the list-view row is converted into `model.CurrencyViewRow`.

- [ ] **Step 9: Update the in-memory fake so the package still builds**

In `internal/currency/manage_test.go`, replace `fakeManageRepo.DeleteCurrency` — it currently removes the record, which now violates the never-hard-delete invariant:

```go
func (f *fakeManageRepo) SoftDeleteCurrency(ctx context.Context, id string) error {
	rec, ok := f.records[id]
	if !ok {
		return errs.NewNotFound("Currency not found")
	}
	rec.IsDeleted = true
	f.records[id] = rec
	return nil
}
```

- [ ] **Step 10: Run the tests**

Run: `go build ./... && go test ./internal/currency/... ./internal/model/...`
Expected: PASS, including the three new repo tests.

- [ ] **Step 11: Commit**

```bash
git add internal/infra/storage/sqlc internal/model internal/currency
git commit -m "feat: soft-delete currencies in the repo layer and count only live usage"
```

---

### Task 3: Service layer — delete soft, reject deleted, report the flag

**Files:**
- Modify: `internal/currency/manage_lifecycle.go:52-70` (`DeleteCurrency`)
- Modify: `internal/currency/repo/lookup.go:86-107` (`EnsureUsable`)
- Modify: `internal/currency/read.go:56-93` (`GetCurrencyList`)
- Modify: `internal/model/currency_dto.go:23-33` (`CurrencyListItem`)
- Test: `internal/currency/manage_test.go`, `internal/currency/read_scoping_test.go`, `internal/currency/repo/lookup_read_integration_test.go`

**Interfaces:**
- Consumes: `ManageModel.SoftDeleteCurrency`, `model.CurrencyRecord.IsDeleted`, `model.CurrencyViewRow.IsDeleted` from Task 2.
- Produces: `model.CurrencyListItem.IsDeleted int` with JSON tag `isDeleted` (values `0`/`1`, matching the `isHidden` convention).

- [ ] **Step 1: Rewrite the existing happy-path assertion**

`TestDeleteCurrency_HappyPath` (`internal/currency/manage_test.go:504`) asserts the record is gone from `repo.records`. That is the behaviour being replaced. Rewrite its tail:

```go
	if _, err := svc.DeleteCurrency(context.Background(), uid, model.DeleteCurrencyRequest{Id: "own-pts"}); err != nil {
		t.Fatalf("DeleteCurrency: %v", err)
	}
	rec, ok := repo.records["own-pts"]
	if !ok {
		t.Fatal("the row was removed; currencies are never hard-deleted")
	}
	if !rec.IsDeleted {
		t.Fatal("IsDeleted = false after a successful delete")
	}
```

`TestDeleteCurrency_RefusesWhenUsed` (`:484`) already asserts the record survives; strengthen it by adding, after the existing check:

```go
	if repo.records["own-pts"].IsDeleted {
		t.Error("a refused delete must not set the flag")
	}
```

- [ ] **Step 2: Write the failing service tests**

In `internal/currency/manage_test.go`, following the file's style (`newFakeManageRepo()`, `newManageSvc(repo, &fakeOps{}, manageNow)`, `strPtr`, `manageMeID`, `manageOtherID`, `vo.MustParseId`):

```go
// A currency whose only references are soft-deleted accounts reaches the
// service with usage 0 -- the census counts live references only -- so this is
// the case the bug report describes.
func TestDeleteCurrency_GlobalIsNeverDeletable(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["global-eur"] = model.CurrencyRecord{ID: "global-eur", Code: "EUR", Symbol: "E"}
	svc := newManageSvc(repo, &fakeOps{}, manageNow)

	_, err := svc.DeleteCurrency(context.Background(), vo.MustParseId(manageMeID), model.DeleteCurrencyRequest{Id: "global-eur"})
	if err == nil {
		t.Fatal("a global currency must never be deletable")
	}
	if repo.records["global-eur"].IsDeleted {
		t.Fatal("a refused delete must not set the flag")
	}
}

func TestDeleteCurrency_ForeignCustomIsNeverDeletable(t *testing.T) {
	repo := newFakeManageRepo()
	repo.records["foreign-pts"] = model.CurrencyRecord{ID: "foreign-pts", Code: "GEM", Symbol: "GEM", UserID: strPtr(manageOtherID)}
	svc := newManageSvc(repo, &fakeOps{}, manageNow)

	_, err := svc.DeleteCurrency(context.Background(), vo.MustParseId(manageMeID), model.DeleteCurrencyRequest{Id: "foreign-pts"})
	if err == nil {
		t.Fatal("another user's currency must never be deletable")
	}
	if repo.records["foreign-pts"].IsDeleted {
		t.Fatal("a refused delete must not set the flag")
	}
}
```

In `internal/currency/repo/lookup_read_integration_test.go`, following its inline setup style (`db := dbtest.New(t)`, `currencyrepo.New(db.Engine, db.TX)`, `currencyrepo.NewReadRepo(db.Engine, db.TX)`, `fixture.New(t, db)`):

```go
func TestLookup_EnsureUsableRejectsDeletedOwnCustom(t *testing.T) {
	db := dbtest.New(t)
	lk := currencyrepo.New(db.Engine, db.TX)
	f := fixture.New(t, db)
	ctx := context.Background()
	uid := f.User(fixture.User{Name: "A"})
	cid := f.Currency(fixture.Currency{Code: "PTS", UserID: uid, Deleted: true})

	if err := lk.EnsureUsable(ctx, uid, cid); err == nil {
		t.Fatal("a deleted currency must not be usable for a NEW entity")
	}
}

// The list view feeds BOTH get-currency-list and the rate list's visibility
// filter, so dropping deleted rows here would strip an account's currency AND
// its exchange rate -- and clients fall back to 1:1 on a missing rate.
func TestCurrencyReadRepo_ListViewKeepsDeletedRows(t *testing.T) {
	db := dbtest.New(t)
	read := currencyrepo.NewReadRepo(db.Engine, db.TX)
	f := fixture.New(t, db)
	ctx := context.Background()
	uid := f.User(fixture.User{Name: "A"})
	cid := f.Currency(fixture.Currency{Code: "PTS", UserID: uid, Rate: "10.00000000", Deleted: true})

	rows, err := read.UserCurrencyListView(ctx, uid)
	if err != nil {
		t.Fatalf("UserCurrencyListView: %v", err)
	}
	for _, r := range rows {
		if r.ID == cid {
			if !r.IsDeleted {
				t.Fatal("IsDeleted did not survive the read")
			}
			return
		}
	}
	t.Fatal("deleted currency missing from the list view: accounts holding it would render at 1:1")
}
```

- [ ] **Step 3: Run them to confirm they fail**

Run: `go test ./internal/currency/... -run 'DeleteCurrency_|EnsureUsableRejectsDeleted|ListViewKeepsDeleted' -v`
Expected: FAIL — the fake still removes the record and `EnsureUsable` still accepts the currency.

- [ ] **Step 4: Make the delete soft**

In `internal/currency/manage_lifecycle.go`, replace the final line of the transaction body in `DeleteCurrency`:

```go
		return s.repo.SoftDeleteCurrency(ctx, rec.ID)
```

Leave the `ownedRecord` call and the `used > 0` guard exactly as they are — the guard now reads as "live usage" because Task 2 changed the query underneath it.

- [ ] **Step 5: Reject deleted currencies in `EnsureUsable`**

In `internal/currency/repo/lookup.go`, replace the stale doc line "Hidden currencies stay usable: hiding is a picker preference, not a lock." and add the check after the row is loaded:

```go
// EnsureUsable reports whether the user may denominate a NEW entity in the
// currency: a global, or their own live custom. Hiding stays a picker
// preference and does not lock; deletion does -- existing entities keep
// resolving a deleted currency, but nothing new may be created in it.
func (l *Lookup) EnsureUsable(ctx context.Context, userID, currencyID string) error {
	row, err := l.q.GetCurrencyRecord(ctx, l.tx.Querier(ctx), currencyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.NewNotFound("Currency not found")
		}
		return err
	}
	if row.IsDeleted {
		return errs.NewValidation("Validation failed",
			errs.FieldError{Key: "currencyId", Message: "Currency is not available", Code: errs.CodeCurrencyNotAvailable})
	}
	if row.UserID == nil {
		return nil
	}
	if *row.UserID == userID {
		return nil
	}
	return errs.NewValidation("Validation failed",
		errs.FieldError{Key: "currencyId", Message: "Currency is not available", Code: errs.CodeCurrencyNotAvailable})
}
```

- [ ] **Step 6: Put the flag on the wire**

In `internal/model/currency_dto.go`, extend `CurrencyListItem` and correct the stale comment (it currently mentions `isArchived`, which the type does not have):

```go
// CurrencyListItem is a get-currency-list item: the shared currency shape plus
// the caller-relative flags. Anonymous embedding keeps the JSON flat.
// scope is "global" | "own" | "shared"; isHidden/isDeleted are ints 0/1.
// A deleted currency is still listed -- accounts and rates that reference it
// must keep resolving -- so consumers filter on the flag themselves.
type CurrencyListItem struct {
	CurrencyResult
	Scope     string `json:"scope"`
	IsHidden  int    `json:"isHidden"`
	IsDeleted int    `json:"isDeleted"`
}
```

In `internal/currency/read.go`, inside the `GetCurrencyList` loop, set it alongside `isHidden`:

```go
		isDeleted := 0
		if r.IsDeleted {
			isDeleted = 1
		}
		items = append(items, model.CurrencyListItem{
			CurrencyResult: model.CurrencyResult{
				Id:             r.ID,
				Code:           r.Code,
				Name:           currencyName(r),
				Symbol:         r.Symbol,
				FractionDigits: int(r.FractionDigits),
			},
			Scope:     scope,
			IsHidden:  isHidden,
			IsDeleted: isDeleted,
		})
```

Do NOT touch `GetCurrencyRateList` — its rate output must keep including deleted currencies.

- [ ] **Step 7: Run the tests**

Run: `go test ./internal/currency/... ./internal/model/...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/currency internal/model
git commit -m "feat: delete a custom currency softly and report isDeleted on the wire"
```

---

### Task 4: API surface — OpenAPI, parity scenario, goldens

**Files:**
- Modify: `internal/test/apiparity/catalogue.go` (currency scenarios)
- Modify: `internal/test/apiparity/testdata/golden/currency_write_read.golden` (regenerated)
- Modify: `internal/test/mcpparity/testdata/golden/reference_tools.golden` (regenerated)
- Modify: `docs/` OpenAPI output (regenerated by `make swagger`)

**Interfaces:**
- Consumes: the `isDeleted` field from Task 3.

- [ ] **Step 1: Add the parity scenario**

The whole sequence is expressible as API calls, so no fixture access is needed. In `internal/test/apiparity/catalogue.go`, register a new scenario next to `currency_write_read` (the `Call` struct, `CaptureIDInto` and the `"owner"` auth role are all used exactly as that scenario uses them):

```go
	// The bug this guards: an account soft-deleted by the owner used to pin its
	// currency forever, because the usage census counted dead accounts. The
	// currency stays LISTED after deletion (with isDeleted 1) so accounts that
	// still reference it keep resolving their symbol and rate.
	register(Scenario{Name: "currency_delete_after_account_delete", Calls: func() []Call {
		const opCreate = "cd000000-0000-0000-0000-0000000000f1"
		const newAcct = "cd000000-0000-0000-0000-0000000000f2"
		var curID string
		var acctID string
		return []Call{
			{Label: "create-currency", Method: "POST", Path: "/api/v1/currency/create-currency", Auth: "owner",
				Body: map[string]any{"id": opCreate, "code": "PTS", "name": "Points", "symbol": "pts", "fractionDigits": 0, "rate": "100"}, CaptureIDInto: &curID},
			{Label: "create-account", Method: "POST", Path: "/api/v1/account/create-account", Auth: "owner",
				Body: map[string]any{"id": newAcct, "name": "Kid", "icon": "wallet", "currencyId": &curID, "folderId": OwnerFolder}, CaptureIDInto: &acctID},
			{Label: "err:delete-currency-in-use", Method: "POST", Path: "/api/v1/currency/delete-currency", Auth: "owner", Body: map[string]any{"id": &curID}},
			{Label: "delete-account", Method: "POST", Path: "/api/v1/account/delete-account", Auth: "owner", Body: map[string]any{"id": &acctID}},
			{Label: "delete-currency", Method: "POST", Path: "/api/v1/currency/delete-currency", Auth: "owner", Body: map[string]any{"id": &curID}},
			{Label: "read-after-delete", Method: "GET", Path: "/api/v1/currency/get-currency-list", Auth: "owner", Body: map[string]any{}},
			{Label: "rates-after-delete", Method: "GET", Path: "/api/v1/currency/get-currency-rate-list", Auth: "owner", Body: map[string]any{}},
		}
	}})
```

`err:delete-currency-in-use` is the half that proves the guard still works: it must be a 400, and only the subsequent `delete-account` makes the same call succeed. `rates-after-delete` is the regression guard for the silent 1:1 conversion — the deleted currency's rate must still be present in that golden.

- [ ] **Step 2: Run the parity suite to see it fail**

Run: `go test ./internal/test/apiparity/ -v`
Expected: FAIL — no golden for the new scenario, plus every currency-list golden now differs by the added `isDeleted` field.

- [ ] **Step 3: Regenerate the goldens and INSPECT the diff**

```bash
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/
UPDATE_GOLDEN=1 go test ./internal/test/mcpparity/
git diff --stat internal/test/apiparity/testdata internal/test/mcpparity/testdata
git diff internal/test/apiparity/testdata internal/test/mcpparity/testdata
```

Expected diff, and nothing else:

- every `get-currency-list` item gains `"isDeleted": 0`;
- the new scenario's golden appears, with the currency present at `"isDeleted": 1` in `read-after-delete` and its rate still present in `rates-after-delete`;
- `currency_write_read`'s existing `read-after-delete` call now shows the PTS currency with `"isDeleted": 1` instead of it being absent from the list.

Any other change — a reordered item, a changed rate, a currency vanishing from a list — means something unintended broke. Stop and fix the code rather than accepting the golden.

`guard_test.go`'s `minRoutes` floor (`:86`) does NOT need raising — this change adds no routes.

- [ ] **Step 4: Regenerate the OpenAPI docs**

Run: `make swagger`
Expected: the `CurrencyListItem` schema gains `isDeleted`.

- [ ] **Step 5: Run the full smoke gate**

Run: `make go-test`
Expected: PASS, including the guard tests that every route has a scenario and no golden is orphaned.

- [ ] **Step 6: Commit**

```bash
git add internal/test docs
git commit -m "test: parity scenario for deleting a currency pinned by a soft-deleted account"
```

---

### Task 5: Frontend — hide deleted currencies from the list and the pickers

**Files:**
- Modify: `web/src/api/dto/currency.ts:13-17` (`CurrencyListItemDto`)
- Modify: `web/src/features/currencies/selectable.ts`
- Modify: `web/src/features/classifications/ClassificationList.tsx:74`, `:147-162`, `:285-292`
- Modify: `web/src/features/currencies/CurrenciesPage.tsx:42-44`, `:139-157`
- Test: `web/src/features/currencies/selectable.test.ts`, `web/src/features/currencies/CurrenciesPage.test.tsx`

**Interfaces:**
- Consumes: the `isDeleted` wire field from Task 3.
- Produces: `rowSwitch` may return `null` to suppress a row's switch entirely.

- [ ] **Step 1: Add `isDeleted` to the shared test fixtures**

`isDeleted` is required (not optional) on the DTO, so every existing currency fixture needs it or `tsc -b` fails. In `web/src/test/fixtures.ts:24-25`:

```ts
export const fixtureUsd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2, scope: 'global', isHidden: 0, isDeleted: 0 }
export const fixtureEur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2, scope: 'global', isHidden: 0, isDeleted: 0 }
```

In `web/src/features/currencies/CurrenciesPage.test.tsx:11-12`, the same for `fixturePts` and `fixtureGbp`. In `web/src/features/currencies/selectable.test.ts:5-8`, add it to the `cur()` builder's defaults:

```ts
const cur = (over: Partial<CurrencyListItemDto>): CurrencyListItemDto => ({
  id: 'x', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2,
  scope: 'global', isHidden: 0, isDeleted: 0, ...over,
})
```

- [ ] **Step 2: Write the failing frontend tests**

In `web/src/features/currencies/selectable.test.ts`, inside the existing `describe`:

```ts
  it('excludes deleted currencies', () => {
    const items = [cur({ id: 'usd' }), cur({ id: 'pts', code: 'PTS', scope: 'own', isDeleted: 1 })]
    expect(selectableCurrencies(items).map((c) => c.id)).toEqual(['usd'])
  })
  // An account denominated in a deleted currency must stay editable, so the
  // current value survives the filter.
  it('keeps a deleted currency when it is the current value', () => {
    const items = [cur({ id: 'usd' }), cur({ id: 'pts', code: 'PTS', scope: 'own', isDeleted: 1 })]
    expect(selectableCurrencies(items, 'pts').map((c) => c.id)).toEqual(['usd', 'pts'])
  })
```

In `web/src/features/currencies/CurrenciesPage.test.tsx`, `renderPage()` takes no arguments — the currency list comes from the `coreHandlers({ currencies, rates })` call in `beforeEach`, so each test overrides it with its own `server.use(...)` exactly as the file's other tests do:

```ts
it('does not list a deleted own currency', async () => {
  server.use(...coreHandlers({ currencies: [fixtureUsd, { ...fixturePts, isDeleted: 1 }], rates: defaultRates }))
  renderPage()
  expect(await screen.findByText('US Dollar')).toBeInTheDocument()
  expect(screen.queryByText('Points')).not.toBeInTheDocument()
})

it('gives own customs no enable/disable switch', async () => {
  renderPage()
  await screen.findByText('Points')
  expect(screen.queryByLabelText('enable Points')).not.toBeInTheDocument()
  // Globals keep theirs.
  expect(screen.getByLabelText('enable Euro')).toBeInTheDocument()
})
```

- [ ] **Step 3: Run them to confirm they fail**

Run: `cd web && pnpm test -- selectable CurrenciesPage`
Expected: FAIL — deleted currencies are still selectable and still listed, and the switch still renders.

- [ ] **Step 4: Add the wire field to the DTO**

In `web/src/api/dto/currency.ts`:

```ts
export interface CurrencyListItemDto extends CurrencyDto {
  scope: 'global' | 'own' | 'shared'
  isHidden: 0 | 1
  // Deleted currencies are still returned: accounts and rates referencing them
  // must keep resolving. Every picker and list filters them out itself.
  isDeleted: 0 | 1
}
```

- [ ] **Step 5: Filter the pickers**

In `web/src/features/currencies/selectable.ts`:

```ts
// Dropdown-eligible currencies: visible, non-deleted globals plus the user's own
// visible, non-deleted customs. Foreign (shared-visible), hidden and deleted
// entries stay out, except the entity's current value so an edit form cannot
// self-corrupt -- which is how an account denominated in a deleted currency
// keeps being editable.
export function selectableCurrencies(items: CurrencyListItemDto[] | undefined, currentId?: string): CurrencyListItemDto[] {
  return (items ?? []).filter(
    (c) => c.id === currentId || (c.scope !== 'shared' && c.isHidden === 0 && c.isDeleted === 0),
  )
}
```

- [ ] **Step 6: Let a page opt a row out of the switch**

In `web/src/features/classifications/ClassificationList.tsx`, widen the prop:

```tsx
  /** per-item switch semantics; default = the archive toggle. null = no switch on this row */
  rowSwitch?: (item: T) => RowSwitchState | null
```

Replace `switchFor` so "no `rowSwitch` prop" and "`rowSwitch` returned null" stay distinguishable — pages that pass no `rowSwitch` (Categories, Tags, Payees, Recurring) must keep the archive default exactly as today:

```tsx
  const switchFor = (item: T): RowSwitchState | null => {
    let base: RowSwitchState
    if (rowSwitch) {
      const custom = rowSwitch(item)
      if (!custom) return null
      base = custom
    } else {
      base = {
        checked: item.isArchived === 0,
        ariaLabel: `archive ${item.name}`,
        onToggle: () => onToggleArchive?.(item),
      }
    }
    return {
      ...base,
      onToggle: () => {
        if (item.isArchived === 0 && activeOnly) {
          stickyArchivedIds.add(item.id)
        }
        base.onToggle()
      },
    }
  }
```

And render the switch conditionally:

```tsx
        {rowSwitchState ? (
          <Switch
            aria-label={rowSwitchState.ariaLabel}
            checked={rowSwitchState.checked}
            disabled={rowSwitchState.disabled}
            title={rowSwitchState.title}
            onClick={(e) => e.stopPropagation()}
            onCheckedChange={() => rowSwitchState.onToggle()}
          />
        ) : null}
```

- [ ] **Step 7: Update the currencies page**

In `web/src/features/currencies/CurrenciesPage.tsx`, drop deleted own customs from the list:

```tsx
  const own = currencies?.filter((c) => c.scope === 'own' && c.isDeleted === 0) ?? []
```

and give own customs no switch — Delete is their only lifecycle action:

```tsx
        rowSwitch={(c): RowSwitchState | null => {
          // Own customs have one lifecycle action, Delete, so they carry no
          // switch. Globals keep the enable/disable toggle; the base and
          // profile currencies must stay enabled, so theirs are locked.
          if (c.scope === 'own') {
            return null
          }
          const locked = c.id === baseId || c.id === profileId
          return {
            checked: c.isHidden === 0,
            disabled: locked,
            title:
              c.id === baseId
                ? t('classifications.currencies.pages.settings.locked_base')
                : c.id === profileId
                  ? t('classifications.currencies.pages.settings.locked_profile')
                  : undefined,
            ariaLabel: `enable ${c.name}`,
            onToggle: () => mutate(c.isHidden === 0 ? hideCurrency : showCurrency, c.id),
          }
        }}
```

Update the `rowActions` comment at `:119-120`, which currently claims "the switch covers hide" for own customs — it no longer does.

- [ ] **Step 8: Audit the other `useCurrencies()` consumers**

Run: `cd web && grep -rn "useCurrencies" src/ | grep -v "features/currencies/"`

For each hit, confirm it either goes through `selectableCurrencies` or looks currencies up by id (which must keep resolving deleted ones). Fix only consumers that render the whole list as choices.

- [ ] **Step 9: Run the frontend gates**

```bash
cd web && pnpm test && pnpm exec tsc -b && pnpm lint
```

Expected: PASS. `CategoriesPage.test.tsx` and `PayeesTagsPages.test.tsx` passing unchanged is the proof that the `ClassificationList` change kept its defaults.

- [ ] **Step 10: Run the full suite**

Run: `make test`
Expected: PASS, including the sqlite-vs-PostgreSQL engine comparison — the check that both partial indexes behave identically.

- [ ] **Step 11: Commit**

```bash
git add web
git commit -m "feat: hide deleted currencies from the settings list and pickers"
```

---

## Notes for the implementer

- **The bug fix is one predicate.** `AND accounts.is_deleted = 0` in `CountCurrencyUsage` is what unblocks the user. Everything else exists to make that safe.
- **Resist filtering `is_deleted` in the list view.** It looks like the tidy place for it and it is the one change that silently corrupts totals. The test `TestCurrencyReadRepo_ListViewKeepsDeletedRows` is there to stop you.
- **The admin `currency:delete` CLI is NOT in this plan.** It has its own spec; the read paths here were designed so it can land without reworking them.
