# Merge Classifications Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a "Merge into…" action for categories, tags, payees and labels that re-points every reference at a target, folds the source's budget limits into the target's, and deletes the source — all in one transaction.

**Architecture:** One `merge.go` use case per feature package, each exposed as `POST /api/v1/{module}/merge-{subject}` plus an MCP tool. Flat column re-points are single UPDATEs in each feature's own sqlc query file (the precedent set by `ReassignCategoryTransactions`). Budget-element merging lives in `internal/budget` behind a consumer-side port that `category` and `tag` declare and `internal/server` wires, so features never import features.

**Tech Stack:** Go (stdlib `net/http`, sqlc, two engines), React 19 + TanStack Query + vitest, i18n catalogues in `locales/{en,ru}.json`.

**Spec:** `docs/superpowers/specs/2026-08-11-merge-classifications-design.md`

## Global Constraints

- Wire format for datetimes is `2006-01-02 15:04:05` (`datetime.Layout`) — space separator, no zone, no fractional seconds.
- Response envelope: success is `{"success":true,"message":"","data":<payload>}`; merge results are `{}`.
- Every merge runs inside one `s.tx.WithTx`; ownership of **both** ids is checked before any write.
- Ownership failure on the **source** reports as that feature's existing not-found; on the **target** it reports as the new `{entity}.cannot_be_merged` code.
- Features never import features. Cross-feature needs go through a port in the consumer's `ports.go` + a `glue_*.go` adapter in `internal/server`. Enforced by `internal/test/archtest`.
- Every new error code needs `errors.<code>` entries in **both** `locales/en.json` and `locales/ru.json`; `i18ntest` enforces two-way coverage against `errs.AllCodes`.
- Every new user-facing action fires an analytics event from `METRICS` (`web/src/lib/metrics.ts`); `metrics-coverage.test.ts` fails if a key is never fired.
- Migration filenames **must be identical in `migrations/sqlite/` and `migrations/pgsql/`** — `sqliteimport` compares `schema_migrations` version sets and refuses on skew.
- Periods are compared with `datetime()` on SQLite, never raw `=`.
- Comments explain *why*, not *what*. No godoc restating a signature. No references to the removed PHP implementation.
- Coverage gate: `GO_COVER_MIN=80` via `make go-test`.

---

### Task 1: Normalize stored `period` format (Phase 0)

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260811000000.sql`
- Create: `internal/infra/storage/migrations/pgsql/20260811000000.sql`
- Create: `internal/infra/storage/migrations/migration_20260811_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: a database where every `budgets_elements_limits.period` on SQLite equals `datetime(period)`, and at most one row exists per `(element_id, datetime(period))`.

- [ ] **Step 1: Write the failing test**

Model it on the existing `migration_20260802_test.go` / `migration_20260803_test.go` in the same package.

```go
package migrations_test

// Legacy rows hold RFC3339 periods ("2026-08-01T00:00:00Z") written before the
// bind was fixed. SQLite's unique index on (element_id, period) is textual, so
// both forms coexist for one month and ListBudgetLimitsForPeriod returns both.
func TestMigration20260811NormalizesPeriodAndDedupes(t *testing.T) {
	db := openSQLiteAtVersion(t, "20260807000000")

	seedElement(t, db, "el-1")
	// same month, two formats: the pair that double-counts today
	insertLimit(t, db, "01000000-0000-0000-0000-000000000001", "el-1", "2026-08-01T00:00:00Z", "100")
	insertLimit(t, db, "01000000-0000-0000-0000-000000000002", "el-1", "2026-08-01 00:00:00", "300")
	// a lone legacy row that only needs rewriting
	insertLimit(t, db, "01000000-0000-0000-0000-000000000003", "el-1", "2026-09-01T00:00:00Z", "500")

	applyMigration(t, db, "20260811000000")

	rows := queryLimits(t, db, "el-1")
	if len(rows) != 2 {
		t.Fatalf("got %d limits, want 2 (August collapsed, September kept)", len(rows))
	}
	for _, r := range rows {
		if r.period != normalized(r.period) {
			t.Errorf("period %q is not canonical", r.period)
		}
	}
	aug := findByPeriod(t, rows, "2026-08-01 00:00:00")
	// MAX(id) wins; amounts are NOT summed — a duplicate pair is one limit
	// recorded twice, and summing would inflate the user's budget.
	if aug.amount != "300" {
		t.Errorf("August amount = %q, want 300 (the MAX(id) row, not the sum)", aug.amount)
	}
	sep := findByPeriod(t, rows, "2026-09-01 00:00:00")
	if sep.amount != "500" {
		t.Errorf("September amount = %q, want 500", sep.amount)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/infra/storage/migrations/ -run TestMigration20260811 -v`
Expected: FAIL — the migration file does not exist yet.

- [ ] **Step 3: Write the SQLite migration**

`internal/infra/storage/migrations/sqlite/20260811000000.sql`:

```sql
-- period is compared with datetime() everywhere because legacy rows hold
-- RFC3339 ("2026-08-01T00:00:00Z") while current writes are 'Y-m-d H:i:s'.
-- The unique index on (element_id, period) is TEXTUAL, so the two forms are
-- distinct keys: one month can hold both rows, and both are returned by
-- ListBudgetLimitsForPeriod. Collapse them, then canonicalize.
--
-- Dedupe BEFORE normalizing: rewriting first would collide on the index.
-- MAX(id) keeps the most recent (ids are UUIDv7, hence time-ordered), matching
-- the precedent in 20251214035500.sql. Amounts are NOT summed — a duplicate
-- pair is one limit recorded twice, not two limits.
DELETE FROM budgets_elements_limits
WHERE id NOT IN (
    SELECT MAX(id)
    FROM budgets_elements_limits
    GROUP BY element_id, datetime(period)
);

UPDATE budgets_elements_limits
SET period = datetime(period)
WHERE period <> datetime(period);
```

- [ ] **Step 4: Write the PostgreSQL counterpart**

`internal/infra/storage/migrations/pgsql/20260811000000.sql`:

```sql
-- PostgreSQL stores period as TIMESTAMP(0) WITHOUT TIME ZONE, so the textual
-- variance this migration fixes on SQLite cannot occur, and the unique index on
-- (element_id, period) already forbids the duplicates. The statement below is
-- therefore a guaranteed no-op; the FILE exists because migration versions must
-- match across engines — data:import-sqlite compares the schema_migrations sets
-- and refuses the import on any skew (sqliteimport.ErrSchemaMismatch).
DELETE FROM budgets_elements_limits l
USING budgets_elements_limits d
WHERE l.element_id = d.element_id
  AND l.period = d.period
  AND l.id < d.id;
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/infra/storage/migrations/ -v`
Expected: PASS

