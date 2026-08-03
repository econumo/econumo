# Reporting Labels — Plan 2: Budget Surfacing, Recurring, CSV

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface per-label period spend as a budget-neutral block in the budget view with click-through to the underlying transactions, let recurring templates carry labels, and split the budgeting tag from reporting labels in CSV export and import.

**Architecture:** The label aggregation is a **separate read-model method** from `CountSpending` — that query groups by `(category_id, tag_id, currency_id)` and the builder assumes **one bucket per row**, so joining a many-to-many into it would fan one transaction into N rows and double-count the element totals. Keeping labels on their own query and their own result slice is what makes the deliberate overlap safe. Labels never become `budgets_elements` rows.

**Tech Stack:** Go, hand-built dynamic SQL in `internal/budget/repo/read.go` (this repo branches by engine on purpose), `encoding/csv`, sqlc for the static queries.

**Spec:** `docs/superpowers/specs/2026-08-02-multi-tag-labels-design.md` — Delivery phases **3, 4 and 5**.

**Depends on:** Plan 1 (`2026-08-03-reporting-labels-01-backend-core.md`) must be complete — this plan uses `model.Label`, `labels`/`transactions_labels` tables, `label.ReadModel`, and `model.Transaction.LabelIDs`.

## Global Constraints

- Every schema change needs **two** migration files (sqlite + pgsql, same version). sqlite ids `TEXT`, pgsql `UUID`; sqlite timestamps `DATETIME`, pgsql `TIMESTAMP(0) WITHOUT TIME ZONE`.
- sqlc query `.sql` files must be **ASCII-only** (an em dash mangles sqlc v1.30's sqlite codegen).
- **Engine differences that bite in `internal/budget/repo/read.go`:** SQLite `SUM()` returns a float — format with `strconv.FormatFloat(v, 'f', 8, 64)`; PostgreSQL `SUM()` is exact NUMERIC scanned as text. SQLite datetime bounds must go through the existing `sqliteDatetime(...)` helper or the first-of-month row is silently dropped.
- An empty `IN ()` list is a no-op on SQLite but a **syntax error** on PostgreSQL — guard empty slices before building the clause, exactly as `CountSpending` does.
- JSON slices are initialised empty (`[]model.LabelSpendResult{}`), never nil — the builder already does this for `children` so the wire never carries `null`.
- Wire shapes frozen: `isArchived` int `0`/`1`; datetimes `"2006-01-02 15:04:05"`.
- The CSV export header row is a documented contract. The new column goes **immediately after `tag`**, shifting `payee,amount,date` right by one.
- TDD: failing test → minimal code → green → commit. One commit per task.
- Verify with `make go-test`; regenerate goldens with `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/` then **INSPECT the diff**.

## Names decided up front

| Thing | Name |
|---|---|
| Read-model method | `CountSpendingByLabel(ctx, accountIDs []vo.Id, start, end time.Time) ([]model.LabelSpendingRow, error)` |
| Aggregation row | `model.LabelSpendingRow{LabelID, CurrencyID, Amount string}` |
| Label metadata | `model.LabelMeta{ID, OwnerID, Name, Icon string; IsArchived bool}` |
| Wire block item | `model.LabelSpendResult` |
| Convert key | `"label-spent_" + labelID` |
| Drill-down param | `labelId` |
| CSV export column | `labels` (after `tag`), values joined `;` |
| CSV import mapping key | `labels`; separator field `labelsSeparator` |
| Labels-per-row cap | `maxLabelsPerImportRow = 10` |

---

## Phase 3 — Budget surfacing

### Task 1: Per-label spending read model

**Files:**
- Modify: `internal/model/budget_view.go` (add `LabelSpendingRow`, `LabelMeta`)
- Modify: `internal/budget/readmodel.go` (add `CountSpendingByLabel`, `LabelsForUsers` to the `ReadModel` interface)
- Modify: `internal/budget/repo/read.go` (implement both)
- Test: `internal/budget/repo/read_labels_test.go`

**Interfaces:**
- Consumes: `transactions_labels`, `labels` tables (Plan 1 Task 1).
- Produces: `ReadModel.CountSpendingByLabel(ctx, accountIDs []vo.Id, start, end time.Time) ([]model.LabelSpendingRow, error)` and `ReadModel.LabelsForUsers(ctx, userIDs []vo.Id) (map[string]model.LabelMeta, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/budget/repo/read_labels_test.go`:

```go
package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
)

// One expense carrying two labels must produce one row per label, each with the
// FULL amount. The overlap is the feature: "how much went to kid A" and "to kid
// B" are both 50, even though only 50 was spent.
func TestCountSpendingByLabelCountsFullAmountPerLabel(t *testing.T) {
	db := dbtest.New(t)
	// seed: user, account (currency C), one expense of 50.00 in period,
	// two labels L1 and L2, both linked to that transaction.
	// (mirror the fixture calls used by the existing CountSpending tests)

	rows, err := newReadRepo(t, db).CountSpendingByLabel(
		context.Background(),
		[]vo.Id{accountID},
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want one per label", len(rows))
	}
	for _, r := range rows {
		if !dbtest.DecimalEqual(r.Amount, "50") {
			t.Fatalf("label %s amount = %s, want the full 50", r.LabelID, r.Amount)
		}
	}
}

func TestCountSpendingByLabelReturnsNilForNoAccounts(t *testing.T) {
	rows, err := newReadRepo(t, dbtest.New(t)).CountSpendingByLabel(
		context.Background(), nil, time.Now(), time.Now().AddDate(0, 1, 0))
	if err != nil || rows != nil {
		t.Fatalf("empty account list must short-circuit; got %v, %v", rows, err)
	}
}
```

Read `internal/budget/repo/read_test.go` first and copy its harness/fixture calls verbatim; decimal assertions must compare normalized `vo.Decimal` values because sqlite text and pgsql NUMERIC differ textually.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/budget/repo/ -run Label -v`
Expected: FAIL — `CountSpendingByLabel undefined`

- [ ] **Step 3: Add the row and metadata types**

In `internal/model/budget_view.go`:

```go
// LabelSpendingRow is one (label, currency) spending total in a period. Unlike
// SpendingRow this is NOT grouped alongside category/tag: a transaction with N
// labels contributes N rows, each carrying the full amount, so these totals
// deliberately overlap and must never feed envelope math.
type LabelSpendingRow struct {
	LabelID    string
	CurrencyID string
	Amount     string
}

// LabelMeta is the rendering metadata for one label in the budget view.
type LabelMeta struct {
	ID         string
	OwnerID    string
	Name       string
	Icon       string
	IsArchived bool
}
```

- [ ] **Step 4: Declare the port methods**

In `internal/budget/readmodel.go`, add to the `ReadModel` interface:

```go
	// CountSpendingByLabel: per (label, currency) spending over [start, end) for
	// the given accounts. Deliberately separate from CountSpending: that query
	// assumes one bucket per row, and joining a many-to-many there would
	// double-count the element totals.
	CountSpendingByLabel(ctx context.Context, accountIDs []vo.Id, start, end time.Time) ([]model.LabelSpendingRow, error)

	// LabelsForUsers returns label metadata keyed by label id for the given
	// owners (the budget's account owners).
	LabelsForUsers(ctx context.Context, userIDs []vo.Id) (map[string]model.LabelMeta, error)
```

- [ ] **Step 5: Implement in the read repo**

In `internal/budget/repo/read.go`, add `CountSpendingByLabel` modelled directly on `CountSpending` (same file, ~line 329). Key points:

```go
func (r *ReadRepo) CountSpendingByLabel(ctx context.Context, accountIDs []vo.Id, start, end time.Time) ([]model.LabelSpendingRow, error) {
	// An empty IN () is a no-op on SQLite but a syntax error on PostgreSQL.
	if len(accountIDs) == 0 {
		return nil, nil
	}
	accArgs := idArgs(accountIDs)
	const sel = "SELECT SUM(t.amount) as amount, tl.label_id, a.currency_id " +
		"FROM transactions t " +
		"JOIN transactions_labels tl ON tl.transaction_id = t.id " +
		"LEFT JOIN accounts a ON t.account_id = a.id AND a.id IN (%s) " +
		"WHERE t.type = 0 AND t.spent_at >= %s AND t.spent_at < %s " +
		"GROUP BY tl.label_id, a.currency_id"
	var sql string
	var args []any
	if r.driver == "postgresql" {
		accIn := r.ph(1, len(accArgs))
		sql = fmt.Sprintf(sel, accIn, "$"+itoa(1+len(accArgs)), "$"+itoa(2+len(accArgs)))
		args = append(args, accArgs...)
		args = append(args, start, end)
	} else {
		accIn := r.ph(1, len(accArgs))
		// Bind the bounds as 'Y-m-d H:i:s' strings: a time.Time bound serializes
		// in a form that mis-compares against the stored TEXT at month
		// boundaries and drops the first-of-month row.
		sql = fmt.Sprintf(sel, accIn, "?", "?")
		args = append(args, accArgs...)
		args = append(args, sqliteDatetime(start), sqliteDatetime(end))
	}
	// scan: pgsql SUM is exact NUMERIC (scan *string); sqlite SUM is float
	// (scan *float64, format with strconv.FormatFloat(v, 'f', 8, 64)).
	// Skip rows whose currency_id is NULL, exactly as CountSpending does.
}
```

Add `LabelsForUsers` as a straightforward `SELECT id, user_id, name, icon, is_archived FROM labels WHERE user_id IN (...)`, with the same empty-slice guard, returning the map keyed by label id.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/budget/... -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/model/budget_view.go internal/budget
git commit -m "feat(budget): add per-label spending read model"
```

---

### Task 2: Labels block in the budget structure

**Files:**
- Modify: `internal/model/budget_dto.go` (add `LabelSpendResult`, add `Labels` to `StructureResult`)
- Modify: `internal/budget/builder_structure.go` (fetch labels + label spending)
- Modify: `internal/budget/builder_structure_build.go` (emit the block)
- Test: `internal/budget/api/label_block_test.go`

**Interfaces:**
- Consumes: `CountSpendingByLabel`, `LabelsForUsers`, the convertor's `BulkConvert`.
- Produces: `StructureResult.Labels []LabelSpendResult` in the `get-budget` response.

- [ ] **Step 1: Write the failing test**

Create `internal/budget/api/label_block_test.go`:

```go
// A 50.00 expense labelled kid-A AND kid-B shows 50.00 under EACH label, while
// the element totals (which the envelope math uses) stay at 50.00 overall.
func TestLabelBlockOverlapsWithoutInflatingElements(t *testing.T)

// Labels with no spend in the period are absent: labels have no limits, so
// spend is the only visibility trigger.
func TestLabelBlockHidesLabelsWithoutSpend(t *testing.T)

// A shared account's spend aggregates under the ACCOUNT OWNER's labels, so a
// collaborator's labels appear in the viewer's budget block.
func TestLabelBlockUsesAccountOwnerLabels(t *testing.T)

// The block is always a list, never null.
func TestLabelBlockIsEmptyListNotNull(t *testing.T)
```

Model the harness on `internal/budget/api/tag_limit_visibility_test.go`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/budget/api/ -run Label -v`
Expected: FAIL — `Labels` is not a field of `StructureResult`

- [ ] **Step 3: Add the wire type**

In `internal/model/budget_dto.go`:

```go
// LabelSpendResult is one reporting label in the budget's labels block. It has
// no budgeted/available fields on purpose: a label carries no limit and takes no
// part in envelope math, so only its period spend is meaningful. Amounts across
// labels deliberately overlap and do not sum to total spend.
type LabelSpendResult struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	IsArchived  int    `json:"isArchived"`
	Spent       string `json:"spent"`
	OwnerUserId string `json:"ownerUserId"`
}
```

and add to `StructureResult`:

```go
	Labels []LabelSpendResult `json:"labels"`
