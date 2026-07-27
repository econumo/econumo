# Optional transaction category + budget drill-down fixes

Date: 2026-07-27

## Summary

Allow expense/income transactions to be saved without a category, surface that
spending in the budget as a read-only "Uncategorized" row, and fix two defects in
the budget drill-down found while investigating the same code.

The work splits into two independently shippable phases. **Phase 1 is pure bug
fixing and should merge first** — Phase 2 builds on the same aggregation and
drill-down code.

## Background

Category is already optional everywhere except two application-level checks:

- `transactions.category_id` is `DEFAULT NULL` in both dialects
  (`internal/infra/storage/migrations/{sqlite,pgsql}/20210812210548.sql:163`), with
  `ON DELETE SET NULL` (`:177`) — the schema is explicitly designed for a null category.
- `model.Transaction.CategoryID` is `*vo.Id` (`internal/model/transaction.go:63`), the
  create/update DTOs use `*string`, and `TransactionResult.CategoryId` is already
  nullable on the wire.
- Transfers already work without a category and are the existing precedent.

No migration, entity, DTO, or sqlc change is required for the transaction half.

## Phase 1 — Budget drill-down bug fixes

### 1.1 Tag children drill down to the wrong transactions

**Symptom.** Expanding a tag shows the correct child categories with correct
numbers, but clicking a child's spent figure lists the wrong transactions.

**Root cause.** The row shows one set and the query asks for its exact complement.

The displayed number is a true intersection: `CountSpending` groups by
`t.category_id, t.tag_id` (`internal/budget/repo/read.go:345`/`:356`), and
`builder_structure.go:111-127` routes each row to a tag-keyed bucket when
`TagID` is set, so `spending["<T>-tag"].spendingInCategories["<C>"]` is
`SUM(tag = T AND category = C)`. The tag pass reads exactly that bucket
(`builder_structure_build.go:127-140`). The arithmetic is correct.

The drill-down loses the tag in three steps:

1. `web/src/features/budgets/BudgetTable.tsx:219` builds the click target from
   `child.id` but never passes `element.id`. `BudgetTransactionsTarget`
   (`BudgetTransactionsDialog.tsx:27-34`) has no field for a parent.
2. A tag child carries `type = 1` (CATEGORY, set at
   `builder_structure_build.go:214`), so `BudgetTransactionsDialog.tsx:58-66`
   sends `categoryId` only, with no `tagId`.
3. The backend therefore takes the `cat != "" && tag == ""` branch
   (`internal/budget/txlist.go:42-48`) into `BudgetTransactionsByCategories`,
   whose SQL hard-filters `AND t.tag_id IS NULL` (`read.go:494`, `:498`).

Result: clicking category C under tag T returns C's **untagged** transactions.

Envelope children are unaffected because they read the category-keyed
(i.e. untagged) bucket (`builder_structure_build.go:88`), which happens to agree
with the query's `tag_id IS NULL`.

**Fix — mostly frontend; the backend capability already exists and is unused.**
`BudgetTransactionsByTag(ctx, tagID, categoryID *vo.Id, …)` (`read.go:511-554`)
already filters both, and `txlist.go:49-62` already accepts a `catFilter`. That
dead branch indicates the wiring was simply dropped.

- Add a parent field to `BudgetTransactionsTarget` and populate it at
  `BudgetTable.tsx:219` for child rows.
- In `BudgetTransactionsDialog.tsx:58-66`, when a CATEGORY-typed target has a
  TAG-typed parent, send **both** `tagId` and `categoryId`.
- No backend change expected. Confirm during implementation that
  `web/src/api/budget.ts:114-131` serializes both params.

**Contract note.** `web/src/api/budget.test.ts:89` asserts a plain category
request does *not* contain `tagId`. That must keep passing — only the
child-of-tag case gains `tagId`.

### 1.2 SQLite drops 1st-of-month rows from every drill-down

**Root cause.** `CountSpending` binds the `spent_at` bounds as `'Y-m-d H:i:s'`
strings via `sqliteDatetime`, with an explicit comment (`read.go:352-359`)
warning that a raw `time.Time` bound "does not compare correctly against the
stored datetime TEXT at month boundaries (it drops the first-of-month row)".

Both drill-down queries bind raw `time.Time` anyway: `read.go:502` and
`read.go:533`/`:545`. `sqliteDatetime` is used at `:109`, `:199`, `:359`, `:412`
but never in `BudgetTransactionsByCategories` / `BudgetTransactionsByTag`.

**Effect.** On SQLite (the default engine), a transaction dated the 1st of the
month counts toward the row total but is missing from the transaction list — the
list silently disagrees with the number above it. Independent of tags; affects
every drill-down.

