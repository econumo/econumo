import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { queryKeys } from '@/app/queryKeys'
import { METRICS, trackEvent } from '@/lib/metrics'
import {
  useCreateDebt,
  useDebt,
  useDebts,
  useDeleteDebt,
  useMarkDebtPaidOff,
  useUpdateDebt,
} from './queries'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})

const wireDebt = {
  id: 'd1', name: 'Car loan', icon: 'directions_car', currencyId: 'cur-usd', categoryId: 'c1',
  payeeId: null, tagId: null, startDate: '', balance: '100', payment: '105', schedule: 'monthly',
  lastPaymentDate: '2027-06-20', currentBalance: '100', paymentsLeft: 1, moneyLeft: '105',
  overpaymentLeft: '5', overpaidSoFar: '0', payoffDate: '2027-06-20', nextDueDate: '2027-06-20',
  periodRate: '0.0500000000', isPaidOff: 0, isSetUp: 1, periodsPaid: 0,
}
const ok = (data: unknown) => HttpResponse.json({ success: true, message: '', data })

const form = {
  name: 'Car loan', currencyId: 'cur-usd', categoryId: 'c1', balance: '100', payment: '105',
  schedule: 'monthly' as const, lastPaymentDate: '2027-06-20',
}

function makeWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  const invalidated = vi.spyOn(queryClient, 'invalidateQueries')
  return { queryClient, wrapper, invalidated }
}

function expectInvalidated(spy: ReturnType<typeof makeWrapper>['invalidated']) {
  const keys = spy.mock.calls.map((c) => JSON.stringify((c[0] as { queryKey: unknown }).queryKey))
  // a whole-category debt write moves comments between the category and debt rows
  for (const key of [queryKeys.debts, queryKeys.budget, queryKeys.budgetPlan, queryKeys.budgetTransactions, queryKeys.budgetComments]) {
    expect(keys).toContain(JSON.stringify(key))
  }
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  vi.mocked(trackEvent).mockClear()
})

it('useDebts fetches the list and useDebt the details', async () => {
  server.use(
    http.get('*/api/v1/debt/get-debt-list', () => ok({ items: [wireDebt] })),
    http.get('*/api/v1/debt/get-debt', ({ request }) => {
      expect(new URL(request.url).searchParams.get('id')).toBe('d1')
      return ok({ item: wireDebt, snapshots: [], payments: [] })
    }),
  )
  const { wrapper } = makeWrapper()
  const list = renderHook(() => useDebts(), { wrapper })
  await waitFor(() => expect(list.result.current.isSuccess).toBe(true))
  expect(list.result.current.data![0].periodRate).toBe('0.0500000000')
  const one = renderHook(() => useDebt('d1'), { wrapper })
  await waitFor(() => expect(one.result.current.isSuccess).toBe(true))
  expect(one.result.current.data!.item.id).toBe('d1')
  expect(one.result.current.data!.payments).toEqual([])
})

it('useCreateDebt fires DEBT_CREATE and invalidates debts, budget and budgetPlan', async () => {
  let body: unknown
  server.use(http.post('*/api/v1/debt/create-debt', async ({ request }) => {
    body = await request.json()
    return ok(wireDebt)
  }))
  const { wrapper, invalidated } = makeWrapper()
  const { result } = renderHook(() => useCreateDebt(), { wrapper })
  result.current.mutate(form)
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(body).toEqual(form)
  expect(result.current.data!.id).toBe('d1')
  expect(trackEvent).toHaveBeenCalledWith(METRICS.DEBT_CREATE)
  expect(trackEvent).not.toHaveBeenCalledWith(METRICS.DEBT_CONVERT)
  expectInvalidated(invalidated)
})

