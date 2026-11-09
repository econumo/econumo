import { act, render, renderHook, screen, waitFor } from '@testing-library/react'
import { onlineManager, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser, fixtureWireBudget, fixtureWirePlan } from '@/test/fixtures'
import type { BudgetDto, BudgetPlanDto } from '@/api/dto/budget'
import { queryKeys } from '@/app/queryKeys'
import { NotSavedIcon, PendingLimitMark } from '@/features/pendingWrites/NotSavedIcon'
import { getOps, loadPendingWrites, resetPendingWritesForTests } from '@/features/pendingWrites/store'
import { useBudgetPeriodStore } from './budgetStore'
import { planFetchWindow, useBudget, useBudgetPlan, useFillPlannedCells, usePlanSetLimit, useSetLimit } from './queries'

function makeWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, networkMode: 'always' }, mutations: { retry: false, networkMode: 'always' } } })
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  return { queryClient, wrapper }
}

describe('limits offline', () => {
  let requests: string[]

  beforeEach(async () => {
    localStorage.clear()
    window.econumoConfig = {}
    requests = []
    server.use(...coreHandlers())
    server.events.on('request:start', ({ request }) => {
      if (request.method === 'POST') {
        requests.push(new URL(request.url).pathname)
      }
    })
    resetPendingWritesForTests()
    onlineManager.setOnline(false)
    await loadPendingWrites('u1')
  })

  afterEach(() => {
    server.events.removeAllListeners()
    onlineManager.setOnline(true)
  })

  it('an offline useSetLimit shows the new budgeted from useBudget, sends nothing, and survives a refetch', async () => {
    const { queryClient, wrapper } = makeWrapper()
    useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
    queryClient.setQueryData(queryKeys.user, { ...fixtureUser, options: fixtureUser.options.map((o) => (o.name === 'budget' ? { ...o, value: 'b1' } : o)) })
    queryClient.setQueryData([...queryKeys.budget, 'b1', '2026-07-01'], fixtureWireBudget)
    const { result } = renderHook(() => ({ budget: useBudget(), set: useSetLimit() }), { wrapper })
    await waitFor(() => expect(result.current.budget.data).toBeDefined())
    const budgetKey = [...queryKeys.budget, result.current.budget.data!.meta.id, '2026-07-01']
    const budgetId = result.current.budget.data!.meta.id

    await act(() => result.current.set.mutateAsync({ budgetId, elementId: 'cat-food', period: '2026-07-01', amount: '250' }))

    expect(requests).not.toContain('/api/v1/budget/set-limit')
    expect(getOps()).toHaveLength(1)
    const budgeted = () => result.current.budget.data!.structure.elements.find((e) => e.id === 'cat-food')?.budgeted
    expect(budgeted()).toBe('250')

    // the server still has the old figure; the waiting change stays on top of it
    act(() => {
      queryClient.setQueryData<BudgetDto>(budgetKey, fixtureWireBudget as unknown as BudgetDto)
    })
    await waitFor(() => expect(budgeted()).toBe('250'))
  })

  it('an offline usePlanSetLimit shows the new planned cell and sends nothing', async () => {
    const { queryClient, wrapper } = makeWrapper()
    const { from, months } = planFetchWindow('2026-07-01', 2, 1)
    queryClient.setQueryData([...queryKeys.budgetPlan, 'b1', from, months], fixtureWirePlan)
    const { result } = renderHook(() => ({ plan: useBudgetPlan('b1', '2026-07-01', 2, 1), set: usePlanSetLimit(['x']) }), { wrapper })
    await waitFor(() => expect(result.current.plan.data).toBeDefined())
    const row = result.current.plan.data!.structure.elements[0]!
    const index = result.current.plan.data!.months.indexOf('2026-07-01')

    await act(() => result.current.set.mutateAsync({ budgetId: 'b1', elementId: row.id, period: '2026-07-01', amount: '321', monthIndex: index }))

    expect(requests).not.toContain('/api/v1/budget/set-limit')
    const planned = () => result.current.plan.data!.structure.elements.find((e) => e.id === row.id)!.cells[index]!.planned
    expect(planned()).toBe('321')
  })

  it('fill-right queues one op per target, each with its own key', async () => {
    const { queryClient, wrapper } = makeWrapper()
    const planKey = [...queryKeys.budgetPlan, 'b1', '2026-05-01', 10] as const
    queryClient.setQueryData<BudgetPlanDto>(planKey, fixtureWirePlan as unknown as BudgetPlanDto)
    const { result } = renderHook(() => useFillPlannedCells(planKey), { wrapper })
    const elementId = (fixtureWirePlan as unknown as BudgetPlanDto).structure.elements[0]!.id

    await act(() =>
      result.current.mutateAsync({
        budgetId: 'b1',
        elementId,
        amount: '40',
        targets: [{ period: '2026-06-01', monthIndex: 1 }, { period: '2026-07-01', monthIndex: 2 }],
      }),
    )

    expect(requests).not.toContain('/api/v1/budget/set-limit')
    const ops = getOps()
    expect(ops.map((o) => o.kind === 'limit' && o.period).sort()).toEqual(['2026-06-01', '2026-07-01'])
    expect(new Set(ops.map((o) => o.key)).size).toBe(2)
  })

  it('a queued limit is kept as an op and the raw cache returns to server truth', async () => {
    const { queryClient, wrapper } = makeWrapper()
    const key = [...queryKeys.budget, 'b1', '2026-09-01']
    queryClient.setQueryData(key, fixtureWireBudget)
    const { result } = renderHook(() => useSetLimit(), { wrapper })
    await act(() => result.current.mutateAsync({ budgetId: 'b1', elementId: 'cat-food', period: '2026-09-01', amount: '250' }))
    expect(getOps()).toHaveLength(1)
    expect(queryClient.getQueryData<BudgetDto>(key)?.structure.elements.find((e) => e.id === 'cat-food')?.budgeted).toBe('200')
  })

  it('an online refusal still rolls the cell back', async () => {
    onlineManager.setOnline(true)
    server.use(http.post('*/api/v1/budget/set-limit', () => HttpResponse.json({ success: false, message: 'no', code: 0, errors: {} }, { status: 400 })))
    const { queryClient, wrapper } = makeWrapper()
    const key = [...queryKeys.budget, 'b1', '2026-09-01']
    queryClient.setQueryData(key, fixtureWireBudget)
    const { result } = renderHook(() => useSetLimit(), { wrapper })
    await act(async () => {
      await result.current.mutateAsync({ budgetId: 'b1', elementId: 'cat-food', period: '2026-09-01', amount: '250' }).catch(() => undefined)
    })
    expect(queryClient.getQueryData<BudgetDto>(key)?.structure.elements.find((e) => e.id === 'cat-food')?.budgeted).toBe('200')
    expect(getOps()).toHaveLength(0)
  })

  it('only the waiting cell renders the not-saved icon', async () => {
    const { wrapper } = makeWrapper()
    const { result } = renderHook(() => useSetLimit(), { wrapper })
    await act(() => result.current.mutateAsync({ budgetId: 'b1', elementId: 'cat-food', period: '2026-07-01', amount: '9' }))
    render(
      <>
        <PendingLimitMark budgetId="b1" elementId="cat-food" period="2026-07-01" />
        <PendingLimitMark budgetId="b1" elementId="env-1" period="2026-07-01" />
        <PendingLimitMark budgetId="b1" elementId="cat-food" period="2026-08-01" />
      </>,
    )
    expect(screen.getAllByRole('img', { name: 'Not saved' })).toHaveLength(1)
  })

  it('NotSavedIcon carries the translated label', () => {
    render(<NotSavedIcon />)
    expect(screen.getByRole('img', { name: 'Not saved' })).toBeInTheDocument()
  })
})

