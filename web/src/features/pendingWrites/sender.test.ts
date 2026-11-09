import { createElement, type ReactNode } from 'react'
import { act, renderHook } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { AxiosError, type AxiosResponse } from 'axios'
import { onlineManager, QueryClientProvider, type QueryClient, QueryObserver } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import type { AccountDto } from '@/api/dto/account'
import type { BudgetDto, BudgetPlanDto } from '@/api/dto/budget'
import type { CreateTransactionDto, TransactionDto } from '@/api/dto/transaction'
import { createTransaction } from '@/api/transaction'
import { createAppQueryClient } from '@/lib/appQueryClient'
import { STORE_PENDING_WRITES, dbRequest } from '@/lib/db'
import { STORE_TIMEOUT_MS } from '@/lib/transactionStore'
import { delayTransactionCompletion } from '@/test/idbDelay'
import { METRICS, trackEvent } from '@/lib/metrics'
import { removeToken, setToken } from '@/lib/storage'
import { useAccounts } from '@/features/accounts/queries'
import { useTransactions } from '@/features/transactions/queries'
import { overlayAccounts, overlayBudget, overlayPlan, overlayTransactions } from './overlay'
import { addChange } from './queue'
import { refreshRestoredQueries } from '@/lib/queryPersist'
import { attemptedWritesResolved, holdRestoredReads, kickPendingWrites, startPendingWrites } from './sender'
import { submitChange } from './submit'
import { realIdOf } from './tempIds'
import { getOps, loadPendingWrites, mutate, resetPendingWritesForTests, subscribe } from './store'
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

// What the server holds when a replay reads the outcome back.
let serverRows: TransactionDto[]
let readBack: () => Response | undefined | Promise<Response | undefined>