```

- [ ] **Step 4: Fetch labels and their spending**

In `internal/budget/builder_structure.go`, alongside the existing tag fetch (`f.tags`), populate `f.labels map[string]model.LabelMeta` from `LabelsForUsers` for the budget's **account owners** — read how `f.tags` is populated in this file and mirror the owner set exactly, so shared-account labels resolve the same way shared tags do.

Add a separate accumulation pass — do **not** touch `buildElementsSpending`:

```go
// buildLabelSpending accumulates per-label spending for the current period
// only. Labels have no carryover, so unlike elements there is no month-by-month
// "before" walk.
func (s *Service) buildLabelSpending(ctx context.Context, f *fetched) (map[string][]amountSpent, error) {
	rows, err := s.read.CountSpendingByLabel(ctx, f.includedAccountIDs, f.periodStart, f.periodEnd)
	if err != nil {
		return nil, err
	}
	out := map[string][]amountSpent{}
	for _, row := range rows {
		cid, perr := vo.ParseId(row.CurrencyID)
		if perr != nil {
			return nil, perr
		}
		out[row.LabelID] = append(out[row.LabelID], amountSpent{
			currencyID: cid, amount: vo.NewDecimal(row.Amount),
			periodStart: f.periodStart, periodEnd: f.periodEnd,
		})
	}
	return out, nil
}
```

- [ ] **Step 5: Emit the block**

In `internal/budget/builder_structure_build.go`, after the Tags section, queue the conversions using a distinct key namespace so they cannot collide with element keys:

```go
	// Labels convert into the BUDGET currency only: a label has no per-element
	// currency option because it has no element.
	for labelID, spends := range labelSpending {
		for _, a := range spends {
			toConvert[fmt.Sprintf("label-spent_%s", labelID)] = append(
				toConvert[fmt.Sprintf("label-spent_%s", labelID)], convItem(a, budgetCurrencyID))
		}
	}
