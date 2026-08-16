# Plan view `/plan` route — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the budget plan view its own `/plan` route and nav entry, removing the in-page Budget | Plan toggle.

**Architecture:** One `BudgetPage` component takes a `mode: 'budget' | 'plan'` prop and is mounted (keyed) at both `/budget` and `/plan`. The `budgetMode` store field is deleted; the header tablist and the compact settings-menu radio group stay and `navigate()` between the two routes. Spec: `docs/superpowers/specs/2026-08-16-plan-route-design.md`.

> **Revision (same day, after Task 2 landed):** the user decided plan is part of the budget, so the sidebar Plan entry (Task 2) was reverted and the in-page toggle restored as route navigation. Task 2 below is kept for the record but is NOT in the final state; Task 1's "toggle removed" steps were superseded by the restore commit.

**Tech Stack:** React 19, react-router (data router, `createMemoryRouter` in tests), zustand, vitest + testing-library + msw, react-i18next, lucide-react.

## Global Constraints

- All commands run from `web/` unless stated. Tests: `pnpm vitest run <file>`; type check: `pnpm exec tsc -b` (tests are type-checked, `noUnusedLocals` is on); lint: `pnpm lint`.
- i18n catalogues are `locales/{en,ru}.json` at the repo root; keys must exist in BOTH languages (guard `go test ./internal/test/i18ntest/` from the repo root).
- No new `METRICS` key. `BUDGET_PLAN_OPEN` must still be fired (metrics-coverage test).
- Commit after each task; branch `feature/budget-plan-view`.

---

### Task 1: `/plan` route + `mode` prop on `BudgetPage`, toggle removed

**Files:**
- Modify: `web/src/app/router-pages.ts`
- Modify: `web/src/app/routes.tsx:50`
- Modify: `web/src/features/budgets/BudgetPage.tsx` (lines ~121–126, 192, 207–208, 226–230, 567–583, 626–636, 644, 660)
- Modify: `web/src/features/budgets/budgetStore.ts:29,65–71`
- Modify: `locales/en.json`, `locales/ru.json` (remove `budgets.page.plan.toggle`)
- Test: `web/src/features/budgets/BudgetPage.test.tsx`

**Interfaces:**
- Produces: `RouterPage.PLAN = '/plan'`; `BudgetPage({ mode }: { mode: 'budget' | 'plan' })`; `BudgetMode` type exported from `BudgetPage.tsx`.

- [ ] **Step 1: Rewrite the test harness and mode tests**

In `BudgetPage.test.tsx`, replace `renderPage`:

```tsx
function renderPage(initialPath: '/budget' | '/plan' = '/budget') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/budget', element: <BudgetPage key="budget" mode="budget" /> },
      { path: '/plan', element: <BudgetPage key="plan" mode="plan" /> },
      { path: '/', element: <BudgetPage key="budget" mode="budget" /> },
      { path: '/settings/budgets', element: <div>BUDGETS LIST</div> },
    ],
    { initialEntries: [initialPath] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return { queryClient, router }
}
```

Every existing call site `renderPage()` still works; the one call `renderPage(<HomePage />)`-style at ~line 241 builds its own router — leave it. Fix any `const queryClient = renderPage()` to `const { queryClient } = renderPage()`.

`beforeEach`: drop `budgetMode: 'budget'` from the `setState` call.

Line 57: `expect(screen.getAllByRole('tab')).toHaveLength(47)` and update the comment to `// 47 period-strip month tabs`.

Replace the four mode tests ("offers hide-empty…", "shows the currency pills in both views…", "keeps edit structure…", "compact viewport: the mode toggle…") with:

