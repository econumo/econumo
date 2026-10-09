# Plan Grid Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the desktop/tablet Plan view the Budget view's structure, look and controls, plus spreadsheet-style editing.

**Architecture:**
- `PlanSheet` keeps its data, selection, fill, clipboard and comments logic, but its window now derives from the shared `selectedDate`.
- It renders through the Budget view's line parts. These are generalized by a column-layout context: three fixed figure columns for Budget, N month columns for Plan.
- The ⋮ menus and their dialogs move out of `BudgetPage` into a shared hook that both views use.
- Plan's own edit mode, row menus, dialogs and sortable wrappers are deleted in favour of the shared parts (`monthLines`, `MonthDrag`).

**Tech Stack:** React 19 + TypeScript, Tailwind 4, Zustand (`budgetStore`), TanStack Query, dnd-kit, vitest + Testing Library + MSW. All work is in `web/`.

**Spec:** `docs/superpowers/specs/2026-10-06-plan-grid-redesign-design.md`. Read it first; it is the source of truth for behaviour.

## Global Constraints

- **Scope:** phone layout untouched. `useIsPhone()` paths render exactly as today, and `PhoneMonthView` is not edited.
- **No backend changes.** A cleared cell is `set-limit` with `amount: null` (already supported by `usePlanSetLimit`).
- **Style:**
  - no `font-semibold`/`font-bold` for hierarchy (regular/medium weights, colour and size only);
  - hairlines (`border-b border-border/60`), no boxed folder cards;
  - red is `text-expense`, only for real problems, never green.
- **Overspend red in the Plan grid** is `isOverspent(type, cell)` from `planMath.ts` (actual > plan, expense side only, never savings). Plan cells carry no carry-over, so `budgetMath.overBudget` (which needs `available`) does not apply. This refines spec §2's "`overBudget()`".
- **Fold keys are shared with the Budget view:** `income`, `savings`, `expense`, folder ids, `__no_folder__`, `archived`.
- **i18n:** every new user-visible string is a key in ALL 11 `locales/<lang>.json` (en is the reference; `internal/test/i18ntest` enforces parity and `t()` coverage). Plural strings use the pipe form.
- **Analytics:** add `BUDGET_PLAN_CLEAR_CELL: 'appBudgetPlanClearCell'` to `METRICS` (`web/src/lib/metrics.ts`). `metrics-coverage.test.ts` must stay green.
- **Comments** follow CLAUDE.md "write sparingly": why, not what. No references to old implementations.
- **Commands** (run from `web/`):
  - `pnpm test <path>` for one file;
  - `pnpm test` for all;
  - `pnpm lint`;
  - `pnpm exec tsc -b --noEmit` for types.
  - Go is only needed for the i18n guard: `cd .. && GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/test/i18ntest/`.
- **Commits:** one per task, message ends with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Branch `feature/plan-grid-redesign` (already checked out). Do not push. The controller pushes.

## Review Focus

1. **Budget starting mid-window:** with the selected month = start month, there is no history column and the selected month is column 1. ← at column 1 does nothing. Test in Task 1 + Task 2.
2. **Typing into a read-only cell** (guest, archived row, month before start) must never open the editor, and Delete must not send `set-limit`. Test in Task 5.
3. **Editor open while the window shifts** (← past column 1 while editing): commit first, then shift. The commit must target the month that was edited, not the new column's month. Test in Task 5.
4. **Narrow screen (1 visible month):** the grid still renders, the selected month is the only column, and → / ← page the month. Test in Task 2.
5. **Budget view regression:** generalizing `monthLines` must not change the Budget view's DOM. The existing `BudgetPage.test.tsx`, `BudgetTable.test.tsx`, `monthLines.test.tsx` and `MonthFlows.drag.test.tsx` must pass unchanged. This is checked in Task 3.

---

### Task 1: Window and cell helpers (pure)

**Files:**
- Modify: `web/src/features/budgets/planMath.ts` (add `planWindow`)
- Create: `web/src/features/budgets/planCell.ts`
- Test: `web/src/features/budgets/planMath.test.ts`, create `web/src/features/budgets/planCell.test.ts`

**Interfaces:**
- Produces:
  - `planWindow(selected: string, visible: number, startedAt: string, endedAt?: string | null): { first: string; selectedCol: number }`
  - `planCellView(input: PlanCellViewInput): PlanCellView`, where
    - `PlanCellViewInput = { type: BudgetElementType; cell: PlanCellDto | undefined; month: string; selected: string }`
    - `PlanCellView = { actual: string | null; plan: string | null; over: boolean }`
  - Both are used by Tasks 2 and 4.

- [ ] **Step 1: Write the failing tests**

Append to `planMath.test.ts`:

```ts
import { planWindow } from './planMath'

describe('planWindow', () => {
  it('puts the selected month in column 2 with one month of history', () => {
    expect(planWindow('2026-10-01', 6, '2025-01-01')).toEqual({ first: '2026-09-01', selectedCol: 1 })
  })
  it('shows only the selected month when one column fits', () => {
    expect(planWindow('2026-10-01', 1, '2025-01-01')).toEqual({ first: '2026-10-01', selectedCol: 0 })
  })
  it('starts at the start month when the history month is before it', () => {
    expect(planWindow('2026-10-01', 6, '2026-10-15')).toEqual({ first: '2026-10-01', selectedCol: 0 })
  })
  it('ends at the end month of an ended budget', () => {
    // Oct selected, 6 columns, budget ends Dec: window Jul..Dec, Oct is column 3
    expect(planWindow('2026-10-01', 6, '2025-01-01', '2026-12-01')).toEqual({ first: '2026-07-01', selectedCol: 3 })
  })
  it('never starts before the start month even when the end pulls it back', () => {
    expect(planWindow('2026-10-01', 6, '2026-09-01', '2026-11-01')).toEqual({ first: '2026-09-01', selectedCol: 1 })
  })
})
```

Create `planCell.test.ts`:

```ts
import { BudgetElementType } from '@/api/dto/budget'
import { planCellView } from './planCell'

const cat = BudgetElementType.CATEGORY
const sel = '2026-10-01'

describe('planCellView', () => {
  it('shows actual and plan up to the selected month', () => {
    expect(planCellView({ type: cat, cell: { actual: '612', planned: '600' }, month: '2026-09-01', selected: sel })).toEqual({ actual: '612', plan: '600', over: true })
    expect(planCellView({ type: cat, cell: { actual: '341', planned: '600' }, month: sel, selected: sel })).toEqual({ actual: '341', plan: '600', over: false })
  })
  it('shows only the plan after the selected month', () => {
    expect(planCellView({ type: cat, cell: { actual: '0', planned: '55' }, month: '2026-11-01', selected: sel })).toEqual({ actual: null, plan: '55', over: false })
  })
  it('leaves an unplanned month blank, never 0', () => {
    expect(planCellView({ type: cat, cell: { actual: '0', planned: '' }, month: '2026-11-01', selected: sel }).plan).toBeNull()
  })
  it('never marks income or savings as over', () => {
    expect(planCellView({ type: BudgetElementType.INCOME_CATEGORY, cell: { actual: '9', planned: '1' }, month: sel, selected: sel }).over).toBe(false)
    expect(planCellView({ type: BudgetElementType.SAVINGS, cell: { actual: '9', planned: '1' }, month: sel, selected: sel }).over).toBe(false)
  })
  it('reads a missing cell as nothing at all', () => {
    expect(planCellView({ type: cat, cell: undefined, month: sel, selected: sel })).toEqual({ actual: null, plan: null, over: false })
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && pnpm test src/features/budgets/planMath.test.ts src/features/budgets/planCell.test.ts`
Expected: FAIL. `planWindow` is not exported, and `./planCell` is not found.

- [ ] **Step 3: Implement**

In `planMath.ts`, add next to `planInitialFirstMonth` (which stays until Task 2 removes its last caller):

```ts
/** The Plan grid's months: the selected month with one month of history before it,
 *  then the future, kept inside the budget's start and end months. */
export function planWindow(selected: string, visible: number, startedAt: string, endedAt?: string | null): { first: string; selectedCol: number } {
  const start = `${startedAt.slice(0, 7)}-01`
  let first = visible > 1 ? addMonths(selected, -1) : selected
  if (endedAt) {
    const lastFirst = addMonths(`${endedAt.slice(0, 7)}-01`, -(visible - 1))
    if (first > lastFirst) {
      first = lastFirst
    }
  }
  if (first < start) {
    first = start
  }
  return { first, selectedCol: Math.max(0, monthDiff(first, selected)) }
}
```

Create `planCell.ts`:

```ts
import type { PlanCellDto } from '@/api/dto/budget'
import { BudgetElementType } from '@/api/dto/budget'
import { isOverspent } from './planMath'

export interface PlanCellViewInput {
  type: BudgetElementType
  cell: PlanCellDto | undefined
  month: string
  /** the month selected in the strip: later months have no actuals worth showing */
  selected: string
}

export interface PlanCellView {
  /** null: no actual is shown in this month */
  actual: string | null
  /** null: nothing planned (blank, never 0) */
  plan: string | null
  over: boolean
}

export function planCellView({ type, cell, month, selected }: PlanCellViewInput): PlanCellView {
  if (!cell) {
    return { actual: null, plan: null, over: false }
  }
  const past = month <= selected
  return {
    actual: past ? cell.actual : null,
    plan: cell.planned === '' ? null : cell.planned,
    over: past && type !== BudgetElementType.SAVINGS && isOverspent(type, cell),
  }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run the same command, then `pnpm exec tsc -b --noEmit`. Expected: PASS, no type errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/budgets/planMath.ts web/src/features/budgets/planMath.test.ts web/src/features/budgets/planCell.ts web/src/features/budgets/planCell.test.ts
git commit -m "feat: Plan window and cell helpers for the shared month strip"
```

---

### Task 2: Shared header, window from the selected month, sticky month row

**Files:**
- Modify: `web/src/features/budgets/budgetStore.ts`
- Modify: `web/src/features/budgets/BudgetPage.tsx`
- Modify: `web/src/features/budgets/PlanSheet.tsx`
- Modify: `web/src/features/budgets/planMath.ts` (delete `planInitialFirstMonth`)
- Test: `web/src/features/budgets/PlanSheet.test.tsx`, `web/src/features/budgets/BudgetPage.test.tsx`

**Interfaces:**
- Consumes: `planWindow` (Task 1).
- Produces:
  - store `stepPeriod(delta: number): void`. It moves `selectedDate` by `delta` months and tracks `METRICS.BUDGET_PLAN_CHANGE_WINDOW`; there is no `BUDGET_CHANGE_DATE` for edge paging.
  - `PlanSheet` props become `{ budget, currencies, userId, editMode }`; `viewSwitch` is removed.
  - `PlanSheet` exposes `data-selected-col` on the month header cells, and `data-testid="plan-month-header"` on the sticky header row.

- [ ] **Step 1: Write the failing tests**

In `PlanSheet.test.tsx`:
- Remove `planFirstMonth: null` from the `beforeEach` `setState`. The fixtures cover May–Aug 2026, and the test clock is 2026-08-15.
- Add these tests:

```ts
it('shares the month strip with the Budget view and centres the window on the selected month', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  await screen.findByTestId('plan-sheet')
  // the strip and the Budget/Plan words are the Budget view's
  expect(screen.getByRole('tablist', { name: 'period' })).toBeInTheDocument()
  expect(screen.getByRole('tab', { name: /plan/i, selected: true })).toBeInTheDocument()
  const header = screen.getByTestId('plan-month-header')
  const cols = within(header).getAllByRole('columnheader')
  // jsdom width 0 -> 3 visible: Jun (history), Jul (selected), Aug
  expect(cols.map((c) => c.getAttribute('data-month'))).toEqual(['2026-06-01', '2026-07-01', '2026-08-01'])
  expect(cols[1]).toHaveAttribute('data-selected-col', 'true')
})

it('moves the selected month when the cursor walks past the last column', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  const grid = await screen.findByTestId('plan-sheet')
  grid.focus()
  // select the first row's last month column, then step right off the edge
  fireEvent.keyDown(grid, { key: 'ArrowDown' })
  fireEvent.keyDown(grid, { key: 'ArrowRight' })
  fireEvent.keyDown(grid, { key: 'ArrowRight' })
  fireEvent.keyDown(grid, { key: 'ArrowRight' })
  await waitFor(() => expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-08-01'))
  expect(trackEvent).toHaveBeenCalledWith(METRICS.BUDGET_PLAN_CHANGE_WINDOW)
})

it('keeps the selected month when switching from Plan to Budget', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('tab', { name: /budget/i }))
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-06-01')
})
```

Then delete or update every existing test that clicks `budgets.page.plan.nav.prev`/`next` ("prev"/"next" aria labels) or sets `planFirstMonth`:
- Replace a `planFirstMonth: 'X'` setup with `selectedDate: addMonths('X', 1)`, since the history column is the window start.
- Replace a nav-button click with `useBudgetPeriodStore.getState().stepPeriod(±1)` inside `act()`.

Run `grep -n "planFirstMonth\|nav.prev\|nav.next\|'prev'\|'next'" src/features/budgets/*.test.tsx` to find them all.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && pnpm test src/features/budgets/PlanSheet.test.tsx`
Expected: the new tests FAIL (there is no period tablist on /plan and no `plan-month-header`).

- [ ] **Step 3: Implement**

`budgetStore.ts`:
- Delete `planFirstMonth` / `setPlanFirstMonth` from the state interface, the initializer and the `persist` `partialize` (if listed). A stale persisted `planFirstMonth` is then simply ignored.
- Add:

```ts
  /** the Plan grid's cursor walked off an edge: the window (and the strip) move one month */
  stepPeriod: (delta: number) => void
```

```ts
      stepPeriod: (delta) => {
        trackEvent(METRICS.BUDGET_PLAN_CHANGE_WINDOW)
        set((s) => ({ selectedDate: addMonthsToPeriod(s.selectedDate, delta) }))
      },