describe('limits offline: cache truth and month switches', () => {
  beforeEach(async () => {
    localStorage.clear()
    window.econumoConfig = {}
    server.use(...coreHandlers())
    resetPendingWritesForTests()
    onlineManager.setOnline(false)
    await loadPendingWrites('u1')
  })
  afterEach(() => onlineManager.setOnline(true))

  const seed = (queryClient: QueryClient) => {
    queryClient.setQueryData(queryKeys.user, { ...fixtureUser, options: fixtureUser.options.map((o) => (o.name === 'budget' ? { ...o, value: 'b1' } : o)) })
    queryClient.setQueryData([...queryKeys.budget, 'b1', '2026-07-01'], fixtureWireBudget)
  }
  const budgeted = (b: BudgetDto | null | undefined) => b?.structure.elements.find((e) => e.id === 'cat-food')?.budgeted

  it('a discarded queued limit leaves the server figure, not the optimistic one', async () => {
    const { queryClient, wrapper } = makeWrapper()
    useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
    seed(queryClient)
    const { result } = renderHook(() => ({ budget: useBudget(), set: useSetLimit() }), { wrapper })
    await act(() => result.current.set.mutateAsync({ budgetId: 'b1', elementId: 'cat-food', period: '2026-07-01', amount: '250' }))
    expect(budgeted(result.current.budget.data)).toBe('250')

    await act(async () => {
      const { mutate } = await import('@/features/pendingWrites/store')
      await mutate(() => [])
    })
    expect(budgeted(result.current.budget.data)).toBe('200')
  })

  it('a placeholder from another month keeps its own waiting limit and takes none of the selected month', async () => {
    const { queryClient, wrapper } = makeWrapper()
    useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
    seed(queryClient)
    const { result } = renderHook(() => ({ budget: useBudget(), set: useSetLimit() }), { wrapper })
    await act(() => result.current.set.mutateAsync({ budgetId: 'b1', elementId: 'cat-food', period: '2026-07-01', amount: '250' }))
    await act(() => result.current.set.mutateAsync({ budgetId: 'b1', elementId: 'cat-food', period: '2026-08-01', amount: '999' }))

    // August is uncached and offline: the query pauses with July as the placeholder
    act(() => useBudgetPeriodStore.setState({ selectedDate: '2026-08-01' }))
    expect(result.current.budget.isPlaceholderData).toBe(true)
    expect(budgeted(result.current.budget.data)).toBe('250')
  })
})
