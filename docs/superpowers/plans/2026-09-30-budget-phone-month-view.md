# Phone Single Month View + Tablet Item Sheet Implementation Plan (redesign stage 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:**
- **Phone (below 640 px):** `/budget` and `/plan` both render one single-month view. Rows show Budget and Spent, and a tap opens an item sheet.
- **Tablet (640–1023 px):** tapping an amount cell opens the same item sheet.
- **Both:** no long-press remains anywhere. Every stage-1 touch path (tap-Available, long-press, the actions modal, the set-limit sheet's "Comments (N)" button) is gone.

**Architecture:**
- **Pure maths:** `rowState`/`rowProgress`/`carryOver` go in `budgetMath.ts`. A new `phoneMonth.ts` holds the month's income, balance and savings-balance figures from `get-budget-plan`, and the sheet's target type.
- **Components:** `ElementSheet.tsx` (the item sheet) is shared by the phone view and the tablet tables. `PhoneMonthView.tsx` is the phone list.
- **Wiring:**
  - `BudgetPage.tsx` hosts the sheet for the Budget view and the phone view.
  - `PlanSheet.tsx` hosts it for the Plan grid.
  - Both route the sheet's actions to the existing `SetLimitDialog`, `CommentsPanel` and `BudgetTransactionsDialog`.
  - `CellShell` loses its touch half (long-press and the actions modal) and keeps the desktop hover preview and Shift+F2.

**Tech Stack:** React 19, TypeScript, Tailwind v4, shadcn/Radix + vaul (`ResponsiveDialog`), TanStack Query, react-i18next, vitest + Testing Library + MSW, pnpm, oxlint.

**Spec:** `docs/superpowers/specs/2026-09-29-budget-ux-redesign-design.md`. This plan implements:
- Part 1 (phone single month view)
- Part 2's "Tablet" section, as revised on 2026-09-30: a tap opens the item sheet, with no long taps
- the Row state rule
- the phone half of the Currency display rule
- the matching items of Removals, i18n and Testing

Part 3 (desktop/tablet density and the currency column) is stage 3.

## Global Constraints

- Frontend only. No endpoint, DTO, permission or analytics contract changes. Add no `METRICS` keys: opening a sheet is navigation, and set limit and comment writes keep their existing events at the same choke points.
- Phone = `useIsPhone()` (`(max-width: 639px)`); touch/tablet = `useIsCompact()` (`(max-width: 1023px)`) and not a phone. Tablet and desktop keep the Budget and Plan table layouts; on a tablet a tap on an amount cell opens the item sheet (Tasks 5–6). **No long-press anywhere** (Dmitry, 2026-09-30).
- Base branch `v1.6-dev`. Work on `feature/budget-phone-month-view`.
- Row state rule, verbatim from the spec: none = `budget` zero and `spent` zero; ok = `available ≥ 0` and `spent ≤ budget`; covered = `available ≥ 0` and `spent > budget` (amber); over = `available < 0` (red). Progress = `min(spent / budget, 1)`, hidden when `budget` is zero. Future months (after the current month) have no Spent: shown as `—`, no bar, state `none`. Carry-over = `available − (budget − spent)`. Income and savings rows never use amber/red.
- Currency, phone: no currency symbol on any amount; the budget currency code appears once, at the left of the heading row; an element whose currency differs from the budget's gets a small code tag next to its name, and its row amounts are in that currency; the item sheet repeats the code beside each amount.
- i18n: every new key goes into all 11 catalogues (`locales/{de,en,es,fr,it,nl,pl,pt,ru,uk,zh}.json`). `en` is the reference, and `{var}` placeholder sets must match across languages. **Hand-edit the JSON.** Never round-trip it through `json.dumps` or a formatter: that reformatted all 11 files once already.
- Comments in code: sparingly, only the non-obvious *why* (see `CLAUDE.md` "Comments — write sparingly").
- Any change to user-observable behaviour updates `docs/regression-test-plan.md` in the same task.
- Commands run from `web/`:
  - one file: `pnpm vitest run <path> --maxWorkers=2`
  - budgets suite: `pnpm vitest run src/features/budgets --maxWorkers=2`
  - typecheck: `pnpm exec tsc -b`
  - lint: `pnpm lint`
  - The machine is memory-constrained: always pass `--maxWorkers=2`.
  - The full suite has one known pre-existing failure, `transaction.test.ts` (Blob `instanceof`). Ignore it.
- Go i18n guard, run from the repo root: `GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/test/i18ntest/`.
- Testing notes:
  - user-event with Radix modals needs `userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })` when a click lands while a dialog is animating.
  - Fake timers need `vi.useFakeTimers({ shouldAdvanceTime: true })`.
  - Viewport mocks:
    - phone: `window.matchMedia` mocked with `matches: true` for every query
    - tablet: `matches: q.includes('1023')`
    - desktop: `matches: false`
- Commit message style: `feat: <Sentence case summary>` (or `refactor:`/`test:`/`docs:`), ending with the trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.


## Rulings made while writing this plan

These go to the SDD ledger as given decisions:

1. **`widgetMath` is not reused.** Stage 1 deleted its only caller (`ExpenseWidget`), and Task 1 deletes the dead code. The sheet's rate note needs the month's rate:
   - the Budget view and the phone view pass `makeBudgetExchange(budget, currencies)`, as the old widget did;
   - the Plan grid passes the rates of the tapped column's month (`planMonthExchange`, Task 2).
2. **Plan window.** The phone view calls `useBudgetPlan(budgetId, selectedDate, 1)`. Its built-in ±2-month buffer fetches 5 months instead of the spec's `months=1`. The spec's point was "one month on screen", not the request size. The buffer makes neighbouring months instant and shares the cache and mutation invalidation with the Plan view. The month's column is found by `plan.months.indexOf(selectedDate)`, and placeholder data from another window counts as "not loaded".
3. **Sheets replace one another directly.** The sheet closes and the next dialog opens in the same state update. This is the stage-1 phone path (`SetLimitDialog` → comments) in daily use on the iOS PWA. There is no pending-action-in-`onCloseAutoFocus` machinery.
4. **Edit-structure mode on a phone** keeps rendering the route's existing editor (`BudgetTable` on `/budget`, `PlanSheet` on `/plan`), unchanged. The phone view is the non-edit view only.
5. **Tablet tap opens the item sheet.** Dmitry, 2026-09-30: "it should show the same details popup on tap as on the mobile. Let's avoid long taps."
   - **Budget view:** a tap on the budgeted amount opens the sheet. This applies to every non-uncategorized row, editable or not, the Archive section included.
   - **Savings block:** a tap on the planned amount opens the sheet.
   - **Plan grid:** a tap anywhere on a month cell (except the uncategorized row, and in edit mode) selects it and opens the sheet for that element and month. The sheet shows Planned and Actual (Budget/Spent, Planned/Received or Planned/Saved/Balance by type).
   - **Kept:** the comment marker's tap still opens the anchored popover, and a tablet's Spent tap still opens transactions.
   - **Removed:** long-press, the actions modal, and tap-Available.
6. **Rows that have one action skip the sheet.** On the phone, reporting-tag rows and the children of an expanded envelope/tag open `BudgetTransactionsDialog` directly. A sheet would be a one-button detour.
7. **One sheet target for plan data.** The target is `{ kind: 'plan', cell: PlanCellFigures }` and serves two callers:
   - the phone's income rows;
   - every Plan grid cell on a tablet.

   Income sheets say "Set plan", and the dialog title matches. Setting from a `plan` target uses the plan mutation: `usePlanSetLimit` on the phone, `PlanSheet`'s own `commit` on the tablet.

   On the phone, income lists active rows plus the income "Uncategorized" row when it is non-zero. Archived income rows are not listed, but their money stays in the totals, as in the Plan view.
8. **Income fold state** uses the persisted `unfoldedElements` store under the reserved key `__phone_income__`, the same pattern as the reporting-tags folder.
9. **Foreign-currency sheet line.**
   - The `≈ {converted} {budgetCode}` line converts the actual figure: Spent (the server's `budgetSpent` for monthly expense rows), Received or Saved.
   - The rate note is `1 {budgetCode} = {rate} {code}` for the sheet's month.
10. **Empty real folders are hidden on the phone view**, because it cannot create envelopes. The "No folder" header is shown only when real folders exist.
11. **The accessible name of the tablet sheet button** is `details {name}`. It follows the untranslated `limit {name}` / `comments {name}` convention these cells already use.

## Review Focus

These are the failure modes most likely to bite a person, most likely first. Each has a pinning test in the task named.

1. **Foreign-currency elements.**
   - The row shows the code tag and amounts in the element's own currency, never converted and never with a symbol.
   - The sheet shows the code beside every amount, plus the `≈` line and the rate note.
   - The rate note uses the sheet's month. That matters for a Plan grid column that is not the Budget view's month.
   - Tests: Task 4, Task 6, Task 7.
2. **Future months.**
   - Spent/Received/Saved read `—`, with no bar, no amber/red and no state sentence, even when the server sends a non-zero figure.
   - Tests: Task 1, Task 4, Task 7.
3. **`get-budget-plan` still loading, failed, or holding another window's placeholder data (phone).**
   - The expense list, savings and the Expenses/Available totals render normally.
   - The income summary, Balance at month end, Total savings and Transfers lines are absent. They are never stale and never another month's.
   - Tests: Task 2, Task 8.
4. **Read-only callers.** This covers a guest, `readonly` access, an archived budget, a month before the start or after the end, an archived element, a deleted savings account, and the uncategorized row.
   - The sheet has no Set budget / Set plan.
   - A read-only thread with no comments shows no comments link.
   - Uncategorized never shows one.
   - On a tablet the sheet is still reachable by tap, so a guest can reach comments.
   - Tests: Task 4, Task 5, Task 6, Task 8.
5. **Touch without long-press.**
   - A tablet tap opens exactly one sheet.
   - Holding a finger on a cell opens nothing extra and never suppresses the tap.
   - The comment marker's tap opens the popover, not the sheet as well.
   - Choosing Set budget, Comments or Transactions replaces the sheet; closing that returns to the table or list.
   - The committed amount lands on the sheet's month.
   - Tests: Task 5, Task 6, Task 8.

---

### Task 1: Row state maths

**Files:**
- Modify: `web/src/features/budgets/budgetMath.ts`
- Test: `web/src/features/budgets/budgetMath.test.ts`

**Interfaces:**
- Consumes: nothing new.
- Produces (exported from `budgetMath.ts`):
  - `export type RowState = 'none' | 'ok' | 'covered' | 'over'`
  - `export function rowState(row: { budgeted: string; spent: string; available: string }, future?: boolean): RowState`. `available` is the **displayed** Available (`displayAvailable(el)`), not the wire field.
  - `export function rowProgress(row: { budgeted: string; spent: string }, future?: boolean): number | null`
  - `export function carryOver(el: { available: string; spent: string }): string`. It takes the **wire** element (`el.available` is the wire field). The result equals `displayAvailable(el) − (el.budgeted − el.spent)`.
- Removes: `widgetMath`, `WidgetMath`.

- [ ] **Step 1: Write the failing tests**

In `budgetMath.test.ts`:
1. Replace `widgetMath` in the import line with `rowState, rowProgress, carryOver`.
2. Delete the whole `widgetMath` test (the block that calls `widgetMath(...)` three times, currently lines ~150–166).
3. Append:

```ts
describe('rowState', () => {
  it('is none with no budget and no spending', () => {
    expect(rowState({ budgeted: '0', spent: '0', available: '0' })).toBe('none')
    // even with a carried-over balance: the spec's table checks this row first
    expect(rowState({ budgeted: '0', spent: '0', available: '-20' })).toBe('none')
  })
  it('is ok within budget', () => {
    expect(rowState({ budgeted: '700', spent: '650', available: '50' })).toBe('ok')
    expect(rowState({ budgeted: '700', spent: '700', available: '0' })).toBe('ok')
  })
  it('is covered when over this month but carry-over keeps Available non-negative', () => {
    expect(rowState({ budgeted: '700', spent: '801.37', available: '649.32' })).toBe('covered')
    expect(rowState({ budgeted: '0', spent: '10', available: '5' })).toBe('covered')
  })
  it('is over when Available is negative', () => {
    expect(rowState({ budgeted: '700', spent: '801.37', available: '-101.37' })).toBe('over')
    expect(rowState({ budgeted: '100', spent: '50', available: '-1' })).toBe('over')
  })
  it('is none for a future month whatever the figures', () => {
    expect(rowState({ budgeted: '700', spent: '801.37', available: '-101.37' }, true)).toBe('none')
  })
})

describe('rowProgress', () => {
  it('is spent over budget, capped at 1', () => {
    expect(rowProgress({ budgeted: '200', spent: '50' })).toBe(0.25)
    expect(rowProgress({ budgeted: '200', spent: '500' })).toBe(1)
  })
  it('is null with no budget or in a future month', () => {
    expect(rowProgress({ budgeted: '0', spent: '50' })).toBeNull()
    expect(rowProgress({ budgeted: '200', spent: '50' }, true)).toBeNull()
  })
  it('never goes below zero (refunds)', () => {
    expect(rowProgress({ budgeted: '200', spent: '-30' })).toBe(0)
  })
})

it('carryOver is what earlier months left: displayed Available less this month’s budget less spent', () => {
  // the spec's worked example: Budget 700, Spent 801.37, Available 649.32 -> 750.69
  const el = { budgeted: '700', spent: '801.37', available: '-50.68' }
  expect(displayAvailable(el)).toBe('649.32')
  expect(carryOver(el)).toBe('750.69')
})
```

- [ ] **Step 2: Run the tests and check that they fail**

Run: `pnpm vitest run src/features/budgets/budgetMath.test.ts --maxWorkers=2`
Expected: FAIL, because `rowState` / `rowProgress` / `carryOver` are not exported.

- [ ] **Step 3: Implement**

In `budgetMath.ts`:
1. Change the decimal import to `import { add, cmp, div, isZero } from '@/lib/decimal'`. `abs` is dropped once `widgetMath` is gone; keep any import still in use.
2. Remove `BudgetBalanceDto` from the type import if nothing else uses it.
3. Delete `WidgetMath` and `widgetMath`.
4. Append:

```ts
export type RowState = 'none' | 'ok' | 'covered' | 'over'

/** The one colour rule for an expense row. `available` is the displayed Available
 *  (`displayAvailable`); a future month has no spending yet, so it has no state. */
export function rowState(row: { budgeted: string; spent: string; available: string }, future = false): RowState {
  if (future || (isZero(row.budgeted) && isZero(row.spent))) {
    return 'none'
  }
  if (cmp(row.available, '0') < 0) {
    return 'over'
  }
  return cmp(row.spent, row.budgeted) > 0 ? 'covered' : 'ok'
}

export function rowProgress(row: { budgeted: string; spent: string }, future = false): number | null {
  if (future || cmp(row.budgeted, '0') <= 0) {
    return null
  }
  return Math.max(0, Math.min(Number(div(row.spent, row.budgeted)), 1))
}

// the wire `available` already nets this month's spending against what earlier
// months left, so adding the spending back leaves the carry-over alone
export const carryOver = (el: { available: string; spent: string }): string => add(el.available, el.spent)
```

- [ ] **Step 4: Run the tests and check that they pass**

Run: `pnpm vitest run src/features/budgets/budgetMath.test.ts --maxWorkers=2`, then `pnpm exec tsc -b`.
Expected: PASS; tsc clean. `widgetMath` has no other importer; `grep -rn widgetMath src` returns nothing.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/budgets/budgetMath.ts web/src/features/budgets/budgetMath.test.ts
git commit -m "feat: Row state rule for budget rows

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Plan month figures and the sheet target (`phoneMonth.ts`)

**Files:**
- Create: `web/src/features/budgets/phoneMonth.ts`
- Test: `web/src/features/budgets/phoneMonth.test.ts`
- Modify: `web/src/features/budgets/planMath.ts` (add `planMonthExchange`)
- Test: `web/src/features/budgets/planMath.test.ts`

**Interfaces:**
- Consumes (`planMath.ts`, unchanged): `bucketPlanRows`, `planTotals`, `balanceRow`, `savingsBalanceRow`, `everydayBalanceRow`, `planHasSavingsData`, `makePlanExchange`.
- Produces in `planMath.ts`:

```ts
/** exchange between any two currencies at one plan month's rates (the sheet's rate note needs base -> element) */
export function planMonthExchange(plan: BudgetPlanDto, currencies: CurrencyDto[], monthIndex: number): (from: Id, to: Id, amount: string) => string
```

- Produces in `phoneMonth.ts`:

```ts
/** one element's figures in one plan month */
export interface PlanCellFigures {
  element: PlanElementDto
  planned: string          // '0' when the cell has no plan
  actual: string
  closingBalance?: string  // savings rows only
}
export interface PlanMonthFigures {
  month: string            // 'YYYY-MM-01'
  index: number            // the month's column in plan.months
  income: { rows: PlanCellFigures[]; planned: string; received: string }  // totals in budget currency
  balance: string          // Balance at month end: the Plan view's Balance line (the everyday side when savings data exist)
  savingsBalance: string | null  // Total savings; null when the plan carries no savings data
  transfersNet: string     // budget currency
}
export function planCellFigures(element: PlanElementDto, index: number): PlanCellFigures
export function planMonthFigures(plan: BudgetPlanDto, currencies: CurrencyDto[], month: string, now?: Date): PlanMonthFigures | null

export type SheetTarget =
  | { kind: 'expense'; element: BudgetElementDto }      // a Budget-view element (get-budget)
  | { kind: 'savings'; row: BudgetSavingsElementDto }   // a Budget-view savings row (get-budget)
  | { kind: 'plan'; cell: PlanCellFigures }             // any plan row in one month (income on the phone; every Plan grid cell on a tablet)
export interface SheetCell { id: Id; name: string; currencyId: Id; amount: string }
export function sheetCell(target: SheetTarget, baseCurrencyId: Id): SheetCell
```

- [ ] **Step 1: Write the failing tests**

Append to `planMath.test.ts`. Use its existing imports; add `planMonthExchange` to the `./planMath` import, and import `fixtureWirePlan` from `@/test/fixtures` if it is not already imported:

```ts
it('planMonthExchange converts at the given month’s rates', () => {
  const plan = JSON.parse(JSON.stringify(fixtureWirePlan)) as BudgetPlanDto
  const usd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
  const eur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 }
  const may = planMonthExchange(plan, [usd, eur], 0)
  const aug = planMonthExchange(plan, [usd, eur], 3)
  // the fixture's EUR rate moves from 0.90 (May) to 0.93 (Aug): the two months must differ
  expect(may('cur-eur', 'cur-usd', '100')).not.toBe(aug('cur-eur', 'cur-usd', '100'))
  expect(may('cur-usd', 'cur-usd', '100')).toBe('100')
})
```

Create `phoneMonth.test.ts`:

```ts
import { coerceBudgetFixture } from '@/test/coerceBudget'
import { fixtureWireBudget, fixtureWirePlan } from '@/test/fixtures'
import type { BudgetPlanDto } from '@/api/dto/budget'
import { cmp } from '@/lib/decimal'
import { planCellFigures, planMonthFigures, sheetCell } from './phoneMonth'

const usd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const eur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 }

// the fixture minus its EUR row, so every figure below is plain USD arithmetic
function usdPlan(): BudgetPlanDto {
  const plan = JSON.parse(JSON.stringify(fixtureWirePlan)) as BudgetPlanDto
  plan.structure.elements = plan.structure.elements.filter((el) => el.id !== 'env-eur')
  return plan
}
const past = new Date(2026, 11, 1)

it('reads the income rows and totals of the selected month', () => {
  const f = planMonthFigures(usdPlan(), [usd, eur], '2026-07-01', past)!
  expect(f.index).toBe(2)
  expect(f.income.rows.map((r) => [r.element.id, r.planned, r.actual])).toEqual([
    ['ie1', '2000', '0'],
    ['cat-freelance', '500', '400'],
  ])
  expect(cmp(f.income.planned, '2500')).toBe(0)
  expect(cmp(f.income.received, '400')).toBe(0)
})

it('lists the income Uncategorized row only in a month it received something', () => {
  expect(planMonthFigures(usdPlan(), [usd, eur], '2026-06-01', past)!.income.rows.map((r) => r.element.id)).toContain('uncategorized')
  expect(planMonthFigures(usdPlan(), [usd, eur], '2026-07-01', past)!.income.rows.map((r) => r.element.id)).not.toContain('uncategorized')
})

it('an unplanned cell counts as a zero plan', () => {
  const ie1 = usdPlan().structure.elements.find((el) => el.id === 'ie1')!
  expect(planCellFigures(ie1, 0)).toEqual({ element: ie1, planned: '0', actual: '2000' })
  expect(planCellFigures(ie1, 9)).toEqual({ element: ie1, planned: '0', actual: '0' })
})

it('carries a savings cell’s closing balance', () => {
  const row = { id: 'acc-s1', type: 0, name: 'Rainy day', icon: 'savings', currencyId: 'cur-usd', isArchived: 0, folderId: null, position: 0, ownerUserId: 'u1',
    cells: [{ actual: '40', planned: '100', closingBalance: '1040' }], children: [] } as unknown as Parameters<typeof planCellFigures>[0]
  expect(planCellFigures(row, 0).closingBalance).toBe('1040')
})

it('chains Balance at month end from the opening balance, past months at actuals', () => {
  // opening 500; May +2300 -200; June +2050 -215 -100 transfers; July +400 -190
  const f = planMonthFigures(usdPlan(), [usd, eur], '2026-07-01', past)!
  expect(cmp(f.balance, '4545')).toBe(0)
  expect(f.savingsBalance).toBeNull()
  expect(cmp(f.transfersNet, '0')).toBe(0)
  expect(cmp(planMonthFigures(usdPlan(), [usd, eur], '2026-06-01', past)!.transfersNet, '-100')).toBe(0)
})

it('projects the current month at the larger of plan and actual, as the Plan view does', () => {
  // July current: income 2000 + 500, expenses 250 + 125 + 50 + 5
  const f = planMonthFigures(usdPlan(), [usd, eur], '2026-07-01', new Date(2026, 6, 15))!
  expect(cmp(f.balance, '6405')).toBe(0)
})

it('is null for a month outside the fetched window', () => {
  expect(planMonthFigures(usdPlan(), [usd, eur], '2027-01-01')).toBeNull()
})

it('splits Total savings off Balance when the plan carries savings data', () => {
  const plan = usdPlan()
  plan.structure.savings = [
    {
      id: 'acc-s1', type: 5, name: 'Rainy day', icon: 'savings', currencyId: 'cur-usd', ownerUserId: 'u1', isArchived: 0, position: 0,
      cells: plan.months.map(() => ({ actual: '0', planned: '' })),
    },
  ]
  plan.savingsOpeningBalances = [{ currencyId: 'cur-usd', amount: '1000' }]
  plan.openingBalances = [{ currencyId: 'cur-usd', amount: '1500' }]
  const f = planMonthFigures(plan, [usd, eur], '2026-07-01', past)!
  expect(cmp(f.savingsBalance!, '1000')).toBe(0)
  // combined 1500 + 4045 of activity, minus the 1000 held in savings
  expect(cmp(f.balance, '4545')).toBe(0)
})

it('sheetCell names the id, currency and settable amount of each target kind', () => {
  const budget = coerceBudgetFixture(fixtureWireBudget)
  const food = budget.structure.elements.find((el) => el.id === 'cat-food')!
  expect(sheetCell({ kind: 'expense', element: food }, 'cur-usd')).toEqual({ id: 'cat-food', name: 'Food', currencyId: 'cur-usd', amount: '200' })
  const ie1 = usdPlan().structure.elements.find((el) => el.id === 'ie1')!
  expect(sheetCell({ kind: 'plan', cell: { element: ie1, planned: '2000', actual: '0' } }, 'cur-usd')).toEqual({
    id: 'ie1', name: 'Salaries', currencyId: 'cur-usd', amount: '2000',
  })
  const saving = { id: 'acc-s1', type: 5 as const, name: 'Rainy day', icon: 'savings', currencyId: 'cur-eur', ownerUserId: 'u1', isArchived: 0 as const, position: 0, budgeted: '50', spent: '0', available: '50' }
  expect(sheetCell({ kind: 'savings', row: saving }, 'cur-usd')).toEqual({ id: 'acc-s1', name: 'Rainy day', currencyId: 'cur-eur', amount: '50' })
})
```

About the savings-split expectation: through July, the combined balance is 1500 + 4045 = 5545. The savings side is 1000 (no flows, nothing planned). The everyday side is 5545 − 1000 = 4545. If the Plan view's own functions give a different figure on this input, fix the expectation, not the implementation. The rule is "same numbers as the Plan view's Balance and Total savings lines".

- [ ] **Step 2: Run the tests and check that they fail**

Run: `pnpm vitest run src/features/budgets/phoneMonth.test.ts src/features/budgets/planMath.test.ts --maxWorkers=2`
Expected: FAIL. The module is missing, and `planMonthExchange` is not exported.

- [ ] **Step 3: Implement**

In `planMath.ts`, add after `makePlanExchange` (`Id` is already imported there):

```ts
export function planMonthExchange(plan: BudgetPlanDto, currencies: CurrencyDto[], monthIndex: number): (from: Id, to: Id, amount: string) => string {
  const rates = (plan.currencyRates[monthIndex]?.rates ?? []).map((r) => ({ ...r, updatedAt: r.periodStart }))
  return (from, to, amount) => exchange(from, to, amount, rates, currencies)
}
```

Create `phoneMonth.ts`:

```ts
import type { BudgetElementDto, BudgetPlanDto, BudgetSavingsElementDto, PlanElementDto } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'
import { isZero } from '@/lib/decimal'
import {
  balanceRow,
  bucketPlanRows,
  everydayBalanceRow,
  makePlanExchange,
  planHasSavingsData,
  planTotals,
  savingsBalanceRow,
} from './planMath'

export interface PlanCellFigures {
  element: PlanElementDto
  planned: string
  actual: string
  closingBalance?: string
}

export interface PlanMonthFigures {
  month: string
  index: number
  income: { rows: PlanCellFigures[]; planned: string; received: string }
  balance: string
  savingsBalance: string | null
  transfersNet: string
}

export function planCellFigures(element: PlanElementDto, index: number): PlanCellFigures {
  const cell = element.cells[index]
  return {
    element,
    planned: cell && cell.planned !== '' ? cell.planned : '0',
    actual: cell?.actual ?? '0',
    ...(cell?.closingBalance !== undefined ? { closingBalance: cell.closingBalance } : {}),
  }
}

/** One month of the Plan view's figures for the phone view: the same functions, so
 *  its income, Balance and Total savings always agree with the Plan view's lines. */
export function planMonthFigures(plan: BudgetPlanDto, currencies: CurrencyDto[], month: string, now?: Date): PlanMonthFigures | null {
  const index = plan.months.indexOf(month)
  if (index === -1) {
    return null
  }
  const ex = makePlanExchange(plan, currencies)
  const totals = planTotals(plan, ex, now)
  const combined = balanceRow(plan, totals, ex, now)
  const savings = planHasSavingsData(plan) ? savingsBalanceRow(plan, totals, ex, now) : null
  const balance = savings ? everydayBalanceRow(combined, savings) : combined

  const income = bucketPlanRows(plan, false).income
  const rows = [...income.folders.flatMap((f) => f.rows), ...income.loose].map((r) => planCellFigures(r.element, index))
  const uncategorized = income.uncategorized?.element
  if (uncategorized && !isZero(uncategorized.cells[index]?.actual ?? '0')) {
    rows.push(planCellFigures(uncategorized, index))
  }

  return {
    month,
    index,
    income: { rows, planned: totals[index].incomePlanned, received: totals[index].incomeActual },
    balance: balance[index],
    savingsBalance: savings ? savings[index] : null,
    transfersNet: totals[index].transfersNet,
  }
}

export type SheetTarget =
  | { kind: 'expense'; element: BudgetElementDto }
  | { kind: 'savings'; row: BudgetSavingsElementDto }
  | { kind: 'plan'; cell: PlanCellFigures }

export interface SheetCell {
  id: Id
  /** the wire name: render it through elementDisplayName */
  name: string
  currencyId: Id
  /** the amount Set budget / Set plan starts from */
  amount: string
}

export function sheetCell(target: SheetTarget, baseCurrencyId: Id): SheetCell {
  switch (target.kind) {
    case 'expense':
      return { id: target.element.id, name: target.element.name, currencyId: target.element.currencyId ?? baseCurrencyId, amount: target.element.budgeted }
    case 'savings':
      return { id: target.row.id, name: target.row.name, currencyId: target.row.currencyId, amount: target.row.budgeted }
    case 'plan':
      return { id: target.cell.element.id, name: target.cell.element.name, currencyId: target.cell.element.currencyId, amount: target.cell.planned }
  }
}
```

- [ ] **Step 4: Run the tests and check that they pass**

Run: `pnpm vitest run src/features/budgets/phoneMonth.test.ts src/features/budgets/planMath.test.ts --maxWorkers=2`, then `pnpm exec tsc -b`.
Expected: PASS; tsc clean.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/budgets/phoneMonth.ts web/src/features/budgets/phoneMonth.test.ts web/src/features/budgets/planMath.ts web/src/features/budgets/planMath.test.ts
git commit -m "feat: Plan month figures and the item sheet's target

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Catalogue keys for the phone view and item sheet

**Files:**
- Modify: `locales/de.json`, `locales/en.json`, `locales/es.json`, `locales/fr.json`, `locales/it.json`, `locales/nl.json`, `locales/pl.json`, `locales/pt.json`, `locales/ru.json`, `locales/uk.json`, `locales/zh.json`

**Interfaces:**
- Produces these keys, used by Tasks 4–8. The `en` values are verbatim:

| Key | en |
|---|---|
| `budgets.page.phone.income_summary` | `Income · {received} of {planned}` |
| `budgets.page.phone.of` | `{value} of {total}` |
| `budgets.page.phone.row_aria` | `{name}, budget {budget}, spent {spent}` |
| `budgets.page.phone.income_row_aria` | `{name}, planned {planned}, received {received}` |
| `budgets.page.phone.savings_row_aria` | `{name}, planned {planned}, saved {saved}` |
| `budgets.page.phone.balance` | `Balance at month end` |
| `budgets.page.sheet.received` | `Received` |
| `budgets.page.sheet.covered` | `Over by {over} — covered by {carry} left from earlier months` |
| `budgets.page.sheet.overspent` | `Overspent by {amount}` |
| `budgets.page.sheet.transactions` | `Transactions` |
| `budgets.page.sheet.set_plan` | `Set plan` |
| `budgets.page.sheet.converted` | `≈ {amount} {currency}` |

- Existing keys the later tasks reuse; do not add these again:
  - `budgets.page.budget.structure.tab.{budgeted,spent,available}`
  - `budgets.page.savings.{title,planned,saved}`
  - `budgets.page.plan.totals.{expenses,savings,savings_balance,transfers}`
  - `budgets.page.budget.structure.total.name`
  - `budgets.page.budget.structure.in_archive`
  - `budgets.page.budget.structure.labels.heading`
  - `budgets.page.plan.menu.no_folder`
  - `budgets.page.plan.comments.{disclosure,add}`
  - `budgets.modal.set_limit_form.header`
  - `budgets.modal.expense_widget.conversion_rate`
  - `common.button.{expand,collapse}.label`

- [ ] **Step 1: Add the keys by hand**

In each catalogue, the `budgets.page` object gets two new sibling objects, `"phone"` and `"sheet"`. Place them right after the existing `"savings"` object and follow the file's existing indentation (2 spaces) and key quoting. `en.json` takes the values from the table. The ten other languages take natural translations. Keep every `{placeholder}` spelled exactly as in `en`, and keep `·`, `—` and `≈` as literal characters.

Edit with the Edit tool (an exact-string insertion after the closing brace of `"savings": { … }`), never with a script that re-serializes the file. Confirm with `git diff --stat locales/`: each file should show only added lines (`+14` or so), with no deletions.

- [ ] **Step 2: Verify parity and placeholders**

Run from the repo root: `GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/test/i18ntest/`
Expected: PASS. It checks key parity and placeholder-set parity across the 11 languages. The frontend `t()` coverage check passes trivially, because nothing uses the keys yet.

- [ ] **Step 3: Commit**

```bash
git add locales/
git commit -m "feat: Catalogue keys for the phone month view and item sheet

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: The item sheet (`ElementSheet`)

**Files:**
- Create: `web/src/features/budgets/ElementSheet.tsx`
- Test: `web/src/features/budgets/ElementSheet.test.tsx`

**Interfaces:**
- Consumes:
  - Task 1: `rowState`, `carryOver` (plus the existing `displayAvailable`, `elementDisplayName`).
  - Task 2: `SheetTarget`, `sheetCell`.
  - Task 3 keys.
  - `sortByCreatedAt` from `./CommentThread`.
  - `currentMonth`, `formatPlanMonth` from `./planMath`.
  - `isIncomeType`, `BudgetElementType`, `UNCATEGORIZED_ID` from `@/api/dto/budget`.
- Produces:

```ts
export interface ElementSheetProps {
  target: SheetTarget | null
  /** the month the sheet describes, 'YYYY-MM-01' (a Plan grid cell's column month; else the selected month) */
  month: string
  baseCurrencyId: Id
  currencies: CurrencyDto[]
  /** converts at `month`'s rates */
  exchange: (from: Id, to: Id, amount: string) => string
  comments: BudgetCommentDto[]   // the target cell's thread for `month`
  commentsReadOnly: boolean
  canSetAmount: boolean
  onClose: () => void
  onSetAmount: () => void
  onOpenComments: () => void
  /** absent: no transaction list for this target */
  onShowTransactions?: () => void
}
export function ElementSheet(props: ElementSheetProps): JSX.Element | null
```

- Figures by target:

| Target | Figures | State sentence | Set label |
|---|---|---|---|
| `expense` | Budget, Spent, Available (Uncategorized: Spent only) | covered / over per `rowState` | Set budget |
| `savings` | Planned, Saved, Balance at month end (`closingBalance`, `—` when absent) | none | Set budget |
| `plan` with an income type | Planned, Received | none | Set plan |
| `plan` with type `SAVINGS` | Planned, Saved, Balance at month end | none | Set budget |
| `plan` of any other type | Budget (= planned), Spent (= actual) | none (the plan has no Available) | Set budget |

  Actual figures (Spent/Received/Saved) read `—` in a month after the current one.
- Test ids:
  - `element-sheet` (the sheet body)
  - `sheet-figure-{budget|spent|available|planned|received|saved|balance}`
  - `sheet-state`
  - `sheet-converted`
  - `sheet-rate`
  - `sheet-latest-comment`

- [ ] **Step 1: Write the failing tests**

Create `ElementSheet.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { coerceBudgetFixture } from '@/test/coerceBudget'
import { fixtureWireBudget } from '@/test/fixtures'
import type { BudgetCommentDto, BudgetElementDto, PlanElementDto } from '@/api/dto/budget'
import { makeBudgetExchange } from './budgetMath'
import { ElementSheet } from './ElementSheet'
import type { ElementSheetProps } from './ElementSheet'
import { formatPlanMonth } from './planMath'

const usd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const eur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 }
const budget = coerceBudgetFixture(fixtureWireBudget)
const food = budget.structure.elements.find((el) => el.id === 'cat-food')!
const living = budget.structure.elements.find((el) => el.id === 'env-1')!
// the one month-label rule: "July" in the current year, "Jul 2026" in any other
const july = formatPlanMonth('2026-07-01', 'en')

const comment = (id: string, text: string, createdAt: string): BudgetCommentDto => ({
  id, elementId: 'cat-food', period: '2026-07-01', comment: text,
  author: { id: 'u1', avatar: 'face:emerald', name: 'Ada' }, createdAt, updatedAt: createdAt,
})
const planElement = (over: Partial<PlanElementDto>): PlanElementDto =>
  ({ id: 'x', type: 1, name: 'X', icon: 'tag', currencyId: 'cur-usd', isArchived: 0, folderId: null, position: 0, ownerUserId: null, cells: [], children: [], ...over }) as PlanElementDto

function renderSheet(overrides: Partial<ElementSheetProps> = {}) {
  const props: ElementSheetProps = {
    target: { kind: 'expense', element: food },
    month: '2026-07-01',
    baseCurrencyId: 'cur-usd',
    currencies: [usd, eur],
    exchange: makeBudgetExchange(budget, [usd, eur]),
    comments: [],
    commentsReadOnly: false,
    canSetAmount: true,
    onClose: vi.fn(),
    onSetAmount: vi.fn(),
    onOpenComments: vi.fn(),
    onShowTransactions: vi.fn(),
    ...overrides,
  }
  render(<ElementSheet {...props} />)
  return props
}

beforeEach(() => {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: true, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it('titles the sheet with the item and month and shows Budget, Spent and Available', () => {
  renderSheet()
  expect(screen.getByText(`Food · ${july}`)).toBeInTheDocument()
  expect(screen.getByTestId('sheet-figure-budget')).toHaveTextContent('Budget200.00')
  expect(screen.getByTestId('sheet-figure-spent')).toHaveTextContent('Spent45.50')
  expect(screen.getByTestId('sheet-figure-available')).toHaveTextContent('Available354.50')
  expect(screen.queryByTestId('sheet-state')).toBeNull()
  expect(screen.queryByTestId('sheet-rate')).toBeNull()
})

it('explains a month over budget that carry-over still covers', () => {
  const over: BudgetElementDto = { ...food, budgeted: '700', spent: '801.37', budgetSpent: '801.37', available: '-50.68' }
  renderSheet({ target: { kind: 'expense', element: over } })
  expect(screen.getByTestId('sheet-state')).toHaveTextContent('Over by 101.37 — covered by 750.69 left from earlier months')
})

it('names the overspend when Available is negative', () => {
  const over: BudgetElementDto = { ...food, budgeted: '100', spent: '150', budgetSpent: '150', available: '-120' }
  renderSheet({ target: { kind: 'expense', element: over } })
  expect(screen.getByTestId('sheet-state')).toHaveTextContent('Overspent by 20.00')
})

it('shows a dash for Spent and no state sentence in a future month', () => {
  const over: BudgetElementDto = { ...food, budgeted: '100', spent: '150', budgetSpent: '150', available: '-120' }
  renderSheet({ target: { kind: 'expense', element: over }, month: '2099-01-01' })
  expect(screen.getByTestId('sheet-figure-spent')).toHaveTextContent('Spent—')
  expect(screen.queryByTestId('sheet-state')).toBeNull()
})

it('tags every amount of a foreign-currency item and adds the converted line and rate note', () => {
  const spending: BudgetElementDto = { ...living, spent: '10', budgetSpent: '11.11' }
  renderSheet({ target: { kind: 'expense', element: spending } })
  expect(screen.getByTestId('sheet-figure-budget')).toHaveTextContent('90.00 EUR')
  expect(screen.getByTestId('sheet-figure-spent')).toHaveTextContent('10.00 EUR')
  expect(screen.getByTestId('sheet-converted')).toHaveTextContent('≈ 11.11 USD')
  expect(screen.getByTestId('sheet-rate')).toHaveTextContent(new RegExp(`^Average rate for ${july}: 1 USD = [\\d.,]+ EUR$`))
})

it('takes the rate note from the exchange it is given (a Plan column month)', () => {
  const eurRow = planElement({ id: 'env-eur', type: 0, name: 'Euro Stash', currencyId: 'cur-eur' })
  renderSheet({
    target: { kind: 'plan', cell: { element: eurRow, planned: '100', actual: '35' } },
    month: '2026-05-01',
    exchange: (from, to, amount) => (from === 'cur-usd' && to === 'cur-eur' ? '0.5' : amount),
  })
  expect(screen.getByTestId('sheet-rate')).toHaveTextContent(`Average rate for ${formatPlanMonth('2026-05-01', 'en')}: 1 USD = 0.5 EUR`)
})

it('previews the latest comment and links to the thread', async () => {
  const props = renderSheet({
    comments: [comment('c2', 'Back to 700 next month', '2026-07-20 09:00:00'), comment('c1', 'First', '2026-07-01 09:00:00')],
  })
  expect(screen.getByTestId('sheet-latest-comment')).toHaveTextContent('Ada')
  expect(screen.getByTestId('sheet-latest-comment')).toHaveTextContent('Back to 700 next month')
  await userEvent.click(screen.getByRole('button', { name: 'Comments (2)' }))
  expect(props.onOpenComments).toHaveBeenCalled()
})

it('offers "Add comment" on an empty writable thread', () => {
  renderSheet()
  expect(screen.getByRole('button', { name: 'Add comment' })).toBeInTheDocument()
})

it('hides the comments link when the thread is read-only and empty', () => {
  renderSheet({ commentsReadOnly: true })
  expect(screen.queryByRole('button', { name: 'Add comment' })).toBeNull()
})

it('routes Set budget and Transactions', async () => {
  const props = renderSheet()
  await userEvent.click(screen.getByRole('button', { name: 'Set budget' }))
  expect(props.onSetAmount).toHaveBeenCalled()
  await userEvent.click(screen.getByRole('button', { name: 'Transactions' }))
  expect(props.onShowTransactions).toHaveBeenCalled()
})

it('has no Set budget when the amount cannot be set here', () => {
  renderSheet({ canSetAmount: false })
  expect(screen.queryByRole('button', { name: 'Set budget' })).toBeNull()
  expect(screen.getByRole('button', { name: 'Transactions' })).toBeInTheDocument()
})

it('shows the uncategorized row as Spent only, with no comments link', () => {
  const uncategorized: BudgetElementDto = { ...food, id: 'uncategorized', name: 'Uncategorized', budgeted: '0', available: '0', spent: '12', budgetSpent: '12' }
  renderSheet({ target: { kind: 'expense', element: uncategorized }, canSetAmount: false })
  expect(screen.queryByTestId('sheet-figure-budget')).toBeNull()
  expect(screen.getByTestId('sheet-figure-spent')).toHaveTextContent('12.00')
  expect(screen.queryByRole('button', { name: /comment/i })).toBeNull()
})

it('an income plan cell shows Planned and Received and offers Set plan', () => {
  const salaries = planElement({ id: 'ie1', type: 4, name: 'Salaries' })
  renderSheet({ target: { kind: 'plan', cell: { element: salaries, planned: '2000', actual: '400' } }, onShowTransactions: undefined })
  expect(screen.getByTestId('sheet-figure-planned')).toHaveTextContent('Planned2,000.00')
  expect(screen.getByTestId('sheet-figure-received')).toHaveTextContent('Received400.00')
  expect(screen.getByRole('button', { name: 'Set plan' })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Transactions' })).toBeNull()
})

it('an expense plan cell shows Budget and Spent, without Available or a state sentence', () => {
  const pe1 = planElement({ id: 'pe1', type: 0, name: 'Living' })
  renderSheet({ target: { kind: 'plan', cell: { element: pe1, planned: '100', actual: '150' } } })
  expect(screen.getByTestId('sheet-figure-budget')).toHaveTextContent('100.00')
  expect(screen.getByTestId('sheet-figure-spent')).toHaveTextContent('150.00')
  expect(screen.queryByTestId('sheet-figure-available')).toBeNull()
  expect(screen.queryByTestId('sheet-state')).toBeNull()
  expect(screen.getByRole('button', { name: 'Set budget' })).toBeInTheDocument()
})

it('a savings row (monthly or plan cell) shows Planned, Saved and the month-end Balance', () => {
  const saving = { id: 'acc-s1', type: 5 as const, name: 'Rainy day', icon: 'savings', currencyId: 'cur-usd', ownerUserId: 'u1', isArchived: 0 as const, position: 0, budgeted: '100', spent: '40', available: '60', closingBalance: '1040' }
  renderSheet({ target: { kind: 'savings', row: saving }, onShowTransactions: undefined })
  expect(screen.getByTestId('sheet-figure-planned')).toHaveTextContent('100.00')
  expect(screen.getByTestId('sheet-figure-saved')).toHaveTextContent('40.00')
  expect(screen.getByTestId('sheet-figure-balance')).toHaveTextContent('Balance at month end1,040.00')
})

it('a savings plan cell shows its closing balance too', () => {
  const s = planElement({ id: 'acc-s1', type: 5, name: 'Rainy day' })
  renderSheet({ target: { kind: 'plan', cell: { element: s, planned: '100', actual: '40', closingBalance: '1040' } }, onShowTransactions: undefined })
  expect(screen.getByTestId('sheet-figure-saved')).toHaveTextContent('40.00')
  expect(screen.getByTestId('sheet-figure-balance')).toHaveTextContent('1,040.00')
})

it('renders nothing without a target', () => {
  renderSheet({ target: null })
  expect(screen.queryByTestId('element-sheet')).toBeNull()
})
```

- [ ] **Step 2: Run the tests and check that they fail**

Run: `pnpm vitest run src/features/budgets/ElementSheet.test.tsx --maxWorkers=2`
Expected: FAIL, because the module does not exist.

- [ ] **Step 3: Implement**

Create `ElementSheet.tsx`:

```tsx
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { moneyFormat } from '@/lib/money'
import { abs, cmp, sub } from '@/lib/decimal'
import type { BudgetCommentDto } from '@/api/dto/budget'
import { BudgetElementType, isIncomeType, UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'
import { carryOver, displayAvailable, elementDisplayName, rowState } from './budgetMath'
import { sortByCreatedAt } from './CommentThread'
import type { SheetTarget } from './phoneMonth'
import { sheetCell } from './phoneMonth'
import { currentMonth, formatPlanMonth } from './planMath'

const EMPTY = '—'

export interface ElementSheetProps {
  target: SheetTarget | null
  /** the month the sheet describes, 'YYYY-MM-01' */
  month: string
  baseCurrencyId: Id
  currencies: CurrencyDto[]
  /** converts at `month`'s rates */
  exchange: (from: Id, to: Id, amount: string) => string
  comments: BudgetCommentDto[]
  commentsReadOnly: boolean
  canSetAmount: boolean
  onClose: () => void
  onSetAmount: () => void
  onOpenComments: () => void
  /** absent: no transaction list for this target */
  onShowTransactions?: () => void
}

interface Figure {
  key: string
  label: string
  value: string
  negative?: boolean
}

export function ElementSheet({
  target,
  month,
  baseCurrencyId,
  currencies,
  exchange,
  comments,
  commentsReadOnly,
  canSetAmount,
  onClose,
  onSetAmount,
  onOpenComments,
  onShowTransactions,
}: ElementSheetProps) {
  const { t, i18n } = useTranslation()
  if (!target) {
    return null
  }
  const cell = sheetCell(target, baseCurrencyId)
  const name = elementDisplayName(cell.id, cell.name, t)
  const monthLabel = formatPlanMonth(month, i18n.language)
  const currency = currencies.find((c) => c.id === cell.currencyId)
  const baseCurrency = currencies.find((c) => c.id === baseCurrencyId)
  const foreign = cell.currencyId !== baseCurrencyId
  const future = month > currentMonth()
  const isUncategorized = cell.id === UNCATEGORIZED_ID
  const fmtIn = (amount: string, c: CurrencyDto | undefined) =>
    moneyFormat(amount, c, { showCurrency: false, useNativePrecision: false, maxPrecision: c?.fractionDigits ?? 2 })
  // the sheet repeats a foreign item's code beside every amount (the row only tags its name)
  const fmt = (amount: string) => `${fmtIn(amount, currency)}${foreign && currency ? ` ${currency.code}` : ''}`
  const actual = (amount: string) => (future ? EMPTY : fmt(amount))
  const label = {
    budget: t('budgets.page.budget.structure.tab.budgeted'),
    spent: t('budgets.page.budget.structure.tab.spent'),
    available: t('budgets.page.budget.structure.tab.available'),
    planned: t('budgets.page.savings.planned'),
    received: t('budgets.page.sheet.received'),
    saved: t('budgets.page.savings.saved'),
    balance: t('budgets.page.phone.balance'),
  }

  const figures: Figure[] = []
  let stateSentence: string | null = null
  let actualInBase: string
  let income = false
  if (target.kind === 'expense') {
    const el = target.element
    const available = displayAvailable(el)
    if (!isUncategorized) {
      figures.push({ key: 'budget', label: label.budget, value: fmt(el.budgeted) })
    }
    figures.push({ key: 'spent', label: label.spent, value: actual(el.spent) })
    if (!isUncategorized) {
      figures.push({ key: 'available', label: label.available, value: fmt(available), negative: cmp(available, '0') < 0 })
      const state = rowState({ budgeted: el.budgeted, spent: el.spent, available }, future)
      if (state === 'covered') {
        stateSentence = t('budgets.page.sheet.covered', { over: fmt(sub(el.spent, el.budgeted)), carry: fmt(carryOver(el)) })
      } else if (state === 'over') {
        stateSentence = t('budgets.page.sheet.overspent', { amount: fmt(abs(available)) })
      }
    }
    actualInBase = el.budgetSpent
  } else {
    const planned = target.kind === 'savings' ? target.row.budgeted : target.cell.planned
    const done = target.kind === 'savings' ? target.row.spent : target.cell.actual
    const closing = target.kind === 'savings' ? target.row.closingBalance : target.cell.closingBalance
    const type = target.kind === 'savings' ? BudgetElementType.SAVINGS : target.cell.element.type
    income = isIncomeType(type)
    if (type === BudgetElementType.SAVINGS) {
      figures.push({ key: 'planned', label: label.planned, value: fmt(planned) })
      figures.push({ key: 'saved', label: label.saved, value: actual(done) })
      figures.push({ key: 'balance', label: label.balance, value: closing !== undefined ? fmt(closing) : EMPTY })
    } else if (income) {
      figures.push({ key: 'planned', label: label.planned, value: fmt(planned) })
      figures.push({ key: 'received', label: label.received, value: actual(done) })
    } else {
      figures.push({ key: 'budget', label: label.budget, value: fmt(planned) })
      figures.push({ key: 'spent', label: label.spent, value: actual(done) })
    }
    actualInBase = foreign ? exchange(cell.currencyId, baseCurrencyId, done) : done
  }

  const latest = sortByCreatedAt(comments).at(-1)
  const showCommentsLink = !isUncategorized && !(commentsReadOnly && comments.length === 0)
  const rate = foreign ? exchange(baseCurrencyId, cell.currencyId, '1') : null

  return (
    <ResponsiveDialog open onOpenChange={(o) => !o && onClose()} title={`${name} · ${monthLabel}`}>
      <div className="flex flex-col gap-4" data-testid="element-sheet">
        <div className={`grid gap-2 ${figures.length === 1 ? 'grid-cols-1' : figures.length === 2 ? 'grid-cols-2' : 'grid-cols-3'}`}>
          {figures.map((f) => (
            <div key={f.key} className="flex flex-col" data-testid={`sheet-figure-${f.key}`}>
              <span className="text-[13px] text-muted-foreground">{f.label}</span>
              <span className={`text-[17px] font-medium tabular-nums ${f.negative ? 'text-expense' : ''}`}>{f.value}</span>
            </div>
          ))}
        </div>
        {stateSentence ? (
          <p className="text-sm" data-testid="sheet-state">
            {stateSentence}
          </p>
        ) : null}
        {foreign && baseCurrency ? (
          <div className="flex flex-col gap-0.5 text-xs text-muted-foreground">
            {!future ? (
              <span data-testid="sheet-converted">
                {t('budgets.page.sheet.converted', { amount: fmtIn(actualInBase, baseCurrency), currency: baseCurrency.code })}
              </span>
            ) : null}
            {rate !== null && currency ? (
              <span data-testid="sheet-rate">
                {t('budgets.modal.expense_widget.conversion_rate', {
                  period: monthLabel,
                  defaultCurrency: baseCurrency.code,
                  rate: `${moneyFormat(rate, null, { showCurrency: false, useNativePrecision: false, maxPrecision: 4 })} ${currency.code}`,
                })}
              </span>
            ) : null}
          </div>
        ) : null}
        {showCommentsLink ? (
          <div className="flex items-center gap-2 border-t pt-3">
            {latest ? (
              <p className="min-w-0 flex-1 truncate text-sm" data-testid="sheet-latest-comment">
                <span className="font-medium">{latest.author.name}</span> <span className="text-muted-foreground">{latest.comment}</span>
              </p>
            ) : (
              <span className="flex-1" />
            )}
            <button type="button" className="shrink-0 text-sm font-medium text-primary hover:underline" onClick={onOpenComments}>
              {comments.length > 0 ? t('budgets.page.plan.comments.disclosure', { count: comments.length }) : t('budgets.page.plan.comments.add')}
            </button>
          </div>
        ) : null}
        {canSetAmount || onShowTransactions ? (
          <div className="flex gap-3 [&>button]:h-11 [&>button]:flex-1">
            {canSetAmount ? (
              <Button type="button" onClick={onSetAmount}>
                {income ? t('budgets.page.sheet.set_plan') : t('budgets.modal.set_limit_form.header')}
              </Button>
            ) : null}
            {onShowTransactions ? (
              <Button type="button" variant="secondary" onClick={onShowTransactions}>
                {t('budgets.page.sheet.transactions')}
              </Button>
            ) : null}
          </div>
        ) : null}
      </div>
    </ResponsiveDialog>
  )
}
```

`useTranslation` is called unconditionally before the early `return null`, so the rules of hooks hold. On a tablet (`useIsPhone()` false), `ResponsiveDialog` renders a centred dialog rather than a bottom sheet. That is intended: the same content in the platform's dialog shell.

- [ ] **Step 4: Run the tests and check that they pass**

Run: `pnpm vitest run src/features/budgets/ElementSheet.test.tsx --maxWorkers=2`, then `pnpm exec tsc -b && pnpm lint`.
Expected: PASS; tsc clean; lint 0 errors. If the vaul Drawer mounts its content a frame late in jsdom, switch the first query of each test to `findBy…`; `CommentsPanel.test.tsx` renders the phone sheet the same way.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/budgets/ElementSheet.tsx web/src/features/budgets/ElementSheet.test.tsx
git commit -m "feat: Item sheet for budget rows and plan cells

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: Tablet Budget view: a tap opens the item sheet; long-press and the stage-1 phone paths go

**Files:**
- Modify:
  - `web/src/features/budgets/CellShell.tsx` (drop the touch half)
  - `web/src/features/budgets/BudgetTable.tsx`
  - `web/src/features/budgets/SavingsBlock.tsx`
  - `web/src/features/budgets/BudgetPage.tsx`
  - `web/src/features/budgets/SetLimitDialog.tsx`
  - `web/src/features/budgets/PlanSheet.tsx`: only the `CellShell` props, `isPhone`, and its `SetLimitDialog` comments props. The Plan grid's tap-to-sheet is Task 6.
- Delete: `web/src/hooks/useLongPress.ts` (only `BudgetPage.tsx` imports it)
- Tests:
  - `CellShell.test.tsx`
  - `BudgetTable.test.tsx`
  - `SavingsBlock.test.tsx`
  - `BudgetPage.test.tsx`
  - `comments.monthly.test.tsx`
  - `comments.plan.test.tsx`
  - `comments.plan.guest.test.tsx`

  All are under `web/src/features/budgets/`.
- Docs: `docs/regression-test-plan.md`

**Interfaces:**
- `CellShell` props become exactly:

```ts
interface CellShellProps {
  children: ReactElement
  comments: BudgetCommentDto[]
  /** no hover preview while a thread is open (touch viewports never preview) */
  previewDisabled?: boolean
  /** no Shift+F2: edit-structure mode */
  shortcutDisabled?: boolean
  /** omitted for the uncategorized row */
  onOpenComments?: (anchor: HTMLElement) => void
}
```

  `title`, `onSetBudget`, `onShowTransactions` and `actionsDisabled` go.
- `ElementRowExtras` (BudgetTable):
  - Adds `onBudgetCellDetails?: (element: BudgetElementDto) => void`, documented as "touch viewports: the budgeted amount opens the item sheet; wins over `renderBudgetCell` and `onBudgetCellComments`".
  - Loses `onAvailableClick` and `onAvailableCommentsClick`.
  - `wrapBudgetCell` becomes `(element, cell) => ReactNode`, dropping the `{ readOnly }` options that only fed `onSetBudget`.
- `SavingsBlock` props:
  - `onEditPlanned` is replaced by `onOpenDetails?: (row: BudgetSavingsElementDto) => void`, documented as "touch viewports: the planned amount opens the item sheet".
  - `wrapPlannedCell` keeps its signature.
- `SetLimitDialog` props become `{ target, onClose, onCommit, title?: string }`. `commentCount` and `onOpenComments` go, and `title` defaults to `t('budgets.modal.set_limit_form.header')`.
- `BudgetPage` gains `sheetTarget: SheetTarget | null` state and hosts `<ElementSheet>` (expense and savings kinds here; Task 8 adds the `plan` kind for phone income).

- [ ] **Step 1: Rewrite the tests for the new behaviour**

**`CellShell.test.tsx`**
- Delete these tests:
  - `on a tablet, a long-press opens the actions modal and its release does not also tap the cell`
  - `on a tablet, a quick tap still taps the cell, and there is no hover preview or native context menu`
- Add:

```tsx
it('on a tablet, a press held past half a second opens nothing and the tap still reaches the cell', async () => {
  tablet()
  vi.useFakeTimers({ shouldAdvanceTime: true })
  try {
    const onClick = vi.fn()
    render(
      <CellShell comments={[comment]} onOpenComments={vi.fn()}>
        <button type="button" onClick={onClick}>cell</button>
      </CellShell>,
    )
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    const cell = screen.getByRole('button', { name: 'cell' })
    await user.pointer({ keys: '[TouchA>]', target: cell })
    await vi.advanceTimersByTimeAsync(800)
    await user.pointer({ keys: '[/TouchA]', target: cell })
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(screen.queryByTestId('comment-preview')).toBeNull()
    expect(onClick).toHaveBeenCalledTimes(1)
  } finally {
    vi.useRealTimers()
  }
})
```

  Use the file's existing `comment` fixture and `tablet()` helper names. If they differ, adapt; `tablet()` is at line ~27.
- In every remaining test:
  - drop the `title`, `onSetBudget` and `onShowTransactions` props;
  - rename `actionsDisabled` to `shortcutDisabled`;
  - rename the Shift+F2 test `does nothing when the cell actions are disabled (phones, edit mode)` to `does nothing when the shortcut is disabled (edit mode)`.
- The right-click test stays: a mouse right-click still gets the browser's own menu.

**`BudgetTable.test.tsx`**
- Delete every test that passes `onAvailableClick` or `onAvailableCommentsClick`.
- Where a test passes `wrapBudgetCell` with three parameters, drop the third.
- Add:

```tsx
it('onBudgetCellDetails turns the budgeted amount into the item-sheet button, Archive rows included', async () => {
  const onDetails = vi.fn()
  renderTable((b) => {
    const old = b.structure.elements.find((el) => el.id === 'tag-old')!
    Object.assign(old, { budgeted: '50', available: '50' })
  }, { onBudgetCellDetails: onDetails, renderBudgetCell: () => 'editor', onBudgetCellComments: vi.fn() })
  await userEvent.click(within(screen.getByTestId('element-cat-food')).getByRole('button', { name: 'details Food' }))
  expect(onDetails).toHaveBeenCalledWith(expect.objectContaining({ id: 'cat-food' }))
  await userEvent.click(within(screen.getByTestId('element-tag-old')).getByRole('button', { name: 'details zzz-archived' }))
  expect(onDetails).toHaveBeenCalledWith(expect.objectContaining({ id: 'tag-old' }))
  expect(screen.queryByText('editor')).toBeNull()
})
```

**`SavingsBlock.test.tsx`**
- Replace every `onEditPlanned={…}` prop with `onOpenDetails={…}`, or drop it where the test is about desktop.
- Where a test tapped the planned amount on a compact viewport and expected `onEditPlanned`, assert `onOpenDetails` receives the row and that the button is named `details {row name}`.

**`BudgetPage.test.tsx`**
- Replace `it('compact: Planned opens the set-limit dialog with a button to the cell thread', …)` with:

```ts
  it('tablet: tapping Planned opens the item sheet; its Set budget saves the plan', async () => {
    window.matchMedia = vi.fn().mockImplementation((q: string) => ({
      matches: q.includes('1023'), media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
    }))
    let body: unknown
    useSavingsHandlers([hangingSetLimit((b) => (body = b))])
    const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
    renderPage()
    const row = await screen.findByTestId('savings-row-acc-s1')
    await user.click(within(row).getByRole('button', { name: 'details Rainy day' }))
    const sheet = await screen.findByTestId('element-sheet')
    expect(within(sheet).getByRole('button', { name: 'Comments (1)' })).toBeInTheDocument()
    await user.click(within(sheet).getByRole('button', { name: 'Set budget' }))
    const input = await screen.findByLabelText('Budget')
    expect(screen.queryByTestId('element-sheet')).toBeNull()
    expect(screen.queryByRole('button', { name: /Comments \(/ })).not.toBeInTheDocument()
    await user.clear(input)
    await user.type(input, '250')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(body).toEqual({ budgetId: 'b1', elementId: 'acc-s1', period: '2026-07-01', amount: '250' }))
    await waitFor(() => expect(within(row).getByTestId('savings-planned')).toHaveTextContent('250.00'))
  })
```

  Import `PointerEventsCheckLevel` from `@testing-library/user-event` if the file does not already.

**`comments.monthly.test.tsx`**
- Delete these tests, together with any comment block above them:
  - `opens the thread from SetLimitDialog as its own dialog on compact viewports`
  - `lets a guest reach the thread from the Available cell on compact viewports`
  - `lets a phone user open the thread of an individually-archived element from the Available cell`
  - `offers no add-comment corner on a phone` (Task 8 covers the phone view)
  - `opens the actions modal from a long-press on a tablet and goes on to the thread`
  - `keeps the tablet amount dialog free of comments (the actions modal reaches the thread)`
  - `keeps the tablet amount plain text for an editable cell (no comments button)`
- In `lets a guest add a comment from the corner, and their tablet actions modal has no set budget`, keep the desktop corner half and delete the tablet long-press half. Rename it `lets a guest add a comment from the corner`.
- Add:

```tsx
it('a tablet tap on the budgeted amount opens the item sheet, whose Comments opens the thread', async () => {
  registerMonthlyHandlers()
  mockTabletViewport()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage('/budget')

  const row = await screen.findByTestId('element-cat-food')
  expect(within(row).queryByLabelText(/^comments /)).toBeNull()
  await user.click(within(row).getByRole('button', { name: 'details Food' }))
  const sheet = await screen.findByTestId('element-sheet')
  expect(within(sheet).getByTestId('sheet-latest-comment')).toHaveTextContent('Trip to Lisbon')
  await user.click(within(sheet).getByRole('button', { name: 'Comments (1)' }))
  expect(await screen.findByText('Trip to Lisbon', { selector: 'p' })).toBeInTheDocument()
  expect(screen.queryByTestId('element-sheet')).toBeNull()
})

it('a tablet sheet’s Set budget opens the amount dialog with no comments in it', async () => {
  registerMonthlyHandlers()
  mockTabletViewport()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage('/budget')

  await user.click(within(await screen.findByTestId('element-cat-food')).getByRole('button', { name: 'details Food' }))
  await user.click(within(await screen.findByTestId('element-sheet')).getByRole('button', { name: 'Set budget' }))
  expect(await screen.findByLabelText('Budget')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /Comments \(/ })).toBeNull()
})

it('a guest’s tablet sheet has no Set budget but reaches the thread', async () => {
  mockTabletViewport()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderGuestPage('/budget')

  await user.click(within(await screen.findByTestId('element-cat-food')).getByRole('button', { name: 'details Food' }))
  const sheet = await screen.findByTestId('element-sheet')
  expect(within(sheet).queryByRole('button', { name: 'Set budget' })).toBeNull()
  await user.click(within(sheet).getByRole('button', { name: 'Comments (1)' }))
  expect(await screen.findByRole('button', { name: 'Post' })).toBeInTheDocument()
})

it('a tablet tap on an archived element’s amount opens its sheet without Set budget', async () => {
  const archivedElementBudget = {
    ...fixtureWireBudget,
    structure: {
      ...fixtureWireBudget.structure,
      elements: fixtureWireBudget.structure.elements.map((el) => (el.id === 'tag-old' ? { ...el, budgeted: '50', available: '50' } : el)),
    },
  }
  registerMonthlyHandlers(archivedElementBudget)
  mockTabletViewport()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage('/budget')
  await user.click(within(await screen.findByTestId('element-tag-old')).getByRole('button', { name: 'details zzz-archived' }))
  const sheet = await screen.findByTestId('element-sheet')
  expect(within(sheet).queryByRole('button', { name: 'Set budget' })).toBeNull()
  expect(within(sheet).getByRole('button', { name: 'Add comment' })).toBeInTheDocument()
})

it('the tablet Available pill is plain text', async () => {
  registerMonthlyHandlers()
  mockTabletViewport()
  renderPage('/budget')
  const row = await screen.findByTestId('element-cat-food')
  expect(within(row).getByTestId('cell-available').closest('button')).toBeNull()
})
```

  The tablet marker test `opens the thread as a popover from a marker tap on a tablet` stays. It proves the marker tap still opens the popover; add `expect(screen.queryByTestId('element-sheet')).toBeNull()` to it. The `Trip to Lisbon` selector above assumes the thread renders comment text in a `<p>`; drop the selector if `findByText` finds a single match without it.

**`comments.plan.test.tsx`**
- Delete these tests:
  - `opens the thread in a sheet on a phone`
  - `opens the thread as a sheet, not a popover, from the amount dialog on a phone`
  - `opens the actions modal from a long-press on a plan cell on a tablet`
- Leave `keeps the amount dialog free of comments on a tablet` as it is; Task 6 rewrites it.

**`comments.plan.guest.test.tsx`**
- In `lets a guest add a comment from a plan cell's corner, and their tablet actions modal has no set budget`, keep the desktop corner half and delete the tablet long-press half. Rename it `lets a guest add a comment from a plan cell's corner`.
- Leave the `compact viewports` test for Task 6.

- [ ] **Step 2: Run the changed files and check that the new tests fail**

Run: `pnpm vitest run src/features/budgets/comments.monthly.test.tsx src/features/budgets/BudgetTable.test.tsx src/features/budgets/BudgetPage.test.tsx src/features/budgets/CellShell.test.tsx --maxWorkers=2`
Expected: FAIL. There is no `details …` button and no `element-sheet`, and the CellShell long-press still opens a dialog.

- [ ] **Step 3: `CellShell.tsx`: remove the touch half**

1. Delete:
   - `LONG_PRESS_MS`, `LONG_PRESS_SLOP_PX`, `ActionRun`, `CellAction`, `BuildActions` and `CellActionsModal`;
   - the `actionsModal` state, and the `pending`, `pressTimer`, `pressStart` and `swallowClick` refs;
   - `touchActions`, `buildActions`, `runPending` and `cancelLongPress`.
2. Remove the imports only they used: `Button`, `ResponsiveDialog`, `TFunction`, and `MouseEvent` if unused.
3. Props: apply the interface above. `actionsDisabled` becomes `shortcutDisabled`, used only in the Shift+F2 guard.
4. Handlers kept on the clone:
   - `onPointerEnter`: unchanged.
   - `onPointerLeave`: `pressed.current = false` and the preview close; no long-press cancel.
   - `onPointerDown`: `pressed.current = true; closePreview()`.
   - `onKeyDown`: the Shift+F2 block, guarded by `!shortcutDisabled`.
5. Delete the `onPointerMove`, `onPointerUp`, `onPointerCancel`, `onClickCapture` and `onContextMenu` wrappers. The cell's own handlers pass through untouched, because `cloneElement` keeps any prop not overridden. Trim `CellProps` to the handlers still overridden (`ref`, `onPointerEnter`, `onPointerLeave`, `onPointerDown`, `onKeyDown`).
6. The unmount cleanup keeps only the preview timer.
7. Update the top-of-component comment to the new shape: "the Radix preview mounts as a sibling only while open".

- [ ] **Step 4: `BudgetTable.tsx`**

1. Apply the `ElementRowExtras` changes above.
2. In `ElementRow`'s budgeted cell, pick the content in this order: uncategorized dash, `onBudgetCellDetails` button, `renderBudgetCell`, `onBudgetCellComments` button, plain value. The new branch:

```tsx
              ) : extras.onBudgetCellDetails ? (
                <button
                  type="button"
                  className="w-full text-right underline-offset-2 hover:underline"
                  aria-label={`details ${displayName}`}
                  onClick={() => extras.onBudgetCellDetails!(element)}
                >
                  {moneyFormat(element.budgeted, currency, opts)}
                </button>
```

3. Change the wrap call to `extras.wrapBudgetCell(element, cell)`.
4. The Available cell renders `<AvailablePill …/>` (or the uncategorized dash) with no button branches.
5. In the Archive section's read-only `extras`:
   - add `onBudgetCellDetails: extras.onBudgetCellDetails`;
   - pass `wrapBudgetCell: extras.wrapBudgetCell`;
   - delete the `onAvailableCommentsClick` line and its comment.

- [ ] **Step 5: `SavingsBlock.tsx`**

1. Replace `onEditPlanned` with the optional `onOpenDetails` in `SavingsBlockProps` and `SavingsRow`, and fix the doc comment on `renderPlannedEditor`.
2. The planned cell's content becomes:

```tsx
      {editMode ? (
        planned
      ) : onOpenDetails ? (
        <button type="button" className="w-full text-right underline-offset-2 hover:underline" aria-label={`details ${row.name}`} onClick={() => onOpenDetails(row)}>
          {planned}
        </button>
      ) : editable && renderPlannedEditor ? (
        renderPlannedEditor(row)
      ) : (
        // a cell that cannot be edited (guest, pre-start month, deleted account) still opens its thread
        <button
          type="button"
          className="w-full text-right underline-offset-2 hover:underline"
          aria-label={`comments ${row.name}`}
          onClick={(e) => onOpenComments(row, commentAnchorOf(e.currentTarget))}
        >
          {planned}
        </button>
      )}
```

- [ ] **Step 6: `SetLimitDialog.tsx`**

1. Props become `{ target, onClose, onCommit, title?: string }`.
2. Delete the comments button block.
3. Set `title={title ?? t('budgets.modal.set_limit_form.header')}`.
4. Change the component comment to `// Compact viewports' amount dialog (Vue's BudgetSetLimitModal), same unified amount rule.`

- [ ] **Step 7: `BudgetPage.tsx`**

1. Delete `ElementLongPress` and the `useLongPress` import. Make `renderRowWrapper` edit-mode only:

```tsx
                    renderRowWrapper={
                      editMode
                        ? (element, _bucket, row) => (
                            <DraggableElement key={element.id} id={element.id}>
                              {row}
                            </DraggableElement>
                          )
                        : undefined
                    }
```

2. Delete the `onAvailableClick` and `onAvailableCommentsClick` props on `<BudgetTable>`, with their comments.
3. Add the imports `import { ElementSheet } from './ElementSheet'` and `import { sheetCell, type SheetTarget } from './phoneMonth'`. Add the state `const [sheetTarget, setSheetTarget] = useState<SheetTarget | null>(null)` next to `transactionsTarget`.
4. Touch viewports open the sheet from the amount; desktop keeps the inline editor and the comments button:

```tsx
                    onBudgetCellDetails={isCompact && !editMode ? (element) => setSheetTarget({ kind: 'expense', element }) : undefined}
                    onBudgetCellComments={!editMode && !isCompact ? (element, anchor) => openComments(element, anchor) : undefined}
```

   Replace the old comment block above `onBudgetCellComments` with one line: `// touch viewports reach set budget, comments and transactions through the item sheet`.
5. `<SavingsBlock>`: replace `onEditPlanned={setLimitTarget}` with `onOpenDetails={isCompact && !editMode ? (row) => setSheetTarget({ kind: 'savings', row }) : undefined}`.
6. `wrapBudgetCell` and `wrapPlannedCell` build `CellShell` with only `comments`, `previewDisabled={commentsTarget !== null || editMode}`, `shortcutDisabled={editMode}` and `onOpenComments`. `wrapBudgetCell`'s signature drops the `{ readOnly }` parameter. Delete `setBudgetFor` and `openLimitEditorIn` if nothing else uses them. `cellActionsDisabled` becomes `const cellActionsDisabled = isPhone || editMode`, used only by `canAddComment`.
7. `<SetLimitDialog>`: delete `commentCount`, `onOpenComments` and their comment line.
8. Sheet helpers, after `transactionsTargetOf`:

```tsx
  const sheetCanSetAmount = (target: SheetTarget): boolean => {
    if (!limitsEditable) {
      return false
    }
    switch (target.kind) {
      case 'expense':
        return target.element.isArchived === 0 && target.element.id !== UNCATEGORIZED_ID
      case 'savings':
        return target.row.isArchived === 0
      case 'plan':
        // the monthly page has no plan column to write to; the phone view wires this (stage 2, Task 8)
        return false
    }
  }
  const sheetCellTarget = (target: SheetTarget): CellTarget => {
    const cell = sheetCell(target, budget.meta.currencyId)
    return { id: cell.id, name: cell.name, budgeted: cell.amount }
  }
  const expenseSheetTarget = sheetTarget?.kind === 'expense' ? sheetTarget : null
```

9. Host the sheet next to `<CommentsPanel>`. Each action closes the sheet in the same update that opens the next dialog, so the two never stack:

```tsx
      <ElementSheet
        target={sheetTarget}
        month={selectedDate}
        baseCurrencyId={budget.meta.currencyId}
        currencies={currencies}
        exchange={makeBudgetExchange(budget, currencies)}
        comments={sheetTarget ? commentsByCell.get(commentCellKey(sheetCell(sheetTarget, budget.meta.currencyId).id, selectedDate)) ?? [] : []}
        commentsReadOnly={commentsReadOnly(budget.meta, selectedDate)}
        canSetAmount={sheetTarget ? sheetCanSetAmount(sheetTarget) : false}
        onClose={() => setSheetTarget(null)}
        onSetAmount={() => {
          if (sheetTarget) {
            setLimitTarget(sheetCellTarget(sheetTarget))
            setSheetTarget(null)
          }
        }}
        onOpenComments={() => {
          if (sheetTarget) {
            openComments(sheetCellTarget(sheetTarget))
            setSheetTarget(null)
          }
        }}
        onShowTransactions={
          expenseSheetTarget
            ? () => {
                setTransactionsTarget(transactionsTargetOf(expenseSheetTarget.element))
                setSheetTarget(null)
              }
            : undefined
        }
      />
```

- [ ] **Step 8: `PlanSheet.tsx` (props only)**

1. Delete `useIsPhone`: its import, the `isPhone` context field, `const isPhone = useIsPhone()`, and both `isPhone` entries in the context object and its deps.
2. Marker condition: change `(!ctx.isPhone && !ctx.editMode && …)` to `(!ctx.editMode && …)`.
3. The cell's `<CellShell>` keeps only `comments`, `previewDisabled`, `shortcutDisabled={ctx.editMode}` and `onOpenComments`.
4. Its `<SetLimitDialog>` loses `commentCount`, `onOpenComments` and their comment line.

- [ ] **Step 9: Delete `web/src/hooks/useLongPress.ts`**

- [ ] **Step 10: Regression checklist**

In `docs/regression-test-plan.md` §9:
1. Delete these items, and any other item describing the long-press actions modal or the "Comments (N)" button inside the set-limit sheet (`grep -n "long-press\|Comments (N)\|Available pill" docs/regression-test-plan.md`):
   - the one beginning `- [ ] 📱 Phone: the corner marker, tap-Available and long-press still reach`
   - the one beginning `- [ ] 📱 On a phone (the iOS home-screen PWA included), tap a cell to open`
   - the one beginning `- [ ] 📱 On a phone, tap the Available pill of an individually-archived`
   - the tablet long-press item (it begins with the long-press that "opens a modal titled with the item's name")
2. In the item that says `on a phone a tap opens the set-limit dialog with a "Comments (N)"`, drop the phone clause.
3. Add, wrapped like the neighbouring items:

```markdown
- [ ] 📱 Tablet (640–1023 px), Budget view: tap a budgeted amount (editable or
      not, Archive rows included) or a Savings planned amount: the item sheet
      opens ("Food · July": Budget, Spent, Available, latest comment,
      "Comments (N)" / "Add comment", "Set budget", "Transactions"). "Set
      budget" replaces the sheet with the amount dialog (no comments in it);
      "Comments" replaces it with the thread. A guest's sheet has no "Set
      budget". The Available pill is not a button; tapping Spent still lists
      the transactions; the corner marker still opens the thread beside the cell.
- [ ] 📱 Tablet: holding a finger on a cell opens nothing extra (no actions
      menu, no text-selection callout from the app) and the tap still works.
```

- [ ] **Step 11: Run the budgets suite, typecheck and lint**

Run: `pnpm vitest run src/features/budgets --maxWorkers=2 && pnpm exec tsc -b && pnpm lint`
Expected: all pass, lint 0 errors. Any other failure is a test asserting a removed path (long-press, actions modal, tap-Available, set-limit "Comments (N)"): delete it, or re-point it to the sheet if the behaviour moved there. List each one in your report.

- [ ] **Step 12: Commit**

```bash
git add -A web/src docs/regression-test-plan.md
git commit -m "feat: Tablet taps open the item sheet; no long-press anywhere

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: Tablet Plan grid: a tap on a month cell opens the item sheet

**Files:**
- Modify: `web/src/features/budgets/PlanSheet.tsx`
- Tests: `web/src/features/budgets/comments.plan.test.tsx`, `web/src/features/budgets/comments.plan.guest.test.tsx`, `web/src/features/budgets/PlanSheet.test.tsx`
- Docs: `docs/regression-test-plan.md`

**Interfaces:**
- Consumes:
  - Task 2: `planCellFigures`, `planMonthExchange`.
  - Task 4: `ElementSheet`.
  - Task 5: `SetLimitDialog`'s `title`.
- Adds `openSheet: (target: PlanLimitTarget) => void` to the grid context. `PlanLimitTarget` is the existing `{ el, month, monthIndex }`.

- [ ] **Step 1: Write the failing tests**

`comments.plan.test.tsx`: replace `it('keeps the amount dialog free of comments on a tablet', …)` with the tests below. Use the file's tablet helper; add one as in Task 5 if the file has none. `PointerEventsCheckLevel` is imported from `@testing-library/user-event`.

```tsx
it('a tablet tap on a plan cell opens the item sheet for that month', async () => {
  usePlanHandlers()
  mockTabletViewport()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage('/plan')

  await user.click(await screen.findByTestId('plan-cell-pe1:1'))
  const sheet = await screen.findByTestId('element-sheet')
  expect(within(sheet).getByTestId('sheet-figure-budget')).toBeInTheDocument()
  expect(within(sheet).getByTestId('sheet-figure-spent')).toBeInTheDocument()
  expect(within(sheet).getByRole('button', { name: 'Comments (1)' })).toBeInTheDocument()
})

it('a tablet sheet’s Set budget opens the amount dialog with no comments in it', async () => {
  usePlanHandlers()
  mockTabletViewport()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage('/plan')

  await user.click(await screen.findByTestId('plan-cell-pe1:1'))
  await user.click(within(await screen.findByTestId('element-sheet')).getByRole('button', { name: 'Set budget' }))
  expect(await screen.findByLabelText('Budget')).toBeInTheDocument()
  expect(screen.queryByTestId('element-sheet')).toBeNull()
  expect(screen.queryByRole('button', { name: /Comments \(/ })).toBeNull()
})

it('a tablet sheet’s Comments opens the thread', async () => {
  usePlanHandlers()
  mockTabletViewport()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage('/plan')

  await user.click(await screen.findByTestId('plan-cell-pe1:1'))
  await user.click(within(await screen.findByTestId('element-sheet')).getByRole('button', { name: 'Comments (1)' }))
  expect(await screen.findByRole('button', { name: 'Post' })).toBeInTheDocument()
  expect(screen.queryByTestId('element-sheet')).toBeNull()
})

it('a tablet marker tap opens only the thread, not the sheet', async () => {
  usePlanHandlers()
  mockTabletViewport()
  const user = userEvent.setup()
  renderPage('/plan')

  await user.click(within(await screen.findByTestId('plan-cell-pe1:1')).getByTestId('comment-marker'))
  expect(await screen.findByTestId('comments-popover')).toBeInTheDocument()
  expect(screen.queryByTestId('element-sheet')).toBeNull()
})
```

`comments.plan.guest.test.tsx`: replace `it('lets a guest start a thread on a cell with no existing comments, on compact viewports', …)` with:

```tsx
it('a guest’s tablet tap opens the sheet without Set budget, and Add comment starts a thread', async () => {
  useGuestPlanHandlers()
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q.includes('1023'), media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage('/plan')

  await user.click(await screen.findByTestId('plan-cell-pe1:0'))
  const sheet = await screen.findByTestId('element-sheet')
  expect(within(sheet).queryByRole('button', { name: 'Set budget' })).toBeNull()
  await user.click(within(sheet).getByRole('button', { name: 'Add comment' }))
  expect(await screen.findByRole('button', { name: 'Post' })).toBeInTheDocument()
})
```

If `mockCompactViewport` ends up unused in either file, delete it.

`PlanSheet.test.tsx`: `grep -n "limit \|matches: true\|1023" PlanSheet.test.tsx`. Rewrite any test that clicks the compact `limit {name}` button so that it taps the cell and chooses "Set budget" in the sheet (`element-sheet`). A test that only checks desktop inline editing stays.

- [ ] **Step 2: Run the tests and check that they fail**

Run: `pnpm vitest run src/features/budgets/comments.plan.test.tsx src/features/budgets/comments.plan.guest.test.tsx --maxWorkers=2`
Expected: FAIL, with no `element-sheet` after a cell tap.

- [ ] **Step 3: Implement in `PlanSheet.tsx`**

1. Imports: `ElementSheet` from `./ElementSheet`; `planCellFigures` from `./phoneMonth`; `planMonthExchange` added to the `./planMath` import list.
2. Grid context: add `openSheet: (target: PlanLimitTarget) => void`, documented as "touch viewports: a cell tap opens the item sheet". Wire it into the context object and its deps exactly as `openDialog` is wired.
3. State in `PlanSheet`, next to `planLimitTarget`:

```ts
  const [sheetCellTarget, setSheetCellTarget] = useState<PlanLimitTarget | null>(null)
  const openSheet = useCallback((target: PlanLimitTarget) => setSheetCellTarget(target), [])
```

4. The month cell's `onClick` becomes:

```tsx
              onClick={(e) => {
                ctx.select(rk, i, e)
                // touch: the whole cell opens the item sheet; the marker stops its own click
                if (ctx.isCompact && !ctx.editMode && !isUncategorized && idx >= 0) {
                  ctx.openSheet(target)
                }
              }}
```

5. The `cell-planned` span no longer nests a button on touch. `ctx.isCompact` renders plain text, `editable ? moneyFormat(plannedValue, currency, { showCurrency: false, useNativePrecision: false }) : plannedText`. Desktop keeps the `LimitEditor` for editable cells and the `comments {name}` button for the others:

```tsx
              <span data-testid="cell-planned" className="text-sm">
                {ctx.isCompact ? (
                  editable ? moneyFormat(plannedValue, currency, { showCurrency: false, useNativePrecision: false }) : plannedText
                ) : editable ? (
                  <LimitEditor … unchanged … />
                ) : !isUncategorized ? (
                  … the existing `comments {displayName}` button, unchanged …
                ) : (
                  plannedText
                )}
              </span>
```

6. Host the sheet next to `<CommentsPanel>`, using the same `plan`, `budget.meta` / `meta`, `userId`, `commentsByCell`, `openDialog`, `openComments` and `openTransactions` values the grid context already receives. Use whatever names those carry in this component:

```tsx
      <ElementSheet
        target={sheetCellTarget ? { kind: 'plan', cell: planCellFigures(sheetCellTarget.el, sheetCellTarget.monthIndex) } : null}
        month={sheetCellTarget?.month ?? ''}
        baseCurrencyId={budget.meta.currencyId}
        currencies={currencies}
        exchange={plan && sheetCellTarget ? planMonthExchange(plan, currencies, sheetCellTarget.monthIndex) : (_from, _to, amount) => amount}
        comments={sheetCellTarget ? commentsByCell.get(commentCellKey(sheetCellTarget.el.id, sheetCellTarget.month)) ?? [] : []}
        commentsReadOnly={sheetCellTarget ? commentsReadOnly(budget.meta, sheetCellTarget.month) : true}
        canSetAmount={sheetCellTarget ? isEditableCell(sheetCellTarget.el, sheetCellTarget.month, sheetCellTarget.monthIndex, budget.meta, userId) : false}
        onClose={() => setSheetCellTarget(null)}
        onSetAmount={() => {
          if (sheetCellTarget) {
            openDialog(sheetCellTarget)
            setSheetCellTarget(null)
          }
        }}
        onOpenComments={() => {
          if (sheetCellTarget) {
            openComments(sheetCellTarget, { anchor: null })
            setSheetCellTarget(null)
          }
        }}
        onShowTransactions={
          sheetCellTarget &&
          sheetCellTarget.el.id !== UNCATEGORIZED_ID &&
          !isIncomeType(sheetCellTarget.el.type) &&
          sheetCellTarget.el.type !== BudgetElementType.SAVINGS
            ? () => {
                openTransactions(sheetCellTarget.el, sheetCellTarget.month)
                setSheetCellTarget(null)
              }
            : undefined
        }
      />
```

   The `el` stored in the target is the rendered row element. For a savings row that is `savingsAsPlanElement(projectSavingsClosings(row, plan.months))`, so the sheet's month-end Balance is the projected one the grid shows.
7. On `PlanSheet`'s own `<SetLimitDialog>`, pass `title={planLimitTarget && isIncomeType(planLimitTarget.el.type) ? t('budgets.page.sheet.set_plan') : undefined}`.

- [ ] **Step 4: Regression checklist**

Add to §9, next to the Task 5 tablet items:

```markdown
- [ ] 📱 Tablet, Plan view: tap any month cell (not the Uncategorized row):
      the cell is selected and the item sheet opens for that element and
      month — Budget/Spent (Planned/Received for income, Planned/Saved/Balance
      for savings), latest comment, "Set budget" ("Set plan" for income),
      "Comments", and "Transactions" for expense rows. The sheet's month and
      any foreign-currency rate are the tapped column's. The corner marker still
      opens only the thread.
```

- [ ] **Step 5: Run the budgets suite, typecheck and lint**

Run: `pnpm vitest run src/features/budgets --maxWorkers=2 && pnpm exec tsc -b && pnpm lint`
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add -A web/src docs/regression-test-plan.md
git commit -m "feat: Tablet taps on a plan cell open the item sheet

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: The phone month list (`PhoneMonthView`)

**Files:**
- Create: `web/src/features/budgets/PhoneMonthView.tsx`
- Test: `web/src/features/budgets/PhoneMonthView.test.tsx`
- Modify: `web/src/features/budgets/budgetStore.ts` (export the reporting-tags fold key)
- Modify: `web/src/features/budgets/BudgetTable.tsx` (import that key instead of its private copy)

**Interfaces:**
- Consumes:
  - Task 1: `rowState`, `rowProgress`, `RowState`.
  - Task 2: `PlanMonthFigures`, `PlanCellFigures`, `SheetTarget`.
  - Task 3 keys.
  - Existing: `BudgetBuckets`, `budgetTotals`, `displayAvailable`, `elementDisplayName`, `makeBudgetExchange`, `totalsWithSavings`, `commentCellKey`, `currentMonth`, `BudgetTransactionsTarget`, `useBudgetPeriodStore`.
- Produces:

```ts
export interface PhoneMonthViewProps {
  budget: BudgetDto
  buckets: BudgetBuckets
  currencies: CurrencyDto[]
  selectedDate: string
  /** null while get-budget-plan loads, fails, or still shows another window */
  planMonth: PlanMonthFigures | null
  commentsByCell: Map<string, BudgetCommentDto[]>
  onOpenSheet: (target: SheetTarget) => void
  /** rows whose only action is their transaction list (children, reporting tags) */
  onShowTransactions: (target: BudgetTransactionsTarget) => void
}
export function PhoneMonthView(props: PhoneMonthViewProps): JSX.Element
```

  and `export const REPORTING_TAGS_FOLD_ID = '__reporting_tags__'` from `budgetStore.ts`.
- Test ids:
  - `phone-month-view`
  - `phone-heading`
  - `phone-income`
  - `phone-income-summary`
  - `phone-folder-{folderId | __no_folder__ | __uncategorized__ | __archive__}`
  - `phone-labels`
  - `phone-savings`
  - `phone-row-{elementId}`
  - `phone-income-row-{elementId}`
  - `phone-savings-row-{id}`
  - `phone-child-{childId}`
  - `phone-label-{labelId}`
  - `phone-progress`
  - `phone-comment-indicator`
  - `phone-currency-tag`
  - `phone-totals`
  - `phone-total-{expenses|savings|available|balance|savings-balance|transfers}`

- [ ] **Step 1: Move the reporting-tags fold key**

In `budgetStore.ts`, add at the top level:

```ts
/** the reporting-tags folder exists only in rendering: no folder row stands behind
 *  it, so its fold state is keyed by a reserved literal no element id (a UUID) can
 *  collide with; the table and the phone view share it */
export const REPORTING_TAGS_FOLD_ID = '__reporting_tags__'
```

In `BudgetTable.tsx`:
1. Delete the local `REPORTING_TAGS_FOLD_ID` constant and its three-line comment.
2. Import the constant: `import { REPORTING_TAGS_FOLD_ID, useBudgetPeriodStore } from './budgetStore'`.

- [ ] **Step 2: Write the failing tests**

Create `PhoneMonthView.test.tsx`:

```tsx
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { coerceBudgetFixture } from '@/test/coerceBudget'
import { fixtureWireBudget } from '@/test/fixtures'
import type { BudgetCommentDto, BudgetDto, PlanElementDto } from '@/api/dto/budget'
import { bucketElements, makeBudgetExchange } from './budgetMath'
import { useBudgetPeriodStore } from './budgetStore'
import type { PlanMonthFigures } from './phoneMonth'
import { PhoneMonthView } from './PhoneMonthView'
import type { PhoneMonthViewProps } from './PhoneMonthView'

const usd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const eur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 }

const salaries = { id: 'ie1', type: 4, name: 'Salaries', icon: 'payments', currencyId: 'cur-usd', isArchived: 0, folderId: null, position: 3, ownerUserId: null, cells: [], children: [] } as PlanElementDto
const planMonth: PlanMonthFigures = {
  month: '2026-07-01',
  index: 2,
  income: { rows: [{ element: salaries, planned: '2000', actual: '400' }], planned: '2000', received: '400' },
  balance: '4545',
  savingsBalance: null,
  transfersNet: '0',
}

function renderView(overrides: Partial<PhoneMonthViewProps> = {}, mutate?: (b: BudgetDto) => void) {
  const budget = coerceBudgetFixture(fixtureWireBudget)
  mutate?.(budget)
  const props: PhoneMonthViewProps = {
    budget,
    buckets: bucketElements(budget, makeBudgetExchange(budget, [usd, eur])),
    currencies: [usd, eur],
    selectedDate: '2026-07-01',
    planMonth,
    commentsByCell: new Map(),
    onOpenSheet: vi.fn(),
    onShowTransactions: vi.fn(),
    ...overrides,
  }
  render(<PhoneMonthView {...props} />)
  return props
}

beforeEach(() => {
  localStorage.clear()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: {}, foldBudgetId: null })
})

it('names the budget currency once in the heading row, above Budget and Spent', () => {
  renderView()
  const heading = screen.getByTestId('phone-heading')
  expect(heading).toHaveTextContent('USD')
  expect(heading).toHaveTextContent('Budget')
  expect(heading).toHaveTextContent('Spent')
  expect(heading).not.toHaveTextContent('Available')
})

it('shows each expense row as one button with Budget and Spent, no symbols', async () => {
  const props = renderView()
  const food = screen.getByRole('button', { name: 'Food, budget 200.00, spent 45.50' })
  expect(within(screen.getByTestId('phone-row-cat-food')).queryByText(/\$/)).toBeNull()
  await userEvent.click(food)
  expect(props.onOpenSheet).toHaveBeenCalledWith({ kind: 'expense', element: expect.objectContaining({ id: 'cat-food' }) })
})

it('tags a foreign-currency element with its code and keeps its amounts unconverted', () => {
  renderView()
  const living = screen.getByTestId('phone-row-env-1')
  expect(within(living).getByTestId('phone-currency-tag')).toHaveTextContent('EUR')
  expect(within(living).getByText('90.00')).toBeInTheDocument()
  expect(within(screen.getByTestId('phone-row-cat-food')).queryByTestId('phone-currency-tag')).toBeNull()
})

it('draws the progress bar and colours Spent by row state', () => {
  renderView({}, (b) => {
    const food = b.structure.elements.find((el) => el.id === 'cat-food')!
    Object.assign(food, { budgeted: '100', spent: '150', budgetSpent: '150', available: '-120' })
  })
  const row = screen.getByTestId('phone-row-cat-food')
  expect(within(row).getByTestId('phone-progress').firstElementChild).toHaveStyle({ width: '100%' })
  expect(within(row).getByText('150.00').className).toContain('text-expense')
})

it('amber when carry-over covers the overspend', () => {
  renderView({}, (b) => {
    const food = b.structure.elements.find((el) => el.id === 'cat-food')!
    Object.assign(food, { budgeted: '100', spent: '150', budgetSpent: '150', available: '10' })
  })
  expect(within(screen.getByTestId('phone-row-cat-food')).getByText('150.00').className).toContain('text-amber-600')
})

it('a future month shows a dash for Spent, no bar, and no colour', () => {
  renderView({ selectedDate: '2099-01-01' }, (b) => {
    const food = b.structure.elements.find((el) => el.id === 'cat-food')!
    Object.assign(food, { budgeted: '100', spent: '150', budgetSpent: '150', available: '-120' })
  })
  const row = screen.getByTestId('phone-row-cat-food')
  expect(within(row).queryByTestId('phone-progress')).toBeNull()
  expect(screen.getByRole('button', { name: 'Food, budget 100.00, spent —' })).toBeInTheDocument()
})

it('marks a commented row with a non-interactive indicator', () => {
  const comment = { id: 'c1', elementId: 'cat-food', period: '2026-07-01', comment: 'x', author: { id: 'u1', avatar: 'face:emerald', name: 'Ada' }, createdAt: '2026-07-01 09:00:00', updatedAt: '2026-07-01 09:00:00' } as BudgetCommentDto
  renderView({ commentsByCell: new Map([['cat-food|2026-07-01', [comment]]]) })
  const indicator = within(screen.getByTestId('phone-row-cat-food')).getByTestId('phone-comment-indicator')
  expect(indicator).toHaveAttribute('aria-hidden', 'true')
  expect(within(screen.getByTestId('phone-row-env-1')).queryByTestId('phone-comment-indicator')).toBeNull()
})

it('folders show their Budget and Spent sums; the unfoldered bucket reads "No folder"', () => {
  renderView()
  const essentials = screen.getByTestId('phone-folder-bf1')
  expect(essentials).toHaveTextContent('Essentials')
  expect(essentials).toHaveTextContent('200.00')
  expect(screen.getByTestId('phone-folder-__no_folder__')).toHaveTextContent('No folder')
})

it('the chevron unfolds children, the row opens the sheet, and a child opens its transactions', async () => {
  const props = renderView()
  const living = screen.getByTestId('phone-row-env-1')
  await userEvent.click(within(living).getByRole('button', { name: 'Expand' }))
  expect(props.onOpenSheet).not.toHaveBeenCalled()
  await userEvent.click(screen.getByTestId('phone-child-cat-rent'))
  expect(props.onShowTransactions).toHaveBeenCalledWith(expect.objectContaining({ id: 'cat-rent', parent: { id: 'env-1', type: 0 } }))
})

it('collapses income into one summary row that unfolds into income rows', async () => {
  const props = renderView()
  const summary = screen.getByTestId('phone-income-summary')
  expect(summary).toHaveTextContent('Income · 400.00 of 2,000.00')
  expect(summary).toHaveAttribute('aria-expanded', 'false')
  expect(screen.queryByTestId('phone-income-row-ie1')).toBeNull()
  await userEvent.click(summary)
  await userEvent.click(screen.getByRole('button', { name: 'Salaries, planned 2,000.00, received 400.00' }))
  expect(props.onOpenSheet).toHaveBeenCalledWith({ kind: 'plan', cell: planMonth.income.rows[0] })
})

it('leaves income and the plan lines out while the plan is not loaded', () => {
  renderView({ planMonth: null })
  expect(screen.queryByTestId('phone-income')).toBeNull()
  expect(screen.queryByTestId('phone-total-balance')).toBeNull()
  expect(screen.getByTestId('phone-total-expenses')).toHaveTextContent('45.50 of 290.00')
})

it('the totals card lists Expenses, Available and Balance; Transfers only when non-zero', () => {
  renderView()
  expect(screen.getByTestId('phone-total-available')).toBeInTheDocument()
  expect(screen.getByTestId('phone-total-balance')).toHaveTextContent('4,545.00')
  expect(screen.queryByTestId('phone-total-transfers')).toBeNull()
  expect(screen.queryByTestId('phone-total-savings')).toBeNull()
  expect(screen.queryByTestId('phone-total-savings-balance')).toBeNull()
})

it('shows Transfers when money crossed the budget boundary', () => {
  renderView({ planMonth: { ...planMonth, transfersNet: '-100' } })
  expect(screen.getByTestId('phone-total-transfers')).toHaveTextContent('-100.00')
})

it('lists savings rows with Planned and Saved, and the totals card adds the savings lines', async () => {
  const props = renderView({ planMonth: { ...planMonth, savingsBalance: '1040' } }, (b) => {
    b.structure.savings = [
      { id: 'acc-s1', type: 5, name: 'Rainy day', icon: 'savings', currencyId: 'cur-usd', ownerUserId: 'u1', isArchived: 0, position: 0, budgeted: '100', spent: '40', available: '60', closingBalance: '1040' },
    ]
  })
  const savings = screen.getByTestId('phone-savings')
  expect(savings).toHaveTextContent('Planned')
  expect(savings).toHaveTextContent('Saved')
  await userEvent.click(screen.getByRole('button', { name: 'Rainy day, planned 100.00, saved 40.00' }))
  expect(props.onOpenSheet).toHaveBeenCalledWith({ kind: 'savings', row: expect.objectContaining({ id: 'acc-s1' }) })
  expect(screen.getByTestId('phone-total-savings')).toHaveTextContent('40.00 of 100.00')
  expect(screen.getByTestId('phone-total-savings-balance')).toHaveTextContent('1,040.00')
})

it('shows the uncategorized row without a budget, and it opens the sheet', async () => {
  const props = renderView({}, (b) => {
    b.structure.elements.push({
      id: 'uncategorized', type: 1, name: 'Uncategorized', icon: 'question_mark', currencyId: null, isArchived: 0, folderId: null,
      position: 99, budgeted: '0', available: '0', spent: '12', budgetSpent: '12', ownerUserId: null, children: [],
    })
  })
  const row = screen.getByTestId('phone-row-uncategorized')
  expect(row).toHaveTextContent('—')
  await userEvent.click(within(row).getByRole('button'))
  expect(props.onOpenSheet).toHaveBeenCalledWith({ kind: 'expense', element: expect.objectContaining({ id: 'uncategorized' }) })
})
```

`fixtureWireBudget` has Food (200 budget) in Essentials and Living (90 EUR, rate 0.9) with no folder. The Expenses total is therefore spent 45.50 of 200 + converted Living budget. `290.00` assumes the EUR exchange lands at 90. If `makeBudgetExchange` converts 90 EUR differently, take the real figure from `budgetTotals(bucketElements(...))` in a scratch run and use it. The assertion's purpose is "Expenses reads `spent of budget` from the budget totals", not the FX maths.

- [ ] **Step 3: Run the tests and check that they fail**

Run: `pnpm vitest run src/features/budgets/PhoneMonthView.test.tsx --maxWorkers=2`
Expected: FAIL, because the module does not exist.

- [ ] **Step 4: Implement**

Create `PhoneMonthView.tsx`:

```tsx
import type { ReactNode } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { EntityIcon } from '@/components/EntityIcon'
import { moneyFormat } from '@/lib/money'
import { cmp, isZero } from '@/lib/decimal'
import type { BudgetCommentDto, BudgetDto, BudgetElementDto, BudgetSavingsElementDto, LabelSpendDto } from '@/api/dto/budget'
import { UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'
import type { BudgetBuckets, FolderBucket, RowState } from './budgetMath'
import { budgetTotals, displayAvailable, elementDisplayName, makeBudgetExchange, rowProgress, rowState, totalsWithSavings } from './budgetMath'
import { REPORTING_TAGS_FOLD_ID, useBudgetPeriodStore } from './budgetStore'
import type { BudgetTransactionsTarget } from './BudgetTransactionsDialog'
import type { PlanCellFigures, PlanMonthFigures, SheetTarget } from './phoneMonth'
import { currentMonth } from './planMath'
import { commentCellKey } from './queries'

const INCOME_FOLD_ID = '__phone_income__'
const EMPTY = '—'
// name | Budget | Spent: the heading row, folder headers and rows share one grid
const GRID = 'grid grid-cols-[minmax(0,1fr)_5.5rem_5.5rem] items-center gap-x-2'

const STATE_TEXT: Record<RowState, string> = {
  none: '',
  ok: '',
  covered: 'text-amber-600 dark:text-amber-500',
  over: 'text-expense',
}
const STATE_BAR: Record<RowState, string> = {
  none: 'bg-muted-foreground/40',
  ok: 'bg-muted-foreground/40',
  covered: 'bg-amber-500',
  over: 'bg-expense',
}

export interface PhoneMonthViewProps {
  budget: BudgetDto
  buckets: BudgetBuckets
  currencies: CurrencyDto[]
  selectedDate: string
  /** null while get-budget-plan loads, fails, or still shows another window */
  planMonth: PlanMonthFigures | null
  commentsByCell: Map<string, BudgetCommentDto[]>
  onOpenSheet: (target: SheetTarget) => void
  /** rows whose only action is their transaction list (children, reporting tags) */
  onShowTransactions: (target: BudgetTransactionsTarget) => void
}

interface RowProps {
  testId: string
  icon: string
  name: string
  tag?: string
  first: string
  second: string
  secondClass?: string
  progress?: number | null
  barClass?: string
  commented?: boolean
  ariaLabel: string
  onOpen: () => void
  /** an expandable row's chevron: a separate button over the icon slot */
  toggle?: { open: boolean; onToggle: () => void; label: string }
}

function PhoneRow({ testId, icon, name, tag, first, second, secondClass = '', progress = null, barClass = '', commented = false, ariaLabel, onOpen, toggle }: RowProps) {
  const Chevron = toggle?.open ? ChevronDown : ChevronRight
  return (
    <div className="relative" data-testid={testId}>
      <button type="button" aria-label={ariaLabel} onClick={onOpen} className={`${GRID} min-h-11 w-full rounded-md px-2 py-2 text-left active:bg-accent/50`}>
        <span className="flex min-w-0 flex-col gap-1">
          <span className="flex min-w-0 items-center gap-2">
            {toggle ? <span className="size-5 shrink-0" /> : <EntityIcon name={icon} className="text-lg text-muted-foreground" />}
            <span className="min-w-0 truncate text-[15px]">{name}</span>
            {tag ? (
              <span data-testid="phone-currency-tag" className="shrink-0 rounded bg-muted px-1 text-[10px] font-medium text-muted-foreground">
                {tag}
              </span>
            ) : null}
          </span>
          {progress !== null ? (
            <span data-testid="phone-progress" className="ml-7 h-1 overflow-hidden rounded-full bg-muted">
              <span className={`block h-full rounded-full ${barClass}`} style={{ width: `${Math.round(progress * 100)}%` }} />
            </span>
          ) : null}
        </span>
        <span className="text-right text-[15px] tabular-nums">{first}</span>
        <span className={`relative text-right text-[15px] tabular-nums ${secondClass}`}>
          {second}
          {commented ? (
            <span
              aria-hidden="true"
              data-testid="phone-comment-indicator"
              className="absolute -top-1.5 -right-2 h-0 w-0 border-t-[7px] border-l-[7px] border-t-primary border-l-transparent"
            />
          ) : null}
        </span>
      </button>
      {toggle ? (
        // a sibling, never nested in the row button: the chevron folds, the row opens the sheet
        <button
          type="button"
          aria-expanded={toggle.open}
          aria-label={toggle.label}
          onClick={toggle.onToggle}
          className="absolute top-0 left-0 flex h-11 w-10 items-center justify-center text-muted-foreground"
        >
          <Chevron className="size-4.5" />
        </button>
      ) : null}
    </div>
  )
}

function Card({ testId, header, children }: { testId: string; header?: ReactNode; children: ReactNode }) {
  return (
    <section className="rounded-md border p-1" data-testid={testId}>
      {header}
      {children}
    </section>
  )
}

function CardHeader({ name, first, second }: { name: string; first?: string; second?: string }) {
  return (
    <div className={`${GRID} px-2 pt-1.5 pb-0.5 text-xs font-medium text-muted-foreground`}>
      <span className="truncate">{name}</span>
      <span className="text-right tabular-nums">{first}</span>
      <span className="text-right tabular-nums">{second}</span>
    </div>
  )
}

function TotalLine({ testId, label, value, negative = false }: { testId: string; label: string; value: string; negative?: boolean }) {
  return (
    <div className="flex items-baseline justify-between gap-3" data-testid={testId}>
      <span className="text-[13px] text-muted-foreground">{label}</span>
      <span className={`text-[15px] tabular-nums ${negative ? 'text-expense' : ''}`}>{value}</span>
    </div>
  )
}

export function PhoneMonthView({ budget, buckets, currencies, selectedDate, planMonth, commentsByCell, onOpenSheet, onShowTransactions }: PhoneMonthViewProps) {
  const { t } = useTranslation()
  const unfolded = useBudgetPeriodStore((s) => s.unfoldedElements)
  const toggleElement = useBudgetPeriodStore((s) => s.toggleElement)

  const base = budget.meta.currencyId
  const baseCurrency = currencies.find((c) => c.id === base)
  const currencyOf = (id: Id | null) => currencies.find((c) => c.id === (id ?? base))
  const fmt = (amount: string, currencyId: Id | null = base) => {
    const c = currencyOf(currencyId)
    return moneyFormat(amount, c, { showCurrency: false, useNativePrecision: false, maxPrecision: c?.fractionDigits ?? 2 })
  }
  const tagOf = (currencyId: Id | null) => (currencyId && currencyId !== base ? currencyOf(currencyId)?.code : undefined)
  const future = selectedDate > currentMonth()
  const commented = (id: Id) => (commentsByCell.get(commentCellKey(id, selectedDate))?.length ?? 0) > 0
  const expandLabel = (open: boolean) => t(open ? 'common.button.collapse.label' : 'common.button.expand.label')

  const expenseRow = (element: BudgetElementDto) => {
    const name = elementDisplayName(element.id, element.name, t)
    const isUncategorized = element.id === UNCATEGORIZED_ID
    const figures = { budgeted: element.budgeted, spent: element.spent, available: displayAvailable(element) }
    const state: RowState = isUncategorized ? 'none' : rowState(figures, future)
    const budgetText = isUncategorized ? EMPTY : fmt(element.budgeted, element.currencyId)
    const spentText = future ? EMPTY : fmt(element.spent, element.currencyId)
    const expandable = element.children.length > 0
    const open = !!unfolded[element.id]
    return (
      <div key={element.id}>
        <PhoneRow
          testId={`phone-row-${element.id}`}
          icon={element.icon}
          name={name}
          tag={tagOf(element.currencyId)}
          first={budgetText}
          second={spentText}
          secondClass={STATE_TEXT[state]}
          progress={isUncategorized ? null : rowProgress(figures, future)}
          barClass={STATE_BAR[state]}
          commented={commented(element.id)}
          ariaLabel={t('budgets.page.phone.row_aria', { name, budget: budgetText, spent: spentText })}
          onOpen={() => onOpenSheet({ kind: 'expense', element })}
          toggle={expandable ? { open, onToggle: () => toggleElement(element.id), label: expandLabel(open) } : undefined}
        />
        {expandable && open
          ? element.children.map((child) => {
              const childName = elementDisplayName(child.id, child.name, t)
              return (
                <button
                  key={child.id}
                  type="button"
                  data-testid={`phone-child-${child.id}`}
                  className={`${GRID} min-h-10 w-full rounded-md py-1.5 pr-2 pl-9 text-left text-sm text-muted-foreground active:bg-accent/50`}
                  onClick={() =>
                    onShowTransactions({
                      id: child.id,
                      type: child.type,
                      name: childName,
                      icon: child.icon,
                      currencyId: element.currencyId,
                      parent: { id: element.id, type: element.type },
                    })
                  }
                >
                  <span className="flex min-w-0 items-center gap-2">
                    <EntityIcon name={child.icon} className="text-lg" />
                    <span className="truncate">{childName}</span>
                  </span>
                  <span />
                  <span className="text-right tabular-nums">{future ? EMPTY : fmt(child.spent, element.currencyId)}</span>
                </button>
              )
            })
          : null}
      </div>
    )
  }

  const folderCard = (key: string, name: string | null, bucket: FolderBucket) => (
    <Card
      key={key}
      testId={`phone-folder-${key}`}
      header={name !== null ? <CardHeader name={name} first={fmt(bucket.stats.budgeted)} second={future ? EMPTY : fmt(bucket.stats.spent)} /> : undefined}
    >
      {bucket.elements.map(expenseRow)}
    </Card>
  )

  const incomeRow = (row: PlanCellFigures) => {
    const el = row.element
    const name = elementDisplayName(el.id, el.name, t)
    const planned = fmt(row.planned, el.currencyId)
    const received = future ? EMPTY : fmt(row.actual, el.currencyId)
    return (
      <PhoneRow
        key={`${el.id}:${el.type}`}
        testId={`phone-income-row-${el.id}`}
        icon={el.icon}
        name={name}
        tag={tagOf(el.currencyId)}
        first={planned}
        second={received}
        commented={commented(el.id)}
        ariaLabel={t('budgets.page.phone.income_row_aria', { name, planned, received })}
        onOpen={() => onOpenSheet({ kind: 'plan', cell: row })}
      />
    )
  }

  const savingsRow = (row: BudgetSavingsElementDto) => {
    const planned = fmt(row.budgeted, row.currencyId)
    const saved = future ? EMPTY : fmt(row.spent, row.currencyId)
    return (
      <PhoneRow
        key={row.id}
        testId={`phone-savings-row-${row.id}`}
        icon={row.icon}
        name={row.name}
        tag={tagOf(row.currencyId)}
        first={planned}
        second={saved}
        commented={commented(row.id)}
        ariaLabel={t('budgets.page.phone.savings_row_aria', { name: row.name, planned, saved })}
        onOpen={() => onOpenSheet({ kind: 'savings', row })}
      />
    )
  }

  const labelRow = (label: LabelSpendDto) => (
    <button
      key={label.id}
      type="button"
      data-testid={`phone-label-${label.id}`}
      className={`${GRID} min-h-11 w-full rounded-md px-2 py-2 text-left active:bg-accent/50`}
      onClick={() => onShowTransactions({ id: label.id, type: 'label', name: label.name, icon: label.icon, currencyId: null })}
    >
      <span className="flex min-w-0 items-center gap-2">
        <EntityIcon name={label.icon} className="text-lg text-muted-foreground" />
        <span className="truncate text-[15px]">{label.name}</span>
      </span>
      <span className="text-right text-[15px] text-muted-foreground">{EMPTY}</span>
      <span className="text-right text-[15px] tabular-nums">{future ? EMPTY : fmt(label.spent)}</span>
    </button>
  )

  const exchangeFn = makeBudgetExchange(budget, currencies)
  const expenseTotals = budgetTotals(buckets)
  const total = totalsWithSavings(expenseTotals, budget, exchangeFn)
  const savingsRows = [...(budget.structure.savings ?? [])].sort((a, b) => a.isArchived - b.isArchived || a.position - b.position)
  const savingsSum = savingsRows.length > 0 ? totalsWithSavings({ budgeted: '0', spent: '0', available: '0' }, budget, exchangeFn) : null
  const labels = budget.structure.labels ?? []
  const labelsOpen = !!unfolded[REPORTING_TAGS_FOLD_ID]
  const incomeOpen = !!unfolded[INCOME_FOLD_ID]
  const hasFolders = buckets.withFolder.length > 0
  const of = (value: string, totalAmount: string) => t('budgets.page.phone.of', { value: future ? EMPTY : fmt(value), total: fmt(totalAmount) })

  return (
    <div className="flex flex-col gap-3" data-testid="phone-month-view">
      <div className={`${GRID} px-3 text-[11px] uppercase tracking-wide text-muted-foreground`} data-testid="phone-heading">
        <span>{baseCurrency?.code}</span>
        <span className="text-right">{t('budgets.page.budget.structure.tab.budgeted')}</span>
        <span className="text-right">{t('budgets.page.budget.structure.tab.spent')}</span>
      </div>

      {planMonth ? (
        <Card testId="phone-income">
          <button
            type="button"
            data-testid="phone-income-summary"
            aria-expanded={incomeOpen}
            onClick={() => toggleElement(INCOME_FOLD_ID)}
            className="flex min-h-11 w-full items-center gap-2 rounded-md px-2 py-2 text-left active:bg-accent/50"
          >
            {incomeOpen ? <ChevronDown className="size-4.5 shrink-0 text-muted-foreground" /> : <ChevronRight className="size-4.5 shrink-0 text-muted-foreground" />}
            <span className="text-[15px]">
              {t('budgets.page.phone.income_summary', {
                received: future ? EMPTY : fmt(planMonth.income.received),
                planned: fmt(planMonth.income.planned),
              })}
            </span>
          </button>
          {incomeOpen ? planMonth.income.rows.map(incomeRow) : null}
        </Card>
      ) : null}

      {buckets.withFolder.filter((b) => b.elements.length > 0).map((b) => folderCard(b.folder!.id, b.folder!.name, b))}
      {buckets.withoutFolder.elements.length > 0
        ? folderCard('__no_folder__', hasFolders ? t('budgets.page.plan.menu.no_folder') : null, buckets.withoutFolder)
        : null}
      {buckets.uncategorized.elements.length > 0 ? folderCard('__uncategorized__', null, buckets.uncategorized) : null}
      {labels.length > 0 ? (
        <Card testId="phone-labels">
          <button
            type="button"
            aria-expanded={labelsOpen}
            onClick={() => toggleElement(REPORTING_TAGS_FOLD_ID)}
            className="flex min-h-11 w-full items-center gap-2 rounded-md px-2 py-2 text-left text-sm font-medium"
          >
            {labelsOpen ? <ChevronDown className="size-4 shrink-0 text-muted-foreground" /> : <ChevronRight className="size-4 shrink-0 text-muted-foreground" />}
            {t('budgets.page.budget.structure.labels.heading')}
          </button>
          {labelsOpen ? labels.map(labelRow) : null}
        </Card>
      ) : null}
      {buckets.archive.elements.length > 0 ? folderCard('__archive__', t('budgets.page.budget.structure.in_archive'), buckets.archive) : null}

      {savingsRows.length > 0 ? (
        <Card
          testId="phone-savings"
          header={<CardHeader name={t('budgets.page.savings.title')} first={t('budgets.page.savings.planned')} second={t('budgets.page.savings.saved')} />}
        >
          {savingsRows.map(savingsRow)}
        </Card>
      ) : null}

      <section
        className="mb-[max(env(safe-area-inset-bottom),0.75rem)] flex flex-col gap-2 rounded-md border px-3 py-2.5"
        data-testid="phone-totals"
      >
        <span className="text-[15px] font-medium">{t('budgets.page.budget.structure.total.name')}</span>
        <TotalLine testId="phone-total-expenses" label={t('budgets.page.plan.totals.expenses')} value={of(expenseTotals.spent, expenseTotals.budgeted)} />
        {savingsSum ? <TotalLine testId="phone-total-savings" label={t('budgets.page.plan.totals.savings')} value={of(savingsSum.spent, savingsSum.budgeted)} /> : null}
        <TotalLine
          testId="phone-total-available"
          label={t('budgets.page.budget.structure.tab.available')}
          value={fmt(total.available)}
          negative={cmp(total.available, '0') < 0}
        />
        {planMonth ? (
          <>
            <TotalLine testId="phone-total-balance" label={t('budgets.page.phone.balance')} value={fmt(planMonth.balance)} />
            {planMonth.savingsBalance !== null ? (
              <TotalLine testId="phone-total-savings-balance" label={t('budgets.page.plan.totals.savings_balance')} value={fmt(planMonth.savingsBalance)} />
            ) : null}
            {!isZero(planMonth.transfersNet) ? (
              <TotalLine testId="phone-total-transfers" label={t('budgets.page.plan.totals.transfers')} value={fmt(planMonth.transfersNet)} />
            ) : null}
          </>
        ) : null}
      </section>
    </div>
  )
}
```

`common.button.expand.label` is "Expand" in `en`; the chevron test clicks by that name. If the key reads differently, use the real value in the test.

- [ ] **Step 5: Run the tests and check that they pass**

Run: `pnpm vitest run src/features/budgets/PhoneMonthView.test.tsx src/features/budgets/BudgetTable.test.tsx --maxWorkers=2`, then `pnpm exec tsc -b && pnpm lint`.
Expected: PASS (`BudgetTable` is still green after the fold-key move); tsc clean; lint 0 errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/budgets/PhoneMonthView.tsx web/src/features/budgets/PhoneMonthView.test.tsx web/src/features/budgets/budgetStore.ts web/src/features/budgets/BudgetTable.tsx
git commit -m "feat: Phone month list with Budget/Spent rows and a totals card

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: Phone view in `BudgetPage` (both routes) and phone income plans

**Files:**
- Modify: `web/src/features/budgets/BudgetPage.tsx`
- Test: `web/src/features/budgets/BudgetPage.test.tsx` (the mode-switch test moves to a tablet)
- Test: `web/src/features/budgets/BudgetPage.phone.test.tsx` (new)
- Docs: `docs/regression-test-plan.md`

**Interfaces:**
- Consumes:
  - Task 2: `planMonthFigures`, `SheetTarget`.
  - Task 5: the `ElementSheet` host already in `BudgetPage` (with `sheetTarget`, `sheetCanSetAmount`, `sheetCellTarget`) and `SetLimitDialog`'s `title`.
  - Task 7: `PhoneMonthView`.
  - Existing: `useBudgetPlan`, `usePlanSetLimit`.

- [ ] **Step 1: Write the failing page tests**

Create `BudgetPage.phone.test.tsx`:

```tsx
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent, { PointerEventsCheckLevel } from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { delay, http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser, fixtureWireBudget, fixtureWirePlan, planHandler } from '@/test/fixtures'
import { BudgetPage } from './BudgetPage'
import { useBudgetPeriodStore } from './budgetStore'

const userWithBudget = {
  ...fixtureUser,
  options: fixtureUser.options.map((o) => (o.name === 'budget' ? { ...o, value: 'b1' } : o)),
}
const comment = {
  id: 'c1', elementId: 'cat-food', period: '2026-07-01', comment: 'Trip to Lisbon',
  author: { id: 'u1', avatar: 'face:emerald', name: 'Ada' }, createdAt: '2026-07-17 09:00:00', updatedAt: '2026-07-17 09:00:00',
}
const guestBudget = {
  ...fixtureWireBudget,
  meta: {
    ...fixtureWireBudget.meta,
    ownerUserId: 'someone-else',
    access: [
      { user: { id: 'someone-else', avatar: 'face:emerald', name: 'Owner' }, role: 'owner', isAccepted: 1 },
      { user: { id: 'u1', avatar: 'face:emerald', name: 'Me' }, role: 'guest', isAccepted: 1 },
    ],
  },
}

function phone() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: true, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function handlers({ budget = fixtureWireBudget, plan = planHandler() }: { budget?: unknown; plan?: ReturnType<typeof planHandler> } = {}) {
  let setLimitBody: unknown
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: budget } })),
    plan,
    http.get('*/api/v1/budget/get-comment-list', () => HttpResponse.json({ success: true, message: '', data: { items: [comment], truncated: false } })),
    http.post('*/api/v1/budget/set-limit', async ({ request }) => {
      setLimitBody = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  return { setLimitBody: () => setLimitBody }
}

function renderPage(path: '/budget' | '/plan' = '/budget') {
  const router = createMemoryRouter(
    [
      { path: '/budget', element: <BudgetPage key="budget" mode="budget" /> },
      { path: '/plan', element: <BudgetPage key="plan" mode="plan" /> },
    ],
    { initialEntries: [path] },
  )
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  phone()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: {}, foldBudgetId: null, planHideEmpty: false })
})

