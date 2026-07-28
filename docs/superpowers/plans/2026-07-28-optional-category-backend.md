# Optional Transaction Category — Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let expense/income transactions be saved without a category, and surface that spending in `get-budget` as a read-only "Uncategorized" element with a working drill-down.

**Architecture:** The database, entity, DTOs and sqlc queries are already nullable — one validation guard in `buildState` is the only thing forbidding a null category. The budget side needs three changes: let NULL-category rows out of `CountSpending`, route them to a third bucket (tag wins over uncategorized), and emit a presentation-only element that is never persisted.

**Tech Stack:** Go (stdlib `net/http`, hand-written SQL in `internal/budget/repo`), SQLite (default) + PostgreSQL.

This is the backend half of `docs/superpowers/specs/2026-07-27-optional-transaction-category-design.md`. The frontend half is a separate plan and must not be started here. Phase 1 (the drill-down fixes) is already merged as `f7d5850`.

## Global Constraints

- Branch: `feature/optional-transaction-category` (already created). This is a feature, so `feature/<slug>`.
- Any SQL change must be made in **both** dialect branches in lockstep; `enginecompare` asserts byte-identical SQLite and PostgreSQL responses.
- SQLite `spent_at` bounds bind as `'Y-m-d H:i:s'` strings via `sqliteDatetime`; PostgreSQL binds raw `time.Time`. Never change a PostgreSQL branch's date binding.
- Never hand-edit a golden. Regenerate with `UPDATE_GOLDEN=1` and **inspect the diff**.
- Comments only for non-obvious *why*. Never reference the removed PHP/Symfony implementation.
- `gofmt` and `go vet` clean; the coverage gate is `GO_COVER_MIN=78`.
- The wire `spent` value is **positive**. Never emit it negated.
- Datetimes on the wire are `"2006-01-02 15:04:05"`.

## Prior art the implementer must read

`model.TransactionListRequest` (`internal/model/transaction_dto.go:139-147`) **already has** `Uncategorized bool \`json:"uncategorized,omitempty"\`` at `:143`, and its `Validate()` already enforces mutual exclusion with `categoryId` at `:173-175`. The transaction feature already filters on it. Mirror that field name, that JSON tag, and that validation shape — do not invent a different convention.

## File Structure

| File | Responsibility | Change |
| --- | --- | --- |
| `internal/transaction/usecase.go` | Transaction use-case state building | Modify — drop the guard at `:261-265` |
| `internal/model/budget_valueobject.go` | Budget value objects + sentinels | Modify — add the `UncategorizedID` constant |
| `internal/model/budget_view.go` | Repo↔builder row types | Modify — `SpendingRow.CategoryID` → `*string` (`:36`) |
| `internal/budget/repo/read.go` | Hand-written budget SQL | Modify — `CountSpending` predicate; new uncategorized drill-down |
| `internal/budget/builder_structure.go` | Spending-row bucketing | Modify — three-way routing (`:111-127`) |
| `internal/budget/builder_structure_build.go` | Element assembly | Modify — uncategorized row + tag child |
| `internal/budget/txlist.go` | Drill-down dispatcher | Modify — new branches |
| `internal/model/budget_dto.go` | Budget wire DTOs | Modify — `Uncategorized` on the list request (`:459-465`) |
| `internal/budget/api/budget.go` | REST binding | Modify — bind the new param (`:117-128`) |

---

### Task 1: Allow a non-transfer transaction with no category

**Files:**
- Modify: `internal/transaction/usecase.go:261-265` (inside `buildState`, whose body is `:238-287`)
- Test: `internal/transaction/` — add to the existing test file covering create/update (find it with `ls internal/transaction/*_test.go`; follow its established harness)

**Interfaces:**
- Consumes: `buildState(...)` is called by `create.go:52-57`, `update.go:63-68`, and `bulk.go`. `checkReferences` (`usecase.go:89-117`) is **already** nil-guarded at `:102-106` and needs no change.
- Produces: no signature changes.

- [ ] **Step 1: Write the failing tests**

Add to the transaction use-case test file. Match the file's existing harness and helper names — read it first.