- [ ] **Step 6: Verify migration filename parity**

Run: `diff <(ls internal/infra/storage/migrations/sqlite/) <(ls internal/infra/storage/migrations/pgsql/)`
Expected: no output (identical). A difference here breaks `data:import-sqlite` for every user.

- [ ] **Step 7: Commit**

```bash
git add internal/infra/storage/migrations/
git commit -m "fix: normalize stored budget limit periods to one format"
```

---

### Task 2: Budget element merge (the `MergeElements` use case)

**Files:**
- Create: `internal/budget/merge.go`
- Modify: `internal/budget/repository.go` (add two methods to `ElementStore` and `LimitStore`)
- Modify: `internal/budget/repo/repo.go`, `internal/budget/repo/engine.go`
- Modify: `internal/infra/storage/sqlc/query/sqlite/budgets.sql`, `internal/infra/storage/sqlc/query/pgsql/budgets.sql`
- Create: `internal/budget/merge_test.go`

**Interfaces:**
- Consumes: `ElementStore.GetElementByExternal`, `SaveElement`, `DeleteElement`; `LimitStore.GetLimit`, `SaveLimit`, `NextIdentity` (all already exist in `internal/budget/repository.go`).
- Produces:
  ```go
  // internal/budget/merge.go
  func (s *Service) MergeElements(ctx context.Context, oldExternalID, newExternalID vo.Id) error
  ```
  Called by the `category` and `tag` services through their own port (Task 4).

- [ ] **Step 1: Add the two repository queries**

`internal/infra/storage/sqlc/query/sqlite/budgets.sql` — append:

```sql
-- name: ListBudgetElementsByExternal :many
-- Every budget in which this category/tag appears. Merging must touch them all,
-- including budgets shared with connected users.
SELECT id, budget_id, currency_id, folder_id, external_id, type, created_at, updated_at, sort_key
FROM budgets_elements WHERE external_id = ?;

-- name: ListBudgetLimitsByElement :many
-- Every period the element holds a limit for — the merge transfers all of them,
-- past and future alike, so there is no period filter here.
SELECT id, element_id, period, created_at, updated_at, amount
FROM budgets_elements_limits WHERE element_id = ?;
```

`internal/infra/storage/sqlc/query/pgsql/budgets.sql` — the same with `$1`.

- [ ] **Step 2: Regenerate sqlc and extend the interfaces**

Run: `cd internal/infra/storage/sqlc && sqlc generate`

Add to `ElementStore` in `internal/budget/repository.go`:
```go
	// ListElementsByExternal returns this external id's element in EVERY budget.
	ListElementsByExternal(ctx context.Context, externalID vo.Id) ([]*model.BudgetElement, error)
```
Add to `LimitStore`:
```go
	// ListLimitsByElement returns every period's limit for one element.
	ListLimitsByElement(ctx context.Context, elementID vo.Id) ([]*model.BudgetElementLimit, error)
```
Implement both in `internal/budget/repo/repo.go`, adding the querier methods to `internal/budget/repo/engine.go` for both `sqliteQuerier` and `pgsqlQuerier`, following the existing `ListBudgetLimitsForPeriod` pair exactly.

- [ ] **Step 3: Write the failing test**

```go
func TestMergeElements_TargetHasNoElement_RepointsInPlace(t *testing.T) {
	// Re-pointing preserves the element's folder and sort position, so the
	// budget row stays where the user put it.
	env := newBudgetEnv(t)
	src := env.seedElement(t, budgetID, srcCatID, model.ElementCategory, folderID)
	env.seedLimit(t, src.ID, "2026-08-01", "200")

	if err := env.svc.MergeElements(ctx, srcCatID, dstCatID); err != nil {
		t.Fatalf("MergeElements: %v", err)
	}

	got := env.getElementByExternal(t, budgetID, dstCatID)
	if got.ID != src.ID {
		t.Errorf("element id = %s, want %s (re-pointed, not recreated)", got.ID, src.ID)
	}
	if got.FolderID == nil || *got.FolderID != folderID {
		t.Error("folder lost by the merge")
	}
	if env.limitAmount(t, got.ID, "2026-08-01") != "200" {
		t.Error("limit lost by the merge")
	}
	if env.elementExistsByExternal(t, budgetID, srcCatID) {
		t.Error("source element still present")
	}
}

func TestMergeElements_BothElements_SumsEveryPeriod(t *testing.T) {
	env := newBudgetEnv(t)
	src := env.seedElement(t, budgetID, srcCatID, model.ElementCategory, nil)
	dst := env.seedElement(t, budgetID, dstCatID, model.ElementCategory, nil)
	env.seedLimit(t, src.ID, "2026-08-01", "200") // both have August
	env.seedLimit(t, dst.ID, "2026-08-01", "300")
	env.seedLimit(t, src.ID, "2026-09-01", "500") // source only
	env.seedLimit(t, dst.ID, "2026-10-01", "700") // target only
	env.seedLimit(t, src.ID, "2027-01-01", "900") // future period still moves

	if err := env.svc.MergeElements(ctx, srcCatID, dstCatID); err != nil {
		t.Fatalf("MergeElements: %v", err)
	}

	for _, c := range []struct{ period, want string }{
		{"2026-08-01", "500"}, // summed
		{"2026-09-01", "500"}, // carried over
		{"2026-10-01", "700"}, // untouched
		{"2027-01-01", "900"}, // future carried over
	} {
		if got := env.limitAmount(t, dst.ID, c.period); got != c.want {
			t.Errorf("period %s = %s, want %s", c.period, got, c.want)
		}
	}
	if env.elementExistsByExternal(t, budgetID, srcCatID) {
		t.Error("source element still present")
	}
}

func TestMergeElements_MixedPeriodFormats_ProducesOneLimit(t *testing.T) {
	// Phase 0 should make this unreachable in a migrated database. The test
	// stays because every limit the current code writes shares one format, so a
	// naive raw-'=' implementation passes an all-Go-fixture suite and fails
	// only on real legacy data.
	env := newBudgetEnv(t)
	src := env.seedElement(t, budgetID, srcCatID, model.ElementCategory, nil)
	dst := env.seedElement(t, budgetID, dstCatID, model.ElementCategory, nil)
	env.seedRawLimit(t, src.ID, "2026-08-01T00:00:00Z", "200")
	env.seedRawLimit(t, dst.ID, "2026-08-01 00:00:00", "300")

	if err := env.svc.MergeElements(ctx, srcCatID, dstCatID); err != nil {
		t.Fatalf("MergeElements: %v", err)
	}

	if n := env.countLimits(t, dst.ID); n != 1 {
		t.Fatalf("target holds %d limits, want 1", n)
	}
	if got := env.limitAmount(t, dst.ID, "2026-08-01"); got != "500" {
		t.Errorf("amount = %s, want 500", got)
	}
}
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `go test ./internal/budget/ -run TestMergeElements -v`
Expected: FAIL — `MergeElements` undefined.

- [ ] **Step 5: Implement `MergeElements`**

`internal/budget/merge.go`:

```go
package budget

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// MergeElements folds one classification's budget presence into another's,
// across every budget the source appears in — including budgets shared with
// connected users.
//
// Limits are SUMMED rather than replaced: the caller is also re-pointing the
// spending, so keeping only the target's limit would make every past period
// where both had one read as over budget.
func (s *Service) MergeElements(ctx context.Context, oldExternalID, newExternalID vo.Id) error {
	return s.tx.WithTx(ctx, func(txCtx context.Context) error {
		sources, err := s.repo.ListElementsByExternal(txCtx, oldExternalID)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		for _, src := range sources {
			target, terr := s.repo.GetElementByExternal(txCtx, src.BudgetID, newExternalID)
			if terr != nil && !errs.IsNotFound(terr) {
				return terr
			}
			if target == nil {
				// No conflict: re-point in place. This keeps the element's folder
				// and sort position, so the budget row stays where the user put it.
				src.ExternalID = newExternalID
				src.UpdatedAt = now
				if serr := s.repo.SaveElement(txCtx, src); serr != nil {
					return serr
				}
				continue
			}
			if merr := s.mergeLimits(txCtx, src, target, now); merr != nil {
				return merr
			}
			// Limits cascade off the element.
			if derr := s.repo.DeleteElement(txCtx, src.ID); derr != nil {
				return derr
			}
		}
		return nil
	})
}