```

(match the exact accumulation shape `addSpendingConvert` uses for `toConvert` — read it at the top of the same file.)

After the existing `BulkConvert` and element emission, build the block:

```go
	labels := []model.LabelSpendResult{}
	for labelID, meta := range f.labels {
		spent := get(fmt.Sprintf("label-spent_%s", labelID))
		// Spend is the only visibility trigger: a label has no limit that could
		// keep it on screen the way a budgeted-but-unspent tag stays visible.
		if spent.IsZero() {
			continue
		}
		labels = append(labels, model.LabelSpendResult{
			Id: labelID, Name: meta.Name, Icon: meta.Icon,
			IsArchived: boolToInt(meta.IsArchived), Spent: spent.String(),
			OwnerUserId: meta.OwnerID,
		})
	}
	sortByPositionThenID(labels, func(l model.LabelSpendResult) int { return 0 }, func(l model.LabelSpendResult) string { return l.Id })
```

If label `position` should drive the order (it should — the spec says "in `position` order"), carry `Position int16` on `model.LabelMeta` and sort on it instead of the constant `0`. Add `position` to the `LabelsForUsers` select.

Return it: `return model.StructureResult{Folders: folders, Elements: result, Labels: labels}, nil`.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/budget/... -v`
Expected: PASS — in particular the existing element/envelope tests must be **unchanged**, proving labels did not leak into budget math.

