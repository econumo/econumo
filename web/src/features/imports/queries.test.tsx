import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { queryKeys } from '@/app/queryKeys'
import type { ImportQueueDto, ImportSourceDto } from '@/api/dto/imports'
import type { TransactionDto } from '@/api/dto/transaction'
import { useImportQueuedEvent, useImportSources, useLinkImportAccount, useSkipQueuedEvent } from './queries'

const card = { externalAccountId: 'wallet', externalName: 'Apple Card', externalCurrency: 'USD', state: 'unmapped' as const, accountId: '', queuedCount: 1, tapCount: 1, lastSeenAt: '2026-08-20 17:42:03' }
const wireSource: ImportSourceDto = { id: 's1', provider: 'apple-wallet', name: 'iPhone', status: 'active', createdAt: '2026-08-01 00:00:00', cards: [card] }
const queued = { linkId: 'l1', sourceId: 's1', externalAccountId: 'wallet', accountId: '', payee: 'Blue Bottle', amount: '4.75', currency: 'USD', type: 'expense' as const, postedAt: '2026-08-20 17:42:03', reason: 'unmapped' as const }
const wireQueue: ImportQueueDto = { queued: [queued], skipped: [], failed: [] }

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

it('useImportSources fetches the source list with its cards', async () => {
  server.use(http.get('*/api/v1/import/get-source-list', () =>
    HttpResponse.json({ success: true, message: '', data: { items: [wireSource] } })))
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useImportSources(), { wrapper })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(result.current.data![0].cards[0].state).toBe('unmapped')
})

it('useLinkImportAccount replaces the source in the cache and refetches the ledger when a run happened', async () => {
  const mapped = { ...wireSource, cards: [{ ...card, state: 'mapped', accountId: 'a1', queuedCount: 0 }] }
  server.use(http.post('*/api/v1/import/link-account', () =>
    HttpResponse.json({ success: true, message: '', data: { item: mapped, run: { id: 'r1', status: 'finished', importedCount: 1, matchedCount: 0, skippedCount: 0, failedCount: 0 } } })))
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData<ImportSourceDto[]>(queryKeys.importSources, [wireSource])
  queryClient.setQueryData<TransactionDto[]>(queryKeys.transactions, [])
  const { result } = renderHook(() => useLinkImportAccount(), { wrapper })
  result.current.mutate({ sourceId: 's1', externalAccountId: 'wallet', accountId: 'a1' })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(queryClient.getQueryData<ImportSourceDto[]>(queryKeys.importSources)![0].cards[0].state).toBe('mapped')
  expect(queryClient.getQueryState(queryKeys.transactions)?.isInvalidated).toBe(true)
})

it('useImportQueuedEvent prepends the transaction and drops the queue row', async () => {
  const wireTx = {
    id: 't1', author: { id: 'u1', name: 'U', avatar: 'face:fuchsia' }, type: 'expense',
    accountId: 'a1', accountRecipientId: null, amount: '4.75', amountRecipient: null,
    categoryId: 'c1', description: 'Blue Bottle', payeeId: null, tagId: null, labelIds: [], recurringId: null, isImported: 1,
    date: '2026-08-20 17:42:03',
  }
  server.use(http.post('*/api/v1/import/import-queued-event', () =>
    HttpResponse.json({ success: true, message: '', data: { item: wireTx, accounts: [] } })))
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData<ImportQueueDto>(queryKeys.importQueue, wireQueue)
  queryClient.setQueryData<TransactionDto[]>(queryKeys.transactions, [])
  const { result } = renderHook(() => useImportQueuedEvent(), { wrapper })
  result.current.mutate({
    linkId: 'l1',
    transaction: { id: 't1', type: 'expense', accountId: 'a1', accountRecipientId: null, amount: '4.75', amountRecipient: null, categoryId: 'c1', description: 'Blue Bottle', payeeId: null, tagId: null, labelIds: [], date: '2026-08-20 17:42:03' },
  })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(queryClient.getQueryData<TransactionDto[]>(queryKeys.transactions)![0].id).toBe('t1')
  expect(queryClient.getQueryData<ImportQueueDto>(queryKeys.importQueue)!.queued).toHaveLength(0)
})

it('useSkipQueuedEvent replaces the whole queue from the response', async () => {
  server.use(http.post('*/api/v1/import/skip-queued-event', () =>
    HttpResponse.json({ success: true, message: '', data: { queued: [], skipped: [queued], failed: [] } })))
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData<ImportQueueDto>(queryKeys.importQueue, wireQueue)
  const { result } = renderHook(() => useSkipQueuedEvent(), { wrapper })
  result.current.mutate('l1')
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(queryClient.getQueryData<ImportQueueDto>(queryKeys.importQueue)!.skipped).toHaveLength(1)
})