// answer: what the server says to the n-th request (0-based); undefined is a normal success
function answerWith(answer: (n: number, r: Seen) => Response | undefined | Promise<Response | undefined>) {
  server.use(
    http.get('*/api/v1/account/get-account-list', async () => (
      (await readBack()) ?? HttpResponse.json({ success: true, message: '', data: { items: [{ id: 'a1', transactionsRevision: 3, owner: me, sharedAccess: [] }] } })
    )),
    http.get('*/api/v1/transaction/get-account-transaction-list', () => (
      HttpResponse.json({ success: true, message: '', data: { revisions: { a1: 3 }, items: serverRows, authors: [] } })
    )),
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

const replay = (resourceId: string) =>
  HttpResponse.json({ success: false, message: 'Operation is locked', code: 409, errors: {}, resourceId }, { status: 409 })

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
  serverRows = []
  readBack = () => undefined
  setToken('tok', 'u1')
  onlineManager.setOnline(true)
  vi.mocked(trackEvent).mockClear()
  client = createAppQueryClient()
  client.setQueryData(queryKeys.user, me)
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

it('a replay resolves the temporary id, reads the data back and leaves one row', async () => {
  serverRows = [row('real')]
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
  expect(getOps()).toEqual([])
})

describe('a replay of a transaction change', () => {
  const shown = () => overlayTransactions(client.getQueryData<TransactionDto[]>(queryKeys.transactions) ?? [], getOps(), me)
  const heldReads = () => {
    let open: () => void = () => {}
    const gate = new Promise<void>((resolve) => { open = resolve })
    readBack = async () => { await gate; return undefined }
    return open
  }
  const started = () => vi.waitFor(() => expect(seen.length).toBeGreaterThan(0))
  const add = () => queue({ kind: 'tx-create', txId: 'temp', payload: payload('temp') }, 'k1', 1)

  beforeEach(() => answerWith(() => replay('real')))

  it('keeps an add on screen through the read, and ends as exactly one row', async () => {
    serverRows = [row('real')]
    const open = heldReads()
    await add()
    const draining = kickPendingWrites()
    await started()
    await new Promise((r) => setTimeout(r, 50))
    expect(op('k1')).toMatchObject({ attempted: true })
    expect(shown().map((t) => t.id)).toEqual(['temp'])
    open()
    await draining
    expect(getOps()).toEqual([])
    expect(shown().map((t) => t.id)).toEqual(['real'])
  })

  it('does not bring a deleted row back while the read runs', async () => {
    client.setQueryData(queryKeys.transactions, [row('real')])
    const open = heldReads()
    await queue({ kind: 'tx-delete', txId: 'real', previous: null }, 'k1', 1)
    const draining = kickPendingWrites()
    await started()
    await new Promise((r) => setTimeout(r, 50))
    expect(shown()).toEqual([])
    open()
    await draining
    expect(getOps()).toEqual([])
    expect(shown()).toEqual([])
  })

  it('keeps showing an edit while the read runs, then shows the server row', async () => {
    client.setQueryData(queryKeys.transactions, [row('real')])
    serverRows = [{ ...row('real'), amount: '9' }]
    const open = heldReads()
    await queue({ kind: 'tx-update', txId: 'real', payload: payload('real', { amount: '9' }) }, 'k1', 1)
    const draining = kickPendingWrites()
    await started()
    await new Promise((r) => setTimeout(r, 50))
    expect(shown()).toMatchObject([{ id: 'real', amount: '9' }])
    open()
    await draining
    expect(getOps()).toEqual([])
    expect(shown()).toMatchObject([{ id: 'real', amount: '9' }])
  })

  it('a read that fails keeps the change for a later attempt, which repeats this', async () => {
    serverRows = [row('real')]
    readBack = () => HttpResponse.json(envelope('Boom'), { status: 500 })
    await add()
    await kickPendingWrites()
    expect(op('k1')).toMatchObject({ attempted: true, status: 'waiting' })
    expect(shown().map((t) => t.id)).toEqual(['temp'])

    readBack = () => undefined
    await kickPendingWrites()
    expect(seen).toHaveLength(2)
    expect(getOps()).toEqual([])
    expect(shown().map((t) => t.id)).toEqual(['real'])
  })

  it('a read that fails backs off before trying again', async () => {
    readBack = () => HttpResponse.json(envelope('Boom'), { status: 500 })
    const timers = vi.spyOn(globalThis, 'setTimeout')
    await add()
    await kickPendingWrites()
    expect(seen).toHaveLength(1)
    expect(timers.mock.calls.some(([, delay]) => delay === 2000)).toBe(true)
  })

  it('a read that never ends stops being waited for after the write timeout', async () => {
    heldReads()
    const timers = vi.spyOn(globalThis, 'setTimeout')
    await add()
    const draining = kickPendingWrites()
    await vi.waitFor(() => expect(timers.mock.calls.some(([, delay]) => delay === 10_000)).toBe(true))
    const giveUp = timers.mock.calls.findLast(([, delay]) => delay === 10_000)![0] as () => void
    giveUp()
    await draining
    expect(op('k1')).toMatchObject({ attempted: true, status: 'waiting' })
  })

  it('does not take a sync already running for the read back: that sync may predate the write', async () => {
    serverRows = [row('real')]
    readBack = () => HttpResponse.json({ success: true, message: '', data: { items: [{ id: 'a1', transactionsRevision: 3, owner: me, sharedAccess: [] }] } })
    let finishStaleSync: (rows: TransactionDto[]) => void = () => {}
    void client.fetchQuery({
      queryKey: queryKeys.transactions,
      queryFn: () => new Promise<TransactionDto[]>((resolve) => { finishStaleSync = resolve }),
      staleTime: 0,
    }).catch(() => {})
    await add()
    const draining = kickPendingWrites()
    await started()
    await new Promise((r) => setTimeout(r, 50))
    finishStaleSync([])
    await draining
    expect(getOps()).toEqual([])
    expect(shown().map((t) => t.id)).toEqual(['real'])
  })

  it('gives the change behind an add the real id when it settles', async () => {
    serverRows = [row('real')]
    answerWith((n) => (n === 0 ? replay('real') : undefined))
    const open = heldReads()
    await add()
    const draining = kickPendingWrites()
    await started()
    await queue({ kind: 'tx-update', txId: 'temp', payload: payload('temp', { amount: '9' }) }, 'k2', 2)
    open()
    await draining
    expect(seen[1].body).toMatchObject({ id: 'real', amount: '9' })
    expect(getOps()).toEqual([])
  })
})

describe('the balance and the list while a replay is read back', () => {
  const account = (balance: string, transactionsRevision: number) =>
    ({ id: 'a1', balance, transactionsRevision, position: 0, owner: me, sharedAccess: [] }) as unknown as AccountDto
  const shownBalance = () => overlayAccounts(client.getQueryData<AccountDto[]>(queryKeys.accounts) ?? [], getOps(), [], new Date())[0].balance
  const shownIds = () => overlayTransactions(client.getQueryData<TransactionDto[]>(queryKeys.transactions) ?? [], getOps(), me).map((t) => t.id)
  const serverBalance = () => HttpResponse.json({ success: true, message: '', data: { items: [account('95', 3)] } })
  const add = () => queue({ kind: 'tx-create', txId: 'temp', payload: payload('temp', { date: '2020-01-01 10:00:00' }) }, 'k1', 1)

  beforeEach(() => {
    answerWith(() => replay('real'))
    client.setQueryData(queryKeys.accounts, [account('100', 2)])
    client.setQueryData(queryKeys.transactions, [])
    serverRows = [row('real')]
    readBack = serverBalance
  })

  it('drops the read if another user signed in while it ran', async () => {
    await add()
    readBack = () => {
      setToken('tok', 'u2')
      return serverBalance()
    }
    await kickPendingWrites()
    expect(client.getQueryData<AccountDto[]>(queryKeys.accounts)?.[0].balance).toBe('100')
  })

  it('no render counts the money twice or shows the row twice', async () => {
    const wrapper = ({ children }: { children: ReactNode }) => createElement(QueryClientProvider, { client }, children)
    const renders: { balance?: string; ids?: string[] }[] = []
    renderHook(() => {
      renders.push({ balance: useAccounts().data?.[0]?.balance, ids: useTransactions().data?.map((t) => t.id) })
    }, { wrapper })
    await act(add)
    renders.length = 0
    let openRows = () => {}
    const rowsHeld = new Promise<void>((resolve) => { openRows = resolve })
    server.use(http.get('*/api/v1/transaction/get-account-transaction-list', async () => {
      await rowsHeld
      return HttpResponse.json({ success: true, message: '', data: { revisions: { a1: 3 }, items: serverRows, authors: [] } })
    }))

    let draining = Promise.resolve()
    await act(async () => {
      draining = kickPendingWrites()
      await vi.waitFor(() => expect(client.getQueryData<AccountDto[]>(queryKeys.accounts)?.[0].balance).toBe('95'))
    })
    expect(renders.at(-1)).toEqual({ balance: '95', ids: ['temp'] })

    await act(async () => {
      openRows()
      await draining
    })
    expect(getOps()).toEqual([])
    expect(renders.at(-1)).toEqual({ balance: '95', ids: ['real'] })
    expect(new Set(renders.map((r) => r.balance))).toEqual(new Set(['95']))
    expect(renders.filter((r) => r.ids?.length !== 1)).toEqual([])
  })

  it('a failed accounts read leaves the display as it was', async () => {
    readBack = () => HttpResponse.json(envelope('Boom'), { status: 500 })
    await add()
    await kickPendingWrites()
    expect(shownBalance()).toBe('95')
    expect(shownIds()).toEqual(['temp'])
    expect(op('k1')).toMatchObject({ status: 'waiting' })
  })

  it('a failed transactions read leaves the display as it was, and the next attempt ends it', async () => {
    server.use(http.get('*/api/v1/transaction/get-account-transaction-list', () => HttpResponse.json(envelope('Boom'), { status: 500 }), { once: true }))
    await add()
    await kickPendingWrites()
    expect(shownBalance()).toBe('95')
    expect(shownIds()).toEqual(['temp'])
    expect(op('k1')).toMatchObject({ status: 'waiting' })

    await kickPendingWrites()
    expect(getOps()).toEqual([])
    expect(shownBalance()).toBe('95')
    expect(shownIds()).toEqual(['real'])
  })

  it('stores that the change was read back and its real id: a reload must not count it again', async () => {
    server.use(http.get('*/api/v1/transaction/get-account-transaction-list', () => HttpResponse.json(envelope('Boom'), { status: 500 }), { once: true }))
    await add()
    await kickPendingWrites()
    expect(op('k1')?.landed).toEqual({ realId: 'real' })
    const stored = await dbRequest<PendingOp>(STORE_PENDING_WRITES, 'readonly', (s) => s.get('k1') as IDBRequest<PendingOp>)
    expect(stored.landed).toEqual({ realId: 'real' })
    expect(stored.attempted).toBe(true)

    resetPendingWritesForTests()
    await loadPendingWrites('u1')
    expect(op('k1')?.landed).toEqual({ realId: 'real' })
    expect(shownBalance()).toBe('95')
  })
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

it('sends what a stalled load delivers only after the boot stopped waiting for it', async () => {
  answerWith(() => undefined)
  stop()
  onlineManager.setOnline(false)
  await queue(limit('e1'), 'k1', 1)
  resetPendingWritesForTests()
  onlineManager.setOnline(true)

  const restore = delayTransactionCompletion(STORE_TIMEOUT_MS + 300)
  stop = startPendingWrites(client)
  await kickPendingWrites()
  expect(getOps()).toEqual([])
  expect(seen).toEqual([])
  restore()

  await vi.waitFor(() => expect(seen.map((r) => r.key)).toEqual(['k1']), { timeout: 5000 })
  await vi.waitFor(() => expect(getOps()).toEqual([]))
})

describe('a change submitted while the stored queue is still loading', () => {
  const edit = (amount: string): OpChange => ({ kind: 'tx-update', txId: 'real', payload: payload('real', { amount }) })

  // The older change sits on disk, the load is slow, and the page is online.
  async function reopenWithStored(over: Partial<PendingOp>, delay: number, onlyReads = false) {
    answerWith(() => undefined)
    stop()
    onlineManager.setOnline(false)
    await mutate((ops) => addChange(ops, edit('1'), { key: 'older', seq: 3 }).map((o) => ({ ...o, ...over }) as PendingOp))
    resetPendingWritesForTests()
    onlineManager.setOnline(true)
    const restore = delayTransactionCompletion(delay, onlyReads ? (_stores, mode) => mode === 'readonly' : undefined)
    stop = startPendingWrites(client)
    return restore
  }

  const newer = vi.fn()
  beforeEach(() => newer.mockClear())

  it('takes the queue when the load outlasts the wait, and only the newer values go out', async () => {
    const restore = await reopenWithStored({}, STORE_TIMEOUT_MS + 300)
    await expect(submitChange(client, edit('9'), newer)).resolves.toEqual({ queued: true })
    expect(newer).not.toHaveBeenCalled()
    expect(seen).toEqual([])
    restore()

    await vi.waitFor(() => expect(getOps()).toEqual([]), { timeout: 5000 })
    expect(seen).toHaveLength(1)
    expect(seen[0].body).toMatchObject({ id: 'real', amount: '9' })
  })

  it('sends a stored attempted change first, so the newer values land last', async () => {
    const restore = await reopenWithStored({ attempted: true }, STORE_TIMEOUT_MS + 300)
    await submitChange(client, edit('9'), newer)
    restore()

    await vi.waitFor(() => expect(getOps()).toEqual([]), { timeout: 5000 })
    expect(newer).not.toHaveBeenCalled()
    expect(seen.map((r) => r.body.amount)).toEqual(['1', '9'])
  })

  it('sends nothing queued meanwhile until the stored rows have arrived', async () => {
    const restore = await reopenWithStored({ attempted: true }, STORE_TIMEOUT_MS + 1500, true)
    await submitChange(client, edit('9'), newer)
    await kickPendingWrites()
    expect(getOps()).toHaveLength(1)
    expect(seen).toEqual([])
    restore()
    await vi.waitFor(() => expect(seen).toHaveLength(2), { timeout: 5000 })
  })

  it('waits for a load that settles in time and goes behind what it found', async () => {
    const restore = await reopenWithStored({}, 200)
    await expect(submitChange(client, edit('9'), newer)).resolves.toEqual({ queued: true })
    expect(newer).not.toHaveBeenCalled()
    restore()
    await vi.waitFor(() => expect(getOps()).toEqual([]), { timeout: 5000 })
    expect(seen.map((r) => [r.key, r.body.amount]).at(-1)?.[1]).toBe('9')
    expect(seen[0].key).toBe('older')
  })

  it('goes out directly once a load in time found nothing on the target', async () => {
    const restore = await reopenWithStored({ kind: 'limit', budgetId: 'b1', elementId: 'e1', period: '2026-11-01', amount: '1' } as Partial<PendingOp>, 200)
    newer.mockResolvedValue({ item: row('real'), accounts })
    await expect(submitChange(client, edit('9'), newer)).resolves.toMatchObject({ queued: false })
    expect(newer).toHaveBeenCalledTimes(1)
    restore()
  })

  it('does not wait with no user to load for', async () => {
    await reopenWithStored({}, STORE_TIMEOUT_MS + 300)
    removeToken()
    newer.mockResolvedValue({ item: row('real'), accounts })
    await expect(submitChange(client, edit('9'), newer)).resolves.toMatchObject({ queued: false })
  })
})

describe('holding the boot refresh for changes a previous run may have sent', () => {
  // The queue as a killed app left it, the page starting over.
  async function reopen(attempted: boolean) {
    stop()
    onlineManager.setOnline(false)
    await mutate((ops) => addChange(ops, limit('e1'), { key: 'k1', seq: 1 }).map((o) => ({ ...o, attempted })))
    resetPendingWritesForTests()
    onlineManager.setOnline(true)
  }
  const watch = () => {
    let resolved = false
    const done = attemptedWritesResolved().then(() => { resolved = true })
    return { done, isResolved: () => resolved }
  }

  it('waits until the attempted change has been resolved', async () => {
    let release: () => void = () => {}
    const held = new Promise<void>((resolve) => { release = resolve })
    answerWith(async () => { await held; return undefined })
    await reopen(true)
    stop = startPendingWrites(client)
    const { done, isResolved } = watch()
    await vi.waitFor(() => expect(seen).toHaveLength(1))
    await new Promise((r) => setTimeout(r, 30))
    expect(isResolved()).toBe(false)
    release()
    await done
    expect(getOps()).toEqual([])
  })

  const fireWriteLimit = (timers: { mock: { calls: unknown[][] } }) => {
    (timers.mock.calls.find(([, delay]) => delay === 10_000)![0] as () => void)()
  }

  it('keeps waiting when the first attempt ends in a network error, until the write timeout', async () => {
    answerWith(() => HttpResponse.error())
    await reopen(true)
    const timers = vi.spyOn(globalThis, 'setTimeout')
    stop = startPendingWrites(client)
    const { done, isResolved } = watch()
    await vi.waitFor(() => expect(seen).toHaveLength(1))
    await new Promise((r) => setTimeout(r, 30))
    expect(isResolved()).toBe(false)
    fireWriteLimit(timers)
    await done
    expect(op('k1')).toMatchObject({ attempted: true, status: 'waiting' })
  })

  it('resolves when a retry after a network error settles the change', async () => {
    answerWith((n) => (n === 0 ? HttpResponse.error() : undefined))
    await reopen(true)
    stop = startPendingWrites(client)
    const { done, isResolved } = watch()
    await vi.waitFor(() => expect(seen).toHaveLength(1))
    await new Promise((r) => setTimeout(r, 30))
    expect(isResolved()).toBe(false)
    await kickPendingWrites()
    await done
    expect(getOps()).toEqual([])
  })

  it('is held no longer than the write timeout', async () => {
    answerWith(() => new Promise<never>(() => {}))
    await reopen(true)
    const timers = vi.spyOn(globalThis, 'setTimeout')
    stop = startPendingWrites(client)
    const { done, isResolved } = watch()
    await vi.waitFor(() => expect(seen).toHaveLength(1))
    expect(isResolved()).toBe(false)
    const limit10s = timers.mock.calls.find(([, delay]) => delay === 10_000)![0] as () => void
    limit10s()
    await done
    expect(op('k1')).toMatchObject({ attempted: true })
  })

  it('does not wait for a change that was never sent', async () => {
    let release: () => void = () => {}
    const held = new Promise<void>((resolve) => { release = resolve })
    answerWith(async () => { await held; return undefined })
    await reopen(false)
    stop = startPendingWrites(client)
    await attemptedWritesResolved()
    release()
    await vi.waitFor(() => expect(getOps()).toEqual([]))
  })

  describe('restored accounts and transactions older than their staleTime', () => {
    const TEN_MINUTES_AGO = () => Date.now() - 11 * 60_000
    // as a restore leaves them, then a screen mounting
    function restoredAged() {
      client.setQueryData(queryKeys.accounts, [], { updatedAt: TEN_MINUTES_AGO() })
      client.setQueryData(queryKeys.transactions, [], { updatedAt: TEN_MINUTES_AGO() })
    }
    function mountAccounts() {
      const queryFn = vi.fn(async () => [])
      const observer = new QueryObserver(client, { queryKey: queryKeys.accounts, queryFn, staleTime: TEN_MINUTES })
      onTestFinished(observer.subscribe(() => {}))
      return queryFn
    }

    it('are held on mount while a previous run\'s change waits, and read once it is resolved', async () => {
      let release: () => void = () => {}
      const held = new Promise<void>((resolve) => { release = resolve })
      answerWith(async () => { await held; return undefined })
      await reopen(true)
      stop = startPendingWrites(client)
      restoredAged()
      const resolved = attemptedWritesResolved()
      await holdRestoredReads(client)
      expect(client.getQueryState(queryKeys.transactions)?.dataUpdatedAt).toBeGreaterThan(TEN_MINUTES_AGO())
      const fetched = mountAccounts()
      await new Promise((r) => setTimeout(r, 30))
      expect(fetched).not.toHaveBeenCalled()

      release()
      await resolved
      refreshRestoredQueries(client)
      await vi.waitFor(() => expect(fetched).toHaveBeenCalledTimes(1))
    })

    it('fetch on mount as usual with nothing waiting for its outcome', async () => {
      answerWith(() => undefined)
      restoredAged()
      await holdRestoredReads(client)
      const fetched = mountAccounts()
      await vi.waitFor(() => expect(fetched).toHaveBeenCalledTimes(1))
    })
  })

  it('does not wait with an empty queue, offline or with no session', async () => {
    answerWith(() => undefined)
    await attemptedWritesResolved()
    expect(seen).toEqual([])

    await reopen(true)
    onlineManager.setOnline(false)
    await attemptedWritesResolved()
    expect(seen).toEqual([])

    onlineManager.setOnline(true)
    removeToken()
    await attemptedWritesResolved()
    expect(seen).toEqual([])
  })
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
  client.setQueryData(budget, { meta: {}, structure: { elements: [] } })
  client.setQueryData(plan, { months: [], structure: { elements: [] } })
  client.setQueryData(reports, {})
  await queue(limit('e1'), 'k1', 1)
  await kickPendingWrites()
  expect(invalidated(budget)).toBe(true)
  expect(invalidated(plan)).toBe(true)
  expect(invalidated(reports)).toBe(true)
  expect(vi.mocked(trackEvent).mock.calls).toEqual([[METRICS.BUDGET_UPDATE_ELEMENT_LIMIT]])
})

describe('a confirmed queued limit', () => {
  const period = '2026-11-01'
  const budget = (budgeted: string) => ({ meta: {}, structure: { elements: [{ id: 'e1', budgeted, isBudgeted: 1 }, { id: 'e2', budgeted: '1', isBudgeted: 1 }] } }) as unknown as BudgetDto
  const plan = (months: string[], planned: string) => ({
    months,
    structure: { elements: [{ id: 'e1', cells: months.map(() => ({ planned })) }] },
  }) as unknown as BudgetPlanDto
  const budgetKey = [...queryKeys.budget, 'b1', period]
  const planKey = [...queryKeys.budgetPlan, 'b1', '2026-10-01', 2]
  const shown = () => ({
    budget: overlayBudget(client.getQueryData<BudgetDto>(budgetKey)!, getOps(), 'b1', period).structure.elements[0].budgeted,
    plan: overlayPlan(client.getQueryData<BudgetPlanDto>(planKey)!, getOps(), 'b1').structure.elements[0].cells[1].planned,
  })

  async function confirmWatching(answer: () => Response | undefined) {
    client.setQueryData(budgetKey, budget('200'))
    client.setQueryData(planKey, plan(['2026-10-01', period], '200'))
    answerWith(answer)
    await queue({ kind: 'limit', budgetId: 'b1', elementId: 'e1', period, amount: '250' }, 'k1', 1)
    const seenViews: unknown[] = []
    const look = () => seenViews.push(shown())
    const offStore = subscribe(look)
    const offCache = client.getQueryCache().subscribe(look)
    await kickPendingWrites()
    offStore()
    offCache()
    expect(getOps()).toEqual([])
    expect(seenViews.length).toBeGreaterThan(0)
    for (const view of seenViews) {
      expect(view).toEqual({ budget: '250', plan: '250' })
    }
  }

  it('keeps its value in the raw caches, so the view never goes back to the old one', async () => {
    await confirmWatching(() => undefined)
    expect(client.getQueryData<BudgetDto>(budgetKey)?.structure.elements.map((e) => e.budgeted)).toEqual(['250', '1'])
    expect(client.getQueryData<BudgetPlanDto>(planKey)?.structure.elements[0].cells.map((c) => c.planned)).toEqual(['200', '250'])
    expect(invalidated(budgetKey)).toBe(true)
    expect(invalidated(planKey)).toBe(true)
  })

  it('does the same for a replay, whose response was lost', async () => {
    await confirmWatching(() => HttpResponse.json({ success: false, message: 'Operation is locked', code: 409, errors: {} }, { status: 409 }))
    expect(client.getQueryData<BudgetDto>(budgetKey)?.structure.elements[0].budgeted).toBe('250')
    expect(invalidated(budgetKey)).toBe(true)
  })

  it('leaves another period, another budget and a window without the month alone', async () => {
    const otherPeriod = [...queryKeys.budget, 'b1', '2026-10-01']
    const otherBudget = [...queryKeys.budget, 'b2', period]
    const otherWindow = [...queryKeys.budgetPlan, 'b1', '2026-01-01', 2]
    client.setQueryData(otherPeriod, budget('200'))
    client.setQueryData(otherBudget, budget('200'))
    client.setQueryData(otherWindow, plan(['2026-01-01', '2026-02-01'], '200'))
    answerWith(() => undefined)
    await queue({ kind: 'limit', budgetId: 'b1', elementId: 'e1', period, amount: '250' }, 'k1', 1)
    await kickPendingWrites()
    expect(client.getQueryData<BudgetDto>(otherPeriod)?.structure.elements[0].budgeted).toBe('200')
    expect(client.getQueryData<BudgetDto>(otherBudget)?.structure.elements[0].budgeted).toBe('200')
    expect(client.getQueryData<BudgetPlanDto>(otherWindow)?.structure.elements[0].cells.map((c) => c.planned)).toEqual(['200', '200'])
  })
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

describe('another tab wrote the row and this tab has not heard yet', () => {
  it('sends the values on the disk, not the copy in memory', async () => {
    answerWith(() => undefined)
    await queue({ kind: 'tx-update', txId: 'real', payload: payload('real', { amount: '1' }) }, 'k1', 1)
    const stored = await dbRequest<PendingOp>(STORE_PENDING_WRITES, 'readonly', (s) => s.get('k1') as IDBRequest<PendingOp>)
    await dbRequest(STORE_PENDING_WRITES, 'readwrite', (s) => s.put({ ...stored, payload: payload('real', { amount: '9' }) }))
    await kickPendingWrites()
    expect(seen).toHaveLength(1)
    expect(seen[0]).toMatchObject({ key: 'k1', body: { amount: '9' } })
  })

  it('sends nothing for a row already gone, and leaves no copy to try again', async () => {
    answerWith(() => undefined)
    await queue(limit('e1'), 'k1', 1)
    await dbRequest(STORE_PENDING_WRITES, 'readwrite', (s) => s.delete('k1'))
    await kickPendingWrites()
    expect(seen).toEqual([])
    expect(getOps()).toEqual([])
  })
})

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

describe('the session changing while a direct request is outstanding', () => {
  const add: OpChange = { kind: 'tx-create', txId: 'temp', payload: payload('temp') }
  const failedAfterCommit = new AxiosError('failed', 'ERR_BAD_RESPONSE', undefined, undefined, { status: 500, data: envelope('Boom') } as AxiosResponse)
  const noAnswer = new AxiosError('timeout', AxiosError.ECONNABORTED)
  const replayed = new AxiosError('locked', 'ERR_BAD_REQUEST', undefined, undefined, { status: 409, data: envelope('Operation is locked') } as AxiosResponse)
  const switchUser = async () => {
    setToken('tok2', 'u2')
    await loadPendingWrites('u2')
  }

  it.each([['no answer', noAnswer], ['a 5xx', failedAfterCommit], ['a replay', replayed]])('%s does not reach the next user\'s queue', async (_name, failure) => {
    const send = vi.fn(async () => {
      await switchUser()
      throw failure
    })
    await expect(submitChange(client, add, send)).resolves.toEqual({ queued: true })
    expect(getOps()).toEqual([])
  })

  it('nothing is queued after a sign-out either', async () => {
    const send = vi.fn(async () => {
      removeToken()
      throw noAnswer
    })
    await expect(submitChange(client, add, send)).resolves.toEqual({ queued: true })
    expect(getOps()).toEqual([])
  })

  it('a refusal still rejects', async () => {
    const refusal = new AxiosError('refused', 'ERR_BAD_REQUEST', undefined, undefined, { status: 400, data: envelope() } as AxiosResponse)
    const send = vi.fn(async () => {
      await switchUser()
      throw refusal
    })
    await expect(submitChange(client, add, send)).rejects.toBe(refusal)
    expect(getOps()).toEqual([])
  })

  it('an accepted answer does not reach the next user\'s caches', async () => {
    const send = vi.fn(async () => {
      await switchUser()
      client.setQueryData(queryKeys.transactions, [])
      return { item: row('real'), accounts }
    })
    await submitChange(client, add, send)
    expect(ids()).toEqual([])
  })

  it('the same user still queues', async () => {
    answerWith(() => new Promise<never>(() => {}))
    await expect(submitChange(client, add, vi.fn().mockRejectedValue(noAnswer))).resolves.toEqual({ queued: true })
  })
})

it('a 409 on the direct path ends with one row and nothing left in the queue', async () => {
  const id = '019a0000-0000-7000-8000-000000000001'
  const form = payload(id)
  serverRows = [row('real')]
  answerWith(() => replay('real'))

  await expect(submitChange(client, { kind: 'tx-create', txId: id, payload: form }, (opts) => createTransaction(form, opts))).resolves.toEqual({ queued: true })
  await vi.waitFor(() => expect(getOps()).toEqual([]))

  expect(seen.slice(0, 2).map((r) => [r.path, r.key])).toEqual([['create-transaction', id], ['create-transaction', id]])
  expect(seen.slice(0, 2).every((r) => !('id' in r.body))).toBe(true)
  expect(overlayTransactions(client.getQueryData<TransactionDto[]>(queryKeys.transactions) ?? [], getOps(), me).map((t) => t.id)).toEqual(['real'])
})

it('a delete whose direct request got a 5xx after committing is finished by the 409, not shown as failed', async () => {
  client.setQueryData(queryKeys.transactions, [row('real')])
  serverRows = []
  answerWith(() => HttpResponse.json({ success: false, message: 'Operation is locked', code: 409, errors: {} }, { status: 409 }))
  const failedAfterCommit = new AxiosError('failed', 'ERR_BAD_RESPONSE', undefined, undefined, { status: 500, data: envelope('Boom') } as AxiosResponse)
  await expect(submitChange(client, { kind: 'tx-delete', txId: 'real', previous: null }, vi.fn().mockRejectedValue(failedAfterCommit))).resolves.toEqual({ queued: true })
  await vi.waitFor(() => expect(getOps()).toEqual([]))
  expect(ids()).toEqual([])
})

it('a 409 on a corrected resubmit ends with the row carrying the later values', async () => {
  const id = '019a0000-0000-7000-8000-000000000002'
  const corrected = payload(id, { amount: '9' })
  serverRows = [row('real')]
  answerWith((_n, r) => {
    if (r.path === 'create-transaction') return replay('real')
    return HttpResponse.json({ success: true, message: '', data: { item: { ...row('real'), amount: r.body.amount }, accounts } })
  })

  await submitChange(client, { kind: 'tx-create', txId: id, payload: corrected }, (opts) => createTransaction(corrected, opts))
  await vi.waitFor(() => expect(getOps()).toEqual([]))

  expect(seen.map((r) => r.path)).toEqual(['create-transaction', 'create-transaction', 'update-transaction'])
  expect(seen[2].key).not.toBe(id)
  expect(seen[2].body).toMatchObject({ id: 'real', amount: '9' })
  expect(client.getQueryData<TransactionDto[]>(queryKeys.transactions)).toMatchObject([{ id: 'real', amount: '9' }])
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