```tsx
it('offers hide-empty in the settings menu only on /plan', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
  )
  const user = userEvent.setup()
  const { router } = renderPage()
  await screen.findByRole('tablist', { name: 'period' })

  await user.click(screen.getByRole('button', { name: 'Configure' }))
  expect(screen.queryByRole('menuitemcheckbox', { name: 'Hide empty rows' })).not.toBeInTheDocument()
  await user.keyboard('{Escape}')

  await act(() => router.navigate('/plan'))
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  const item = await screen.findByRole('menuitemcheckbox', { name: 'Hide empty rows' })
  expect(useBudgetPeriodStore.getState().planHideEmpty).toBe(false)
  await user.click(item)
  expect(useBudgetPeriodStore.getState().planHideEmpty).toBe(true)
})

it('shows the currency pills on both routes; on /plan a pill toggles the period widget above the sheet', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
  )
  const user = userEvent.setup()
  const { router } = renderPage()
  await screen.findByRole('tablist', { name: 'period' })
  expect(screen.getAllByRole('button', { name: /^currency /i }).length).toBeGreaterThan(0)

  await act(() => router.navigate('/plan'))
  await screen.findByTestId('plan-sheet')
  expect(screen.getAllByRole('button', { name: /^currency /i }).length).toBeGreaterThan(0)
  expect(screen.queryByTestId('expense-widget')).not.toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'currency USD' }))
  const widget = await screen.findByTestId('expense-widget')
  // the page's selected period (the store's 2026-07-01), not a plan column
  expect(widget).toHaveTextContent('Jul 2026')
  expect(screen.getByTestId('plan-sheet')).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'currency USD' }))
  expect(screen.queryByTestId('expense-widget')).not.toBeInTheDocument()
})

it('no header mode toggle on either route, on desktop or compact', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
  )
  const user = userEvent.setup()
  const { router } = renderPage()
  await screen.findByRole('tablist', { name: 'period' })
  expect(screen.queryByRole('tablist', { name: 'budget mode' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  expect(screen.queryByRole('menuitemradio')).not.toBeInTheDocument()
  await user.keyboard('{Escape}')

  await act(() => router.navigate('/plan'))
  await screen.findByTestId('plan-sheet')
  expect(screen.queryByRole('tab')).not.toBeInTheDocument()
})

it('the route hop remounts the page: edit structure started on /plan is off again on /budget', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
  )
  const user = userEvent.setup()
  const { router } = renderPage('/plan')
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))
  expect(screen.getByRole('button', { name: 'Done editing' })).toBeInTheDocument()

  await act(() => router.navigate('/budget'))
  await screen.findByRole('tablist', { name: 'period' })
  expect(screen.queryByRole('button', { name: 'Done editing' })).not.toBeInTheDocument()
})

it('rendering /plan fires BUDGET_PLAN_OPEN once; /budget does not', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
  )
  const { router } = renderPage()
  await screen.findByRole('tablist', { name: 'period' })
  expect(trackEvent).not.toHaveBeenCalledWith(METRICS.BUDGET_PLAN_OPEN)

  await act(() => router.navigate('/plan'))
  await screen.findByTestId('plan-sheet')
  expect(vi.mocked(trackEvent).mock.calls.filter(([k]) => k === METRICS.BUDGET_PLAN_OPEN)).toHaveLength(1)
})
```

Add the imports the file lacks: `act` from `@testing-library/react`, `trackEvent` from `@/lib/metrics` (already `vi.mock`ed in this file? — if not, add `vi.mock('@/lib/metrics', async (orig) => ({ ...(await orig<typeof import('@/lib/metrics')>()), trackEvent: vi.fn() }))` next to the other mocks and `vi.clearAllMocks()` in `beforeEach`), `METRICS` from `@/lib/metrics`.

- [ ] **Step 2: Run the file — expect failures**

Run: `pnpm vitest run src/features/budgets/BudgetPage.test.tsx`
Expected: type/compile errors on `mode` prop and the removed tab; the new tests fail.

- [ ] **Step 3: Implement**

`web/src/app/router-pages.ts` — after `BUDGET: '/budget',` add `PLAN: '/plan',`.

`web/src/app/routes.tsx` — replace line 50 with:

```tsx
                { path: '/budget', element: <BudgetPage key="budget" mode="budget" /> },
                { path: '/plan', element: <BudgetPage key="plan" mode="plan" /> },
```

`web/src/features/budgets/BudgetPage.tsx`:

- Replace lines 121–126 with `export type BudgetMode = 'budget' | 'plan'` (delete `BUDGET_MODES` and `BUDGET_MODE_LABEL`).
- Signature: `export function BudgetPage({ mode }: { mode: BudgetMode }) {`.
- Delete the two store reads `budgetMode` / `setBudgetMode` (lines 207–208) and the `switchBudgetMode` helper + its comment (226–230); keep `const [editMode, setEditMode] = useState(false)`.
- After the `editMode` state add:

```tsx
  useEffect(() => {
    if (mode === 'plan') {
      trackEvent(METRICS.BUDGET_PLAN_OPEN)
    }
  }, [mode])
```

  and import `trackEvent`, `METRICS` from `@/lib/metrics` (check whether the file already imports them).
- Header: delete the `{isCompact ? null : (<div role="tablist" …>…)}` block (567–583) including its comment.
- Settings menu: delete the `{isCompact ? (<> <DropdownMenuRadioGroup …/> <DropdownMenuSeparator/> </>) : null}` block (626–636). Remove now-unused imports `DropdownMenuRadioGroup`, `DropdownMenuRadioItem` (keep `DropdownMenuSeparator` — still used by the plan block).
- Replace `budgetMode === 'plan'` with `mode === 'plan'` (lines 644, 660).

`web/src/features/budgets/budgetStore.ts` — delete `budgetMode: 'budget' | 'plan'` and `setBudgetMode` from the state type and their implementations (lines 65–71); drop the `trackEvent`/`METRICS` import ONLY if nothing else in the file uses them (`setPlanFirstMonth` does — keep).

`locales/en.json` and `locales/ru.json` — delete the `"toggle": { "budget": …, "plan": … },` object under `budgets.page.plan`.

- [ ] **Step 4: Run tests + type check**

Run: `pnpm vitest run src/features/budgets/BudgetPage.test.tsx && pnpm exec tsc -b`
Expected: BudgetPage tests PASS. `tsc` will now fail in `PlanSheet.test.tsx` (`<BudgetPage />` without `mode`, `budgetMode` in setState) — that is Task 3; also `budgetStore.test`? (`grep -rn budgetMode src` to find any other consumer and fix it here.)

- [ ] **Step 5: Commit**

```bash
git add web/src/app/router-pages.ts web/src/app/routes.tsx web/src/features/budgets/BudgetPage.tsx web/src/features/budgets/BudgetPage.test.tsx web/src/features/budgets/budgetStore.ts locales/en.json locales/ru.json
git commit -m "feat(web): mount the plan view at /plan and drop the in-page mode toggle"
```

---

### Task 2: Plan link in the sidebar (list, compact, icon rail)

**Files:**
- Modify: `web/src/app/layouts/ApplicationLayout.tsx:4,178–184,204–206`
- Modify: `locales/en.json`, `locales/ru.json` (`common.nav.plan`)
- Test: `web/src/app/layouts/ApplicationLayout.test.tsx`

**Interfaces:**
- Consumes: `RouterPage.PLAN` (Task 1).

- [ ] **Step 1: Tests**

In `ApplicationLayout.test.tsx`, in the test 'shows the loading gate, then the sidebar tree with folder totals' after `expect(screen.getByText('Budget')).toBeInTheDocument()` add:

```tsx
  expect(screen.getByRole('link', { name: 'Plan' })).toHaveAttribute('href', '/plan')
```

Add a new test after 'desktop divider click collapses the sidebar to an icon rail and back':

```tsx
it('the icon rail carries Budget and Plan icon links', async () => {
  mockViewport(false)
  server.use(...coreHandlers())
  const user = userEvent.setup()
  renderShell('/account/a1')
  expect(await screen.findByText('Cash')).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'toggle sidebar' }))
  expect(screen.queryByText('Plan')).not.toBeInTheDocument()
  expect(screen.getByTitle('Budget')).toHaveAttribute('href', '/budget')
  expect(screen.getByTitle('Plan')).toHaveAttribute('href', '/plan')
})
```

