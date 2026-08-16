# Plan View Overspend Highlight Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** In the plan sheet, render an expense cell's actual in red whenever it exceeds the plan — every month (past included), an unset plan counting as 0.

**Architecture:** Pull the overspend predicate out of `PlanSheet.tsx`'s JSX into a pure `isOverspent(type, cell)` in `planMath.ts` (unit-tested), and call it from `ElementRow`. Display only — totals/Balance math is untouched.

**Tech Stack:** React 19 + Vite SPA in `web/`, vitest + Testing Library + msw. Run tests from `web/` with `pnpm vitest run <file>`.

**Spec:** `docs/superpowers/specs/2026-08-16-plan-overspend-highlight-design.md`

## Global Constraints

- The red is the existing `text-destructive` class on the `[data-testid="cell-actual"]` span; nothing else in the cell changes.
- Income rows (`isIncomeType`), envelope child rows, and Uncategorized are never red.
- Comments only for the *why* (CLAUDE.md "Comments — write sparingly").
- All commands run from `/Users/dmitry/Development/Personal/econumo/econumo/web`.

---

### Task 1: `isOverspent` in planMath

**Files:**
- Modify: `web/src/features/budgets/planMath.ts` (add an export near `visibleSectionRows`, ~line 104)
- Test: `web/src/features/budgets/planMath.test.ts`

**Interfaces:**
- Produces: `export function isOverspent(type: BudgetElementType, cell: PlanCellDto | undefined): boolean` — Task 2 imports it from `./planMath`.

- [ ] **Step 1: Write the failing tests**

Append to `web/src/features/budgets/planMath.test.ts` (the file already imports `describe/expect/it` from vitest). Add `isOverspent` to the existing `./planMath` import list, and add `BudgetElementType` to the imports:

```ts
import { BudgetElementType } from '@/api/dto/budget'
```

then at the end of the file:

```ts
describe('isOverspent', () => {
  const { CATEGORY, INCOME_CATEGORY } = BudgetElementType

  it('is true when an expense actual clears a set plan', () => {
    expect(isOverspent(CATEGORY, { actual: '222.93', planned: '0' })).toBe(true)
    expect(isOverspent(CATEGORY, { actual: '150.01', planned: '150' })).toBe(true)
  })

  it('treats an unset plan as 0', () => {
    expect(isOverspent(CATEGORY, { actual: '50', planned: '' })).toBe(true)
    expect(isOverspent(CATEGORY, { actual: '0', planned: '' })).toBe(false)
  })

  it('is false at exactly the plan', () => {
    expect(isOverspent(CATEGORY, { actual: '150', planned: '150' })).toBe(false)
  })

  it('never flags an income row', () => {
    expect(isOverspent(INCOME_CATEGORY, { actual: '3000', planned: '2000' })).toBe(false)
    expect(isOverspent(INCOME_CATEGORY, { actual: '300', planned: '' })).toBe(false)
  })

  it('is false for a missing cell', () => {
    expect(isOverspent(CATEGORY, undefined)).toBe(false)
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `pnpm vitest run src/features/budgets/planMath.test.ts`
Expected: FAIL — `isOverspent` is not exported (`TypeError: isOverspent is not a function` or a module import error).

- [ ] **Step 3: Implement**

In `web/src/features/budgets/planMath.ts`, add `PlanCellDto` and `BudgetElementType` to the existing `@/api/dto/budget` imports (`isIncomeType` is already imported there — check the import line and extend it), then add after `visibleSectionRows`:

```ts
/** the overspend highlight: an expense actual past its plan, in ANY month — an
 *  unset plan reads as 0 everywhere else in the grid, so it counts as 0 here too */
export function isOverspent(type: BudgetElementType, cell: PlanCellDto | undefined): boolean {
  if (!cell || isIncomeType(type)) {
    return false
  }
  return cmp(cell.actual, cell.planned === '' ? '0' : cell.planned) > 0
}
```

`cmp` comes from `@/lib/decimal` — it is already imported in `planMath.ts` (verify; add to that import if not).

- [ ] **Step 4: Run the tests to verify they pass**

Run: `pnpm vitest run src/features/budgets/planMath.test.ts`
Expected: PASS, all tests green including the five new ones.

- [ ] **Step 5: Commit**

```bash
git add src/features/budgets/planMath.ts src/features/budgets/planMath.test.ts
git commit -m "feat(web): isOverspent — plan cell overspend rule, any month, unset plan = 0

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: PlanSheet uses `isOverspent`

**Files:**
- Modify: `web/src/features/budgets/PlanSheet.tsx:476-478` (the `overspend` boolean in `ElementRow`) and its `./planMath` import block (~line 62)
- Test: `web/src/features/budgets/PlanSheet.test.tsx`

**Interfaces:**
- Consumes: `isOverspent(type, cell)` from `./planMath` (Task 1).

- [ ] **Step 1: Write the failing render test**