it('useCreateDebt({fromBudget}) fires DEBT_CONVERT instead', async () => {
  server.use(http.post('*/api/v1/debt/create-debt', () => ok(wireDebt)))
  const { wrapper, invalidated } = makeWrapper()
  const { result } = renderHook(() => useCreateDebt({ fromBudget: true }), { wrapper })
  result.current.mutate(form)
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(trackEvent).toHaveBeenCalledWith(METRICS.DEBT_CONVERT)
  expect(trackEvent).not.toHaveBeenCalledWith(METRICS.DEBT_CREATE)
  expectInvalidated(invalidated)
})

it('useUpdateDebt fires DEBT_UPDATE and invalidates', async () => {
  server.use(http.post('*/api/v1/debt/update-debt', () => ok(wireDebt)))
  const { wrapper, invalidated } = makeWrapper()
  const { result } = renderHook(() => useUpdateDebt(), { wrapper })
  result.current.mutate({ ...form, id: 'd1' })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(trackEvent).toHaveBeenCalledWith(METRICS.DEBT_UPDATE)
  expectInvalidated(invalidated)
})

it('useUpdateDebt finishing setup fires DEBT_FINISH_SETUP instead, once the debt comes back set up', async () => {
  server.use(http.post('*/api/v1/debt/update-debt', () => ok(wireDebt)))
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useUpdateDebt({ finishingSetup: true }), { wrapper })
  result.current.mutate({ ...form, id: 'd1' })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(trackEvent).toHaveBeenCalledWith(METRICS.DEBT_FINISH_SETUP)
  expect(trackEvent).not.toHaveBeenCalledWith(METRICS.DEBT_UPDATE)
})

it('useUpdateDebt finishing setup without terms is a plain update', async () => {
  server.use(http.post('*/api/v1/debt/update-debt', () => ok({ ...wireDebt, isSetUp: 0 })))
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useUpdateDebt({ finishingSetup: true }), { wrapper })
  result.current.mutate({ name: 'Car loan', currencyId: 'cur-usd', categoryId: 'c1', payment: '105', schedule: 'monthly', id: 'd1' })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(trackEvent).toHaveBeenCalledWith(METRICS.DEBT_UPDATE)
  expect(trackEvent).not.toHaveBeenCalledWith(METRICS.DEBT_FINISH_SETUP)
})

it('useDeleteDebt sends confirmDataRemoval, fires DEBT_DELETE and invalidates', async () => {
  let body: unknown
  server.use(http.post('*/api/v1/debt/delete-debt', async ({ request }) => {
    body = await request.json()
    return ok({})
  }))
  const { wrapper, invalidated } = makeWrapper()
  const { result } = renderHook(() => useDeleteDebt(), { wrapper })
  result.current.mutate({ id: 'd1', confirmDataRemoval: true })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(body).toEqual({ id: 'd1', confirmDataRemoval: true })
  expect(trackEvent).toHaveBeenCalledWith(METRICS.DEBT_DELETE)
  expectInvalidated(invalidated)
})

it('useMarkDebtPaidOff fires DEBT_MARK_PAID_OFF and invalidates', async () => {
  server.use(http.post('*/api/v1/debt/mark-debt-paid-off', () => ok({ ...wireDebt, isPaidOff: 1 })))
  const { wrapper, invalidated } = makeWrapper()
  const { result } = renderHook(() => useMarkDebtPaidOff(), { wrapper })
  result.current.mutate('d1')
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(result.current.data!.isPaidOff).toBe(1)
  expect(trackEvent).toHaveBeenCalledWith(METRICS.DEBT_MARK_PAID_OFF)
  expectInvalidated(invalidated)
})

it('a failed mutation fires nothing', async () => {
  server.use(http.post('*/api/v1/debt/create-debt', () =>
    HttpResponse.json({ success: false, message: 'no', code: 400, errors: {} }, { status: 400 })))
  const { wrapper, invalidated } = makeWrapper()
  const { result } = renderHook(() => useCreateDebt(), { wrapper })
  result.current.mutate(form)
  await waitFor(() => expect(result.current.isError).toBe(true))
  expect(trackEvent).not.toHaveBeenCalled()
  expect(invalidated).not.toHaveBeenCalled()
})