it('renders the single month view on /budget and on /plan alike', async () => {
  handlers()
  renderPage('/budget')
  expect(await screen.findByTestId('phone-month-view')).toBeInTheDocument()
  expect(screen.queryByTestId('budget-table')).toBeNull()
})

it('/plan on a phone is the same month view, not the plan grid', async () => {
  handlers()
  renderPage('/plan')
  expect(await screen.findByTestId('phone-month-view')).toBeInTheDocument()
  expect(screen.queryByTestId('plan-sheet')).toBeNull()
})

it('has no Budget/Plan switch in the settings menu, and the title is not all caps', async () => {
  handlers()
  const user = userEvent.setup()
  renderPage()
  const title = await screen.findByRole('heading', { name: 'Main budget' })
  expect(title.className).not.toContain('uppercase')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  expect(await screen.findByRole('menuitem', { name: 'Edit structure' })).toBeInTheDocument()
  expect(screen.queryByRole('menuitemradio')).toBeNull()
})

it('row tap → sheet → Set budget replaces the sheet and saves the selected month', async () => {
  const api = handlers()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage()
  await user.click(await screen.findByRole('button', { name: /^Food, budget 200.00/ }))
  const sheet = await screen.findByTestId('element-sheet')
  await user.click(within(sheet).getByRole('button', { name: 'Set budget' }))
  const input = await screen.findByLabelText('Budget')
  expect(screen.queryByTestId('element-sheet')).toBeNull()
  await user.clear(input)
  await user.type(input, '250')
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(api.setLimitBody()).toEqual({ budgetId: 'b1', elementId: 'cat-food', period: '2026-07-01', amount: '250' }))
  await waitFor(() => expect(screen.queryByLabelText('Budget')).toBeNull())
  expect(screen.queryByTestId('element-sheet')).toBeNull()
})

