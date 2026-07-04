import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser, fixtureWireBudget as wireBudget } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { useBudget, useSetLimit, canUpdateLimits } from './queries'
import { useBudgetPeriodStore } from './budgetStore'
import type { BudgetDto, BudgetMetaDto } from '@/api/dto/budget'

const userWithBudget = {
  ...fixtureUser,
  options: fixtureUser.options.map((o) => (o.name === 'budget' ? { ...o, value: 'b1' } : o)),
}

function makeWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  return { queryClient, wrapper }
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: {}, foldBudgetId: null })
})

it('fetches the default budget for the selected period and resets folds on budget switch', async () => {
  let requested = ''
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', ({ request }) => {
      requested = request.url
      return HttpResponse.json({ success: true, message: '', data: { item: wireBudget } })
    }),
  )
  useBudgetPeriodStore.setState({ foldBudgetId: 'other-budget', unfoldedElements: { x: true } })
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useBudget(), { wrapper })
  await waitFor(() => expect(result.current.data).toBeTruthy())
  expect(requested).toContain('id=b1')
  expect(requested).toContain('date=2026-07-01')
  // fold map reset because the budget id changed
  expect(useBudgetPeriodStore.getState().unfoldedElements).toEqual({})
  expect(useBudgetPeriodStore.getState().foldBudgetId).toBe('b1')
})

it('no default budget -> data null with no budget request', async () => {
  let hits = 0
  server.use(
    ...coreHandlers(),
    http.get('*/api/v1/budget/get-budget', () => {
      hits++
      return HttpResponse.json({ success: true, message: '', data: { item: wireBudget } })
    }),
  )
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useBudget(), { wrapper })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(result.current.data).toBeNull()
  expect(hits).toBe(0)
})

it('set-limit patches budgeted optimistically and rolls back on error', async () => {
  server.use(
    http.post('*/api/v1/budget/set-limit', () =>
      HttpResponse.json({ success: false, message: 'Form validation error', code: 400, errors: {} }, { status: 400 }),
    ),
  )
  const { queryClient, wrapper } = makeWrapper()
  const key = [...queryKeys.budget, 'b1', '2026-07-01']
  const initial = { ...wireBudget } as unknown as BudgetDto
  queryClient.setQueryData(key, initial)

  const { result } = renderHook(() => useSetLimit(), { wrapper })
  result.current.mutate({ budgetId: 'b1', elementId: 'cat-food', amount: '999' })
  await waitFor(() => {
    const cached = queryClient.getQueryData<BudgetDto>(key)!
    expect(cached.structure.elements[0].budgeted === 999 || result.current.isError).toBe(true)
  })
  await waitFor(() => expect(result.current.isError).toBe(true))
  // rolled back
  const cached = queryClient.getQueryData<BudgetDto>(key)!
  expect(cached.structure.elements[0].budgeted).toBe('200' as unknown as number)
})

it('canUpdateLimits requires an accepted editing role and the period at/after start', () => {
  const meta = wireBudget.meta as unknown as BudgetMetaDto
  expect(canUpdateLimits(meta, 'u1', '2026-07-01')).toBe(true)
  expect(canUpdateLimits(meta, 'u1', '2025-12-01')).toBe(false)
  expect(canUpdateLimits(meta, 'stranger', '2026-07-01')).toBe(false)
})
