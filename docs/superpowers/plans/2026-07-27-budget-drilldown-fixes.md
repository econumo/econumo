# Budget Drill-Down Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix two defects in the budget transaction drill-down — a category nested under a tag lists the complement of the transactions it displays, and SQLite silently omits 1st-of-month rows from every drill-down list.

**Architecture:** Both fixes are small and surgical. The tag fix is almost entirely frontend: the backend already supports a tag+category intersection (`BudgetTransactionsByTag` with a category filter, reachable via an existing but unused dispatcher branch), and the frontend simply never sends the parent tag id. The boundary fix binds SQLite date bounds as `'Y-m-d H:i:s'` strings, matching what `CountSpending` already does and documents.

**Tech Stack:** Go 1.x (stdlib `net/http`, hand-written SQL in `internal/budget/repo`), React 19 + TypeScript, vitest + testing-library, SQLite (default) and PostgreSQL.

This is Phase 1 of `docs/superpowers/specs/2026-07-27-optional-transaction-category-design.md`. Phase 2 (optional transaction category + the Uncategorized row) is a separate plan and must not be started here.

## Global Constraints

- Branch name: `bug/budget-drilldown` — this is a bug fix, so `bug/<slug>`, not `feature/<slug>`.
- Any SQL change must be made in **both** dialect branches in lockstep; `enginecompare` asserts byte-identical SQLite and PostgreSQL responses.
- Never hand-edit a golden file. Regenerate with `UPDATE_GOLDEN=1` and inspect the diff.
- Comments only for non-obvious *why* — no comments restating a symbol's name. Never reference the removed PHP/Symfony implementation.
- Frontend: `pnpm exec tsc -b` must pass. vitest and oxlint do **not** type-check.
- Run all commands from the repo root unless a step says otherwise.
- The wire `spent` value is positive and must never be rendered negated.

## File Structure

| File | Responsibility | Change |
| --- | --- | --- |
| `internal/budget/repo/read_integration_test.go` | Repo-level integration tests against migrated SQLite | Modify — add 3 tests |
| `internal/budget/repo/read.go` | Hand-written budget report SQL | Modify — `:481-509`, `:512-554` date binding |
| `internal/budget/api/tag_child_drilldown_test.go` | End-to-end regression for the tag-child drill-down | Create |
| `web/src/features/budgets/BudgetTransactionsDialog.tsx` | Target type + request params | Modify — `:27-34`, `:58-66` |
| `web/src/features/budgets/BudgetTransactionsDialog.test.tsx` | Param-mapping regression for the parent-tag spread | Create (final fix wave) |
| `web/src/features/budgets/BudgetTable.tsx` | Row rendering, builds the click target | Modify — `:219` |
| `web/src/features/budgets/BudgetTable.test.tsx` | Table behavior tests | Modify — update `:145-153`, add 1 test |

No schema, migration, or sqlc change anywhere in this plan.

---

### Task 1: Characterize `BudgetTransactionsByCategories`

This query has **no test at all** today, which is how its `tag_id IS NULL` clause
went unexamined. Lock in its real semantics before touching anything. Test-only —
no production change in this task, and all tests must pass on the first run.

**Files:**
- Test: `internal/budget/repo/read_integration_test.go` (append)

**Interfaces:**
- Consumes: `newReadRepo(t) (*budgetrepo.ReadRepo, *dbtest.DB)`, `seedCategory(t, db, id, userID)`, `seedExpense(t, db, id, account, category, amount, spentAt)`, package constants `userA`, `acctA`, `usdID`, and `fixture.New(t, db).Tag(...)` / `.Transaction(...)` — all already defined in this file.
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Write the characterization test**

Append to `internal/budget/repo/read_integration_test.go`:

```go
// BudgetTransactionsByCategories backs the drill-down of a standalone category
// row and of an envelope's children. Both display the UNTAGGED bucket, so the
// query deliberately excludes tagged rows ("t.tag_id IS NULL"). A category
// nested under a tag shows the opposite bucket and must not use this query.
func TestBudgetReadRepo_BudgetTransactionsByCategories(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	cat := "c0000000-0000-0000-0000-0000000000c1"
	tag := "7a000000-0000-0000-0000-0000000000b1"
	seedCategory(t, db, cat, userA)
	f := fixture.New(t, db)
	f.Tag(fixture.Tag{ID: tag, UserID: userA, Name: "Tag"})
	// The only row this query should return.
	seedExpense(t, db, "73000000-0000-0000-0000-000000000001", acctA, cat, "10.00", "2024-04-05 00:00:00")
	// Same category but tagged: excluded by design.
	f.Transaction(fixture.Transaction{ID: "73000000-0000-0000-0000-000000000002", UserID: userA, AccountID: acctA, CategoryID: cat, TagID: tag, Type: 0, Amount: "99.00", SpentAt: "2024-04-06 00:00:00"})
	// An income in the same category is not budget spending (type != 0).
	f.Transaction(fixture.Transaction{ID: "73000000-0000-0000-0000-000000000003", UserID: userA, AccountID: acctA, CategoryID: cat, Type: 1, Amount: "50.00", SpentAt: "2024-04-07 00:00:00"})
	// Outside the period.
	seedExpense(t, db, "73000000-0000-0000-0000-000000000004", acctA, cat, "77.00", "2024-05-02 00:00:00")

	start := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	catIDs := []vo.Id{vo.MustParseId(cat)}
	accIDs := []vo.Id{vo.MustParseId(acctA)}

	rows, err := read.BudgetTransactionsByCategories(ctx, catIDs, accIDs, start, end)
	if err != nil {
		t.Fatalf("BudgetTransactionsByCategories: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 untagged in-period expense, got %d: %+v", len(rows), rows)
	}
	if rows[0].ID != "73000000-0000-0000-0000-000000000001" {
		t.Errorf("wrong row returned: %q", rows[0].ID)
	}

	// Empty id sets short-circuit: "IN ()" is a no-op on SQLite but a PostgreSQL
	// syntax error, so both lists must be guarded.
	if none, err := read.BudgetTransactionsByCategories(ctx, nil, accIDs, start, end); err != nil || none != nil {
		t.Errorf("empty category ids should be nil,nil; got %v, %v", none, err)
	}
	if none, err := read.BudgetTransactionsByCategories(ctx, catIDs, nil, start, end); err != nil || none != nil {
		t.Errorf("empty account ids should be nil,nil; got %v, %v", none, err)
	}
}
```

- [ ] **Step 2: Run the test — it must PASS**

Run: `go test ./internal/budget/repo/ -run TestBudgetReadRepo_BudgetTransactionsByCategories -v`
Expected: PASS. This is a characterization test of existing correct behavior, not a bug reproduction. If it FAILS, stop and report — the query does something other than what this plan assumes, and Task 2 onward rests on it.

- [ ] **Step 3: Commit**

```bash
git add internal/budget/repo/read_integration_test.go
git commit -m "test: characterize BudgetTransactionsByCategories

The query had no test at all; its deliberate tag_id IS NULL clause was
unverified, which is how the tag-child drill-down bug went unnoticed."
```

---

### Task 2: Fix the SQLite month-boundary in both drill-down queries

`CountSpending` binds its `spent_at` bounds as `'Y-m-d H:i:s'` strings and
documents why at `read.go:352-355`: a `time.Time` bound "does not compare
correctly against the stored datetime TEXT at month boundaries (it drops the
first-of-month row)". Both drill-down queries bind raw `time.Time` anyway, so a
1st-of-month transaction counts toward the displayed total but is missing from
the list.

**Files:**
- Test: `internal/budget/repo/read_integration_test.go` (append)
- Modify: `internal/budget/repo/read.go:481-509` (`BudgetTransactionsByCategories`), `internal/budget/repo/read.go:512-554` (`BudgetTransactionsByTag`)

**Interfaces:**
- Consumes: `sqliteDatetime(t time.Time) string` — already defined at `read.go:122`.
- Produces: no signature changes. Both functions keep their exact signatures.

- [ ] **Step 1: Write the failing test**

Append to `internal/budget/repo/read_integration_test.go`:

```go
// A first-of-month transaction is counted in the row total by CountSpending, so
// the drill-down list must contain it too — otherwise the list silently
// contradicts the number above it. SQLite only: the bounds must be bound as
// 'Y-m-d H:i:s' strings, not as time.Time (see sqliteDatetime).
func TestBudgetReadRepo_Drilldown_MonthBoundary(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	cat := "c0000000-0000-0000-0000-0000000000c1"
	tag := "7a000000-0000-0000-0000-0000000000c2"
	seedCategory(t, db, cat, userA)
	f := fixture.New(t, db)
	f.Tag(fixture.Tag{ID: tag, UserID: userA, Name: "Tag"})
	seedExpense(t, db, "74000000-0000-0000-0000-000000000001", acctA, cat, "10.00", "2024-04-01 00:00:00")
	f.Transaction(fixture.Transaction{ID: "74000000-0000-0000-0000-000000000002", UserID: userA, AccountID: acctA, CategoryID: cat, TagID: tag, Type: 0, Amount: "20.00", SpentAt: "2024-04-01 00:00:00"})
	// The previous month must stay excluded (the lower bound is inclusive, the
	// upper bound exclusive).
	seedExpense(t, db, "74000000-0000-0000-0000-000000000003", acctA, cat, "99.00", "2024-03-31 23:59:59")
	seedExpense(t, db, "74000000-0000-0000-0000-000000000004", acctA, cat, "88.00", "2024-05-01 00:00:00")

	start := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	accIDs := []vo.Id{vo.MustParseId(acctA)}

	byCat, err := read.BudgetTransactionsByCategories(ctx, []vo.Id{vo.MustParseId(cat)}, accIDs, start, end)
	if err != nil {
		t.Fatalf("BudgetTransactionsByCategories: %v", err)
	}
	if len(byCat) != 1 {
		t.Fatalf("want the Apr 1 untagged expense in the list, got %d rows: %+v", len(byCat), byCat)
	}
	if byCat[0].ID != "74000000-0000-0000-0000-000000000001" {
		t.Errorf("wrong row: %q", byCat[0].ID)
	}

	byTag, err := read.BudgetTransactionsByTag(ctx, vo.MustParseId(tag), nil, accIDs, start, end)
	if err != nil {
		t.Fatalf("BudgetTransactionsByTag: %v", err)
	}
	if len(byTag) != 1 {
		t.Fatalf("want the Apr 1 tagged expense in the list, got %d rows: %+v", len(byTag), byTag)
	}
	if byTag[0].ID != "74000000-0000-0000-0000-000000000002" {
		t.Errorf("wrong row: %q", byTag[0].ID)
	}
}
```

- [ ] **Step 2: Run the test to verify it FAILS**

Run: `go test ./internal/budget/repo/ -run TestBudgetReadRepo_Drilldown_MonthBoundary -v`
Expected: FAIL — "want the Apr 1 untagged expense in the list, got 0 rows". Both sub-assertions should fail, one per query. If it passes, stop and report: the boundary bug is not reproducible and this task should be reconsidered rather than "fixed".

- [ ] **Step 3: Fix `BudgetTransactionsByCategories`**

In `internal/budget/repo/read.go`, the date args are currently appended once
after the if/else (`:500-502`), so they cannot differ per dialect. Move the
append into each branch. Replace the body from `accArgs := idArgs(accountIDs)`
through `args = append(args, start, end)` with:

```go
	accArgs := idArgs(accountIDs)
	catArgs := idArgs(categoryIDs)
	var sql string
	args := make([]any, 0, len(accArgs)+len(catArgs)+2)
	args = append(args, accArgs...)
	args = append(args, catArgs...)
	if r.driver == "postgresql" {
		accIn := r.ph(1, len(accArgs))
		catIn := r.ph(1+len(accArgs), len(catArgs))
		dStart := "$" + itoa(1+len(accArgs)+len(catArgs))
		dEnd := "$" + itoa(2+len(accArgs)+len(catArgs))
		sql = "SELECT " + budgetTxCols + " FROM transactions t JOIN accounts a ON a.id = t.account_id WHERE t.account_id IN (" + accIn + ") AND t.category_id IN (" + catIn + ") AND t.type = 0 AND t.tag_id IS NULL AND t.spent_at >= " + dStart + " AND t.spent_at < " + dEnd + " ORDER BY t.spent_at DESC"
		args = append(args, start, end)
	} else {
		accIn := r.ph(1, len(accArgs))
		catIn := r.ph(1, len(catArgs))
		sql = "SELECT " + budgetTxCols + " FROM transactions t JOIN accounts a ON a.id = t.account_id WHERE t.account_id IN (" + accIn + ") AND t.category_id IN (" + catIn + ") AND t.type = 0 AND t.tag_id IS NULL AND t.spent_at >= ? AND t.spent_at < ? ORDER BY t.spent_at DESC"
		// Bind the bounds as 'Y-m-d H:i:s' strings (see sqliteDatetime): a
		// time.Time bound does not compare correctly against the stored
		// datetime TEXT and drops the first-of-month row.
		args = append(args, sqliteDatetime(start), sqliteDatetime(end))
	}
```

The SQL strings themselves are unchanged — only the argument binding moves.

- [ ] **Step 4: Fix `BudgetTransactionsByTag`**

In the same file, in the SQLite branch of `BudgetTransactionsByTag`, replace:

```go
		where += " AND t.spent_at >= ? AND t.spent_at < ?"
		args = append(args, start, end)
```

with:

```go
		where += " AND t.spent_at >= ? AND t.spent_at < ?"
		// See sqliteDatetime: a time.Time bound drops the first-of-month row.
		args = append(args, sqliteDatetime(start), sqliteDatetime(end))
```

Leave the PostgreSQL branch (`args = append(args, start, end)` inside the
`r.driver == "postgresql"` block) untouched — pg compares timestamps correctly.

- [ ] **Step 5: Run the tests to verify they PASS**

Run: `go test ./internal/budget/repo/ -v`
Expected: PASS, including `TestBudgetReadRepo_Drilldown_MonthBoundary`, `TestBudgetReadRepo_BudgetTransactionsByCategories` from Task 1, and the pre-existing `TestBudgetReadRepo_BudgetTransactionsByTag` and `TestBudgetReadRepo_CountSpending_MonthBoundary`.

- [ ] **Step 6: Verify PostgreSQL is unaffected**

Run: `make test-repo-pgsql`
Expected: PASS. This reruns the repo suite against PostgreSQL, confirming the pg branch still binds correctly.

- [ ] **Step 7: Commit**

```bash
git add internal/budget/repo/read.go internal/budget/repo/read_integration_test.go
git commit -m "fix: include first-of-month rows in budget drill-down on SQLite

CountSpending binds its spent_at bounds as 'Y-m-d H:i:s' strings and
documents that a time.Time bound drops the first-of-month row. Both
drill-down queries bound raw time.Time anyway, so a 1st-of-month
transaction counted toward the row total but was missing from the list."
```

---

### Task 3: End-to-end regression for the tag-child drill-down

Prove at the API level what the correct behavior is, and that the backend
already supports it. This test fails only on the frontend's behalf — the backend
path is reachable, so this task also **documents** the untagged-vs-tagged split
so a future reader does not "fix" `BudgetTransactionsByCategories` instead.

**Files:**
- Create: `internal/budget/api/tag_child_drilldown_test.go`

**Interfaces:**
- Consumes, all already defined in `package api_test` (`internal/budget/api/harness_test.go` and siblings): `newHarness(t) *harness`, `h.token(t) string`, `h.do(t, method, path, token, body) (int, envelope)` where the envelope exposes `.Data` and `.raw`, `mustUnmarshal[T](t, data) T`, `h.db`, and the seeded constants `budgetID1`, `tagID`, `catID`, `accountID`, `seedUserID`, `usdID`.
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Write the test**

Create `internal/budget/api/tag_child_drilldown_test.go`:

```go
package api_test

import (
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

// txListView is the slice of get-transaction-list these tests care about.
type txListView struct {
	Items []struct {
		Id     string `json:"id"`
		Amount string `json:"amount"`
	} `json:"items"`
}

// A category listed as a child of a tag displays the tag-and-category
// intersection. Its drill-down must return exactly those transactions. Sending
// categoryId alone selects the "t.tag_id IS NULL" branch, which returns the
// complement — the reported bug.
func TestTagChildDrilldown_ReturnsTheIntersection(t *testing.T) {
	h := newHarness(t)
	tok := h.token(t)
	h.do(t, http.MethodPost, "/api/v1/budget/create-budget", tok,
		map[string]any{"id": budgetID1, "name": "Drilldown Budget", "currencyId": usdID, "startDate": "2024-04-01"})

	f := fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})
	// Same category, same period: one tagged, one not.
	taggedID := f.Transaction(fixture.Transaction{
		ID: "eeee2222-0000-7000-8000-000000000001", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, TagID: tagID, Type: 0, Amount: "42.00", SpentAt: "2024-04-10 00:00:00",
	})
	untaggedID := f.Transaction(fixture.Transaction{
		ID: "eeee2222-0000-7000-8000-000000000002", UserID: seedUserID, AccountID: accountID,
		CategoryID: catID, Type: 0, Amount: "7.00", SpentAt: "2024-04-11 00:00:00",
	})

	base := "/api/v1/budget/get-transaction-list?budgetId=" + budgetID1 + "&periodStart=2024-04-01"

	// The tag child: tagId AND categoryId together.
	st, b := h.do(t, http.MethodGet, base+"&tagId="+tagID+"&categoryId="+catID, tok, nil)
	if st != http.StatusOK {
		t.Fatalf("get-transaction-list=%d body=%s", st, b.raw)
	}
	got := mustUnmarshal[txListView](t, b.Data)
	if len(got.Items) != 1 {
		t.Fatalf("tag child drill-down must return only the tagged transaction, got %d: %s", len(got.Items), b.Data)
	}
	if got.Items[0].Id != taggedID {
		t.Errorf("tag child drill-down returned %q, want the tagged transaction %q", got.Items[0].Id, taggedID)
	}

	// The standalone category row shows the untagged bucket, and its drill-down
	// must keep matching that. This is the branch the tag child wrongly used.
	_, b = h.do(t, http.MethodGet, base+"&categoryId="+catID, tok, nil)
	got = mustUnmarshal[txListView](t, b.Data)
	if len(got.Items) != 1 || got.Items[0].Id != untaggedID {
		t.Errorf("category-only drill-down must return only the untagged transaction %q; got %s", untaggedID, b.Data)
	}

	// The tag row itself shows everything tagged, across categories.
	_, b = h.do(t, http.MethodGet, base+"&tagId="+tagID, tok, nil)
	got = mustUnmarshal[txListView](t, b.Data)
	if len(got.Items) != 1 || got.Items[0].Id != taggedID {
		t.Errorf("tag drill-down must return the tagged transaction %q; got %s", taggedID, b.Data)
	}
}
```

- [ ] **Step 2: Run the test to verify it PASSES**

Run: `go test ./internal/budget/api/ -run TestTagChildDrilldown -v`
Expected: PASS. The backend already handles `tagId`+`categoryId` (`internal/budget/txlist.go:49-62`); only the frontend fails to send both. If this FAILS, the bug is not purely frontend — stop and report before starting Task 4.

- [ ] **Step 3: Commit**

```bash
git add internal/budget/api/tag_child_drilldown_test.go
git commit -m "test: lock the tag-child drill-down contract end to end

Pins the three drill-down shapes: tag+category returns the
intersection, category alone the untagged bucket, tag alone everything
tagged. No golden contained a tag with children, so nothing covered this."
```

---

### Task 4: Send the parent tag from the frontend

The actual bug fix. `BudgetTable.tsx:219` builds a child's click target without
the parent, so `BudgetTransactionsDialog` sends `categoryId` alone.

**Files:**
- Modify: `web/src/features/budgets/BudgetTransactionsDialog.tsx:27-34` (the target type) and `:58-66` (request params)
- Modify: `web/src/features/budgets/BudgetTable.tsx:219`
- Test: `web/src/features/budgets/BudgetTable.test.tsx` (update one test, add one)

