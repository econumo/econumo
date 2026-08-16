# Plan view: overspend highlight on every overspent cell

**Date:** 2026-08-16 · **Branch:** `feature/budget-plan-view` · **Scope:** `web/` only

## Problem

The plan sheet (`web/src/features/budgets/PlanSheet.tsx`) already renders a
cell's actual in `text-destructive` when the spend exceeds the plan, but the
predicate is narrowed two ways that the 2026-08-02 plan-view spec introduced
("current and future months … a set plan"):

- past months never turn red (a July cell with 222.93 spent against a 0.00 plan
  stays grey once August starts);
- a cell with no plan stored stays grey however much was spent, even though the
  editable cell prints its plan as `0.00`.

The Budget view is stricter — its `available` pill (`budgeted − spent`) turns
red whenever negative, including with no limit at all — so the two views
disagree about the same month.

## Rule

An **expense-side parent row's** cell renders its actual in `text-destructive`
when `actual > planned`, where an unset plan (`planned === ''`) counts as `0`,
for **every** month in the window (past, current, future). Nothing else in the
cell changes.

Unchanged:

- income rows (an "over" income is good news);
- envelope child rows and Uncategorized (they carry no plan of their own);
- a future-month zero actual still prints as `—` and cannot be red (`0 > 0`);
- the totals/Balance math — `planTotals` already counts an overspent cell at its
  actual (`max(actual, planned)`), so this is display only.

## Implementation

- `web/src/features/budgets/planMath.ts`: a pure
  `isOverspent(type: BudgetElementType, cell: PlanCellDto | undefined): boolean`
  next to the other cell math. `false` for income types and for a missing cell.
- `PlanSheet.tsx` `ElementRow`: replace the inline `overspend` boolean with
  `isOverspent(el.type, cell)`; drop the now-stale comment.
- The 2026-08-02 spec's Cells section is superseded by this document for the
  highlight rule; the older file is left as written (historical record).

## Tests

- `planMath.test.ts` — `isOverspent`: past-month overspend → true; unset plan
  with spend → true; income row with actual > planned → false; actual equal to
  planned → false; zero actual with unset plan → false; missing cell → false.
- `PlanSheet.test.tsx` — one render assertion: a past-month expense cell whose
  actual exceeds its plan carries `text-destructive` on `[data-testid="cell-actual"]`,
  and the matching income cell does not.