```go
// A non-transfer no longer requires a category: the DB column is nullable, the
// entity holds *vo.Id, and transfers have always worked without one.
func TestCreateTransaction_NoCategory(t *testing.T) {
	// Arrange using this package's existing fixture/harness, then create an
	// expense with CategoryId nil. Assert: no error, and the persisted row's
	// category is NULL (read it back through the same service).
}

// An empty string is not a category id. Before this change the blank guard
// rejected it, so it never reached the reference lookup; it must normalize to
// absent rather than becoming a "category not found" error.
func TestCreateTransaction_EmptyCategoryIdIsAbsent(t *testing.T) {
	// Create an expense with CategoryId pointing at "". Assert: no error, and
	// the persisted category is NULL.
}

// Clearing a category on an existing transaction must work: buildState does a
// full-state replace, so a nil categoryId means "no category".
func TestUpdateTransaction_ClearsCategory(t *testing.T) {
	// Create an expense WITH a category, then update it with CategoryId nil.
	// Assert: the persisted category is NULL.
}

// The reference check must not have been weakened along with the blank guard.
func TestCreateTransaction_UnknownCategoryStillRejected(t *testing.T) {
	// Create an expense with a well-formed but non-existent category id.
	// Assert: it still returns an error.
}
```

Fill each body using the package's real harness. Every test must make a real assertion about persisted state — a test that only checks "no error returned" is not acceptable for the first three.

- [ ] **Step 2: Run the tests to verify they FAIL**

Run: `go test ./internal/transaction/ -run 'TestCreateTransaction_NoCategory|TestCreateTransaction_EmptyCategoryIdIsAbsent|TestUpdateTransaction_ClearsCategory' -v`
Expected: the first three FAIL with the validation error `categoryId: This value should not be blank.`. `TestCreateTransaction_UnknownCategoryStillRejected` should already PASS — it guards against over-correction.

- [ ] **Step 3: Remove the guard and normalize the empty string**

In `internal/budget/../internal/transaction/usecase.go`, the non-transfer branch opens at `:260`. Replace the blank guard at `:261-265` and the parse that follows at `:266-270` with:

```go
		// A non-transfer may have no category. Treat an empty string as absent:
		// the removed blank guard used to reject it before it could reach the
		// reference lookup, so passing "" through would turn into a spurious
		// "category not found".
		if categoryID != nil && *categoryID != "" {
			cid, err := vo.ParseId(*categoryID)
			if err != nil {
				return st, model.ValidateBlank(map[string]string{"categoryId": ""})
			}
			st.CategoryID = &cid
		}
```

Read `:266-270` before replacing it and preserve its exact parse-failure behavior — the error it returns on a malformed id is a frozen contract asserted elsewhere. If it differs from the snippet above, keep the original form and only add the outer `if`.

- [ ] **Step 4: Run the tests to verify they PASS**

Run: `go test ./internal/transaction/ -v`
Expected: all pass, including the pre-existing transfer tests (a transfer must still clear category/payee/tag).

- [ ] **Step 5: Commit**

```bash
git add internal/transaction/usecase.go internal/transaction/*_test.go
git commit -m "feat: allow a transaction without a category

The column, entity and DTOs were already nullable and transfers have
always worked without one; a single validation guard in buildState was
the only thing forbidding it. An empty categoryId normalizes to absent."
```

---

### Task 2: Let NULL-category spending through, and route it

`SpendingRow.CategoryID` becoming a pointer breaks compilation at the routing site, so the type change, the query change and the routing change are **one** task — the tree does not build between them.

**Files:**
- Modify: `internal/model/budget_view.go:35-40` (`SpendingRow`)
- Modify: `internal/model/budget_valueobject.go` (add the sentinel constant)
- Modify: `internal/budget/repo/read.go:330-402` (`CountSpending`)
- Modify: `internal/budget/builder_structure.go:111-127` (routing)
- Test: `internal/budget/repo/read_integration_test.go`

**Interfaces:**
- Produces: `model.UncategorizedID` (string constant) — Tasks 3 and 4 both depend on it.
- Produces: `SpendingRow.CategoryID *string` — nil means "no category".

- [ ] **Step 1: Add the sentinel constant**

