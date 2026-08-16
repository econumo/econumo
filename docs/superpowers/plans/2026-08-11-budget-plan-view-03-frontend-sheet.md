# Budget Plan View — 03 Frontend Plan Sheet Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The Plan mode of the budget page: a Budget | Plan toggle, a responsive multi-month spreadsheet of every income and expense row with editable planned cells, a totals block with a running Balance projection, row-density controls, keyboard grid navigation, and a usable single-month mobile degradation — all over the `get-budget-plan` read shipped in plan 02.

**Architecture:** All new code lives in `web/src/features/budgets/`. A pure-function core (`planMath.ts`: window math, row bucketing with the income-on-top invariant, totals + Balance math) is unit-tested without DOM; the sheet component (`PlanSheet.tsx`) is a thin renderer over it. Plan state (mode, first visible month, folds, hide-empty) extends the existing persisted `budgetStore`. `useSetLimit` gains an explicit `period`; the plan's own commit path (`usePlanSetLimit`) optimistically patches the plan cache and invalidates that month's budget query. The sheet reuses `LimitEditor`/`SetLimitDialog` after decoupling them from `BudgetElementDto`.

**Tech Stack:** React 19 + Vite, TypeScript, TanStack Query, Zustand (persist), Tailwind 4 / shadcn components, react-i18next, vitest + Testing Library + msw. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-08-02-budget-plan-view-design.md` — the entire "Frontend" section plus "Analytics", "i18n", and the frontend bullet of "Testing". Backend (plans 01–02) is complete on this branch: `GET /api/v1/budget/get-budget-plan?id&from&months(1..24, default 12)` returns `{meta, months[], openingBalances[], currencyRates[{period, rates[]}], structure:{folders, elements}}` with element types 0=envelope, 1=category, 2=tag, 3=income category, 4=income envelope; cells `[{actual, planned}]` aligned with `months` (planned `""` = no limit); children carry `[{actual}]` cells; two synthetic Uncategorized rows share id `"uncategorized"` (types 1 and 3); `openingBalances` is per-currency balances STRICTLY BEFORE `months[0]`; `create-envelope` takes an optional `side: "income"`.

## Global Constraints

- **Every new user-facing action fires a `METRICS` event** (frozen `app`-prefixed camelCase; PostHog name derives). This plan adds exactly three keys: `BUDGET_PLAN_OPEN: 'appBudgetPlanOpen'`, `BUDGET_PLAN_CHANGE_WINDOW: 'appBudgetPlanChangeWindow'`, `BUDGET_PLAN_HIDE_EMPTY_TOGGLE: 'appBudgetPlanHideEmptyToggle'`. `web/src/lib/metrics-coverage.test.ts` fails if a key is never fired.
- **Every new i18n key lands in ALL ELEVEN catalogues** — `locales/{de,en,es,fr,it,nl,pl,pt,ru,uk,zh}.json` — with identical key sets and `{var}` placeholder sets (`internal/test/i18ntest` enforces; it runs inside `make go-test`). The frontend `t()`-key guard scans `web/src` for literal keys.
- **No backend changes.** The wire contract is frozen; this plan only consumes it. If a needed field is missing, STOP and report — do not change Go code.
- **Hard layout invariant:** income rows on top, expense rows under them, never interleaved (spec). Neutral (empty) folders render in the expense area.
- Amount strings are wire-format decimals; all arithmetic via `@/lib/decimal` (`add/sub/cmp/isZero/abs`) — never `parseFloat`.
- Existing budget-page behavior is unchanged: same wire calls, same rendering in Budget mode. `useSetLimit`'s refactor must be call-site-compatible (all existing callers updated in the same task).
- Frontend gates per task: `pnpm exec tsc -b` clean, `pnpm test` green, `pnpm lint` clean (run from `web/`). Final task also runs `make go-test` (i18n guards) and the full `make test`.
- Node is on the host (`/usr/bin/node`, v22); **pnpm is NOT installed** — Task 1 installs it. All `pnpm` commands run from `web/`.
- Branch: `feature/budget-plan-view`. Commit per task; push after each task (the session rule).
- Dates are `YYYY-MM-01` strings throughout (the store's `normalizePeriod` convention); month arithmetic via the `addMonths` helper defined in Task 3 — never `Date` mutation scattered inline.

---

### Task 1: Frontend tooling + explicit-period `useSetLimit`

Installs pnpm (one-time host setup), then makes the one shared mutation the plan sheet needs period-explicit, updating every existing call site.

**Files:**
- Modify: `web/src/features/budgets/queries.ts` (`useSetLimit`, ~line 114)
- Modify: `web/src/features/budgets/BudgetPage.tsx` (the two `setLimit.mutate` call sites, ~lines 648 and 788)
- Test: `web/src/features/budgets/queries.test.tsx` (extend)

**Interfaces:**
- Consumes: nothing new.
- Produces: `useSetLimit()` whose `mutate` form is `{ budgetId: Id; elementId: Id; period: string; amount: string | null }` — `period` REQUIRED, `Y-m-01`. The optimistic patch targets `[...queryKeys.budget, budgetId, period]`. Later tasks pass the cell's month here; the budget page passes `selectedDate`.

- [ ] **Step 1: Install pnpm and prove the suite runs**

```bash
cd /path/to/worktree/web
npm install --prefix "$HOME/.local" -g pnpm
export PATH="$HOME/.local/bin:$PATH"
pnpm --version
pnpm install
pnpm test -- --run 2>&1 | tail -5
pnpm exec tsc -b && pnpm lint
```

Expected: install succeeds, the existing vitest suite is green, tsc and oxlint clean. If `pnpm install` needs a lockfile-pinned version, use `corepack enable --install-directory $HOME/.local/bin && corepack prepare --activate` instead. Record the working invocation in your report — later tasks repeat it.

- [ ] **Step 2: Write the failing test**

Append to `web/src/features/budgets/queries.test.tsx` (mirror the file's existing renderHook/msw scaffolding):

```tsx
it('useSetLimit sends the explicit period and patches that period cache', async () => {
  let sent: Record<string, unknown> | null = null
  server.use(
    http.post('*/api/v1/budget/set-limit', async ({ request }) => {
      sent = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const { result, queryClient } = renderSetLimitHarness() // the file's existing harness pattern
  queryClient.setQueryData([...queryKeys.budget, 'b1', '2026-09-01'], fixtureWireBudget)
  result.current.mutate({ budgetId: 'b1', elementId: 'el1', period: '2026-09-01', amount: '250' })
  await waitFor(() => expect(sent).not.toBeNull())
  expect(sent).toMatchObject({ budgetId: 'b1', elementId: 'el1', period: '2026-09-01', amount: '250' })
  const patched = queryClient.getQueryData<BudgetDto>([...queryKeys.budget, 'b1', '2026-09-01'])
  expect(patched?.structure.elements.find((e) => e.id === 'el1')?.budgeted).toBe('250')
})
```

Adapt the harness/fixture names to what `queries.test.tsx` actually exports — the substance is: the wire body carries the passed `period` (not the store's `selectedDate`), and the optimistic patch lands on the passed-period cache key.

- [ ] **Step 3: Run test to verify it fails**

Run: `pnpm test -- --run queries`
Expected: FAIL — TypeScript: `period` not in the mutate form (or wire body carries the store date).

- [ ] **Step 4: Implement**

`web/src/features/budgets/queries.ts` — `useSetLimit` becomes period-explicit (the store subscription goes away):

```ts
export function useSetLimit() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (form: { budgetId: Id; elementId: Id; period: string; amount: string | null }) =>
      budgetApi.setLimit(form),
    onMutate: async (form) => {
      // optimistic budgeted patch with rollback (Vue parity: instant cell feedback)
      const key = [...queryKeys.budget, form.budgetId, form.period]
      await queryClient.cancelQueries({ queryKey: key })
      const previous = queryClient.getQueryData<BudgetDto | null>(key)
      queryClient.setQueryData<BudgetDto | null>(key, (prev) => {
        if (!prev) {
          return prev
        }
        return {
          ...prev,
          structure: {
            ...prev.structure,
            elements: prev.structure.elements.map((el) =>
              el.id === form.elementId ? { ...el, budgeted: form.amount === null ? '0' : form.amount } : el,
            ),
          },
        }
      })
      return { previous, key }
    },
    onError: (_err, _form, context) => {
      if (context) {
        queryClient.setQueryData(context.key, context.previous)
      }
    },
    onSuccess: () => trackEvent(METRICS.BUDGET_UPDATE_ELEMENT_LIMIT),
  })
}
```

`web/src/features/budgets/BudgetPage.tsx` — both call sites gain `period: selectedDate`:

```tsx
onCommit={(amount) => setLimit.mutate({ budgetId: budget.meta.id, elementId: element.id, period: selectedDate, amount })}
```

```tsx
onCommit={(elementId, amount) => setLimit.mutate({ budgetId: budget.meta.id, elementId, period: selectedDate, amount })}
```

(`budgetApi.setLimit` already takes `period` in its form — no API-client change.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `pnpm test -- --run && pnpm exec tsc -b && pnpm lint`
Expected: PASS — the whole suite, incl. the untouched BudgetPage tests (they exercise limit commits through the page).

- [ ] **Step 6: Commit + push**

```bash
git add web/src/features/budgets/queries.ts web/src/features/budgets/BudgetPage.tsx web/src/features/budgets/queries.test.tsx
git commit -m "refactor(web): useSetLimit takes an explicit period"
git push
```

---

### Task 2: Plan DTOs, API client, `useBudgetPlan` query

**Files:**
- Modify: `web/src/api/dto/budget.ts` (income element types + plan DTOs)
- Modify: `web/src/api/budget.ts` (`getBudgetPlan`)
- Modify: `web/src/app/queryKeys.ts` (add `budgetPlan`)
- Modify: `web/src/features/budgets/queries.ts` (`useBudgetPlan`, `usePlanSetLimit`)
- Modify: `web/src/test/fixtures.ts` (a reusable `fixtureWirePlan` + msw handler)
- Test: `web/src/features/budgets/queries.test.tsx` (extend)

**Interfaces:**
- Consumes: Task 1's `useSetLimit` shape (for symmetry only).
- Produces (every later task builds on these exact names):

```ts
// dto/budget.ts
export const BudgetElementType = { ENVELOPE: 0, CATEGORY: 1, TAG: 2, INCOME_CATEGORY: 3, INCOME_ENVELOPE: 4 } as const
export const isIncomeType = (t: BudgetElementType): boolean =>
  t === BudgetElementType.INCOME_CATEGORY || t === BudgetElementType.INCOME_ENVELOPE

