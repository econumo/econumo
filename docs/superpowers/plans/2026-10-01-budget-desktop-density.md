# Budget Desktop/Tablet Density (Redesign Stage 3) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the desktop and tablet Budget and Plan views calmer: no currency symbol column, one emphasised number, colour only for problems, "No folder", and a way to see the Plan grid's transactions (the Uncategorized row included).

**Architecture:** Frontend only, in `web/src/features/budgets/`. `BudgetTable.tsx` (Budget view, desktop/tablet and the phone's edit mode) and `PlanSheet.tsx` (Plan grid) change their rendering. The row colour and bar rules already exist in `budgetMath.ts` (`overBudget`, `rowProgress`, `carryOver`) and the phone uses them; this stage applies them to the table. One new shared component, `CurrencyTag.tsx`.

**Tech Stack:** React 19, TypeScript, Tailwind v4, react-i18next, vitest + Testing Library + MSW, pnpm.

**Spec:** `docs/superpowers/specs/2026-09-29-budget-ux-redesign-design.md`, Part 3 ("Calmer desktop and tablet"), "Row state rule" and "Currency display rule".

## Global Constraints

- Frontend only. No endpoint, DTO, permission or analytics change; `metrics-coverage.test.ts` stays green.
- Branch `feature/budget-desktop-density`, from `origin/v1.6-dev`; the PR goes into `v1.6-dev`.
- The phone month view (`PhoneMonthView.tsx`, `ElementSheet.tsx`) is not changed by this stage.
- Amounts keep their decimals. Column structure, keyboard grid, fill handle, drag-and-drop and edit-structure mode stay as they are.
- Currency rule: budget-currency amounts carry no symbol; the budget currency code is shown once, at the left end of the column-heading row (Budget view) and at the left of the month header row (Plan grid); an element in another currency gets a small code tag next to its name.
- Row colour: the Spent figure and the bar are red only when `overBudget()` is true (this month spent more than its budget AND the displayed Available is below zero); every other row is gray. A future month (after the current month) has no Spent (`—`) and no bar. Income and savings rows are never coloured.
- Locales: `locales/*.json`, 11 files (`de en es fr it nl pl pt ru uk zh`). Edit them by hand (Edit tool or a targeted line edit); never re-serialise a whole file.
- Tests: run vitest from `web/` with `--maxWorkers=2`, e.g. `cd web && pnpm vitest run src/features/budgets/BudgetTable.test.tsx --maxWorkers=2`. `src/.../transaction.test.ts` has one known pre-existing failure (`Blob` instanceof); ignore it, nothing else may fail.
- i18n guard: `GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/test/i18ntest/` from the repo root.
- Comments: only for non-obvious "why" (see `CLAUDE.md` → "Comments — write sparingly"). Update any comment your change makes false (e.g. ones that mention the currency symbol column).
- `docs/regression-test-plan.md`: every task that changes what a user sees updates its items in the same commit.
- Commits end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus

1. **Edit mode alignment.** Removing the symbol column must not shift the amounts: in edit mode the folder header puts its "+" button in the w-6 slot the symbol used to take, so every element row, the column headings, read-only section headers and the Total row must pad that slot. Test: Task 1 (`edit mode pads the "+" slot on every row`).
2. **A blank plan cell must still be clickable.** A button with empty text collapses to zero height, so a blank planned amount must keep a hit area. Test: Task 4 (`a blank plan cell still opens the editor`).
3. **Negative Available without overspending this month** (an earlier month overspent): the Available is a red pill but the Spent and bar stay gray. Test: Task 2 (`an earlier overspend reds the Available, not the Spent`).
4. **Transfers line in a window where nothing crossed** must hide, but a month where money crossed and netted to zero must keep it. Test: Task 3 (`hides the transfers line only when nothing crossed in the window`).
5. **A foreign-currency element** shows its own code tag and its own amounts, while folder and grand totals stay in the budget currency without a symbol. Test: Task 1 (`tags a foreign-currency element with its code`) and Task 3 (`tags the foreign-currency plan row`).

---

### Task 1: Budget table — currency rule and "No folder"

Removes the currency symbol column from the Budget table, names the budget currency once in the column headings, tags foreign-currency elements, and renames the unfoldered bucket.

**Files:**
- Create: `web/src/features/budgets/CurrencyTag.tsx`
- Modify: `web/src/features/budgets/BudgetTable.tsx`
- Modify: `web/src/features/budgets/BudgetPage.tsx:578-585` (the `folderActions` comment and the no-folder name)
- Modify: `locales/{de,en,es,fr,it,nl,pl,pt,ru,uk,zh}.json` (delete `budgets.page.budget.structure.no_folder`)
- Test: `web/src/features/budgets/BudgetTable.test.tsx`, `web/src/features/budgets/PlanSheet.test.tsx:996`
- Docs: `docs/regression-test-plan.md`

**Interfaces:**
- Produces: `CurrencyTag({ code }: { code: string })` from `CurrencyTag.tsx`, rendering `<span data-testid="currency-tag">`. Task 3 uses it in the Plan grid.
- Produces: in `BudgetTable.tsx`, an `EditSlot` component (`data-testid="edit-slot"`, `hidden w-6 shrink-0 sm:block`). Task 2 keeps it in place.

- [ ] **Step 1: Write the failing tests**

In `BudgetTable.test.tsx`, replace every `'budget-folder-Default folder'` with `'budget-folder-No folder'`, and rename the two test titles that say "Default folder" to "No folder". Then add:

```tsx
it('names the budget currency once and shows no currency symbols', async () => {
  renderTable()
  await screen.findByTestId('budget-folder-Essentials')
  await waitFor(() => expect(screen.getByTestId('column-headers')).toHaveTextContent('USD'))
  const table = screen.getByTestId('budget-table')
  expect(table).not.toHaveTextContent('$')
  expect(table).not.toHaveTextContent('€')
})

it('tags a foreign-currency element with its code', async () => {
  renderTable()
  const living = await screen.findByTestId('element-env-1')
  await waitFor(() => expect(within(living).getByTestId('currency-tag')).toHaveTextContent('EUR'))
  expect(within(screen.getByTestId('element-cat-food')).queryByTestId('currency-tag')).not.toBeInTheDocument()
  // the folder line is already converted to the budget currency: no tag there
  const noFolder = screen.getByTestId('budget-folder-No folder')
  expect(within(within(noFolder).getByTestId('stat-line')).queryByTestId('currency-tag')).not.toBeInTheDocument()
})

it('edit mode pads the "+" slot on every row', async () => {
  renderTable(undefined, {
    renderActions: () => <button type="button">actions</button>,
    renderFolderActions: () => <span />,
  })
  const food = await screen.findByTestId('element-cat-food')
  expect(within(food).getAllByTestId('edit-slot')).toHaveLength(1)
  expect(within(screen.getByTestId('element-env-1')).getAllByTestId('edit-slot')).toHaveLength(1)
  expect(within(screen.getByTestId('column-headers')).getAllByTestId('edit-slot')).toHaveLength(1)
  expect(within(screen.getByTestId('budget-totals')).getAllByTestId('edit-slot')).toHaveLength(1)
})

it('outside edit mode there is no "+" slot', async () => {
  renderTable()
  await screen.findByTestId('element-cat-food')
  expect(screen.queryByTestId('edit-slot')).not.toBeInTheDocument()
})
```

In `PlanSheet.test.tsx` line 996, change `'create envelope Default folder'` to `'create envelope No folder'`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && pnpm vitest run src/features/budgets/BudgetTable.test.tsx --maxWorkers=2`
Expected: FAIL — no `budget-folder-No folder`, no `currency-tag`, no `edit-slot`, and `$` found in the table.

- [ ] **Step 3: Create `CurrencyTag.tsx`**

```tsx
/** marks an amount that is not in the budget currency (those carry no symbol) */
export function CurrencyTag({ code }: { code: string }) {
  return (
    <span data-testid="currency-tag" className="shrink-0 rounded bg-muted px-1 text-[10px] font-medium text-muted-foreground">
      {code}
    </span>
  )
}
```

- [ ] **Step 4: Change `BudgetTable.tsx`**

1. Add the slot component next to `ActionsSpacer`:

```tsx
/* edit mode puts the folder header's "+" in this slot, so every other row pads it
   to keep the amount columns aligned */
function EditSlot() {
  return <span data-testid="edit-slot" className="hidden w-6 shrink-0 sm:block" />
}
```

2. `StatCells`: delete the `hideSymbol` prop and the symbol `<span>`. At the call site, delete the `hideSymbol={...}` prop and its comment.
3. `ElementRow`:
   - Delete the symbol span (`<span className="hidden w-6 text-center text-xs text-muted-foreground sm:block">{currency?.symbol}</span>`).
   - Replace the actions line with:
     ```tsx
     {actionsColumn ? <EditSlot /> : null}
     {extras.renderActions ? extras.renderActions(element, bucket) : actionsColumn ? <ActionsSpacer /> : null}
     ```
   - In the child `<li>`, replace `<span className="hidden w-6 sm:block" />` with `{actionsColumn ? <EditSlot /> : null}`; keep the `ActionsSpacer` line after it.
   - After the name `<span className="truncate text-[15px]" ...>`, add the tag. Compute it at the top of the component:
     ```tsx
     const tag = element.currencyId && element.currencyId !== budget.meta.currencyId ? currency?.code : undefined
     ```
     and render `{tag ? <CurrencyTag code={tag} /> : null}` right after the name span (before the uncategorized `InfoNote`).
4. `LabelRow`: delete the symbol span in the row and `<span className="hidden w-6 sm:block" />` in the child rows. Labels are always in the budget currency, so they get no tag. (The labels section has never padded the edit-mode columns; leave that as it is.)
5. Column headings: the first span becomes the currency code, and the trailing spans become the edit-mode pads:
   ```tsx
   <span className="min-w-0 flex-1 truncate">{budgetCurrency?.code}</span>
   <span className="hidden w-24 text-right sm:block">{t('budgets.page.budget.structure.tab.budgeted')}</span>
   <span className="w-20 text-center sm:w-24">{t('budgets.page.budget.structure.tab.spent')}</span>
   <span className="w-20 text-center sm:w-24">{t('budgets.page.budget.structure.tab.available')}</span>
   {actionsColumn ? (
     <>
       <EditSlot />
       <ActionsSpacer />
     </>
   ) : null}
   ```
6. Read-only section header (Archive): replace `{isReadOnlySection && actionsColumn ? <ActionsSpacer /> : null}` with:
   ```tsx
   {isReadOnlySection && actionsColumn ? (
     <>
       <EditSlot />
       <ActionsSpacer />
     </>
   ) : null}
   ```
7. Sections: the no-folder entry uses `t('budgets.page.plan.menu.no_folder')` instead of `t('budgets.page.budget.structure.no_folder')`. Update the comment there that says "the empty Default folder" to "the empty No folder bucket".
8. `BudgetTotals` (the desktop row): delete the symbol span; replace `{actionsColumn ? <ActionsSpacer /> : null}` with `{actionsColumn ? (<><EditSlot /><ActionsSpacer /></>) : null}`.
9. Update the `LabelRow` doc comment, which describes a `[symbol w-6]` column and "gets a currency symbol like every neighbouring row": drop both mentions.

- [ ] **Step 5: Change `BudgetPage.tsx`**

In `folderActions`: change `t('budgets.page.budget.structure.no_folder')` to `t('budgets.page.plan.menu.no_folder')`, and change the comment above it to: `// In edit mode the plus takes the w-6 slot every row pads (EditSlot in BudgetTable), so the stat columns line up with the element rows; folder ordering moved to dragging.` Update the `renderFolderActions` comment at the `<BudgetTable` call ("its presence also swaps the folder currency symbol for the plus slot") to `// only in edit mode`.

- [ ] **Step 6: Delete the locale key**

In each of the 11 `locales/*.json`, delete the line `"no_folder": "…",` inside `budgets.page.budget.structure` (the one followed by `"in_archive"`). Do NOT touch `budgets.page.plan.menu.no_folder`. Verify: `grep -c '"no_folder"' locales/*.json` prints `1` for every file.

- [ ] **Step 7: Run the tests**

Run: `cd web && pnpm vitest run src/features/budgets --maxWorkers=2`
Expected: PASS. Fix any other assertion in the budgets suites that still expects "Default folder" or a currency symbol in the table.
Run: `GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/test/i18ntest/` (repo root). Expected: `ok`.

- [ ] **Step 8: Update the regression plan**

In `docs/regression-test-plan.md`, section "## 9. Budgets — table & plan", add after the "budget header shows no currency chips" item:

```markdown
- [ ] 📱 Budget view (desktop and tablet): no amount carries a currency
      symbol; the budget currency code (e.g. "USD") sits once at the left of
      the column headings. An envelope in another currency shows a small code
      tag next to its name ("Travel EUR") and its amounts in that currency;
      folder lines and the Total stay in the budget currency. Items outside
      any folder are listed under "No folder". In Edit structure mode the
      folder "+" buttons and the row menus keep every amount column aligned.
```

- [ ] **Step 9: Commit**

```bash
git add web/src/features/budgets/CurrencyTag.tsx web/src/features/budgets/BudgetTable.tsx web/src/features/budgets/BudgetTable.test.tsx web/src/features/budgets/BudgetPage.tsx web/src/features/budgets/PlanSheet.test.tsx locales docs/regression-test-plan.md
git commit -m "feat: Budget table names its currency once and drops the symbol column

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Budget table — one emphasised number, colour only for problems

Available becomes the plain, semibold figure (a red pill only when negative); Spent and a thin bar follow the shipped row colour rule; future months show no Spent; folder headers become a tinted band; the page title becomes normal case.

**Files:**
- Modify: `web/src/features/budgets/BudgetTable.tsx`
- Modify: `web/src/features/budgets/BudgetPage.tsx:673-675` (title)
- Test: `web/src/features/budgets/BudgetTable.test.tsx`, `web/src/features/budgets/BudgetPage.test.tsx`
- Docs: `docs/regression-test-plan.md`

**Interfaces:**
- Consumes: `overBudget(row, future)`, `rowProgress(row, future)`, `carryOver(el)`, `displayAvailable(el)` from `budgetMath.ts` (already exported); `currentMonth()` from `planMath.ts` (returns `'YYYY-MM-01'`); `EditSlot` from Task 1.
- Produces: `AvailableFigure({ available, currency, testId?, className? })` exported from `BudgetTable.tsx`, replacing `AvailablePill` (which has no other users).

- [ ] **Step 1: Write the failing tests**

Add to `BudgetTable.test.tsx` (the fixture's Food row: budgeted 200, spent 45.5, wire available 154.5, so displayed Available 354.50 and carry 200):

```tsx
const food = (budget: BudgetDto) => budget.structure.elements.find((el) => el.id === 'cat-food')!

it('shows Available as plain emphasised text when it is not negative', async () => {
  renderTable()
  const row = await screen.findByTestId('element-cat-food')
  const available = within(row).getByTestId('cell-available')
  await waitFor(() => expect(available).toHaveTextContent('354.50'))
  expect(available).toHaveClass('font-semibold')
  expect(available).not.toHaveClass('text-income')
  expect(available).not.toHaveClass('bg-expense/10')
})

it('an earlier overspend reds the Available, not the Spent', async () => {
  // spent 45.5 of 200 this month, but earlier months left -260: Available -60
  renderTable(
    (b) => {
      food(b).available = '-260'
    },
    { onSpentClick: () => {} },
  )
  const row = await screen.findByTestId('element-cat-food')
  await waitFor(() => expect(within(row).getByTestId('cell-available')).toHaveTextContent('-60.00'))
  expect(within(row).getByTestId('cell-available')).toHaveClass('bg-expense/10')
  expect(within(row).getByRole('button', { name: 'transactions Food' })).not.toHaveClass('text-expense')
})

it('reds the Spent and the bar when this month went over budget and nothing covers it', async () => {
  renderTable(
    (b) => {
      Object.assign(food(b), { spent: '250', budgetSpent: '250', available: '-250' })
    },
    { onSpentClick: () => {} },
  )
  const row = await screen.findByTestId('element-cat-food')
  await waitFor(() => expect(within(row).getByRole('button', { name: 'transactions Food' })).toHaveClass('text-expense'))
  expect(within(row).getByTestId('row-progress').firstElementChild).toHaveClass('bg-expense')
})

it('keeps an overspend covered by earlier months gray', async () => {
  // 250 spent of 200, but 200 left from earlier months: Available +150
  renderTable(
    (b) => {
      Object.assign(food(b), { spent: '250', budgetSpent: '250', available: '-50' })
    },
    { onSpentClick: () => {} },
  )
  const row = await screen.findByTestId('element-cat-food')
  await waitFor(() => expect(within(row).getByTestId('cell-available')).toHaveTextContent('150.00'))
  expect(within(row).getByRole('button', { name: 'transactions Food' })).not.toHaveClass('text-expense')
  expect(within(row).getByTestId('row-progress').firstElementChild).not.toHaveClass('bg-expense')
})

it('draws the bar against the budget plus what earlier months left', async () => {
  renderTable()
  const row = await screen.findByTestId('element-cat-food')
  // 45.5 / (200 + 200) = 11.4%
  await waitFor(() => expect(within(row).getByTestId('row-progress').firstElementChild).toHaveStyle({ width: '11%' }))
})

it('a future month shows no Spent and no bar', async () => {
  renderTable((b) => {
    b.filters.periodStart = '2099-01-01 00:00:00'
  })
  const row = await screen.findByTestId('element-cat-food')
  expect(within(row).getByTestId('cell-spent')).toHaveTextContent('—')
  expect(within(row).queryByTestId('row-progress')).not.toBeInTheDocument()
})

it('the uncategorized row has no bar', async () => {
  renderTable((b) => {
    b.structure.elements.push({
      id: UNCATEGORIZED_ID, type: 1, name: 'Uncategorized', icon: 'question_mark', currencyId: null, isArchived: 0,
      folderId: null, position: 9, budgeted: '0', available: '-12', spent: '12', budgetSpent: '12', ownerUserId: null, children: [],
    } as BudgetElementDto)
  })
  const row = await screen.findByTestId(`element-${UNCATEGORIZED_ID}`)
  expect(within(row).queryByTestId('row-progress')).not.toBeInTheDocument()
})

it('folder headers are a tinted band with semibold totals', async () => {
  renderTable()
  const essentials = await screen.findByTestId('budget-folder-Essentials')
  const header = within(essentials).getByTestId('folder-header')
  expect(header).toHaveClass('bg-muted/60')
  expect(within(header).getByTestId('stat-line')).toHaveClass('font-semibold')
})

it('the Total row shows Available as plain emphasised text', async () => {
  renderTable()
  const totals = await screen.findByTestId('budget-totals')
  const available = within(totals).getByTestId('totals-available')
  expect(available).toHaveClass('font-semibold')
  expect(available).not.toHaveClass('bg-income/10')
})
```

If an existing test in this file builds the uncategorized element differently, reuse its shape instead of the literal above. If `ElementRowExtras.onSpentClick` is not given, the Spent figure is a `<span>`, so the colour tests pass `onSpentClick`.

Add to `BudgetPage.test.tsx` (use that file's existing desktop render helper and fixtures; the budget is named "Main budget"):

```tsx
it('shows the budget name in normal case', async () => {
  // render /budget the way the file's other desktop tests do
  const heading = await screen.findByRole('heading', { name: 'Main budget' })
  expect(heading).not.toHaveClass('uppercase')
  expect(heading).toHaveClass('text-lg')
})
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && pnpm vitest run src/features/budgets/BudgetTable.test.tsx src/features/budgets/BudgetPage.test.tsx --maxWorkers=2`
Expected: FAIL — no `row-progress`, no `folder-header`, no `totals-available`, Available still a green pill.

- [ ] **Step 3: Replace `AvailablePill` with `AvailableFigure`**

```tsx
/* the view's one emphasised number: plain unless it is negative */
export function AvailableFigure({
  available,
  currency,
  testId,
  className = 'text-[15px]',
}: {
  available: string
  currency: CurrencyDto | undefined
  testId?: string
  className?: string
}) {
  const negative = cmp(available, '0') < 0
  return (
    <span
      data-testid={testId}
      className={`inline-flex items-center font-semibold tabular-nums ${className} ${negative ? 'rounded-full bg-expense/10 px-2 py-0.5 text-expense' : ''}`}
    >
      {moneyFormat(available, currency, cellOpts(currency))}
    </span>
  )
}
```

Replace each `AvailablePill` use: the element row (`testId="cell-available"`), the desktop Total row (`testId="totals-available"`), and the phone Total card in `BudgetTotals` (no testId).

- [ ] **Step 4: Colour, bar and future month in `ElementRow`**

At the top of `ElementRow`, after `carry`:

```tsx
const future = budget.filters.periodStart.slice(0, 7) > currentMonth().slice(0, 7)
const figures = { budgeted: element.budgeted, spent: element.spent, available, carry }
const overspent = !isUncategorized && overBudget(figures, future)
const progress = isUncategorized ? null : rowProgress(figures, future)
```

(`isUncategorized` is declared a few lines below today; move its declaration above this block.) Import `overBudget, rowProgress` from `./budgetMath` and `currentMonth` from `./planMath`.

`spentCell` takes the tone and shows a dash in a future month. Replace `text-muted-foreground` in both branches with `${tone}` and render the dash:

```tsx
const spentCell = (target: BudgetTransactionsTarget, spent: string, tone = 'text-muted-foreground') => {
  if (future) {
    return <span className="w-20 text-center text-[15px] tabular-nums text-muted-foreground sm:w-24">{EMPTY_CELL}</span>
  }
  return extras.onSpentClick ? (
    <button
      type="button"
      title={showTransactionsTitle}
      aria-label={`transactions ${target.name}`}
      className={`w-20 text-center text-[15px] tabular-nums underline-offset-2 hover:text-foreground hover:underline sm:w-24 ${tone}`}
      onClick={() => extras.onSpentClick!(target)}
    >
      {moneyFormat(spent, currency, opts)}
    </button>
  ) : (
    <span className={`w-20 text-center text-[15px] tabular-nums sm:w-24 ${tone}`}>{moneyFormat(spent, currency, opts)}</span>
  )
}
```

The element's own Spent passes `overspent ? 'text-expense' : 'text-muted-foreground'`; child rows keep the default.

Below the row's flex line (the `<div className="flex items-center gap-1.5 rounded-md ...">`), still inside `data-testid={`element-${element.id}`}`, add the bar. It spans the row, as on the phone: the Budget cell widens per row when there is a carry-over, so a bar under the name alone would be a different length on every row.

```tsx
{progress !== null ? (
  <span data-testid="row-progress" className="mx-1.5 -mt-1.5 mb-1 h-1 overflow-hidden rounded-full bg-muted sm:mx-2">
    <span
      className={`block h-full rounded-full ${overspent ? 'bg-expense' : 'bg-muted-foreground/40'}`}
      style={{ width: `${Math.round(progress * 100)}%` }}
    />
  </span>
) : null}
```

- [ ] **Step 5: Folder band, spacing and the Total row**

- `StatCells` wrapper: `className="flex items-center gap-2 text-[13px] font-semibold text-muted-foreground"`; replace its Available span with `<span className="flex w-20 justify-center sm:w-24"><AvailableFigure available={available} currency={currency} className="text-[13px]" /></span>`.
- Folder `<header>`: add `data-testid="folder-header"` and change its class to `flex items-center gap-1.5 rounded bg-muted/60 px-1.5 py-1.5 mb-1 sm:gap-2 sm:px-2`; the name span gets `font-semibold` instead of `font-medium`.
- The reporting-tags folder header row (`ReportingTagsFolder`, the `div` holding the trigger) gets the same band classes (`rounded bg-muted/60 py-1.5 mb-1`), and its heading `font-semibold`.
- The table container: `gap-3` → `gap-5`.
- Desktop Total row: the Available span becomes `<span className="flex w-24 justify-center"><AvailableFigure available={totals.available} currency={budgetCurrency} testId="totals-available" /></span>`.

- [ ] **Step 6: Title**

In `BudgetPage.tsx`, the `<h1>` className becomes `'min-w-0 shrink truncate text-lg font-medium'` for every viewport (drop the `isPhone ?` branch).

- [ ] **Step 7: Run the tests**

Run: `cd web && pnpm vitest run src/features/budgets --maxWorkers=2`
Expected: PASS. Update existing assertions that expected the green pill (`text-income` on Available) or `0.00` Spent in a future month.

- [ ] **Step 8: Update the regression plan**

In section 9, add:

```markdown
- [ ] 📱 Budget view (desktop and tablet): Available is the one bold figure,
      plain when zero or above and a red pill when below zero; there are no
      green pills. Spent and the thin bar under each row are gray, and turn
      red only when the month spent more than its budget and earlier months
      do not cover it (Available below zero). A row whose earlier months
      overspent but which is within this month's budget shows a red
      Available and a gray Spent. The bar fills against budget plus what
      earlier months left. A future month shows "—" for Spent and no bars.
      Folder headers are a tinted band with bold totals; the budget name is
      in normal case.
```

- [ ] **Step 9: Commit**

```bash
git add web/src/features/budgets/BudgetTable.tsx web/src/features/budgets/BudgetTable.test.tsx web/src/features/budgets/BudgetPage.tsx web/src/features/budgets/BudgetPage.test.tsx docs/regression-test-plan.md
git commit -m "feat: Calmer Budget table — Available emphasised, red only for real overspending

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Plan grid — currency rule and the Transfers line

**Files:**
- Modify: `web/src/features/budgets/planMath.ts:46-63` (tail constants, `planVisibleCount`)
- Modify: `web/src/features/budgets/PlanSheet.tsx` (`ElementRow` name and tail, `tailPx`, month header row, `PlanTotals`)
- Test: `web/src/features/budgets/planMath.test.ts:113-129`, `web/src/features/budgets/PlanSheet.test.tsx`
- Docs: `docs/regression-test-plan.md`

**Interfaces:**
- Consumes: `CurrencyTag` from `./CurrencyTag` (Task 1).
- Produces: `PLAN_CURRENCY_COL_PX` is deleted; the tail track is `editMode ? PLAN_ACTIONS_COL_PX : 0`.

- [ ] **Step 1: Write the failing tests**

`planMath.test.ts`: remove `PLAN_CURRENCY_COL_PX` from the import list and replace the `planVisibleCount` test with:

```ts
it('planVisibleCount: 3..12 fit, collapse below 3, cap at 12', () => {
  // Derived from the constants so widening a fixed column cannot silently drift.
  // `fixed` is everything that is not a month: name + the row's px-2, plus the
  // leading gap; `month` is a month column plus its own gap.
  const fixed = PLAN_NAME_COL_PX + 16 + 4
  const month = PLAN_MIN_MONTH_COL_PX + 4
  expect(planVisibleCount(fixed + month * 2)).toBe(1) // only 2 fit -> mobile collapse
  expect(planVisibleCount(fixed + month * 3)).toBe(3)
  expect(planVisibleCount(fixed + month * 7 + 50)).toBe(7)
  expect(planVisibleCount(fixed + month * 40)).toBe(12)

  // edit mode adds the actions slot; months must not be measured against space it
  // takes, or they stretch and the window silently narrows
  expect(planVisibleCount(fixed + month * 8, true)).toBe(7)
  expect(planVisibleCount(fixed + PLAN_ACTIONS_COL_PX + month * 8, true)).toBe(8)
  expect(planVisibleCount(fixed + month * 8)).toBe(8)
})
```

`PlanSheet.test.tsx`, add:

```tsx
it('names the budget currency once and shows no currency symbols', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-05-01' })
  renderPage()
  const sheet = await screen.findByTestId('plan-sheet')
  await waitFor(() => expect(screen.getByTestId('plan-currency-code')).toHaveTextContent('USD'))
  expect(sheet).not.toHaveTextContent('$')
  expect(sheet).not.toHaveTextContent('€')
})

it('tags the foreign-currency plan row', async () => {
  usePlanHandlers()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const eurRow = document.querySelector('[data-row-id="env-eur:0"]') as HTMLElement
  await waitFor(() => expect(within(eurRow).getByTestId('currency-tag')).toHaveTextContent('EUR'))
  const foodRow = document.querySelector('[data-row-id="cat-food:1"]') as HTMLElement
  expect(within(foodRow).queryByTestId('currency-tag')).not.toBeInTheDocument()
})

it('hides the transfers line only when nothing crossed in the window', async () => {
  usePlanHandlers()
  // window from July: the fixture's only transfers are in June
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-07-01' })
  renderPage()
  await screen.findByTestId('plan-sheet')
  const totals = screen.getByTestId('plan-totals')
  expect(within(totals).queryByText('Transfers')).not.toBeInTheDocument()
  // back one month: June is in view again
  await userEvent.setup().click(screen.getByRole('button', { name: 'Earlier months' }))
  await waitFor(() => expect(within(screen.getByTestId('plan-totals')).getByText('Transfers')).toBeInTheDocument())
})
```

Then add a second case: build the plan handler with a June transfer item of `{ in: '50', out: '50' }` (net 0) and window from June — the Transfers line must stay. Follow how other tests in this file override `fixtureWirePlan` for a single test (search for `planHandler(` with an argument); if there is no such override helper, use `server.use(http.get('*/api/v1/budget/get-budget-plan', ...))` with a cloned fixture.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && pnpm vitest run src/features/budgets/planMath.test.ts src/features/budgets/PlanSheet.test.tsx --maxWorkers=2`
Expected: FAIL — `PLAN_CURRENCY_COL_PX` still counted, no `plan-currency-code`, `$` in the sheet, Transfers always listed.

- [ ] **Step 3: `planMath.ts`**

Delete `PLAN_CURRENCY_COL_PX` and its doc comment; give `PLAN_ACTIONS_COL_PX` this comment: `/** the trailing track: the actions menu in edit mode, nothing otherwise. Months must not be measured against space it occupies, or they stretch and the visible window silently narrows. */`. In `planVisibleCount`: `const tail = editMode ? PLAN_ACTIONS_COL_PX : 0`. Update the `PLAN_ROW_PADDING_PX` comment so it no longer mentions "the currency track".

- [ ] **Step 4: `PlanSheet.tsx`**

1. Imports: drop `PLAN_CURRENCY_COL_PX`; add `import { CurrencyTag } from './CurrencyTag'`.
2. `tailPx`: `const tailPx = editMode ? PLAN_ACTIONS_COL_PX : 0`. Rewrite the comment above it: `// The trailing track holds the actions menu in edit mode. Every grid consumer (rows, month header, totals, balance) shares this string, so they gain the column together and stay aligned.`
3. `ElementRow` name: after the name `<span className="truncate text-sm" ...>`, add `{el.currencyId !== ctx.meta.currencyId && currency ? <CurrencyTag code={currency.code} /> : null}`.
4. `ElementRow` tail: replace the trailing `div` and its comment with:
   ```tsx
   {/* trailing track: the actions menu in edit mode */}
   <div className="flex items-center justify-end">
     {ctx.editMode && !isUncategorized ? <RowMenu el={el} ctx={ctx} /> : null}
   </div>
   ```
5. Month header row, first cell: put the code at the left and push the arrows right:
   ```tsx
   <div className="flex items-center gap-1 px-2">
     <span data-testid="plan-currency-code" className="text-xs uppercase tracking-wide text-muted-foreground">
       {planCurrency?.code}
     </span>
     <span className="flex-1" />
     {/* the existing prev / next buttons, unchanged */}
   </div>
   ```
   (`planCurrency` is the budget-currency variable the component already passes to `PlanTotals`; if it is declared after this JSX's data, use it as is — it is in component scope.)
6. `PlanTotals`: list the transfers spec only when some visible month moved money across the budget boundary:
   ```tsx
   const transfersInView = visibleMonths.some((m) => {
     const idx = monthIndex(m)
     const row = idx >= 0 ? totals[idx] : undefined
     return row !== undefined && !(isZero(row.transfersIn) && isZero(row.transfersOut))
   })
   const specs = [...TOTALS_ROWS.filter((s) => s.key !== 'transfers' || transfersInView), ...(showSavings ? [SAVINGS_TOTALS_ROW] : [])]
   ```

The income "Uncategorized" row already hides when its actual is zero in every visible month (`shownRows` → `visibleUncat`); leave it.

- [ ] **Step 5: Run the tests**

Run: `cd web && pnpm vitest run src/features/budgets --maxWorkers=2`
Expected: PASS. Existing tests that read `plan-totals-transfers-*` in a window without transfers, or count totals rows (e.g. `totalRows[2]` near the "transfers line" test), must be updated to the new row list.

- [ ] **Step 6: Update the regression plan**

Replace the "**Plan sheet** 📱" item's last line ("transfers/balance totals rows show tooltips.") with:

```markdown
      scrolling; hide-empty-rows toggle; the Transfers line appears only when
      money crossed the budget boundary in a visible month and shows the in/out
      split as a tooltip; the Balance row shows tooltips. No amount carries a
      currency symbol: the budget currency code sits at the left of the month
      header, and a row in another currency has a code tag next to its name.
```

- [ ] **Step 7: Commit**

```bash
git add web/src/features/budgets/planMath.ts web/src/features/budgets/planMath.test.ts web/src/features/budgets/PlanSheet.tsx web/src/features/budgets/PlanSheet.test.tsx docs/regression-test-plan.md
git commit -m "feat: Plan grid names its currency once; Transfers line only when money crossed

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Plan grid — actuals, blank cells, colour and the current month

Future months show only the plan; an unplanned month is blank; an actual is red only over plan (no green); the current month's column is tinted.

**Files:**
- Modify: `web/src/features/budgets/planMath.ts:155-175` (`isOverspent`, delete `isUnderspent`; add `PLAN_CURRENT_MONTH_TINT`)
- Modify: `web/src/features/budgets/LimitEditor.tsx`
- Modify: `web/src/features/budgets/PlanSheet.tsx` (`ChildRow`, `ElementRow` cells, month header, `PlanTotals`, `PlanBalanceLine`)
- Test: `web/src/features/budgets/planMath.test.ts`, `web/src/features/budgets/PlanSheet.test.tsx:140-160`
- Docs: `docs/regression-test-plan.md` (section 9, the savings item near line 752)

**Interfaces:**
- Produces: `PLAN_CURRENT_MONTH_TINT = 'bg-muted/50'` exported from `planMath.ts`; `LimitEditor` gains `blankWhenZero?: boolean`.

- [ ] **Step 1: Write the failing tests**

`planMath.test.ts`: delete the whole `describe('isUnderspent', ...)` block and `isUnderspent` from the imports; in `describe('isOverspent')`, add:

```ts
it('never flags a savings row', () => {
  expect(isOverspent(BudgetElementType.SAVINGS, { actual: '600', planned: '500' })).toBe(false)
})
```

`PlanSheet.test.tsx`: replace the test `'overspend turns the actual red in a past month and with no plan set; never on income'` with:

```tsx
it('an actual turns red only over plan; under plan and income stay plain', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-05-01' })
  renderPage()
  await screen.findByText(/may/i)
  // July: 125 spent, no plan stored
  const foodJuly = screen.getAllByTestId('plan-cell-cat-food:2')[0]
  expect(within(foodJuly).getByTestId('cell-actual')).toHaveClass('text-destructive')
  // May: 120 spent against a 150 plan — under plan is not a signal
  const foodMay = screen.getAllByTestId('plan-cell-cat-food:0')[0]
  expect(within(foodMay).getByTestId('cell-actual')).not.toHaveClass('text-destructive')
  expect(within(foodMay).getByTestId('cell-actual')).not.toHaveClass('text-income')
  const freelanceMay = screen.getAllByTestId('plan-cell-cat-freelance:0')[0]
  expect(within(freelanceMay).getByTestId('cell-actual')).not.toHaveClass('text-destructive')
  const eurJune = screen.getAllByTestId('plan-cell-env-eur:1')[0]
  expect(within(eurJune).getByTestId('cell-actual')).not.toHaveClass('text-income')
})

it('a future month shows only the plan', async () => {
  vi.setSystemTime(new Date(2026, 6, 15, 12, 0, 0)) // July is current, August is future
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-05-01' })
  renderPage()
  await screen.findByText(/may/i)
  expect(within(screen.getAllByTestId('plan-cell-pe1:2')[0]).getByTestId('cell-actual')).toBeInTheDocument()
  expect(within(screen.getAllByTestId('plan-cell-pe1:3')[0]).queryByTestId('cell-actual')).not.toBeInTheDocument()
  // a child row's future cell is empty too
  await userEvent.setup().click(screen.getAllByRole('button', { name: /expand/i })[0])
  const rentAug = await screen.findByTestId('plan-cell-cat-rent:3')
  expect(rentAug.textContent).toBe('')
})

it('an unplanned month is blank, not 0.00', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-05-01' })
  renderPage()
  await screen.findByText(/may/i)
  // Food: May planned 150, July unset
  expect(within(screen.getAllByTestId('plan-cell-cat-food:0')[0]).getByTestId('cell-planned')).toHaveTextContent('150.00')
  expect(within(screen.getAllByTestId('plan-cell-cat-food:2')[0]).getByTestId('cell-planned').textContent).toBe('')
})

