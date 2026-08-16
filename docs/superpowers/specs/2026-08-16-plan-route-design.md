# Plan view: its own route (`/plan`)

**Date:** 2026-08-16 · **Branch:** `feature/budget-plan-view`

## Problem

The plan view lives inside `/budget` behind a Budget | Plan segmented tab
persisted in `useBudgetPeriodStore.budgetMode`. That has three costs:

- The plan is not addressable: no deep link, no bookmark, browser back does
  not return from Plan to Budget.
- On compact screens the tablist does not fit the header, so the switch was
  moved into the settings menu — Plan is effectively hidden on phones.
- The desktop plan header is crowded (tablist + currency pills + settings +
  the period widget).

Budget (this month, execution) and Plan (the months ahead, planning) are
different activities; each gets its own destination.

## Decisions

| Topic | Decision |
|---|---|
| URL | `/plan`, a sibling of `/budget` (not `/budget/plan`, not a query param) — the two are peers in the nav |
| Component shape | ONE page component (`BudgetPage`) with a `mode: 'budget' \| 'plan'` prop, mounted at both routes. Not two page components: after the toggle goes, the mode-dependent surface is only the settings-menu "hide empty rows" item and the body; the header, guards and the shared dialogs/mutations are common. Splitting is a possible later refactor, not part of this change |
| Remount on hop | The route elements are keyed (`key="budget"` / `key="plan"`) so React remounts the page when navigating between the two. Per-view local state (edit structure, selected currency pill, open dialog targets) therefore starts clean, preserving today's "leaving the view ends edit structure" rule. The query cache makes the remount free |
| Nav | A **Plan** link directly under **Budget** on every breakpoint: expanded sidebar list, compact home sidebar, and the icon rail (`CalendarRange` icon beside the `Wallet`). Plan is a first-class mobile destination now (single-month degraded sheet as designed). No active-state styling — the Budget link has none |
| Store | `budgetMode` / `setBudgetMode` are removed from `useBudgetPeriodStore`. The stale key in the persisted `budgetPeriod` blob is ignored by zustand's merge; no migration |
| Analytics | `BUDGET_PLAN_OPEN` fires once per mount of the page in `plan` mode (was: on toggle-to-plan) |
| Toggle UI | Removed entirely: header tablist, compact settings-menu radio group, `BUDGET_MODES` / `BUDGET_MODE_LABEL`, and the `budgets.page.plan.toggle.*` catalogue keys in both languages |

## Frontend

### Routing (`web/src/app/routes.tsx`, `web/src/app/router-pages.ts`)

- `RouterPage.PLAN = '/plan'`.
- `{ path: '/budget', element: <BudgetPage key="budget" mode="budget" /> }`
- `{ path: '/plan', element: <BudgetPage key="plan" mode="plan" /> }`

Existing navigations to `RouterPage.BUDGET` (onboarding completion, budgets
list "open", sharing-requests dialog) are unchanged.

### `BudgetPage`

- New required prop `mode`. Every `budgetMode === 'plan'` branch reads the
  prop instead of the store; the `switchBudgetMode` helper, the header
  tablist and the compact radio group go away. The settings menu keeps its
  plan-only "hide empty rows" checkbox.
- `useEffect(() => { if (mode === 'plan') trackEvent(METRICS.BUDGET_PLAN_OPEN) }, [mode])`
  — with the keyed remount this is exactly once per visit.
- Everything else (compact back button → home, period store, plan window and
  folds, currency pills + `ExpenseWidget`, dialogs) is untouched.

### Navigation (`web/src/app/layouts/ApplicationLayout.tsx`)

- Expanded and compact list: `<Link to={RouterPage.PLAN}>` with
  `t('common.nav.plan')`, rendered right after the Budget link with the same
  classes.
- Icon rail: a `CalendarRange` icon link with `title={t('common.nav.plan')}`
  after the Wallet.

### i18n (`locales/{en,ru}.json`)

- Add `common.nav.plan`: en `"Plan"`, ru `"План"`.
- Remove `budgets.page.plan.toggle.budget` / `.plan` (both languages).

## Testing

- `BudgetPage.test.tsx`: the memory router registers both routes with the
  keyed elements; plan-mode tests render at `/plan` instead of clicking the
  tab; the "toggle moves into the settings menu on compact" test is
  removed; the "pills in both views" test renders each route in turn. Fixture
  store setup drops `budgetMode`.
- `PlanSheet.test.tsx`: the two `setBudgetMode` store tests are deleted; the
  analytics rule moves to `BudgetPage.test.tsx` as "rendering `/plan` fires
  `BUDGET_PLAN_OPEN` exactly once; rendering `/budget` does not fire it".
- `ApplicationLayout.test.tsx`: the nav shows **Plan** next to **Budget** in
  the list, and the rail carries the Plan icon link.
- `metrics-coverage.test.ts` and the i18n guards stay green (no new metric
  key; the toggle keys are removed from both catalogues together).

## Out of scope

- Splitting `BudgetPage` into `BudgetPage` + `PlanPage` over a shared shell.
- Active-link highlighting in the sidebar.
- Any backend change.