// mergeLimits adds each of src's per-period limits into dst's. GetLimit
// normalizes the period with datetime() on SQLite, which is what makes this
// correct across the legacy RFC3339 rows Phase 0 sweeps up: a raw '=' match
// would miss the pair and leave a second row for the same month.
func (s *Service) mergeLimits(ctx context.Context, src, dst *model.BudgetElement, now time.Time) error {
	limits, err := s.repo.ListLimitsByElement(ctx, src.ID)
	if err != nil {
		return err
	}
	for _, l := range limits {
		existing, gerr := s.repo.GetLimit(ctx, dst.ID, l.Period)
		if gerr != nil && !errs.IsNotFound(gerr) {
			return gerr
		}
		if existing == nil {
			if serr := s.repo.SaveLimit(ctx, &model.BudgetElementLimit{
				ID: s.repo.NextIdentity(), ElementID: dst.ID, Period: l.Period,
				Amount: l.Amount, CreatedAt: now, UpdatedAt: now,
			}); serr != nil {
				return serr
			}
			continue
		}
		existing.Amount = existing.Amount.Add(l.Amount)
		existing.UpdatedAt = now
		if serr := s.repo.SaveLimit(ctx, existing); serr != nil {
			return serr
		}
	}
	return nil
}
```

Adjust the not-found handling to match how the budget repo actually signals a
missing row (check `GetElementByExternal`'s existing callers in
`internal/budget/move.go` and follow the same convention).

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/budget/ -run TestMergeElements -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/budget/ internal/infra/storage/sqlc/
git commit -m "feat: merge one budget element's limits into another"
```

---

### Task 3: Payee merge — the reference implementation

**Files:**
- Create: `internal/payee/merge.go`, `internal/payee/merge_test.go`
- Modify: `internal/payee/repository.go`, `internal/payee/repo/repo.go`, `repo/sqlite.go`, `repo/pgsql.go`
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/payees.sql`
- Modify: `internal/model/payee_dto.go`
- Modify: `internal/shared/errs/codes.go`
- Modify: `internal/payee/api/payee.go`, `internal/payee/api/routes.go`
- Modify: `internal/payee/mcp/mcp.go`
- Modify: `locales/en.json`, `locales/ru.json`

**Interfaces:**
- Consumes: `payee.Repository.GetByID`, `Delete`; `port.TxRunner`.
- Produces:
  ```go
  // internal/model/payee_dto.go
  type MergePayeeRequest struct {
      SourceId string `json:"sourceId"`
      TargetId string `json:"targetId"`
  }
  func (r MergePayeeRequest) Validate() error
  type MergePayeeResult struct{}

  // internal/payee/repository.go — added to Repository
  ReassignTransactions(ctx context.Context, oldID, newID vo.Id) error
  ReassignRecurring(ctx context.Context, oldID, newID vo.Id) error

  // internal/payee/merge.go
  func (s *Service) MergePayee(ctx context.Context, userID vo.Id, req model.MergePayeeRequest) (*model.MergePayeeResult, error)
  ```

- [ ] **Step 1: Add the error code**

`internal/shared/errs/codes.go` — add the constant next to `CodePayeeAlreadyExists`, and add it to the `AllCodes` slice:
```go
	CodePayeeCannotBeMerged = "payee.cannot_be_merged"
```

Add to `locales/en.json` under `errors`:
```json
"payee.cannot_be_merged": "Payees cannot be merged"
```
and to `locales/ru.json`:
```json
"payee.cannot_be_merged": "Плательщиков нельзя объединить"
```

- [ ] **Step 2: Write the failing test**

`internal/payee/merge_test.go`:

```go
func TestMergePayee_ReassignsAndDeletes(t *testing.T) {
	env := newPayeeEnv(t)
	src := env.seedPayee(t, ownerID, "Grocer")
	dst := env.seedPayee(t, ownerID, "Grocery Store")
	txID := env.seedTransaction(t, ownerID, withPayee(src))
	recID := env.seedRecurring(t, ownerID, withPayee(src))

	if _, err := env.svc.MergePayee(ctx, ownerID, model.MergePayeeRequest{
		SourceId: src.String(), TargetId: dst.String(),
	}); err != nil {
		t.Fatalf("MergePayee: %v", err)
	}

	if got := env.transactionPayee(t, txID); got != dst.String() {
		t.Errorf("transaction payee = %s, want %s", got, dst)
	}
	// The bug the old delete-category replace mode had: recurring templates were
	// left behind and silently nulled.
	if got := env.recurringPayee(t, recID); got != dst.String() {
		t.Errorf("recurring payee = %s, want %s", got, dst)
	}
	if env.payeeExists(t, src) {
		t.Error("source payee still exists")
	}
}