it('a blank plan cell still opens the editor', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-05-01' })
  renderPage()
  await screen.findByText(/may/i)
  const user = userEvent.setup()
  const foodJuly = screen.getAllByTestId('plan-cell-cat-food:2')[0]
  const trigger = within(foodJuly).getByRole('button', { name: 'limit Food' })
  expect(trigger).toHaveClass('min-h-5')
  await user.click(trigger)
  expect(await screen.findByRole('textbox')).toBeInTheDocument()
})

it('tints the current month column', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-05-01' })
  renderPage()
  await screen.findByText(/may/i)
  // system time is mid-August: column 3
  expect(screen.getByRole('columnheader', { name: /aug/i })).toHaveClass('bg-muted/50')
  expect(screen.getAllByTestId('plan-cell-cat-food:3')[0]).toHaveClass('bg-muted/50')
  expect(screen.getAllByTestId('plan-cell-cat-food:0')[0]).not.toHaveClass('bg-muted/50')
})
```

Adjust to the file's conventions where they differ: the expand button is named "Expand" (`common.button.expand.label`), and check how other tests in the file find the open limit editor (search `limit Food` / `limit ` in the file; reuse its assertion if it is not a `textbox`).

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && pnpm vitest run src/features/budgets/planMath.test.ts src/features/budgets/PlanSheet.test.tsx --maxWorkers=2`
Expected: FAIL — under-plan cells still `text-income`, future cells still render `—`, unset cells show `0.00`, no tint.

