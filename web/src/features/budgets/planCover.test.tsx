import { act, renderHook, waitFor } from '@testing-library/react'
import { onlineManager, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import type { BudgetPlanDto } from '@/api/dto/budget'
import { BudgetElementType } from '@/api/dto/budget'
import { server } from '@/test/msw'
import { coreHandlers, fixtureEur, fixtureUsd, fixtureWirePlan } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { isUnavailableOffline } from '@/lib/offlineQuery'
import { addMonths, planWindow } from './planMath'
import { planMonthFigures } from './phoneMonth'
import { monthPlanSpan, useBudgetPlan } from './queries'
import { coveredPlan } from './planCover'

/** a plan window as the server answers it: every month-indexed part follows `months` */
function planFor(from: string, count: number, startedAt = '2026-01-01 00:00:00'): BudgetPlanDto {
  const months = Array.from({ length: count }, (_, i) => addMonths(from, i))
  const cell = (i: number) => ({ actual: String(10 + i), planned: String(20 + i), upcoming: '0' })
  return {
    ...(fixtureWirePlan as unknown as BudgetPlanDto),
    meta: { ...(fixtureWirePlan.meta as unknown as BudgetPlanDto['meta']), startedAt },
    months,
    openingBalances: [{ currencyId: 'cur-usd', amount: '500' }],
    currencyRates: months.map((m) => ({
      period: m,
      rates: [{ currencyId: 'cur-usd', baseCurrencyId: 'cur-usd', rate: '1', periodStart: m, periodEnd: addMonths(m, 1) }],
    })),
    transfers: months.map((m) => ({ period: m, items: [] })),
    structure: {
      folders: [],
      elements: [
        {
          id: 'salary', type: BudgetElementType.INCOME_CATEGORY, name: 'Salary', icon: '', currencyId: 'cur-usd', isArchived: 0, folderId: null,
          position: 0, ownerUserId: 'u1', cells: months.map((_, i) => cell(i)),
          children: [{ id: 'child', type: BudgetElementType.CATEGORY, name: 'Child', icon: '', isArchived: 0, ownerUserId: 'u1', cells: months.map((_, i) => ({ actual: String(i) })) }],
        },
      ],
      savings: [
        {
          id: 'acc-sav', type: BudgetElementType.SAVINGS, name: 'Savings', icon: '', currencyId: 'cur-usd', ownerUserId: 'u1', isArchived: 0, position: 0,
          cells: months.map((_, i) => ({ ...cell(i), closingBalance: String(100 + i) })),
        },
      ],
    },
    savingsOpeningBalances: [{ currencyId: 'cur-usd', amount: '100' }],
    savingsFlows: months.map((m, i) => ({ month: m, currencyId: 'cur-usd', amount: String(i) })),
    savingsIncomeExpense: months.map((m, i) => ({ month: m, currencyId: 'cur-usd', amount: String(i) })),
  }
}

function makeClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } })
}

const wrapperFor = (client: QueryClient) => ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>

// the window useKeepBudgetMonths keeps for July 2026: six periods either side
const KEPT = { from: '2025-12-01', months: 21 }

describe('coveredPlan', () => {
  it('keeps the window start and trims every month-indexed part after the last month', () => {
    const plan = planFor('2026-01-01', 13)
    const out = coveredPlan(plan, '2026-08-01')
    expect(out.months).toEqual(plan.months.slice(0, 8))
    expect(out.currencyRates.map((r) => r.period)).toEqual(out.months)
    expect(out.transfers.map((t) => t.period)).toEqual(out.months)
    expect(out.structure.elements[0].cells).toHaveLength(8)
    expect(out.structure.elements[0].children[0].cells).toHaveLength(8)
    expect(out.structure.savings?.[0].cells).toHaveLength(8)
    expect(out.savingsFlows?.map((f) => f.month)).toEqual(out.months)
    expect(out.savingsIncomeExpense?.map((f) => f.month)).toEqual(out.months)
    // the balances open where the window opens, so they stay as they are
    expect(out.openingBalances).toBe(plan.openingBalances)
    expect(out.savingsOpeningBalances).toBe(plan.savingsOpeningBalances)
  })
})