**Fix.** Use the same string-bound helper in both drill-down queries, matching
`CountSpending`. SQLite only; the PostgreSQL path is unaffected.

### 1.3 Not doing: the duplicate top-level row

A category whose spending is entirely tagged renders twice — once as a tag child
with the tagged amount, once as a top-level row showing only the untagged
remainder, often `0`.

This was considered and **explicitly rejected**. Suppressing the top-level row
would hide the only place a limit can be set (tag children have no budget cell),
so a limitless, fully-tagged category would become unreachable for budgeting.
Nothing double-counts — `bucketStats` (`web/src/features/budgets/budgetMath.ts:39-45`)
sums only top-level `budgetSpent`, and the backend buckets are a disjoint
partition. Current behavior stands.

## Phase 2 — Optional category

### 2.1 Backend: relax the requirement

`internal/transaction/usecase.go` `buildState()` lines 260-265 is the single
backend enforcement point: for a non-transfer it rejects a nil/empty category
with `CodeIsBlank`. Remove that check.

- `checkReferences()` (`:89-118`) already validates the category only when
  `st.CategoryID != nil` (`:102`), so nil flows through cleanly.
- Both `create.go:52-57` and `update.go:63-68` call `buildState`, so this covers
  create and update, on both the REST and MCP edges.
- Clearing an existing category needs no extra work: `buildState` does a
  full-state replace, so `categoryId: null` clears it.

### 2.2 Budget aggregation: let NULL-category spending through

`CountSpending` (`internal/budget/repo/read.go:328`) currently drops NULL-category
rows twice — the `t.category_id IN (…)` predicate, and a scan guard at
`:379-381` (pg) / `:390-392` (sqlite).

- Widen the predicate to `(t.category_id IN (…) OR t.category_id IS NULL)` in
  **both** dialects, keeping the account, date and `type = 0` filters.
- Change `SpendingRow.CategoryID` from `string` to `*string`
  (`internal/model/budget_view.go:35`).
- Drop the nil-**category** scan guard; keep the nil-**currency** guard.
- The early return at `:333` bails when `len(categoryIDs) == 0`; it must still run
  for the uncategorized case. Bail only on empty `accountIDs`.

### 2.3 Routing rule: tag wins

`builder_structure.go:113-117` routes each spending row to exactly one bucket.
Extend the existing if/else to three cases:

| tag | category | bucket |
| --- | --- | --- |
| set | any | tag bucket (**tag wins**, even with no category) |
| nil | set | category bucket |
| nil | nil | new uncategorized bucket |

So a tagged-but-uncategorized expense rolls up under its tag and appears as an
"Uncategorized" **child** of that tag. Only untagged, uncategorized expenses feed
the top-level row. The buckets stay a disjoint partition, so nothing
double-counts.

Implementation notes:

- `spendingInCategories` is keyed by category id; nil needs a sentinel key.
- `builder_structure_build.go:129` does `cat, ok := f.categories[catID]; if !ok { continue }`,
  which would silently skip the nil-category child. It needs an explicit branch
  emitting the synthetic name/icon.
- `:210` already drops tag children with zero spend, so an empty uncategorized
  child disappears for free.
- Children sort by id (`:221`). `"uncategorized"` starts with `u`, which sorts
  after any lowercase-hex UUID, so it lands last deterministically.

### 2.4 The synthetic row

Presentation-only, emitted in `buildStructure` only. **Never persisted** as a
`budget_elements` row. Precedent for a presentation-only sentinel:
`BudgetRoleOwner = -1` (`internal/model/budget_valueobject.go:51`).

| field | value |
| --- | --- |
| `id` | `"uncategorized"` |
| `type` | `1` (category) |
| `name` / `icon` | "Uncategorized" / `question_mark` |
| `budgeted` | `"0"` |
| `spent` / `budgetSpent` | positive (wire convention) |
| `available` | `-spent` (same formula as a limitless category) |
| `folderId` | `null`, with a large sentinel `position` → bottom of the no-folder section |
| `children` | `[]` |
| `isArchived` | `0` |
| `ownerUserId` | `null` |
| currency | the budget's default currency |

Visible only when its spending is nonzero, mirroring the existing "spending or a
limit" visibility rule (it can never have a limit).

**Writes are already rejected, by construction.** `SetLimit`
(`internal/budget/accounts.go:113-174`) first calls `vo.ParseId(req.ElementId)`
(`:118`); `"uncategorized"` is not a UUID, so it fails there. `ChangeElementCurrency`
and `MoveElementList` resolve the same way. No new validation is needed — and no
`budget_elements` row may be seeded, or `restoreElementsOrder`
(`internal/budget/move.go:119`) would garbage-collect it anyway.