- [ ] **Step 3: `planMath.ts`**

```ts
/** the overspend highlight: an expense actual past its plan, in ANY month — an unset
 *  plan reads as 0 everywhere else in the grid, so it counts as 0 here too. Income
 *  and savings are never coloured: more received or saved is no problem. */
export function isOverspent(type: BudgetElementType, cell: PlanCellDto | undefined): boolean {
  if (!cell || isIncomeType(type) || type === BudgetElementType.SAVINGS) {
    return false
  }
  return cmp(cell.actual, cell.planned === '' ? '0' : cell.planned) > 0
}

export const PLAN_CURRENT_MONTH_TINT = 'bg-muted/50'
```

Delete `isUnderspent` and its comment.

- [ ] **Step 4: `LimitEditor.tsx`**

Add `blankWhenZero?: boolean` to the props. The trigger button:

```tsx
<button
  type="button"
  data-limit-trigger=""
  className="min-h-5 w-full text-right underline-offset-2 hover:underline"
  aria-label={`limit ${name}`}
>
  {blankWhenZero && isZero(currentValue) ? '' : moneyFormat(currentValue, currency, { showCurrency: false, useNativePrecision: false })}
</button>
```

- [ ] **Step 5: `PlanSheet.tsx`**

1. Imports: drop `isUnderspent`; add `PLAN_CURRENT_MONTH_TINT`.
2. Month header cell: append `${m === cur ? ` ${PLAN_CURRENT_MONTH_TINT}` : ''}` to its className.
3. `ChildRow`: for `m > ctx.cur` render no actual (the cell stays, empty):
   ```tsx
   {m > ctx.cur ? null : <span data-testid="cell-actual">{actualText}</span>}
   ```
   and add `${m === ctx.cur ? ` ${PLAN_CURRENT_MONTH_TINT}` : ''}` to the cell className. The aria-label keeps `actual: actualText`.
