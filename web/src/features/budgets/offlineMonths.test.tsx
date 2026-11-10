import { act, renderHook, waitFor } from '@testing-library/react'
import { onlineManager, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser, fixtureWireBudget, fixtureWirePlan } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { useBudgetPeriodStore } from './budgetStore'
import { monthPlanSpan, useBudget, useBudgetPlan } from './queries'
import { useKeepBudgetMonths } from './offlineMonths'

const userWith = (budget: string | null) => ({
  ...fixtureUser,
  options: fixtureUser.options.map((o) => (o.name === 'budget' ? { ...o, value: budget } : o)),
})

function makeClient() {
  // gcTime 0: anything left unobserved is dropped at once
  return new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
}

function wrapperFor(client: QueryClient) {
  return ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

describe('useKeepBudgetMonths', () => {
  let requests: string[]

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date(2026, 6, 15, 12))
    localStorage.clear()
    window.econumoConfig = {}
    requests = []
    server.use(
      ...coreHandlers(),
      http.get('*/api/v1/budget/get-budget', ({ request }) => {
        const url = new URL(request.url)
        requests.push(`budget ${url.searchParams.get('id')} ${url.searchParams.get('date')}`)
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
    `plan ${id} 2026-04-01 7`,
  ]

  it('fetches the previous, current and next period and one plan window, once', async () => {
    const client = makeClient()
    client.setQueryData(queryKeys.user, userWith('b1'))
    const { rerender } = renderHook(() => useKeepBudgetMonths(), { wrapper: wrapperFor(client) })
    await waitFor(() => expect([...requests].sort()).toEqual(expectedSet('b1')))
    rerender()
    await new Promise((r) => setTimeout(r, 50))
    expect(requests).toHaveLength(4)
  })

  it('fetches nothing without a default budget', async () => {
    const client = makeClient()
    client.setQueryData(queryKeys.user, userWith(null))
    renderHook(() => useKeepBudgetMonths(), { wrapper: wrapperFor(client) })
    await waitFor(() => expect(client.getQueryData(queryKeys.budgets)).toBeDefined())
    await new Promise((r) => setTimeout(r, 50))
    expect(requests).toEqual([])
  })

  it('fetches nothing offline, and the set once the connection returns', async () => {
    const client = makeClient()
    client.setQueryData(queryKeys.user, userWith('b1'))
    client.setQueryData(queryKeys.budgets, [fixtureWireBudget.meta])
    onlineManager.setOnline(false)
    renderHook(() => useKeepBudgetMonths(), { wrapper: wrapperFor(client) })
    await new Promise((r) => setTimeout(r, 50))
    expect(requests).toEqual([])
    act(() => onlineManager.setOnline(true))
    await waitFor(() => expect([...requests].sort()).toEqual(expectedSet('b1')))
  })

  it('a page opened on one of the months renders from the cache with no request', async () => {
    const client = makeClient()
    client.setQueryData(queryKeys.user, userWith('b1'))
    const wrapper = wrapperFor(client)
    renderHook(() => useKeepBudgetMonths(), { wrapper })
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
    renderHook(() => useKeepBudgetMonths(), { wrapper })
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
    expect(client.getQueryData([...queryKeys.budgetPlan, 'b1', '2026-04-01', 7])).toBeDefined()
  })

  it('refetches the set once after a write invalidates the budget queries', async () => {
    const client = makeClient()
    client.setQueryData(queryKeys.user, userWith('b1'))
    renderHook(() => useKeepBudgetMonths(), { wrapper: wrapperFor(client) })
    await waitFor(() => expect(requests).toHaveLength(4))
    await act(() => client.invalidateQueries({ queryKey: queryKeys.budget }))
    await act(() => client.invalidateQueries({ queryKey: queryKeys.budgetPlan }))
    await waitFor(() => expect(requests).toHaveLength(8))
    await new Promise((r) => setTimeout(r, 50))
    expect(requests).toHaveLength(8)
  })

  it('a new default budget prefetches its own set', async () => {
    const client = makeClient()
    client.setQueryData(queryKeys.user, userWith('b1'))
    renderHook(() => useKeepBudgetMonths(), { wrapper: wrapperFor(client) })
    await waitFor(() => expect(requests).toHaveLength(4))
    act(() => client.setQueryData(queryKeys.user, userWith('b2')))
    await waitFor(() => expect([...requests.slice(4)].sort()).toEqual(expectedSet('b2')))
  })

  it('a new current period prefetches the new set', async () => {
    const client = makeClient()
    client.setQueryData(queryKeys.user, userWith('b1'))
    renderHook(() => useKeepBudgetMonths(), { wrapper: wrapperFor(client) })
    await waitFor(() => expect(requests).toHaveLength(4))
    vi.setSystemTime(new Date(2026, 7, 2, 9))
    act(() => {
      document.dispatchEvent(new Event('visibilitychange'))
    })
    // July and August are still fresh; September and the moved plan window are new
    await waitFor(() => expect([...requests.slice(4)].sort()).toEqual(['budget b1 2026-09-01', 'plan b1 2026-05-01 7']))
  })
})

describe('monthPlanSpan', () => {
  it('gives the previous, current and next period one window', () => {
    const spans = ['2026-06-01', '2026-07-01', '2026-08-01'].map((m) => monthPlanSpan(m, '2026-07-01'))
    expect(spans).toEqual([spans[0], spans[0], spans[0]])
    expect(spans[0]).toEqual({ first: '2026-06-01', visible: 3 })
  })

  it('reaches out to a month further away', () => {
    expect(monthPlanSpan('2026-03-01', '2026-07-01')).toEqual({ first: '2026-03-01', visible: 6 })
    expect(monthPlanSpan('2026-10-01', '2026-07-01')).toEqual({ first: '2026-06-01', visible: 5 })
  })
})
