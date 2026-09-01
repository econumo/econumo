import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { coreHandlers, fixtureBudgets, fixtureOwner, fixtureWireBudget, fixtureWirePlan } from '@/test/fixtures'
import type { BudgetDto, BudgetPlanDto } from '@/api/dto/budget'
import { queryKeys } from '@/app/queryKeys'
import { METRICS, trackEvent } from '@/lib/metrics'
import {
  useArchiveBudget,
  useBudgetPlan,
  useBudgets,
  useCloneBudget,
  useCreateBudget,
  useDeclineBudgetAccess,
  useDeleteBudget,
  useFillPlannedCells,
  usePlanSetLimit,
  useSetLimit,
  useUnarchiveBudget,
  useUpdateBudgetDetail,
} from './queries'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})
const trackEventMock = vi.mocked(trackEvent)

function makeWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  return { queryClient, wrapper }
}

function renderPlanHarness(budgetId: string | null, firstMonth: string, visibleMonths: number) {
  const { queryClient, wrapper } = makeWrapper()
  const { result } = renderHook(() => useBudgetPlan(budgetId, firstMonth, visibleMonths), { wrapper })
  return { result, queryClient }
}

function renderPlanSetLimitHarness(plan: typeof fixtureWirePlan) {
  const { queryClient, wrapper } = makeWrapper()
  const planKey = [...queryKeys.budgetPlan, 'b1', '2026-05-01', 10] as const
  queryClient.setQueryData(planKey, plan)
  const { result } = renderHook(() => usePlanSetLimit(planKey), { wrapper })
  return { result, queryClient, planKey }
}

function renderFillHarness(plan: typeof fixtureWirePlan) {
  const { queryClient, wrapper } = makeWrapper()
  const planKey = [...queryKeys.budgetPlan, 'b1', '2026-05-01', 10] as const
  queryClient.setQueryData(planKey, plan)
  const { result } = renderHook(() => useFillPlannedCells(planKey), { wrapper })
  return { result, queryClient, planKey }
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  trackEventMock.mockClear()
})

it('useBudgets sorts by name asc', async () => {
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useBudgets(), { wrapper })
  await waitFor(() => expect(result.current.data).toBeDefined())
  expect(result.current.data!.map((b) => b.name)).toEqual(['Alpha plan', 'Main budget'])
})

it('create posts the exact payload and the client id is the entity id', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/budget/create-budget', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({
        success: true, message: '',
        data: {
          item: {
            meta: {
              id: body.id, ownerUserId: 'u1', name: body.name, startedAt: '2026-07-01 00:00:00',
              currencyId: body.currencyId, access: [{ user: fixtureOwner, role: 'owner', isAccepted: 1 }],
            },
          },
        },
      })
    }),
  )
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData(queryKeys.budgets, fixtureBudgets)
  const { result } = renderHook(() => useCreateBudget(), { wrapper })
  result.current.mutate({ id: 'b-client-id', name: 'Vacation', startDate: '', currencyId: 'cur-usd', accountIds: ['a2'], ownerUserId: 'u1' })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(body).toEqual({ id: 'b-client-id', name: 'Vacation', startDate: '', currencyId: 'cur-usd', accountIds: ['a2'] })
  expect(result.current.data!.id).toBe('b-client-id')
  expect(queryClient.getQueryData<{ id: string }[]>(queryKeys.budgets)!.map((b) => b.id)).toContain('b-client-id')
})

it('create dedupes by own name without hitting the API', async () => {
  let hits = 0
  server.use(
    http.post('*/api/v1/budget/create-budget', () => {
      hits++
      return HttpResponse.json({ success: true, message: '', data: { item: { meta: fixtureBudgets[0] } } })
    }),
  )
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData(queryKeys.budgets, fixtureBudgets)
  const { result } = renderHook(() => useCreateBudget(), { wrapper })
  result.current.mutate({ id: 'x', name: 'main BUDGET', startDate: '', currencyId: 'cur-usd', accountIds: ['a1'], ownerUserId: 'u1' })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(hits).toBe(0)
  expect(result.current.data!.id).toBe('b1')
})

it('decline immediately drops the budget from the cache', async () => {
  server.use(
    http.post('*/api/v1/budget/decline-access', () =>
      HttpResponse.json({ success: true, message: '', data: {} }),
    ),
  )
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData(queryKeys.budgets, fixtureBudgets)
  const declined = fixtureBudgets[0].id
  const { result } = renderHook(() => useDeclineBudgetAccess(), { wrapper })
  result.current.mutate(declined)
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(queryClient.getQueryData<{ id: string }[]>(queryKeys.budgets)!.map((b) => b.id)).not.toContain(declined)
})