func TestMergePayee_ForeignOwnedSource_IsNotFoundAndTouchesNothing(t *testing.T) {
	env := newPayeeEnv(t)
	foreign := env.seedPayee(t, otherUserID, "Theirs")
	mine := env.seedPayee(t, ownerID, "Mine")
	txID := env.seedTransaction(t, otherUserID, withPayee(foreign))

	_, err := env.svc.MergePayee(ctx, ownerID, model.MergePayeeRequest{
		SourceId: foreign.String(), TargetId: mine.String(),
	})

	if !errs.IsNotFound(err) {
		t.Fatalf("err = %v, want NotFound", err)
	}
	if !env.payeeExists(t, foreign) {
		t.Error("foreign payee was deleted")
	}
	if got := env.transactionPayee(t, txID); got != foreign.String() {
		t.Error("foreign user's transaction was re-pointed")
	}
}

func TestMergePayee_ForeignOwnedTarget_CannotBeMerged(t *testing.T) {
	env := newPayeeEnv(t)
	mine := env.seedPayee(t, ownerID, "Mine")
	foreign := env.seedPayee(t, otherUserID, "Theirs")

	_, err := env.svc.MergePayee(ctx, ownerID, model.MergePayeeRequest{
		SourceId: mine.String(), TargetId: foreign.String(),
	})

	assertValidationCode(t, err, errs.CodePayeeCannotBeMerged)
	if !env.payeeExists(t, mine) {
		t.Error("source was deleted despite the failure")
	}
}