export interface PlanCellDto { actual: string; planned: string } // planned '' = no limit
export interface PlanChildDto {
  id: Id; type: BudgetElementType; name: string; icon: string; isArchived: 0 | 1
  ownerUserId: Id; cells: { actual: string }[]
}
export interface PlanElementDto {
  id: Id; type: BudgetElementType; name: string; icon: string
  currencyId: Id; isArchived: 0 | 1; folderId: Id | null; position: number
  ownerUserId: Id | null; cells: PlanCellDto[]; children: PlanChildDto[]
}
export interface PlanMonthRatesDto { period: string; rates: BudgetRateDto[] }
export interface BudgetPlanDto {
  meta: BudgetMetaDto
  months: string[]
  openingBalances: { currencyId: Id; amount: string }[]
  currencyRates: PlanMonthRatesDto[]
  structure: { folders: BudgetFolderDto[]; elements: PlanElementDto[] }
}

// api/budget.ts
export async function getBudgetPlan(id: Id, from: string, months: number): Promise<BudgetPlanDto>

// queries.ts
export function useBudgetPlan(budgetId: Id | null, firstMonth: string, visibleMonths: number): UseQueryResult<BudgetPlanDto | null> & { fetchFrom: string }
// fetches [firstMonth - PLAN_BUFFER, visibleMonths + 2*PLAN_BUFFER] with keepPreviousData
export const PLAN_BUFFER = 2
export function usePlanSetLimit(planKey: readonly unknown[])
// mutate form: { budgetId; elementId; period; amount; monthIndex: number }
// optimistic: patch the plan cache cell's planned; onError rollback;
// onSuccess: BUDGET_UPDATE_ELEMENT_LIMIT + invalidate [...queryKeys.budget, budgetId, period] + invalidate the plan key
```

Note: existing `BudgetElementType` usages compile unchanged (the const object only gains members). `PlanChildDto.cells` deliberately reuses `{ actual: string }` inline — the wire has no `planned` on children.

- [ ] **Step 1: Write the failing tests**

Append to `web/src/features/budgets/queries.test.tsx`:

```tsx
it('useBudgetPlan fetches a buffered window and keeps previous data across shifts', async () => {
  const calls: string[] = []
  server.use(
    http.get('*/api/v1/budget/get-budget-plan', ({ request }) => {
      calls.push(new URL(request.url).search)
      return HttpResponse.json({ success: true, message: '', data: { item: fixtureWirePlan } })
    }),
  )
  // firstMonth 2026-07, 6 visible -> fetch from 2026-05 for 10 months
  const { result } = renderPlanHarness('b1', '2026-07-01', 6)
  await waitFor(() => expect(result.current.data).toBeTruthy())
  expect(calls[0]).toContain('from=2026-05-01')
  expect(calls[0]).toContain('months=10')
})

it('usePlanSetLimit patches the cell and clears on null', async () => {
  server.use(http.post('*/api/v1/budget/set-limit', () => HttpResponse.json({ success: true, message: '', data: {} })))
  const { result, queryClient, planKey } = renderPlanSetLimitHarness(fixtureWirePlan)
  result.current.mutate({ budgetId: 'b1', elementId: 'pe1', period: '2026-07-01', amount: '400', monthIndex: 2 })
  const plan = queryClient.getQueryData<BudgetPlanDto>(planKey)
  expect(plan?.structure.elements.find((e) => e.id === 'pe1')?.cells[2]?.planned).toBe('400')
})
```

Build `fixtureWirePlan` in `web/src/test/fixtures.ts`: 4 months `['2026-05-01','2026-06-01','2026-07-01','2026-08-01']`, one folder, one expense envelope `pe1` (type 0, one child), one standalone expense category, one tag, one income envelope (type 4, one type-3 child), one standalone income category, both uncategorized rows (types 1 and 3), one openingBalances entry, `currencyRates` with one rate per month — cells arrays of length 4 with a mix of actuals and `''`/non-empty planned. Reuse `fixtureUser`/currency ids the file already exports. This fixture is THE shared plan fixture for every later task — make it representative, not minimal.

- [ ] **Step 2: Run tests to verify they fail**

Run: `pnpm test -- --run queries`
Expected: FAIL — `getBudgetPlan`/`useBudgetPlan` undefined.

- [ ] **Step 3: Implement**

`web/src/api/dto/budget.ts`: extend `BudgetElementType`, add `isIncomeType` and the plan DTOs exactly as in Interfaces.

`web/src/api/budget.ts`:

```ts
export async function getBudgetPlan(id: Id, from: string, months: number): Promise<BudgetPlanDto> {
  const response = await api.get<Envelope<{ item: BudgetPlanDto }>>(
    apiUrl(`/api/v1/budget/get-budget-plan?id=${encodeURIComponent(id)}&from=${encodeURIComponent(from)}&months=${months}`),
  )
  return response.data.data.item
}
```

`web/src/app/queryKeys.ts`: add `budgetPlan: ['budgetPlan'] as const` beside `budget`.

`web/src/features/budgets/queries.ts`:

```ts
export const PLAN_BUFFER = 2

