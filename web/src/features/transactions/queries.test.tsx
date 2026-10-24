import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { queryKeys } from '@/app/queryKeys'
import { useCreateTransaction, useDeleteTransaction, useMoveTransaction } from './queries'

const wireOwner = { id: 'u1', avatar: '', name: 'Ada' }
const wireTx = {
  id: 't-created', author: wireOwner, type: 'expense', accountId: 'a1', accountRecipientId: null,
  amount: '9.99', amountRecipient: '9.99', categoryId: 'cat1', description: '', payeeId: null, tagId: null,
  date: '2026-07-01 09:30:00',
}
const wireAccount = {
  id: 'a1', owner: wireOwner, folderId: 'f1', name: 'Cash', position: 0,
  currency: { id: 'c1', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 },
  balance: '90.01', type: 1, icon: 'wallet', sharedAccess: [],
}

function makeWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  return { queryClient, wrapper }
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('create-transaction replaces the accounts cache and prepends the item', async () => {
  server.use(
    http.post('*/api/v1/transaction/create-transaction', () =>
      HttpResponse.json({ success: true, message: '', data: { item: wireTx, accounts: [wireAccount] } }),
    ),
  )
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData(queryKeys.transactions, [{ ...wireTx, id: 't-existing', amount: '1' }])
  queryClient.setQueryData(queryKeys.accounts, [])

  const { result } = renderHook(() => useCreateTransaction(), { wrapper })
  result.current.mutate({
    id: 'op1', type: 'expense', accountId: 'a1', accountRecipientId: null, amount: '9.99',
    amountRecipient: null, categoryId: 'cat1', description: '', payeeId: null, tagId: null, labelIds: [], date: '2026-07-01 09:30:00',
  })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))

  const accounts = queryClient.getQueryData<{ balance: string }[]>(queryKeys.accounts)!
  expect(accounts[0].balance).toBe('90.01')
  const txs = queryClient.getQueryData<{ id: string }[]>(queryKeys.transactions)!
  expect(txs.map((t) => t.id)).toEqual(['t-created', 't-existing'])
})

it('create-transaction invalidates the budget and budget plan caches', async () => {
  server.use(
    http.post('*/api/v1/transaction/create-transaction', () =>
      HttpResponse.json({ success: true, message: '', data: { item: wireTx, accounts: [wireAccount] } }),
    ),
  )
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData(queryKeys.transactions, [])
  queryClient.setQueryData(queryKeys.accounts, [])
  queryClient.setQueryData([...queryKeys.budget, 'b1', '2026-09-01'], null)
  queryClient.setQueryData([...queryKeys.budgetPlan, 'b1', '2026-07-01', 6], null)

  const { result } = renderHook(() => useCreateTransaction(), { wrapper })
  result.current.mutate({
    id: 'op1', type: 'expense', accountId: 'a1', accountRecipientId: null, amount: '9.99',
    amountRecipient: null, categoryId: 'cat1', description: '', payeeId: null, tagId: null, labelIds: [], date: '2026-07-01 09:30:00',
  })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))

  expect(queryClient.getQueryState([...queryKeys.budget, 'b1', '2026-09-01'])!.isInvalidated).toBe(true)
  expect(queryClient.getQueryState([...queryKeys.budgetPlan, 'b1', '2026-07-01', 6])!.isInvalidated).toBe(true)
})

it('delete-transaction removes the item and refreshes accounts', async () => {
  server.use(
    http.post('*/api/v1/transaction/delete-transaction', () =>
      HttpResponse.json({ success: true, message: '', data: { item: wireTx, accounts: [wireAccount] } }),
    ),
  )
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData(queryKeys.transactions, [wireTx, { ...wireTx, id: 't-other' }])

  const { result } = renderHook(() => useDeleteTransaction(), { wrapper })
  result.current.mutate('t-created')
  await waitFor(() => expect(result.current.isSuccess).toBe(true))

  const txs = queryClient.getQueryData<{ id: string }[]>(queryKeys.transactions)!
  expect(txs.map((t) => t.id)).toEqual(['t-other'])
  expect(queryClient.getQueryData<{ balance: string }[]>(queryKeys.accounts)![0].balance).toBe('90.01')
})

describe('useMoveTransaction', () => {
  const day = [
    { ...wireTx, id: 'm1', date: '2024-03-01 09:00:00', sortKey: 2, recipientSortKey: null },
    { ...wireTx, id: 'm2', date: '2024-03-01 10:00:00', sortKey: 1, recipientSortKey: null },
  ]
  const req = { id: 'm2', accountId: 'a1', date: '2024-03-01', transactionIds: ['m2', 'm1'] }
  const keysOf = (rows: { id: string; sortKey?: number }[] | undefined) => Object.fromEntries((rows ?? []).map((r) => [r.id, r.sortKey]))

  it('shows the new order at once and keeps the server rows', async () => {
    let release: () => void = () => {}
    const gate = new Promise<void>((resolve) => { release = resolve })
    server.use(
      http.post('*/api/v1/transaction/move-transaction', async () => {
        await gate
        return HttpResponse.json({ success: true, message: '', data: { items: [{ ...day[1], sortKey: 2 }, { ...day[0], sortKey: 1 }], accounts: [wireAccount] } })
      }),
    )
    const { queryClient, wrapper } = makeWrapper()
    queryClient.setQueryData(queryKeys.transactions, day)
    const { result } = renderHook(() => useMoveTransaction(), { wrapper })
    result.current.mutate(req)
    await waitFor(() => expect(keysOf(queryClient.getQueryData(queryKeys.transactions))).toEqual({ m1: 1, m2: 2 }))
    release()
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(keysOf(queryClient.getQueryData(queryKeys.transactions))).toEqual({ m1: 1, m2: 2 })
    expect(queryClient.getQueryData(queryKeys.accounts)).toEqual([wireAccount])
  })

  it('puts the old order back when the server refuses, keeps rows that arrived meanwhile and refreshes', async () => {
    let release: () => void = () => {}
    const gate = new Promise<void>((resolve) => { release = resolve })
    server.use(
      http.post('*/api/v1/transaction/move-transaction', async () => {
        await gate
        return HttpResponse.json({ success: false, message: 'This day changed meanwhile. Refresh and try again.', code: 400, errors: {} }, { status: 400 })
      }),
    )
    const { queryClient, wrapper } = makeWrapper()
    queryClient.setQueryData(queryKeys.transactions, day)
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
    const { result } = renderHook(() => useMoveTransaction(), { wrapper })
    result.current.mutate(req)
    await waitFor(() => expect(keysOf(queryClient.getQueryData(queryKeys.transactions))).toEqual({ m1: 1, m2: 2 }))
    queryClient.setQueryData<{ id: string }[]>(queryKeys.transactions, (rows) => [...(rows ?? []), { ...day[0], id: 'arrived', sortKey: 3 }])
    release()
    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(keysOf(queryClient.getQueryData(queryKeys.transactions))).toEqual({ m1: 2, m2: 1, arrived: 3 })
    expect(invalidate).toHaveBeenCalledWith({ queryKey: queryKeys.transactions })
  })
})