Append to `web/src/features/budgets/PlanSheet.test.tsx` (top level, after the existing `'/plan renders the sheet…'` test is fine). The fixture `fixtureWirePlan` (`src/test/fixtures.ts`) has months May–Aug 2026; `cat-food` (expense) has July `{ actual: '125', planned: '' }`, `cat-freelance` (income) has May `{ actual: '300', planned: '' }`. Pinning `planFirstMonth` to May with jsdom's 3-column fallback puts May at col 0 and July at col 2.

```ts
it('overspend turns the actual red in a past month and with no plan set; never on income', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-05-01' })
  renderPage()
  await screen.findByText(/may/i)
  // July: 125 spent, no plan stored — a past month by the time this runs (fixture months are 2026)
  const foodJuly = screen.getAllByTestId('plan-cell-cat-food:2')[0]
  expect(within(foodJuly).getByTestId('cell-actual')).toHaveClass('text-destructive')
  // May: 120 spent against a 150 plan — under, so plain
  const foodMay = screen.getAllByTestId('plan-cell-cat-food:0')[0]
  expect(within(foodMay).getByTestId('cell-actual')).not.toHaveClass('text-destructive')
  // income over an unset plan is not overspend
  const freelanceMay = screen.getAllByTestId('plan-cell-cat-freelance:0')[0]
  expect(within(freelanceMay).getByTestId('cell-actual')).not.toHaveClass('text-destructive')
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `pnpm vitest run src/features/budgets/PlanSheet.test.tsx -t "overspend turns the actual red"`
Expected: FAIL on the first assertion — July is a past month, so the current `m >= ctx.cur` guard leaves the span with `text-muted-foreground` only.

- [ ] **Step 3: Switch `ElementRow` to `isOverspent`**

In `web/src/features/budgets/PlanSheet.tsx`, add `isOverspent` to the `./planMath` import list (keep it alphabetical between `folderSides` and `makePlanExchange`), then replace lines 476-478:

```tsx
          // overspend highlight: expense side only, current/future months, a set plan the actual has already cleared
          const overspend =
            !isIncomeType(el.type) && m >= ctx.cur && !!cell && cell.planned !== '' && cmp(cell.actual, cell.planned) > 0
```

with:

```tsx
          const overspend = isOverspent(el.type, cell)
```

`cmp` and `isIncomeType` stay imported in `PlanSheet.tsx` — both are still used elsewhere in the file (`PlanTotals`, `PlanBalanceRow`, `MoveToFolderDialog`).

- [ ] **Step 4: Run the plan sheet suite**

Run: `pnpm vitest run src/features/budgets/PlanSheet.test.tsx`
Expected: PASS, including the new test. If an existing test asserted the OLD narrowing (a past-month cell without `text-destructive`), it is now wrong by spec — update that assertion, don't weaken the new one.

- [ ] **Step 5: Lint + full frontend suite**

Run: `pnpm lint && pnpm test`
Expected: lint clean; all suites green (`metrics-coverage.test.ts` is unaffected — no new `METRICS` key, this is display only, no user action).

- [ ] **Step 6: Commit**

```bash
git add src/features/budgets/PlanSheet.tsx src/features/budgets/PlanSheet.test.tsx
git commit -m "feat(web): plan sheet flags overspend in every month, unset plan counts as 0

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: Visual check on the demo DB

**Files:** none modified.

- [ ] **Step 1: Confirm the scratch backend is up**

Run: `lsof -nP -iTCP:8181 -sTCP:LISTEN`
Expected: an `econumo` process. If absent, from the repo root:

```bash
S=/private/tmp/claude-501/-Users-dmitry-Development-Personal-econumo-econumo/d91645d5-262f-4c9c-bc8e-ad573c9448be/scratchpad; go build -o ./econumo ./cmd/econumo && (DATABASE_URL="sqlite:///$S/verify-demo.sqlite" PORT=8181 ECONUMO_ANALYTICS=false ECONUMO_ADMIN_PORT= ECONUMO_ADMIN_TOKEN= ECONUMO_BILLING_URL= nohup ./econumo serve > $S/serve.log 2>&1 &)
```

- [ ] **Step 2: Open the plan and check the highlighted cells**

Open `http://localhost:9000/plan` in the browser pane (vite on :9000 is the user's long-running dev server; the session is already logged in as john@econumo.com). In the July column, `Italy 2026` (222.93 spent vs 0.00 planned) and `Шоппинг` in August (50.00 spent, no plan) must show the actual in red; `Salary`/`Freelance` (income) must not. Take a screenshot as proof.

Query to confirm from the DOM:

```js
[...document.querySelectorAll('[data-testid="cell-actual"].text-destructive')].map(e => e.closest('[role="gridcell"]').getAttribute('aria-label'))
```

Expected: a non-empty list containing entries like `"Italy 2026, July: actual 222.93, planned 0.00"` and no income row names.