4. `ElementRow` cells:
   ```tsx
   const future = m > ctx.cur
   const plannedSet = !!cell && cell.planned !== '' && !isZero(cell.planned)
   const plannedText = plannedSet ? moneyFormat(cell!.planned, currency, { showCurrency: false, useNativePrecision: false }) : ''
   ```
   - Delete `underspend`; the actual's class becomes `` `text-xs ${overspend ? 'text-destructive' : 'text-muted-foreground'}` ``.
   - Render the actual only when `!future`.
   - The aria-label passes `planned: plannedText || '—'` and, for a future month, `actual: '—'`.
   - Compact branch: `{ctx.isCompact ? plannedText : editable ? (<LimitEditor ... blankWhenZero />) : ...}`.
   - The non-editable comments button gets `min-h-5` in its className, so a blank cell can still be clicked.
   - The cell className: add `${m === ctx.cur && !filled ? ` ${PLAN_CURRENT_MONTH_TINT}` : ''}` (the fill highlight wins over the tint).
   - Delete the now-unused `plannedValue` if nothing else reads it.
5. `PlanTotals` cells and `PlanBalanceLine` cells: add the same tint for `m === cur`. `PlanTotals` has no `cur` prop today; add `cur: string` and pass `cur={cur}` at the call site.
6. `renderActual`'s comment: it now only serves past and current months; change "A future month with no activity yet reads as a dash, same as a missing cell; a real (possibly zero) actual in a past/current month still prints." to "A missing cell reads as a dash; a real (possibly zero) actual still prints." and drop the `month > cur` branch only if no caller can still pass a future month (both callers now skip future months).

