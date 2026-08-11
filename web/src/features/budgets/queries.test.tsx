import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { coreHandlers, fixtureBudgets, fixtureOwner, fixtureWireBudget, fixtureWirePlan } from '@/test/fixtures'
import type { BudgetDto, BudgetPlanDto } from '@/api/dto/budget'
import { queryKeys } from '@/app/queryKeys'
import { useBudgetPlan, useBudgets, useCreateBudget, useDeclineBudgetAccess, useDeleteBudget, usePlanSetLimit, useSetLimit } from './queries'

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

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
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
  result.current.mutate({ id: 'b-client-id', name: 'Vacation', startDate: '', currencyId: 'cur-usd', excludedAccounts: ['a2'], ownerUserId: 'u1' })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(body).toEqual({ id: 'b-client-id', name: 'Vacation', startDate: '', currencyId: 'cur-usd', excludedAccounts: ['a2'] })
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
  result.current.mutate({ id: 'x', name: 'main BUDGET', startDate: '', currencyId: 'cur-usd', excludedAccounts: [], ownerUserId: 'u1' })
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