it('sheet → Comments opens the thread as a sheet', async () => {
  handlers()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage()
  await user.click(await screen.findByRole('button', { name: /^Food, budget/ }))
  await user.click(within(await screen.findByTestId('element-sheet')).getByRole('button', { name: 'Comments (1)' }))
  expect(await screen.findByTestId('comments-sheet')).toHaveTextContent('Trip to Lisbon')
  expect(screen.queryByTestId('element-sheet')).toBeNull()
})

it('sheet → Transactions opens the month’s transaction list', async () => {
  handlers()
  server.use(http.get('*/api/v1/budget/get-transaction-list', () => HttpResponse.json({ success: true, message: '', data: { items: [] } })))
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage()
  await user.click(await screen.findByRole('button', { name: /^Food, budget/ }))
  await user.click(within(await screen.findByTestId('element-sheet')).getByRole('button', { name: 'Transactions' }))
  await waitFor(() => expect(screen.queryByTestId('element-sheet')).toBeNull())
  expect(await screen.findByRole('dialog', { name: /Food/ })).toBeInTheDocument()
})

it('a guest’s sheet has no Set budget but still reaches comments', async () => {
  handlers({ budget: guestBudget })
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage()
  await user.click(await screen.findByRole('button', { name: /^Food, budget/ }))
  const sheet = await screen.findByTestId('element-sheet')
  expect(within(sheet).queryByRole('button', { name: 'Set budget' })).toBeNull()
  expect(within(sheet).getByRole('button', { name: 'Comments (1)' })).toBeInTheDocument()
})

