import { STORE_PENDING_WRITES, dbRequest } from '@/lib/db'
import { STORE_TIMEOUT_MS } from '@/lib/transactionStore'
import { delayTransactionCompletion } from '@/test/idbDelay'
import { clearPendingWrites, getOps, isQueueLoaded, loadPendingWrites, mutate, resetPendingWritesForTests, subscribe, subscribeAdopted, subscribeLeftElsewhere } from './store'
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

describe('a change made before the stored rows arrive', () => {
  const tx = (key: string, seq: number, kind: 'tx-create' | 'tx-update', amount: string, over: Partial<PendingOp> = {}) =>
    op(key, seq, { kind, txId: 't1', payload: { id: 't1', amount }, ...over } as unknown as Partial<PendingOp>)

  async function storedThen(stored: PendingOp) {
    await loadPendingWrites('A')
    await mutate(() => [stored])
    resetPendingWritesForTests()
  }

  it('folds into a stored change on its target that was never sent', async () => {
    await storedThen(tx('stored', 5, 'tx-update', '1'))
    await mutate(() => [tx('early', 1, 'tx-update', '9')])
    await loadPendingWrites('A')
    expect(getOps()).toMatchObject([{ key: 'stored', seq: 5, payload: { amount: '9' } }])
    expect(await rows()).toMatchObject([{ key: 'stored', payload: { amount: '9' } }])
  })

  it('stays behind a stored change that may have reached the server', async () => {
    await storedThen(tx('stored', 5, 'tx-update', '1', { attempted: true }))
    await mutate(() => [tx('early', 1, 'tx-update', '9')])
    await loadPendingWrites('A')
    expect([...getOps()].sort((a, b) => a.seq - b.seq)).toMatchObject([
      { key: 'stored', seq: 5, payload: { amount: '1' } },
      { key: 'early', seq: 6, payload: { amount: '9' } },
    ])
  })

  it('becomes an edit behind a stored add of the same form, never a second add', async () => {
    await storedThen(tx('stored', 5, 'tx-create', '1', { attempted: true }))
    await mutate(() => [tx('early', 1, 'tx-create', '9')])
    await loadPendingWrites('A')
    expect([...getOps()].sort((a, b) => a.seq - b.seq)).toMatchObject([
      { key: 'stored', kind: 'tx-create', payload: { amount: '1' } },
      { key: 'early', kind: 'tx-update', payload: { amount: '9' } },
    ])
  })

  it('drops from the disk once folded away', async () => {
    await storedThen(tx('stored', 5, 'tx-update', '1'))
    const restore = delayTransactionCompletion(200)
    const loading = loadPendingWrites('A')
    await mutate(() => [tx('early', 1, 'tx-update', '9')])
    await loading
    restore()
    await vi.waitFor(async () => expect((await rows()).map((r) => r.key)).toEqual(['stored']))
  })
})

describe('isQueueLoaded', () => {
  it('is false until the stored rows are in memory, and only for their owner', async () => {
    await loadPendingWrites('A')
    await mutate(() => [op('k1', 1)])
    resetPendingWritesForTests()

    expect(isQueueLoaded('A')).toBe(false)
    const restore = delayTransactionCompletion(STORE_TIMEOUT_MS + 300)
    await loadPendingWrites('A')
    expect(isQueueLoaded('A')).toBe(false)
    restore()
    await vi.waitFor(() => expect(isQueueLoaded('A')).toBe(true), { timeout: 5000 })
    expect(isQueueLoaded('B')).toBe(false)
  })

  it('is true at the moment the stored rows are announced', async () => {
    let seen: boolean | undefined
    subscribeAdopted(() => { seen = isQueueLoaded('A') })
    await loadPendingWrites('A')
    expect(seen).toBe(true)
  })

  it('counts a database that broke as loaded, and says so: its rows will never come', async () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    await loadPendingWrites('A')
    resetPendingWritesForTests()
    const heard = vi.fn()
    subscribeAdopted(heard)
    const failing = vi.spyOn(IDBDatabase.prototype, 'transaction').mockImplementation(() => {
      throw new DOMException('broken', 'InvalidStateError')
    })
    await loadPendingWrites('A')
    failing.mockRestore()
    expect(isQueueLoaded('A')).toBe(true)
    expect(heard).toHaveBeenCalledTimes(1)
  })

  it('is false again after a clear', async () => {
    await loadPendingWrites('A')
    await clearPendingWrites()
    expect(isQueueLoaded('A')).toBe(false)
  })
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

it('says when stored rows land in memory, even long after the load stopped being waited for', async () => {
  await loadPendingWrites('A')
  await mutate(() => [op('k1', 1)])
  resetPendingWritesForTests()

  const adopted = vi.fn()
  subscribeAdopted(adopted)
  delayTransactionCompletion(STORE_TIMEOUT_MS + 300)
  await loadPendingWrites('A')
  expect(adopted).not.toHaveBeenCalled()
  await vi.waitFor(() => expect(adopted).toHaveBeenCalledTimes(1), { timeout: 5000 })
  expect(getOps().map((o) => o.key)).toEqual(['k1'])
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

it('says when a change left in another tab, and not when it left here or one arrived', async () => {
  await loadPendingWrites('A')
  await mutate(() => [op('local', 1), op('theirs', 2)])
  const left = vi.fn()
  subscribeLeftElsewhere(left)
  await mutate((ops) => ops.filter((o) => o.key !== 'local'))
  expect(left).not.toHaveBeenCalled()

  const otherTab = new BroadcastChannel('econumo-pending-writes')
  const refreshed = () => new Promise<void>((resolve) => { const off = subscribe(() => { off(); resolve() }) })
  await dbRequest(STORE_PENDING_WRITES, 'readwrite', (s) => s.put({ ...op('new', 3), userId: 'A' }))
  let heard = refreshed()
  otherTab.postMessage('changed')
  await heard
  expect(left).not.toHaveBeenCalled()

  await dbRequest(STORE_PENDING_WRITES, 'readwrite', (s) => s.delete('theirs'))
  heard = refreshed()
  otherTab.postMessage('changed')
  await heard
  otherTab.close()
  expect(left).toHaveBeenCalledTimes(1)
  expect(getOps().map((o) => o.key)).toEqual(['new'])
})