In `internal/model/budget_valueobject.go`, after the `ElementType` constants (`:13-17`), add:

```go
// UncategorizedID is the wire id of the presentation-only budget element that
// carries spending with no category. It is deliberately NOT a UUID: vo.ParseId
// rejects it, so set-limit and the other element writes refuse it with the
// existing validation error and no new checks. It is never persisted.
const UncategorizedID = "uncategorized"
```

Do **not** add a new `ElementType`. The uncategorized element reports `type = 1` (`ElementCategory`). `elementAliases` (`:19`) is a fixed-size array indexed by value and cannot carry a sentinel.

- [ ] **Step 2: Write the failing repo test**

Append to `internal/budget/repo/read_integration_test.go`:

```go
// Spending with no category must reach the builder so it can be shown as
// "Uncategorized". Tagged rows keep their tag: the tag bucket wins, and the
// uncategorized row only collects what is neither categorized nor tagged.
func TestBudgetReadRepo_CountSpending_NullCategory(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	cat := "c0000000-0000-0000-0000-0000000000c1"
	tag := "7a000000-0000-0000-0000-0000000000d1"
	seedCategory(t, db, cat, userA)
	f := fixture.New(t, db)
	f.Tag(fixture.Tag{ID: tag, UserID: userA, Name: "Tag"})
	// Categorized, untagged.
	seedExpense(t, db, "75000000-0000-0000-0000-000000000001", acctA, cat, "10.00", "2024-04-05 00:00:00")
	// No category, no tag -> the uncategorized bucket.
	f.Transaction(fixture.Transaction{ID: "75000000-0000-0000-0000-000000000002", UserID: userA, AccountID: acctA, Type: 0, Amount: "20.00", SpentAt: "2024-04-06 00:00:00"})
	// No category but tagged -> the tag bucket.
	f.Transaction(fixture.Transaction{ID: "75000000-0000-0000-0000-000000000003", UserID: userA, AccountID: acctA, TagID: tag, Type: 0, Amount: "30.00", SpentAt: "2024-04-07 00:00:00"})

	start := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	rows, err := read.CountSpending(ctx, []vo.Id{vo.MustParseId(cat)}, []vo.Id{vo.MustParseId(acctA)}, start, end)
	if err != nil {
		t.Fatalf("CountSpending: %v", err)
	}

	var categorized, uncatUntagged, uncatTagged int
	for _, r := range rows {
		switch {
		case r.CategoryID != nil:
			categorized++
		case r.TagID != nil && *r.TagID != "":
			uncatTagged++
		default:
			uncatUntagged++
		}
	}
	if categorized != 1 || uncatUntagged != 1 || uncatTagged != 1 {
		t.Fatalf("want one row of each kind; got categorized=%d uncategorized-untagged=%d uncategorized-tagged=%d: %+v",
			categorized, uncatUntagged, uncatTagged, rows)
	}

	// An empty category list must still return the uncategorized rows rather
	// than short-circuiting -- and must not emit "IN ()", a PostgreSQL syntax
	// error.
	noCats, err := read.CountSpending(ctx, nil, []vo.Id{vo.MustParseId(acctA)}, start, end)
	if err != nil {
		t.Fatalf("CountSpending with no categories: %v", err)
	}
	if len(noCats) != 2 {
		t.Fatalf("want the 2 uncategorized rows when no categories are given, got %d: %+v", len(noCats), noCats)
	}

	// No accounts still short-circuits.
	if none, err := read.CountSpending(ctx, []vo.Id{vo.MustParseId(cat)}, nil, start, end); err != nil || none != nil {
		t.Errorf("empty account ids should be nil,nil; got %v, %v", none, err)
	}
}
```

- [ ] **Step 3: Run it to verify it FAILS**

Run: `go test ./internal/budget/repo/ -run TestBudgetReadRepo_CountSpending_NullCategory -v`
Expected: FAIL — it will not compile yet (`r.CategoryID != nil` on a `string`), which is the expected first failure. That compile error IS the red state for this step; proceed to the type change.

- [ ] **Step 4: Make `SpendingRow.CategoryID` nullable**

In `internal/model/budget_view.go`, change `:36`:

```go
type SpendingRow struct {
	// nil when the transaction has no category (shown as Uncategorized).
	CategoryID *string
	TagID      *string
	CurrencyID string
	Amount     string
}
```

- [ ] **Step 5: Widen the `CountSpending` predicate in both dialects**

In `internal/budget/repo/read.go`:

Replace the early-return guard at `:335-337` with an accounts-only guard:

```go
	if len(accountIDs) == 0 {
		return nil, nil
	}
```

Then build the category predicate conditionally, because an empty list must not produce `IN ()` (a no-op on SQLite but a syntax error on PostgreSQL). In the **pgsql** branch, replace the fixed `catIn` interpolation so the predicate reads:

- when `len(catArgs) > 0`: `(t.category_id IN (<catIn>) OR t.category_id IS NULL)`
- when `len(catArgs) == 0`: `t.category_id IS NULL`

Apply the identical change in the **sqlite** branch. Keep every other clause byte-identical, keep the pgsql `$N` arithmetic consistent with the args actually appended, and keep `sqliteDatetime(start), sqliteDatetime(end)` on the sqlite branch and raw `start, end` on pgsql.

Then delete the nil-**category** halves of the two scan guards (`:381-383` pgsql, `:392-394` sqlite) — keep the nil-**currency** check:

```go
			if currencyID == nil {
				continue
			}
```

And change the row append at `:399` to carry the pointer through:

```go
		out = append(out, model.SpendingRow{CategoryID: categoryID, TagID: tagID, CurrencyID: *currencyID, Amount: a})
```

- [ ] **Step 6: Route the three buckets (tag wins)**

In `internal/budget/builder_structure.go`, replace the if/else at `:113-117` and the map key at `:121`:

```go
		for _, row := range rows {
			catKey := model.UncategorizedID
			if row.CategoryID != nil {
				catKey = *row.CategoryID
			}
			var key string
			switch {
			case row.TagID != nil && *row.TagID != "":
				// A tag wins over the category, and over having none: a tagged
				// but uncategorized expense belongs to its tag, surfacing as an
				// Uncategorized child there rather than in the top-level row.
				key = elementKey(*row.TagID, model.ElementTag)
			default:
				key = elementKey(catKey, model.ElementCategory)
			}
```

and use `catKey` everywhere the old code used `row.CategoryID` for the `spendingInCategories` map (`:121-126`) and for `categorySpending{categoryID: ...}`.

The three buckets stay a disjoint partition — every row lands in exactly one.

- [ ] **Step 7: Fix remaining compile errors**

Run `go build ./...` and fix every other consumer of `SpendingRow.CategoryID`. Do not change behavior while doing so — a nil category must reach the same place the sentinel key points at. Report anything surprising.

- [ ] **Step 8: Run the tests**

Run: `go test ./internal/budget/... ./internal/model/... -v` then `go test ./...`
Expected: all pass, including the new test and every pre-existing budget test.

> **Amended after review.** An earlier version of this step required that Task 2
> change no response at all. That is not achievable and was wrong: NULL-category
> transactions **already exist** in production, because `delete-category` in
> delete mode relies on `ON DELETE SET NULL` (`internal/category/delete.go:16-17`).
> A tagged one previously had its row dropped before it could count, so its tag
> stayed hidden; it now counts, so the tag appears. In Task 2 alone that renders
> as an all-zero ghost row, because nothing yet emits the child that carries the
> amount — Task 3 completes it. The branch ships atomically, so the intermediate
> state never reaches production. Surfacing this historical money is intended.
>
> The goldens are structurally blind to this: no fixture pairs a NULL category
> with a tag. **Task 3 Step 1a adds that fixture.**

- [ ] **Step 9: Verify PostgreSQL**

Run: `make test-repo-pgsql`
Expected: PASS. This exercises the widened predicate on the engine where `IN ()` is a syntax error. If PostgreSQL is unavailable, say so explicitly rather than claiming it passed.

- [ ] **Step 10: Commit**

