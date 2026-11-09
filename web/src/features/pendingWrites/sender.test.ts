import { http, HttpResponse } from 'msw'
import { onlineManager, type QueryClient } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { queryKeys } from '@/app/queryKeys'
import type { AccountDto } from '@/api/dto/account'
import type { CreateTransactionDto, TransactionDto } from '@/api/dto/transaction'
import { createTransaction } from '@/api/transaction'
import { createAppQueryClient } from '@/lib/appQueryClient'
import { STORE_PENDING_WRITES, dbRequest } from '@/lib/db'
import { METRICS, trackEvent } from '@/lib/metrics'
import { removeToken, setToken } from '@/lib/storage'
import { overlayTransactions } from './overlay'
import { addChange } from './queue'
import { kickPendingWrites, startPendingWrites } from './sender'
import { submitChange } from './submit'
import { realIdOf } from './tempIds'
import { getOps, loadPendingWrites, mutate, subscribe } from './store'
import type { OpChange, PendingOp } from './types'

vi.mock('@/app/routerRef', () => ({ navigateTo: vi.fn() }))
vi.mock('@/lib/metrics', async (original) => ({
  ...(await original<typeof import('@/lib/metrics')>()),
  trackEvent: vi.fn(),
}))

const me = { id: 'u1', name: 'Ann', avatar: 'face:fuchsia' }
const payload = (id: string, over: Partial<CreateTransactionDto> = {}): CreateTransactionDto => ({
  id, type: 'expense', accountId: 'a1', accountRecipientId: null, amount: '5', amountRecipient: null,
  categoryId: null, description: '', payeeId: null, tagId: null, labelIds: [], date: '2026-11-08 10:00:00', ...over,
})
const row = (id: string): TransactionDto => ({ ...payload(id), author: me, recurringId: null, isImported: 0 })
const accounts = [{ id: 'a1', transactionsRevision: 2 } as AccountDto]
const limit = (elementId: string): OpChange => ({ kind: 'limit', budgetId: 'b1', elementId, period: '2026-11-01', amount: '10' })
const envelope = (message = 'Nope') => ({ success: false, message, code: 0, errors: {} })

type Seen = { path: string; key: string | null; body: Record<string, unknown> }
let seen: Seen[]
let client: QueryClient
let stop: () => void

// answer: what the server says to the n-th request (0-based); undefined is a normal success
function answerWith(answer: (n: number, r: Seen) => Response | undefined | Promise<Response | undefined>) {
  server.use(
    http.post('*/api/v1/:module/:action', async ({ request, params }) => {
      const r: Seen = { path: String(params.action), key: request.headers.get('idempotency-key'), body: (await request.json()) as Record<string, unknown> }
      seen.push(r)
      const custom = await answer(seen.length - 1, r)
      if (custom) return custom
      if (r.path === 'set-limit') return HttpResponse.json({ success: true, message: '', data: {} })
      return HttpResponse.json({ success: true, message: '', data: { item: row(r.path === 'create-transaction' ? 'real' : String(r.body.id)), accounts } })
    }),
  )
}

const queue = (change: OpChange, key: string, seq: number) => mutate((ops) => addChange(ops, change, { key, seq }))
const ids = () => client.getQueryData<TransactionDto[]>(queryKeys.transactions)?.map((t) => t.id)
const op = (key: string): PendingOp | undefined => getOps().find((o) => o.key === key)
const invalidated = (key: readonly unknown[]) => client.getQueryState(key)?.isInvalidated
const addThenHold = async () => {
  await queue({ kind: 'tx-create', txId: 'temp', payload: payload('temp') }, 'k1', 1)
  const draining = kickPendingWrites()
  await vi.waitFor(() => expect(seen).toHaveLength(1))
  return { draining }
}

beforeEach(async () => {
  seen = []
  setToken('tok', 'u1')
  onlineManager.setOnline(true)
  vi.mocked(trackEvent).mockClear()
  client = createAppQueryClient()
  client.setQueryData(queryKeys.accounts, [])
  client.setQueryData(queryKeys.transactions, [])
  await loadPendingWrites('u1')
  stop = startPendingWrites(client)
  await kickPendingWrites()
})