func TestMergePayee_SameId_CannotBeMerged(t *testing.T) {
	env := newPayeeEnv(t)
	mine := env.seedPayee(t, ownerID, "Mine")

	_, err := env.svc.MergePayee(ctx, ownerID, model.MergePayeeRequest{
		SourceId: mine.String(), TargetId: mine.String(),
	})

	assertValidationCode(t, err, errs.CodePayeeCannotBeMerged)
	if !env.payeeExists(t, mine) {
		t.Error("payee merged into itself and vanished")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/payee/ -run TestMergePayee -v`
Expected: FAIL — `MergePayee` undefined.

- [ ] **Step 4: Add the DTO**

`internal/model/payee_dto.go`:

```go
// MergePayeeRequest is the merge-payee request body. sourceId is absorbed into
// targetId and then deleted; the names are spelled out rather than reusing the
// usual "id" because this ships as an MCP tool, where the destructive side must
// be unambiguous to a model choosing arguments.
type MergePayeeRequest struct {
	SourceId string `json:"sourceId"`
	TargetId string `json:"targetId"`
}

func (r MergePayeeRequest) Validate() error {
	var fields []errs.FieldError
	if strings.TrimSpace(r.SourceId) == "" {
		fields = append(fields, errs.FieldError{Key: "sourceId", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if strings.TrimSpace(r.TargetId) == "" {
		fields = append(fields, errs.FieldError{Key: "targetId", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

type MergePayeeResult struct{}
```

- [ ] **Step 5: Add the reassign queries**

`internal/infra/storage/sqlc/query/sqlite/payees.sql` — append:

```sql
-- name: ReassignPayeeTransactions :exec
-- Merge: point every transaction on the old payee at the new one before the old
-- payee is deleted. Not scoped by user on purpose — a shared account's
-- transactions carry the account owner's payee and must follow it.
UPDATE transactions SET payee_id = ? WHERE payee_id = ?;

-- name: ReassignPayeeRecurring :exec
UPDATE recurring_transactions SET payee_id = ? WHERE payee_id = ?;
```

The pgsql variant uses `$1`/`$2`. Regenerate: `cd internal/infra/storage/sqlc && sqlc generate`.

Add both to the `querier` interface in `internal/payee/repo/repo.go` and implement in `sqlite.go`/`pgsql.go`, following `internal/category/repo/{sqlite,pgsql}.go:34-45` exactly. Add `ReassignTransactions`/`ReassignRecurring` to `payee.Repository` and to `Repo`.

- [ ] **Step 6: Implement the use case**

`internal/payee/merge.go`:

```go
package payee

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// MergePayee absorbs one payee into another: every transaction and recurring
// template on the source is re-pointed at the target, then the source is
// deleted. Irreversible, and one transaction end to end.
//
// A foreign-owned SOURCE reports as not-found so ids cannot be probed (matching
// delete); a foreign-owned TARGET reports as cannot-be-merged.
func (s *Service) MergePayee(ctx context.Context, userID vo.Id, req model.MergePayeeRequest) (*model.MergePayeeResult, error) {
	sourceID, err := vo.ParseId(req.SourceId)
	if err != nil {
		return nil, err
	}
	targetID, err := vo.ParseId(req.TargetId)
	if err != nil {
		return nil, err
	}

	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		source, gerr := s.repo.GetByID(txCtx, sourceID)
		if gerr != nil {
			return gerr
		}
		if !source.UserID.Equal(userID) {
			return errs.NewNotFound("Payee not found")
		}
		if sourceID.Equal(targetID) {
			return &errs.ValidationError{Msg: "Payees cannot be merged", MsgCode: errs.CodePayeeCannotBeMerged}
		}
		target, terr := s.repo.GetByID(txCtx, targetID)
		if terr != nil {
			return terr
		}
		if !target.UserID.Equal(userID) {
			return &errs.ValidationError{Msg: "Payees cannot be merged", MsgCode: errs.CodePayeeCannotBeMerged}
		}

		if rerr := s.repo.ReassignTransactions(txCtx, sourceID, targetID); rerr != nil {
			return rerr
		}
		if rerr := s.repo.ReassignRecurring(txCtx, sourceID, targetID); rerr != nil {
			return rerr
		}
		return s.repo.Delete(txCtx, sourceID)
	}); err != nil {
		return nil, err
	}

	return &model.MergePayeeResult{}, nil
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/payee/... -v`
Expected: PASS

- [ ] **Step 8: Add the HTTP handler and route**

`internal/payee/api/payee.go` — copy the annotation block shape from `DeletePayee` (lines 91-108):

```go
// MergePayee handles POST /api/v1/payee/merge-payee (auth).
//
// @Summary     Merge two payees
// @Description Re-points every transaction and recurring template from sourceId to targetId, then deletes sourceId. Requires ownership of both.
// @Tags        Payee
// @Accept      json
// @Produce     json
// @Param       request body     model.MergePayeeRequest true "Merge payee request"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.MergePayeeResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/payee/merge-payee [post]
func (h *Handlers) MergePayee(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.MergePayee)
}
```

`internal/payee/api/routes.go` — add after the delete route, and update the count in the `RegisterAPI` comment from 7 to 8:
```go
		mux.Handle("POST /api/v1/payee/merge-payee", auth(h.MergePayee))
```

- [ ] **Step 9: Add the MCP tool**

`internal/payee/mcp/mcp.go` — follow the `create_payee` registration shape:
```go
	sdk.AddTool(s, &sdk.Tool{Name: "merge_payee",
		Description: "Merge one payee into another: moves its transactions and recurring templates to the target, then deletes it. Irreversible."},
		/* handler calling svc.MergePayee, mapping errors with mcp.MapErr */)
```

- [ ] **Step 10: Run the full backend suite**

Run: `make go-test`
Expected: PASS (goldens for the new route are added in Task 8; if the route-count guard trips here, note it and proceed — Task 8 resolves it).

- [ ] **Step 11: Commit**

```bash
git add internal/payee/ internal/model/ internal/shared/errs/ internal/infra/storage/sqlc/ locales/
git commit -m "feat: merge one payee into another"
```

---

### Task 4: Category merge (with the budget port)

**Files:**
- Create: `internal/category/merge.go`, `internal/category/merge_test.go`
- Create: `internal/server/glue_category_budgetmerge.go`
- Modify: `internal/category/ports.go`, `internal/category/repository.go`, `internal/category/repo/*.go`
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/categories.sql`
- Modify: `internal/model/category_dto.go`, `internal/shared/errs/codes.go`
- Modify: `internal/category/api/category.go`, `routes.go`, `internal/category/mcp/mcp.go`
- Modify: `internal/server/server.go` (wire the port), `locales/{en,ru}.json`

**Interfaces:**
- Consumes: `budget.Service.MergeElements` (Task 2), via the port below.
- Produces:
  ```go
  // internal/category/ports.go
  type BudgetElementMerger interface {
      MergeElements(ctx context.Context, oldExternalID, newExternalID vo.Id) error
  }

  // internal/model/category_dto.go
  type MergeCategoryRequest struct {
      SourceId string `json:"sourceId"`
      TargetId string `json:"targetId"`
  }
  type MergeCategoryResult struct{}

  // internal/category/merge.go
  func (s *Service) MergeCategory(ctx context.Context, userID vo.Id, req model.MergeCategoryRequest) (*model.MergeCategoryResult, error)
  ```

- [ ] **Step 1: Write the failing test**

Same four cases as Task 3 (happy path, foreign source, foreign target, same id) with `Category` substituted, **plus** the two category-specific ones:

```go
func TestMergeCategory_TypeMismatch_CannotBeMerged(t *testing.T) {
	env := newCategoryEnv(t)
	expense := env.seedCategory(t, ownerID, "Food", model.CategoryTypeExpense)
	income := env.seedCategory(t, ownerID, "Salary", model.CategoryTypeIncome)

	_, err := env.svc.MergeCategory(ctx, ownerID, model.MergeCategoryRequest{
		SourceId: expense.String(), TargetId: income.String(),
	})

	assertValidationCode(t, err, errs.CodeCategoryCannotBeMerged)
	if !env.categoryExists(t, expense) {
		t.Error("source deleted despite the type mismatch")
	}
}

func TestMergeCategory_DelegatesToBudgetMerger(t *testing.T) {
	env := newCategoryEnv(t)
	src := env.seedCategory(t, ownerID, "Food", model.CategoryTypeExpense)
	dst := env.seedCategory(t, ownerID, "Groceries", model.CategoryTypeExpense)

	if _, err := env.svc.MergeCategory(ctx, ownerID, model.MergeCategoryRequest{
		SourceId: src.String(), TargetId: dst.String(),
	}); err != nil {
		t.Fatalf("MergeCategory: %v", err)
	}

	if env.merger.calls != 1 {
		t.Fatalf("budget merger called %d times, want 1", env.merger.calls)
	}
	if env.merger.lastOld != src || env.merger.lastNew != dst {
		t.Errorf("merger got (%s,%s), want (%s,%s)", env.merger.lastOld, env.merger.lastNew, src, dst)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/category/ -run TestMergeCategory -v`
Expected: FAIL — `MergeCategory` undefined.

- [ ] **Step 3: Add the error code and catalogue entries**

`internal/shared/errs/codes.go`: add `CodeCategoryCannotBeMerged = "category.cannot_be_merged"` and register it in `AllCodes`.
`locales/en.json`: `"category.cannot_be_merged": "Categories cannot be merged"`.
`locales/ru.json`: `"category.cannot_be_merged": "Категории нельзя объединить"`.

- [ ] **Step 4: Add the reassign queries and port**

`internal/infra/storage/sqlc/query/sqlite/categories.sql` — append (the transactions one already exists as `ReassignCategoryTransactions`):
```sql
-- name: ReassignCategoryRecurring :exec
UPDATE recurring_transactions SET category_id = ? WHERE category_id = ?;
```
pgsql variant with `$1`/`$2`; regenerate sqlc; add to the querier interface and both adapters; add `ReassignRecurring` to `category.Repository`.

`internal/category/ports.go` — add:
```go
// BudgetElementMerger folds the source classification's budget presence into
// the target's. Implemented by internal/budget and wired in internal/server, so
// this package never imports it.
type BudgetElementMerger interface {
	MergeElements(ctx context.Context, oldExternalID, newExternalID vo.Id) error
}
```

Add the field to `Service` and the `NewService` parameter list.

- [ ] **Step 5: Implement the use case**

Same body as `MergePayee` (Task 3, Step 6) with these differences: the not-found error is `&errs.ValidationError{Msg: "Category not found", MsgCode: errs.CodeCategoryNotFound}` (category reports its own not-found as a ValidationError — see `internal/category/delete.go:36`); after the ownership checks add

```go
		if source.Type != target.Type {
			return &errs.ValidationError{Msg: "Categories cannot be merged", MsgCode: errs.CodeCategoryCannotBeMerged}
		}
```

and between the recurring re-point and the delete add

```go
		if berr := s.budget.MergeElements(txCtx, sourceID, targetID); berr != nil {
			return berr
		}
```

- [ ] **Step 6: Wire the glue**

`internal/server/glue_category_budgetmerge.go`:
```go
package server

// categoryBudgetMerger adapts the budget service to category.BudgetElementMerger.
type categoryBudgetMerger struct{ svc *budget.Service }

func (m categoryBudgetMerger) MergeElements(ctx context.Context, oldExternalID, newExternalID vo.Id) error {
	return m.svc.MergeElements(ctx, oldExternalID, newExternalID)
}
```
Pass `categoryBudgetMerger{budgetSvc}` into `category.NewService` in `internal/server/server.go`.

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/category/... ./internal/server/... -v`
Expected: PASS

- [ ] **Step 8: Add handler, route and MCP tool**

Mirror Task 3 steps 8-9 with `Category`/`merge-category`/`merge_category`. Update the route-count comment in `internal/category/api/routes.go`.

- [ ] **Step 9: Commit**

```bash
git add internal/category/ internal/server/ internal/model/ internal/shared/errs/ internal/infra/storage/sqlc/ locales/
git commit -m "feat: merge one category into another"
```

---

### Task 5: Tag merge

**Files:** the `tag` equivalents of Task 4, plus `internal/server/glue_tag_budgetmerge.go`.

**Interfaces:**
- Produces: `MergeTagRequest{SourceId,TargetId}`, `MergeTagResult{}`, `func (s *Service) MergeTag(ctx, userID, req) (*model.MergeTagResult, error)`, `tag.BudgetElementMerger` (same shape as category's).

- [ ] **Step 1: Write the failing test**

The four Task 3 cases with `Tag` substituted, plus the `DelegatesToBudgetMerger` case from Task 4 Step 1. Tags have **no type**, so there is no type-mismatch case. The source not-found error is `errs.NewNotFound("Tag not found")` (see `internal/tag/delete.go:29`), not a ValidationError.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tag/ -run TestMergeTag -v`
Expected: FAIL

- [ ] **Step 3: Add code, catalogue, queries, port, use case, glue**

`CodeTagCannotBeMerged = "tag.cannot_be_merged"`; `locales/en.json` `"tag.cannot_be_merged": "Tags cannot be merged"`; `locales/ru.json` `"tag.cannot_be_merged": "Теги нельзя объединить"`.

```sql
-- name: ReassignTagTransactions :exec
UPDATE transactions SET tag_id = ? WHERE tag_id = ?;

-- name: ReassignTagRecurring :exec
UPDATE recurring_transactions SET tag_id = ? WHERE tag_id = ?;
```

Everything else follows Task 4 exactly.

- [ ] **Step 4: Run tests, add handler/route/MCP, commit**

Run: `go test ./internal/tag/... -v`
```bash
git commit -m "feat: merge one tag into another"
```

---

### Task 6: Label merge (many-to-many dedupe)

**Files:**
- Create: `internal/label/merge.go`, `internal/label/merge_test.go`
- Modify: `internal/label/repository.go`, `internal/label/repo/{repo,sqlite,pgsql}.go`
- Modify: `internal/infra/storage/sqlc/query/{sqlite,pgsql}/labels.sql`
- Modify: `internal/model/label_dto.go`, `internal/shared/errs/codes.go`
- Modify: `internal/label/api/*.go`, `internal/label/mcp/mcp.go`, `locales/{en,ru}.json`

**Interfaces:**
- Produces: `MergeLabelRequest{SourceId,TargetId}`, `MergeLabelResult{}`, `func (s *Service) MergeLabel(...)`, plus `Repository.ReassignTransactionLabels` / `ReassignRecurringLabels`.

- [ ] **Step 1: Write the failing test**

The four Task 3 cases with `Label` substituted, plus the dedupe case that is unique to labels:

```go
func TestMergeLabel_TransactionCarryingBoth_EndsWithOneRow(t *testing.T) {
	// transactions_labels is many-to-many, so a transaction can already hold
	// both labels. Re-pointing must dedupe, not duplicate.
	env := newLabelEnv(t)
	src := env.seedLabel(t, ownerID, "Kid A")
	dst := env.seedLabel(t, ownerID, "Kids")
	both := env.seedTransaction(t, ownerID, withLabels(src, dst))
	onlySrc := env.seedTransaction(t, ownerID, withLabels(src))

	if _, err := env.svc.MergeLabel(ctx, ownerID, model.MergeLabelRequest{
		SourceId: src.String(), TargetId: dst.String(),
	}); err != nil {
		t.Fatalf("MergeLabel: %v", err)
	}

	if got := env.labelIDs(t, both); !reflect.DeepEqual(got, []string{dst.String()}) {
		t.Errorf("labels on the both-carrier = %v, want exactly [%s]", got, dst)
	}
	if got := env.labelIDs(t, onlySrc); !reflect.DeepEqual(got, []string{dst.String()}) {
		t.Errorf("labels on the source-only transaction = %v, want [%s]", got, dst)
	}
	if env.labelExists(t, src) {
		t.Error("source label still exists")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/label/ -run TestMergeLabel -v`
Expected: FAIL

- [ ] **Step 3: Add the per-engine queries**

`internal/infra/storage/sqlc/query/sqlite/labels.sql`:
```sql
-- name: ReassignTransactionLabels :exec
-- Many-to-many: a transaction may already carry BOTH labels, so re-pointing
-- must dedupe. The source's own rows cascade away when the label is deleted.
INSERT OR IGNORE INTO transactions_labels (transaction_id, label_id)
SELECT transaction_id, ? FROM transactions_labels WHERE label_id = ?;

-- name: ReassignRecurringLabels :exec
INSERT OR IGNORE INTO recurring_transactions_labels (recurring_transaction_id, label_id)
SELECT recurring_transaction_id, ? FROM recurring_transactions_labels WHERE label_id = ?;
```

`internal/infra/storage/sqlc/query/pgsql/labels.sql` — same shape, `$1`/`$2`, and `ON CONFLICT DO NOTHING` in place of `INSERT OR IGNORE`:
```sql
-- name: ReassignTransactionLabels :exec
INSERT INTO transactions_labels (transaction_id, label_id)
SELECT transaction_id, $1 FROM transactions_labels WHERE label_id = $2
ON CONFLICT DO NOTHING;

-- name: ReassignRecurringLabels :exec
INSERT INTO recurring_transactions_labels (recurring_transaction_id, label_id)
SELECT recurring_transaction_id, $1 FROM recurring_transactions_labels WHERE label_id = $2
ON CONFLICT DO NOTHING;
```

Verify the exact join-table column names against `migrations/sqlite/20260805000000.sql:25-31` and `20260806000000.sql:4-10` before writing these.

- [ ] **Step 4: Implement, run tests, add handler/route/MCP, commit**

`CodeLabelCannotBeMerged = "label.cannot_be_merged"`; en `"Labels cannot be merged"`; ru `"Метки нельзя объединить"`. Use-case body follows Task 3 with the two reassign calls replaced by the label pair and no budget merge (labels are budget-neutral).

Run: `go test ./internal/label/... -v`
```bash
git commit -m "feat: merge one label into another"
```

---

### Task 7: Remove `delete-category` `mode=replace`

**Files:**
- Modify: `internal/model/category_dto.go:129-164`
- Modify: `internal/category/delete.go:39-60`
- Modify: `internal/category/repository.go` (drop `ReassignTransactions` if now unused)
- Modify: `internal/shared/errs/codes.go` (drop two codes from the constants and `AllCodes`)
- Modify: `locales/en.json`, `locales/ru.json`
- Modify: `internal/test/apiparity/catalogue.go:103`

- [ ] **Step 1: Write the failing test**

```go
func TestDeleteCategory_ReplaceMode_IsRejected(t *testing.T) {
	// mode=replace was a redundant, incomplete second merge path (it moved
	// transactions but not recurring templates). It fails loudly rather than
	// silently degrading into a destructive plain delete.
	err := model.DeleteCategoryRequest{Id: "x", Mode: "replace"}.Validate()

	assertFieldCode(t, err, "mode", errs.CodeInvalidChoice)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run TestDeleteCategory_ReplaceMode -v`
Expected: FAIL — `replace` is still accepted.

- [ ] **Step 3: Remove the mode**

In `internal/model/category_dto.go`: delete the `ModeReplace` constant, the `ReplaceId` field, and the `case ModeReplace:` branch of `Validate` (lines 152-155). Keep `Mode` required and keep `ModeDelete`.

In `internal/category/delete.go`: delete the whole `if req.Mode == model.ModeReplace { … }` block (lines 39-60), leaving the ownership check and `s.repo.Delete`.

In `internal/shared/errs/codes.go`: remove `CodeCategoryReplaceIDRequired` and `CodeCategoryCannotBeReplaced` from both the constant block and `AllCodes`. Remove `category.replace_id_required` and `category.cannot_be_replaced` from **both** locale files.

In `internal/test/apiparity/catalogue.go:103`, the `delete-category` call already uses `"mode": "delete"` — leave it. Remove any scenario elsewhere that passes `"mode": "replace"`.

- [ ] **Step 4: Run the guards**

Run: `go test ./internal/model/... ./internal/category/... ./internal/test/i18ntest/... -v`
Expected: PASS. `i18ntest` proves the removed codes have no orphaned catalogue entries and vice versa.

- [ ] **Step 5: Commit**

```bash
git add internal/model/ internal/category/ internal/shared/errs/ locales/ internal/test/
git commit -m "refactor: drop delete-category replace mode in favour of merge-category"
```

---

### Task 8: Parity scenarios and goldens

**Files:**
- Modify: `internal/test/apiparity/catalogue.go`
- Modify: `internal/test/mcpparity/` catalogue
- Regenerate: `internal/test/apiparity/testdata/golden/`, `internal/test/mcpparity/testdata/golden/`

- [ ] **Step 1: Add the four merge scenarios**

In `internal/test/apiparity/catalogue.go`, register one scenario per kind, following the `payee_write_read` shape at lines 141-151:

```go
	register(Scenario{Name: "payee_merge", Calls: func() []Call {
		const srcPayee = "20000000-0000-0000-0000-0000000000a1"
		const dstPayee = "20000000-0000-0000-0000-0000000000a2"
		var srcID, dstID string
		return []Call{
			{Label: "create-source", Method: "POST", Path: "/api/v1/payee/create-payee", Auth: "owner", Body: map[string]any{"id": srcPayee, "name": "Grocer"}, CaptureIDInto: &srcID},
			{Label: "create-target", Method: "POST", Path: "/api/v1/payee/create-payee", Auth: "owner", Body: map[string]any{"id": dstPayee, "name": "Grocery Store"}, CaptureIDInto: &dstID},
			{Label: "merge-payee", Method: "POST", Path: "/api/v1/payee/merge-payee", Auth: "owner", Body: map[string]any{"sourceId": &srcID, "targetId": &dstID}},
			{Label: "read-after-merge", Method: "GET", Path: "/api/v1/payee/get-payee-list", Auth: "owner", Body: map[string]any{}},
			{Label: "merge-into-self", Method: "POST", Path: "/api/v1/payee/merge-payee", Auth: "owner", Body: map[string]any{"sourceId": &dstID, "targetId": &dstID}},
		}
	}})
```

Repeat for `category_merge` (add a `merge-across-types` call asserting the 400), `tag_merge` and `label_merge`.

- [ ] **Step 2: Regenerate the goldens**

Run: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`
Run: `UPDATE_GOLDEN=1 go test ./internal/test/mcpparity/`

- [ ] **Step 3: INSPECT the diff**

Run: `git diff internal/test/apiparity/testdata/ internal/test/mcpparity/testdata/`

Confirm the only changes are **added** files/blocks for the new scenarios plus the removal of any `mode=replace` scenario. A change to an unrelated golden means observable behaviour changed — stop and investigate. Never hand-edit a golden.

- [ ] **Step 4: Regenerate the OpenAPI docs**

Run: `make swagger`

- [ ] **Step 5: Run the whole suite and commit**

Run: `make go-test`
```bash
git add internal/test/ docs/
git commit -m "test: parity scenarios and goldens for the merge endpoints"
```

---

### Task 9: Frontend — merge dialog and mutations

**Files:**
- Create: `web/src/features/classifications/MergeDialog.tsx`
- Create: `web/src/features/classifications/MergeDialog.test.tsx`
- Modify: `web/src/features/classifications/queries.ts`
- Modify: `CategoriesPage.tsx`, `TagsPage.tsx`, `PayeesPage.tsx`
- Modify: `web/src/lib/metrics.ts`, `locales/en.json`, `locales/ru.json`

**Interfaces:**
- Consumes: the four `merge-*` endpoints from Tasks 3-6.
- Produces:
  ```ts
  export function MergeDialog<T extends ClassificationItem>(props: {
    open: boolean
    source: T | null
    candidates: T[]      // already filtered to own + eligible by the caller
    noun: string
    info?: string
    onClose: () => void
    onConfirm: (targetId: string) => void
  }): JSX.Element

  // queries.ts
  export function useMergeCategory(): UseMutationResult<...>
  export function useMergeTag(): UseMutationResult<...>
  export function useMergePayee(): UseMutationResult<...>
  export function useMergeLabel(): UseMutationResult<...>
  ```

- [ ] **Step 1: Add the metrics key**

`web/src/lib/metrics.ts` — add to `METRICS`:
```ts
  CLASSIFICATION_MERGE: 'appClassificationMerge',
```

- [ ] **Step 2: Write the failing test**

`MergeDialog.test.tsx`:
```tsx
it('excludes the source from the candidate list', () => {
  render(<MergeDialog open source={food} candidates={[food, groceries]} noun="category" onClose={noop} onConfirm={noop} />)
  expect(screen.queryByRole('option', { name: 'Food' })).not.toBeInTheDocument()
  expect(screen.getByRole('option', { name: 'Groceries' })).toBeInTheDocument()
})

it('confirms with the chosen target id', async () => {
  const onConfirm = vi.fn()
  render(<MergeDialog open source={food} candidates={[food, groceries]} noun="category" onClose={noop} onConfirm={onConfirm} />)
  await userEvent.click(screen.getByRole('option', { name: 'Groceries' }))
  await userEvent.click(screen.getByRole('button', { name: /merge/i }))
  expect(onConfirm).toHaveBeenCalledWith(groceries.id)
})

it('warns that the merge cannot be undone', () => {
  render(<MergeDialog open source={food} candidates={[groceries]} noun="category" onClose={noop} onConfirm={noop} />)
  expect(screen.getByText(/cannot be undone/i)).toBeInTheDocument()
})
```

`PayeesTagsPages.test.tsx` — add:
```tsx
it('offers Merge into… and omits foreign-owned rows as targets', async () => {
  // seeded: p1/p2 owned by u1, p3 owned by u2
  await userEvent.click(screen.getByLabelText('actions Grocer'))
  await userEvent.click(screen.getByText(/merge into/i))
  expect(screen.getByRole('option', { name: 'Grocer Twin' })).toBeInTheDocument()
  expect(screen.queryByRole('option', { name: 'Someone Elses' })).not.toBeInTheDocument()
})
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd web && pnpm test -- MergeDialog`
Expected: FAIL — module not found.

- [ ] **Step 4: Build `MergeDialog`**

Use `ResponsiveDialog` for the shell (as `ClassificationList` does at line 459) and `fuzzyMatch` from `@/lib/fuzzy` for the picker filter, so search behaves like the list's. Render candidates as `role="option"` buttons. Include the source name, the optional `info` note, and the irreversibility warning.

- [ ] **Step 5: Add the mutations**

In `queries.ts`, one hook per kind. Each `onSuccess` must invalidate **more than its own list** — a merge changes transactions, recurring templates and budget limits:

```ts
export function useMergeCategory() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (v: { sourceId: string; targetId: string }) => {
      const res = await post('/api/v1/category/merge-category', v)
      trackEvent(METRICS.CLASSIFICATION_MERGE, { type: 'category' })
      return res
    },
    onSuccess: () => {
      // A stale budget would show the pre-merge limits and read as data loss.
      for (const key of [categoriesKey, transactionsKey, recurringKey, budgetKey]) {
        queryClient.invalidateQueries({ queryKey: key })
      }
    },
  })
}
```

Match the exact query-key names already used in this file.

- [ ] **Step 6: Wire the pages**

On each page pass an `extraActions` prop into `ClassificationList`:
```tsx
extraActions={(item) => [
  { label: t('classifications.common.merge.action'), onSelect: () => setMergeTarget(item) },
]}
```
Build `candidates` from the page's existing `own`-filtered array, not the raw query data. On `TagsPage`, `extraActions` must dispatch to the tag or label mutation based on `row.kind`, and candidates must be restricted to the same kind. On `CategoriesPage`, restrict candidates to the same `type`.

- [ ] **Step 7: Add the i18n strings**

Add to both locale files under `classifications.common.merge`: `action` ("Merge into…"), `title`, `confirm`, `warning` ("This cannot be undone."), and `envelope_info` for categories.

- [ ] **Step 8: Run tests and lint**

Run: `cd web && pnpm test && pnpm lint`
Expected: PASS, including `metrics-coverage.test.ts`.

- [ ] **Step 9: Commit**

```bash
git add web/ locales/
git commit -m "feat: merge classifications from the settings lists"
```

---

### Task 10: Full verification

- [ ] **Step 1: Run the full suite**

Run: `make test`
Expected: PASS — this includes the sqlite-vs-PostgreSQL `enginecompare` suite, which is what proves the label dedupe SQL agrees across engines.

- [ ] **Step 2: Run the repo suite against PostgreSQL**

Run: `make test-repo-pgsql`
Expected: PASS

- [ ] **Step 3: Confirm the coverage gate**

Run: `make go-test`
Expected: coverage ≥ 80.

- [ ] **Step 4: Commit any fixes**

```bash
git commit -m "test: fix cross-engine merge assertions"
```

---

## Self-Review

**Spec coverage:** Phase 0 → Task 1. Budget limit summing + re-point branch → Task 2. Payee/category/tag/label merges → Tasks 3-6. Ownership rules → tested in every one of Tasks 3-6. `mode=replace` removal → Task 7. apiparity/mcpparity/swagger → Task 8. Frontend, analytics, i18n → Task 9. enginecompare, pgsql repo suite, coverage → Task 10. Envelope membership is untouched by design, so it correctly has no task.

**Type consistency:** `MergeElements(ctx, oldExternalID, newExternalID)` is used identically in Tasks 2, 4 and 5. `{SourceId, TargetId}` is the DTO shape in Tasks 3-6 and the wire shape in Tasks 8-9. `CLASSIFICATION_MERGE` is defined in Task 9 Step 1 and used in Step 5.

**Known verification points for the implementer:** the exact join-table column names for labels (Task 6 Step 3), the budget repo's not-found convention (Task 2 Step 5), and the query-key names in `queries.ts` (Task 9 Step 5) must each be read from the source before writing the code, not assumed.