- [ ] **Step 7: Regenerate goldens and inspect**

Run: `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ && git diff internal/test/apiparity/testdata/golden/`
Expected: every `get-budget` golden gains `"labels": []`; **no element amount changes anywhere**. An element amount that moves means the aggregation leaked — stop and fix.

- [ ] **Step 8: Commit**

```bash
git add internal/model/budget_dto.go internal/budget internal/test/apiparity
git commit -m "feat(budget): surface a budget-neutral labels block"
```

---

### Task 3: Label drill-down

**Files:**
- Modify: `internal/model/budget_dto.go` (`BudgetTransactionListRequest` gains `LabelId`)
- Modify: `internal/budget/repo/read.go` (add `BudgetTransactionsByLabel`)
- Modify: `internal/budget/readmodel.go`, `internal/budget/txlist.go` (dispatch branch)
- Modify: `internal/budget/api/budget.go` (query param + swag annotation)
- Test: `internal/budget/api/label_drilldown_test.go`

**Interfaces:**
- Consumes: `transactions_labels`, the existing `budgetTxCols` column list.
- Produces: `GET /api/v1/budget/get-transaction-list?labelId=<id>` returning that label's transactions.

- [ ] **Step 1: Write the failing test**

Create `internal/budget/api/label_drilldown_test.go`:

```go
// Clicking a label chip lists exactly the transactions carrying it, including a
// transaction that also carries other labels.
func TestGetTransactionListByLabel(t *testing.T)

// labelId is mutually exclusive with the other selectors, like tagId/envelopeId.
func TestGetTransactionListRejectsLabelIdWithCategoryId(t *testing.T)
```

Mirror `internal/budget/api/tag_child_drilldown_test.go` for the harness.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/budget/api/ -run Label -v`
Expected: FAIL — unknown query param `labelId` (returns the "filter required" validation error)

- [ ] **Step 3: Add the request field**

In `internal/model/budget_dto.go`, add to `BudgetTransactionListRequest`:

```go
	LabelId       *string `json:"labelId"`
