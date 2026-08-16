# Plan Sheet Fill Handle + Section Split Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Four plan-sheet refinements: Excel-style fill-right of a planned cell's value via a drag handle; an unmistakable visual split between the income and expense areas (labeled foldable "Expenses" header, tinted bands, strong divider); a compact totals footer showing one effective value per cell; and zero-valued Uncategorized rows hidden in both plan and budget modes.

**Architecture:** All changes live in `web/src/features/budgets/` + locales. A pure `fillTargetCol` goes into `planMath.ts`; a `useFillPlannedCells` mutation (N idempotent `set-limit` POSTs, one optimistic patch) goes into `queries.ts`; `PlanSheet.tsx` gains the handle (raw pointer events + `setPointerCapture`, drag state beside the existing `selection` state) and the Expenses section header/tints. No backend changes — the wire contract is frozen.

**Tech Stack:** React 19 + TS, TanStack Query, vitest + Testing Library + msw. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-08-12-plan-fill-handle-design.md`.

## Global Constraints

- **No backend changes**; the only wire calls are the existing `POST /api/v1/budget/set-limit`, one per covered month.
- **Overwrite semantics**: covered cells' existing planned values are replaced. Rightward only; leftward/downward drags clamp back to the source. Visible columns only.
- Handle renders ONLY on the SELECTED, editable, non-archived, non-uncategorized parent cell whose planned value is non-empty, and never in compact/single-column mode.
- New METRICS key (exact): `BUDGET_PLAN_FILL_RIGHT: 'appBudgetPlanFillRight'`, fired ONCE per fill from the mutation's `onSuccess` (the choke point — `metrics-coverage` enforces it).
- New i18n keys in ALL ELEVEN catalogues (`locales/{de,en,es,fr,it,nl,pl,pt,ru,uk,zh}.json`), English values frozen: `budgets.page.plan.fill.handle_aria` = "Copy planned value to the following months"; `budgets.page.plan.section.expenses` = "Expenses". Placeholder sets identical across languages (neither key has placeholders). Verify with `go test ./internal/test/i18ntest/ -count=1` (Go at `~/.local/go/bin`).
- The income-above-expense DOM-order invariant and every existing PlanSheet test must keep passing (selector-only updates allowed, never behavior).
- Amounts via `@/lib/decimal`; no `parseFloat`.
- Gates per task from `web/` (pnpm at `~/.local/bin`): `pnpm test -- --run` (green except the KNOWN pre-existing Blob failure in `src/api/transaction.test.ts`), `pnpm exec tsc -b`, `pnpm lint`. Everything runs in the foreground.
- Branch `feature/budget-plan-view`; commit AND `git push` per task.

---

### Task 1: `fillTargetCol` + `useFillPlannedCells`

**Files:**
- Modify: `web/src/features/budgets/planMath.ts` (one function, after `clampFirstMonth`)
- Modify: `web/src/lib/metrics.ts` (one key, after `BUDGET_PLAN_HIDE_EMPTY_TOGGLE`)
- Modify: `web/src/features/budgets/queries.ts` (one mutation, after `usePlanSetLimit`)
- Test: `web/src/features/budgets/planMath.test.ts`, `web/src/features/budgets/queries.test.tsx` (extend both)

**Interfaces:**
- Consumes: existing `usePlanSetLimit` shape, `queryKeys.budget`/`budgetPlan`, `budgetApi.setLimit`, `BudgetPlanDto`.
- Produces (Task 2 relies on these exact names):

```ts
// planMath.ts
/** Excel fill: the column the drag currently targets. Right-only — never
 *  before startCol; clamped to the last visible column; a degenerate
 *  colWidth (<= 0) stays on the source. */
export function fillTargetCol(startCol: number, deltaX: number, colWidth: number, lastCol: number): number

// metrics.ts
BUDGET_PLAN_FILL_RIGHT: 'appBudgetPlanFillRight',

