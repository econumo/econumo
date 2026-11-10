import { act, renderHook, waitFor } from '@testing-library/react'
import { onlineManager, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser, fixtureWireBudget, fixtureWirePlan } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { useBudgetPeriodStore } from './budgetStore'
import { monthPlanSpan, planFetchWindow, useBudget, useBudgetPlan } from './queries'
import { addMonths, monthDiff, planWindow } from './planMath'
import { keptBudgetMonths, useKeepBudgetMonths } from './offlineMonths'
import type { BudgetMetaDto } from '@/api/dto/budget'

const userWith = (budget: string | null) => ({
  ...fixtureUser,
  options: fixtureUser.options.map((o) => (o.name === 'budget' ? { ...o, value: budget } : o)),
})

const DELAY = 60

function makeClient() {
  // gcTime 0: anything left unobserved is dropped at once
  return new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
}

function wrapperFor(client: QueryClient) {
  return ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

describe('useKeepBudgetMonths', () => {
  let requests: string[]
  let budgetDelay: number

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date(2026, 6, 15, 12))
    localStorage.clear()
    window.econumoConfig = {}
    requests = []
    budgetDelay = 0
    server.use(
      ...coreHandlers(),
      http.get('*/api/v1/budget/get-budget', async ({ request }) => {
        const url = new URL(request.url)
        requests.push(`budget ${url.searchParams.get('id')} ${url.searchParams.get('date')}`)
        if (budgetDelay > 0) {
          await new Promise((r) => setTimeout(r, budgetDelay))
        }
        return HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })
      }),
      http.get('*/api/v1/budget/get-budget-plan', ({ request }) => {
        const url = new URL(request.url)
        requests.push(`plan ${url.searchParams.get('id')} ${url.searchParams.get('from')} ${url.searchParams.get('months')}`)
        return HttpResponse.json({ success: true, message: '', data: { item: fixtureWirePlan } })
      }),
    )
    onlineManager.setOnline(true)
  })

  afterEach(() => {
    vi.useRealTimers()
    onlineManager.setOnline(true)
  })

  const expectedSet = (id: string) => [
    `budget ${id} 2026-06-01`,
    `budget ${id} 2026-07-01`,
    `budget ${id} 2026-08-01`,
    `plan ${id} 2025-12-01 21`,
  ]

  it('fetches the previous, current and next period and one plan window, once', async () => {
    const client = makeClient()
    client.setQueryData(queryKeys.user, userWith('b1'))
    const { rerender } = renderHook(() => useKeepBudgetMonths(DELAY), { wrapper: wrapperFor(client) })
    await waitFor(() => expect([...requests].sort()).toEqual(expectedSet('b1')))
    rerender()
    await new Promise((r) => setTimeout(r, 50))
    expect(requests).toHaveLength(4)
  })

  it('fetches nothing without a default budget', async () => {
    const client = makeClient()
    client.setQueryData(queryKeys.user, userWith(null))
    renderHook(() => useKeepBudgetMonths(DELAY), { wrapper: wrapperFor(client) })
    await waitFor(() => expect(client.getQueryData(queryKeys.budgets)).toBeDefined())
    await new Promise((r) => setTimeout(r, 50))
    expect(requests).toEqual([])
  })

  it('fetches nothing offline, and the set once the connection returns', async () => {
    const client = makeClient()
    client.setQueryData(queryKeys.user, userWith('b1'))
    client.setQueryData(queryKeys.budgets, [fixtureWireBudget.meta])
    onlineManager.setOnline(false)
    renderHook(() => useKeepBudgetMonths(DELAY), { wrapper: wrapperFor(client) })
    await new Promise((r) => setTimeout(r, 50))
    expect(requests).toEqual([])
    act(() => onlineManager.setOnline(true))
    await waitFor(() => expect([...requests].sort()).toEqual(expectedSet('b1')))
  })

  it('a page opened on one of the months renders from the cache with no request', async () => {
    const client = makeClient()
    client.setQueryData(queryKeys.user, userWith('b1'))
    const wrapper = wrapperFor(client)
    renderHook(() => useKeepBudgetMonths(DELAY), { wrapper })
    await waitFor(() => expect(requests).toHaveLength(4))

    for (const month of ['2026-06-01', '2026-07-01', '2026-08-01']) {
      useBudgetPeriodStore.setState({ selectedDate: month, periodChosen: true })
      const span = monthPlanSpan(month, '2026-07-01')
      const { result, unmount } = renderHook(() => ({ budget: useBudget(), plan: useBudgetPlan('b1', span.first, span.visible, 1) }), { wrapper })
      expect(result.current.budget.data).toBeDefined()
      expect(result.current.plan.data).toBeDefined()
      unmount()
    }
    await new Promise((r) => setTimeout(r, 50))
    expect(requests).toHaveLength(4)
  })

  it('keeps the entries from garbage collection while another month is dropped', async () => {
    const client = makeClient()
    client.setQueryData(queryKeys.user, userWith('b1'))
    const wrapper = wrapperFor(client)
    renderHook(() => useKeepBudgetMonths(DELAY), { wrapper })
    await waitFor(() => expect(requests).toHaveLength(4))
    useBudgetPeriodStore.setState({ selectedDate: '2026-11-01', periodChosen: true })
    const { result, unmount } = renderHook(() => useBudget(), { wrapper })
    await waitFor(() => expect(result.current.data).toBeDefined())
    unmount()
    await new Promise((r) => setTimeout(r, 20))

    expect(client.getQueryData([...queryKeys.budget, 'b1', '2026-11-01'])).toBeUndefined()
    for (const month of ['2026-06-01', '2026-07-01', '2026-08-01']) {
      expect(client.getQueryData([...queryKeys.budget, 'b1', month])).toBeDefined()
    }
    expect(client.getQueryData([...queryKeys.budgetPlan, 'b1', '2025-12-01', 21])).toBeDefined()
  })

  it('a burst of invalidations refetches the kept set once, after a pause', async () => {
    const client = makeClient()
    client.setQueryData(queryKeys.user, userWith('b1'))
    renderHook(() => useKeepBudgetMonths(DELAY), { wrapper: wrapperFor(client) })
    await waitFor(() => expect(requests).toHaveLength(4))
    for (let i = 0; i < 5; i++) {
      await act(() => client.invalidateQueries({ queryKey: queryKeys.budget }))
      await act(() => client.invalidateQueries({ queryKey: queryKeys.budgetPlan }))
      await new Promise((r) => setTimeout(r, 10))
    }
    // nothing yet: no page shows the budget
    expect(requests).toHaveLength(4)
    await waitFor(() => expect([...requests.slice(4)].sort()).toEqual(expectedSet('b1')))
    await new Promise((r) => setTimeout(r, DELAY * 3))
    expect(requests).toHaveLength(8)
  })

  it('a Budget page on screen still refetches its month at once', async () => {
    const client = makeClient()
    client.setQueryData(queryKeys.user, userWith('b1'))
    const wrapper = wrapperFor(client)
    renderHook(() => useKeepBudgetMonths(DELAY), { wrapper })
    await waitFor(() => expect(requests).toHaveLength(4))
    useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', periodChosen: true })
    renderHook(() => useBudget(), { wrapper })
    await act(() => client.invalidateQueries({ queryKey: queryKeys.budget }))
    expect(requests.slice(4)).toEqual(['budget b1 2026-07-01'])
    // the rest of the set follows after the pause, the page's month not again
    await waitFor(() => expect([...requests.slice(5)].sort()).toEqual(['budget b1 2026-06-01', 'budget b1 2026-08-01']))
    await new Promise((r) => setTimeout(r, DELAY * 3))
    expect(requests).toHaveLength(7)
  })

  // a write as the app makes one: a mutation whose success invalidates the budget queries
  const save = (client: QueryClient) =>
    act(() =>
      client
        .getMutationCache()
        .build(client, {
          mutationFn: async () => 'ok',
          onSuccess: () => {
            void client.invalidateQueries({ queryKey: queryKeys.budget })
            void client.invalidateQueries({ queryKey: queryKeys.budgetPlan })
          },
        })
        .execute(undefined),
    )

  it('a save while the delayed refetch is in flight refetches the set again', async () => {
    const client = makeClient()
    client.setQueryData(queryKeys.user, userWith('b1'))
    renderHook(() => useKeepBudgetMonths(DELAY), { wrapper: wrapperFor(client) })
    await waitFor(() => expect(requests).toHaveLength(4))
    budgetDelay = 150
    await save(client)
    await waitFor(() => expect(requests).toHaveLength(8))
    // the refetch is still waiting for get-budget: this save's figures are not in it
    await save(client)
    await waitFor(() => expect(requests).toHaveLength(12), { timeout: 3000 })
    await new Promise((r) => setTimeout(r, DELAY * 3 + 200))
    expect(requests).toHaveLength(12)
  })

  it('a save while the first fetch is in flight refetches the set after it', async () => {
    budgetDelay = 150
    const client = makeClient()
    client.setQueryData(queryKeys.user, userWith('b1'))
    renderHook(() => useKeepBudgetMonths(DELAY), { wrapper: wrapperFor(client) })
    await waitFor(() => expect(requests).toHaveLength(4))
    await save(client)
    await waitFor(() => expect(requests).toHaveLength(8), { timeout: 3000 })
    await new Promise((r) => setTimeout(r, DELAY * 3 + 200))
    expect(requests).toHaveLength(8)
  })

  it('every save restarts the wait', async () => {
    const client = makeClient()
    client.setQueryData(queryKeys.user, userWith('b1'))
    renderHook(() => useKeepBudgetMonths(DELAY), { wrapper: wrapperFor(client) })
    await waitFor(() => expect(requests).toHaveLength(4))
    for (let i = 0; i < 4; i++) {
      await save(client)
      await new Promise((r) => setTimeout(r, DELAY / 2))
    }
    // two delays' worth of saves, none of them a pause long enough
    expect(requests).toHaveLength(4)
    await waitFor(() => expect(requests).toHaveLength(8))
  })

  it('a budget starting two periods out keeps the previous period covered by the plan window', () => {
    const meta = { ...fixtureWireBudget.meta, startedAt: '2026-09-01 00:00:00' } as unknown as BudgetMetaDto
    const kept = keptBudgetMonths(meta, new Date(2026, 6, 15))
    // it opens on its first period, so the kept ones are around that
    expect(kept.periods).toEqual(['2026-08-01', '2026-09-01', '2026-10-01'])
    const last = addMonths(kept.plan.from, kept.plan.months - 1)
    for (const selected of kept.periods) {
      // the Plan grid clamps its window to the budget's start; the Budget view does not
      for (const [first, visible] of [[planWindow(selected, 12, meta.startedAt), 12], [monthPlanSpan(selected, '2026-07-01').first, monthPlanSpan(selected, '2026-07-01').visible]] as const) {
        expect(kept.plan.from <= first).toBe(true)
        expect(addMonths(first, visible - 1) <= last).toBe(true)
      }
    }
  })

  it('the plan window holds the widest Plan grid on every kept period, in 21 months', () => {
    const meta = fixtureWireBudget.meta as unknown as BudgetMetaDto
    const kept = keptBudgetMonths(meta, new Date(2026, 6, 15))
    expect(kept.plan).toEqual({ from: '2025-12-01', months: 21 })
    const last = addMonths(kept.plan.from, kept.plan.months - 1)
    for (const selected of kept.periods) {
      for (const visible of [3, 6, 9, 12]) {
        const first = planWindow(selected, visible, meta.startedAt)
        expect(kept.plan.from <= first && addMonths(first, visible - 1) <= last).toBe(true)
      }
    }
  })

  it('keeps the period before the first of a budget that starts this period', async () => {
    const client = makeClient()
    client.setQueryData(queryKeys.user, userWith('b1'))
    client.setQueryData(queryKeys.budgets, [{ ...fixtureWireBudget.meta, startedAt: '2026-07-01 00:00:00' }])
    renderHook(() => useKeepBudgetMonths(DELAY), { wrapper: wrapperFor(client) })
    await waitFor(() => expect([...requests].sort()).toEqual(expectedSet('b1')))
  })

  it('a new default budget prefetches its own set', async () => {
    const client = makeClient()
    client.setQueryData(queryKeys.user, userWith('b1'))
    renderHook(() => useKeepBudgetMonths(DELAY), { wrapper: wrapperFor(client) })
    await waitFor(() => expect(requests).toHaveLength(4))
    act(() => client.setQueryData(queryKeys.user, userWith('b2')))
    await waitFor(() => expect([...requests.slice(4)].sort()).toEqual(expectedSet('b2')))
  })

  it('a new current period prefetches the new set', async () => {
    const client = makeClient()
    client.setQueryData(queryKeys.user, userWith('b1'))
    renderHook(() => useKeepBudgetMonths(DELAY), { wrapper: wrapperFor(client) })
    await waitFor(() => expect(requests).toHaveLength(4))
    vi.setSystemTime(new Date(2026, 7, 2, 9))
    act(() => {
      document.dispatchEvent(new Event('visibilitychange'))
    })
    // July and August are still fresh; September and the moved plan window are new
    await waitFor(() => expect([...requests.slice(4)].sort()).toEqual(['budget b1 2026-09-01', 'plan b1 2026-01-01 21']))
  })
})

describe('monthPlanSpan', () => {
  it('gives the previous, current and next period the kept window', () => {
    const spans = ['2026-06-01', '2026-07-01', '2026-08-01'].map((m) => monthPlanSpan(m, '2026-07-01'))
    expect(spans).toEqual([spans[0], spans[0], spans[0]])
    expect(planFetchWindow(spans[0].first, spans[0].visible, 1, new Date(2026, 6, 15))).toEqual({ from: '2025-12-01', months: 21 })
  })

  it('a month further away keeps the window it had: from it, or from the current period to it', () => {
    expect(monthPlanSpan('2026-05-01', '2026-07-01')).toEqual({ first: '2026-05-01', visible: 1 })
    expect(monthPlanSpan('2026-09-01', '2026-07-01')).toEqual({ first: '2026-07-01', visible: 3 })
  })

  it('a far future month stays inside the fetched window', () => {
    const now = new Date(2026, 6, 15)
    for (const ahead of [2, 21]) {
      const selected = addMonths('2026-07-01', ahead)
      const span = monthPlanSpan(selected, '2026-07-01')
      const w = planFetchWindow(span.first, span.visible, 1, now)
      expect(monthDiff(w.from, selected)).toBeLessThan(w.months)
    }
  })
})