- [ ] **Step 6: Run the tests**

Run: `cd web && pnpm vitest run src/features/budgets --maxWorkers=2`
Expected: PASS. Update existing assertions that expected `0.00` in an unset cell, `—` for a future actual, or `text-income` (search the budgets tests for `text-income`, `'0.00'` next to `cell-planned`, and `cell-actual` in future columns; `PlanSheet.savings.test.tsx` included).

- [ ] **Step 7: Update the regression plan**

In section 9, add:

```markdown
- [ ] 📱 Plan grid: past months and the current month show the small actual
      above the plan; future months show only the plan. A month with no plan
      is blank (not 0.00) and clicking it still opens the editor. An expense
      actual is red only when it is over the plan; nothing turns green. The
      current month's column has a light tint.
```

and change the savings item "A past month where a savings account saved less than planned: the cell does NOT take the green under-plan style an expense row gets." to "A savings account that saved more or less than planned: its actual stays plain (never red or green)."

- [ ] **Step 8: Commit**

```bash
git add web/src/features/budgets/planMath.ts web/src/features/budgets/planMath.test.ts web/src/features/budgets/LimitEditor.tsx web/src/features/budgets/PlanSheet.tsx web/src/features/budgets/PlanSheet.test.tsx web/src/features/budgets/PlanSheet.savings.test.tsx docs/regression-test-plan.md
git commit -m "feat: Plan grid shows actuals only where they exist, blanks unplanned months, tints the current month

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

(Drop `PlanSheet.savings.test.tsx` from `git add` if it did not change.)

---

### Task 5: Plan grid — Show transactions, Uncategorized included

On desktop the Plan grid has no way to a cell's transactions since the right-click menu went, and the Uncategorized row never had one (it has no item sheet). The actual becomes a link, as Spent is in the Budget view; on a tablet a tap on an Uncategorized cell opens its transactions.

**Files:**
- Modify: `web/src/features/budgets/PlanSheet.tsx` (`GridCtx`, `ElementRow` cell, ctx construction)
- Test: `web/src/features/budgets/PlanSheet.test.tsx`
- Docs: `docs/regression-test-plan.md`

**Interfaces:**
- Consumes: the component's existing `openTransactions(el: PlanElementDto, month: string)` callback.
- Produces: `GridCtx.openTransactions: (el: PlanElementDto, month: string) => void`.

- [ ] **Step 1: Write the failing tests**

Model the transactions request handler and dialog assertion on the existing transfers-link test (it opens the same `BudgetTransactionsDialog`); search for `plan-totals-transfers-link-0` and the dialog assertion that follows its click.

```tsx
it('desktop: an actual opens that month's transactions, the Uncategorized row included', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-05-01' })
  renderPage()
  await screen.findByText(/may/i)
  const user = userEvent.setup()
  // the expense Uncategorized row: May actual 10
  const uncatRow = document.querySelector('[data-row-id="uncategorized:1"]') as HTMLElement
  await user.click(within(uncatRow).getAllByRole('button', { name: /^transactions /i })[0])
  const dialog = await screen.findByRole('dialog')
  expect(dialog).toHaveTextContent('Uncategorized')
})