it('an income row’s Set plan writes that month’s plan', async () => {
  const api = handlers()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage()
  await user.click(await screen.findByTestId('phone-income-summary'))
  await user.click(await screen.findByRole('button', { name: /^Freelance, planned 500.00/ }))
  await user.click(within(await screen.findByTestId('element-sheet')).getByRole('button', { name: 'Set plan' }))
  expect(await screen.findByRole('dialog', { name: /Set plan/ })).toBeInTheDocument()
  const input = screen.getByLabelText('Budget')
  await user.clear(input)
  await user.type(input, '650')
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(api.setLimitBody()).toEqual({ budgetId: 'b1', elementId: 'cat-freelance', period: '2026-07-01', amount: '650' }))
})

it('keeps the expense list while get-budget-plan is still loading', async () => {
  handlers({
    plan: http.get('*/api/v1/budget/get-budget-plan', async () => {
      await delay('infinite')
      return HttpResponse.json({})
    }) as ReturnType<typeof planHandler>,
  })
  renderPage()
  expect(await screen.findByTestId('phone-row-cat-food')).toBeInTheDocument()
  expect(screen.queryByTestId('phone-income')).toBeNull()
  expect(screen.queryByTestId('phone-total-balance')).toBeNull()
})

it('leaves the plan lines out when get-budget-plan fails', async () => {
  handlers({ plan: http.get('*/api/v1/budget/get-budget-plan', () => HttpResponse.json({ success: false, message: 'x', code: 0, errors: {} }, { status: 500 })) as ReturnType<typeof planHandler> })
  renderPage()
  expect(await screen.findByTestId('phone-row-cat-food')).toBeInTheDocument()
  await waitFor(() => expect(screen.queryByTestId('phone-income')).toBeNull())
})