// queries.ts
export function useFillPlannedCells(planKey: readonly unknown[])
// mutate form: { budgetId: Id; elementId: Id; amount: string;
//               targets: { period: string; monthIndex: number }[] }
```

- [ ] **Step 1: Write the failing tests**

Append to `planMath.test.ts`:

```ts
describe('fillTargetCol', () => {
  it('rounds the pointer delta to whole columns', () => {
    expect(fillTargetCol(1, 0, 110, 5)).toBe(1)
    expect(fillTargetCol(1, 54, 110, 5)).toBe(1)   // < half a column
    expect(fillTargetCol(1, 56, 110, 5)).toBe(2)   // past half
    expect(fillTargetCol(1, 275, 110, 5)).toBe(4)  // 2.5 -> round -> 3 cols right
  })
  it('never goes left of the source and clamps at the last visible column', () => {
    expect(fillTargetCol(2, -500, 110, 5)).toBe(2)
    expect(fillTargetCol(2, 5000, 110, 5)).toBe(5)
  })
  it('degrades to the source column on zero/negative width', () => {
    expect(fillTargetCol(1, 300, 0, 5)).toBe(1)
    expect(fillTargetCol(1, 300, -10, 5)).toBe(1)
  })
})
```

Append to `queries.test.tsx` (mirror the file's existing `usePlanSetLimit` harness):

```tsx
it('useFillPlannedCells posts one set-limit per target, patches all cells, fires the metric once', async () => {
  const sent: Record<string, unknown>[] = []
  server.use(
    http.post('*/api/v1/budget/set-limit', async ({ request }) => {
      sent.push((await request.json()) as Record<string, unknown>)
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const { result, queryClient, planKey } = renderFillHarness(fixtureWirePlan) // sibling of renderPlanSetLimitHarness
  result.current.mutate({
    budgetId: 'b1', elementId: 'pe1', amount: '250',
    targets: [
      { period: '2026-06-01', monthIndex: 1 },
      { period: '2026-07-01', monthIndex: 2 },
    ],
  })
  // optimistic: both covered cells patched immediately
  const plan = queryClient.getQueryData<BudgetPlanDto>(planKey)
  const cells = plan?.structure.elements.find((e) => e.id === 'pe1')?.cells
  expect(cells?.[1]?.planned).toBe('250')
  expect(cells?.[2]?.planned).toBe('250')
  await waitFor(() => expect(sent).toHaveLength(2))
  expect(sent).toEqual(expect.arrayContaining([
    expect.objectContaining({ budgetId: 'b1', elementId: 'pe1', period: '2026-06-01', amount: '250' }),
    expect.objectContaining({ budgetId: 'b1', elementId: 'pe1', period: '2026-07-01', amount: '250' }),
  ]))
  await waitFor(() => expect(trackEventMock).toHaveBeenCalledWith(METRICS.BUDGET_PLAN_FILL_RIGHT))
  expect(trackEventMock.mock.calls.filter((c) => c[0] === METRICS.BUDGET_PLAN_FILL_RIGHT)).toHaveLength(1)
})

it('useFillPlannedCells invalidates the plan on any failure (server truth resyncs)', async () => {
  server.use(http.post('*/api/v1/budget/set-limit', () => HttpResponse.json({ success: false, message: 'boom', code: 400, errors: {} }, { status: 400 })))
  // mutate one target; assert invalidateQueries was called with the plan key (spy pattern from the F6 test)
})
```

(Adapt spy/mock names to what `queries.test.tsx` already uses for `trackEvent` and `invalidateQueries` — both patterns exist in the file from plan 03.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `pnpm test -- --run planMath queries`
Expected: FAIL — `fillTargetCol`/`useFillPlannedCells` undefined.

- [ ] **Step 3: Implement**

`planMath.ts`:

```ts
export function fillTargetCol(startCol: number, deltaX: number, colWidth: number, lastCol: number): number {
  if (colWidth <= 0) {
    return startCol
  }
  const target = startCol + Math.round(deltaX / colWidth)
  return Math.min(Math.max(target, startCol), lastCol)
}
```

`metrics.ts`: add `BUDGET_PLAN_FILL_RIGHT: 'appBudgetPlanFillRight',` after the hide-empty key.

`queries.ts`:

```ts
export function useFillPlannedCells(planKey: readonly unknown[]) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (form: { budgetId: Id; elementId: Id; amount: string; targets: { period: string; monthIndex: number }[] }) =>
      Promise.all(
        form.targets.map((t) =>
          budgetApi.setLimit({ budgetId: form.budgetId, elementId: form.elementId, period: t.period, amount: form.amount }),
        ),
      ),
    onMutate: async (form) => {
      await queryClient.cancelQueries({ queryKey: planKey })
      const covered = new Set(form.targets.map((t) => t.monthIndex))
      queryClient.setQueryData<BudgetPlanDto | null>(planKey, (prev) => {
        if (!prev) {
          return prev
        }
        return {
          ...prev,
          structure: {
            ...prev.structure,
            elements: prev.structure.elements.map((el) =>
              el.id === form.elementId
                ? { ...el, cells: el.cells.map((c, i) => (covered.has(i) ? { ...c, planned: form.amount } : c)) }
                : el,
            ),
          },
        }
      })
    },
    // No partial rollback: any failure means some months may have landed —
    // resync from the server rather than pretending nothing happened.
    onError: () => {
      void queryClient.invalidateQueries({ queryKey: planKey })
    },
    onSuccess: (_res, form) => {
      trackEvent(METRICS.BUDGET_PLAN_FILL_RIGHT)
      for (const t of form.targets) {
        void queryClient.invalidateQueries({ queryKey: [...queryKeys.budget, form.budgetId, t.period] })
      }
      void queryClient.invalidateQueries({ queryKey: planKey })
    },
  })
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm test -- --run && pnpm exec tsc -b && pnpm lint`
Expected: PASS (the new metric is fired from the mutation, so `metrics-coverage` stays green).

- [ ] **Step 5: Commit + push**

```bash
git add web/src/features/budgets/planMath.ts web/src/features/budgets/planMath.test.ts web/src/lib/metrics.ts web/src/features/budgets/queries.ts web/src/features/budgets/queries.test.tsx
git commit -m "feat(web): fill-right math and mutation for plan cells"
git push
```

---

### Task 2: The fill handle in `PlanSheet.tsx`

**Files:**
- Modify: `web/src/features/budgets/PlanSheet.tsx`
- Modify: `locales/{de,en,es,fr,it,nl,pl,pt,ru,uk,zh}.json` (ONE key: `budgets.page.plan.fill.handle_aria`; nest under the existing `budgets.page.plan` object as `"fill": { "handle_aria": ... }`)
- Test: `web/src/features/budgets/PlanSheet.test.tsx` (extend)

**Interfaces:**
- Consumes: `fillTargetCol`, `useFillPlannedCells` (Task 1); the existing `GridCtx`, `selection`, `isEditableCell`, `cellDomId` machinery.
- Produces: `GridCtx` gains
  ```ts
  fill: {
    active: { rowKey: string; startCol: number; targetCol: number } | null
    start: (rowKey: string, el: PlanElementDto, col: number, e: React.PointerEvent<HTMLElement>) => void
    move: (e: React.PointerEvent<HTMLElement>) => void
    end: () => void
    cancel: () => void
  }
  ```
  and the handle carries `data-testid="fill-handle"`; covered cells carry the class token `fill-covered` (plus visual classes).

**Mechanics (implement exactly):**

- Drag state in `PlanSheet` beside `selection`:
  ```ts
  interface FillDrag { rowKey: string; elementId: Id; amount: string; startCol: number; targetCol: number; startX: number; colWidth: number }
  const [fillDrag, setFillDrag] = useState<FillDrag | null>(null)
  ```
- `fillStart(rowKey, el, col, e)`: read the source gridcell's width once — `colWidth = (e.currentTarget.closest('[role="gridcell"]') as HTMLElement)?.getBoundingClientRect().width ?? 0` — then `e.currentTarget.setPointerCapture(e.pointerId)`, `e.preventDefault()`, `e.stopPropagation()` (the handle must not trigger cell selection-click logic), and set `{ rowKey, elementId: el.id, amount: el.cells[monthIndex]!.planned, startCol: col, targetCol: col, startX: e.clientX, colWidth }`. The `amount` is captured from the SOURCE cell at drag start (fetched index via `ctx.monthIndex`).
- `fillMove(e)`: if no drag, return; `setFillDrag({ ...d, targetCol: fillTargetCol(d.startCol, e.clientX - d.startX, d.colWidth, visible - 1) })`.
- `fillEnd()`: if no drag, return; when `targetCol > startCol`, build `targets` from visible columns `startCol+1 .. targetCol`: `{ period: visibleMonths[c], monthIndex: monthIndex(visibleMonths[c]) }` (skip any with monthIndex < 0 — cannot happen for visible columns, but keep the guard), and `fillCells.mutate({ budgetId, elementId, amount, targets })`. Always `setFillDrag(null)`. Releasing on the source column is a no-op by the `targetCol > startCol` check.
- `fillCancel()`: `setFillDrag(null)` — wired to `onPointerCancel` on the handle AND an `Escape` branch at the TOP of the existing `handleKeyDown` (before the escape-selector bail): `if (e.key === 'Escape' && fillDrag) { setFillDrag(null); return }`.
- The handle element, rendered inside the month gridcell in `ElementRow` when `selected && editable && cell && cell.planned !== '' && !ctx.isCompact && ctx.visibleMonths.length > 1`:
  ```tsx
  <span
    data-testid="fill-handle"
    role="button"
    aria-label={t('budgets.page.plan.fill.handle_aria')}
    className="absolute -bottom-0.5 -right-0.5 z-10 size-2 cursor-crosshair rounded-[1px] border border-background bg-ring"
    onPointerDown={(e) => ctx.fill.start(rk, el, i, e)}
    onPointerMove={(e) => ctx.fill.move(e)}
    onPointerUp={() => ctx.fill.end()}
    onPointerCancel={() => ctx.fill.cancel()}
    onClick={(e) => e.stopPropagation()}
  />
  ```
  The month gridcell needs `relative` added to its className for the handle's absolute positioning. (Pointer capture keeps move/up firing on the handle even when the pointer travels across other cells.)
- Covered-cell highlight in `ElementRow`'s month-cell class: when `ctx.fill.active?.rowKey === rk && i > ctx.fill.active.startCol && i <= ctx.fill.active.targetCol`, append `' fill-covered bg-ring/15'`.
- `ctx` memo: add `fill` (state + the four callbacks, each `useCallback`); include `fillDrag` in the memo deps — a fill drag re-renders rows, which is unavoidable and fine (drags are short; the F7 memoization concern was about totals/FX, which don't depend on this state).
- The `CLICK_ESCAPE_SELECTOR` guard: the handle is a `role="button"` span, so a click on it already skips grid-focus/selection via the existing `button, [role="button"]` selector — no guard change needed; verify in the test.

- [ ] **Step 1: Write the failing tests**

Append to `PlanSheet.test.tsx` (reuse the file's render helpers; `pe1` has planned values in the fixture; use `fireEvent.pointerDown/pointerMove/pointerUp` — user-event has no pointer-capture simulation; jsdom lacks `setPointerCapture`, so stub it once in this describe block: `beforeEach(() => { HTMLElement.prototype.setPointerCapture ??= () => {}; HTMLElement.prototype.releasePointerCapture ??= () => {} })`; mock the source gridcell's `getBoundingClientRect` to `{ width: 110, ... }` via a spy on `HTMLElement.prototype.getBoundingClientRect` returning a 110-wide rect):

```tsx
describe('fill handle', () => {
  it('renders only on the selected editable non-empty cell', async () => {
    // select pe1's col-0 planned cell (click) -> handle present in that cell;
    // select the uncategorized row's cell -> no handle;
    // select a cell with planned '' -> no handle
  })
  it('drag right copies the value into covered months, one set-limit per month', async () => {
    // pointerDown on the handle at clientX 100, pointerMove to 320 (2 columns), pointerUp
    // -> two POSTs with the two later periods and the source amount; covered cells patched optimistically;
    // releasing without moving (targetCol === startCol) posts nothing
  })
  it('Escape during the drag cancels without any request', async () => {
    // pointerDown, pointerMove one column, keyboard Escape on the grid, pointerUp -> zero POSTs
  })
})
```

Write the three bodies as real assertions (msw capture array for POST bodies, `getAllByTestId('fill-handle')`, `queryByTestId` for absence, class assertions for `fill-covered`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `pnpm test -- --run PlanSheet`
Expected: FAIL — no `fill-handle` testid.

- [ ] **Step 3: Implement** per Mechanics above; add the i18n key to `locales/en.json` and translated equivalents to the other ten catalogues (match each catalogue's existing plan-section terminology).

- [ ] **Step 4: Verify i18n + gates**

Run: `export PATH=$HOME/.local/go/bin:$PATH && go test ./internal/test/i18ntest/ -count=1` (repo root) then from `web/`: `pnpm test -- --run && pnpm exec tsc -b && pnpm lint`
Expected: PASS.

- [ ] **Step 5: Commit + push**

```bash
git add web/src/features/budgets/PlanSheet.tsx web/src/features/budgets/PlanSheet.test.tsx locales/
git commit -m "feat(web): Excel-style fill handle copies planned values into later months"
git push
```

---

### Task 3: Income/expense section split

**Files:**
- Modify: `web/src/features/budgets/PlanSheet.tsx` (Expenses header, fold gating, tints, divider; `buildFlatRows` expense gating)
- Modify: `locales/{de,en,es,fr,it,nl,pl,pt,ru,uk,zh}.json` (ONE key: `budgets.page.plan.section.expenses`, next to `section.income`)
- Test: `web/src/features/budgets/PlanSheet.test.tsx` (extend)

**Interfaces:**
- Consumes: the existing `SectionHeader`, `togglePlanFold`/`planFolds` (fold key `'expense'` — the store already accepts arbitrary keys), `HiddenRowsNotice`, `buildFlatRows`, `visibleSectionRows`.
- Produces: the expense section renders a foldable labeled header; band classes `plan-band-income` / `plan-band-expense` (marker tokens alongside the visual classes) and a divider class on the expense section.

**Semantics (implement exactly):**

- In the render: the expense `<section>` gains, at its top, a `SectionHeader` with `label={t('budgets.page.plan.section.expenses')}`, `foldKey="expense"`, `folded={expenseFolded}` (`const expenseFolded = folded('expense')` — same helper the income header uses), `onToggleFold={togglePlanFold}`, `hiddenCount={expenseHiddenCount}`, `onShow={() => revealSection('expense')}`, no `action`. The whole expense body (folders, loose rows, uncategorized) wraps in `{!expenseFolded ? (...) : null}` exactly like the income section's body. DELETE the old bottom-of-section `HiddenRowsNotice` block (`expenseHiddenCount > 0 ? ...` at the section's end) — the count now lives on the header.
- **`buildFlatRows` must mirror the render**: it currently walks the expense area unconditionally. Add an `expenseFolded: boolean` parameter (threaded like the existing income fold handling — the income side gates on `folded('income')` via the `folded` callback already passed in; mirror that shape: gate expense folders AND `rows.expense.loose` AND `rows.expense.uncategorized` behind `folded('expense')`). Keep the loose-rows call's `visibleSectionRows(rows.expense.loose, folded('expense'), hideEmpty, revealed('expense'))` consistent with the render.
- **Tints + divider** (marker token first, then visual classes, so tests don't assert colors):
  - income section className gains `plan-band-income bg-emerald-500/[0.06] dark:bg-emerald-400/[0.06] rounded-md`
  - expense section className gains `plan-band-expense bg-rose-500/[0.04] dark:bg-rose-400/[0.04] rounded-md mt-2 border-t-2 border-border`
  - the archived section and `TotalsFooter` are untouched (untinted).

- [ ] **Step 1: Write the failing tests**

Append to `PlanSheet.test.tsx`:

```tsx
describe('income/expense split', () => {
  it('renders a foldable Expenses header that collapses the whole expense area', async () => {
    // header with the Expenses label exists inside plan-section-expense;
    // click it -> expense folder + loose rows + uncategorized disappear, income rows unaffected;
    // click again -> back. Fold state drives keyboard nav too: with the section
    // folded, ArrowDown from the last income row skips the expense rows.
  })
  it('hide-empty count for expense loose rows sits on the Expenses header', async () => {
    // enable hide-empty -> the "{count} hidden" notice renders inside the Expenses header
    // (within the header's element), not as a trailing section line; Show reveals.
  })
  it('bands and divider are distinguishable', async () => {
    // plan-section-income classList contains 'plan-band-income';
    // plan-section-expense contains 'plan-band-expense' and 'border-t-2'
  })
})
```

Write the bodies as real assertions.

- [ ] **Step 2: Run tests to verify they fail**

Run: `pnpm test -- --run PlanSheet`
Expected: FAIL — no Expenses header.

- [ ] **Step 3: Implement** per Semantics; add `section.expenses` to all 11 catalogues (reuse each language's existing word for expenses — grep the catalogue's `budgets.page.plan.totals.expenses` value and match it).

- [ ] **Step 4: Full gates**

Run: `export PATH=$HOME/.local/go/bin:$PATH && go test ./internal/test/i18ntest/ -count=1`; from `web/`: `pnpm test -- --run && pnpm exec tsc -b && pnpm lint && pnpm build`
Expected: PASS (the known Blob failure aside).

- [ ] **Step 5: Commit + push**

```bash
git add web/src/features/budgets/PlanSheet.tsx web/src/features/budgets/PlanSheet.test.tsx locales/
git commit -m "feat(web): split income and expense areas — labeled foldable Expenses header, tinted bands"
git push
```

---

### Task 4: Compact totals footer

**Files:**
- Modify: `web/src/features/budgets/planMath.ts` (`PlanMonthTotals` + `planTotals`)
- Modify: `web/src/features/budgets/PlanSheet.tsx` (`TOTALS_ROWS` + `TotalsFooter` cell rendering)
- Test: `web/src/features/budgets/planMath.test.ts`, `web/src/features/budgets/PlanSheet.test.tsx` (extend both)

**Interfaces:**
- Consumes: the existing `planTotals` internals (`effIncome`/`effExpense` accumulators already exist inside the loop).
- Produces:
  ```ts
  export interface PlanMonthTotals {
    incomeActual: string; incomePlanned: string
    expenseActual: string; expensePlanned: string
    netActual: string; netPlanned: string
    effectiveIncome: string   // NEW: per-cell max(actual, planned') summed; past months = actual
    effectiveExpense: string  // NEW: same for the expense side
    effectiveNet: string      // unchanged; === sub(effectiveIncome, effectiveExpense)
  }
  ```
  Footer rows render ONE value per cell: Income → `effectiveIncome`, Expenses → `effectiveExpense`, Net → `effectiveNet`. Balance row untouched. Keep the actual/planned fields on the type — `planTotals` stays the single totals source and removing them would churn existing tests for no benefit.

- [ ] **Step 1: Write the failing tests**

`planMath.test.ts` — extend the existing effectiveNet test block:

```ts
it('exposes effectiveIncome/effectiveExpense; net is their difference', () => {
  // reuse the block's fixture: past month -> effectiveIncome === incomeActual and
  // effectiveExpense === expenseActual; current/future month -> the hand-computed
  // per-cell max sums from the existing test; every month:
  // sub(effectiveIncome, effectiveExpense) === effectiveNet
})
```

`PlanSheet.test.tsx` — update the totals test: each Income/Expenses/Net footer cell now contains exactly ONE rendered amount (no `cell-actual`/`cell-planned` pair), and the value equals the corresponding `planTotals(...)` effective field formatted with `moneyFormat` (compute the expectation through the math core, as the existing test does for Balance).

- [ ] **Step 2: Run tests to verify they fail**

Run: `pnpm test -- --run planMath PlanSheet`
Expected: FAIL — `effectiveIncome` missing; footer still renders pairs.

- [ ] **Step 3: Implement**

`planMath.ts`: in `planTotals`'s return literal add `effectiveIncome: effIncome, effectiveExpense: effExpense,` (the accumulators already exist; no logic change).

`PlanSheet.tsx`: change `TotalsRowSpec` to `{ key: 'income' | 'expenses' | 'net'; value: (t: PlanMonthTotals) => string }` with
```ts
const TOTALS_ROWS: TotalsRowSpec[] = [
  { key: 'income', value: (t) => t.effectiveIncome },
  { key: 'expenses', value: (t) => t.effectiveExpense },
  { key: 'net', value: (t) => t.effectiveNet },
]
```
and render each footer cell as a single right-aligned `moneyFormat(spec.value(totals[idx]), planCurrency, { showCurrency: false, useNativePrecision: false })` span (drop the stacked pair markup; keep the row labels and grid columns; Balance row untouched).

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm test -- --run && pnpm exec tsc -b && pnpm lint`
Expected: PASS.

- [ ] **Step 5: Commit + push**

```bash
git add web/src/features/budgets/planMath.ts web/src/features/budgets/planMath.test.ts web/src/features/budgets/PlanSheet.tsx web/src/features/budgets/PlanSheet.test.tsx
git commit -m "feat(web): compact plan totals — one effective value per cell"
git push
```

---

### Task 5: Hide zero Uncategorized rows (plan + budget modes)

**Files:**
- Modify: `web/src/features/budgets/PlanSheet.tsx` (visible-window filter for uncategorized rows)
- Modify: `web/src/features/budgets/budgetMath.ts` (`bucketElements`' uncategorized filter)
- Test: `web/src/features/budgets/PlanSheet.test.tsx`, `web/src/features/budgets/budgetMath.test.ts` (extend both)

**Interfaces:**
- Consumes: `isZero` from `@/lib/decimal`; the sheet's `visibleMonths`/`monthIndex`; `bucketElements`' existing uncategorized handling.
- Produces: no signature changes — behavior only.

**Semantics (implement exactly):**
- PLAN mode: in `PlanSheet`, before rendering (and inside the same memo that feeds both the render and `buildFlatRows` — the filter must apply to ONE shared value so keyboard nav can't diverge), replace each side's `uncategorized` row with `null` when every VISIBLE column's cell has a zero actual:
  ```ts
  const visibleUncat = (r: PlanRow | null): PlanRow | null =>
    r && visibleMonths.some((m) => { const i = monthIndex(m); return i >= 0 && !isZero(r.element.cells[i]?.actual ?? '0') }) ? r : null
  ```
  applied to `rows.income.uncategorized` and `rows.expense.uncategorized` in a derived memoized structure (e.g. map `rows` → `shownRows` and use `shownRows` everywhere `rows` currently feeds the sections and `buildFlatRows`). Depends on `visibleMonths`, so the row re-appears/disappears as the window moves — intended.
- BUDGET mode: in `budgetMath.ts` `bucketElements`, the uncategorized bucket's filter gains a spent check:
  ```ts
  const uncategorizedElements = elements.filter(
    (el) => el.id === UNCATEGORIZED_ID && el.isArchived === 0 && cmp(el.spent, '0') !== 0,
  )
  ```
  (`el.spent` is the selected month's spend in the element's currency — budget currency for the synthetic row. `budgetTotals` keeps adding the bucket's stats, which are zero when the bucket is empty, so totals are unchanged.)

- [ ] **Step 1: Write the failing tests**

`budgetMath.test.ts`:

```ts
it('drops the Uncategorized bucket when the selected month has no uncategorized spend', () => {
  // fixture budget with an uncategorized element whose spent is '0' (carried by
  // prior-month spend server-side) -> buckets.uncategorized.elements is empty;
  // with spent '5' -> present. budgetTotals unchanged in the zero case.
})
```

`PlanSheet.test.tsx`:

```tsx
it('hides an uncategorized row whose visible cells are all zero, and it returns when the window covers its spend', async () => {
  // fixture variant: income-uncat has its only nonzero actual in month 0 (2026-05);
  // initial window starting 2026-07 (visible 3: Jul..) -> row absent (also absent
  // from keyboard flat rows: ArrowDown never lands on it);
  // navigate back so 2026-05 is visible -> row renders.
})
```

Write both as real assertions (clone `fixtureWirePlan` inline for the variant rather than mutating the shared fixture).

- [ ] **Step 2: Run tests to verify they fail**

Run: `pnpm test -- --run budgetMath PlanSheet`
Expected: FAIL.

- [ ] **Step 3: Implement** per Semantics.

- [ ] **Step 4: Full gates**

Run from `web/`: `pnpm test -- --run && pnpm exec tsc -b && pnpm lint && pnpm build`
Expected: PASS (known Blob failure aside). No i18n/metrics changes in this task.

- [ ] **Step 5: Commit + push**

```bash
git add web/src/features/budgets/PlanSheet.tsx web/src/features/budgets/PlanSheet.test.tsx web/src/features/budgets/budgetMath.ts web/src/features/budgets/budgetMath.test.ts
git commit -m "feat(web): hide zero uncategorized rows in plan and budget modes"
git push
```

---

## Ordering & dependencies

1 → 2 → 3 → 4 → 5 (Task 2 consumes Task 1's exports; Tasks 3-5 touch the same component/test files — sequential only). Tasks 4 and 5 are independent of the fill handle but share `PlanSheet.tsx`/its test file with Tasks 2-3.

## Out of scope (v1 — from the spec)

- Downward/multi-row fill, series extrapolation, leftward fill, clear-right (empty-source) fills, scroll-extending the drag, a keyboard fill equivalent.
- Any change to the budget page's single-month view.
