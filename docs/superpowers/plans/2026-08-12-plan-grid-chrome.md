# Plan Sheet Grid Chrome Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reclaim vertical space and make the plan sheet read as a ledger — only the Balance row stays pinned, the mode toggle and hide-empty switch fold into the page header, rows sit flush with hairline dividers, and hovering highlights the row and month column.

**Architecture:** Four independent changes to two files (`PlanSheet.tsx`, `BudgetPage.tsx`) plus a small block of hand-written CSS in `web/src/index.css`. The column hover avoids per-row DOM writes and React re-renders by keying a container `data-hover-col` attribute against per-cell `data-col={i}` with a fixed set of static CSS rules.

**Tech Stack:** React 19, TypeScript, Tailwind 4, shadcn/ui (Radix), vitest + Testing Library, oxlint.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-08-12-plan-grid-chrome-design.md`.
- Branch: `feature/budget-plan-view` (already checked out; do not create a new branch).
- Run the frontend suite from `web/`: `pnpm test`. Lint: `pnpm lint`.
- Tailwind 4 — no `tailwind.config.js` colour additions; use existing tokens (`accent`, `border`, `background`, `ring`, `destructive`).
- Do NOT alter the income/expense band tints (`bg-emerald-500/[0.06]`, `bg-rose-500/[0.04]` and their `dark:` variants) — recent deliberate work in `361daf76`.
- Do NOT alter the current-month tint (`bg-accent/40`) or the fill-drag tint (`bg-ring/15`).
- Comments: this repo's CLAUDE.md requires comments only for non-obvious rationale. Do not add comments restating what code does.
- Hover tint values in Task 4 are starting points; the invariant is ordering — column strictly fainter than row, both fainter than selection and fill.
- All hover CSS must be wrapped in `@media (hover: hover)`.

---

### Task 1: Split the totals footer so only Balance is pinned

**Files:**
- Modify: `web/src/features/budgets/PlanSheet.tsx` (the `TotalsFooter` component, ~lines 536-591; its call site, ~lines 1197-1205)
- Test: `web/src/features/budgets/PlanSheet.test.tsx` (existing test at ~line 168; new test appended)

**Interfaces:**
- Consumes: nothing from earlier tasks (first task).
- Produces: two components replacing `TotalsFooter`, both used by later tasks' markup only incidentally:
  - `PlanTotals({ visibleMonths, monthIndex, cur, gridCols, totals, currency })` — renders `data-testid="plan-totals"` with exactly three `role="row"` children (income, expenses, net), NOT sticky.
  - `PlanBalanceRow({ visibleMonths, monthIndex, cur, gridCols, balance, currency })` — renders `data-testid="plan-balance-row"`, sticky, keeps per-cell `data-testid={`plan-balance-${i}`}`.
  - Prop types are unchanged from the current `TotalsFooter` signature, just partitioned: `totals: PlanMonthTotals[]` goes to `PlanTotals`, `balance: string[]` goes to `PlanBalanceRow`, and `visibleMonths: string[]`, `monthIndex: (m: string) => number`, `cur: string`, `gridCols: string`, `currency: CurrencyDto | undefined` go to both.

- [ ] **Step 1: Write the failing test**

Append to `web/src/features/budgets/PlanSheet.test.tsx`:

```tsx
it('scrolls the income/expenses/net trio and pins only the balance row', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  const totals = screen.getByTestId('plan-totals')
  const balance = screen.getByTestId('plan-balance-row')

  // the trio and the balance are separate elements; neither contains the other
  expect(totals).not.toContainElement(balance)
  expect(balance).not.toContainElement(totals)

  // only the balance row is pinned
  expect(balance.className).toContain('sticky')
  expect(totals.className).not.toContain('sticky')

  // the trio keeps exactly its three rows
  expect(within(totals).getAllByRole('row')).toHaveLength(3)
  expect(within(totals).getByText('Income')).toBeInTheDocument()
  expect(within(totals).getByText('Expenses')).toBeInTheDocument()
  expect(within(totals).getByText('Net')).toBeInTheDocument()
  expect(within(balance).getByText('Balance')).toBeInTheDocument()
})
```

- [ ] **Step 2: Run test to verify it fails**

Run from `web/`: `pnpm test -- PlanSheet.test.tsx -t 'pins only the balance row'`
Expected: FAIL — `Unable to find an element by: [data-testid="plan-balance-row"]`.

- [ ] **Step 3: Replace `TotalsFooter` with the two components**

In `PlanSheet.tsx`, delete the whole `TotalsFooter` function (from `function TotalsFooter({` through its closing `}` before `interface EnvelopeDialogState`) and put these two in its place. `TOTALS_ROWS` and `TotalsRowSpec` above it stay as they are.

```tsx
function PlanTotals({
  visibleMonths,
  monthIndex,
  cur,
  gridCols,
  totals,
  currency,
}: {
  visibleMonths: string[]
  monthIndex: (m: string) => number
  cur: string
  gridCols: string
  totals: PlanMonthTotals[]
  currency: CurrencyDto | undefined
}) {
  const { t } = useTranslation()
  return (
    <div role="rowgroup" className="mt-2 flex flex-col border-t" data-testid="plan-totals">
      {TOTALS_ROWS.map((spec) => (
        <div key={spec.key} role="row" className="grid items-center gap-1 px-2 py-1" style={{ gridTemplateColumns: gridCols }}>
          <span className="truncate text-xs font-medium text-muted-foreground">{t(`budgets.page.plan.totals.${spec.key}`)}</span>
          {visibleMonths.map((m, i) => {
            const idx = monthIndex(m)
            const row = idx >= 0 ? totals[idx] : undefined
            return (
              <div key={m} data-col={i} className={`flex items-center justify-end px-2 py-1 ${m === cur ? 'bg-accent/40' : ''}`}>
                <span className="text-sm">
                  {row ? moneyFormat(spec.value(row), currency, { showCurrency: false, useNativePrecision: false }) : '—'}
                </span>
              </div>
            )
          })}
        </div>
      ))}
    </div>
  )
}

function PlanBalanceRow({
  visibleMonths,
  monthIndex,
  cur,
  gridCols,
  balance,
  currency,
}: {
  visibleMonths: string[]
  monthIndex: (m: string) => number
  cur: string
  gridCols: string
  balance: string[]
  currency: CurrencyDto | undefined
}) {
  const { t } = useTranslation()
  return (
    <div
      role="rowgroup"
      className="sticky bottom-0 z-10 border-t bg-background"
      data-testid="plan-balance-row"
    >
      <div role="row" className="grid items-center gap-1 px-2 py-1.5 font-semibold" style={{ gridTemplateColumns: gridCols }}>
        <span className="truncate text-xs">{t('budgets.page.plan.totals.balance')}</span>
        {visibleMonths.map((m, i) => {
          const idx = monthIndex(m)
          const value = idx >= 0 ? balance[idx] : undefined
          const negative = value !== undefined && cmp(value, '0') < 0
          return (
            <div
              key={m}
              data-col={i}
              data-testid={`plan-balance-${i}`}
              className={`px-2 py-1 text-right text-sm ${m === cur ? 'bg-accent/40' : ''} ${negative ? 'text-destructive' : ''}`}
            >
              {value !== undefined ? moneyFormat(value, currency, { showCurrency: false, useNativePrecision: false }) : '—'}
            </div>
          )
        })}
      </div>
    </div>
  )
}
```

The `data-col={i}` attributes are added now so Task 4 needs no second pass over this file.

- [ ] **Step 4: Update the call site**

In `PlanSheet.tsx`, replace the single `<TotalsFooter ... />` element (inside the `role="grid"` scroller, after the archived section) with:

```tsx
        <PlanTotals
          visibleMonths={visibleMonths}
          monthIndex={monthIndex}
          cur={cur}
          gridCols={gridCols}
          totals={totals}
          currency={planCurrency}
        />

        <PlanBalanceRow
          visibleMonths={visibleMonths}
          monthIndex={monthIndex}
          cur={cur}
          gridCols={gridCols}
          balance={balance}
          currency={planCurrency}
        />
```

Both stay inside the `role="grid"` scroll container — `PlanBalanceRow`'s `sticky` resolves against that scroller, so moving it outside would break the pinning.

- [ ] **Step 5: Fix the existing totals test**

In `PlanSheet.test.tsx`, in the test named `'totals block renders one effective value per cell for income/expenses/net, and the running balance'`, the Balance assertion now targets the new element. Change:

```tsx
  expect(within(totalsBlock).getByText('Balance')).toBeInTheDocument()
```

to:

```tsx
  expect(within(screen.getByTestId('plan-balance-row')).getByText('Balance')).toBeInTheDocument()
```

Leave every other line of that test alone — the `getAllByRole('row')` destructure and all computed-value assertions still hold, because `plan-totals` keeps exactly its three rows.

- [ ] **Step 6: Run the full plan sheet suite**

Run from `web/`: `pnpm test -- PlanSheet.test.tsx`
Expected: PASS, including the pre-existing balance tests that use `plan-balance-${i}`.

- [ ] **Step 7: Commit**

```bash
git add web/src/features/budgets/PlanSheet.tsx web/src/features/budgets/PlanSheet.test.tsx
git commit -m "feat(web): unpin the plan totals trio, keep only balance sticky"
```

---

### Task 2: Fold the mode toggle and hide-empty switch into the page header

**Files:**
- Modify: `web/src/features/budgets/BudgetPage.tsx` (header ~lines 536-599; standalone tablist ~lines 601-614; dropdown content ~lines 586-596)
- Modify: `web/src/features/budgets/PlanSheet.tsx` (toolbar div ~lines 1035-1046; imports lines 7-8; store hooks ~lines 690-691)
- Test: `web/src/features/budgets/BudgetPage.test.tsx`

**Interfaces:**
- Consumes: nothing from Task 1 (independent).
- Produces: no new exported symbols. Behavioural contract later tasks rely on: the mode toggle keeps `role="tab"` with the same accessible names, so `getByRole('tab', { name: /plan/i })` continues to work everywhere.

- [ ] **Step 1: Write the failing tests**

Append to `web/src/features/budgets/BudgetPage.test.tsx`:

```tsx
it('offers hide-empty in the settings menu only in plan mode', async () => {
  const user = userEvent.setup()
  renderPage()
  await screen.findByRole('tab', { name: /budget/i })

  // budget mode: the settings menu has no hide-empty entry
  await user.click(screen.getByRole('button', { name: /settings/i }))
  expect(screen.queryByRole('menuitemcheckbox', { name: /hide empty/i })).not.toBeInTheDocument()
  await user.keyboard('{Escape}')

  // plan mode: it is there, and toggling it flips the persisted store flag
  await user.click(screen.getByRole('tab', { name: /plan/i }))
  await user.click(screen.getByRole('button', { name: /settings/i }))
  const item = await screen.findByRole('menuitemcheckbox', { name: /hide empty/i })
  expect(useBudgetPeriodStore.getState().planHideEmpty).toBe(false)
  await user.click(item)
  expect(useBudgetPeriodStore.getState().planHideEmpty).toBe(true)
})

it('shows the currency pills in budget mode and hides them in plan mode', async () => {
  const user = userEvent.setup()
  renderPage()
  await screen.findByRole('tab', { name: /budget/i })

  expect(screen.getAllByRole('button', { name: /^currency /i }).length).toBeGreaterThan(0)

  await user.click(screen.getByRole('tab', { name: /plan/i }))
  expect(screen.queryByRole('button', { name: /^currency /i })).not.toBeInTheDocument()
})
```

If `useBudgetPeriodStore` is not already imported in this test file, add it:
`import { useBudgetPeriodStore } from './budgetStore'`.

Match the settings-button name to whatever `budgets.page.budget.settings.button` resolves to in the `en` catalogue — check with `grep -n '"button"' -A2 locales/en.json` under the budget settings key, and use that string in the `name:` matcher instead of `/settings/i` if it differs.

- [ ] **Step 2: Run tests to verify they fail**

Run from `web/`: `pnpm test -- BudgetPage.test.tsx -t 'hide-empty'`
Expected: FAIL — no `menuitemcheckbox` is found in plan mode.

- [ ] **Step 3: Scope the currency pills to budget mode**

In `BudgetPage.tsx`, wrap the currency-pill span. Change:

```tsx
        <span className="flex shrink-0 items-center gap-1">
          {budgetCurrencyIds.map((currencyId) => {
```

to:

```tsx
        {budgetMode === 'budget' ? (
          <span className="flex shrink-0 items-center gap-1">
            {budgetCurrencyIds.map((currencyId) => {
```

and close it after the `.map(...)` call — the existing `})}` and `</span>` become `})}
          </span>
        ) : null}`. Keep the `.map` body byte-identical; only the wrapper and indentation change.

- [ ] **Step 4: Move the mode toggle into the header**

In `BudgetPage.tsx`, delete the standalone block:

```tsx
      <div role="tablist" aria-label="budget mode" className="flex w-fit rounded-md border p-0.5">
        {(['budget', 'plan'] as const).map((m) => (
          ...
        ))}
      </div>
```

and insert it inside `<header>`, immediately before the `{editMode ? (` ternary (i.e. after `<span className="flex-1" />`), with `shrink-0` added so it does not squash:

```tsx
        <div role="tablist" aria-label="budget mode" className="flex w-fit shrink-0 rounded-md border p-0.5">
          {(['budget', 'plan'] as const).map((m) => (
            <button
              key={m}
              type="button"
              role="tab"
              aria-selected={budgetMode === m}
              className={`rounded px-3 py-1 text-sm uppercase tracking-wide ${budgetMode === m ? 'bg-accent font-bold' : 'text-muted-foreground'}`}
              onClick={() => setBudgetMode(m)}
            >
              {t(m === 'budget' ? 'budgets.page.plan.toggle.budget' : 'budgets.page.plan.toggle.plan')}
            </button>
          ))}
        </div>
```

- [ ] **Step 5: Add the hide-empty checkbox item to the settings menu**

In `BudgetPage.tsx`, extend the dropdown import on line 16:

```tsx
import { DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
```

Add the two store hooks next to the existing ones (~line 191):

```tsx
  const planHideEmpty = useBudgetPeriodStore((s) => s.planHideEmpty)
  const togglePlanHideEmpty = useBudgetPeriodStore((s) => s.togglePlanHideEmpty)
```

Then inside `<DropdownMenuContent align="end">`, before the existing `budget_list` item, add:

```tsx
              {budgetMode === 'plan' ? (
                <>
                  <DropdownMenuCheckboxItem checked={planHideEmpty} onCheckedChange={() => togglePlanHideEmpty()}>
                    {t('budgets.page.plan.density.hide_empty')}
                  </DropdownMenuCheckboxItem>
                  <DropdownMenuSeparator />
                </>
              ) : null}
```

- [ ] **Step 6: Delete the PlanSheet toolbar**

In `PlanSheet.tsx`, delete the entire toolbar div:

```tsx
      <div className="flex items-center gap-2 border-b px-2 py-1.5">
        <Switch ... />
        <Label ... >{t('budgets.page.plan.density.hide_empty')}</Label>
      </div>
```

Delete the now-unused store hook line `const togglePlanHideEmpty = useBudgetPeriodStore((s) => s.togglePlanHideEmpty)` (keep the `planHideEmpty` read — the sheet still filters by it). Delete the `Label` and `Switch` imports (lines 7-8) if `grep -n "Switch\|<Label" PlanSheet.tsx` shows no other use.

- [ ] **Step 7: Run the tests**

Run from `web/`: `pnpm test -- BudgetPage.test.tsx PlanSheet.test.tsx`
Expected: PASS. `PlanSheet.test.tsx:77`'s `getByRole('tab', { name: /plan/i })` still resolves because the toggle kept its role and labels.

- [ ] **Step 8: Commit**

```bash
git add web/src/features/budgets/BudgetPage.tsx web/src/features/budgets/PlanSheet.tsx web/src/features/budgets/BudgetPage.test.tsx
git commit -m "feat(web): fold plan mode toggle and hide-empty into the budget header"
```

---

### Task 3: Rule the rows flush inside the bands

**Files:**
- Modify: `web/src/features/budgets/PlanSheet.tsx` (`ElementRow` wrapper ~line 313-317; the two `<section>` band elements ~lines 1095 and 1147)
- Modify: `web/src/index.css` (append)

**Interfaces:**
- Consumes: nothing from Tasks 1-2 (independent).
- Produces: the CSS marker classes `.plan-band-income` / `.plan-band-expense` gain their first real rule. Task 4 appends to the same CSS block.

- [ ] **Step 1: Write the failing test**

Append to `web/src/features/budgets/PlanSheet.test.tsx`:

```tsx
it('rules element rows flush with hairline dividers', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  // bands no longer space their children apart
  const income = screen.getByTestId('plan-section-income')
  expect(income.className).not.toContain('gap-1')

  // rows carry a hairline divider and are no longer rounded cards
  const row = screen.getByTestId('plan-cell-pe1:0').closest('[role="row"]') as HTMLElement
  expect(row.className).toContain('border-b')
  expect(row.className).not.toContain('rounded-md')
})
```

- [ ] **Step 2: Run test to verify it fails**

Run from `web/`: `pnpm test -- PlanSheet.test.tsx -t 'hairline dividers'`
Expected: FAIL — the band className still contains `gap-1`.

- [ ] **Step 3: Close the row gaps on both bands**

In `PlanSheet.tsx`, on the income section, change:

```tsx
          className="plan-band-income flex flex-col gap-1 rounded-md bg-emerald-500/[0.06] px-1 py-1 dark:bg-emerald-400/[0.06]"
```

to (drop `gap-1` only):

```tsx
          className="plan-band-income flex flex-col rounded-md bg-emerald-500/[0.06] px-1 py-1 dark:bg-emerald-400/[0.06]"
```

And on the expense section, change:

```tsx
          className="plan-band-expense mt-2 flex flex-col gap-1 rounded-md border-t-2 border-border bg-rose-500/[0.04] px-1 py-1 dark:bg-rose-400/[0.04]"
```

to:

```tsx
          className="plan-band-expense mt-2 flex flex-col rounded-md border-t-2 border-border bg-rose-500/[0.04] px-1 py-1 dark:bg-rose-400/[0.04]"
```

Leave the tints and `rounded-md` on the bands themselves untouched.

- [ ] **Step 4: Rule the element rows**

In `ElementRow`, change the row wrapper:

```tsx
        className="grid items-center gap-1 rounded-md px-2 py-1.5 hover:bg-accent/50"
```

to:

```tsx
        className="grid items-center gap-1 border-b border-border/60 px-2 py-1.5"
```

The `hover:bg-accent/50` is removed here because Task 4 replaces row hover with a
CSS rule that also covers the column crosshair; if Task 4 is not being run, restore
`hover:bg-accent/50` to this class list.

- [ ] **Step 5: Suppress the trailing divider and rule the folder rows**

Append to `web/src/index.css`:

```css
/* Plan grid: rows sit flush and share hairline dividers, so the last row in a
   band must not draw a line across the band's rounded edge. */
.plan-band-income > :last-child,
.plan-band-expense > :last-child,
.plan-band-income [role='row']:last-child,
.plan-band-expense [role='row']:last-child {
  border-bottom: 0;
}
```

- [ ] **Step 6: Run the tests**

Run from `web/`: `pnpm test -- PlanSheet.test.tsx`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add web/src/features/budgets/PlanSheet.tsx web/src/features/budgets/PlanSheet.test.tsx web/src/index.css
git commit -m "feat(web): rule plan rows flush with hairline dividers"
```

---

### Task 4: Row and column hover crosshair

**Files:**
- Modify: `web/src/features/budgets/PlanSheet.tsx` (scroller container ~line 1083; `ChildRow` cell ~line 270-279; `ElementRow` cell ~line 358-367; month header cell ~line 1070-1079)
- Modify: `web/src/index.css` (append to the block from Task 3)
- Test: `web/src/features/budgets/PlanSheet.test.tsx`

**Interfaces:**
- Consumes: `data-col={i}` already added to the totals cells by Task 1 Step 3. If Task 1 has not run, add `data-col={i}` to those cells as part of this task.
- Produces: nothing exported. Contract: the grid container carries `data-hover-col={index}` while a month cell is hovered and drops the attribute on mouse leave; every month cell in every row type carries `data-col={i}`.

- [ ] **Step 1: Write the failing test**

Append to `web/src/features/budgets/PlanSheet.test.tsx`:

```tsx
it('tracks the hovered month column on the grid container', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  const grid = await screen.findByTestId('plan-sheet')

  expect(grid).not.toHaveAttribute('data-hover-col')

  const cell = screen.getByTestId('plan-cell-pe1:1')
  expect(cell).toHaveAttribute('data-col', '1')

  await user.hover(cell)
  expect(grid).toHaveAttribute('data-hover-col', '1')

  await user.unhover(cell)
  expect(grid).not.toHaveAttribute('data-hover-col')
})
```

- [ ] **Step 2: Run test to verify it fails**

Run from `web/`: `pnpm test -- PlanSheet.test.tsx -t 'hovered month column'`
Expected: FAIL — the cell has no `data-col` attribute.

- [ ] **Step 3: Add `data-col` to the three remaining cell sites**

In `ChildRow`, add `data-col={i}` beside the existing `data-month={m}`:

```tsx
            data-month={m}
            data-col={i}
```

In `ElementRow`, the same addition beside its `data-month={m}`:

```tsx
              data-month={m}
              data-col={i}
```

In the month header, the `.map` currently has no index — change
`{visibleMonths.map((m) => (` to `{visibleMonths.map((m, i) => (` and add the attribute:

```tsx
              data-month={m}
              data-col={i}
```

(The two totals blocks already got `data-col` in Task 1.)

- [ ] **Step 4: Track the hovered column on the container**

In `PlanSheet`, add this handler above the `return` (near the other `useCallback`s):

```tsx
  const [hoverCol, setHoverCol] = useState<number | null>(null)

  // One listener on the container instead of per-cell handlers: hovering only
  // rewrites an attribute, so the crosshair costs no re-render of the rows.
  const handleHover = useCallback((e: ReactPointerEvent<HTMLDivElement> | React.MouseEvent<HTMLDivElement>) => {
    const cell = (e.target as HTMLElement).closest('[data-col]')
    const raw = cell?.getAttribute('data-col')
    setHoverCol(raw === null || raw === undefined ? null : Number(raw))
  }, [])
```

Then on the `role="grid"` container add the attribute and the two listeners:

```tsx
        data-hover-col={hoverCol ?? undefined}
        onMouseOver={handleHover}
        onMouseLeave={() => setHoverCol(null)}
```

`data-hover-col={undefined}` omits the attribute entirely, which is what the test
asserts on leave.

- [ ] **Step 5: Add the hover CSS**

Append to `web/src/index.css`, after the Task 3 block:

```css
/* Plan grid hover crosshair. Each row is its own CSS grid, so a column has no
   element to hover — the container publishes the hovered index and cells match
   it by position. Twelve rules covers the widest month window. */
@media (hover: hover) {
  .plan-row:hover {
    background: color-mix(in srgb, var(--accent), transparent 40%);
  }

  [data-hover-col='0'] [data-col='0'],
  [data-hover-col='1'] [data-col='1'],
  [data-hover-col='2'] [data-col='2'],
  [data-hover-col='3'] [data-col='3'],
  [data-hover-col='4'] [data-col='4'],
  [data-hover-col='5'] [data-col='5'],
  [data-hover-col='6'] [data-col='6'],
  [data-hover-col='7'] [data-col='7'],
  [data-hover-col='8'] [data-col='8'],
  [data-hover-col='9'] [data-col='9'],
  [data-hover-col='10'] [data-col='10'],
  [data-hover-col='11'] [data-col='11'] {
    background: color-mix(in srgb, var(--accent), transparent 85%);
  }
}
```

Row hover is a class rather than Tailwind's `hover:` so it composes with the
column rule at the same specificity. Add `plan-row` to the `ElementRow` wrapper
class list from Task 3 Step 4:

```tsx
        className="plan-row grid items-center gap-1 border-b border-border/60 px-2 py-1.5"
```

- [ ] **Step 6: Verify tint ordering in the running app**

Start the dev server (`make web-run` from the repo root, or `pnpm dev` from `web/`)
and open a budget in plan mode. Confirm, in both light and dark mode:

1. Row hover is clearly visible; column hover is noticeably fainter.
2. A selected cell still reads as selected while hovered.
3. A fill-drag range still reads as covered while hovered.
4. The current-month column still reads as current.

If any ordering is violated, adjust only the two `transparent NN%` values — raise
the column percentage to weaken it, lower the row percentage to strengthen it.

- [ ] **Step 7: Run the tests and lint**

Run from `web/`: `pnpm test -- PlanSheet.test.tsx BudgetPage.test.tsx && pnpm lint`
Expected: PASS, no lint errors.

- [ ] **Step 8: Commit**

```bash
git add web/src/features/budgets/PlanSheet.tsx web/src/features/budgets/PlanSheet.test.tsx web/src/index.css
git commit -m "feat(web): highlight the hovered plan row and month column"
```

---

### Task 5: Full-suite verification

**Files:** none modified unless a regression surfaces.

**Interfaces:**
- Consumes: all of Tasks 1-4.
- Produces: a green suite.

- [ ] **Step 1: Run the whole frontend suite**

Run from `web/`: `pnpm test`
Expected: PASS. Pay attention to `BudgetTable.test.tsx` and `budgetDetail.test.tsx` —
they render `BudgetPage` and may assert on header layout or the currency pills.

- [ ] **Step 2: Fix any fallout**

If a test fails because the currency pills are absent, check whether it was
running in plan mode; if so, the test's expectation is now wrong and should
assert absence. If it fails on the header, update the query rather than
reverting the layout.

- [ ] **Step 3: Lint**

Run from `web/`: `pnpm lint`
Expected: no errors. Most likely finding is an unused `Switch`/`Label` import in
`PlanSheet.tsx` — delete it.

- [ ] **Step 4: Commit any fixes**

```bash
git add -A web/src
git commit -m "test(web): update budget view tests for the new plan chrome"
```

## Self-Review Notes

- **Spec coverage:** §1 totals split → Task 1. §2 header consolidation → Task 2.
  §3 ruled rows → Task 3. §4 hover → Task 4. Spec's "new coverage to add" list →
  the three new tests in Tasks 1, 2, 4. Spec's required test edit → Task 1 Step 5.
- **Spec correction:** the spec says "every month cell already carries
  `data-month`"; in fact only `ChildRow`, `ElementRow` and the month header do —
  the totals cells do not. The plan adds `data-col` to all five sites and does not
  depend on `data-month`, so the design is unaffected.
- **Cross-task coupling:** Task 3 removes `hover:bg-accent/50` and Task 4 restores
  hover via `.plan-row`. Task 3 Step 4 notes the restore if Task 4 is skipped.
  Task 1 pre-adds `data-col` to the totals; Task 4 Step 1 notes the fallback.