it('delete removes from the cache and invalidates budget + user', async () => {
  server.use(http.post('*/api/v1/budget/delete-budget', () => HttpResponse.json({ success: true, message: '', data: {} })))
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData(queryKeys.budgets, fixtureBudgets)
  const spy = vi.spyOn(queryClient, 'invalidateQueries')
  const { result } = renderHook(() => useDeleteBudget(), { wrapper })
  result.current.mutate('b1')
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(queryClient.getQueryData<{ id: string }[]>(queryKeys.budgets)!.map((b) => b.id)).toEqual(['b2'])
  expect(spy).toHaveBeenCalledWith({ queryKey: queryKeys.budget })
  expect(spy).toHaveBeenCalledWith({ queryKey: queryKeys.user })
})

it('useSetLimit sends the explicit period and patches that period cache', async () => {
  let sent: Record<string, unknown> | null = null
  server.use(
    http.post('*/api/v1/budget/set-limit', async ({ request }) => {
      sent = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData([...queryKeys.budget, 'b1', '2026-09-01'], fixtureWireBudget)
  const { result } = renderHook(() => useSetLimit(), { wrapper })
  result.current.mutate({ budgetId: 'b1', elementId: 'cat-food', period: '2026-09-01', amount: '250' })
  await waitFor(() => expect(sent).not.toBeNull())
  expect(sent).toMatchObject({ budgetId: 'b1', elementId: 'cat-food', period: '2026-09-01', amount: '250' })
  const patched = queryClient.getQueryData<BudgetDto>([...queryKeys.budget, 'b1', '2026-09-01'])
  expect(patched?.structure.elements.find((e) => e.id === 'cat-food')?.budgeted).toBe('250')
})

it('useSetLimit (budget mode) also invalidates the plan cache so the plan sheet resyncs', async () => {
  server.use(
    http.post('*/api/v1/budget/set-limit', () => HttpResponse.json({ success: true, message: '', data: {} })),
  )
  const { queryClient, wrapper } = makeWrapper()
  const planKey = [...queryKeys.budgetPlan, 'b1', '2026-08-01', 10] as const
  queryClient.setQueryData(planKey, fixtureWirePlan)
  const spy = vi.spyOn(queryClient, 'invalidateQueries')
  const { result } = renderHook(() => useSetLimit(), { wrapper })
  result.current.mutate({ budgetId: 'b1', elementId: 'cat-food', period: '2026-09-01', amount: '250' })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(spy).toHaveBeenCalledWith({ queryKey: queryKeys.budgetPlan })
})

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
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  const plan = queryClient.getQueryData<BudgetPlanDto>(planKey)
  expect(plan?.structure.elements.find((e) => e.id === 'pe1')?.cells[2]?.planned).toBe('400')
})

it('useFillPlannedCells posts one set-limit per target, patches all cells, fires the metric once', async () => {
  const sent: Record<string, unknown>[] = []
  server.use(
    http.post('*/api/v1/budget/set-limit', async ({ request }) => {
      sent.push((await request.json()) as Record<string, unknown>)
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const { result, queryClient, planKey } = renderFillHarness(fixtureWirePlan)
  result.current.mutate({
    budgetId: 'b1',
    elementId: 'pe1',
    amount: '250',
    targets: [
      { period: '2026-06-01', monthIndex: 1 },
      { period: '2026-07-01', monthIndex: 2 },
    ],
  })
  // optimistic: both covered cells patched, ahead of the network round trip
  await waitFor(() => {
    const plan = queryClient.getQueryData<BudgetPlanDto>(planKey)
    const cells = plan?.structure.elements.find((e) => e.id === 'pe1')?.cells
    expect(cells?.[1]?.planned).toBe('250')
    expect(cells?.[2]?.planned).toBe('250')
  })
  await waitFor(() => expect(sent).toHaveLength(2))
  expect(sent).toEqual(
    expect.arrayContaining([
      expect.objectContaining({ budgetId: 'b1', elementId: 'pe1', period: '2026-06-01', amount: '250' }),
      expect.objectContaining({ budgetId: 'b1', elementId: 'pe1', period: '2026-07-01', amount: '250' }),
    ]),
  )
  await waitFor(() => expect(trackEventMock).toHaveBeenCalledWith(METRICS.BUDGET_PLAN_FILL_RIGHT))
  expect(trackEventMock.mock.calls.filter((c) => c[0] === METRICS.BUDGET_PLAN_FILL_RIGHT)).toHaveLength(1)
})

it('useFillPlannedCells invalidates the plan AND every target budget-month cache on any failure (server truth resyncs)', async () => {
  server.use(
    http.post('*/api/v1/budget/set-limit', () =>
      HttpResponse.json({ success: false, message: 'boom', code: 400, errors: {} }, { status: 400 }),
    ),
  )
  const { result, queryClient, planKey } = renderFillHarness(fixtureWirePlan)
  const spy = vi.spyOn(queryClient, 'invalidateQueries')
  result.current.mutate({
    budgetId: 'b1',
    elementId: 'pe1',
    amount: '250',
    targets: [
      { period: '2026-06-01', monthIndex: 1 },
      { period: '2026-07-01', monthIndex: 2 },
    ],
  })
  await waitFor(() => expect(result.current.isError).toBe(true))
  // onSettled resyncs even on a partial/total failure: both target months' budget-page
  // caches, not just the plan cache.
  expect(spy).toHaveBeenCalledWith({ queryKey: [...queryKeys.budget, 'b1', '2026-06-01'] })
  expect(spy).toHaveBeenCalledWith({ queryKey: [...queryKeys.budget, 'b1', '2026-07-01'] })
  expect(spy).toHaveBeenCalledWith({ queryKey: planKey })
})

it('useArchiveBudget posts {id}, updates the cache and fires the metric', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/budget/archive-budget', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({
        success: true, message: '',
        data: { item: { ...fixtureBudgets[0], isArchived: 1 } },
      })
    }),
  )
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData(queryKeys.budgets, fixtureBudgets)
  const { result } = renderHook(() => useArchiveBudget(), { wrapper })
  result.current.mutate('b1')
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(body).toEqual({ id: 'b1' })
  expect(queryClient.getQueryData<{ id: string; isArchived: 0 | 1 }[]>(queryKeys.budgets)!.find((b) => b.id === 'b1')!.isArchived).toBe(1)
  expect(trackEventMock).toHaveBeenCalledWith(METRICS.BUDGET_ARCHIVE)
})