```bash
git add internal/model/budget_view.go internal/model/budget_valueobject.go internal/budget/repo/read.go internal/budget/builder_structure.go internal/budget/repo/read_integration_test.go
git commit -m "feat(budget): carry NULL-category spending through to the builder

CountSpending dropped those rows twice (the IN predicate and a scan
guard). Widening it needs SpendingRow.CategoryID to be nullable, which
in turn needs the bucket routing to gain a third case. A tag still wins,
so a tagged-but-uncategorized expense stays under its tag."
```

---

### Task 3: Emit the Uncategorized element

**Files:**
- Modify: `internal/budget/builder_structure_build.go` (`buildStructure`, `:42-241`)
- Test: `internal/budget/api/` — a new API-level test file, following `internal/budget/api/tag_limit_visibility_test.go`

**Interfaces:**
- Consumes: `model.UncategorizedID` from Task 2; the bucket key `elementKey(model.UncategorizedID, model.ElementCategory)`.
- Produces: a `ParentElementResult` with `Id: model.UncategorizedID`, and a `ChildElementResult` of the same id under tags.

**Element shape (all values exact):**

| field | value |
| --- | --- |
| `id` | `model.UncategorizedID` |
| `type` | `1` (`ElementCategory`) |
| `name` | `"Uncategorized"` |
| `icon` | `"question_mark"` |
| `budgeted` | `"0"` |
| `spent` / `budgetSpent` | positive, converted like any other element |
| `available` | the same `budgetedBefore - spentBefore - spent` formula every other element uses, which with no limits reduces to `-spent` |
| `folderId` | `null` |
| `position` | a sentinel larger than any real position, so it sorts last |
| `children` | `[]` |
| `isArchived` | `0` |
| `ownerUserId` | `null` |
| currency | the budget's currency (`budgetCurrencyID`) |

It is emitted **only when its spending is nonzero** — it can never have a limit, so the existing "spending or a limit" visibility rule reduces to spending. It is **never persisted**: do not create a `budget_elements` row, or `restoreElementsOrder` (`internal/budget/move.go`) will garbage-collect it.

- [ ] **Step 1: Write the failing API test**

Create `internal/budget/api/uncategorized_test.go`. Read `internal/budget/api/tag_limit_visibility_test.go` first for the harness (`newHarness`, `h.token`, `h.do`, `mustUnmarshal`, `fixture.New(t, &dbtest.DB{Raw: h.db, Engine: "sqlite"})`, and the seeded constants `budgetID1`, `tagID`, `catID`, `accountID`, `seedUserID`, `usdID`).

Write four tests:

1. `TestUncategorized_TopLevelRowAppears` — seed an expense with no category and no tag; `get-budget` contains an element with id `"uncategorized"`, `budgeted == "0"`, `spent` equal to the seeded amount (positive), `folderId` null, and empty `children`.
2. `TestUncategorized_HiddenWhenNoSuchSpending` — seed only a categorized expense; `get-budget` contains **no** element with that id.
3. `TestUncategorized_TaggedGoesToTheTagAsAChild` — seed an expense with a tag but no category; the top-level uncategorized row is **absent**, and the tag element has a child with id `"uncategorized"` carrying that amount.
4. `TestUncategorized_SetLimitIsRejected` — `POST /api/v1/budget/set-limit` with `elementId: "uncategorized"` returns the existing validation error (400 with `elementId` blank), **and** a direct DB query shows no `budget_elements` row was created for it:

```go
	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM budget_elements WHERE external_id = ?`, "uncategorized").Scan(&n); err != nil {
		t.Fatalf("count budget_elements: %v", err)
	}
	if n != 0 {
		t.Errorf("the uncategorized element must never be persisted; found %d rows", n)
	}
