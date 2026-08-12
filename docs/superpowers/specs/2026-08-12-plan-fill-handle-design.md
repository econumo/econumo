# Plan Sheet Fill Handle + Section Split — Design

Date: 2026-08-12
Status: Approved design, pending implementation plan

## Overview

Two plan-sheet refinements:

1. **Fill handle** — Excel-style: select an editable planned cell, grab the
   small square at its bottom-right corner, drag right, release — the planned
   value is copied into the covered later months. Copy only (no series
   extrapolation), rightward only, one row at a time.
2. **Income/expense section split** — make the two areas unmistakable: a
   labeled "Expenses" section header symmetric to the existing "Income" one,
   subtle distinct background tints per area, and a strong divider between
   them.
3. **Compact totals footer** — Income/Expenses/Net show ONE number per cell
   instead of the actual|planned pair: the actual for fully elapsed months, the
   per-cell-effective value (`max(actual, planned)` summed per element — the
   same numbers that feed the Balance row) for the current and future months.
   The Balance row is unchanged.
4. **Hide zero Uncategorized rows** — an Uncategorized row that shows only
   zeros is noise: in PLAN mode, drop an uncategorized row when every VISIBLE
   cell's actual is zero (re-evaluated as the window moves); in BUDGET mode,
   drop the Uncategorized section when the selected month's uncategorized
   spend is zero (past months' spend no longer props it up).

## Decisions

| Topic | Decision |
|---|---|
| Overwrite | Excel semantics: covered cells' existing planned values are replaced |
| Handle placement | On the SELECTED cell (the sheet's existing selection model), not on hover |
| Direction | Right only; leftward/downward drags clamp back to the source cell |
| Source eligibility | Editable parent planned cell (non-archived, not uncategorized, `canUpdateLimits`) with a NON-EMPTY planned value; hidden in single-column (mobile) mode |
| Drag tech | Raw pointer events + `setPointerCapture` on the handle — no new dependency (`@dnd-kit` is shaped for sortable lists, HTML5 drag has ghost-image/control problems) |
| Range limit | Visible columns only; no scroll-while-dragging past the window edge |
| Cancel | `Esc` during the drag aborts; releasing on the source cell is a no-op |
| Keyboard equivalent | None in v1 |

## Interaction

- The handle renders inside the selected cell's gridcell at the bottom-right
  corner, `cursor: crosshair`, with an aria-label (new i18n key, all 11
  catalogues: `budgets.page.plan.fill.handle_aria` = "Copy planned value to the
  following months").
- `pointerdown` on the handle captures the pointer and records the source
  column and the source cell's measured pixel width.
- During the drag, the target column is `fillTargetCol(startCol, deltaX,
  colWidth)` — a pure function in `planMath.ts`:
  `clamp(startCol + round(deltaX / colWidth), startCol, lastVisibleCol)`
  (colWidth ≤ 0 degrades to startCol). Covered cells (source+1 .. target)
  get a live highlight class.
- `pointerup` commits; `pointercancel`/`Esc` aborts with no calls.

## Commit path

New mutation in `queries.ts`:

```ts
useFillPlannedCells(planKey) // mutate form:
// { budgetId: Id; elementId: Id; amount: string; targets: { period: string; monthIndex: number }[] }
```

- Issues the EXISTING `POST /api/v1/budget/set-limit` once per target month —
  no backend changes; the wire contract stays frozen; each call is idempotent
  per (element, month).
- `onMutate`: optimistically patches every covered cell's `planned` in the plan
  cache (one `setQueryData` pass).
- On ANY request failure: invalidate the plan query (server truth resyncs); no
  partial rollback.
- `onSuccess`: fire `METRICS.BUDGET_PLAN_FILL_RIGHT`
  (`'appBudgetPlanFillRight'`, new key) once per fill (not per month), and
  invalidate each covered month's budget query key plus the plan key —
  mirroring `usePlanSetLimit`'s invalidation shape.

Targets are only ever later, visible months, so every target is editable by
construction (later than the source ⇒ not before the budget start; role checks
already gated the source).

## Income/expense section split

- A new **"Expenses" section header** renders at the top of the expense area,
  symmetric to the Income one: same `SectionHeader` component, foldable via the
  existing store fold mechanism under the key `'expense'` (folding collapses
  the expense folders AND loose rows, exactly as the Income header collapses
  its whole area). New i18n key `budgets.page.plan.section.expenses` =
  "Expenses" in all 11 catalogues.
- The hide-empty "{count} hidden — Show" notice for the expense area's loose
  rows moves onto this header (removing Task 7's title-less notice line);
  folder sections keep their own per-folder counts.
- **Tinted bands**: the income area gets a subtle green-tinted background
  (Tailwind emerald at low opacity, dark-mode safe), the expense area a subtle
  neutral/red tint — applied to the section wrapper so headers and rows read as
  one band. The archived section and totals footer stay untinted.
- A **strong divider** (a thicker border, not a new element) separates the
  income band from the expense band.
- Rendering-only: no wire, store-shape, or bucketing changes beyond reading
  the new `'expense'` fold key. The income-above-expense DOM-order invariant
  and its test are unchanged.

## Compact totals footer

- `planMath.planTotals` additionally exposes per-month `effectiveIncome` and
  `effectiveExpense` (already computed internally for `effectiveNet`; the net
  is their difference).
- The footer's Income / Expenses / Net rows render ONE value per cell:
  `effectiveIncome` / `effectiveExpense` / `effectiveNet`. No actual|planned
  stacking. The Balance row and its styling are unchanged.
- Rationale: for totals, the elapsed months' truth is the actual, and the
  forward-looking figure users plan against is the effective (max) value —
  the same convention the Balance projection already uses, so the four footer
  rows finally agree with each other.

## Hidden zero Uncategorized rows

- PLAN mode: `PlanSheet` drops an uncategorized row when every VISIBLE
  column's actual is zero (the backend only emits the row when the FETCHED
  window has spend, so a buffer-month-only actual could otherwise surface an
  all-zero visible row). Keyboard navigation and totals follow automatically
  (flat rows derive from the rendered buckets; totals iterate raw elements and
  zeros contribute nothing).
- BUDGET mode: `bucketElements` drops the Uncategorized element when its
  selected-month `spent` is zero (the backend emits it whenever spend exists
  in ANY month since the budget start). Frontend-only; the wire is untouched.

## Testing

- Unit: `fillTargetCol` (clamping both ends, rounding, zero/negative width
  degrade).
- Component (`PlanSheet.test.tsx`): handle visibility rules (selected editable
  non-empty cell only; absent on uncategorized/child/read-only cells and empty
  planned); a full drag (pointerdown → pointermove → pointerup, with a mocked
  source-cell `getBoundingClientRect`) asserting one `set-limit` per covered
  month with the right `period`/`amount`, the optimistic patch, and overwrite
  of an existing planned value; Esc-cancel issues no requests; releasing on the
  source cell issues no requests.
- Section split: the Expenses header renders with its label and folds/unfolds
  the whole expense area (store key `'expense'`, persisted); the hidden-count
  notice sits on the header; income band, expense band, and the divider carry
  distinguishable classes (assert via `data-testid`/class, not pixel colors).
- `metrics-coverage` and the i18n guards pick up the new keys automatically.

## Out of scope (v1)

- Downward/multi-row fill, series extrapolation, leftward fill.
- Scroll-extending the drag beyond the visible window.
- A keyboard fill equivalent (Excel's Ctrl+R).
- Copying an EMPTY planned value (i.e. clear-right); the handle simply doesn't
  render on empty cells.