**Interfaces:**
- Consumes: `BudgetElementType` from `@/api/dto/budget` (`ENVELOPE: 0`, `CATEGORY: 1`, `TAG: 2`); `BudgetTransactionsParams` in `web/src/api/budget.ts:114-120`, which already declares optional `categoryId` and `tagId` and already serializes both when present (`:124-125`) — **no change needed there**.
- Produces: `BudgetTransactionsTarget` gains an optional `parent?: { id: Id; type: BudgetElementType }`.

- [ ] **Step 1: Write the failing test**

In `web/src/features/budgets/BudgetTable.test.tsx`, append:

```tsx
it('clicking a tag child spent amount reports the parent tag', async () => {
  const user = userEvent.setup()
  const onSpentClick = vi.fn()
  renderTable((budget) => {
    budget.structure.elements.push({
      id: 'tag-travel', type: 2, name: 'Travel', icon: 'flight', currencyId: null, isArchived: 0,
      folderId: null, position: 3, budgeted: '0', available: '-30', spent: '30', budgetSpent: '30',
      ownerUserId: 'u1',
      children: [{ id: 'cat-hotel', type: 1, name: 'Hotel', icon: 'hotel', isArchived: 0, spent: '30', budgetSpent: '30', ownerUserId: 'u1' }],
    })
  }, { onSpentClick })
  const travel = await screen.findByTestId('element-tag-travel')
  await user.click(within(travel).getByText('Travel'))
  await user.click(await screen.findByRole('button', { name: 'transactions Hotel' }))
  // Without the parent the dialog sends categoryId alone, and the backend
  // returns the untagged complement of what this row displays.
  expect(onSpentClick).toHaveBeenCalledWith({
    id: 'cat-hotel', type: 1, name: 'Hotel', icon: 'hotel', currencyId: null,
    parent: { id: 'tag-travel', type: 2 },
  })
})
```

Then update the existing envelope-child test at `:145-153`, which asserts the
target's exact shape and will now also receive a parent. Change its expectation to:

```tsx
  expect(onSpentClick).toHaveBeenCalledWith({
    id: 'cat-rent', type: 1, name: 'Rent', icon: 'house', currencyId: 'cur-eur',
    parent: { id: 'env-1', type: 0 },
  })
```

Leave the top-level test at `:136-143` alone — top-level rows have no parent and
its expectation stays exact.

- [ ] **Step 2: Run the tests to verify they FAIL**

Run: `cd web && pnpm exec vitest run src/features/budgets/BudgetTable.test.tsx`
Expected: FAIL — both the new test and the updated envelope test report a missing `parent` property in the received call.

- [ ] **Step 3: Add the parent to the target type**

In `web/src/features/budgets/BudgetTransactionsDialog.tsx`, replace the
`BudgetTransactionsTarget` interface at `:27-34` with:

```tsx
/** enough of an element (or a nested child category) to list its transactions */
export interface BudgetTransactionsTarget {
  id: Id
  type: BudgetElementType
  name: string
  icon: string
  /** null = the budget base currency */
  currencyId: Id | null
  /** set on a nested child row: the row it is listed under */
  parent?: { id: Id; type: BudgetElementType }
}
```

- [ ] **Step 4: Populate the parent when rendering children**

In `web/src/features/budgets/BudgetTable.tsx`, replace line 219 with:

```tsx
                  {spentCell({ id: child.id, type: child.type, name: child.name, icon: child.icon, currencyId: element.currencyId, parent: { id: element.id, type: element.type } }, child.spent)}
```

- [ ] **Step 5: Send both params for a category under a tag**

In `web/src/features/budgets/BudgetTransactionsDialog.tsx`, replace the `params`
block at `:58-66` with:

```tsx
  // A category listed under a tag displays the tag-and-category intersection.
  // Sending categoryId alone puts the backend on its "tag_id IS NULL" branch,
  // which returns the complement of what the row shows.
  const parentTagId = element?.parent?.type === BudgetElementType.TAG ? element.parent.id : undefined
  const params = element
    ? {
        budgetId: budget.meta.id,
        periodStart: selectedDate,
        ...(element.type === BudgetElementType.CATEGORY ? { categoryId: element.id } : {}),
        ...(element.type === BudgetElementType.TAG ? { tagId: element.id } : {}),
        ...(element.type === BudgetElementType.ENVELOPE ? { envelopeId: element.id } : {}),
        ...(parentTagId ? { tagId: parentTagId } : {}),
      }
    : null
```