```

Assert on real values, not just presence.

- [ ] **Step 1a: Cover the historical category-deleted case**

Added after Task 2's review. NULL-category transactions already exist in every
production database: `delete-category` in delete mode hard-deletes the row so the
FK nulls its transactions (`internal/category/delete.go:16-17`). Before this
branch those rows were dropped from the aggregation, so a tag whose only spending
was on one stayed **hidden**. It must now appear, showing that spending with an
Uncategorized child — surfacing that money is the point of the feature.

No existing fixture pairs a NULL category with a tag, which is exactly why the
goldens could not catch the intermediate breakage Task 2 introduced. Add a fifth
test:

5. `TestUncategorized_TagWithOnlyCategoryDeletedSpending` — seed an expense with
   a tag and **no** category (the shape `delete-category` leaves behind), with the
   tag having no limit and no other spending. Assert `get-budget` shows that tag
   as a **parent element carrying the spending** (not an all-zero ghost row), with
   exactly one child whose id is `"uncategorized"` and whose `spent` equals the
   seeded amount. Assert the tag's own `spent` is that amount too, and that the
   **top-level** uncategorized row is absent (the tag won).

This is the regression guard for the whole tag-wins rule against real historical
data. If it renders as an all-zero row, Task 3 is not finished.

- [ ] **Step 2: Run to verify they FAIL**

Run: `go test ./internal/budget/api/ -run TestUncategorized -v`
Expected: tests 1 and 3 FAIL (no such element yet). Tests 2 and 4 should already PASS — they guard against over-emitting and against persistence.

- [ ] **Step 3: Emit the top-level element**

In `buildStructure`, after the standalone-category pass (which ends at `:180`), add a pass that reads the bucket `elementKey(model.UncategorizedID, model.ElementCategory)` and, when it has spending, appends a `structElement` with the shape in the table above. Follow the standalone pass (`:146-180`) for how spending is converted into the `toConvert` keys — an element's own spending is keyed **without** a sub-prefix (`spent_<index>`, `spent-budget_<index>`, `spent-before_<index>`, `:169-177`).

Use a `position` sentinel that cannot collide with a real element; define it as a named constant next to the pass with a comment saying why.

- [ ] **Step 4: Emit the tag child**

In the tag pass (`:101-144`), the child loop at `:127-139` does `cat, ok := f.categories[catID]; if !ok { continue }`, which would silently drop the uncategorized child. Add an explicit branch: when `catID == model.UncategorizedID`, emit a child named `"Uncategorized"` with icon `"question_mark"` instead of looking it up in `f.categories`.

The existing rule at `:210-212` already drops zero-spend tag children, so an empty one disappears for free. The child sort at `:221` is by id; `"uncategorized"` starts with `u`, which sorts after any lowercase-hex UUID, so it lands last with no extra work.

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/budget/... -v`
Expected: all four new tests pass, plus every pre-existing budget test.

- [ ] **Step 6: Regenerate and inspect the goldens**

The new element changes `get-budget` responses **only for fixtures that have uncategorized spending**. Run:

```bash
UPDATE_GOLDEN=1 go test ./internal/test/apiparity/
git diff internal/test/apiparity/testdata/golden/
```

**Inspect the diff and report exactly what changed.** If a golden changed for a scenario with no uncategorized spending, that is a bug — stop and report rather than accepting it. If no golden changes at all, say so.

- [ ] **Step 7: Commit**

```bash
git add internal/budget/builder_structure_build.go internal/budget/api/uncategorized_test.go internal/test/apiparity/testdata/golden/
git commit -m "feat(budget): show uncategorized spending as a read-only element

Presentation-only: it is never written to budget_elements, and its
non-UUID id makes vo.ParseId reject every element write with the
existing validation error. Tagged-but-uncategorized spending surfaces
as a child of its tag instead."
```

---

### Task 4: Drill down into uncategorized spending

**Files:**
- Modify: `internal/model/budget_dto.go:459-465` (`BudgetTransactionListRequest`)
- Modify: `internal/budget/txlist.go:13-82` (doc comment + dispatcher)
- Modify: `internal/budget/repo/read.go` (a new query + a third state on `BudgetTransactionsByTag`, `:518`)
- Modify: `internal/budget/api/budget.go:110-128` (swagger params + binding)
- Test: `internal/budget/repo/read_integration_test.go`, `internal/budget/api/uncategorized_test.go`

**Interfaces:**
- Consumes: `model.UncategorizedID`.
- Produces: `BudgetTransactionListRequest.Uncategorized bool`; two repo entry points (below).