describe('useBudgetPlan served from a covering window offline', () => {
  let requests: string[]
  let planDelay: number

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date(2026, 6, 15, 12))
    requests = []
    planDelay = 0
    server.use(
      ...coreHandlers(),
      http.get('*/api/v1/budget/get-budget-plan', async ({ request }) => {
        const url = new URL(request.url)
        requests.push(`${url.searchParams.get('from')} ${url.searchParams.get('months')}`)
        if (planDelay > 0) {
          await new Promise((r) => setTimeout(r, planDelay))
        }
        return HttpResponse.json({ success: true, message: '', data: { item: planFor(url.searchParams.get('from')!, Number(url.searchParams.get('months'))) } })
      }),
    )
    onlineManager.setOnline(false)
  })

  afterEach(() => {
    vi.useRealTimers()
    onlineManager.setOnline(true)
  })

  function seeded(plan = planFor(KEPT.from, KEPT.months)) {
    const client = makeClient()
    client.setQueryData([...queryKeys.budgetPlan, 'b1', KEPT.from, KEPT.months], plan)
    return client
  }

  it('a wide grid gets its months from the kept window, never written under its own key', async () => {
    const client = seeded()
    // eight columns from the month before the selected one
    const first = planWindow('2026-07-01', 8, '2026-01-01')
    const { result } = renderHook(() => useBudgetPlan('b1', first, 8, 1), { wrapper: wrapperFor(client) })
    await waitFor(() => expect(result.current.data).toBeTruthy())
    const plan = result.current.data!
    for (let i = 0; i < 8; i++) {
      expect(plan.months).toContain(addMonths(first, i))
    }
    const cells = plan.structure.elements[0].cells
    expect(cells).toHaveLength(plan.months.length)
    // the cell under a month is that month's own
    expect(cells[plan.months.indexOf('2026-07-01')].actual).toBe('17')
    expect(isUnavailableOffline(result.current, false)).toBe(false)
    expect(result.current.isPending).toBe(false)
    expect(client.getQueryData(result.current.planKey)).toBeUndefined()
    expect(requests).toEqual([])
  })

  it('a 1920 px grid (twelve columns) on the next period is served too', async () => {
    const client = seeded()
    const first = planWindow('2026-08-01', 12, '2026-01-01')
    const { result } = renderHook(() => useBudgetPlan('b1', first, 12, 1), { wrapper: wrapperFor(client) })
    await waitFor(() => expect(result.current.data).toBeTruthy())
    expect(result.current.data!.months).toContain('2027-06-01')
  })

  it('keeps serving the covering window while the connection returns and the window loads', async () => {
    const client = seeded()
    const { result } = renderHook(() => useBudgetPlan('b1', '2026-06-01', 8, 1), { wrapper: wrapperFor(client) })
    await waitFor(() => expect(result.current.data?.months[0]).toBe(KEPT.from))
    planDelay = 150
    act(() => onlineManager.setOnline(true))
    await waitFor(() => expect(requests).toEqual(['2026-04-01 12']))
    expect(result.current.data?.months[0]).toBe(KEPT.from)
    expect(result.current.isPending).toBe(false)
    await waitFor(() => expect(result.current.data?.months[0]).toBe('2026-04-01'))
  })

  it('a cached window from an older server, with no transfers, is served without throwing', async () => {
    const { transfers: _t, savingsFlows: _f, savingsIncomeExpense: _i, ...old } = planFor(KEPT.from, KEPT.months)
    const client = seeded({ ...old, structure: { ...old.structure, savings: undefined } } as unknown as BudgetPlanDto)
    const { result } = renderHook(() => useBudgetPlan('b1', '2026-06-01', 3, 1), { wrapper: wrapperFor(client) })
    await waitFor(() => expect(result.current.data).toBeTruthy())
    expect(result.current.data!.transfers).toBeUndefined()
    expect(planMonthFigures(result.current.data!, [fixtureUsd, fixtureEur], '2026-07-01')).not.toBeNull()
  })

  it('a budget in its first period: the clamped window is served too', async () => {
    const client = seeded(planFor(KEPT.from, KEPT.months, '2026-07-01 00:00:00'))
    const first = planWindow('2026-07-01', 3, '2026-07-01')
    expect(first).toBe('2026-07-01')
    const { result } = renderHook(() => useBudgetPlan('b1', first, 3, 1), { wrapper: wrapperFor(client) })
    await waitFor(() => expect(result.current.data).toBeTruthy())
    expect(isUnavailableOffline(result.current, false)).toBe(false)
  })

  it('after the period rolled over offline, the Budget view figures come from the kept window', async () => {
    const kept = planFor(KEPT.from, KEPT.months)
    const client = seeded(kept)
    vi.setSystemTime(new Date(2026, 7, 2, 9))
    const span = monthPlanSpan('2026-08-01', '2026-08-01')
    const { result } = renderHook(() => useBudgetPlan('b1', span.first, span.visible, 1), { wrapper: wrapperFor(client) })
    await waitFor(() => expect(result.current.data).toBeTruthy())
    expect(result.current.isPlaceholderData).toBe(false)
    const currencies = [fixtureUsd, fixtureEur]
    const figures = planMonthFigures(result.current.data!, currencies, '2026-08-01')
    expect(figures).not.toBeNull()
    // the same figures as the whole window gives for that month (the rows carry their cells, cut)
    const numbers = (f: unknown) => JSON.stringify(f, (k, v) => (k === 'cells' ? undefined : v))
    expect(numbers(figures)).toBe(numbers(planMonthFigures(kept, currencies, '2026-08-01')))
    expect(numbers(figures)).toMatch(/"balance"/)
  })

  it('a window no cached one covers stays not saved', async () => {
    const client = seeded()
    const { result } = renderHook(() => useBudgetPlan('b1', '2028-03-01', 3, 1), { wrapper: wrapperFor(client) })
    await new Promise((r) => setTimeout(r, 20))
    expect(result.current.data).toBeUndefined()
    expect(isUnavailableOffline(result.current, false)).toBe(true)
  })

  it('another budget’s window covers nothing', async () => {
    const client = seeded()
    const { result } = renderHook(() => useBudgetPlan('b2', '2026-06-01', 3, 1), { wrapper: wrapperFor(client) })
    await new Promise((r) => setTimeout(r, 20))
    expect(result.current.data).toBeUndefined()
  })

  it('online, the exact window is fetched as before', async () => {
    onlineManager.setOnline(true)
    const client = seeded()
    const { result } = renderHook(() => useBudgetPlan('b1', '2026-06-01', 8, 1), { wrapper: wrapperFor(client) })
    await waitFor(() => expect(requests).toEqual(['2026-04-01 12']))
    await waitFor(() => expect(result.current.data?.months[0]).toBe('2026-04-01'))
  })
})
