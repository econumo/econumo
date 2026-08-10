import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { queryKeys } from '@/app/queryKeys'
import type { RecurringDto } from '@/api/dto/recurring'
import { useCreateRecurring, useDeleteRecurring, usePostRecurring, useRecurring, useSkipRecurring } from './queries'

const wireRecurring = {
  id: 'r1', ownerUserId: 'u1', type: 'expense', accountId: 'a1', accountRecipientId: null,
  amount: '50.5', categoryId: 'c1', payeeId: null, tagId: null, description: 'rent',
  schedule: 'monthly', nextPaymentAt: '2026-08-31 00:00:00',
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

it('useRecurring fetches the list with decimal-string amounts', async () => {
  server.use(http.get('*/api/v1/recurring/get-recurring-transaction-list', () =>
    HttpResponse.json({ success: true, message: '', data: { items: [wireRecurring] } })))
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useRecurring(), { wrapper })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(result.current.data![0].amount).toBe('50.5')
})

it('useSkipRecurring updates the cached template', async () => {
  server.use(http.post('*/api/v1/recurring/skip-recurring-transaction', () =>
    HttpResponse.json({ success: true, message: '', data: { item: { ...wireRecurring, nextPaymentAt: '2026-09-30 00:00:00' } } })))
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData<RecurringDto[]>(queryKeys.recurring, [{ ...wireRecurring, amount: '50.5' } as RecurringDto])
  const { result } = renderHook(() => useSkipRecurring(), { wrapper })
  result.current.mutate('r1')
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  const cached = queryClient.getQueryData<RecurringDto[]>(queryKeys.recurring)
  expect(cached![0].nextPaymentAt).toBe('2026-09-30 00:00:00')
})

it('usePostRecurring prepends the transaction, replaces accounts, advances the template', async () => {
  const wireTx = {
    id: 't1', author: { id: 'u1', name: 'U', avatar: 'face:fuchsia' }, type: 'expense',
    accountId: 'a1', accountRecipientId: null, amount: '50.5', amountRecipient: null,
    categoryId: 'c1', description: 'rent', payeeId: null, tagId: null, date: '2026-08-31 00:00:00',
  }
  server.use(http.post('*/api/v1/recurring/post-recurring-transaction', () =>
    HttpResponse.json({ success: true, message: '', data: { item: wireTx, accounts: [], nextPaymentAt: '2026-09-30 00:00:00' } })))
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData<RecurringDto[]>(queryKeys.recurring, [{ ...wireRecurring, amount: '50.5' } as RecurringDto])
  queryClient.setQueryData(queryKeys.transactions, [])
  const { result } = renderHook(() => usePostRecurring(), { wrapper })
  result.current.mutate({ recurringId: 'r1', id: 'op1', type: 'expense', accountId: 'a1', accountRecipientId: null, amount: '50.5', amountRecipient: null, categoryId: 'c1', description: 'rent', payeeId: null, tagId: null, labelIds: [], date: '2026-08-31 00:00:00' })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect((queryClient.getQueryData(queryKeys.transactions) as unknown[]).length).toBe(1)
  expect(queryClient.getQueryData<RecurringDto[]>(queryKeys.recurring)![0].nextPaymentAt).toBe('2026-09-30 00:00:00')
})

it('useCreateRecurring from a transaction links the cached source to the new template', async () => {
  server.use(http.post('*/api/v1/recurring/create-recurring-transaction', () =>
    HttpResponse.json({ success: true, message: '', data: { item: wireRecurring } })))
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData(queryKeys.transactions, [
    { id: 't-source', recurringId: null },
    { id: 't-other', recurringId: null },
  ])
  const { result } = renderHook(() => useCreateRecurring(), { wrapper })
  result.current.mutate({
    id: 'op1', type: 'expense', accountId: 'a1', accountRecipientId: null, amount: '50.5',
    categoryId: 'c1', payeeId: null, tagId: null, labelIds: [], description: 'rent',
    schedule: 'monthly', nextPaymentAt: '2026-08-31 00:00:00', sourceTransactionId: 't-source',
  })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))

  expect(queryClient.getQueryData<RecurringDto[]>(queryKeys.recurring)![0].id).toBe('r1')
  // the backend linked the source in the same write; the cache mirrors it
  const txs = queryClient.getQueryData<{ id: string; recurringId: string | null }[]>(queryKeys.transactions)!
  expect(txs.find((tx) => tx.id === 't-source')?.recurringId).toBe('r1')
  expect(txs.find((tx) => tx.id === 't-other')?.recurringId).toBeNull()
})

it('useDeleteRecurring drops the template and severs cached transaction links', async () => {
  server.use(http.post('*/api/v1/recurring/delete-recurring-transaction', () =>
    HttpResponse.json({ success: true, message: '', data: {} })))
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData<RecurringDto[]>(queryKeys.recurring, [wireRecurring as RecurringDto])
  // one posted from r1, one posted from another template, one plain
  queryClient.setQueryData(queryKeys.transactions, [
    { id: 't-from-r1', recurringId: 'r1' },
    { id: 't-from-other', recurringId: 'r2' },
    { id: 't-plain', recurringId: null },
  ])
  const { result } = renderHook(() => useDeleteRecurring(), { wrapper })
  result.current.mutate('r1')
  await waitFor(() => expect(result.current.isSuccess).toBe(true))

  expect(queryClient.getQueryData<RecurringDto[]>(queryKeys.recurring)).toEqual([])
  // mirrors the backend's ON DELETE SET NULL — only r1's transactions lose the link
  const txs = queryClient.getQueryData<{ id: string; recurringId: string | null }[]>(queryKeys.transactions)!
  expect(txs.find((tx) => tx.id === 't-from-r1')?.recurringId).toBeNull()
  expect(txs.find((tx) => tx.id === 't-from-other')?.recurringId).toBe('r2')
  expect(txs.find((tx) => tx.id === 't-plain')?.recurringId).toBeNull()
})
