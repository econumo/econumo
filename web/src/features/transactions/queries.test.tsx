import { act, renderHook, waitFor } from '@testing-library/react'
import { onlineManager, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { queryKeys } from '@/app/queryKeys'
import { coreHandlers } from '@/test/fixtures'
import { useAccounts } from '@/features/accounts/queries'
import { getOps, loadPendingWrites } from '@/features/pendingWrites/store'
import type { CreateTransactionDto } from '@/api/dto/transaction'
import { useCreateTransaction, useDeleteTransaction, useMoveTransaction, useTransactions, useUpdateTransaction } from './queries'

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
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false, networkMode: 'always' }, queries: { retry: false, networkMode: 'always' } } })
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
  queryClient.setQueryData(queryKeys.debts, [])
  queryClient.setQueryData(queryKeys.debt('d1'), null)

  const { result } = renderHook(() => useCreateTransaction(), { wrapper })
  result.current.mutate({
    id: 'op1', type: 'expense', accountId: 'a1', accountRecipientId: null, amount: '9.99',
    amountRecipient: null, categoryId: 'cat1', description: '', payeeId: null, tagId: null, labelIds: [], date: '2026-07-01 09:30:00',
  })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))

  expect(queryClient.getQueryState([...queryKeys.budget, 'b1', '2026-09-01'])!.isInvalidated).toBe(true)
  expect(queryClient.getQueryState([...queryKeys.budgetPlan, 'b1', '2026-07-01', 6])!.isInvalidated).toBe(true)
  // a payment moves its debt's balance, and Edit pre-fills from it
  expect(queryClient.getQueryState(queryKeys.debts)!.isInvalidated).toBe(true)
  expect(queryClient.getQueryState(queryKeys.debt('d1'))!.isInvalidated).toBe(true)
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
    { ...wireTx, id: 'm1', date: '2024-03-01 09:00:00', sortKey: 'a2', recipientSortKey: null },
    { ...wireTx, id: 'm2', date: '2024-03-01 10:00:00', sortKey: 'a1', recipientSortKey: null },
  ]
  const req = { id: 'm2', accountId: 'a1', fromDate: '2024-03-01', date: '2024-03-01', aboveId: null }
  const keysOf = (rows: { id: string; sortKey?: string }[] | undefined) => Object.fromEntries((rows ?? []).map((r) => [r.id, r.sortKey]))

  it('takes the server rows on success and leaves the others alone', async () => {
    server.use(
      http.post('*/api/v1/transaction/move-transaction', () =>
        HttpResponse.json({ success: true, message: '', data: { items: [{ ...day[1], sortKey: 'a3' }], accounts: [wireAccount] } }),
      ),
    )
    const { queryClient, wrapper } = makeWrapper()
    queryClient.setQueryData(queryKeys.transactions, day)
    const { result } = renderHook(() => useMoveTransaction(), { wrapper })
    result.current.mutate(req)
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(keysOf(queryClient.getQueryData(queryKeys.transactions))).toEqual({ m1: 'a2', m2: 'a3' })
    expect(queryClient.getQueryData(queryKeys.accounts)).toEqual([wireAccount])
  })

  it('leaves the cache untouched and syncs again when the server refuses', async () => {
    server.use(
      http.post('*/api/v1/transaction/move-transaction', () =>
        HttpResponse.json({ success: false, message: 'This day changed meanwhile. Refresh and try again.', code: 400, errors: {} }, { status: 400 }),
      ),
    )
    const { queryClient, wrapper } = makeWrapper()
    queryClient.setQueryData(queryKeys.transactions, day)
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
    const { result } = renderHook(() => useMoveTransaction(), { wrapper })
    result.current.mutate(req)
    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(keysOf(queryClient.getQueryData(queryKeys.transactions))).toEqual({ m1: 'a2', m2: 'a1' })
    expect(invalidate).toHaveBeenCalledWith({ queryKey: queryKeys.transactions })
  })
})

describe('offline writes', () => {
  const form: CreateTransactionDto = {
    id: 'temp-1', type: 'expense', accountId: 'a1', accountRecipientId: null, amount: '10',
    amountRecipient: null, categoryId: 'cat-food', description: 'lunch', payeeId: null, tagId: null,
    labelIds: [], date: '2026-07-01 09:30:00',
  }
  let requests: string[]

  beforeEach(async () => {
    requests = []
    server.use(...coreHandlers())
    server.events.on('request:start', ({ request }) => {
      if (request.method === 'POST') {
        requests.push(new URL(request.url).pathname)
      }
    })
    onlineManager.setOnline(false)
    await loadPendingWrites('u1')
  })

  afterEach(() => {
    server.events.removeAllListeners()
    onlineManager.setOnline(true)
  })

  function mount() {
    const { queryClient, wrapper } = makeWrapper()
    const hooks = renderHook(
      () => ({ txs: useTransactions(), accounts: useAccounts(), create: useCreateTransaction(), update: useUpdateTransaction(), del: useDeleteTransaction() }),
      { wrapper },
    )
    return { queryClient, ...hooks }
  }

  it('an offline add sends nothing, shows the row and moves the balance', async () => {
    const { result } = mount()
    await waitFor(() => expect(result.current.txs.data).toBeDefined())
    await waitFor(() => expect(result.current.accounts.data).toBeDefined())
    const before = result.current.accounts.data!.find((a) => a.id === 'a1')!.balance

    await act(() => result.current.create.mutateAsync(form))

    expect(requests).not.toContain('/api/v1/transaction/create-transaction')
    expect(result.current.txs.data!.find((t) => t.id === 'temp-1')).toMatchObject({ amount: '10', description: 'lunch' })
    await waitFor(() => expect(result.current.accounts.data!.find((a) => a.id === 'a1')!.balance).toBe(String(Number(before) - 10)))
  })

  it('a sync landing server rows keeps the waiting row', async () => {
    const { result, queryClient } = mount()
    await waitFor(() => expect(result.current.txs.data).toBeDefined())
    await act(() => result.current.create.mutateAsync(form))

    act(() => {
      queryClient.setQueryData(queryKeys.transactions, [{ ...wireTx, id: 't-from-sync' }])
    })

    await waitFor(() => expect(result.current.txs.data!.map((t) => t.id).sort()).toEqual(['t-from-sync', 'temp-1']))
  })

  it('editing a waiting row offline leaves one op carrying the last values', async () => {
    const { result } = mount()
    await waitFor(() => expect(result.current.txs.data).toBeDefined())
    await act(() => result.current.create.mutateAsync(form))
    await act(() => result.current.update.mutateAsync({ ...form, amount: '12' }))

    expect(getOps()).toHaveLength(1)
    expect(getOps()[0]).toMatchObject({ kind: 'tx-create', payload: { amount: '12' } })
    expect(result.current.txs.data!.find((t) => t.id === 'temp-1')!.amount).toBe('12')
  })

  it('an offline delete drops the row from the list and remembers the previous row', async () => {
    const { result, queryClient } = mount()
    await waitFor(() => expect(result.current.txs.data).toBeDefined())
    const target = queryClient.getQueryData<{ id: string }[]>(queryKeys.transactions)![0]

    await act(() => result.current.del.mutateAsync(target.id))

    expect(result.current.txs.data!.some((t) => t.id === target.id)).toBe(false)
    expect(getOps()[0]).toMatchObject({ kind: 'tx-delete', txId: target.id, previous: { id: target.id } })
  })
})
