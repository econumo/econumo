import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import type { ImportQueueDto, ImportRuleDto, ImportSourceDto } from '@/api/dto/imports'
import type { TransactionDto } from '@/api/dto/transaction'
import { METRICS, trackEvent } from '@/lib/metrics'
import {
  useApplyImportRule,
  useCreateImportRule,
  useImportCredentialKey,
  useImportQueuedEvent,
  useImportRules,
  useImportSources,
  useLinkImportAccount,
  useSkipQueuedEvent,
  useSuggestImportRules,
  useSyncImportSource,
} from './queries'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})
const trackEventMock = vi.mocked(trackEvent)

const card = { externalAccountId: 'wallet', externalName: 'Apple Card', externalCurrency: 'USD', state: 'unmapped' as const, accountId: '', queuedCount: 1, tapCount: 1, lastSeenAt: '2026-08-20 17:42:03' }
const wireSource: ImportSourceDto = { id: 's1', provider: 'apple-wallet', name: 'iPhone', status: 'active', createdAt: '2026-08-01 00:00:00', lastSyncedAt: '', credentialCiphertext: '', cards: [card] }
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
  server.use(...coreHandlers())
  trackEventMock.mockClear()
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

it('useSyncImportSource refreshes ledger caches only when the run wrote something, and fires IMPORT_SYNC', async () => {
  const run = {
    id: 'r1', sourceId: 's2', provider: 'simplefin', status: 'completed', trigger: 'manual',
    importedCount: 2, matchedCount: 1, amountsUpdatedCount: 0, queuedCount: 0, skippedCount: 0, failedCount: 0,
    errors: [], startedAt: '2026-09-07 10:00:00', finishedAt: '2026-09-07 10:00:02',
  }
  let body: unknown
  server.use(http.post('*/api/v1/import/sync-source', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ success: true, message: '', data: { run, accounts: [] } })
  }))
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData(queryKeys.transactions, { stale: true })
  const { result } = renderHook(() => useSyncImportSource(), { wrapper })
  await act(() => result.current.mutateAsync({ sourceId: 's2', accessUrl: 'https://u:p@b/x', startDate: '2026-08-01' }))
  expect(body).toEqual({ sourceId: 's2', accessUrl: 'https://u:p@b/x', startDate: '2026-08-01' })
  expect(queryClient.getQueryState(queryKeys.transactions)?.isInvalidated).toBe(true)
  expect(trackEventMock).toHaveBeenCalledWith(METRICS.IMPORT_SYNC, { trigger: 'manual', imported: 2, matched: 1 })
})

it('getImportCredentialKey maps the empty no-key-yet payload to null', async () => {
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useImportCredentialKey(), { wrapper })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(result.current.data).toBeNull()
})

const wireRule: ImportRuleDto = {
  id: 'rule1', sourceId: '', action: 'classify', matchField: 'external_payee', matchType: 'contains', matchValue: 'BLUE BOTTLE',
  isCaseSensitive: false, categoryId: 'c1', payeeId: '', tagId: '', labelIds: [], priority: 0,
  createdAt: '2026-09-08 10:00:00', updatedAt: '2026-09-08 10:00:00',
}

it('useImportRules fetches the rule list', async () => {
  server.use(http.get('*/api/v1/import/get-rule-list', () =>
    HttpResponse.json({ success: true, message: '', data: { items: [wireRule] } })))
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useImportRules(), { wrapper })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(result.current.data![0].matchValue).toBe('BLUE BOTTLE')
})

// The server REQUIRES a client-minted id (it is the create's idempotency
// key) and rejects a blank one with a 400 on field `id` — no call site passes
// one, so the hook mints it. Asserting on the posted body is the only thing
// that would have caught the mismatch: msw does not validate it.
it('useCreateImportRule mints the rule id, posts the spec, appends to the cache, and reports the action', async () => {
  let body: Record<string, unknown> | null = null
  server.use(http.post('*/api/v1/import/create-rule', async ({ request }) => {
    body = await request.json() as Record<string, unknown>
    return HttpResponse.json({ success: true, message: '', data: { ...wireRule, action: body.action } })
  }))
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData<ImportRuleDto[]>(queryKeys.importRules, [])
  const { result } = renderHook(() => useCreateImportRule(), { wrapper })
  const { id: _id, createdAt: _c, updatedAt: _u, ...spec } = wireRule
  await act(async () => { await result.current.mutateAsync({ spec: { ...spec, action: 'skip' } }) })
  expect(body).toMatchObject({ action: 'skip', matchValue: 'BLUE BOTTLE' })
  expect(body!.id).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
  expect(queryClient.getQueryData<ImportRuleDto[]>(queryKeys.importRules)).toHaveLength(1)
  expect(trackEventMock).toHaveBeenCalledWith(METRICS.IMPORT_RULE_CREATE, { action: 'skip' })
})

it('useCreateImportRule posts a caller-supplied id verbatim', async () => {
  let body: Record<string, unknown> | null = null
  server.use(http.post('*/api/v1/import/create-rule', async ({ request }) => {
    body = await request.json() as Record<string, unknown>
    return HttpResponse.json({ success: true, message: '', data: wireRule })
  }))
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useCreateImportRule(), { wrapper })
  const { id: _id, createdAt: _c, updatedAt: _u, ...spec } = wireRule
  await act(async () => { await result.current.mutateAsync({ spec, id: '0192b1e4-0000-7000-8000-000000000001' }) })
  expect(body!.id).toBe('0192b1e4-0000-7000-8000-000000000001')
})

it('useApplyImportRule posts the scope and invalidates the ledger caches', async () => {
  let body: unknown
  server.use(http.post('*/api/v1/import/apply-rule', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ success: true, message: '', data: { updated: 3, skipped: 2 } })
  }))
  const { queryClient, wrapper } = makeWrapper()
  const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
  const { result } = renderHook(() => useApplyImportRule(), { wrapper })
  let out: unknown
  await act(async () => {
    out = await result.current.mutateAsync({ ruleId: 'rule1', scope: { scope: 'run', runId: 'r1', scopeSourceId: '' }, includeEdited: false })
  })
  expect(body).toEqual({ ruleId: 'rule1', scope: 'run', runId: 'r1', scopeSourceId: '', includeEdited: false })
  expect(out).toEqual({ updated: 3, skipped: 2 })
  expect(invalidate).toHaveBeenCalledWith({ queryKey: queryKeys.transactions })
  expect(invalidate).toHaveBeenCalledWith({ queryKey: queryKeys.budget })
  // an apply rewrites applied_* on the rows it touched; that cache holds them
  // for ten minutes and the post-edit rule prompt compares against it
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ['transactionImports'] })
  expect(trackEventMock).toHaveBeenCalledWith(METRICS.IMPORT_RULE_APPLY, { updated: 3, skipped: 2 })
})

it('useSuggestImportRules returns the model proposals and fires the metric with the count', async () => {
  server.use(http.post('*/api/v1/import/suggest-rules', () =>
    HttpResponse.json({ success: true, message: '', data: { items: [{ ...wireRule, reason: 'Every Blue Bottle tap was categorised Coffee' }] } })))
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useSuggestImportRules(), { wrapper })
  let items: unknown[] = []
  await act(async () => { items = await result.current.mutateAsync({ scope: 'all', runId: '', scopeSourceId: '' }) })
  expect(items).toHaveLength(1)
  expect(trackEventMock).toHaveBeenCalledWith(METRICS.IMPORT_RULES_SUGGEST, { count: 1 })
})
