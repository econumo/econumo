import { act, render, renderHook, screen, waitFor } from '@testing-library/react'
import { onlineManager, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser, fixtureWireBudget, fixtureWirePlan } from '@/test/fixtures'
import type { BudgetDto, BudgetPlanDto } from '@/api/dto/budget'
import { queryKeys } from '@/app/queryKeys'
import { NotSavedIcon, PendingLimitMark } from '@/features/pendingWrites/NotSavedIcon'
import { getOps, loadPendingWrites, mutate, resetPendingWritesForTests } from '@/features/pendingWrites/store'
import { discardWrite } from '@/features/pendingWrites/submit'
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
    expect(result.current.budget.dataPeriod).toBe('2026-07-01')
  })
})

describe('limits: a queued result gives back only its own cells', () => {
  type Answer = 'lost' | 'refused' | 'ok'
  let answers: Map<string, (a: Answer) => void>

  const respond = (a: Answer) =>
    a === 'lost'
      ? HttpResponse.error()
      : a === 'refused'
        ? HttpResponse.json({ success: false, message: 'no', code: 0, errors: {} }, { status: 400 })
        : HttpResponse.json({ success: true, message: '', data: {} })

  beforeEach(async () => {
    localStorage.clear()
    window.econumoConfig = {}
    localStorage.setItem('token', 'tok')
    localStorage.setItem('sessionUserId', 'u1')
    answers = new Map()
    server.use(
      ...coreHandlers(),
      http.post('*/api/v1/budget/set-limit', async ({ request }) => {
        const body = (await request.json()) as { amount: string | null; period: string }
        const answer = await new Promise<Answer>((resolve) => answers.set(`${body.period}/${body.amount}`, resolve))
        return respond(answer)
      }),
    )
    resetPendingWritesForTests()
    onlineManager.setOnline(true)
    await loadPendingWrites('u1')
  })

  afterEach(() => {
    localStorage.clear()
  })

  const answer = async (name: string, a: Answer) => {
    await waitFor(() => expect(answers.has(name)).toBe(true))
    await act(async () => {
      answers.get(name)!(a)
      await new Promise((resolve) => setTimeout(resolve, 20))
    })
  }

  describe('useSetLimit', () => {
    const key = [...queryKeys.budget, 'b1', '2026-07-01']
    const food = { budgetId: 'b1', elementId: 'cat-food', period: '2026-07-01' }
    const rent = { budgetId: 'b1', elementId: 'env-1', period: '2026-07-01' }

    function setUp() {
      const { queryClient, wrapper } = makeWrapper()
      queryClient.setQueryData(key, fixtureWireBudget)
      const server0 = (id: string) => (fixtureWireBudget as unknown as BudgetDto).structure.elements.find((e) => e.id === id)?.budgeted
      const cell = (id: string) => queryClient.getQueryData<BudgetDto>(key)?.structure.elements.find((e) => e.id === id)?.budgeted
      const { result } = renderHook(() => useSetLimit(), { wrapper })
      return { queryClient, wrapper, cell, server0, send: (form: typeof food, amount: string) => { void result.current.mutateAsync({ ...form, amount }).catch(() => undefined) } }
    }

    it('A queueing leaves the patch of B, sent after it, in place', async () => {
      const { cell, server0, send } = setUp()
      send(food, '250')
      await waitFor(() => expect(cell('cat-food')).toBe('250'))
      send(rent, '777')
      await waitFor(() => expect(cell('env-1')).toBe('777'))

      await answer('2026-07-01/250', 'lost')

      expect(getOps()).toHaveLength(1)
      expect(cell('cat-food')).toBe(server0('cat-food'))
      expect(cell('env-1')).toBe('777')
    })

    it('B queueing never writes the unconfirmed A into the cache, so discarding a refused A leaves server truth', async () => {
      const { queryClient, wrapper, cell, server0, send } = setUp()
      queryClient.setQueryData(queryKeys.user, { ...fixtureUser, options: fixtureUser.options.map((o) => (o.name === 'budget' ? { ...o, value: 'b1' } : o)) })
      useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
      const { result: shown } = renderHook(() => useBudget(), { wrapper })
      send(food, '250')
      await waitFor(() => expect(cell('cat-food')).toBe('250'))
      send(rent, '777')
      await waitFor(() => expect(cell('env-1')).toBe('777'))

      await answer('2026-07-01/250', 'lost')
      await answer('2026-07-01/777', 'lost')
      expect(cell('cat-food')).toBe(server0('cat-food'))
      expect(cell('env-1')).toBe(server0('env-1'))

      const a = getOps().find((op) => op.kind === 'limit' && op.elementId === 'cat-food')!
      act(() => discardWrite(a.key))
      await waitFor(() => expect(shown.current.data?.structure.elements.find((e) => e.id === 'cat-food')?.budgeted).toBe(server0('cat-food')))
      expect(shown.current.data?.structure.elements.find((e) => e.id === 'env-1')?.budgeted).toBe('777')
    })

    it('two edits of one cell, both queued, leave the server value whichever answers first', async () => {
      for (const order of [['250', '300'], ['300', '250']]) {
        answers.clear()
        const { cell, server0, send } = setUp()
        send(food, '250')
        await waitFor(() => expect(cell('cat-food')).toBe('250'))
        send(food, '300')
        await waitFor(() => expect(cell('cat-food')).toBe('300'))
        await answer(`2026-07-01/${order[0]}`, 'lost')
        await answer(`2026-07-01/${order[1]}`, 'lost')
        expect(cell('cat-food')).toBe(server0('cat-food'))
        await act(() => mutate(() => []))
      }
    })

    it('an online refusal rolls back only the refused cell', async () => {
      const { cell, server0, send } = setUp()
      send(food, '250')
      await waitFor(() => expect(cell('cat-food')).toBe('250'))
      send(rent, '777')
      await waitFor(() => expect(cell('env-1')).toBe('777'))

      await answer('2026-07-01/250', 'refused')

      expect(cell('cat-food')).toBe(server0('cat-food'))
      expect(cell('env-1')).toBe('777')
      expect(getOps()).toHaveLength(0)
    })

    it('a refusal of the earlier edit of a cell leaves the later one, and a refusal of that restores the server value', async () => {
      const { cell, server0, send } = setUp()
      send(food, '250')
      await waitFor(() => expect(cell('cat-food')).toBe('250'))
      send(food, '300')
      await waitFor(() => expect(cell('cat-food')).toBe('300'))

      await answer('2026-07-01/250', 'refused')
      expect(cell('cat-food')).toBe('300')

      await answer('2026-07-01/300', 'refused')
      expect(cell('cat-food')).toBe(server0('cat-food'))
    })
  })

  describe('plan cells', () => {
    const planKey = [...queryKeys.budgetPlan, 'b1', '2026-05-01', 4] as const
    const plan = fixtureWirePlan as unknown as BudgetPlanDto
    const elementId = plan.structure.elements[0]!.id
    const original = (i: number) => plan.structure.elements[0]!.cells[i]!.planned

    function setUp() {
      const { queryClient, wrapper } = makeWrapper()
      queryClient.setQueryData(planKey, fixtureWirePlan)
      const cell = (i: number) => queryClient.getQueryData<BudgetPlanDto>(planKey)?.structure.elements[0]!.cells[i]!.planned
      return { wrapper, cell }
    }

    it('usePlanSetLimit: A queueing keeps B, B queueing brings back nothing of A', async () => {
      const { wrapper, cell } = setUp()
      const { result } = renderHook(() => usePlanSetLimit(planKey), { wrapper })
      void result.current.mutateAsync({ budgetId: 'b1', elementId, period: '2026-06-01', amount: '250', monthIndex: 1 })
      await waitFor(() => expect(cell(1)).toBe('250'))
      void result.current.mutateAsync({ budgetId: 'b1', elementId, period: '2026-07-01', amount: '777', monthIndex: 2 })
      await waitFor(() => expect(cell(2)).toBe('777'))

      await answer('2026-06-01/250', 'lost')
      expect(cell(1)).toBe(original(1))
      expect(cell(2)).toBe('777')
      await answer('2026-07-01/777', 'lost')
      expect(cell(1)).toBe(original(1))
      expect(cell(2)).toBe(original(2))
    })

    it('fill-right gives back the targets that queued and keeps the ones confirmed directly', async () => {
      const { wrapper, cell } = setUp()
      const { result } = renderHook(() => useFillPlannedCells(planKey), { wrapper })
      void result.current.mutateAsync({ budgetId: 'b1', elementId, amount: '40', targets: [{ period: '2026-06-01', monthIndex: 1 }, { period: '2026-07-01', monthIndex: 2 }] })
      await waitFor(() => expect(cell(1)).toBe('40'))
      await answer('2026-06-01/40', 'ok')
      await answer('2026-07-01/40', 'lost')
      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(cell(1)).toBe('40')
      expect(cell(2)).toBe(original(2))
    })

    it('fill-right leaves a cell another edit patched while it was out', async () => {
      const { wrapper, cell } = setUp()
      const { result } = renderHook(() => ({ fill: useFillPlannedCells(planKey), set: usePlanSetLimit(planKey) }), { wrapper })
      void result.current.fill.mutateAsync({ budgetId: 'b1', elementId, amount: '40', targets: [{ period: '2026-06-01', monthIndex: 1 }] })
      await waitFor(() => expect(cell(1)).toBe('40'))
      void result.current.set.mutateAsync({ budgetId: 'b1', elementId, period: '2026-07-01', amount: '777', monthIndex: 2 })
      await waitFor(() => expect(cell(2)).toBe('777'))

      await answer('2026-06-01/40', 'lost')
      await waitFor(() => expect(result.current.fill.isSuccess).toBe(true))
      expect(cell(1)).toBe(original(1))
      expect(cell(2)).toBe('777')
    })
  })
})
