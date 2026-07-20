import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { queryKeys } from '@/app/queryKeys'
import { useAccountTransactionPager, useCreateTransaction, useDeleteTransaction } from './queries'

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
  queryClient.setQueryData(queryKeys.transactions, [{ ...wireTx, id: 't-existing', amount: 1 }])
  queryClient.setQueryData(queryKeys.accounts, [])

  const { result } = renderHook(() => useCreateTransaction(), { wrapper })
  result.current.mutate({
    id: 'op1', type: 'expense', accountId: 'a1', accountRecipientId: null, amount: 9.99,
    amountRecipient: null, categoryId: 'cat1', description: '', payeeId: null, tagId: null, date: '2026-07-01 09:30:00',
  })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))

  const accounts = queryClient.getQueryData<{ balance: number }[]>(queryKeys.accounts)!
  expect(accounts[0].balance).toBe(90.01)
  const txs = queryClient.getQueryData<{ id: string }[]>(queryKeys.transactions)!
  expect(txs.map((t) => t.id)).toEqual(['t-created', 't-existing'])
})

const wireAccountKnown = { ...wireAccount, id: 'known' }

describe('useAccountTransactionPager ensure-window gating', () => {
  it('does not fetch for an accountId absent from the accounts cache (pre-access-check guard)', async () => {
    let calls = 0
    server.use(
      http.get('*/api/v1/transaction/get-transaction-list', () => {
        calls++
        return HttpResponse.json({ success: true, message: '', data: { items: [] } })
      }),
    )
    const { queryClient, wrapper } = makeWrapper()
    queryClient.setQueryData(queryKeys.accounts, [wireAccountKnown])
    queryClient.setQueryData(queryKeys.transactionPages, {})

    renderHook(() => useAccountTransactionPager('unknown-foreign-account'), { wrapper })

    // give any (incorrect) effect-driven fetch a chance to fire
    await new Promise((resolve) => setTimeout(resolve, 10))
    expect(calls).toBe(0)
  })

  it('fetches the first page for a known account missing from the pages map', async () => {
    let calls = 0
    server.use(
      http.get('*/api/v1/transaction/get-transaction-list', () => {
        calls++
        return HttpResponse.json({ success: true, message: '', data: { items: [], page: { nextCursor: null, hasMore: false } } })
      }),
    )
    const { queryClient, wrapper } = makeWrapper()
    // a hidden-folder account: known to the client but absent from the boot pages map
    queryClient.setQueryData(queryKeys.accounts, [wireAccountKnown])
    queryClient.setQueryData(queryKeys.transactionPages, {})

    renderHook(() => useAccountTransactionPager('known'), { wrapper })

    await waitFor(() => expect(calls).toBe(1))
  })
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
  expect(queryClient.getQueryData<{ balance: number }[]>(queryKeys.accounts)![0].balance).toBe(90.01)
})