```

Extend the existing mutual-exclusion validation (in `internal/budget/txlist.go`, ~line 30) so `LabelId` cannot combine with `CategoryId`, `TagId`, `EnvelopeId`, or `Uncategorized`.

- [ ] **Step 4: Add the query**

In `internal/budget/repo/read.go`, add `BudgetTransactionsByLabel` mirroring `BudgetTransactionsByTag` (~line 531) — same `budgetTxCols` select list and scanner, but joining the link table:

```go
	// JOIN rather than a tag_id predicate: the link is many-to-many. DISTINCT is
	// unnecessary because the join is on a single label id, which can match a
	// given transaction at most once (composite PK).
	" FROM transactions t" +
	" JOIN transactions_labels tl ON tl.transaction_id = t.id AND tl.label_id = ?" +
	" LEFT JOIN accounts a ON t.account_id = a.id"
```

Keep the same account/period predicates and ordering as `BudgetTransactionsByTag`, and the same `sqliteDatetime` bound handling. Declare it on the `ReadModel` interface in `internal/budget/readmodel.go`.

- [ ] **Step 5: Add the dispatch branch**

In `internal/budget/txlist.go`, add a branch to the selector switch (~line 59), before the `default` that returns `CodeBudgetTransactionFilterRequired`:

```go
	case req.LabelId != nil:
		labelID, err := vo.ParseId(*req.LabelId)
		if err != nil {
			return nil, err
		}
		rows, err = s.read.BudgetTransactionsByLabel(ctx, budgetID, labelID, accountIDs, periodStart, periodEnd)
```

- [ ] **Step 6: Accept the query param**

In `internal/budget/api/budget.go`, add to the `GetTransactionList` handler's request build:

```go
		LabelId:       optQuery(q.Get("labelId")),
```

and add the swag line next to the existing `tagId` one:

```go
// @Param    labelId       query string false "Label id (reporting label)"
```

- [ ] **Step 7: Run tests, regenerate docs and goldens**

Run: `go test ./internal/budget/... -v && make swagger && UPDATE_GOLDEN=1 go test ./internal/test/apiparity/`
Expected: PASS; inspect the golden diff — a new drill-down scenario, nothing else moving.

- [ ] **Step 8: Commit**

```bash
git add internal/model/budget_dto.go internal/budget internal/test/apiparity docs
git commit -m "feat(budget): drill down into a label's transactions"
```

---

## Phase 4 — Recurring

### Task 4: `recurring_transactions_labels`

**Files:**
- Create: `internal/infra/storage/migrations/sqlite/20260804000000.sql`, `.../pgsql/20260804000000.sql`
- Create: `internal/infra/storage/sqlc/query/sqlite/recurring_labels.sql`, `.../pgsql/recurring_labels.sql`
- Modify: `internal/model/recurring.go` (entity gains `LabelIDs []vo.Id`)
- Modify: `internal/recurring/repo/` (replace/load methods)
- Test: `internal/recurring/repo/` integration test

**Interfaces:**
- Consumes: `labels`, `recurring_transactions`.
- Produces: table `recurring_transactions_labels`; repo methods `ReplaceLabels(ctx, recurringID vo.Id, labelIDs []vo.Id) error`, `LabelsByRecurringIDs(ctx, ids []vo.Id) (map[string][]string, error)`.

- [ ] **Step 1: Write the failing test**

Add to the recurring repo integration test: saving a template with two labels and re-loading returns both; re-saving with one label leaves exactly one link row (delete-then-insert, no duplicates).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/recurring/... -v`
Expected: FAIL — no such table `recurring_transactions_labels`

- [ ] **Step 3: Write the migrations**

Create `internal/infra/storage/migrations/sqlite/20260804000000.sql`:

```sql
-- Reporting labels on recurring templates. Posting a due template copies these
-- onto the created transaction, so a repeating shared expense stays attributed
-- without re-tagging it every month.
CREATE TABLE recurring_transactions_labels
(
    recurring_transaction_id TEXT NOT NULL
    , label_id               TEXT NOT NULL
    , PRIMARY KEY (recurring_transaction_id, label_id)
    , FOREIGN KEY (recurring_transaction_id) REFERENCES recurring_transactions (id) ON DELETE CASCADE
    , FOREIGN KEY (label_id) REFERENCES labels (id) ON DELETE CASCADE
);
CREATE INDEX label_id_idx_recurring_transactions_labels ON recurring_transactions_labels (label_id);
```