it('edit structure on a phone still shows the table editor', async () => {
  handlers()
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('phone-month-view')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))
  expect(await screen.findByTestId('budget-table')).toBeInTheDocument()
  expect(screen.queryByTestId('phone-month-view')).toBeNull()
})
```

Notes for the implementer:
- `fixtureWirePlan.months` is May–Aug 2026, and `useBudgetPlan(…, '2026-07-01', 1)` asks for `from=2026-05-01&months=5`. The MSW handler ignores the query and returns the 4-month fixture, and `indexOf('2026-07-01')` is 2, so income shows.
- `planHandler` returns an `http.get` handler. The casts above only satisfy the helper's parameter type; widen the parameter to `HttpHandler` (from `msw`) if that reads better.
- The guest fixture's `access[].user` shape must match `UserDto`. Copy a user from `comments.monthly.test.tsx`'s `guestWireBudget` instead if it already exists there.
- Check the transaction dialog's accessible name in `BudgetTransactionsDialog.tsx`. If it is not titled with the element name, assert on its real title.

- [ ] **Step 2: Run the tests and check that they fail**

Run: `pnpm vitest run src/features/budgets/BudgetPage.phone.test.tsx --maxWorkers=2`
Expected: FAIL. `phone-month-view` is not found, because the page still renders the table on phones.

- [ ] **Step 3: Wire the phone view into `BudgetPage.tsx`**

Imports to add:
- `PhoneMonthView` from `./PhoneMonthView`
- `planMonthFigures` added to the `./phoneMonth` import
- `useBudgetPlan`, `usePlanSetLimit` added to the `./queries` import list

State and hooks. Every hook stays above the early returns.

1. Move `const [editMode, setEditMode] = useState(false)` up to sit before the `useBudgetComments` call.
2. Right after it, add `const phoneView = isPhone && !editMode`.
3. Change the comments query to fetch for the phone view on either route:

```ts
  const { byCell: commentsByCell, truncated: commentsTruncated } = useBudgetComments(mode === 'budget' || phoneView ? budgetId : null, selectedDate, 1)