An envelope child keeps sending `categoryId` alone, which is correct — envelope
children display the untagged bucket.

- [ ] **Step 6: Run the tests to verify they PASS**

Run: `cd web && pnpm exec vitest run src/features/budgets/`
Expected: PASS, including `budget.test.ts`, whose existing assertion that a plain
category request does **not** contain `tagId` (`web/src/api/budget.test.ts:89`)
must still hold.

- [ ] **Step 7: Type-check and lint**

Run: `cd web && pnpm exec tsc -b && pnpm lint`
Expected: both clean. vitest and oxlint do not type-check, so `tsc -b` is the gate that catches a malformed optional property.

- [ ] **Step 8: Commit**

```bash
git add web/src/features/budgets/BudgetTable.tsx web/src/features/budgets/BudgetTransactionsDialog.tsx web/src/features/budgets/BudgetTable.test.tsx
git commit -m "fix: drill a tag's child category into the tagged transactions

The child click target dropped the parent tag, so the dialog sent
categoryId alone and the backend took its tag_id IS NULL branch —
listing the exact complement of the amount shown on the row. The
tag+category branch already existed and was unused."
```

---

### Task 5: Full verification

**Files:** none — verification only.

- [ ] **Step 1: Run the full Go smoke suite**

Run: `make go-test`
Expected: PASS, including the coverage gate, `gofmt`, `go vet`, and the apiparity/mcpparity golden suites. No golden should change — this plan alters no response shape. **If a golden does change, stop and inspect the diff**: it means observable behavior moved and that needs explaining before the branch merges.

- [ ] **Step 2: Run the full suite including PostgreSQL and the frontend**

Run: `make test`
Expected: PASS, including the `enginecompare` suite asserting byte-identical SQLite and PostgreSQL responses.

- [ ] **Step 3: Confirm the fix by hand**

Start the app, open a budget with a tag that has spending across at least two
categories, expand the tag, and click a child's spent amount. The listed
transactions must be the tagged ones, and their sum must equal the number on the
row. Before the fix the list showed that category's untagged transactions
instead, usually summing to a different figure.

---

## Self-Review

**Spec coverage.** This plan implements spec §1.1 (Tasks 3, 4) and §1.2 (Task 2),
plus spec tests 1-6 (Tasks 1, 2), 12 (Task 3), and 13-14 (Task 4). Spec §1.3 is
explicitly "not doing" and needs no task.

Deliberately deferred from the spec's Phase 1 list, because they belong with the
code they exercise:

- Test 7 (per-branch dispatcher unit tests) — Task 3 covers all three reachable
  shapes end to end, which is stronger; a separate unit test of the same switch
  would be redundant.
- Tests 8-10 (builder-level tag/envelope bucket assertions) — the tag pass
  arithmetic was verified correct during investigation and is untouched by this
  plan. These become genuinely necessary in Phase 2, which rewrites that
  aggregation, and they are already in the Phase 2 test list.
- Test 11 (an apiparity scenario containing a tag with children) — a golden
  addition; Task 3 covers the same behavior with sharper assertions. Worth adding
  in Phase 2, when the goldens must be regenerated anyway.
- Test 15 (`BudgetTransactionsDialog` param mapping) — added during the final
  fix wave (`BudgetTransactionsDialog.test.tsx`): the review that surfaced the
  order-dependent `parentTagId` spread needed a real assertion that a
  TAG-typed element's own `tagId` survives, which `budget.test.ts` alone
  can't exercise.

**Placeholder scan.** No TBD/TODO. Every code step contains the actual code.

**Type consistency.** `parent?: { id: Id; type: BudgetElementType }` is declared
in Task 4 Step 3 and used identically in Steps 1, 4, and 5. `sqliteDatetime`
matches its definition at `read.go:122`. `BudgetTransactionsByCategories` and
`BudgetTransactionsByTag` keep their exact existing signatures. Test helper names
(`newReadRepo`, `seedCategory`, `seedExpense`, `newHarness`, `h.do`,
`mustUnmarshal`) are all verified against the files they come from.