Create the pgsql sibling with `UUID` columns and the header `-- See the sqlite sibling for the semantics.`

- [ ] **Step 4: Write the queries and repo methods**

Create `recurring_labels.sql` for both engines with `DeleteRecurringLabels`, `InsertRecurringLabel` (`ON CONFLICT DO NOTHING`), and `ListLabelIDsByRecurring`. Implement `ReplaceLabels` / `LabelsByRecurringIDs` on the recurring repo exactly as Plan 1 Task 9 did for transactions (delete-then-insert inside the caller's tx; batch load for reads).

Add `LabelIDs []vo.Id` to the recurring entity and its state struct.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/recurring/... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/infra/storage internal/model internal/recurring
git commit -m "feat(recurring): persist labels on templates"
```

---

### Task 5: `labelIds` on recurring, and copy on post

**Files:**
- Modify: `internal/model/recurring_dto.go` (create/update requests + result gain `labelIds`)
- Modify: `internal/recurring/` create/update use cases and the post-template use case
- Test: `internal/recurring/api/` endpoint tests
- Modify: `internal/test/apiparity/` goldens

**Interfaces:**
- Consumes: recurring `ReplaceLabels` / `LabelsByRecurringIDs`, transaction `ReplaceLabels`.
- Produces: wire field `labelIds []string` on recurring create/update/read; posting copies template labels onto the created transaction.

- [ ] **Step 1: Write the failing test**

```go
// Posting a due template copies its labels onto the created transaction.
func TestPostRecurringCopiesLabels(t *testing.T)

// Posting is idempotent on the client-supplied transaction id: a re-post must
// not duplicate label links.
func TestPostRecurringTwiceDoesNotDuplicateLabels(t *testing.T)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/recurring/api/ -v`
Expected: FAIL — unknown field `labelIds`

- [ ] **Step 3: Add the wire fields and validation**

Add `LabelIds []string \`json:"labelIds"\`` to the recurring create/update requests and `LabelIds []string \`json:"labelIds"\`` (initialised empty, never nil) to the recurring result DTO. Validate ownership against the template's account owner, mirroring Plan 1 Task 10.

- [ ] **Step 4: Copy labels when posting**

In the post-template use case, after the transaction is built and saved inside the existing transaction boundary, call the transaction repo's `ReplaceLabels` with the template's label ids. Because `ReplaceLabels` is delete-then-insert and posting is already idempotent on the client-supplied transaction id, a re-post rewrites the same set rather than duplicating it.

- [ ] **Step 5: Run tests, regenerate goldens, inspect**

Run: `go test ./internal/recurring/... -v && UPDATE_GOLDEN=1 go test ./internal/test/apiparity/ && git diff internal/test/apiparity/testdata/golden/`
Expected: recurring goldens gain `"labelIds": []`.

- [ ] **Step 6: Commit**

```bash
git add internal/model internal/recurring internal/test/apiparity docs
git commit -m "feat(recurring): carry labels and copy them when posting"
```

---

## Phase 5 — CSV

### Task 6: Export the `labels` column

**Files:**
- Modify: `internal/transaction/export.go` (`exportHeaders`, `buildExportRow`, `resolveNames`)
- Modify: `internal/transaction/ports.go` (label-name lookup)
- Test: `internal/transaction/export_test.go`

**Interfaces:**
- Consumes: `Transaction.LabelIDs`, a label-name lookup port.
- Produces: header row `transaction_id,account_name,account_currency,category,description,tag,labels,payee,amount,date`; the `labels` cell is `;`-joined label names in position order.

- [ ] **Step 1: Write the failing test**

```go
func TestExportHeaderPutsLabelsAfterTag(t *testing.T) {
	want := []string{"transaction_id", "account_name", "account_currency",
		"category", "description", "tag", "labels", "payee", "amount", "date"}
	// assert exportHeaders == want
}

func TestExportJoinsLabelNamesWithSemicolon(t *testing.T) {
	// a transaction with labels "Kid A" and "Kid B" (positions 0,1) exports
	// "Kid A;Kid B" in the labels column
}

func TestExportWritesEmptyLabelsForTransferRecipientRow(t *testing.T) {
	// the recipient row writes "" for labels, exactly as it does for tag
}

func TestExportSanitizesEachLabelNameBeforeJoining(t *testing.T) {
	// a label named "=cmd" exports as "'=cmd" (formula-injection guard applied
	// per name, BEFORE the join, so the guard is not defeated by the delimiter)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/transaction/ -run Export -v`
Expected: FAIL — header has 9 columns, want 10

- [ ] **Step 3: Insert the column**

In `internal/transaction/export.go`, insert `"labels"` into `exportHeaders` immediately after `"tag"`. In `resolveNames`, resolve each of the transaction's label ids to a name (ordered by label position) and join:

```go
// Each name is sanitized BEFORE joining: sanitizing the joined string would let
// a leading "=" on the second name through.
parts := make([]string, 0, len(labelIDs))
for _, id := range orderedLabelIDs {
	parts = append(parts, sanitizeExportValue(labelNames[id]))
}
labels := strings.Join(parts, ";")
```

In `buildExportRow`, place `labels` at index 6 (after `tag`) — and write `""` on the transfer recipient row alongside the empty `tag`.

Add the lookup to `internal/transaction/ports.go` next to the existing `TagName`, e.g. `LabelNames(ctx context.Context, ids []vo.Id) (map[string]string, error)`, wired in `internal/server`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/transaction/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/transaction internal/server
git commit -m "feat(export): add a labels column after tag"
```

---

### Task 7: Import the `labels` column with a configurable separator

**Files:**
- Modify: `internal/model/transaction_view.go` (`ImportMapping` gains `Labels`; `ImportRequest` gains `LabelIds`, `LabelsSeparator`)
- Modify: `internal/transaction/api/import.go` (`parseImportMapping`, form fields)
- Modify: `internal/transaction/import.go`, `internal/transaction/import_parse.go`
- Modify: `internal/transaction/ports.go` (`LabelsByOwner`, `CreateLabel`)
- Test: `internal/transaction/import_test.go`

**Interfaces:**
- Consumes: label repo via new ports.
- Produces: mapping key `labels`; form fields `labelsSeparator` (default `;`) and `labelIds` (comma-joined override).

- [ ] **Step 1: Write the failing test**

```go
// Default separator is ";" and each piece is name-matched, auto-creating misses
// exactly as the tag column does.
func TestImportSplitsLabelsOnDefaultSemicolon(t *testing.T)

// A CSV from another tool can use any delimiter.
func TestImportSplitsLabelsOnCustomSeparator(t *testing.T)

// Blank cell -> no labels; blank pieces dropped; duplicates deduped
// case-insensitively.
func TestImportLabelsBlankAndDuplicateHandling(t *testing.T)

// The labelIds override applies the same labels to every row and is
// belongs-to checked against the account owner.
func TestImportLabelIdsOverrideRejectsForeignLabel(t *testing.T)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/transaction/ -run ImportLabel -v`
Expected: FAIL — `Labels` is not a field of `ImportMapping`

- [ ] **Step 3: Add the model fields**

`internal/model/transaction_view.go`: add `Labels string` to `ImportMapping`; add to `ImportRequest`:

```go
	LabelIds        *string // comma-joined id list; applies to every row
	LabelsSeparator string  // splits the mapped labels cell; defaults to ";"
```

- [ ] **Step 4: Parse the new form fields**

In `internal/transaction/api/import.go`, add `m.Labels = get("labels")` to `parseImportMapping`, and read the two new multipart fields. Default the separator when absent or empty:

```go
	sep := r.FormValue("labelsSeparator")
	if sep == "" {
		sep = ";"
	}
```

- [ ] **Step 5: Resolve labels per row**

In `internal/transaction/import.go`, mirror the tag find-or-create path. Build a case-insensitive `labelByName` cache from `LabelsByOwner` (reuse `newNamedCache` from `import_parse.go`), then per row:

```go
	// The whole cell is one field; split it on the caller's separator, since a
	// CSV from another tool may use any delimiter.
	pieces := strings.Split(cell, sep)
	seen := map[string]struct{}{}
	ids := make([]vo.Id, 0, len(pieces))
	for _, p := range pieces {
		name := strings.TrimSpace(p)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		id, err := findOrCreateNamed(...)  // auto-creates on miss, like tags
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}
```

Attach the ids via the transaction repo's `ReplaceLabels` in the same tx as the row's `Save`. Add `LabelsByOwner` and `CreateLabel(ctx, ownerID vo.Id, name string) (vo.Id, error)` to `internal/transaction/ports.go` and wire the adapters in `internal/server`.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/transaction/... -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/model internal/transaction internal/server
git commit -m "feat(import): map a labels column with a configurable separator"
```

---

### Task 8: Import guardrails

**Files:**
- Modify: `internal/transaction/import.go`
- Test: `internal/transaction/import_test.go`

**Interfaces:**
- Consumes: the label resolution from Task 7.
- Produces: `maxLabelsPerImportRow = 10`, enforced as a row-level error.

- [ ] **Step 1: Write the failing test**

```go
// Auto-create is convenient but a user who maps the description column to
// labels would otherwise mass-create junk - a bigger blast radius than tags,
// since every cell now splits into many values.
func TestImportRejectsRowWithTooManyLabels(t *testing.T) {
	// 11 distinct pieces in one cell -> that ROW fails with a row-level error;
	// the rest of the import still proceeds
}

// A piece outside the 3-64 rune name rule is reported, not silently dropped.
func TestImportReportsInvalidLabelNameAsRowError(t *testing.T) {
	// "AB" (2 runes) -> row error mentioning the label name rule
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/transaction/ -run TestImportRejectsRow -v`
Expected: FAIL — the row succeeds and creates 11 labels

- [ ] **Step 3: Enforce the cap**

In `internal/transaction/import.go`:

```go
// A mapped column that is not really a label list (e.g. description) would
// otherwise mass-create junk labels. Cap per row and surface it as a row error
// rather than silently truncating, so the user sees the mis-mapping.
const maxLabelsPerImportRow = 10
```

Check the deduped count before resolving; over the cap, return a row-level error. Let the label name validation error from `CreateLabel` propagate as a row error rather than swallowing it.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/transaction/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/transaction
git commit -m "feat(import): cap labels per row and report invalid names"
```

> The **"this import will create N new labels" preview** is a frontend concern — the SPA already parses the CSV locally, so it can diff the distinct label values against `useLabels()` without a backend call. It is specified in Plan 3.

---

### Task 9: Full-suite and engine parity

**Files:** none (verification only)

- [ ] **Step 1: Run the smoke tier**

Run: `make go-test`
Expected: PASS, coverage ≥ `GO_COVER_MIN`

- [ ] **Step 2: Run the repo suite against PostgreSQL**

Run: `make test-repo-pgsql`
Expected: PASS — exercises the pgsql `CountSpendingByLabel` / `BudgetTransactionsByLabel` SQL, which branches by engine and is the highest-risk code in this plan.

- [ ] **Step 3: Run the engine-comparison suite**

Run: `make test`
Expected: PASS — byte-identical responses on both engines, including the labels block (watch for sqlite float-vs-NUMERIC formatting in `Spent`).

- [ ] **Step 4: Commit any adjustments**

```bash
git add -A
git commit -m "test(labels): confirm engine parity for budget, recurring and CSV"
```

---

## Self-Review

**Spec coverage (phases 3–5):** separate aggregation query → Task 1; budget-neutral block with overlap + owner-scoped shared labels → Task 2; drill-down via the budget module's `get-transaction-list` → Task 3; recurring join table → Task 4; recurring wire + copy-on-post → Task 5; export column after `tag`, `;`-joined → Task 6; import mapping key + configurable separator + `labelIds` override → Task 7; per-row cap + name-validation row errors → Task 8; engine parity → Task 9.

**Deliberately deferred:** the import's new-label **count preview** (frontend, Plan 3); MCP tools and analytics keys (Plan 3, so `mcpparity` regenerates once); all UI.

**Risk flagged for the reviewer:** Task 2 is the one place a mistake corrupts existing behaviour. The regression signal is precise — if any **element** amount changes in the `get-budget` goldens, the label aggregation has leaked into the element buckets and must be fixed before proceeding.

**Type consistency:** `LabelSpendingRow` (read row) → `LabelMeta` (metadata, carries `Position`) → `LabelSpendResult` (wire) are distinct and used consistently. `ReplaceLabels`/`LabelsBy*IDs` keep the same signature shape across the transaction (Plan 1) and recurring (Task 4) repos. The convert-key namespace `label-spent_` cannot collide with the element keys `spent_`/`spent-budget_`/`spent-before_`.