export function planFetchWindow(firstMonth: string, visibleMonths: number): { from: string; months: number } {
  // buffer both sides so arrow navigation renders instantly; the server caps months at 24
  const months = Math.min(visibleMonths + 2 * PLAN_BUFFER, 24)
  return { from: addMonths(firstMonth, -PLAN_BUFFER), months }
}

export function useBudgetPlan(budgetId: Id | null, firstMonth: string, visibleMonths: number) {
  const { from, months } = planFetchWindow(firstMonth, visibleMonths)
  const query = useQuery<BudgetPlanDto | null>({
    queryKey: [...queryKeys.budgetPlan, budgetId ?? 'none', from, months],
    queryFn: () => (budgetId ? budgetApi.getBudgetPlan(budgetId, from, months) : Promise.resolve(null)),
    enabled: budgetId !== null,
    staleTime: TEN_MINUTES,
    placeholderData: keepPreviousData,
  })
  return { ...query, fetchFrom: from, planKey: [...queryKeys.budgetPlan, budgetId ?? 'none', from, months] as const }
}

export function usePlanSetLimit(planKey: readonly unknown[]) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (form: { budgetId: Id; elementId: Id; period: string; amount: string | null; monthIndex: number }) =>
      budgetApi.setLimit({ budgetId: form.budgetId, elementId: form.elementId, period: form.period, amount: form.amount }),
    onMutate: async (form) => {
      await queryClient.cancelQueries({ queryKey: planKey })
      const previous = queryClient.getQueryData<BudgetPlanDto | null>(planKey)
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
                ? {
                    ...el,
                    cells: el.cells.map((c, i) =>
                      i === form.monthIndex ? { ...c, planned: form.amount === null ? '' : form.amount } : c,
                    ),
                  }
                : el,
            ),
          },
        }
      })
      return { previous }
    },
    onError: (_err, _form, context) => {
      if (context) {
        queryClient.setQueryData(planKey, context.previous)
      }
    },
    onSuccess: (_res, form) => {
      trackEvent(METRICS.BUDGET_UPDATE_ELEMENT_LIMIT)
      // budget-page cache for that month is now stale; the plan cache resyncs too
      void queryClient.invalidateQueries({ queryKey: [...queryKeys.budget, form.budgetId, form.period] })
      void queryClient.invalidateQueries({ queryKey: planKey })
    },
  })
}
```

`addMonths` does not exist yet — Task 3 owns it in `planMath.ts`; for THIS task declare it there already (Task 3 fleshes the rest of the file):

```ts
// web/src/features/budgets/planMath.ts
export function addMonths(month: string, delta: number): string {
  const [y, m] = month.split('-').map(Number)
  const d = new Date(y, m - 1 + delta, 1)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-01`
}
```

(import it in queries.ts from `./planMath`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm test -- --run && pnpm exec tsc -b && pnpm lint`
Expected: PASS.

- [ ] **Step 5: Commit + push**

```bash
git add web/src/api/dto/budget.ts web/src/api/budget.ts web/src/app/queryKeys.ts web/src/features/budgets/queries.ts web/src/features/budgets/planMath.ts web/src/features/budgets/queries.test.tsx web/src/test/fixtures.ts
git commit -m "feat(web): plan DTOs, get-budget-plan client, buffered useBudgetPlan + usePlanSetLimit"
git push
```

---

### Task 3: `planMath` — window, bucketing, totals, Balance

The pure core. Everything here is testable without DOM and everything the sheet renders comes from here.

**Files:**
- Modify: `web/src/features/budgets/planMath.ts` (created in Task 2)
- Test: `web/src/features/budgets/planMath.test.ts` (create)

**Interfaces:**
- Consumes: `BudgetPlanDto`, `PlanElementDto`, `isIncomeType` (Task 2); `add/sub/cmp/isZero` from `@/lib/decimal`; `exchange` from `@/lib/exchange`; `compareNames` from `@/lib/collate`.
- Produces (exact signatures; Tasks 4–9 import these):

```ts
export function addMonths(month: string, delta: number): string            // Task 2, already present
export function monthDiff(a: string, b: string): number                    // b - a in months
export function currentMonth(now?: Date): string                           // 'YYYY-MM-01'

// responsive column count: 3..12 that fit, or 1 (mobile collapse) when even 3 don't
export const PLAN_NAME_COL_PX = 180
export const PLAN_MIN_MONTH_COL_PX = 110
export function planVisibleCount(containerWidthPx: number): number

// initial first month: persisted wins; else current-1; single-column starts at current; always clamped to budget start
export function planInitialFirstMonth(persisted: string | null, startedAt: string, visible: number, now?: Date): string
export function clampFirstMonth(firstMonth: string, startedAt: string): string

export interface PlanRow { element: PlanElementDto; hidden: boolean }      // hidden = empty in every fetched month
export interface PlanFolderSection { folder: BudgetFolderDto; rows: PlanRow[] }
export interface PlanRows {
  income: { folders: PlanFolderSection[]; loose: PlanRow[]; uncategorized: PlanRow | null; hiddenCount: number }
  expense: { folders: PlanFolderSection[]; loose: PlanRow[]; uncategorized: PlanRow | null; hiddenCount: number }
  archived: PlanRow[]
}
export function bucketPlanRows(plan: BudgetPlanDto, hideEmpty: boolean): PlanRows

export interface PlanMonthTotals {
  incomeActual: string; incomePlanned: string
  expenseActual: string; expensePlanned: string
  netActual: string; netPlanned: string
  /** per-element-cell max(actual, planned): the Balance row's contribution */
  effectiveNet: string
}
export type MonthExchange = (fromCurrencyId: string, amount: string, monthIndex: number) => string
export function makePlanExchange(plan: BudgetPlanDto, currencies: CurrencyDto[]): MonthExchange
export function planTotals(plan: BudgetPlanDto, ex: MonthExchange, now?: Date): PlanMonthTotals[]
export function balanceRow(plan: BudgetPlanDto, totals: PlanMonthTotals[], ex: MonthExchange, now?: Date): string[]
```

**Semantics (spec, restated precisely):**
- `planVisibleCount(w)`: columns that fit = `floor((w - PLAN_NAME_COL_PX) / PLAN_MIN_MONTH_COL_PX)`; result = `1` when fit < 3, else `min(fit, 12)`.
- `planInitialFirstMonth`: persisted (clamped) if given; else `visible === 1 ? currentMonth : addMonths(currentMonth, -1)`, clamped to `startedAt`'s month.
- `bucketPlanRows`: a row's side = `isIncomeType(el.type)`; uncategorized rows are the `id === UNCATEGORIZED_ID` elements split by type (3 → income, 1 → expense); archived rows (isArchived 1, both sides) go to `archived` sorted by localized name — the server already prunes inactive archived rows. Folder side is derived from member rows: any income member → the whole folder section renders in the income area; any expense member → expense area; memberless folder → expense area (neutral). Within a section rows sort by `position`; folders sort by their `position`. `hidden` = every cell has zero/`''` actual AND `''` planned (children ignored); with `hideEmpty` true, hidden rows are EXCLUDED from `rows` and counted in `hiddenCount` per side (uncategorized/archived rows are never hidden — they only exist when they have data).
- `makePlanExchange`: month-indexed FX — `(from, amount, i)` converts into `plan.meta.currencyId` using `plan.currencyRates[i].rates` via the shared `exchange` (map each rate to `{...r, updatedAt: r.periodStart}` exactly as `makeBudgetExchange` does).
- `planTotals`: per month index, sum over ALL non-archived parent rows (both sides — envelopes, standalone categories, tags, and the uncategorized rows; children never counted — the parent's actual already includes them): actual and planned (empty planned counts as `'0'`) converted per month. `effectiveNet(i)`: for a FULLY ELAPSED month (`plan.months[i] < currentMonth(now)`) it equals `netActual`; for the current and future months it is `Σ_incomeRows max(actual, planned') − Σ_expenseRows max(actual, planned')` — the max taken PER ELEMENT CELL, planned' = planned or '0'. Archived rows count toward actuals (money really moved) but never toward planned.
- `balanceRow`: `balance[0] = seed + effectiveNet[0]`, `balance[i] = balance[i-1] + effectiveNet[i]`; seed = Σ over `plan.openingBalances` of `ex(entry.currencyId, entry.amount, 0)`.

- [ ] **Step 1: Write the failing tests**

Create `web/src/features/budgets/planMath.test.ts`. Cover, with hand-computed expected values (use a tiny inline plan fixture where amounts are round numbers and one row is in a second currency with rate 2:1 in month 0 and 4:1 in month 1 — proving per-month FX):

```ts
describe('window math', () => {
  it('addMonths crosses years both ways', () => { ... '2026-01-01' -1 -> '2025-12-01'; '2026-11-01' +2 -> '2027-01-01' })
  it('planVisibleCount: 3..12 fit, collapse below 3, cap at 12', () => {
    expect(planVisibleCount(180 + 110 * 2)).toBe(1)      // only 2 fit -> mobile collapse
    expect(planVisibleCount(180 + 110 * 3)).toBe(3)
    expect(planVisibleCount(180 + 110 * 7 + 50)).toBe(7)
    expect(planVisibleCount(180 + 110 * 40)).toBe(12)
  })
  it('planInitialFirstMonth anchors current month second, clamps at start, single-column starts current', () => { ... })
  it('clampFirstMonth never precedes the budget start month', () => { ... })
})

describe('bucketPlanRows', () => {
  it('income above expenses: income envelope + income category + income uncat in income; folders derive side from members', () => { ... })
  it('neutral folder renders in the expense area', () => { ... })
  it('hideEmpty removes all-empty rows and counts them per side; rows with any planned survive', () => { ... })
  it('archived rows leave the sections and sort by name', () => { ... })
})

describe('totals + balance', () => {
  it('planTotals converts each month with its own rates (2:1 then 4:1)', () => { ... })
  it('effectiveNet: past month = actual net; current/future = per-cell max', () => {
    // expense row: actual 120, planned 100 -> counts 120 (overspend never vanishes)
    // second expense row: actual 10, planned 50 -> counts 50
    // income row: actual 900, planned 1000 -> counts 1000; max is per cell, not on totals
  })
  it('balanceRow chains: seed(+FX) + effectiveNet cumulative', () => { ... })
  it('empty planned counts as zero everywhere; archived rows count in actuals only', () => { ... })
})
```

Write every `...` as a real assertion with concrete numbers — the fixture is yours to design; keep it small enough to verify by hand in the test comments.

- [ ] **Step 2: Run tests to verify they fail**

Run: `pnpm test -- --run planMath`
Expected: FAIL — functions undefined.

- [ ] **Step 3: Implement `planMath.ts`**

Implement exactly the Interfaces block. Reference implementations for the two subtle ones:

```ts
export function planTotals(plan: BudgetPlanDto, ex: MonthExchange, now?: Date): PlanMonthTotals[] {
  const cur = currentMonth(now)
  const rows = plan.structure.elements
  return plan.months.map((month, i) => {
    let incomeActual = '0', incomePlanned = '0', expenseActual = '0', expensePlanned = '0'
    let effIncome = '0', effExpense = '0'
    for (const el of rows) {
      const cell = el.cells[i]
      if (!cell) {
        continue
      }
      const actual = ex(el.currencyId, cell.actual, i)
      const planned = ex(el.currencyId, cell.planned === '' ? '0' : cell.planned, i)
      const isPast = month < cur
      // per-cell effective value: overspend keeps its actual, underspend keeps its plan
      const effective = isPast ? actual : cmp(actual, planned) >= 0 ? actual : planned
      if (isIncomeType(el.type)) {
        incomeActual = add(incomeActual, actual)
        if (el.isArchived === 0) incomePlanned = add(incomePlanned, planned)
        effIncome = add(effIncome, el.isArchived === 0 ? effective : actual)
      } else {
        expenseActual = add(expenseActual, actual)
        if (el.isArchived === 0) expensePlanned = add(expensePlanned, planned)
        effExpense = add(effExpense, el.isArchived === 0 ? effective : actual)
      }
    }
    return {
      incomeActual, incomePlanned, expenseActual, expensePlanned,
      netActual: sub(incomeActual, expenseActual),
      netPlanned: sub(incomePlanned, expensePlanned),
      effectiveNet: sub(effIncome, effExpense),
    }
  })
}

export function balanceRow(plan: BudgetPlanDto, totals: PlanMonthTotals[], ex: MonthExchange): string[] {
  let running = plan.openingBalances.reduce((acc, b) => add(acc, ex(b.currencyId, b.amount, 0)), '0')
  return totals.map((t) => {
    running = add(running, t.effectiveNet)
    return running
  })
}
```

(uncategorized rows have `planned: ''` in every cell, so they flow through the same loop; the type encodes their side.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm test -- --run planMath && pnpm exec tsc -b && pnpm lint`
Expected: PASS.

- [ ] **Step 5: Commit + push**

```bash
git add web/src/features/budgets/planMath.ts web/src/features/budgets/planMath.test.ts
git commit -m "feat(web): plan window/bucketing/totals/balance math"
git push
```

---

### Task 4: Store state, mode toggle, sheet skeleton with cells

The plan becomes visible: Budget | Plan toggle on the page, a real grid with sticky name column, month headers, arrow navigation, current-month highlight, all rows with read-only cells, and the two new i18n/metrics wirings for open/navigate.

**Files:**
- Modify: `web/src/features/budgets/budgetStore.ts` (plan state)
- Create: `web/src/features/budgets/PlanSheet.tsx`
- Create: `web/src/features/budgets/PlanSheet.test.tsx`
- Modify: `web/src/features/budgets/BudgetPage.tsx` (toggle + plan-mode branch)
- Modify: `web/src/lib/metrics.ts` (3 keys — all three land now; hide-empty fires in Task 7)
- Modify: `locales/{de,en,es,fr,it,nl,pl,pt,ru,uk,zh}.json` (ALL plan keys for the whole plan land here once — one catalogue pass, not five)
- Test: `web/src/features/budgets/PlanSheet.test.tsx`

**Interfaces:**
- Consumes: Tasks 2–3 exports.
- Produces:

```ts
// budgetStore.ts additions (same persisted store)
interface BudgetPeriodState {
  // ...existing...
  budgetMode: 'budget' | 'plan'
  setBudgetMode: (m: 'budget' | 'plan') => void          // fires BUDGET_PLAN_OPEN when switching TO plan
  planFirstMonth: string | null
  setPlanFirstMonth: (month: string) => void             // fires BUDGET_PLAN_CHANGE_WINDOW
  planFolds: Record<string, true>                        // folded section keys ('income', folder ids, 'archived')
  togglePlanFold: (key: string) => void
  planHideEmpty: boolean
  togglePlanHideEmpty: () => void                        // fires BUDGET_PLAN_HIDE_EMPTY_TOGGLE (wired in Task 7 UI)
}

// PlanSheet.tsx
export function PlanSheet({ budget, currencies, userId }: {
  budget: BudgetDto            // the ALREADY-LOADED budget (meta for permissions/currency); plan data fetched inside
  currencies: CurrencyDto[]
  userId: Id | undefined
}): ReactNode
```

- The month header row and every cell carry `data-month={month}` and rows `data-row-id={element.id}:{element.type}` (types disambiguate the two uncategorized rows) — Tasks 5/8/9 hook onto these.
- New metrics keys (exact): `BUDGET_PLAN_OPEN: 'appBudgetPlanOpen'`, `BUDGET_PLAN_CHANGE_WINDOW: 'appBudgetPlanChangeWindow'`, `BUDGET_PLAN_HIDE_EMPTY_TOGGLE: 'appBudgetPlanHideEmptyToggle'`. All three are FIRED from the store actions above, so `metrics-coverage` passes from this task on (the store is the choke point; the hide-empty UI lands in Task 7 but the action exists now).
- New i18n keys (namespace `budgets.page.plan.*`; English values frozen here):

```
budgets.page.plan.toggle.budget      = "Budget"
budgets.page.plan.toggle.plan        = "Plan"
budgets.page.plan.section.income     = "Income"
budgets.page.plan.section.archived   = "Archived"
budgets.page.plan.nav.prev           = "Earlier months"
budgets.page.plan.nav.next           = "Later months"
budgets.page.plan.cell.aria          = "{name}, {month}: actual {actual}, planned {planned}"
budgets.page.plan.totals.income      = "Income"
budgets.page.plan.totals.expenses    = "Expenses"
budgets.page.plan.totals.net         = "Net"
budgets.page.plan.totals.balance     = "Balance"
budgets.page.plan.density.hide_empty = "Hide empty rows"
budgets.page.plan.density.hidden     = "{count} hidden"
budgets.page.plan.density.show       = "Show"
budgets.page.plan.menu.move_to_folder = "Move to folder…"
budgets.page.plan.menu.no_folder      = "No folder"
```

Translate all 16 into the other ten catalogues (de, es, fr, it, nl, pl, pt, ru, uk, zh) in this task — the i18n guards enforce key + `{var}` parity, and later tasks then touch no catalogue. Match each catalogue's existing tone/terminology for "envelope/budget/folder" (grep the sibling `budgets.*` keys for the terms used per language).

- [ ] **Step 1: Write the failing test**

Create `web/src/features/budgets/PlanSheet.test.tsx` (reuse `BudgetPage.test.tsx`'s scaffolding: msw `coreHandlers`, `renderPage` with QueryClient+Router, `mockViewport`; add a `get-budget-plan` handler returning `fixtureWirePlan`):

```tsx
it('toggle switches to plan mode and renders the sheet: months, income on top, cells', async () => {
  renderBudgetPageWithPlanHandlers()
  await screen.findByRole('tablist', { name: 'period' })          // budget mode loaded
  await userEvent.click(screen.getByRole('tab', { name: /plan/i }))
  // month headers from the fixture window
  await screen.findByText(/jul/i)                                  // localized month header
  // income section header exists and sits ABOVE the first expense row in DOM order
  const income = screen.getByTestId('plan-section-income')
  const firstExpense = screen.getByTestId('plan-section-expense')
  expect(income.compareDocumentPosition(firstExpense) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  // a cell shows actual over planned
  const cell = screen.getAllByTestId(`plan-cell-pe1:0`)[0]         // element pe1, month col 0
  expect(within(cell).getByTestId('cell-actual')).toBeInTheDocument()
  expect(within(cell).getByTestId('cell-planned')).toBeInTheDocument()
})

it('arrows shift the window by one month, clamped at the budget start', async () => { ... assert setPlanFirstMonth effect via rendered month headers; at the start month the prev arrow is disabled ... })

it('mode and first month persist in the store', async () => { ... switch to plan, navigate, unmount, remount, still plan mode at the same month ... })
```

Also a store-level test (same file): `setBudgetMode('plan')` fires `BUDGET_PLAN_OPEN` once and not when already in plan; `setPlanFirstMonth` fires `BUDGET_PLAN_CHANGE_WINDOW` (spy on `trackEvent` via `vi.mock('@/lib/metrics', ...)` — grep how existing tests spy on it and mirror).

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm test -- --run PlanSheet`
Expected: FAIL — no toggle, no PlanSheet.

- [ ] **Step 3: Implement the store additions**

`budgetStore.ts` — add to the persisted state (defaults: `budgetMode: 'budget'`, `planFirstMonth: null`, `planFolds: {}`, `planHideEmpty: false`):

```ts
      budgetMode: 'budget',
      setBudgetMode: (m) => {
        if (m === 'plan' && get().budgetMode !== 'plan') {
          trackEvent(METRICS.BUDGET_PLAN_OPEN)
        }
        set({ budgetMode: m })
      },
      planFirstMonth: null,
      setPlanFirstMonth: (month) => {
        trackEvent(METRICS.BUDGET_PLAN_CHANGE_WINDOW)
        set({ planFirstMonth: normalizePeriod(month) })
      },
      planFolds: {},
      togglePlanFold: (key) =>
        set((state) => {
          const next = { ...state.planFolds }
          if (next[key]) {
            delete next[key]
          } else {
            next[key] = true
          }
          return { planFolds: next }
        }),
      planHideEmpty: false,
      togglePlanHideEmpty: () => {
        trackEvent(METRICS.BUDGET_PLAN_HIDE_EMPTY_TOGGLE)
        set((state) => ({ planHideEmpty: !state.planHideEmpty }))
      },
```

- [ ] **Step 4: Implement `PlanSheet.tsx` (skeleton scope)**

This task's scope: layout + data + navigation + read-only cells. (Editing = Task 5; totals = Task 6; density UI = Task 7; keyboard = Task 9 — leave clear seams, no dead UI.)

Structure (complete component, ~200 lines):

```tsx
export function PlanSheet({ budget, currencies, userId }: PlanSheetProps) {
  const { t, i18n } = useTranslation()
  const containerRef = useRef<HTMLDivElement | null>(null)
  const [width, setWidth] = useState(0)
  useEffect(() => {                                   // ResizeObserver -> width
    const el = containerRef.current
    if (!el) return
    const ro = new ResizeObserver((entries) => setWidth(entries[0]?.contentRect.width ?? 0))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  const visible = width > 0 ? planVisibleCount(width) : 3

  const startedAt = budget.meta.startedAt
  const persisted = useBudgetPeriodStore((s) => s.planFirstMonth)
  const setPlanFirstMonth = useBudgetPeriodStore((s) => s.setPlanFirstMonth)
  const hideEmpty = useBudgetPeriodStore((s) => s.planHideEmpty)
  const firstMonth = clampFirstMonth(persisted ?? planInitialFirstMonth(null, startedAt, visible), startedAt)

  const { data: plan, isPending, planKey, fetchFrom } = useBudgetPlan(budget.meta.id, firstMonth, visible)

  // visible month strings + their index into the FETCHED window (buffered)
  const visibleMonths = Array.from({ length: visible }, (_, i) => addMonths(firstMonth, i))
  const monthIndex = (m: string): number => (plan ? plan.months.indexOf(m) : -1)

  const rows = useMemo(() => (plan ? bucketPlanRows(plan, hideEmpty) : null), [plan, hideEmpty])
  const ex = useMemo(() => (plan ? makePlanExchange(plan, currencies) : null), [plan, currencies])

  if (!plan || !rows) {
    return isPending ? <div className="flex flex-1 items-center justify-center"><CoinLoader label={t('common.app.modal.loading.data_loading')} /></div> : null
  }

  const cur = currentMonth()
  const monthFmt = new Intl.DateTimeFormat(i18n.language, { month: 'short', year: '2-digit' })
  const atStart = firstMonth <= startedAt.slice(0, 7) + '-01'

  const headerCell = (m: string) => (
    <div key={m} data-month={m} className={`px-2 py-1 text-right text-xs uppercase tracking-wide ${m === cur ? 'font-bold text-foreground' : 'text-muted-foreground'}`}>
      {monthFmt.format(new Date(m))}
    </div>
  )

  // one row of cells for a parent element
  const cellsFor = (el: PlanElementDto) => visibleMonths.map((m) => {
    const i = monthIndex(m)
    const cell = i >= 0 ? el.cells[i] : undefined
    return (
      <div key={m} data-month={m} data-testid={`plan-cell-${el.id}:${visibleMonths.indexOf(m)}`} className={`flex flex-col items-end px-2 py-1 ${m === cur ? 'bg-accent/40' : ''}`}>
        <span data-testid="cell-actual" className="text-xs text-muted-foreground">{renderActual(cell, m, cur)}</span>
        <span data-testid="cell-planned" className="text-sm">{cell && cell.planned !== '' ? moneyFormat(cell.planned, currencyOf(el), { showCurrency: false, useNativePrecision: false }) : '—'}</span>
      </div>
    )
  })
  // renderActual: '-' (dash) when zero in a FUTURE month, else moneyFormat of actual
  ...

  return (
    <div ref={containerRef} className="flex min-h-0 flex-1 flex-col overflow-y-auto" data-testid="plan-sheet">
      <div className="sticky top-0 z-10 grid bg-background" style={{ gridTemplateColumns: gridCols }}>
        <div className="flex items-center gap-1 px-2">
          <Button type="button" variant="ghost" size="icon" aria-label={t('budgets.page.plan.nav.prev')} disabled={atStart}
            onClick={() => setPlanFirstMonth(addMonths(firstMonth, -1))}><ChevronLeft className="size-4" /></Button>
          <Button type="button" variant="ghost" size="icon" aria-label={t('budgets.page.plan.nav.next')}
            onClick={() => setPlanFirstMonth(addMonths(firstMonth, 1))}><ChevronRight className="size-4" /></Button>
        </div>
        {visibleMonths.map(headerCell)}
      </div>
      {/* Income area */}
      <section data-testid="plan-section-income">
        <SectionHeader label={t('budgets.page.plan.section.income')} />
        {rows.income.folders.map((f) => <FolderRows key={f.folder.id} section={f} />)}
        {rows.income.loose.map((r) => <ElementRow key={rowKey(r)} row={r} />)}
        {rows.income.uncategorized ? <ElementRow row={rows.income.uncategorized} /> : null}
      </section>
      {/* Expense area (folders as section headers, then loose, then uncategorized) */}
      <section data-testid="plan-section-expense"> ... same shape ... </section>
      {/* Archived */}
      {rows.archived.length > 0 ? <section data-testid="plan-section-archived"> ... </section> : null}
      {/* Totals block mounts here in Task 6 */}
    </div>
  )
}
```

Fill in the elided pieces as real code: `gridTemplateColumns` = `${PLAN_NAME_COL_PX}px repeat(${visible}, minmax(${PLAN_MIN_MONTH_COL_PX}px, 1fr))`; `ElementRow` renders the sticky name cell (icon + `elementDisplayName(el.id, el.name, t)`) then `cellsFor(el)`; envelope rows get a chevron toggling child rows (children render actual-only cells, reusing the store's existing `unfoldedElements`/`toggleElement`); rows are `role="row"`-annotated grid rows with `data-row-id`. Long names truncate. On `visible === 1` the same grid renders with one month column — no separate code path.

- [ ] **Step 5: Wire the toggle into `BudgetPage.tsx`**

After the header, replace the bare `<PeriodStrip …/>` line with a mode branch:

```tsx
const budgetMode = useBudgetPeriodStore((s) => s.budgetMode)
const setBudgetMode = useBudgetPeriodStore((s) => s.setBudgetMode)
```

```tsx
<div role="tablist" aria-label="budget mode" className="flex w-fit rounded-md border p-0.5">
  {(['budget', 'plan'] as const).map((m) => (
    <button key={m} type="button" role="tab" aria-selected={budgetMode === m}
      className={`rounded px-3 py-1 text-sm uppercase tracking-wide ${budgetMode === m ? 'bg-accent font-bold' : 'text-muted-foreground'}`}
      onClick={() => setBudgetMode(m)}>
      {t(m === 'budget' ? 'budgets.page.plan.toggle.budget' : 'budgets.page.plan.toggle.plan')}
    </button>
  ))}
</div>
{budgetMode === 'plan' ? (
  <PlanSheet budget={budget} currencies={currencies} userId={user?.id} />
) : (
  <> {/* the ENTIRE existing PeriodStrip + edit-mode strip + table block moves inside this branch, unchanged */} </>
)}
```

Plan mode keeps the page header (budget name, currency chips, settings) and hides the period strip + month table. Edit-structure mode remains a budget-mode-only affair (the settings menu item can stay enabled; entering it just shows budget mode — set `budgetMode` to `'budget'` when `editMode` turns on).

- [ ] **Step 6: metrics + locales**

Add the three `METRICS` keys; add all 16 `budgets.page.plan.*` keys to `locales/en.json` (exact English values above) and translated equivalents to the other ten catalogues. Then verify the Go-side guards:

```bash
export PATH=$HOME/.local/go/bin:$PATH
go test ./internal/test/i18ntest/ -count=1
```

Expected: PASS (key parity, placeholder parity, frontend `t()` coverage).

- [ ] **Step 7: Run tests to verify they pass**

Run: `pnpm test -- --run && pnpm exec tsc -b && pnpm lint`
Expected: PASS — including `metrics-coverage` (all three new keys fire from the store) and all pre-existing BudgetPage tests (budget mode default means they see the old page; if a test renders the page fresh and trips over the new toggle, update ONLY selectors, never behavior).

- [ ] **Step 8: Commit + push**

```bash
git add web/src/features/budgets/ web/src/lib/metrics.ts locales/ web/src/test/fixtures.ts
git commit -m "feat(web): plan mode — toggle, responsive month grid, read-only cells"
git push
```

---

### Task 5: Editable planned cells

**Files:**
- Modify: `web/src/features/budgets/LimitEditor.tsx` (decouple from `BudgetElementDto`)
- Modify: `web/src/features/budgets/SetLimitDialog.tsx` (same)
- Modify: `web/src/features/budgets/BudgetPage.tsx` (adapt the two call sites to the new props)
- Modify: `web/src/features/budgets/PlanSheet.tsx` (editable cells)
- Test: `web/src/features/budgets/PlanSheet.test.tsx` (extend)

**Interfaces:**
- Consumes: `usePlanSetLimit` (Task 2), `canUpdateLimits(meta, userId, period)` (existing — already period-explicit).
- Produces:

```tsx
// LimitEditor decoupled props (desktop popover)
interface LimitEditorProps {
  id: string                 // stable htmlFor/id seed
  name: string               // aria label
  value: string              // current amount ('' or '0' -> empty input)
  currency: CurrencyDto | undefined
  onCommit: (amount: string | null) => void
}
// SetLimitDialog decoupled props (compact screens)
interface SetLimitDialogProps {
  target: { id: string; name: string; value: string } | null
  onClose: () => void
  onCommit: (elementId: string, amount: string | null) => void
}
```

**Semantics:** a plan cell is editable iff `canUpdateLimits(budget.meta, userId, month)` AND the row is a non-archived parent that is NOT an uncategorized row. Children and uncategorized are actual-only. Past months stay editable; months before the budget start are read-only (that IS `canUpdateLimits`' date check). Desktop: clicking the planned value opens `LimitEditor`'s popover; compact (`useIsCompact`): tapping the planned value opens `SetLimitDialog`. Commit calls `usePlanSetLimit(planKey).mutate({ budgetId, elementId: el.id, period: month, amount, monthIndex })` — `monthIndex` is the index in the FETCHED window (`plan.months.indexOf(month)`).

- [ ] **Step 1: Write the failing tests**

```tsx
it('editing a planned cell sends set-limit with the cell month and patches optimistically', async () => {
  // click pe1's planned value in the second visible column, type 350, save;
  // assert POST body { elementId: 'pe1', period: <that month>, amount: '350' }
  // and the cell immediately shows 350 (before the invalidated refetch resolves)
})
it('uncategorized and child cells are not editable; guest role sees no editors', async () => { ... })
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `pnpm test -- --run PlanSheet`
Expected: FAIL.

- [ ] **Step 3: Refactor the editors**

`LimitEditor.tsx`: replace `element`/`currency` props with the Interfaces shape — inside, `element.budgeted` → `value`, `element.name` → `name`, `element.id` → `id`. `SetLimitDialog.tsx`: `element: BudgetElementDto | null` → `target`; `element.budgeted` → `target.value`, description → `target.name`. Update `BudgetPage.tsx` call sites:

```tsx
<LimitEditor id={element.id} name={element.name} value={element.budgeted} currency={...} onCommit={...} />
...
<SetLimitDialog target={limitTarget ? { id: limitTarget.id, name: limitTarget.name, value: limitTarget.budgeted } : null} ... />
```

(`limitTarget` stays a `BudgetElementDto` state — only the dialog's props narrow.)

- [ ] **Step 4: Wire editing into `PlanSheet.tsx`**

In `cellsFor`, when the cell is editable render the planned span as a `LimitEditor` (desktop) trigger with `id={`${el.id}-${m}`}`, `value={cell.planned === '' ? '0' : cell.planned}`, committing through `usePlanSetLimit(planKey)`; on compact, planned taps set a `planLimitTarget` state `{ el, month, monthIndex }` driving one shared `SetLimitDialog`. Add the overspend highlight while here: on the CURRENT and FUTURE months, an expense cell with `cmp(actual, planned') > 0 && planned !== ''` gets a subtle warning class (e.g. `text-destructive` on the actual span) — spec: the cell whose actual feeds the projection.

- [ ] **Step 5: Run tests to verify they pass**

Run: `pnpm test -- --run && pnpm exec tsc -b && pnpm lint`
Expected: PASS (including the untouched budget-mode limit tests against the refactored editors).

- [ ] **Step 6: Commit + push**

```bash
git add web/src/features/budgets/
git commit -m "feat(web): editable plan cells via period-explicit limit editors"
git push
```

---

### Task 6: Totals block with the Balance row

**Files:**
- Modify: `web/src/features/budgets/PlanSheet.tsx` (totals footer)
- Test: `web/src/features/budgets/PlanSheet.test.tsx` (extend)

**Interfaces:**
- Consumes: `planTotals`, `balanceRow`, `makePlanExchange` (Task 3).
- Produces: a sticky-bottom totals block, four labeled rows — Income / Expenses / Net as `actual | planned` pairs, Balance as one running value per column — all in the budget currency.

- [ ] **Step 1: Write the failing test**

```tsx
it('totals block renders income/expenses/net pairs and the running balance', async () => {
  // with fixtureWirePlan's hand-computable numbers: assert the rendered Balance
  // value of the LAST visible column equals seed + Σ effectiveNet (precompute in the test
  // by calling planTotals/balanceRow directly on the fixture — the sheet must agree with the math core)
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm test -- --run PlanSheet` — FAIL (no totals block).

- [ ] **Step 3: Implement**

Compute once per render: `const totals = planTotals(plan, ex)`, `const balance = balanceRow(plan, totals, ex)`, indexed by FETCHED month index; render only visible columns (`monthIndex(m)`). Footer = a `sticky bottom-0` grid with the same `gridTemplateColumns`; rows labeled with `t('budgets.page.plan.totals.income'|'expenses'|'net'|'balance')`; actual|planned pairs stacked like data cells; negative Balance values styled `text-destructive`. `data-testid="plan-totals"` + per-cell `data-testid={`plan-balance-${i}`}`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm test -- --run && pnpm exec tsc -b && pnpm lint` — PASS.

- [ ] **Step 5: Commit + push**

```bash
git add web/src/features/budgets/
git commit -m "feat(web): plan totals block with running balance projection"
git push
```

---

### Task 7: Density controls — fold sections, hide empty rows

**Files:**
- Modify: `web/src/features/budgets/PlanSheet.tsx`
- Test: `web/src/features/budgets/PlanSheet.test.tsx` (extend)

**Interfaces:**
- Consumes: store `planFolds`/`togglePlanFold`/`planHideEmpty`/`togglePlanHideEmpty` (Task 4), `bucketPlanRows`' `hidden`/`hiddenCount` (Task 3).
- Produces: clickable section headers (income header, each folder header, archived header) that fold/unfold their rows (chevron state from `planFolds`, key = `'income'` | folder id | `'archived'`); a "hide empty rows" toggle in the sheet's toolbar row (a small labeled switch/checkbox above the grid); while ON, each section header with hidden rows shows `{count} hidden — Show` (i18n keys from Task 4), where Show reveals that section's hidden rows for the session (a `revealedSections: Set<string>` component state passed down so `bucketPlanRows` gets `hideEmpty && !revealed`).

- [ ] **Step 1: Write the failing tests**

```tsx
it('folding a section header collapses its rows and persists', async () => { ... })
it('hide-empty removes dormant rows, shows the per-section count, Show reveals them', async () => { ... })
it('the hide-empty toggle fires its metric', async () => { ... spy on trackEvent ... })
```

- [ ] **Step 2: Run to verify FAIL**, **Step 3: implement** (fold state on `SectionHeader` — clicking the header text toggles; the toolbar checkbox binds `planHideEmpty`; per-section reveal as described), **Step 4: full gates** (`pnpm test -- --run && pnpm exec tsc -b && pnpm lint`), **Step 5: commit + push**:

```bash
git add web/src/features/budgets/
git commit -m "feat(web): plan density controls — section folds and hide-empty rows"
git push
```

---

### Task 8: Income management — side-constrained envelope dialog, create-from-section, move-to-folder menu

**Files:**
- Modify: `web/src/api/budget.ts` (`EnvelopeForm` gains `side?: 'income'`)
- Modify: `web/src/features/budgets/EnvelopeDialog.tsx` (side prop)
- Modify: `web/src/features/budgets/PlanSheet.tsx` (income section "+" + row menus)
- Modify: `web/src/features/budgets/BudgetPage.tsx` (pass `side="expense"` explicitly at its call site)
- Test: `web/src/features/budgets/PlanSheet.test.tsx` (extend)

**Interfaces:**
- Consumes: existing `useCreateEnvelope`/`useUpdateEnvelope`/`useMoveElement` mutations (they invalidate the budget query; ALSO invalidate the plan: extend `useInvalidateBudget` in `queries.ts` to invalidate `queryKeys.budgetPlan` alongside `queryKeys.budget` — one-line change that fixes every structure mutation for plan mode at once).
- Produces:

```tsx
// EnvelopeDialog gains:
interface EnvelopeDialogProps { /* existing */ side: 'expense' | 'income' }
// filter: c.type === (side === 'income' ? 'income' : 'expense'); create submits side to the API when 'income'
```

**Semantics (spec):** the income section header shows the "+" (members with edit rights — `canConfigureBudget`) opening `EnvelopeDialog` with `side="income"`; the dialog's category picker offers ONLY that side's non-archived categories; a single dialog never mixes sides; editing an income envelope from a plan row menu opens it with `side="income"` (derive from `isIncomeType(el.type)`); update never sends `side` (immutable). Every non-uncategorized, non-child plan row gets a row menu (`MoreVertical`) with "Move to folder…" listing side-filtered targets: income rows see income-sided + neutral folders, expense rows see expense-sided + neutral folders, plus "No folder"; picking one calls `useMoveElement().mutate({ budgetId, item: { id: el.id, folderId, afterId: null } })`. Envelope rows additionally get Edit (and income envelopes render under the income section per Task 3's bucketing — no new code). Folder side for the menu comes from the SAME derivation as `bucketPlanRows` — export a small `folderSides(plan): Map<Id, 'income' | 'expense' | 'neutral'>` from `planMath.ts` and use it in both places.

- [ ] **Step 1: Write the failing tests**

```tsx
it('income + opens the envelope dialog constrained to income categories and submits side=income', async () => { ... assert POST create-envelope body has side: 'income' and the picker lists no expense category ... })
it('move-to-folder menu lists only same-side and neutral folders and calls move-element', async () => { ... })
it('budget-mode envelope dialog still offers expense categories only', async () => { ... })
```

- [ ] **Step 2: FAIL**, **Step 3: implement** (per Interfaces/Semantics; `folderSides` extracted in planMath with a unit test appended to `planMath.test.ts`), **Step 4: gates**, **Step 5: commit + push**:

```bash
git add web/src/features/budgets/ web/src/api/budget.ts web/src/features/budgets/planMath.ts web/src/features/budgets/planMath.test.ts
git commit -m "feat(web): income envelopes and move-to-folder from the plan sheet"
git push
```

---

### Task 9: Keyboard grid, ARIA, mobile polish, full gates

**Files:**
- Modify: `web/src/features/budgets/PlanSheet.tsx`
- Test: `web/src/features/budgets/PlanSheet.test.tsx` (extend)

**Interfaces:**
- Consumes: everything prior.
- Produces: the ARIA grid pattern — the sheet container `role="grid"`, rows `role="row"`, cells `role="gridcell"` with `aria-selected`; a roving selection `{ rowKey: string; col: number } | null` in component state.

**Semantics (spec):** ArrowUp/Down walk data rows (skipping section headers and the totals block); ArrowLeft/Right walk visible columns; moving left from column 0 (or right from the last) shifts the window one month (same as the arrow buttons) keeping the selection's row; Enter on an editable selected cell opens its planned editor prefilled (Enter commits, Esc cancels — the popover already does both); Enter on read-only cells does nothing; Enter on an envelope's NAME cell toggles expansion. Cells expose the `budgets.page.plan.cell.aria` label with `{name}/{month}/{actual}/{planned}` filled. Selection is set by click too. Keyboard handler lives on the grid container (`onKeyDown`, container `tabIndex={0}`).

- [ ] **Step 1: Write the failing tests**

```tsx
it('arrow keys move the selection and shift the window at the edges', async () => { ... })
it('Enter opens the editor on an editable cell and is inert on read-only cells', async () => { ... })
it('cells expose the aria label and aria-selected', async () => { ... })
```

- [ ] **Step 2: FAIL**, **Step 3: implement** (selection state + key handler + `aria-*`; build the flat ordered row list from the same `PlanRows` the renderer walks so navigation and rendering can't diverge), **Step 4: FULL gates**:

```bash
cd web && pnpm test -- --run && pnpm exec tsc -b && pnpm lint && pnpm build
cd .. && export PATH=$HOME/.local/go/bin:$PATH && make go-test
make test   # full: go + pgsql/engines (docker postgres:17 pattern from plan 02) + web
```

Expected: ALL PASS. (`pnpm build` proves the production bundle; `make go-test` re-proves the i18n guards over the final catalogues.)

- [ ] **Step 5: Commit + push**

```bash
git add web/src/features/budgets/
git commit -m "feat(web): plan sheet keyboard grid and ARIA; full-suite gate"
git push
```

---

## Ordering & dependencies

1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9. Tasks 5–8 all extend `PlanSheet.tsx` + its test file and MUST run sequentially (no parallel dispatch). Task 4 lands ALL i18n catalogue keys and ALL metrics keys for the whole plan so later tasks never touch `locales/` again.

## Out of scope (v1 — from the spec)

- Drag re-ordering of income rows/folders in the plan view (assignment is the move-to-folder menu only).
- Per-cell carryover ("available") display; server-side totals.
- Any change to the budget page's single-month view or wire contract.
- Archive/unarchive actions in row menus.
- The `exchange` missing-rate silent-1:1 fallback: known and accepted (spec notes awareness, not a fix).