afterEach(() => {
  stop()
  removeToken()
  onlineManager.setOnline(true)
  vi.restoreAllMocks()
})

it('sends oldest first, each under its stored key', async () => {
  answerWith(() => undefined)
  await queue(limit('e2'), 'k2', 2)
  await queue(limit('e1'), 'k1', 1)
  await kickPendingWrites()
  expect(seen.map((r) => r.key)).toEqual(['k1', 'k2'])
  expect(getOps()).toEqual([])
})

it('records the attempt on disk before the request leaves', async () => {
  let storedAttempted: boolean | undefined
  answerWith(async () => {
    const stored = await dbRequest<PendingOp>(STORE_PENDING_WRITES, 'readonly', (s) => s.get('k1') as IDBRequest<PendingOp>)
    storedAttempted = stored.attempted
    return undefined
  })
  await queue(limit('e1'), 'k1', 1)
  await kickPendingWrites()
  expect(storedAttempted).toBe(true)
})

it('never sends a temporary id on an add', async () => {
  answerWith(() => undefined)
  await queue({ kind: 'tx-create', txId: 'temp', payload: payload('temp') }, 'k1', 1)
  await kickPendingWrites()
  expect(seen[0].path).toBe('create-transaction')
  expect(seen[0].body).not.toHaveProperty('id')
})

it('an edit of a real row sends the real id', async () => {
  answerWith(() => undefined)
  await queue({ kind: 'tx-update', txId: 'real', payload: payload('real') }, 'k1', 1)
  await kickPendingWrites()
  expect(seen[0]).toMatchObject({ path: 'update-transaction', body: { id: 'real' } })
})

it('a delete posts the id and removes the row', async () => {
  answerWith(() => undefined)
  client.setQueryData(queryKeys.transactions, [row('real')])
  await queue({ kind: 'tx-delete', txId: 'real', previous: null }, 'k1', 1)
  await kickPendingWrites()
  expect(seen[0]).toMatchObject({ path: 'delete-transaction', body: { id: 'real' } })
  expect(ids()).toEqual([])
  expect(vi.mocked(trackEvent).mock.calls).toEqual([[METRICS.TRANSACTION_DELETE]])
})

it('a confirmed add swaps the row in one step, never twice and never missing', async () => {
  answerWith(() => undefined)
  await queue({ kind: 'tx-create', txId: 'temp', payload: payload('temp') }, 'k1', 1)
  const views: string[][] = []
  const off = subscribe(() => {
    views.push(overlayTransactions(client.getQueryData<TransactionDto[]>(queryKeys.transactions) ?? [], getOps(), me).map((t) => t.id))
  })
  await kickPendingWrites()
  off()
  expect(views.at(-1)).toEqual(['real'])
  for (const view of views) expect(view).toHaveLength(1)
  expect(client.getQueryData(queryKeys.accounts)).toEqual(accounts)
  expect(vi.mocked(trackEvent).mock.calls).toEqual([[METRICS.TRANSACTION_CREATE]])
})

it('a confirmed add gives the change behind it the real id', async () => {
  let release: () => void = () => {}
  const held = new Promise<void>((resolve) => { release = resolve })
  answerWith(async (n) => { if (n === 0) await held; return undefined })
  const { draining } = await addThenHold()
  await queue({ kind: 'tx-update', txId: 'temp', payload: payload('temp', { amount: '9' }) }, 'k2', 2)
  release()
  await draining
  expect(seen[1]).toMatchObject({ path: 'update-transaction', body: { id: 'real', amount: '9' } })
  expect(getOps()).toEqual([])
})