**Wire contract:**
- `{uncategorized: true}` → transactions with `category_id IS NULL AND tag_id IS NULL`
- `{uncategorized: true, tagId: T}` → `tag_id = T AND category_id IS NULL`
- `{uncategorized: true, categoryId: X}` → validation error, mirroring `TransactionListRequest.Validate()` (`internal/model/transaction_dto.go:173-175`), including its `CodeInvalidChoice` code.

- [ ] **Step 1: Write the failing repo tests**

Append to `internal/budget/repo/read_integration_test.go` a test that seeds four expenses in one period — categorized+untagged, categorized+tagged, uncategorized+untagged, uncategorized+tagged — and asserts:

- the uncategorized-untagged query returns exactly the uncategorized+untagged row
- the uncategorized-by-tag query returns exactly the uncategorized+tagged row
- neither returns any categorized row

Use distinct amounts so a wrong row is obvious, and assert on ids, not just counts.

- [ ] **Step 2: Add the repo support**

In `internal/budget/repo/read.go`:

Add `BudgetTransactionsUncategorized(ctx context.Context, accountIDs []vo.Id, start, end time.Time) ([]model.BudgetTransactionRow, error)` — same shape as `BudgetTransactionsByCategories` (`:483`) but with `AND t.category_id IS NULL AND t.tag_id IS NULL` and no category list. Keep the `len(accountIDs) == 0` guard. Bind dates with `sqliteDatetime` on the sqlite branch and raw `time.Time` on pgsql, exactly like its sibling.

Extend `BudgetTransactionsByTag` (`:518`) with a third state. Its `categoryID *vo.Id` today means nil = "no narrowing", non-nil = "= this category"; add a separate `uncategorized bool` parameter meaning "category IS NULL", documented as mutually exclusive with a non-nil `categoryID`. In both dialects the fragment is `" AND t.category_id IS NULL"` with **no bound argument** — in the pgsql branch be careful not to advance the `next` placeholder counter (`:528-539`) for a clause that binds nothing.

Update the existing callers of `BudgetTransactionsByTag` (`txlist.go:65`) for the new signature.

- [ ] **Step 3: Add the request field and validation**

In `internal/model/budget_dto.go`, add to `BudgetTransactionListRequest`:

```go
	Uncategorized bool `json:"uncategorized,omitempty"`
```

This DTO has no `Validate()` today — validation is done ad hoc inside `GetTransactionList`. Add the mutual-exclusion check there, in the same style as the existing `model.ValidateBlank` calls, returning the same shape `TransactionListRequest.Validate()` uses at `internal/model/transaction_dto.go:173-175` (key `categoryId`, code `errs.CodeInvalidChoice`).

- [ ] **Step 4: Add the dispatcher branches**

In `internal/budget/txlist.go`, add two cases to the switch (`:45-78`), **before** the existing `cat`/`tag` cases so the uncategorized flag takes precedence:

- `req.Uncategorized && tag == "" && env == ""` → `BudgetTransactionsUncategorized(...)`
- `req.Uncategorized && tag != "" && env == ""` → `BudgetTransactionsByTag(ctx, tagID, nil, true, ...)`

Then update the doc comment at `:13-18` to describe the new modes accurately. That comment was rewritten in the last PR precisely because a stale one caused a bug — keep it true.

- [ ] **Step 5: Bind the parameter at the edge**

In `internal/budget/api/budget.go`, the handler is at `:117` with the request literal at `:122-128` and swagger `@Param` lines at `:110-112`. Add an `uncategorized` query parameter and a swagger line for it. It is a **query** parameter on a GET, so parse it as a string and treat `"1"` and `"true"` as true; follow whatever the file's `optQuery` helper (`:137`) does for the existing params, and add a boolean equivalent rather than bending `optQuery`.

- [ ] **Step 6: Add the API-level test**

Add to `internal/budget/api/uncategorized_test.go`: drill into the top-level uncategorized row and assert it returns exactly the uncategorized+untagged transactions; drill into a tag's uncategorized child (`tagId` + `uncategorized=1`) and assert it returns exactly the uncategorized+tagged ones. Also assert `uncategorized=1&categoryId=<id>` returns the validation error.

- [ ] **Step 7: Run everything**

Run: `go test ./internal/budget/... -v` then `make go-test`
Expected: all pass, coverage gate met.

