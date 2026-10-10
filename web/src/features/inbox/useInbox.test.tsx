import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { formatInboxCount, isSyncProblem, useInbox } from './useInbox'

const owner = { id: 'u2', avatar: 'pets:sky', name: 'Partner' }

const pendingAccount = {
  id: 'a-pending', owner, folderId: null, name: 'Shared cash', position: 0,
  currency: { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 },
  balance: '0', type: 1, icon: 'wallet',
  sharedAccess: [{ user: { id: 'u1', avatar: 'face:emerald', name: 'Ada' }, role: 'user', isAccepted: 0 }],
}

function makeWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  return wrapper
}

const src = (over: Record<string, unknown> = {}) => ({
  id: 's1', provider: 'simplefin', name: 'Bank', status: 'active', createdAt: '2026-08-01 00:00:00',
  lastSyncedAt: '', lastRunStatus: '', lastRunAt: '', lastRunError: '', lastRunErrorAccountId: '', credentialCiphertext: '', cards: [], ...over,
})
const q = (linkId: string) => ({ linkId, sourceId: 's1', externalAccountId: 'Apple Card', accountId: '', payee: 'P', amount: '1',
  currency: 'USD', type: 'expense', postedAt: '2026-08-20 10:42:03', reason: 'unmapped' })
const f = (eventId: string) => ({ eventId, sourceId: 's1', receivedAt: '2026-08-21 08:00:00', error: 'x', payload: '{}' })

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = { TRANSACTION_IMPORT: true }
})

it.each([
  ['failed', true], ['partial', true], ['running', false], ['completed', false], ['', false],
])('isSyncProblem(simplefin, %s) = %s', (status, expected) => {
  expect(isSyncProblem(src({ lastRunStatus: status }) as never)).toBe(expected)
})

it.each([
  ['partial', false], ['failed', false],
])('isSyncProblem(apple-wallet, %s) = %s', (status, expected) => {
  expect(isSyncProblem(src({ provider: 'apple-wallet', lastRunStatus: status }) as never)).toBe(expected)
})

it('formats the badge count', () => {
  expect(formatInboxCount(1)).toBe('1')
  expect(formatInboxCount(99)).toBe('99')
  expect(formatInboxCount(100)).toBe('99+')
})

it('counts invites + sync problems + failed + queued, never skipped', async () => {
  const wrapper = makeWrapper()
  server.use(...coreHandlers({
    accounts: [pendingAccount],
    importSources: [src({ lastRunStatus: 'failed' }), src({ id: 's2', lastRunStatus: 'completed' })],
    importQueue: { queued: [q('l1'), q('l2')], skipped: [q('l3')], failed: [f('e1')] },
  }))
  const { result } = renderHook(() => useInbox(), { wrapper })
  await waitFor(() => expect(result.current.isLoaded).toBe(true))
  await waitFor(() => expect(result.current.invites).toHaveLength(1))
  expect(result.current.syncProblems.map((s) => s.id)).toEqual(['s1'])
  expect(result.current.skipped).toHaveLength(1)
  expect(result.current.count).toBe(1 + 1 + 1 + 2)
})

it('is not loaded until the queue and sources resolve, and flags a failed queue fetch', async () => {
  const wrapper = makeWrapper()
  server.use(...coreHandlers({ importSources: [] }))
  // registered after the coreHandlers call above, so it wins the override
  server.use(http.get('*/api/v1/import/get-queued-event-list', () => HttpResponse.json({ success: false, message: 'boom', code: 0 }, { status: 500 })))
  const { result } = renderHook(() => useInbox(), { wrapper })
  expect(result.current.isLoaded).toBe(false)
  await waitFor(() => expect(result.current.importsError).toBe(true))
  // sources resolved (empty list) and the queue resolved to an error: both
  // queries are settled, so the page has enough information to render.
  await waitFor(() => expect(result.current.isLoaded).toBe(true))
})

it('flags importsError and still loads when the source list fails, even with an empty queue', async () => {
  const wrapper = makeWrapper()
  server.use(...coreHandlers({ importQueue: { queued: [], skipped: [], failed: [] } }))
  server.use(http.get('*/api/v1/import/get-source-list', () => HttpResponse.json({ success: false, message: 'boom', code: 0 }, { status: 500 })))
  const { result } = renderHook(() => useInbox(), { wrapper })
  await waitFor(() => expect(result.current.isLoaded).toBe(true))
  expect(result.current.importsError).toBe(true)
})

it('retryImports refetches both the queue and the source list', async () => {
  const wrapper = makeWrapper()
  let queueCalls = 0
  let sourceCalls = 0
  server.use(...coreHandlers())
  server.use(
    http.get('*/api/v1/import/get-queued-event-list', () => {
      queueCalls += 1
      return HttpResponse.json({ success: false, message: 'boom', code: 0 }, { status: 500 })
    }),
    http.get('*/api/v1/import/get-source-list', () => {
      sourceCalls += 1
      return HttpResponse.json({ success: true, message: '', data: { items: [] } })
    }),
  )
  const { result } = renderHook(() => useInbox(), { wrapper })
  await waitFor(() => expect(result.current.isLoaded).toBe(true))
  const queueCallsBefore = queueCalls
  const sourceCallsBefore = sourceCalls
  result.current.retryImports()
  await waitFor(() => expect(queueCalls).toBeGreaterThan(queueCallsBefore))
  await waitFor(() => expect(sourceCalls).toBeGreaterThan(sourceCallsBefore))
})

it('with transaction import off: loads on invites alone and never calls an import endpoint', async () => {
  window.econumoConfig = {}
  const wrapper = makeWrapper()
  const importCalls: string[] = []
  const record = ({ request }: { request: Request }) => {
    if (request.url.includes('/api/v1/import/')) importCalls.push(request.url)
  }
  server.events.on('request:start', record)
  onTestFinished(() => server.events.removeListener('request:start', record))
  server.use(...coreHandlers({ accounts: [pendingAccount] }))
  const { result } = renderHook(() => useInbox(), { wrapper })
  expect(result.current.isLoaded).toBe(true)
  await waitFor(() => expect(result.current.invites).toHaveLength(1))
  expect(result.current.count).toBe(1)
  expect(result.current.importsError).toBe(false)
  expect(importCalls).toEqual([])
})