it('desktop: a zero actual, a future month and a savings row are not links', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-05-01' })
  renderPage()
  await screen.findByText(/may/i)
  // expense Uncategorized June actual is 0
  expect(within(screen.getAllByTestId('plan-cell-uncategorized:1')[0]).queryByRole('button', { name: /^transactions /i })).not.toBeInTheDocument()
  // Food May 120: a link
  expect(within(screen.getAllByTestId('plan-cell-cat-food:0')[0]).getByRole('button', { name: /^transactions Food/ })).toBeInTheDocument()
})

it('tablet: a tap on an Uncategorized cell opens its transactions', async () => {
  mockCompactViewport()
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-05-01' })
  renderPage()
  await screen.findByText(/may/i)
  const user = userEvent.setup()
  const uncatRow = document.querySelector('[data-row-id="uncategorized:1"]') as HTMLElement
  await user.click(within(uncatRow).getAllByRole('gridcell')[1]) // May (index 0 is the name)
  expect(await screen.findByRole('dialog')).toHaveTextContent('Uncategorized')
})
```

The `uncategorized:1` row key is `${id}:${type}` (expense uncategorized is type 1, income type 3). If the window shows fewer columns on a tablet, pick the first month cell. If the row-scoped queries match both the plan row and a drag overlay, narrow them the way the file's other tests do (`getAllBy…[0]`). If the fixture has a savings row with a non-zero actual, add an assertion that its cell has no transactions link; otherwise leave savings to `PlanSheet.savings.test.tsx` with one case of the same shape.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && pnpm vitest run src/features/budgets/PlanSheet.test.tsx --maxWorkers=2`
Expected: FAIL — no transactions button in plan cells; a tablet tap on Uncategorized does nothing.

