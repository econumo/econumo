# Plan view: its own route (`/plan`)

**Date:** 2026-08-16 · **Branch:** `feature/budget-plan-view`

## Problem

The plan view lives inside `/budget` behind a Budget | Plan segmented tab
persisted in `useBudgetPeriodStore.budgetMode`. That has three costs:

- The plan is not addressable: no deep link, no bookmark, browser back does
  not return from Plan to Budget.
- The switch is page state, not navigation: nothing outside the page can
  point at the plan.

Plan is part of the budget, so it keeps the in-page Budget | Plan switch as
its navigation — but each side of that switch is now a route.

## Decisions

| Topic | Decision |
|---|---|
| URL | `/plan`, a sibling of `/budget` (not `/budget/plan`, not a query param) — the two are peers in the nav |
| Component shape | ONE page component (`BudgetPage`) with a `mode: 'budget' \| 'plan'` prop, mounted at both routes. Not two page components: after the toggle goes, the mode-dependent surface is only the settings-menu "hide empty rows" item and the body; the header, guards and the shared dialogs/mutations are common. Splitting is a possible later refactor, not part of this change |
| Remount on hop | The route elements are keyed (`key="budget"` / `key="plan"`) so React remounts the page when navigating between the two. Per-view local state (edit structure, selected currency pill, open dialog targets) therefore starts clean, preserving today's "leaving the view ends edit structure" rule. The query cache makes the remount free |
| Nav | Unchanged: the sidebar keeps its single **Budget** entry (plan is part of the budget, not a peer). The existing Budget \| Plan header tablist (desktop) and the settings-menu radio group (compact) STAY and become the navigation — each option `navigate()`s to `/budget` or `/plan`; the selected option reflects the current route |
| Store | `budgetMode` / `setBudgetMode` are removed from `useBudgetPeriodStore`. The stale key in the persisted `budgetPeriod` blob is ignored by zustand's merge; no migration |
| Analytics | `BUDGET_PLAN_OPEN` fires once per mount of the page in `plan` mode (was: on toggle-to-plan) |
| Toggle UI | Kept as-is visually; `aria-selected` / the radio value derive from the `mode` prop instead of the store; the `budgets.page.plan.toggle.*` catalogue keys stay |

## Frontend

### Routing (`web/src/app/routes.tsx`, `web/src/app/router-pages.ts`)

- `RouterPage.PLAN = '/plan'`.
- `{ path: '/budget', element: <BudgetPage key="budget" mode="budget" /> }`
- `{ path: '/plan', element: <BudgetPage key="plan" mode="plan" /> }`

Existing navigations to `RouterPage.BUDGET` (onboarding completion, budgets
list "open", sharing-requests dialog) are unchanged.

### `BudgetPage`

- New required prop `mode`. Every `budgetMode === 'plan'` branch reads the
  prop instead of the store. `switchBudgetMode(m)` becomes
  `navigate(BUDGET_MODE_ROUTE[m])` (no-op when `m === mode`); the header
  tablist and the compact radio group call it exactly as before. The
  settings menu keeps its plan-only "hide empty rows" checkbox.
- `useEffect(() => { if (mode === 'plan') trackEvent(METRICS.BUDGET_PLAN_OPEN) }, [mode])`
  — with the keyed remount this is exactly once per visit.
- Everything else (compact back button → home, period store, plan window and
  folds, currency pills + `ExpenseWidget`, dialogs) is untouched.

### Navigation / i18n

No sidebar or catalogue change.

## Testing

- `BudgetPage.test.tsx`: the memory router registers both routes with the
  keyed elements; the toggle tests assert `router.state.location.pathname`
  flips between `/budget` and `/plan` (desktop tabs and the compact radio),
  a keyed-remount test checks edit structure ends on the hop, and the
  "pills in both views" test navigates between routes. Fixture store setup
  drops `budgetMode`.
- `PlanSheet.test.tsx`: the two `setBudgetMode` store tests are deleted; the
  analytics rule moves to `BudgetPage.test.tsx` as "rendering `/plan` fires
  `BUDGET_PLAN_OPEN` exactly once; rendering `/budget` does not fire it".
- `metrics-coverage.test.ts` and the i18n guards stay green (no metric or
  catalogue change).

## Out of scope

- Splitting `BudgetPage` into `BudgetPage` + `PlanPage` over a shared shell.
- A sidebar Plan entry (considered and rejected: plan is part of the budget).
- Any backend change.