it('a replay resolves the temporary id, refreshes the data and leaves one row', async () => {
  client.setQueryData(queryKeys.transactions, [row('real')])
  const invalidate = vi.spyOn(client, 'invalidateQueries')
  let release: () => void = () => {}
  const held = new Promise<void>((resolve) => { release = resolve })
  answerWith(async (n) => {
    if (n !== 0) return undefined
    await held
    return HttpResponse.json({ ...envelope('Already done'), resourceId: 'real' }, { status: 409 })
  })
  const { draining } = await addThenHold()
  await queue({ kind: 'tx-update', txId: 'temp', payload: payload('temp', { amount: '9' }) }, 'k2', 2)
  release()
  await draining

  expect(seen[1].body).toMatchObject({ id: 'real', amount: '9' })
  expect(ids()).toEqual(['real'])
  expect(invalidate).toHaveBeenCalledWith({ queryKey: queryKeys.transactions })
  expect(invalidate).toHaveBeenCalledWith({ queryKey: queryKeys.accounts })
  expect(getOps()).toEqual([])
})

it('a replay of an add with no resource id sends the change behind it to the Inbox', async () => {
  let release: () => void = () => {}
  const held = new Promise<void>((resolve) => { release = resolve })
  answerWith(async (n) => {
    if (n !== 0) return undefined
    await held
    return HttpResponse.json(envelope('Already done'), { status: 409 })
  })
  const { draining } = await addThenHold()
  await queue({ kind: 'tx-update', txId: 'temp', payload: payload('temp') }, 'k2', 2)
  release()
  await draining

  expect(seen).toHaveLength(1)
  expect(op('k1')).toBeUndefined()
  expect(op('k2')).toMatchObject({ status: 'failed', error: 'Already done', attempted: false })
})

it('a refusal fails that change and the next one still goes', async () => {
  answerWith((n) => (n === 0 ? HttpResponse.json(envelope('Account deleted'), { status: 400 }) : undefined))
  await queue(limit('e1'), 'k1', 1)
  await queue(limit('e2'), 'k2', 2)
  await kickPendingWrites()
  expect(op('k1')).toMatchObject({ status: 'failed', error: 'Account deleted', attempted: false })
  expect(op('k2')).toBeUndefined()
  expect(seen.map((r) => r.key)).toEqual(['k1', 'k2'])
})

it('a network failure stops the drain and keeps everything waiting', async () => {
  answerWith(() => HttpResponse.error())
  await queue(limit('e1'), 'k1', 1)
  await queue(limit('e2'), 'k2', 2)
  await kickPendingWrites()
  expect(seen).toHaveLength(1)
  expect(op('k1')).toMatchObject({ status: 'waiting', attempted: true, retries: 0 })
  expect(op('k2')).toMatchObject({ status: 'waiting', attempted: false })
})

it('a network failure backs off from 2 s, doubling', async () => {
  answerWith(() => HttpResponse.error())
  const timers = vi.spyOn(globalThis, 'setTimeout')
  const waitOf = (ms: number) => timers.mock.calls.filter(([, delay]) => delay === ms).at(-1)?.[0] as (() => void) | undefined

  await queue(limit('e1'), 'k1', 1)
  await kickPendingWrites()
  expect(seen).toHaveLength(1)
  const realNow = Date.now.bind(Date)
  vi.spyOn(Date, 'now').mockImplementation(() => realNow() + 3000)
  waitOf(2000)?.()
  await vi.waitFor(() => expect(seen).toHaveLength(2))
  await vi.waitFor(() => expect(waitOf(4000)).toBeDefined())
})

it('a page that is not the app says nothing about the write: it waits', async () => {
  answerWith(() => new HttpResponse('<html>Bad gateway</html>', { status: 502, headers: { 'Content-Type': 'text/html' } }))
  await queue(limit('e1'), 'k1', 1)
  await kickPendingWrites()
  expect(op('k1')).toMatchObject({ status: 'waiting', retries: 0 })
})

it('a 5xx backs that change off and lets others pass', async () => {
  answerWith((n) => (n === 0 ? HttpResponse.json(envelope('Boom'), { status: 500 }) : undefined))
  await queue(limit('e1'), 'k1', 1)
  await queue(limit('e2'), 'k2', 2)
  const before = Date.now()
  await kickPendingWrites()
  expect(op('k1')).toMatchObject({ status: 'waiting', retries: 1 })
  expect(op('k1')!.notBefore).toBeGreaterThanOrEqual(before + 2000)
  expect(op('k2')).toBeUndefined()
})