```

   and update the comment above it: "the monthly view's own one-month window (also the phone view's, on both routes); PlanSheet fetches its own …".

4. Below the existing mutation hooks add:

```ts
  // the phone view's income, Balance and Total savings: the Plan view's own figures
  // for the selected month; null until that month's window has really loaded
  const phonePlan = useBudgetPlan(phoneView ? budgetId : null, selectedDate, 1)
  const planSetLimit = usePlanSetLimit(phonePlan.planKey)
  const planMonth = useMemo(
    () => (phonePlan.data && !phonePlan.isPlaceholderData ? planMonthFigures(phonePlan.data, currencies, selectedDate) : null),
    [phonePlan.data, phonePlan.isPlaceholderData, currencies, selectedDate],
  )
```

5. Widen `limitTarget` so an income plan goes through the plan mutation:

```ts
  const [limitTarget, setLimitTarget] = useState<(CellTarget & { viaPlan?: boolean }) | null>(null)
```

   `budgetId` is `userOption(...)`; if its type is not `Id | null`, coerce with `?? null`, as the comments call already expects.

Sheet helpers (Task 5 added them): the `plan` case of `sheetCanSetAmount` now allows phone income plans:

```ts
      case 'plan':
        return target.cell.element.isArchived === 0 && target.cell.element.id !== UNCATEGORIZED_ID && planMonth !== null