The wire `spent` convention is **positive**, asserted by
`web/src/features/budgets/BudgetTable.test.tsx:83-85` ("the wire value is positive
and must NOT be rendered negated").

### 2.5 Drill-down for uncategorized

Add an `uncategorized bool` to the budget transaction-list request. It composes
with the existing `tagId`:

- `{uncategorized: true}` → `category_id IS NULL AND tag_id IS NULL`
- `{uncategorized: true, tagId: T}` → `category_id IS NULL AND tag_id = T`

`assembleTxList` already tolerates `row.CategoryID == nil` (`txlist.go:116`).

**Careful:** in `BudgetTransactionsByTag`, a nil `categoryID *vo.Id` currently
means "no category filter". "Category IS NULL" is a third state, so the repo
signature needs an explicit flag rather than overloading nil.

### 2.6 Frontend

- `TransactionDialog.tsx`: remove the required-category check at `validate()`
  lines 162-164; add `clearable` to the category `EntitySelect` (`:363-379`) so an
  existing category can be cleared. Payee at `:388` is the pattern to copy.
- `useTransactionForm.ts` needs no change — `buildPayload()` already sends
  `categoryId: isTransfer ? null : form.categoryId` (`:96`).
- Read-only Uncategorized row: reuse the **existing archive-section mechanism**
  (`BudgetTable.tsx:301`), which passes a filtered `extras` keeping `onSpentClick`
  and dropping `renderBudgetCell` / `renderActions` / `onAvailableClick`. No new
  row concept.
- `useSetLimit` optimistically patches on `el.id === form.elementId`
  (`web/src/features/budgets/queries.ts:128-130`), so suppress the affordance
  client-side rather than relying on the server 400.
- "Uncategorized" label in the transaction list: `transactionTitleInfo()` currently
  returns an empty title with `source: 'none'`
  (`web/src/features/transactions/useAccountTransactions.ts:133`) when a
  transaction has no category, description, tag, or payee. Render the label there.
  Existing description/tag/payee fallbacks are unchanged.
- `ViewTransactionDialog.tsx`: show the label in the Category field when null.
- The `question_mark` icon fallback (`TransactionRow.tsx:38`) already exists; leave it.

### 2.7 i18n

Add one placeholder-free key to **both** `locales/en.json` and `locales/ru.json`,
reused by the budget row, the transaction row, and the view dialog. It spans the
`budgets` and `transactions` features, so it belongs in the cross-cutting
namespace: `common.uncategorized` = "Uncategorized".

Remove `transactions.modal.form.category.validation.required_field`
(`locales/en.json:345-347`) together with its `t()` call, in the same change — the
`i18ntest` guards check both directions.

### 2.8 Analytics

No new `METRICS` key. This rides the existing create/update-transaction flow,
which already fires its event.

## Testing

Phase 1:

- Regression test: a tag child drill-down returns the **tagged** transactions.
- `BudgetTransactionsByCategories` currently has **no test at all** — add one,
  covering its `tag_id IS NULL` filter.
- Month-boundary test using a 1st-of-month transaction. The existing test
  (`internal/budget/repo/read_integration_test.go:109-162`) uses Apr 5/6 and cannot
  catch it; `:95` shows the "incl. Apr 1 boundary" pattern to copy.
- Frontend: assert the child target carries the parent tag id and that the request
  sends both params. `BudgetTable.test.tsx:145-153` covers only an **envelope**
  child, where omitting the parent is correct.

Phase 2:

- Create and update a non-transfer transaction with no category; clear a category
  from an existing transaction.
- Uncategorized aggregation on both engines, including the tag-wins case.
- `SetLimit` against `"uncategorized"` returns the existing validation error.
- The only tag fixture (`web/src/test/fixtures.ts:124`) is archived with no
  children; a tag-with-children fixture is needed.

Golden files:

- No apiparity golden currently contains a tag with children, and nothing asserts
  the buggy behavior, so Phase 1 will not fight the goldens.
- Phase 2 adds a request field, so regenerate apiparity **and** mcpparity goldens
  (`UPDATE_GOLDEN=1`) and **inspect the diff** — a golden change means observable
  behavior changed.
- Both dialects must be edited in lockstep; `enginecompare` asserts byte-identical
  SQLite and PostgreSQL responses.

## Risk

§2.2's predicate widening and the `SpendingRow.CategoryID` → pointer change touch
shared aggregation code that **every** budget row flows through, including the
envelope and tag passes. This is the riskiest part of the work. The
`enginecompare` suite and the budget goldens are the safety net.

## Out of scope

- No "Uncategorized" budget *limit* — the row is read-only by design.
- No suppression of the duplicate top-level category row (see §1.3).
- No change to how envelopes group categories.
