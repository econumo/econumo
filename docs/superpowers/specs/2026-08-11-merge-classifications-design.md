# Merging classifications: categories, tags, payees, labels

**Date:** 2026-08-11
**Status:** Design approved. Ready for implementation planning.

## Problem

Duplicate classifications accumulate. A user creates **Groceries**, later creates
**Food**, and months of transactions end up split across two buckets that mean the
same thing. Today the only way out is to re-categorise every transaction by hand and
then delete the loser — and deleting is worse than useless, because
`ON DELETE SET NULL` silently strips the classification off every transaction and
recurring template that referenced it.

The same shape applies to tags, payees and labels.

## Goal

A **Merge into…** action in the settings list for all four classification kinds. It
re-points everything that referenced the source at the target, folds the source's
budget limits into the target's, deletes the source, and does it all in one
transaction.

**Non-goals.** No undo (the confirm dialog says so plainly). No many-to-one bulk
merge. No automatic duplicate detection. No merging across users.

## Delivery

One PR, sequenced so the suite is green at each boundary:

1. **Payee merge** — the simplest kind (no budget coupling), end to end: use case,
   route, MCP tool, tests. Establishes the pattern.
2. **Category + tag merge** — adds the budget-element port, the `budget.MergeElements`
   algorithm, and the `glue_*` wiring.
3. **Label merge** — the many-to-many dedupe and its per-engine SQL.
4. **Removal of `delete-category` `mode=replace`** — including its two error codes and
   their catalogue entries.
5. **Frontend** — `MergeDialog`, the `extraActions` entries on all three pages,
   mutations and cache invalidation, analytics, i18n.
6. **Golden regeneration** — `apiparity`, `mcpparity`, and `enginecompare`.

Phase 1 is the reference implementation; 2 and 3 are independent of each other.

**No migration.** The feature adds no tables and no columns; it is entirely new
queries over the existing schema.

## What a merge moves

| | transactions | recurring | budget limits | envelope membership |
|---|---|---|---|---|
| **category** | `category_id` → target | `category_id` → target | summed per period | untouched |
| **tag** | `tag_id` → target | `tag_id` → target | summed per period | n/a |
| **payee** | `payee_id` → target | `payee_id` → target | n/a | n/a |
| **label** | join rows re-pointed, deduped | join rows re-pointed, deduped | n/a | n/a |

The source row is then deleted, inside the same transaction.

### What a period is

A **period is a budget month**. `budgets_elements_limits` holds at most one row per
element per month — "the limit for *Groceries* in August 2026" — with the date
snapped to first-of-month on the way in (`model.FirstOfMonth`, `internal/budget/read.go:66`).

"Summed per period" therefore means: for every month in which the source element had
a limit, add that amount into the target element's limit for that same month.

### Why limits are summed

Limits are money, and the merge is already summing the *spending* side by re-pointing
transactions. If the target simply kept its own limit, every past period where both
had one would suddenly read as over budget — the merged category would show combined
spending against the smaller of the two limits. Summing is the only rule that leaves
the budget internally consistent with the transactions that now sit under it.

Amounts within a single budget share that budget's currency, so the addition is
always well defined.

### Why envelope membership is untouched

Envelope membership is a structural choice, not an accumulated amount; there is no
meaningful "sum" of two memberships. In practice two categories being merged are
already under the same envelope, and where they are not, the right resolution depends
on intent the merge cannot infer.

So: the source's `budgets_envelopes_categories` rows cascade away with the category,
and the target keeps whatever it had. The merge dialog carries an info note
suggesting the user settle the budget structure first when the two differ.

## API surface

Four routes, all `POST`, following `{module}/{action}-{subject}`:

```
/api/v1/category/merge-category   {sourceId, targetId}  ->  {}
/api/v1/tag/merge-tag             {sourceId, targetId}  ->  {}
/api/v1/payee/merge-payee         {sourceId, targetId}  ->  {}
/api/v1/label/merge-label         {sourceId, targetId}  ->  {}
```

MCP tools mirror them: `merge_category`, `merge_tag`, `merge_payee`, `merge_label`.

**Field naming.** House style uses `id` for the subject of a write, which would give
`{id, targetId}`. These ship as MCP tools, where `merge_category(id, targetId)` gives
a model no signal about which argument gets destroyed. `sourceId`/`targetId` is
self-describing and worth the small inconsistency.

Read-only callers get 402 with no extra work — these are `POST` routes outside the
middleware's security allowlist.

### Errors

| condition | result |
|---|---|
| `sourceId` or `targetId` blank | `CodeIsBlank` on the field |
| source not found or not owned | not-found, exactly as that feature's delete reports it |
| target not owned | `{entity}.cannot_be_merged` |
| `sourceId == targetId` | `{entity}.cannot_be_merged` |
| category type mismatch (expense vs income) | `category.cannot_be_merged` |

Four new codes in `internal/shared/errs/codes.go`:
`category.cannot_be_merged`, `tag.cannot_be_merged`, `payee.cannot_be_merged`,
`label.cannot_be_merged`, each with `errors.*` catalogue entries in `en` and `ru`.