it('retries a 5xx five times, then fails it with the server message', async () => {
  answerWith(() => HttpResponse.json(envelope('Boom'), { status: 500 }))
  await queue(limit('e1'), 'k1', 1)
  let now = Date.now()
  vi.spyOn(Date, 'now').mockImplementation(() => now)

  for (let attempt = 0; attempt < 6; attempt++) {
    await kickPendingWrites()
    now += 120_000
  }
  expect(seen).toHaveLength(6)
  expect(op('k1')).toMatchObject({ status: 'failed', error: 'Boom', retries: 0 })
  await kickPendingWrites()
  expect(seen).toHaveLength(6)
})

it('a 401 stops sending and keeps the queue', async () => {
  answerWith(() => HttpResponse.json(envelope('Unauthorized'), { status: 401 }))
  await queue(limit('e1'), 'k1', 1)
  await queue(limit('e2'), 'k2', 2)
  await kickPendingWrites()
  expect(seen).toHaveLength(1)
  expect(getOps()).toHaveLength(2)
  await kickPendingWrites()
  expect(seen).toHaveLength(1)
})

it('the same user signing back in resumes the queue when the user data arrives', async () => {
  answerWith((n) => (n === 0 ? HttpResponse.json(envelope('Unauthorized'), { status: 401 }) : undefined))
  await queue(limit('e1'), 'k1', 1)
  await kickPendingWrites()
  setToken('fresh', 'u1')
  await client.fetchQuery({ queryKey: queryKeys.user, queryFn: async () => me })
  await vi.waitFor(() => expect(getOps()).toEqual([]))
  expect(seen).toHaveLength(2)
})

it("sends nothing while the session belongs to another user, and drops that user's queue", async () => {
  answerWith(() => undefined)
  await queue(limit('e1'), 'k1', 1)
  setToken('tok', 'u2')
  await kickPendingWrites()
  expect(seen).toHaveLength(0)
  expect(getOps()).toEqual([])
})

it('sends nothing while the session names no user yet, and keeps the queue', async () => {
  answerWith(() => undefined)
  await queue(limit('e1'), 'k1', 1)
  setToken('tok')
  await kickPendingWrites()
  expect(seen).toHaveLength(0)
  expect(getOps()).toHaveLength(1)
})

it('sends nothing offline, and goes on reconnect', async () => {
  answerWith(() => undefined)
  await queue(limit('e1'), 'k1', 1)
  onlineManager.setOnline(false)
  await kickPendingWrites()
  expect(seen).toHaveLength(0)
  onlineManager.setOnline(true)
  await vi.waitFor(() => expect(getOps()).toEqual([]))
  expect(seen).toHaveLength(1)
})

it('a limit confirms by refreshing its budget, its plan and the reports', async () => {
  answerWith(() => undefined)
  const budget = [...queryKeys.budget, 'b1', '2026-11-01']
  const plan = [...queryKeys.budgetPlan, 'b1', 'x']
  const reports = [...queryKeys.reportNetWorth, 'mine']
  client.setQueryData(budget, {})
  client.setQueryData(plan, {})
  client.setQueryData(reports, {})
  await queue(limit('e1'), 'k1', 1)
  await kickPendingWrites()
  expect(invalidated(budget)).toBe(true)
  expect(invalidated(plan)).toBe(true)
  expect(invalidated(reports)).toBe(true)
  expect(vi.mocked(trackEvent).mock.calls).toEqual([[METRICS.BUDGET_UPDATE_ELEMENT_LIMIT]])
})

it('a change another tab confirmed refreshes the data here', async () => {
  await queue({ kind: 'tx-create', txId: 'temp', payload: payload('temp') }, 'k1', 1)
  const invalidate = vi.spyOn(client, 'invalidateQueries')
  await dbRequest(STORE_PENDING_WRITES, 'readwrite', (s) => s.delete('k1'))
  const otherTab = new BroadcastChannel('econumo-pending-writes')
  otherTab.postMessage('changed')
  await vi.waitFor(() => expect(getOps()).toEqual([]))
  otherTab.close()
  for (const queryKey of [queryKeys.transactions, queryKeys.accounts, queryKeys.budget, queryKeys.budgetPlan]) {
    expect(invalidate).toHaveBeenCalledWith({ queryKey })
  }
})