(Match the exact `server.use(...)`/`renderShell` idiom of the neighbouring rail test if `coreHandlers()` needs arguments there.)

- [ ] **Step 2: Run — expect FAIL**

Run: `pnpm vitest run src/app/layouts/ApplicationLayout.test.tsx`
Expected: FAIL — no link named "Plan".

- [ ] **Step 3: Implement**

`locales/en.json` `common.nav`: add `"plan": "Plan"` after `"budget": "Budget"`. `locales/ru.json`: `"plan": "План"`.

`ApplicationLayout.tsx`:
- Import: `import { CalendarRange, RefreshCw, Rocket, Settings, UserPlus, Wallet } from 'lucide-react'`.
- Rail (after the Wallet `<Link>` closing tag, ~line 184):

```tsx
                    <Link
                      to={RouterPage.PLAN}
                      title={t('common.nav.plan')}
                      className="grid size-10 place-items-center rounded-lg text-muted-foreground hover:bg-accent"
                    >
                      <CalendarRange className="size-5" />
                    </Link>
```

- List (after the Budget `<Link>` at ~line 204–206):

```tsx
                    <Link to={RouterPage.PLAN} className={`rounded-md px-2 py-2 hover:bg-accent ${isCompact ? 'text-lg' : 'text-[15px]'}`}>
                      {t('common.nav.plan')}
                    </Link>
```

- [ ] **Step 4: Run — expect PASS**

Run: `pnpm vitest run src/app/layouts/ApplicationLayout.test.tsx`

- [ ] **Step 5: Commit**

```bash
git add web/src/app/layouts/ApplicationLayout.tsx web/src/app/layouts/ApplicationLayout.test.tsx locales/en.json locales/ru.json
git commit -m "feat(web): Plan entry in the sidebar next to Budget"
```

---

### Task 3: `PlanSheet.test.tsx` renders `/plan` directly; store tests removed

**Files:**
- Modify: `web/src/features/budgets/PlanSheet.test.tsx`

- [ ] **Step 1: Rewrite the harness**

```tsx
function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/plan', element: <BudgetPage key="plan" mode="plan" /> },
      { path: '/budget', element: <BudgetPage key="budget" mode="budget" /> },
    ],
    { initialEntries: ['/plan'] },
  )
  const result = render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return { queryClient, router, ...result }
}
```

- [ ] **Step 2: Mechanical edits**

- Delete every line `await user.click(await screen.findByRole('tab', { name: /plan/i }))` (66 sites; `sed -i '' "/findByRole('tab', { name: \/plan\/i })/d"` from `web/`).
- First test (line ~110): delete `await screen.findByRole('tablist', { name: 'period' })` and the following `await user.click(screen.getByRole('tab', { name: /plan/i }))`; rename the test to `'/plan renders the sheet: months, income on top, cells'`.
- Test 'toggle switches to plan mode…' persistence variant (line ~150): delete `expect(useBudgetPeriodStore.getState().budgetMode).toBe('plan')`; rename to `'the plan window position survives a remount'`.
- Delete the whole test `'setBudgetMode fires BUDGET_PLAN_OPEN once switching to plan, not on a no-op switch'`.
- `beforeEach` setState: drop `budgetMode: 'budget'`. Compact test (~line 1258): `useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })` and delete the comment above it.
- Any test where removing the click leaves `const user = userEvent.setup()` unused: delete that line (tsc `noUnusedLocals`).
- Where a test's first await after `renderPage()` was the deleted click, make sure the next statement is an `await screen.findBy…` (it is in every sampled site; verify by running).

- [ ] **Step 3: Run**

Run: `pnpm vitest run src/features/budgets/PlanSheet.test.tsx && pnpm exec tsc -b && pnpm lint`
Expected: PASS, no type errors, lint clean.

- [ ] **Step 4: Full frontend + i18n guard**

Run: `pnpm test` (in `web/`) and `go test ./internal/test/i18ntest/` (repo root).
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/budgets/PlanSheet.test.tsx
git commit -m "test(web): plan sheet tests render /plan directly"
```