```

Header:
1. Title classes become `isPhone ? 'min-w-0 shrink truncate text-lg font-medium' : 'min-w-0 shrink truncate text-[22px] uppercase tracking-wide'`.
2. The settings-menu radio group renders only when `isCompact && !isPhone`.
3. The "Hide empty rows" checkbox renders only when `mode === 'plan' && !phoneView`.
4. Update the comment on the desktop tablist to say tablets keep the switch in the settings menu and phones have one view.

Body: replace `{mode === 'plan' ? (<PlanSheet …/>) : (<> … </>)}` with a three-way branch. The phone branch comes first:

```tsx
      {phoneView ? (
        <>
          {archived ? <InfoBox>{t('budgets.page.budget.archived_banner')}</InfoBox> : null}
          <PeriodStrip startedAt={budget.meta.startedAt} endedAt={budget.meta.endedAt} />
          {isPlaceholderData || periodSwitching ? (
            <div className="flex flex-1 items-center justify-center" data-testid="budget-loading">
              <CoinLoader label={t('common.app.modal.loading.data_loading')} />
            </div>
          ) : (
            <div ref={tableScrollRef} className="min-h-0 flex-1 overflow-y-auto">
              <PhoneMonthView
                budget={budget}
                buckets={buckets}
                currencies={currencies}
                selectedDate={selectedDate}
                planMonth={planMonth}
                commentsByCell={commentsByCell}
                onOpenSheet={setSheetTarget}
                onShowTransactions={setTransactionsTarget}
              />
            </div>
          )}
        </>
      ) : mode === 'plan' ? (
        <PlanSheet budget={budget} currencies={currencies} userId={user?.id} editMode={editMode} />
      ) : (
        <>{/* the existing budget-mode branch, unchanged */}</>
      )}
