import { STORE_PENDING_WRITES, dbRequest } from '@/lib/db'
import { STORE_TIMEOUT_MS } from '@/lib/transactionStore'
import { delayTransactionCompletion } from '@/test/idbDelay'
import { clearPendingWrites, getOps, loadPendingWrites, mutate, resetPendingWritesForTests, subscribe } from './store'
import type { PendingOp } from './types'

const op = (key: string, seq: number, over: Partial<PendingOp> = {}): PendingOp => ({
  key, seq, attempted: false, status: 'waiting', retries: 0, notBefore: 0,
  kind: 'limit', budgetId: 'b', elementId: key, period: '2026-11-01', amount: '1',
  ...over,
} as PendingOp)

type Row = PendingOp & { userId: string }
const rows = () => dbRequest<Row[]>(STORE_PENDING_WRITES, 'readonly', (s) => s.getAll() as IDBRequest<Row[]>)

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

it('keeps the operations across a reload, tagged with their user', async () => {
  await loadPendingWrites('A')
  await mutate((ops) => [...ops, op('k1', 1), op('k2', 2)])
  expect((await rows()).map((r) => r.userId)).toEqual(['A', 'A'])

  resetPendingWritesForTests()
  expect(getOps()).toEqual([])
  const loaded = await loadPendingWrites('A')
  expect(loaded.map((o) => o.key).sort()).toEqual(['k1', 'k2'])
  expect(loaded[0]).not.toHaveProperty('userId')
})

it('writes a change and a removal row by row', async () => {
  await loadPendingWrites('A')
  await mutate(() => [op('k1', 1), op('k2', 2)])
  await mutate((ops) => ops.filter((o) => o.key !== 'k1').map((o) => ({ ...o, attempted: true })))
  const stored = await rows()
  expect(stored.map((r) => r.key)).toEqual(['k2'])
  expect(stored[0].attempted).toBe(true)
})

it("loading another user's queue returns nothing and deletes their rows", async () => {
  await loadPendingWrites('A')
  await mutate(() => [op('k1', 1)])

  resetPendingWritesForTests()
  expect(await loadPendingWrites('B')).toEqual([])
  expect(await rows()).toEqual([])
})

it('drops the memory of the previous user when the owner changes', async () => {
  await loadPendingWrites('A')
  await mutate(() => [op('k1', 1)])
  expect(await loadPendingWrites('B')).toEqual([])
  expect(getOps()).toEqual([])
})

it('clear empties memory and disk', async () => {
  await loadPendingWrites('A')
  await mutate(() => [op('k1', 1)])
  await clearPendingWrites()
  expect(getOps()).toEqual([])
  expect(await rows()).toEqual([])
})

it('a change made before the load lands after the stored ones and is written', async () => {
  await loadPendingWrites('A')
  await mutate(() => [op('stored', 5)])
  resetPendingWritesForTests()

  await mutate(() => [op('early', 1)])
  expect(await rows()).toHaveLength(1)
  await loadPendingWrites('A')
  expect([...getOps()].sort((a, b) => a.seq - b.seq).map((o) => o.key)).toEqual(['stored', 'early'])
  expect((await rows()).find((r) => r.key === 'early')?.seq).toBe(6)
})

it('tells subscribers, and an unsubscribed one no longer hears', async () => {
  const heard = vi.fn()
  const off = subscribe(heard)
  await mutate(() => [op('k1', 1)])
  expect(heard).toHaveBeenCalledTimes(1)
  off()
  await mutate(() => [])
  expect(heard).toHaveBeenCalledTimes(1)
})

it('a database that fails keeps the queue in memory and warns without the contents', async () => {
  const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
  await loadPendingWrites('A')
  const failing = vi.spyOn(IDBDatabase.prototype, 'transaction').mockImplementation(() => {
    throw new DOMException('broken', 'InvalidStateError')
  })
  await mutate(() => [op('k1', 1, { amount: '123.45' } as Partial<PendingOp>)])
  failing.mockRestore()

  expect(getOps().map((o) => o.key)).toEqual(['k1'])
  expect(warn).toHaveBeenCalledTimes(1)
  expect(JSON.stringify(warn.mock.calls)).not.toContain('123.45')
})

it('a stalled database does not hold the load past the limit, and its rows arrive afterwards', async () => {
  await loadPendingWrites('A')
  await mutate(() => [op('k1', 1)])
  resetPendingWritesForTests()

  delayTransactionCompletion(STORE_TIMEOUT_MS + 300)
  expect(await loadPendingWrites('A')).toEqual([])
  await vi.waitFor(() => expect(getOps().map((o) => o.key)).toEqual(['k1']), { timeout: 5000 })
})

it('another tab announcing a change refreshes this one', async () => {
  await loadPendingWrites('A')
  const heard = new Promise<void>((resolve) => { subscribe(resolve) })
  await dbRequest(STORE_PENDING_WRITES, 'readwrite', (s) => s.put({ ...op('theirs', 1), userId: 'A' }))
  const otherTab = new BroadcastChannel('econumo-pending-writes')
  otherTab.postMessage('changed')
  await heard
  otherTab.close()
  expect(getOps().map((o) => o.key)).toEqual(['theirs'])
})