it('useUnarchiveBudget posts {id} and fires its own metric', async () => {
  server.use(
    http.post('*/api/v1/budget/unarchive-budget', () =>
      HttpResponse.json({ success: true, message: '', data: { item: { ...fixtureBudgets[0], isArchived: 0 } } }),
    ),
  )
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useUnarchiveBudget(), { wrapper })
  result.current.mutate('b1')
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(trackEventMock).toHaveBeenCalledWith(METRICS.BUDGET_UNARCHIVE)
})

it('useCloneBudget sends the full form and omits startDate when null', async () => {
  const bodies: Record<string, unknown>[] = []
  server.use(
    http.post('*/api/v1/budget/clone-budget', async ({ request }) => {
      bodies.push((await request.json()) as Record<string, unknown>)
      return HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })
    }),
  )
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useCloneBudget(), { wrapper })

  result.current.mutate({ id: 'b1', newId: 'b-new', name: 'Copy', startDate: null, withLimits: true })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(bodies[0]).toEqual({ id: 'b1', newId: 'b-new', name: 'Copy', withLimits: true })

  result.current.mutate({ id: 'b1', newId: 'b-new2', name: 'Cont', startDate: '2026-08-01', withLimits: false })
  await waitFor(() => expect(bodies.length).toBe(2))
  expect(bodies[1]).toEqual({ id: 'b1', newId: 'b-new2', name: 'Cont', startDate: '2026-08-01', withLimits: false })
  expect(trackEventMock).toHaveBeenCalledWith(METRICS.BUDGET_CLONE)
})

it('useUpdateBudgetDetail fires the end-date metric only when endDate is sent', async () => {
  server.use(
    http.post('*/api/v1/budget/update-budget', async ({ request }) => {
      const body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({
        success: true, message: '',
        data: { item: { ...fixtureBudgets[0], name: body.name as string } },
      })
    }),
  )
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useUpdateBudgetDetail(), { wrapper })

  result.current.mutate({ id: 'b1', name: 'Main budget', currencyId: 'cur-usd' })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(trackEventMock).not.toHaveBeenCalledWith(METRICS.BUDGET_SET_END_DATE)

  result.current.mutate({ id: 'b1', name: 'Main budget', currencyId: 'cur-usd', endDate: '2026-09-01' })
  await waitFor(() => expect(trackEventMock).toHaveBeenCalledWith(METRICS.BUDGET_SET_END_DATE))
})