```

Dialogs:

1. `SetLimitDialog` becomes:

```tsx
      <SetLimitDialog
        target={limitTarget ? { id: limitTarget.id, name: elementDisplayName(limitTarget.id, limitTarget.name, t), value: limitTarget.budgeted } : null}
        title={limitTarget?.viaPlan ? t('budgets.page.sheet.set_plan') : undefined}
        onClose={() => setLimitTarget(null)}
        onCommit={(elementId, amount) => {
          if (limitTarget?.viaPlan) {
            if (planMonth) {
              planSetLimit.mutate({ budgetId: budget.meta.id, elementId, period: selectedDate, amount, monthIndex: planMonth.index })
            }
            return
          }
          setLimit.mutate({ budgetId: budget.meta.id, elementId, period: selectedDate, amount })
        }}
      />
```

2. In the `<ElementSheet>` Task 5 added, `onSetAmount` marks a plan target so its commit goes through the plan mutation:

```tsx
        onSetAmount={() => {
          if (sheetTarget) {
            setLimitTarget({ ...sheetCellTarget(sheetTarget), viaPlan: sheetTarget.kind === 'plan' })
            setSheetTarget(null)
          }
        }}
```

   Everything else on the sheet stays as Task 5 left it. `month={selectedDate}` and `exchange={makeBudgetExchange(budget, currencies)}` are right for the phone view, because income rows are the selected month.

- [ ] **Step 4: Run the phone tests, then the whole budgets suite**

Run: `pnpm vitest run src/features/budgets/BudgetPage.phone.test.tsx --maxWorkers=2`
Expected: PASS.

Run: `pnpm vitest run src/features/budgets --maxWorkers=2`
Expected: PASS. Existing tests that mock a phone (`matches: true`) and expect the old table or `PlanSheet` now meet the phone view.
- If such a test is about table/plan behaviour that tablets still have, switch it to the tablet mock (`matches: q.includes('1023')`).
- If it asserts the old phone table itself (for example the `budget-totals-mobile` card lines), drop those phone-only assertions; `PhoneMonthView.test.tsx` covers the phone totals.

List every test you changed in the report. Known ones:
- `BudgetPage.test.tsx`'s `compact viewport: the mode switch sits in the settings menu …` now meets a phone. Switch its mock to the tablet form (`matches: q.includes('1023')`) and rename it `tablet viewport: …`. Tablets keep the switch in the menu.
- `BudgetPage.test.tsx`'s `a budget without savings rows renders no block` asserts the old phone Total card test ids. Those still exist for desktop/tablet markup (the card is `sm:hidden`), so leave it unless it fails.

- [ ] **Step 5: Typecheck, lint, i18n guard, metrics coverage**

Run from `web/`: `pnpm exec tsc -b && pnpm lint && pnpm vitest run src/lib/metrics-coverage.test.ts --maxWorkers=2`
Run from the repo root: `GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/test/i18ntest/`
Expected: all green. The i18n guard now also sees the new keys used from `t()` calls.

- [ ] **Step 6: Regression checklist**

In `docs/regression-test-plan.md` §9:
1. Rewrite the comment and savings items that still describe a phone table: the Savings block item's "On a phone it …" sentences (~lines 776–795) and the Total-card phone sentences. On a phone they now refer to the month view's Savings card and Totals card.
2. Add a `### Phone month view 📱` group of items:

```markdown
- [ ] 📱 On a phone (< 640 px, the iOS home-screen PWA included) `/budget` and
      `/plan` show the same single month view: header with the budget name in
      normal case, the month strip, a heading row naming the budget currency
      (e.g. `USD`) above Budget and Spent, no Budget/Plan switch in the
      settings menu.
- [ ] 📱 Each expense row shows Budget and Spent with no currency symbol and a
      thin bar under the name; Spent is neutral within budget, amber when this
      month is over budget but carry-over still covers it, red when Available
      is negative. A future month shows `—` for Spent, with no bar and no colour.
- [ ] 📱 A category/envelope in another currency carries a small code tag
      (`Travel EUR`) and its amounts are in that currency.
- [ ] 📱 Income is one row "Income · received of planned"; tapping it unfolds
      the income rows (Planned / Received), and the fold state survives a
      month switch. The income Uncategorized row appears only in a month that
      received something.
- [ ] 📱 Tapping a row opens its item sheet ("Food · July"): Budget, Spent,
      Available; "Over by … — covered by … left from earlier months" when
      carry-over covers an overspend, "Overspent by …" when Available is
      negative; the latest comment and "Comments (N)" (or "Add comment");
      "Set budget" and "Transactions". A foreign-currency item adds its code to
      every amount, the amount in the budget currency, and the month's average
      rate.
- [ ] 📱 From the sheet, "Set budget", "Comments" and "Transactions" each
      replace the sheet (never stacked); closing them returns to the list.
      Saving a budget updates the row at once and survives a reload.
- [ ] 📱 An income row's sheet offers "Set plan" (Planned / Received); a
      savings row's sheet shows Planned, Saved and Balance at month end.
- [ ] 📱 A guest, a read-only account, an archived budget, a month outside the
      budget's range, an archived element and the Uncategorized row get no
      "Set budget" in the sheet; Uncategorized has no comments link, and an
      empty read-only thread shows none.
- [ ] 📱 The Totals card lists Expenses (spent of budget), Savings (saved of
      planned, only with savings accounts), Available, Balance at month end,
      Total savings (with savings accounts), and Transfers only when money
      crossed the budget boundary that month. Balance and Total savings match
      the Plan view's figures for the same month on a desktop.
- [ ] 📱 Children of an envelope/tag unfold from the chevron; tapping a child
      or a reporting tag opens its transactions directly.
- [ ] 📱 "Edit structure" on a phone still shows the table editor (drag to
      reorder, folder menus); "Done" returns to the month view.
```

- [ ] **Step 7: Commit**

```bash
git add -A web/src docs/regression-test-plan.md
git commit -m "feat: Single month view on phones for Budget and Plan

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---


## Self-review notes

Spec coverage:
- **Part 1, the phone single month view:**
  - Header (normal-case name, no switch): Task 8.
  - Month strip: Task 8.
  - Heading row with the currency code: Task 7.
  - Income summary row: Tasks 2 and 7.
  - Folder cards and "No folder": Task 7.
  - Savings card: Task 7.
  - Totals card: Task 7.
  - Expense row (bar, colour, indicator, chevron vs. row, one button with an accessible name, ≥ 44 px): Task 7.
  - Item sheet: Task 4.
  - Sheets replace one another: Tasks 5 and 8.
  - Removed on phone (tap-Available, tap-Spent, long-press, the mode radio): Tasks 5 and 8.
  - Data (`get-budget`, the plan window, comments): Task 8.
- **Part 2, tablet (revised 2026-09-30):**
  - A tap on an amount opens the item sheet: Budget view and Savings in Task 5, Plan grid in Task 6.
  - The long-press actions modal is removed: Task 5.
  - The marker tap still opens the popover: Tasks 5 and 6.
- **Row state rule:** Task 1.
- **Currency rule on the phone and in the sheet:** Tasks 4 and 7.
- **Removals:** `ElementLongPress`, `onAvailableClick`, `onAvailableCommentsClick`, the phone `renderRowWrapper`, the `SetLimitDialog` comments and `useLongPress`, all in Task 5.
- **i18n:** Task 3. **Testing:** every task. **Regression plan:** Tasks 5, 6 and 8.

Deliberately not in this plan:
- Part 3: desktop density, the currency symbol column on desktop/tablet, "No folder" on desktop, and the uncategorized "Show transactions" on desktop.
- Budget configuration.