- [ ] **Step 3: Implement**

1. `GridCtx`: add `/** the month's transaction list for a row (desktop: the actual is a link; touch: the Uncategorized cell) */ openTransactions: (el: PlanElementDto, month: string) => void`, and pass the existing `openTransactions` where the ctx object is built (add it to that memo's dependency list if there is one).
2. In `ElementRow`'s cell, replace the actual span with:
   ```tsx
   {future ? null : ctx.isCompact || ctx.editMode || !cell || isZero(cell.actual) || el.type === BudgetElementType.SAVINGS ? (
     <span data-testid="cell-actual" className={`text-xs ${overspend ? 'text-destructive' : 'text-muted-foreground'}`}>
       {actualText}
     </span>
   ) : (
     <button
       type="button"
       data-testid="cell-actual"
       title={t('budgets.page.budget.structure.element.action.show_transactions')}
       aria-label={`transactions ${displayName} ${ctx.monthLabel(m)}`}
       className={`text-xs underline-offset-2 hover:underline ${overspend ? 'text-destructive' : 'text-muted-foreground hover:text-foreground'}`}
       onClick={(e) => {
         e.stopPropagation()
         ctx.openTransactions(el, m)
       }}
     >
       {actualText}
     </button>
   )}
   ```
   Savings rows stay plain: their transfers are not listed by the transactions dialog (the item sheet offers no Show transactions for them either).
3. The cell's `onClick`: after the existing compact branch, open an Uncategorized cell's list:
   ```tsx
   if (ctx.isCompact && !ctx.editMode && idx >= 0) {
     if (!isUncategorized) {
       ctx.openSheet(target)
     } else if (cell && !isZero(cell.actual)) {
       ctx.openTransactions(el, m)
     }
   }
   ```
   Update the comment above it: `// touch: the cell opens the item sheet — or, for Uncategorized (no sheet), its transactions; the marker stops its own click`.

- [ ] **Step 4: Run the tests**

Run: `cd web && pnpm vitest run src/features/budgets --maxWorkers=2`
Expected: PASS. Existing tests that read `cell-actual` still find it (the button keeps the test id); fix any that relied on it being a `span`.

- [ ] **Step 5: Update the regression plan**

In section 9, add:

```markdown
- [ ] Plan grid (desktop): clicking a month's actual opens that month's
      transactions for the row, the Uncategorized rows included; a zero
      actual, a future month and a savings row are not links. 📱 On a tablet a
      tap on an Uncategorized cell with spending opens its transactions (other
      cells open the item sheet).
```

- [ ] **Step 6: Commit**

```bash
git add web/src/features/budgets/PlanSheet.tsx web/src/features/budgets/PlanSheet.test.tsx docs/regression-test-plan.md
git commit -m "feat: Plan grid actuals open their transactions, Uncategorized included

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: Spec notes and full verification

**Files:**
- Modify: `docs/superpowers/specs/2026-09-29-budget-ux-redesign-design.md`

- [ ] **Step 1: Record the stage 3 decisions in the spec**

In "## Part 3 — Calmer desktop and tablet":
- Under "### Budget view", change "thin progress bar under the item name" to "thin progress bar under the row (it spans the row, as on the phone: the Budget cell widens with a carry-over, so a name-width bar would differ per row)".
- Under "### Plan view", add a bullet: "Show transactions: on desktop a non-zero actual in a past or current month is a link to that month's transactions (not for savings rows); on a tablet a tap on an Uncategorized cell opens its transactions, since that row has no item sheet. *(stage 3)*"
- Under "### Plan view", the "Transfers total row and the income Uncategorized row appear only when non-zero" bullet: append "in the visible months (Transfers: when money crossed the boundary in either direction, even if it nets to zero)."

- [ ] **Step 2: Full verification**

Run each from the repo root unless noted; every one must pass (apart from the known `transaction.test.ts` failure):

```bash
cd web && pnpm vitest run --maxWorkers=2
cd web && pnpm lint
cd web && pnpm build
GOTOOLCHAIN=go1.27.1 /usr/local/go/bin/go test ./internal/test/i18ntest/
grep -rn "AvailablePill\|isUnderspent\|PLAN_CURRENCY_COL_PX\|structure.no_folder" web/src || echo clean
```

Expected: tests pass, lint clean, build succeeds, i18ntest `ok`, the grep prints `clean`.

- [ ] **Step 3: Commit**

```bash
git add docs/superpowers/specs/2026-09-29-budget-ux-redesign-design.md
git commit -m "docs: Record redesign stage 3 decisions in the spec

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

## Spec coverage

- Budget view: Available emphasised, no green pills, red pill only when negative — Task 2. Spent coloured by the row rule, bar — Task 2. Currency rule (symbol column, code once, foreign tag) — Task 1. Folder band, spacing — Task 2. "No folder" — Task 1. Title — Task 2.
- Plan view: actuals only for past/current — Task 4. Blank unplanned months — Task 4. Red only over plan, no green — Task 4. Current-month tint — Task 4. Currency rule — Task 3. Transfers only when non-zero — Task 3; income Uncategorized already only when non-zero (`visibleUncat`).
- Row state rule's future month (`—`, no bar) on desktop — Task 2.
- Deferred from stage 1: Uncategorized "Show transactions" — Task 5.

Deliberately not in this plan: folder headers showing "leftover + budget" (offered, not asked); the desktop Plan view's far-future Balance (pre-existing window-start issue); read-only access gating; budget configuration.