it('a change this tab confirmed does not read the transactions back', async () => {
  answerWith(() => undefined)
  await queue({ kind: 'tx-create', txId: 'temp', payload: payload('temp') }, 'k1', 1)
  const invalidate = vi.spyOn(client, 'invalidateQueries')
  await kickPendingWrites()
  expect(getOps()).toEqual([])
  expect(invalidate).not.toHaveBeenCalledWith({ queryKey: queryKeys.transactions })
})

describe('with Web Locks', () => {
  afterEach(() => {
    delete (navigator as { locks?: unknown }).locks
  })

  function stubLocks(available: boolean) {
    const request = vi.fn(async (_name: string, _opts: unknown, run: (lock: unknown) => Promise<void>) => run(available ? {} : null))
    Object.defineProperty(navigator, 'locks', { value: { request }, configurable: true })
    return request
  }

  it('the holder sends', async () => {
    answerWith(() => undefined)
    const request = stubLocks(true)
    await queue(limit('e1'), 'k1', 1)
    await kickPendingWrites()
    expect(request.mock.calls[0][1]).toEqual({ ifAvailable: true })
    expect(seen).toHaveLength(1)
  })

  it('a tab that cannot take the lock sends nothing', async () => {
    answerWith(() => undefined)
    stubLocks(false)
    await queue(limit('e1'), 'k1', 1)
    await kickPendingWrites()
    expect(seen).toHaveLength(0)
    expect(getOps()).toHaveLength(1)
  })
})

const replay = (resourceId: string) =>
  HttpResponse.json({ success: false, message: 'Operation is locked', code: 409, errors: {}, resourceId }, { status: 409 })

it('remembers the server id of an add that landed', async () => {
  answerWith(() => undefined)
  await queue({ kind: 'tx-create', txId: 'temp-accepted', payload: payload('temp-accepted') }, 'k1', 1)
  await kickPendingWrites()
  expect(realIdOf('temp-accepted')).toBe('real')
  expect(realIdOf('unrelated')).toBe('unrelated')
})

it('remembers the server id an add replay reports', async () => {
  answerWith(() => replay('real-replayed'))
  await queue({ kind: 'tx-create', txId: 'temp-replayed', payload: payload('temp-replayed') }, 'k1', 1)
  await kickPendingWrites()
  expect(realIdOf('temp-replayed')).toBe('real-replayed')
})

it('a 409 on the direct path ends with one row and nothing left in the queue', async () => {
  const id = '019a0000-0000-7000-8000-000000000001'
  const form = payload(id)
  client.setQueryData(queryKeys.transactions, [row('real')])
  answerWith(() => replay('real'))

  await expect(submitChange(client, { kind: 'tx-create', txId: id, payload: form }, (opts) => createTransaction(form, opts))).resolves.toEqual({ queued: true })
  await vi.waitFor(() => expect(getOps()).toEqual([]))

  expect(seen.map((r) => r.key)).toEqual([id, id])
  expect(seen.every((r) => !('id' in r.body))).toBe(true)
  expect(overlayTransactions(client.getQueryData<TransactionDto[]>(queryKeys.transactions) ?? [], getOps(), me).map((t) => t.id)).toEqual(['real'])
})

// The drain ends a few microtasks after its last check of the flag: a kick in
// that gap must start a new one. Every offset is tried, the gap moves with the engine.
it.each(Array.from({ length: 12 }, (_, ticks) => ticks))('a kick %i microtasks into a drain is honoured', async (ticks) => {
  answerWith(() => undefined)
  await queue(limit('e1'), 'k1', 1)
  onlineManager.setOnline(false)
  void kickPendingWrites()
  for (let i = 0; i < ticks; i++) {
    await Promise.resolve()
  }
  onlineManager.setOnline(true)
  await vi.waitFor(() => expect(getOps()).toEqual([]))
  expect(seen).toHaveLength(1)
})