- [ ] **Step 8: Regenerate goldens if the route's shape changed**

Run `UPDATE_GOLDEN=1 go test ./internal/test/apiparity/` and inspect. A new optional query parameter should not change existing scenario responses; if a golden moves, explain why before accepting it. `mcpparity` is **not** affected — there is no budget transaction-list MCP tool, and MCP schemas come from separate `*Input` structs.

- [ ] **Step 9: Commit**

```bash
git add internal/model/budget_dto.go internal/budget/txlist.go internal/budget/repo/read.go internal/budget/api/budget.go internal/budget/api/uncategorized_test.go internal/budget/repo/read_integration_test.go
git commit -m "feat(budget): drill down into uncategorized spending

Mirrors the existing TransactionListRequest.Uncategorized flag, including
its mutual exclusion with categoryId. Composes with tagId so a tag's
uncategorized child lists exactly its own transactions."
```

---

### Task 5: Full backend verification

**Files:** none — verification only.

- [ ] **Step 1: Go smoke suite**

Run: `make go-test`
Expected: PASS, coverage ≥ 78%.

- [ ] **Step 2: Full suite including PostgreSQL**

Run: `make test`

This needs a PostgreSQL. The compose `postgres` service is commented out in this repo, so provision a throwaway one if none is running:

```bash
docker run -d --name econumo-verify-pg -e POSTGRES_USER=econumo -e POSTGRES_PASSWORD=econumo -e POSTGRES_DB=econumo -p 5432:5432 postgres:17-alpine
until docker exec econumo-verify-pg pg_isready -U econumo -q; do sleep 2; done
docker exec econumo-verify-pg psql -U econumo -d econumo -c "CREATE DATABASE econumo_test"
```

Expected: PASS, including `enginecompare` (byte-identical SQLite and PostgreSQL responses) — the primary safety net for Task 2's widened predicate. **Leave the container running**; the frontend plan's verification needs it. Report that it is running.

- [ ] **Step 3: Confirm the golden state**

Run: `git status --porcelain internal/test/apiparity/testdata/golden/ internal/test/mcpparity/testdata/golden/`
Expected: empty (any intended golden changes were committed in Tasks 3-4). Report the full list of goldens this plan changed and the one-line reason for each.

---

## Self-Review

**Spec coverage.** Implements spec §2.1 (Task 1), §2.2 + §2.3 (Task 2), §2.4 (Task 3), §2.5 (Task 4). Spec tests 17-22 → Task 1; 23-27 → Task 2; 28-34 → Task 3; 35-37 → Task 4. Frontend items (§2.6, §2.7) and their tests (38-44) belong to the separate frontend plan and are deliberately absent here.

**Corrections to the spec made in this plan:**
- The spec said `CountSpending`'s guard should "bail only on empty accountIDs". That alone is wrong: an empty category list would emit `IN ()`, valid on SQLite but a **syntax error** on PostgreSQL. Task 2 Step 5 builds the predicate conditionally instead, and Step 2's test covers the empty-category case explicitly.
- The spec said to regenerate **both** apiparity and mcpparity goldens. mcpparity is unaffected: there is no budget transaction-list MCP tool, and MCP input schemas come from dedicated `*Input` structs rather than the model DTOs.
- The spec proposed inventing an `uncategorized` request flag. `model.TransactionListRequest` already has exactly that field, JSON tag, and mutual-exclusion validation — this plan mirrors the existing convention instead.
- `checkReferences` needs no change; it is already nil-guarded.

**Placeholder scan.** Task 1 Step 1 and Task 4 Steps 1/6 describe test bodies rather than transcribing them, because they depend on package harnesses whose exact helper names the implementer must read first; each specifies the exact assertions required. Every production-code step contains the actual code or an exact, unambiguous edit.

**Type consistency.** `model.UncategorizedID` is defined in Task 2 Step 1 and used identically in Tasks 3 and 4. `SpendingRow.CategoryID *string` is introduced in Task 2 Step 4 and consumed in Step 6. `BudgetTransactionsByTag`'s new `uncategorized bool` parameter is added in Task 4 Step 2 and called in Step 4. The element reports `type = 1`; no new `ElementType` is introduced anywhere.