```

with a local helper (the store must not import `planMath`, to avoid a cycle via `budgetMath`):

```ts
function addMonthsToPeriod(period: string, delta: number): string {
  const [y, m] = period.split('-').map(Number)
  const d = new Date(y, m - 1 + delta, 1)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-01`
}
```

`BudgetPage.tsx`, in the `mode === 'plan' && !isPhone` branch, render the archived banner and the strip exactly as the Budget branch does, then the sheet:

```tsx
      ) : mode === 'plan' && !isPhone ? (
        <>
          {archived ? <InfoBox>{t('budgets.page.budget.archived_banner')}</InfoBox> : null}
          <PeriodStrip startedAt={budget.meta.startedAt} endedAt={budget.meta.endedAt} leading={viewSwitch} />
          <PlanSheet budget={budget} currencies={currencies} userId={user?.id} editMode={editMode} />
        </>
      ) : (
```

`PlanSheet.tsx`:
- Remove the `viewSwitch` prop; remove `persisted`/`setPlanFirstMonth`; remove `clampFirstMonth` and `planInitialFirstMonth` imports. Delete `planInitialFirstMonth` from `planMath.ts`. `clampFirstMonth` stays if other code uses it (grep); otherwise delete it with its tests.
- Derive the window:

```ts
  const selectedDate = useBudgetPeriodStore((s) => s.selectedDate)
  const stepPeriod = useBudgetPeriodStore((s) => s.stepPeriod)
  const { first: firstMonth, selectedCol } = planWindow(selectedDate, visible, budget.meta.startedAt, budget.meta.endedAt)
```

- Replace every `setPlanFirstMonth(addMonths(firstMonth, ±1))` in `handleKeyDown` with `stepPeriod(±1)`:
  - ArrowLeft at col -1 when `!atStart`;
  - ArrowRight past the last column, also clamped at an ended budget's end month (`!atEnd`, where `atEnd = endedAt && addMonths(firstMonth, visible - 1) >= endMonth`).
  - The selection's `col` stays as it is: the window moves by one, so the same column now shows the neighbouring month. That matches spreadsheet scrolling.
- Delete the Plan-only prev/next buttons and the `{viewSwitch}` slot in the header row. The header row becomes a sticky month row inside the scroller (move it into the `role="grid"` div as its first child so it shares the scroller's width):

```tsx
        <div
          role="row"
          data-testid="plan-month-header"
          className="sticky top-0 z-20 grid items-center border-b bg-background"
          style={{ gridTemplateColumns: gridCols }}
        >
          <span />
          {visibleMonths.map((m, i) => (
            <div
              key={m}
              role="columnheader"
              data-month={m}
              data-col={i}
              data-selected-col={i === selectedCol ? 'true' : undefined}
              className={`px-2 py-1.5 text-right text-[10.5px] uppercase tracking-wider ${i === selectedCol ? 'bg-accent/40 text-foreground' : 'text-muted-foreground'}`}
            >
              {monthLabel(m)}
            </div>
          ))}
          <span />
        </div>
```

  (Task 4 replaces `gridCols` with the shared layout. Keep the grid here so this task stays small.) The keyboard scroll-into-view effect must measure against this sticky header too: subtract its height from `view.top` the same way it subtracts the sticky balance footer at the bottom.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && pnpm test src/features/budgets/` and `pnpm exec tsc -b --noEmit`.
Expected: PASS, and no type errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/budgets
git commit -m "feat: Plan view shares the Budget view's month strip; the window follows the selected month"
```

---

### Task 3: Column layout context for the shared line parts

**Files:**
- Modify: `web/src/features/budgets/monthLayout.ts`
- Modify: `web/src/features/budgets/monthLines.tsx`
- Modify: `web/src/features/budgets/MonthFlows.tsx` (move `TotalLine` into `monthLines.tsx`, re-import)
- Test: `web/src/features/budgets/monthLines.test.tsx`

**Interfaces:**
- Produces (used by Tasks 4, 6 and 7):
  - In `monthLayout.ts`:
    - `export type LineLayout = { kind: 'budget' } | { kind: 'plan'; cols: number; selectedCol: number }`
    - `export const LineLayoutContext = createContext<LineLayout>({ kind: 'budget' })`
    - `export const useLineLayout = () => useContext(LineLayoutContext)`
    - `export const PLAN_NAME_COL = 'flex w-52 shrink-0 min-w-0 items-center gap-2'` (13rem = 208px ≈ `PLAN_NAME_COL_PX` 210)
    - `export const PLAN_FIGURE_COL = 'flex min-w-0 flex-1 basis-0 items-baseline justify-end gap-1 px-2 text-right tabular-nums'`
    - `export const PLAN_SELECTED_TINT = 'bg-accent/40'`
  - In `monthLines.tsx`: `export function FigureCells({ cells }: { cells: ReactNode[] })`. It renders `cells` in the current layout's columns:
    - **budget:** exactly 3 cells in `FIRST_COL`/`SECOND_COL`/`THIRD_COL`, byte-identical to today's markup;
    - **plan:** one `PLAN_FIGURE_COL` span per cell; the cell at `selectedCol` adds `PLAN_SELECTED_TINT`. Each span carries `data-col={i}`.
  - `MonthSectionHeader` props: `headings: ReactNode[]`, `sums: ReactNode[]` (budget passes 3).
    - In the budget layout, open shows headings and folded shows sums (unchanged).
    - In the plan layout it always shows `sums`, with no uppercase heading styling.
  - `FolderLine` props: `sums: ReactNode[] | null`. In the plan layout, `null` renders `cols` dashes.
  - `TotalLine` exported from `monthLines.tsx` with props `{ testId, label, values: ReactNode[], strong?, negative?: boolean | boolean[], actionsColumn }`.
    - Budget layout: `values.length === 1`, rendered in `THIRD_COL` as today.
    - Plan layout: rendered through `FigureCells`, with `negative[i]` colouring per column.
  - Plan-layout name column: when `useLineLayout().kind === 'plan'`, `MonthSectionHeader`, `FolderLine` and `TotalLine` use `PLAN_NAME_COL` instead of `NAME_COL` (`flex-1`), so month columns line up under the sticky header.

- [ ] **Step 1: Write the failing tests**

Append to `monthLines.test.tsx`:

```tsx
import { FolderLine, MonthSectionHeader, TotalLine } from './monthLines'
import { LineLayoutContext } from './monthLayout'

function inPlan(node: ReactNode, cols = 3, selectedCol = 1) {
  return render(<LineLayoutContext.Provider value={{ kind: 'plan', cols, selectedCol }}>{node}</LineLayoutContext.Provider>)
}

it('plan layout: a section line shows its per-month sums while open, never headings', () => {
  inPlan(<MonthSectionHeader foldKey="expense" label="Expenses" headings={['A', 'B', 'C']} sums={['1', '2', '3']} actionsColumn={false} testId="sec" />)
  const line = screen.getByTestId('sec')
  expect(within(line).getByText('2')).toBeInTheDocument()
  expect(within(line).queryByText('B')).not.toBeInTheDocument()
})

it('plan layout: the selected month column is tinted on every line', () => {
  inPlan(<FolderLine name="Daily" folded={false} onToggle={() => {}} sums={['1', '2', '3']} actionsColumn={false} />)
  const cells = document.querySelectorAll('[data-col]')
  expect(cells).toHaveLength(3)
  expect(cells[1].className).toContain('bg-accent/40')
  expect(cells[0].className).not.toContain('bg-accent/40')
})

it('plan layout: an empty folder shows one dash per month', () => {
  inPlan(<FolderLine name="Empty" folded={false} onToggle={() => {}} sums={null} actionsColumn={false} />, 4, 0)
  expect(screen.getAllByText('—')).toHaveLength(4)
})

it('plan layout: a totals line colours each negative month on its own', () => {
  inPlan(<TotalLine testId="bal" label="Balance" values={['5', '-3', '1']} negative={[false, true, false]} actionsColumn={false} />)
  expect(screen.getByText('-3').closest('[data-col]')!.className).toContain('text-expense')
  expect(screen.getByText('5').closest('[data-col]')!.className).not.toContain('text-expense')
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && pnpm test src/features/budgets/monthLines.test.tsx`
Expected: FAIL. `LineLayoutContext` and `TotalLine` are not exported.

- [ ] **Step 3: Implement**

Add the context and constants above to `monthLayout.ts`. In `monthLines.tsx`:

```tsx
export function FigureCells({ cells }: { cells: ReactNode[] }) {
  const layout = useLineLayout()
  if (layout.kind === 'budget') {
    return (
      <>
        <span className={FIRST_COL}>{cells[0]}</span>
        <span className={SECOND_COL}>{cells[1]}</span>
        <span className={THIRD_COL}>{cells[2]}</span>
      </>
    )
  }
  return (
    <>
      {cells.map((cell, i) => (
        <span key={i} data-col={i} className={`${PLAN_FIGURE_COL} ${i === layout.selectedCol ? PLAN_SELECTED_TINT : ''}`}>
          {cell}
        </span>
      ))}
    </>
  )
}
```

- Rewrite `MonthSectionHeader` and `FolderLine` to render their figure spans through `FigureCells`, and to pick `PLAN_NAME_COL` vs `NAME_COL` (and drop the open-state uppercase heading style) from `useLineLayout()`.
- Keep the budget-layout markup identical: `FolderLine`'s `<span data-testid=… className="contents">` wrapper stays around the cells.
- Move `TotalLine` from `MonthFlows.tsx` into `monthLines.tsx` with the new `values`/`negative` props. In the budget layout it renders `values[0]` in `THIRD_COL` exactly as before.
- Update `MonthFlows.tsx`'s `MonthTotalsLines` call sites to `values={[x]}`.
- Update every Budget-view caller of `MonthSectionHeader`/`FolderLine` only where TypeScript now complains (tuple → array is assignable, so most need no change).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && pnpm test src/features/budgets/` and `pnpm exec tsc -b --noEmit`.
Expected: PASS, including every existing Budget-view test unchanged (Review Focus 5).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/budgets
git commit -m "refactor: Shared budget lines take their figure columns from a layout context"
```

---

### Task 4: Plan rows and cells in the Budget view's style

**Files:**
- Create: `web/src/features/budgets/PlanRows.tsx`. `ElementRow`/`ChildRow`/cell move here from `PlanSheet.tsx`, together with `GridCtx`, `PlanLimitTarget`, `cellDomId`, `isEditableCell`, `sourceAmount` and `renderActual` (export what `PlanSheet` still needs).
- Modify: `web/src/features/budgets/PlanSheet.tsx`
- Test: `web/src/features/budgets/PlanSheet.test.tsx`, `web/src/features/budgets/PlanSheet.savings.test.tsx`, `web/src/features/budgets/comments.plan*.test.tsx`

**Interfaces:**
- Consumes:
  - `planCellView` (Task 1);
  - `LineLayoutContext`, `PLAN_NAME_COL`, `PLAN_FIGURE_COL`, `PLAN_SELECTED_TINT`, `FigureCells` (Task 3);
  - `LINE`, `ROW_INDENT`, `CHILD_INDENT`, `FOLDER_INDENT`, `RowLevelContext`, `CurrencyTag`, `Dash` (existing).
- Produces:
  - `GridCtx` gains `selectedCol: number`, `selected: string` (the selected month) and `openTransactions(el: PlanElementDto, month: string): void`.
  - It loses `gridCols` and the `editMode`-only callbacks `onChangeCurrency`, `onMoveToFolder`, `onEditEnvelope`, `onDeleteEnvelope`, `onRenameFolder`, `onDeleteFolder` and `canDeleteEnvelopes`. Task 6 wires menus through `rowMenu`/`childMenu` instead.
  - Cells keep these test ids: `plan-cell-<id>:<col>` on the gridcell, `cell-actual`, `cell-planned`, `fill-handle`.

Layout of a row (one line, in the Budget view's `LINE` + `ROW_INDENT[level]` classes, `min-h-9 py-1`, `border-b border-border/60`, `hover:bg-accent/50`):

```
[PLAN_NAME_COL: chevron? icon name CurrencyTag? ][⋮ hover][cell 0][cell 1 tinted]...[cell n-1]
```

- [ ] **Step 1: Write the failing tests**

Add to `PlanSheet.test.tsx`:

```ts
it('a row is one line: actual · plan up to the selected month, plan alone after it', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  await screen.findByTestId('plan-sheet')
  // fixture pe1 (expense): Jun = history, Jul = selected, Aug = future
  const jun = screen.getByTestId('plan-cell-pe1:0')
  const aug = screen.getByTestId('plan-cell-pe1:2')
  expect(within(jun).getByTestId('cell-actual')).toBeInTheDocument()
  expect(within(jun).getByTestId('cell-planned')).toBeInTheDocument()
  expect(within(aug).queryByTestId('cell-actual')).not.toBeInTheDocument()
  // no currency symbol column, no savings balance line
  expect(screen.queryByText('$')).not.toBeInTheDocument()
  expect(screen.queryByTestId('cell-closing')).not.toBeInTheDocument()
})

it('an unplanned month is blank, and only an over-plan actual is red', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  await screen.findByTestId('plan-sheet')
  for (const actual of screen.getAllByTestId('cell-actual')) {
    expect(actual.className).not.toContain('text-income')
  }
  for (const planned of screen.getAllByTestId('cell-planned')) {
    expect(planned.textContent).not.toBe('0.00')
  }
})

it('clicking an actual opens that row and month in the transactions list', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  await user.click(within(screen.getByTestId('plan-cell-pe1:1')).getByTestId('cell-actual'))
  expect(await screen.findByRole('dialog')).toBeInTheDocument()
})
```

Fixture: `pe1` has Aug (`plan-cell-pe1:2`) unplanned, so also assert `expect(within(aug).getByTestId('cell-planned')).toHaveTextContent(/^$/)`. `pe1` Jul is 45 of 250 (not over); `cat-food` Jun is 130 of 150.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && pnpm test src/features/budgets/PlanSheet.test.tsx`
Expected: the new tests FAIL (the future cell still shows an actual, and the `$` column is present).

- [ ] **Step 3: Implement**

Move the row parts into `PlanRows.tsx` and restyle:
- Wrap the grid body in `<LineLayoutContext.Provider value={{ kind: 'plan', cols: visible, selectedCol }}>`.
- **Name cell:** `<div role="gridcell" id={cellDomId(rk,-1)} className={PLAN_NAME_COL}>`. Inside:
  - the chevron, if `children.length > 0` or the row is an envelope, using the same chevron button as today;
  - `EntityIcon text-lg text-muted-foreground`;
  - the name in `text-[15px]`;
  - `CurrencyTag code` when `el.currencyId !== budget currency`. `GridCtx` needs `baseCurrencyId`.
  - Uncategorized gets the folder-like style: `FOLDER_INDENT`, `text-sm text-muted-foreground`, no icon.
- Then `<RowMenu name={displayName} actions={ctx.rowMenu?.(el)} />`. `rowMenu` is optional in `GridCtx` and stays undefined until Task 6.
- **Month cell** (`PLAN_FIGURE_COL` + tint at `selectedCol`), keeping `role="gridcell"`, `id`, `aria-selected`, `aria-label`, `data-month`, `data-col`, `data-testid`, `COMMENT_ANCHOR_ATTR`, `CellShell`, `CommentMarker`, the fill handle and the selection ring. Content from `planCellView({ type: el.type, cell, month: m, selected: ctx.selected })`:

```tsx
{view.actual !== null ? (
  <button
    type="button"
    data-testid="cell-actual"
    title={t('budgets.page.budget.structure.element.action.show_transactions')}
    className={`text-xs tabular-nums underline-offset-2 hover:underline ${view.over ? 'text-expense' : 'text-muted-foreground'}`}
    onClick={(e) => { e.stopPropagation(); ctx.openTransactions(el, m) }}
    disabled={el.id === UNCATEGORIZED_ID && isIncomeType(el.type)}
  >
    {fmt(view.actual)}
  </button>
) : null}
{view.actual !== null ? <span aria-hidden="true" className="text-xs text-muted-foreground/60">·</span> : null}
<span data-testid="cell-planned" className="text-[15px] tabular-nums">
  {view.plan !== null ? fmt(view.plan) : ''}
</span>
```

  - On desktop the planned figure is plain text here; the `LimitEditor` is no longer rendered in the grid. Task 5 adds the in-cell editor, and until then Enter keeps working through the `SetLimitDialog` (`setPlanLimitTarget`), so editing stays reachable.
  - In `handleEnter`, replace the "click the LimitEditor trigger" code with `setPlanLimitTarget({ el: entry.el, month, monthIndex: idx })`.
- **Hover outline:** the hovered cell gets `outline outline-1 outline-border` (row-local `hoverCol` state already exists).
- **Width fallback:** when the measured cell width is below 150px, past-month actuals are not rendered (only the plan). Compute `const showActuals = width === 0 || (width - 208) / visible >= 150` in `PlanSheet` and pass it in `GridCtx`. jsdom width is 0, so actuals show there.
- **Child rows:** `CHILD_INDENT[level]`, `text-sm text-muted-foreground`, one `PLAN_FIGURE_COL` per month with the actual only (months up to `selected`), no plan.
- **Savings rows:** the same as other rows; delete the `cell-closing` line.
- **Remove:** the `$` tail track (`PLAN_CURRENCY_COL_PX`/`PLAN_ACTIONS_COL_PX` and `gridCols`); `planVisibleCount(width)` loses its `editMode` param and tail term (update `planMath.test.ts`); `isUnderspent` usage (delete `isUnderspent` and its tests if unused elsewhere); `HiddenRowsNotice`, `hideEmpty`, `revealedSections` and `planHideEmpty` usage (the store flag goes too, together with its `setState` in tests).
- **`openTransactions`** already exists in `PlanSheet`; pass it in `GridCtx`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && pnpm test src/features/budgets/` and `pnpm exec tsc -b --noEmit`.
Expected: PASS. Port the existing assertions that relied on removed markup:
- `cell-closing` tests in `PlanSheet.savings.test.tsx`: assert the Total savings line instead (Task 4b).
- `$` symbol tests.
- `text-income` underspend tests: delete them, since the spec drops green.
- `limit <name>` trigger clicks: switch to Enter → `SetLimitDialog` for now.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/budgets
git commit -m "feat: Plan rows are one line in the Budget view's style: actual · plan"
```

---

### Task 4b: Section and folder lines, totals block, sticky Balance

**Files:**
- Modify: `web/src/features/budgets/planMath.ts` (add `planGroupSums`)
- Modify: `web/src/features/budgets/PlanSheet.tsx`
- Create: `web/src/features/budgets/PlanTotalsLines.tsx`. `PlanTotals`, `PlanBalanceRow` and `PlanBalanceLine` move here, rebuilt on `TotalLine`.
- Test: `web/src/features/budgets/planMath.test.ts`, `web/src/features/budgets/PlanSheet.test.tsx`, `web/src/features/budgets/PlanSheet.savings.test.tsx`

**Interfaces:**
- Consumes: `MonthSectionHeader`, `FolderLine`, `TotalLine`, `FigureCells` (Task 3); `planCellView` (Task 1).
- Produces: `planGroupSums(rows: PlanElementDto[], months: string[], monthIndex: (m: string) => number, ex: MonthExchange): { actual: string; planned: string }[]`. It returns one entry per visible month, in budget currency, with archived rows skipped.

- [ ] **Step 1: Write the failing tests**

`planMath.test.ts`:

```ts
import { planGroupSums } from './planMath'

it('planGroupSums adds actual and plan per month in budget currency, skipping archived rows', () => {
  const el = (id: string, cells: { actual: string; planned: string }[], isArchived: 0 | 1 = 0) =>
    ({ id, type: BudgetElementType.CATEGORY, name: id, icon: '', currencyId: 'usd', isArchived, folderId: null, position: 0, ownerUserId: null, cells, children: [] })
  const months = ['2026-06-01', '2026-07-01']
  const ex: MonthExchange = (_from, amount) => amount
  const sums = planGroupSums(
    [el('a', [{ actual: '10', planned: '20' }, { actual: '1', planned: '' }]), el('b', [{ actual: '5', planned: '5' }, { actual: '0', planned: '7' }]), el('z', [{ actual: '99', planned: '99' }, { actual: '99', planned: '99' }], 1)],
    months,
    (m) => months.indexOf(m),
    ex,
  )
  expect(sums).toEqual([{ actual: '15', planned: '25' }, { actual: '1', planned: '7' }])
})
```

`PlanSheet.test.tsx`:

```ts
it('section and folder lines carry per-month sums whether open or folded', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const expenses = screen.getByTestId('plan-section-line-expense')
  expect(within(expenses).getAllByTestId(/^plan-sum-/)).toHaveLength(3)
  await user.click(within(expenses).getByRole('button', { expanded: true }))
  expect(within(screen.getByTestId('plan-section-line-expense')).getAllByTestId(/^plan-sum-/)).toHaveLength(3)
})

it('totals: Income, Expenses, Savings, Total savings, then a sticky Balance; Transfers only when non-zero', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  await screen.findByTestId('plan-sheet')
  const lines = within(screen.getByTestId('plan-totals')).getAllByTestId(/^plan-total-/).map((l) => l.getAttribute('data-testid'))
  expect(lines[0]).toBe('plan-total-income')
  expect(lines[1]).toBe('plan-total-expenses')
  expect(within(screen.getByTestId('plan-balance-row')).getByTestId('plan-total-balance')).toBeInTheDocument()
})
```

(Adjust the transfers assertion to the fixture. If the fixture has transfers, assert `plan-total-transfers` is present; also add a case with zero transfers via `planHandler` override and assert it is absent.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && pnpm test src/features/budgets/planMath.test.ts src/features/budgets/PlanSheet.test.tsx`
Expected: FAIL (`planGroupSums` is undefined, and there are no `plan-section-line-*` test ids).

- [ ] **Step 3: Implement**

`planGroupSums` in `planMath.ts`:

```ts
export function planGroupSums(
  rows: PlanElementDto[],
  months: string[],
  monthIndex: (m: string) => number,
  ex: MonthExchange,
): { actual: string; planned: string }[] {
  return months.map((m) => {
    const i = monthIndex(m)
    let actual = '0'
    let planned = '0'
    if (i >= 0) {
      for (const el of rows) {
        const cell = el.isArchived === 0 ? el.cells[i] : undefined
        if (cell) {
          actual = add(actual, ex(el.currencyId, cell.actual, i))
          planned = add(planned, ex(el.currencyId, cell.planned === '' ? '0' : cell.planned, i))
        }
      }
    }
    return { actual, planned }
  })
}
```

In `PlanSheet.tsx`, replace `SectionHeader` with `MonthSectionHeader`:
- `testId` `plan-section-line-<income|savings|expense>`, `foldKey` as today.
- `label` from the existing section keys.
- `headings={[]}`.
- `sums` = `planGroupSums(sectionElements, visibleMonths, monthIndex, ex)` mapped to a `SumCell`:

```tsx
function SumCell({ index, sum, month, selected, fmt }: { index: number; sum: { actual: string; planned: string }; month: string; selected: string; fmt: (v: string) => string }) {
  return (
    <span data-testid={`plan-sum-${index}`} className="text-sm text-muted-foreground">
      {month <= selected ? `${fmt(sum.actual)} · ` : ''}
      {isZero(sum.planned) ? '' : fmt(sum.planned)}
    </span>
  )
}
```

- Section elements are: income = income folders' rows + loose + uncategorized; savings = `savingsRows`; expense = expense folders' rows + loose + uncategorized.
- Replace the boxed `FolderRows` header with `FolderLine` (`name`, `folded`, `onToggle`, `sums` from that folder's rows, or `null` when empty). Keep `role="row"`, the folder selection (`cellDomId(folderRowKey(id), -1)`, `aria-selected`) and the click-to-select by wrapping `FolderLine` in a `<div role="row" …>` with the existing click handler. Delete the `rounded-md border p-1.5` box.
- Indents: rows inside a folder render under `RowLevelContext.Provider value="in-folder"`; loose, uncategorized, savings and archived rows render under `"top"`, as `BudgetTable` does.
- Spacing: sections are separated by hairlines (`border-t` on the section line), not `mt-6`.

`PlanTotalsLines.tsx`:
- Order: Income (`effectiveIncome`), Expenses (`effectiveExpense`), Savings (`effectiveSavings`, only with savings rows), Transfers (`transfersNet`, only when any visible month's `transfersIn` or `transfersOut` is non-zero; keeps the existing drill-down button and tooltip), Total savings (`savingsBalance`, only when `planHasSavingsData`).
- Each is a `TotalLine` with `testId="plan-total-<key>"` and `values` per visible month.
- Then the sticky `<div role="rowgroup" className="sticky bottom-0 z-10 border-t bg-background" data-testid="plan-balance-row">` holds the single Balance `TotalLine` (`plan-total-balance`, `strong`, `negative` per month). It shows `everydayBalance` when savings data exists, else `balance` (labels unchanged: `budgets.page.plan.totals.balance`).
- The old sticky "Savings balance" line moves up as "Total savings": reuse key `budgets.page.plan.totals.savings_balance` (it already reads "Total savings" in en; verify, and otherwise add a key in all 11 locales).
- Keep the test ids the existing tests use where they still apply (`plan-balance-<i>` → now inside `plan-total-balance`; update the tests).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && pnpm test src/features/budgets/` and `pnpm exec tsc -b --noEmit`.
Expected: PASS after porting the totals and balance assertions in `PlanSheet.test.tsx` and `PlanSheet.savings.test.tsx` to the new test ids.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/budgets locales
git commit -m "feat: Plan sections and folders as Budget-view lines with per-month sums; totals lines"
```

---

### Task 5: In-cell editing, spreadsheet keys, Delete clears

**Files:**
- Create: `web/src/features/budgets/PlanCellInput.tsx`
- Modify: `web/src/features/budgets/PlanRows.tsx`, `web/src/features/budgets/PlanSheet.tsx`, `web/src/lib/metrics.ts`
- Test: create `web/src/features/budgets/PlanSheet.editing.test.tsx` (use `PlanSheet.test.tsx`'s setup: copy its imports, mocks, `renderPage`, `usePlanHandlers` and `beforeEach`)

**Interfaces:**
- Consumes: `limitAmountFromInput` (existing), `GridCtx` (Task 4).
- Produces:
  - `PlanCellInput({ initial, replace, label, onCommit, onCancel }: { initial: string; replace: boolean; label: string; onCommit: (raw: string, move: CellMove) => void; onCancel: () => void })`
  - `type CellMove = 'down' | 'right' | 'left' | 'up' | 'none'`
  - `GridCtx.editing: { rowKey: string; col: number; initial: string; replace: boolean } | null`
  - `GridCtx.startEdit(rowKey: string, col: number, opts: { replace: boolean; text?: string }): void`

Behaviour (spec §3):

| Grid state | Key | Effect |
|---|---|---|
| selected editable month cell | digit, `-`, `.`, `,` | open editor, value = that char (`replace`) |
| selected editable month cell | F2, Enter, double-click | open editor with current plan (`normalizeNumber(planned)`, `''` when unset), caret at end |
| selected editable month cell | Delete, Backspace | `commit(el, month, idx, null)`; `trackEvent(METRICS.BUDGET_PLAN_CLEAR_CELL)` on success; skip when already unset |
| editing | Enter / Tab / Shift+Tab / ↑ / ↓ | commit (if valid and changed), close, move down/right/left/up (edge rule: right past last column → `stepPeriod(1)`, left past column 0 → column -1, as arrows do) |
| editing | ← / → | caret (input default) |
| editing | Esc | close, no write |
| editing | blur (click elsewhere) | commit if valid and changed, close |
| editing, invalid | Enter/Tab/arrows | stay open, `aria-invalid`, error text `t('common.validation.invalid_formula')` below the input (absolute, `text-xs text-expense`) |
| read-only cell | any of the above | nothing (no editor, no request); Enter on a read-only cell opens its comments as today's button did? No — Enter does nothing; comments stay on Shift+Enter/Shift+F2 and the marker |

On touch (`isCompact`), none of this applies: tap still opens the item sheet, and Enter opens it too, as today.

- [ ] **Step 1: Write the failing tests** (`PlanSheet.editing.test.tsx`)

```ts
async function gridAtFirstExpenseCell() {
  const grid = await screen.findByTestId('plan-sheet')
  const cell = screen.getByTestId('plan-cell-pe1:1')
  fireEvent.click(cell)
  return { grid, cell }
}

it('typing a digit edits the cell in place and Enter commits and moves down', async () => {
  usePlanHandlers()
  const calls: unknown[] = []
  server.use(http.post('*/api/v1/budget/set-limit', async ({ request }) => {
    calls.push(await request.json())
    return HttpResponse.json({ success: true, message: '', data: {} })
  }))
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  const { grid } = await gridAtFirstExpenseCell()
  fireEvent.keyDown(grid, { key: '7' })
  const input = await screen.findByRole('textbox', { name: /pe1|limit/i })
  expect(input).toHaveValue('7')
  fireEvent.change(input, { target: { value: '75' } })
  fireEvent.keyDown(input, { key: 'Enter' })
  await waitFor(() => expect(calls).toEqual([expect.objectContaining({ elementId: 'pe1', period: '2026-07-01', amount: '75' })]))
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  // the selection moved one row down
  expect(screen.getByTestId('plan-cell-pe1:1')).toHaveAttribute('aria-selected', 'false')
})

it('F2 edits the current value; Esc cancels without a request', async () => {
  usePlanHandlers()
  const spy = vi.fn()
  server.use(http.post('*/api/v1/budget/set-limit', () => { spy(); return HttpResponse.json({ success: true, message: '', data: {} }) }))
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  const { grid } = await gridAtFirstExpenseCell()
  fireEvent.keyDown(grid, { key: 'F2' })
  const input = await screen.findByRole('textbox')
  expect((input as HTMLInputElement).value).not.toBe('')
  fireEvent.keyDown(input, { key: 'Escape' })
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  expect(spy).not.toHaveBeenCalled()
})

it('Tab commits and moves right; an unchanged value sends nothing', async () => {
  usePlanHandlers()
  const spy = vi.fn()
  server.use(http.post('*/api/v1/budget/set-limit', () => { spy(); return HttpResponse.json({ success: true, message: '', data: {} }) }))
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  const { grid } = await gridAtFirstExpenseCell()
  fireEvent.keyDown(grid, { key: 'Enter' })
  const input = await screen.findByRole('textbox')
  fireEvent.keyDown(input, { key: 'Tab' })
  expect(spy).not.toHaveBeenCalled()
  expect(screen.getByTestId('plan-cell-pe1:2')).toHaveAttribute('aria-selected', 'true')
})

it('an invalid value keeps the editor open with a message', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  const { grid } = await gridAtFirstExpenseCell()
  fireEvent.keyDown(grid, { key: '1' })
  const input = await screen.findByRole('textbox')
  fireEvent.change(input, { target: { value: '1+*' } })
  fireEvent.keyDown(input, { key: 'Enter' })
  expect(input).toHaveAttribute('aria-invalid', 'true')
  expect(screen.getByRole('textbox')).toBeInTheDocument()
})

it('Delete clears a planned cell and tracks it', async () => {
  usePlanHandlers()
  const calls: unknown[] = []
  server.use(http.post('*/api/v1/budget/set-limit', async ({ request }) => {
    calls.push(await request.json())
    return HttpResponse.json({ success: true, message: '', data: {} })
  }))
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  const { grid } = await gridAtFirstExpenseCell()
  fireEvent.keyDown(grid, { key: 'Delete' })
  await waitFor(() => expect(calls).toEqual([expect.objectContaining({ elementId: 'pe1', amount: null })]))
  await waitFor(() => expect(trackEvent).toHaveBeenCalledWith(METRICS.BUDGET_PLAN_CLEAR_CELL))
})

it('a read-only cell never opens an editor and Delete sends nothing', async () => {
  // a guest: role decides editability (same setup as comments.plan.guest.test.tsx)
  const guestWireBudget = {
    ...fixtureWireBudget,
    meta: {
      ...fixtureWireBudget.meta,
      ownerUserId: 'u9',
      access: [
        { user: { id: 'u9', avatar: 'face:sky', name: 'Owner' }, role: 'owner', isAccepted: 1 },
        { user: { id: 'u1', avatar: 'face:emerald', name: 'Ada' }, role: 'guest', isAccepted: 1 },
      ],
    },
  }
  const spy = vi.fn()
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: guestWireBudget } })),
    planHandler(),
    http.post('*/api/v1/budget/set-limit', () => { spy(); return HttpResponse.json({ success: true, message: '', data: {} }) }),
  )
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  const { grid } = await gridAtFirstExpenseCell()
  fireEvent.keyDown(grid, { key: '5' })
  fireEvent.keyDown(grid, { key: 'F2' })
  fireEvent.keyDown(grid, { key: 'Delete' })
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  await new Promise((r) => setTimeout(r, 50))
  expect(spy).not.toHaveBeenCalled()
})

it('editing at the right edge: Tab commits to the edited month, then the window moves', async () => {
  usePlanHandlers()
  const calls: { period: string }[] = []
  server.use(http.post('*/api/v1/budget/set-limit', async ({ request }) => {
    calls.push((await request.json()) as { period: string })
    return HttpResponse.json({ success: true, message: '', data: {} })
  }))
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  await screen.findByTestId('plan-sheet')
  fireEvent.click(screen.getByTestId('plan-cell-pe1:2')) // Aug, last column
  const grid = screen.getByTestId('plan-sheet')
  fireEvent.keyDown(grid, { key: '9' })
  const input = await screen.findByRole('textbox')
  fireEvent.keyDown(input, { key: 'Tab' })
  await waitFor(() => expect(calls[0]?.period).toBe('2026-08-01'))
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-08-01')
})
```

Fixture facts (`web/src/test/fixtures.ts`, `fixtureWirePlan`, months May–Aug 2026): with `selectedDate` Jul and 3 columns the grid shows Jun/Jul/Aug; `pe1` (expense envelope "Living", folder `bf1`) has Jul `actual 45 / planned 250` and Aug unplanned; `cat-food` is a loose expense category with Jul unplanned.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && pnpm test src/features/budgets/PlanSheet.editing.test.tsx`
Expected: FAIL (no textbox appears).

- [ ] **Step 3: Implement**

`PlanCellInput.tsx`:

```tsx
import { useEffect, useRef, useState } from 'react'
import type { KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { limitAmountFromInput } from './limitAmount'

export type CellMove = 'down' | 'right' | 'left' | 'up' | 'none'

const MOVES: Record<string, CellMove> = { Enter: 'down', ArrowDown: 'down', ArrowUp: 'up', Tab: 'right' }

/** The Plan grid's in-cell editor: it sits in the cell and keeps the grid's keys. */
export function PlanCellInput({
  initial,
  label,
  onCommit,
  onCancel,
}: {
  initial: string
  label: string
  /** called only with a valid value; the grid decides whether it changed */
  onCommit: (raw: string, move: CellMove) => void
  onCancel: () => void
}) {
  const { t } = useTranslation()
  const [value, setValue] = useState(initial)
  const [invalid, setInvalid] = useState(false)
  const ref = useRef<HTMLInputElement | null>(null)
  // closing by key already committed or cancelled; the blur that follows must not commit again
  const closed = useRef(false)

  useEffect(() => {
    const el = ref.current
    if (el) {
      el.focus()
      el.setSelectionRange(el.value.length, el.value.length)
    }
  }, [])

  const tryCommit = (move: CellMove): boolean => {
    if (!limitAmountFromInput(value).ok) {
      setInvalid(true)
      return false
    }
    closed.current = true
    onCommit(value, move)
    return true
  }

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    e.stopPropagation()
    if (e.key === 'Escape') {
      e.preventDefault()
      closed.current = true
      onCancel()
      return
    }
    const move = e.key === 'Tab' && e.shiftKey ? 'left' : MOVES[e.key]
    if (move) {
      e.preventDefault()
      tryCommit(move)
    }
  }

  return (
    <span className="relative block w-full">
      <input
        ref={ref}
        type="text"
        inputMode="decimal"
        aria-label={label}
        aria-invalid={invalid}
        value={value}
        onChange={(e) => {
          setValue(e.target.value)
          setInvalid(false)
        }}
        onKeyDown={onKeyDown}
        onBlur={() => {
          if (!closed.current && !tryCommit('none')) {
            onCancel()
          }
        }}
        className="w-full bg-background px-1 text-right text-[15px] tabular-nums outline-none ring-2 ring-ring"
      />
      {invalid ? (
        <span role="alert" className="absolute top-full right-0 z-30 mt-0.5 whitespace-nowrap rounded bg-background px-1 text-xs text-expense shadow">
          {t('common.validation.invalid_formula')}
        </span>
      ) : null}
    </span>
  )
}
```

`PlanSheet.tsx`:
- Add state `const [editing, setEditing] = useState<{ rowKey: string; col: number; initial: string } | null>(null)`.
- `startEdit(rk, col, { replace, text })`:
  - only for `isEditableCell(...)` and `!isCompact`;
  - `initial = replace ? text : (planned === '' || isZero(planned) ? '' : normalizeNumber(planned))`;
  - sets `editing` and the selection.
- `finishEdit(raw, move)`:
  - `const parsed = limitAmountFromInput(raw)`;
  - when `parsed.ok` and `parsed.amount !== (planned === '' ? null : planned)` (compare with `cmp` when both are non-null, so `75` vs `75.00` counts as unchanged), call `commit(el.id, month, idx, parsed.amount)` using the month captured when editing STARTED (Review Focus 3);
  - then `setEditing(null)`, refocus the grid, and apply `move` with the same rules the arrow keys use (extract the arrow-move code in `handleKeyDown` into `moveSelection(dir)` and call it).
- `handleKeyDown`, before the arrow switch on an element row's month cell (`selection.col >= 0`):

```ts
    if (!e.ctrlKey && !e.metaKey && !e.altKey) {
      if (/^[0-9.,-]$/.test(e.key)) { e.preventDefault(); startEdit(entry.rowKey, selection.col, { replace: true, text: e.key }); return }
      if (e.key === 'F2' && !e.shiftKey) { e.preventDefault(); startEdit(entry.rowKey, selection.col, { replace: false }); return }
      if (e.key === 'Delete' || e.key === 'Backspace') { e.preventDefault(); clearSelectedCell(); return }
    }
```

  - `Enter` on a month cell calls `startEdit(..., { replace: false })` on desktop (the `handleEnter` desktop branch); `Shift+Enter` keeps opening comments.
  - The `KEYDOWN_ESCAPE_SELECTOR` guard already skips keys from the input; the input also stops propagation.
- `clearSelectedCell()`: on an editable cell whose `planned !== ''`, call `setLimit.mutate({ … amount: null }, { onSuccess: () => trackEvent(METRICS.BUDGET_PLAN_CLEAR_CELL) })`. Use `setLimit.mutate` directly so `onSuccess` is available; `commit` stays for the other paths.
- `PlanRows.tsx`: when `ctx.editing` matches this cell, render `<PlanCellInput initial label={t('budgets.page.plan.cell.edit_aria', { name, month })} …/>` in place of the `cell-actual`/`cell-planned` content, and hide the fill handle and comment marker while editing. A double-click on the cell (`onDoubleClick`) calls `ctx.startEdit(rk, i, { replace: false })`.
- New locale key `budgets.page.plan.cell.edit_aria` = `"Plan for {name}, {month}"` in all 11 catalogues (translate it).
- `metrics.ts`: add `BUDGET_PLAN_CLEAR_CELL: 'appBudgetPlanClearCell',` next to `BUDGET_PLAN_PASTE_CELL`.
- Delete the now-unused `LimitEditor` import from `PlanSheet`/`PlanRows` (`LimitEditor` itself stays; the Budget view uses it).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && pnpm test src/features/budgets/ src/lib/metrics-coverage.test.ts` and `pnpm exec tsc -b --noEmit`, then `cd .. && GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/test/i18ntest/`.
Expected: all PASS. Remove the Task-4 interim (Enter → `SetLimitDialog` on desktop) and its test adjustments: those tests now use the inline editor.

- [ ] **Step 5: Commit**

```bash
git add web/src locales
git commit -m "feat: Plan cells edit in place like a spreadsheet; Delete clears a plan"
```

---

### Task 6: One set of ⋮ menus for both views

**Files:**
- Create: `web/src/features/budgets/useBudgetLineMenus.tsx`
- Modify: `web/src/features/budgets/BudgetPage.tsx`, `web/src/features/budgets/PlanSheet.tsx`, `web/src/features/budgets/PlanRows.tsx`
- Delete: `web/src/features/budgets/PlanCreateFolderDialog.tsx`, and Plan's `RowMenu`/`MoveToFolderDialog` (in `PlanSheet.tsx`)
- Test: `web/src/features/budgets/PlanSheet.test.tsx`, `web/src/features/budgets/BudgetPage.test.tsx`

**Interfaces:**
- Produces:

```ts
export interface BudgetLineMenus {
  /** a row of the Budget view (get-budget) */
  expenseRowMenu: (element: BudgetElementDto) => MenuAction[]
  incomeRowMenu: (cell: PlanCellFigures) => MenuAction[]
  /** a row of the Plan grid: any side, envelopes and savings included */
  planRowMenu: (el: PlanElementDto, monthIndex: number) => MenuAction[]
  envelopeChildMenu: (child: { id: Id; type: BudgetElementType; name: string; icon: string; ownerUserId: Id | null }) => MenuAction[]
  savingsRowMenu: (row: BudgetSavingsElementDto) => MenuAction[]
  labelMenu: (label: LabelSpendDto) => MenuAction[]
  folderActionsFor: (folder: { id: Id; name: string } | null, empty: boolean, side: BudgetFolderSide) => MenuAction[] | undefined
  sectionMenu: (side: BudgetFolderSide) => MenuAction[] | undefined
  savingsSectionMenu: MenuAction[] | undefined
  /** the element's own edit dialog (sheet pencil, Enter on a name cell) */
  editFromSheet: (target: SheetTarget) => void
  /** every dialog the menus open; render once */
  dialogs: ReactNode
}

export function useBudgetLineMenus(args: {
  budget: BudgetDto | undefined
  /** the plan window the caller has loaded: income folders for Move to folder */
  plan: BudgetPlanDto | null | undefined
  userId: Id | undefined
  /** opens Budget settings (savings accounts) */
  onOpenSettings: () => void
}): BudgetLineMenus
```

- [ ] **Step 1: Write the failing tests**

`PlanSheet.test.tsx`:

```ts
it('Plan rows, folders and sections offer the Budget view ⋮ menus on hover', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getAllByRole('button', { name: /^menu / })[0])
  const items = screen.getAllByRole('menuitem').map((i) => i.textContent)
  expect(items.some((x) => /create folder/i.test(x ?? ''))).toBe(true)
})

it('desktop Plan view: Configure opens Budget settings at once, no Edit structure', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: /configure/i }))
  expect(screen.queryByText(/edit structure/i)).not.toBeInTheDocument()
  expect(await screen.findByRole('dialog')).toBeInTheDocument()
})
```

Delete the PlanSheet tests that drive the old edit mode (`aria-label` "element actions …", "budget folder actions …", the "Create folder" button above the grid, `PlanCreateFolderDialog`). Keep their intent by asserting the same actions through the hover menus where an equivalent exists:
- Change currency, Move to folder, Edit and Delete envelope → row ⋮;
- Edit and Delete folder → folder ⋮;
- Create folder → section ⋮.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && pnpm test src/features/budgets/PlanSheet.test.tsx`
Expected: FAIL (no `menu …` buttons in Plan rows; Configure opens the chooser).

- [ ] **Step 3: Implement**

- Move these from `BudgetPage.tsx` into `useBudgetLineMenus`:
  - the menu builders (`editAction`, `structureActions`, `classificationOf`, `classificationActions`, `expenseRowMenu`, `incomeRowMenu`, `envelopeChildMenu`, `savingsRowMenu`, `labelMenu`, `newEnvelopeAction`, `folderActionsFor`, `sectionMenu`, `savingsSectionMenu`, `moveTargetFolders`, `sheetEditAccess`, `editFromSheet`);
  - the state and dialogs they open (`createFolderSide`, `renameFolder`, `envelopeDialog`, `categoryTarget`, `tagTarget`, `deleteEnvelopeTarget`, `deleteFolderTarget`, `currencyTarget`, `moveFolderTarget`, plus their `PromptDialog`/`EnvelopeDialog`/`CategoryDialog`/`TagDialog`/`ConfirmDialog`/`CurrencyPickerDialog`/move-to-folder `ResponsiveDialog` JSX and `classificationMenu.dialogs`);
  - the mutations they use.
- `configure`/`editDetails` derive inside the hook from `budget.meta`, `userId` and `isArchived`, exactly as in `BudgetPage`.
- Add `planRowMenu`:

```ts
  const planRowMenu = (el: PlanElementDto, monthIndex: number): MenuAction[] => {
    if (el.id === UNCATEGORIZED_ID) {
      return []
    }
    if (el.type === BudgetElementType.SAVINGS) {
      return editAction({ kind: 'plan', cell: planCellFigures(el, monthIndex) })
    }
    const side: BudgetFolderSide = isIncomeType(el.type) ? 'income' : 'expense'
    return [...editAction({ kind: 'plan', cell: planCellFigures(el, monthIndex) }), ...structureActions(el, side), ...classificationActions(el)]
  }
```

- `moveTargetFolders('expense')` must work without a get-budget payload too: when `plan` is given, use `folderSides(plan)` for both sides (`!== 'income'` for expense, `!== 'expense'` for income). Otherwise fall back to `budget.structure.folders` for expense.
- `BudgetPage` calls the hook with `plan: monthPlan.data` and renders `{menus.dialogs}`.
- `PlanSheet` calls it with its own `plan`, renders `{menus.dialogs}`, and passes into the rows and lines:
  - `GridCtx.rowMenu = (el) => menus.planRowMenu(el, monthIndex(selected))`;
  - `GridCtx.childMenu = menus.envelopeChildMenu` (envelope children only);
  - `FolderLine menu={menus.folderActionsFor(folder, rows.length === 0, side)}`;
  - section lines `menu={menus.sectionMenu(side)}`, and savings `menu={menus.savingsSectionMenu}`.
- Wrap the grid body in `<LineControlsContext.Provider value={isCompact ? (editMode ? 'always' : …) : 'hover'}>`. Menus render only when controls are on: compact and not edit mode → pass `undefined` menus, as `BudgetPage` does with `hoverMenus`.
- Delete from `PlanSheet`:
  - `RowMenu`, `MoveToFolderDialog`, `PlanCreateFolderDialog` usage, and the edit-mode "Create folder" button;
  - its own currency, envelope, category, tag, folder rename and folder delete dialogs and state (the hook owns them now);
  - `openElementEditor` (Enter on a name cell now calls `menus.editFromSheet({ kind: 'plan', cell: planCellFigures(el, idx) })` when `planRowMenu`'s Edit item is enabled, and otherwise shows the existing no-access toast);
  - the `ctx.editMode` branches of the row markup.
- Delete the file `PlanCreateFolderDialog.tsx` and any now-unused locale keys only if `grep -r` finds no other user. Unused keys are allowed by the i18n guard, so leaving them is fine.
- `BudgetPage.tsx` Configure: `const structureMode = isCompact` (was `isCompact || mode === 'plan'`). Desktop Plan now opens `BudgetUpdateDialog` directly; tablet keeps the chooser.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && pnpm test src/features/budgets/` and `pnpm exec tsc -b --noEmit`, then `pnpm lint`.
Expected: PASS, with no unused-variable lint errors.

- [ ] **Step 5: Commit**

```bash
git add web/src locales
git commit -m "refactor: One set of hover menus and dialogs for the Budget and Plan views"
```

---

### Task 7: Drag in the Plan grid with the Budget view's parts

**Files:**
- Modify: `web/src/features/budgets/PlanSheet.tsx`, `web/src/features/budgets/PlanRows.tsx`
- Test: `web/src/features/budgets/PlanSheet.test.tsx` (the existing captured-`DndContext` harness)

**Interfaces:**
- Consumes:
  - `DragRow`, `DragFolder`, `FolderGrip`, `DragChild`, `EnvelopeDrop`, `EnvelopeHeadDrop`, `DragGhost` (`MonthDrag.tsx`);
  - `envelopeCollisions`, `envelopeOfDrop`, `placeFromEnvelope`, `dropIndicatorFor`, `moveElementInArrangement`, `arrangementItem`, `centerRowOnPointer`/`besidePointer` (check where `BudgetPage` imports the two modifiers from and reuse them);
  - `useMoveIntoEnvelope` (`queries.ts`).
- Produces: nothing new for later tasks.

Rules:
- One `DndContext` per section (income, savings, expense), so a drag never crosses sides. This is the same constraint `PlanBand` enforced.
- Drag is enabled when `configure` is true and controls are showing (hover on desktop, edit mode on tablet).

- [ ] **Step 1: Write the failing tests**

Using the captured `DndContext` handlers (the existing `capturedDragContexts`: income first, then savings when present, then expense, per render; take the last set):

```ts
it('dragging an expense category onto an envelope moves it into the envelope', async () => {
  usePlanHandlers()
  const calls: unknown[] = []
  server.use(http.post('*/api/v1/budget/move-element', async ({ request }) => {
    calls.push(await request.json())
    return HttpResponse.json({ success: true, message: '', data: {} })
  }))
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  await screen.findByTestId('plan-sheet')
  // contexts mount income, savings (when present), expense: the expense one is last
  const expense = capturedDragContexts[capturedDragContexts.length - 1]
  act(() => {
    expense.onDragStart({ active: { id: 'cat-food' } })
    expense.onDragEnd({ active: { id: 'cat-food' }, over: { id: 'benv:pe1' } })
  })
  // cat-food is a loose expense category, pe1 the expense envelope "Living"
  await waitFor(() => expect(calls).toEqual([expect.objectContaining({ id: 'cat-food', envelopeId: 'pe1' })]))
})

it('rows show the hover drag grip, folders the folder grip', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  await screen.findByTestId('plan-sheet')
  expect(screen.getAllByRole('button', { name: /^move / }).length).toBeGreaterThan(0)
  expect(screen.getAllByRole('button', { name: /^move folder / }).length).toBeGreaterThan(0)
})
```

Port the existing drag tests (they fire `onDragEnd` with `pfolder:`/`bfolder:` ids). Folder ids are now plain folder ids for the sortable plus `bfolder:<id|null>` drop ids, the same scheme as `BudgetPage`. Update the expected `over` ids to match.

`moveIntoEnvelope` posts to `/api/v1/budget/move-element` with `envelopeId` (`web/src/api/budget.ts:164`). The capture mock records `onDragStart`/`onDragEnd` only; extend it to `onDragOver`/`onDragCancel` if your handlers need them in tests.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && pnpm test src/features/budgets/PlanSheet.test.tsx`
Expected: FAIL (no grips without edit mode; no envelope drop).

- [ ] **Step 3: Implement**

- Replace `PlanBand`, `PlanSortableRow`, `PlanSortableFolder`, `PlanFolderGrip`, `LooseRowsContainer` and `PlanRowList`'s edit-mode branch with the `MonthDrag` parts, mirroring `BudgetPage`'s expense `DndContext`:
  - `collisionDetection={envelopeCollisions(canEnterEnvelope)}`;
  - `measuring={{ droppable: { strategy: MeasuringStrategy.Always } }}`;
  - `modifiers={[centerRowOnPointer]}`;
  - `onDragStart`, `onDragOver` (drop indicator), `onDragEnd`, `onDragCancel`;
  - `<DragOverlay dropAnimation={null} modifiers={[besidePointer]}>`.
- Folders: `<SortableContext items={folderIds}>` with `DragFolder sortableId={folder.id} dropId={`bfolder:${folder.id}`} rowIds=… indicator=… folderDragging=…>` around each folder's line + rows.
- Loose rows: `DragFolder sortableId={null} dropId="bfolder:null"`.
- `FolderGrip` goes in `FolderLine`'s `handle` prop. Rows go in `DragRow id={el.id} indicator=…`.
- Envelope children: `DragChild`. Envelope rows: `EnvelopeDrop` around their child list, `EnvelopeHeadDrop` on a folded envelope row.
- `onDragEnd` logic: port `BudgetPage.handleDragEnd`.
  - Folder reorder → `orderFolders.mutate` with `afterIdFromDrop`.
  - Into envelope → `moveIntoEnvelope.mutate`.
  - Out of envelope → `placeFromEnvelope` + `moveElement.mutate`.
  - Row move → `moveElementInArrangement` + `arrangementItem` + the existing `commitElementMove`, which keeps the optimistic `dragArrangement`.
  - The arrangement is built from the Plan section's visible rows (the existing `bandArrangement(side)`, minus `hideEmpty`).
- `envelopeOfCategory`/`canEnterEnvelope` are built from `plan.structure.elements` the same way `BudgetPage` builds them from `budget.structure.elements` (only categories of the same side may enter).
- Savings: one `DndContext` with a single `DragFolder sortableId={null} dropId="bfolder:null"` and `DragRow`s; reuse the existing `handleSavingsDragEnd`.
- Archived rows, Uncategorized and deleted savings accounts are not wrapped in `DragRow`.
- Remove `draggingFolder` and replace it with `draggingFolderId`, as in `BudgetPage`: while a folder drags, every folder renders header-only.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && pnpm test src/features/budgets/` and `pnpm exec tsc -b --noEmit` and `pnpm lint`.
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/budgets
git commit -m "feat: Drag rows, folders and envelope categories in the Plan grid like in the Budget view"
```

---

### Task 8: Regression plan, full verification

**Files:**
- Modify: `docs/regression-test-plan.md` (section "9. Budgets — table & plan")

- [ ] **Step 1: Update the checklist**

Edit the Plan items (around lines 728–734, 932–940, 877–890, and any item naming the Plan nav arrows, the `$` column, boxed folders, "Edit structure" on desktop Plan, the savings balance line in rows, or the Plan "Savings balance" sticky line) so they describe:

- 📱 Desktop and tablet Plan view: the header is the Budget view's (`Budget · Plan │ ‹ › month strip`). Clicking a month makes it the tinted column, with one month before it. Switching views keeps the month. The ‹ › arrows pan the strip only.
- Plan window at the edges: with the start month selected there is no history column; an ended budget's window ends at its end month; with a single column it is the selected month.
- Plan rows are one line: `actual · plan` up to the selected month and the plan alone after it. An unplanned month is blank. The actual is red only when over plan, never green. There is no `$` column; a foreign row shows its currency code next to the name.
- Section and folder lines show per-month sums, open or folded, in the Budget view's thin-line style (no boxes, no bold). Folds are shared with the Budget view.
- Totals: Income, Expenses, Savings, Transfers (only when not zero), Total savings, then a sticky Balance.
- Editing (desktop):
  - typing a digit edits the cell in place;
  - F2, Enter or a double-click edit the current value;
  - Enter commits and moves down; Tab / Shift+Tab move right / left; ↑ / ↓ commit and move; Esc cancels;
  - an invalid formula keeps the editor open with a message;
  - Delete clears the plan;
  - a read-only cell never opens an editor;
  - → past the last column moves the selected month (and the strip).
- Clicking a past or current actual opens that row's transactions for that month (not for income Uncategorized).
- Hover ⋮ menus and drag grips in the Plan grid offer the same actions as in the Budget view (including dragging a category into and out of an envelope). Desktop Configure opens Budget settings directly in both views; tablet keeps Budget settings / Edit structure.

Keep the 📱 markers accurate: tablet = 📱; desktop-only items have none.

- [ ] **Step 2: Full verification**

Run from `web/`: `pnpm test`, `pnpm lint`, `pnpm exec tsc -b --noEmit`, `pnpm build`.
Run from the repo root: `GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/test/i18ntest/`.
Expected: all PASS / exit 0. Report the exact counts.

- [ ] **Step 3: Commit**

```bash
git add docs/regression-test-plan.md
git commit -m "docs: Regression plan for the redesigned Plan view"
```