The not-found branch deliberately keeps each feature's **existing** convention rather
than unifying it: `category` has a coded `category.not_found`, while `tag`, `payee`
and `label` raise a code-less `errs.NewNotFound`. Harmonising those is a separate
concern from merging, and doing it here would change unrelated responses.

## Ownership

Both ids are resolved and ownership-checked **before any write**, inside the
transaction. The reporting is deliberately asymmetric: an unowned **source** is
not-found, so ids cannot be probed (matching today's delete on every kind); an
unowned **target** is `cannot_be_merged`.

This matters because all four list endpoints return **foreign-owned items** —
connected users' classifications arrive via shared accounts, and every settings page
narrows to `ownerUserId === user.id` before rendering (`CategoriesPage.tsx:32`,
`PayeesPage.tsx:24`, `TagsPage.tsx:44-45`). The frontend must build the target picker
from that same filtered array, and the backend must not trust that it did.

**What ownership does not restrict.** Merging my own tag A into my own tag B
re-points transactions **authored by connected users** on my shared accounts, because
those transactions legitimately carry my tag. This is the behaviour established by
\#204 (shared-account transactions use the account owner's classifications); the
alternative would strand those rows on a deleted tag.

## Backend

### Use cases

A `merge.go` per feature package (`internal/{category,tag,payee,label}/`), each
naming a `Service.Merge{Entity}` method, the whole body inside one `tx.WithTx`:

1. parse and validate both ids
2. load both, check ownership and per-kind constraints
3. re-point transactions
4. re-point recurring transactions
5. merge budget elements (category and tag only)
6. delete the source

### Where the SQL lives

Flat column re-points are single UPDATEs carrying no business rules, so they follow
the precedent already set by `ReassignCategoryTransactions` — which lives in
`categories.sql` and writes the `transactions` table. Each feature gains sibling
queries in its own query file for `transactions` and `recurring_transactions`.

Budget element merging carries real logic, so it stays behind a port.

### The budget element port

```go
// internal/{category,tag}/ports.go
type BudgetElementMerger interface {
    MergeElements(ctx context.Context, oldExternalID, newExternalID vo.Id) error
}
```

Wired in `internal/server/glue_category_budgetmerge.go` and
`glue_tag_budgetmerge.go` over the budget feature's public API, so neither feature
imports the other and `internal/test/archtest` stays green.

### The merge algorithm, in `budget`

Budget elements are keyed `UNIQUE (budget_id, external_id)`, with limits cascading
off the element. For every element whose `external_id` is the source:

- **the budget has no element for the target** → re-point that element's
  `external_id` to the target. This preserves the element's limits, folder and sort
  position for free, so the budget row stays exactly where the user placed it and
  simply becomes the surviving classification.
- **the budget already has an element for the target** → for each of the source
  element's limits, add its amount to the target's limit for that period, or insert
  it where the target has none; then delete the source element, whose limits cascade
  away with it.

Scoped by `external_id` alone, so every budget the classification appears in —
including budgets shared with connected users — stays consistent.

#### Periods must be compared with `datetime()`, never raw `=`

`period` is stored as datetime TEXT **whose exact form varies** —
`2026-08-01T00:00:00Z` from Go writes versus `2026-08-01 00:00:00` from older
fixtures — which is why every existing query normalizes both sides
(`budgets.sql:135-142`).

This is a live hazard for the merge, not a stylistic note. A self-join pairing the
two elements' limits on `l1.period = l2.period` would silently **fail to pair** a
Go-written limit with an older-format one. And nothing downstream catches it:
`budgets_elements_limits` has no unique constraint on `(element_id, period)` — the
primary key is `id`, and `UpsertBudgetLimit` conflicts on `id` as well — so the
unpaired source limit would be inserted as a **second row for the same month**,
double-counting in the budget. That is exactly the failure the summing rule exists to
prevent.

The merge therefore resolves the target's limit per period through the normalizing
read (`GetBudgetLimit`, which already wraps both sides in `datetime()`), then either
updates that row by id or inserts a fresh one. It never matches periods by string
equality.

### Labels are the one per-engine divergence

`transactions_labels` and `recurring_transactions_labels` are many-to-many, so a
transaction may already carry **both** labels. Re-pointing must therefore dedupe
rather than overwrite, and a transaction that had both ends up with one row, not a
duplicate:

- sqlite: `INSERT OR IGNORE INTO transactions_labels SELECT transaction_id, ?
  FROM transactions_labels WHERE label_id = ?`
- pgsql: the same with `ON CONFLICT DO NOTHING`

Written once against the querier interface, per the engine-adapter pattern; the
source label's own join rows cascade away when it is deleted.

## Removing `delete-category` `mode=replace`

`delete-category` currently accepts `mode: "replace"` with a `replaceId`, reassigning
transactions before deleting (`internal/category/delete.go:39-60`). It is unused by
the SPA, and it is **incomplete** — it moves transactions but not recurring
templates, so a replace silently nulls the category on every recurring template that
referenced it. Rather than fix a redundant second path, it is removed in favour of
`merge-category`.

`mode` is required and validated against a fixed set (`category_dto.go:150-160`), so
dropping the `ModeReplace` branch and the `ReplaceId` field makes `mode: "replace"`
fall through to `default:` and fail loudly with `CodeInvalidChoice`. It does **not**
silently degrade into a destructive plain delete, which is the failure mode worth
avoiding. Clients sending `mode: "delete"` are unaffected.

`CodeCategoryReplaceIDRequired` and `CodeCategoryCannotBeReplaced` are removed along
with their `errors.*` entries in both languages; `i18ntest` enforces the two-way
coverage between `errs.AllCodes` and the catalogue, so a missed entry fails the
build.

## Frontend

**Entry point.** `ClassificationList` already accepts an `extraActions(item)` prop
that renders menu entries between Edit and Delete, in both the desktop kebab and the
mobile action sheet (`ClassificationList.tsx:93, 373-377, 496`). A **Merge into…**
entry is supplied from each page; `ClassificationList` itself needs no change.

**`MergeDialog`** (new, in `web/src/features/classifications/`) shows:

- the source being merged away
- a `fuzzyMatch`-filtered picker of eligible targets — own items only, the source
  excluded, and for categories restricted to the same type
- for categories, the envelope info note
- a destructive confirm stating plainly that all transactions and recurring
  transactions move to the target, that the source is deleted, and that it cannot be
  undone

There is no live count of affected rows; the copy states the outcome instead, which
avoids a count endpoint per entity.

**Cache invalidation is wider than the page.** A merge changes transactions,
recurring templates and budget limits, so `onSuccess` must invalidate those query
keys and not just the classification list. Leaving them stale would show a budget
with pre-merge limits, which reads as data loss.

**Analytics**, per the CLAUDE.md rule: one `METRICS.CLASSIFICATION_MERGE` key fired
with `{ type }`, mirroring how `CLASSIFICATION_SEARCH` already discriminates by
entity — one catalogue key covering all four call sites, satisfying
`metrics-coverage.test.ts`.

**i18n**: new `classifications.*` strings and the four new `errors.*` entries in `en`
and `ru`, minus the two removed codes.

## Testing

- **Use-case tests per feature**, including the ownership rejection: a merge naming a
  foreign-owned classification on **either** side must fail with **zero rows
  touched** — asserted against transactions, recurring transactions and budget
  limits, not just the response.
- **Budget merge tests**: both branches (re-point when the target has no element;
  sum-and-delete when it does), including a period only the source has, a period only
  the target has, and a period both have.
- **Mixed period formats**: a fixture where the source's limit is written
  `2026-08-01 00:00:00` and the target's `2026-08-01T00:00:00Z` for the same month
  must produce **one** summed limit row, not two. Without this test the
  double-counting bug described above is invisible — every limit written by the
  current Go code shares one format, so a naive implementation passes an
  all-Go-fixture suite.
- **Repo integration tests** on sqlite, rerun against PostgreSQL via
  `make test-repo-pgsql`.
- **`apiparity`**: four new scenarios plus goldens; the `delete-category` replace
  scenarios are removed. Net scenario count still grows, satisfying the never-shrink
  guard — which is precisely the check that would catch an accidental over-deletion
  here.
- **`mcpparity`**: four new scenarios plus goldens.
- **`enginecompare`**: covers the label dedupe, the one place the engines run
  different SQL.
- **vitest**: the menu entry renders on each page; the picker excludes the source and
  every foreign-owned row; the mutation fires with the right ids; the dialog's
  destructive copy is present.
- **`make swagger`** regenerated for the four new annotated handlers.

Coverage gate stays at `GO_COVER_MIN=80`.

## Decisions log

| Decision | Rationale |
|---|---|
| All four kinds, including labels | Labels share the Tags settings screen; Merge on tag rows but not label rows would look broken |
| Sum budget limits per period | The only rule that keeps limits consistent with the spending the merge just combined |
| Leave envelope membership alone | No meaningful "sum" of two memberships; an info note handles the rare mismatch |
| One-to-one, acting row is the source | Matches the user's mental model ("merge food into groceries") and makes the destructive outcome obvious |
| Dedicated `merge-*` routes, not `delete-*` modes | Merge surfacing as *delete* is a trap for an MCP caller, and does not fit labels' many-to-many shape |
| Per-feature routes, not one generic `classification/merge` | No such backend module exists, and an orchestrating package would have to import all four features, which `archtest` forbids |
| `sourceId`/`targetId` over `id`/`targetId` | Self-describing for MCP callers, where the destructive side must be unambiguous |
| Re-point the element when the target has none | Preserves the budget row's folder and sort position; same arithmetic as delete-and-recreate |
| Pair limits via the normalizing `datetime()` read, never raw `=` | Stored period formats vary, and no unique constraint on `(element_id, period)` would catch a mispair — it would double-count silently |
| Remove `mode=replace` outright | Redundant with merge, and incomplete today (misses recurring transactions); removal fails loudly rather than degrading silently |
| No undo, no bulk merge, no duplicate detection | YAGNI; the confirm dialog states irreversibility plainly |
