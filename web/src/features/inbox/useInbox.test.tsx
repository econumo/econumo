import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts } from '@/test/fixtures'
import { formatDateTime } from '@/lib/datetime'
import { formatInboxCount, isDueRecurring, isSyncProblem, useInbox } from './useInbox'

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
  window.econumoConfig = { IMPORT_APPLE_WALLET: true, IMPORT_SIMPLEFIN: true }
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
  await waitFor(() => expect(result.current.isLoaded).toBe(true))
  await waitFor(() => expect(result.current.invites).toHaveLength(1))
  expect(result.current.count).toBe(1)
  expect(result.current.importsError).toBe(false)
  expect(importCalls).toEqual([])
})

const daysFromNow = (days: number) => formatDateTime(new Date(Date.now() + days * 24 * 3600 * 1000))
const template = (over: Record<string, unknown> = {}) => ({
  id: 'r1', ownerUserId: 'u1', type: 'expense', accountId: 'a1', accountRecipientId: null,
  amount: '50', categoryId: 'cat-food', payeeId: null, tagId: null, labelIds: [], description: 'rent',
  schedule: 'monthly', nextPaymentAt: daysFromNow(-3), ...over,
})
const startOfToday = () => {
  const d = new Date()
  d.setHours(0, 0, 0, 0)
  return formatDateTime(d)
}
const sharedWith = (role: string, isAccepted = 1) => ({
  ...fixtureAccounts[0], id: 'a-shared', owner,
  sharedAccess: [{ user: { id: 'u1', avatar: 'face:emerald', name: 'Ada' }, role, isAccepted }],
})

describe('isDueRecurring', () => {
  const accounts = [...fixtureAccounts, sharedWith('user'), { ...sharedWith('guest'), id: 'a-readonly' }] as never

  it.each([
    ['overdue', daysFromNow(-3), true],
    ['due at the start of today', startOfToday(), true],
    ['due tomorrow', daysFromNow(1), false],
    ['due next year', daysFromNow(365), false],
  ])('%s on my own account', (_name, nextPaymentAt, expected) => {
    expect(isDueRecurring(template({ nextPaymentAt }) as never, accounts, 'u1')).toBe(expected)
  })

  it('includes a template on an account shared with me for writing, whoever created it', () => {
    expect(isDueRecurring(template({ accountId: 'a-shared', ownerUserId: 'u2' }) as never, accounts, 'u1')).toBe(true)
  })

  it('leaves out a template on an account shared with me read-only', () => {
    expect(isDueRecurring(template({ accountId: 'a-readonly', ownerUserId: 'u2' }) as never, accounts, 'u1')).toBe(false)
  })

  it('leaves out a template whose account is not in my account list', () => {
    expect(isDueRecurring(template({ accountId: 'a-gone' }) as never, accounts, 'u1')).toBe(false)
  })

  it('leaves everything out before the user is known', () => {
    expect(isDueRecurring(template() as never, accounts, undefined)).toBe(false)
  })
})

it('counts due recurring templates, oldest first, and leaves future ones out', async () => {
  const wrapper = makeWrapper()
  server.use(...coreHandlers({
    recurring: [
      template({ id: 'r-recent', nextPaymentAt: daysFromNow(-1) }),
      template({ id: 'r-future', nextPaymentAt: daysFromNow(30) }),
      template({ id: 'r-old', nextPaymentAt: daysFromNow(-20) }),
    ],
  }))
  const { result } = renderHook(() => useInbox(), { wrapper })
  await waitFor(() => expect(result.current.dueRecurring.map((r) => r.id)).toEqual(['r-old', 'r-recent']))
  expect(result.current.count).toBe(2)
})

it('is not loaded until the recurring list resolves', async () => {
  const wrapper = makeWrapper()
  server.use(...coreHandlers())
  server.use(http.get('*/api/v1/recurring/get-recurring-transaction-list', () => new Promise(() => {})))
  const { result } = renderHook(() => useInbox(), { wrapper })
  // the import queries settle, the recurring one never does
  await new Promise((r) => setTimeout(r, 50))
  expect(result.current.isLoaded).toBe(false)
})
