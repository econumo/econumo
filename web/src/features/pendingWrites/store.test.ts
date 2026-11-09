import { STORE_PENDING_WRITES, dbRequest } from '@/lib/db'
import { STORE_TIMEOUT_MS } from '@/lib/transactionStore'
import { delayTransactionCompletion } from '@/test/idbDelay'
import { addChange } from './queue'
import { claimForSend, clearPendingWrites, getOps, isQueueLoaded, loadPendingWrites, mutate, resetPendingWritesForTests, subscribe, subscribeAdopted, subscribeLeftElsewhere } from './store'
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

describe('two tabs: the database decides what a sender took', () => {
  const tx = (key: string, seq: number, kind: 'tx-create' | 'tx-update', amount: string, over: Partial<PendingOp> = {}) =>
    op(key, seq, { kind, txId: 't1', payload: { id: 't1', amount }, ...over } as unknown as Partial<PendingOp>)
  const edit = (amount: string) => ({ kind: 'tx-update', txId: 't1', payload: { id: 't1', amount } }) as unknown as Parameters<typeof addChange>[1]
  const ordered = async () => (await rows()).sort((a, b) => a.seq - b.seq)
  const storeRow = (row: PendingOp) => dbRequest(STORE_PENDING_WRITES, 'readwrite', (s) => s.put({ ...row, userId: 'A' }))
  const held = async (row: PendingOp) => {
    await loadPendingWrites('A')
    await mutate(() => [row])
  }

  describe('claiming a row to send it', () => {
    it('marks it attempted on the disk and returns the row stored there, not the copy in memory', async () => {
      await held(tx('k1', 1, 'tx-update', '1'))
      await storeRow(tx('k1', 1, 'tx-update', '9'))
      const claimed = await claimForSend('k1')
      expect(claimed).toMatchObject({ key: 'k1', attempted: true, payload: { amount: '9' } })
      expect(await rows()).toMatchObject([{ key: 'k1', attempted: true, payload: { amount: '9' } }])
      expect(getOps()).toMatchObject([{ key: 'k1', attempted: true, payload: { amount: '9' } }])
    })

    it('marks memory at once, so a fold in this tab queues behind it as before', async () => {
      await held(tx('k1', 1, 'tx-update', '1'))
      const claiming = claimForSend('k1')
      expect(getOps()).toMatchObject([{ key: 'k1', attempted: true }])
      await claiming
    })

    it('gives nothing for a row another tab settled or a refusal it recorded', async () => {
      await held(tx('k1', 1, 'tx-update', '1'))
      await dbRequest(STORE_PENDING_WRITES, 'readwrite', (s) => s.delete('k1'))
      expect(await claimForSend('k1')).toBeNull()

      await mutate(() => [tx('k2', 2, 'tx-update', '1')])
      await storeRow(tx('k2', 2, 'tx-update', '1', { status: 'failed', error: 'No' }))
      expect(await claimForSend('k2')).toBeNull()
    })

    it('falls back on memory when the database is broken or stalled', async () => {
      vi.spyOn(console, 'warn').mockImplementation(() => {})
      await held(tx('k1', 1, 'tx-update', '1'))
      const restore = delayTransactionCompletion(null)
      await expect(claimForSend('k1')).resolves.toMatchObject({ key: 'k1', attempted: true })
      restore()
      const failing = vi.spyOn(IDBDatabase.prototype, 'transaction').mockImplementation(() => {
        throw new DOMException('broken', 'InvalidStateError')
      })
      await mutate((ops) => [...ops, tx('k2', 2, 'tx-update', '1')])
      await expect(claimForSend('k2')).resolves.toMatchObject({ key: 'k2', attempted: true })
      failing.mockRestore()
    })
  })

  describe('a tab folding into a row a sender already took', () => {
    const fold = (amount: string) => mutate((ops) => addChange(ops, edit(amount), { key: 'unused', seq: 99 }))

    it('leaves the sent row alone and lands the newer values behind it, under a key of their own', async () => {
      await held(tx('k1', 1, 'tx-update', '1'))
      await storeRow(tx('k1', 1, 'tx-update', '1', { attempted: true }))
      await fold('9')
      const stored = await ordered()
      expect(stored).toMatchObject([
        { key: 'k1', attempted: true, payload: { amount: '1' } },
        { kind: 'tx-update', attempted: false, status: 'waiting', payload: { amount: '9' }, seq: 2 },
      ])
      expect(stored[1].key).not.toBe('k1')
      expect(getOps().map((o) => o.key).sort()).toEqual(stored.map((r) => r.key).sort())
    })

    it('makes the newer values an edit when the sent row is an add', async () => {
      await held(tx('k1', 1, 'tx-create', '1'))
      await storeRow(tx('k1', 1, 'tx-create', '1', { attempted: true }))
      await fold('9')
      expect(await ordered()).toMatchObject([
        { key: 'k1', kind: 'tx-create', payload: { amount: '1' } },
        { kind: 'tx-update', payload: { amount: '9' } },
      ])
    })

    it('deleting an add a sender already took queues a delete behind it', async () => {
      await held(tx('k1', 1, 'tx-create', '1'))
      await storeRow(tx('k1', 1, 'tx-create', '1', { attempted: true }))
      await mutate((ops) => addChange(ops, { kind: 'tx-delete', txId: 't1', previous: null }, { key: 'unused', seq: 99 }))
      expect(await ordered()).toMatchObject([
        { key: 'k1', kind: 'tx-create', attempted: true },
        { kind: 'tx-delete', txId: 't1', attempted: false },
      ])
      expect(getOps()).toHaveLength(2)
    })

    it('adds nothing when the write changes no value', async () => {
      await held(tx('k1', 1, 'tx-update', '1', { status: 'failed', error: 'No' }))
      await storeRow(tx('k1', 1, 'tx-update', '1', { attempted: true, status: 'waiting' }))
      await mutate((ops) => ops.map((o) => ({ ...o, status: 'waiting' as const })))
      expect(await ordered()).toMatchObject([{ key: 'k1', attempted: true }])
      expect(await rows()).toHaveLength(1)
    })

    it('lets the holder settle a refusal of the row it sent', async () => {
      await held(tx('k1', 1, 'tx-update', '1', { attempted: true }))
      await mutate((ops) => ops.map((o) => ({ ...o, attempted: false, status: 'failed' as const, error: 'No' })))
      expect(await rows()).toMatchObject([{ key: 'k1', attempted: false, status: 'failed' }])
    })

    it('is unchanged for one tab: a fold into an unsent row replaces it', async () => {
      await held(tx('k1', 1, 'tx-update', '1'))
      await fold('9')
      expect(await rows()).toMatchObject([{ key: 'k1', attempted: false, payload: { amount: '9' } }])
    })
  })

  describe('with a second copy of the store, as a second tab has', () => {
    async function secondTab() {
      vi.resetModules()
      const other = await import('./store')
      await other.loadPendingWrites('A')
      return other
    }

    it('a claim that ran first keeps its payload and the fold lands behind it', async () => {
      await held(tx('k1', 1, 'tx-update', '1'))
      const b = await secondTab()
      const claimed = await claimForSend('k1')
      expect(claimed).toMatchObject({ payload: { amount: '1' } })

      await b.mutate((ops) => addChange(ops, edit('9'), { key: 'unused', seq: 99 }))
      expect(await ordered()).toMatchObject([
        { key: 'k1', attempted: true, payload: { amount: '1' } },
        { attempted: false, payload: { amount: '9' } },
      ])
      b.resetPendingWritesForTests()
    })

    it('a fold that ran first is what the claim sends', async () => {
      await held(tx('k1', 1, 'tx-update', '1'))
      const b = await secondTab()
      await b.mutate((ops) => addChange(ops, edit('9'), { key: 'unused', seq: 99 }))
      await expect(claimForSend('k1')).resolves.toMatchObject({ attempted: true, payload: { amount: '9' } })
      expect(await rows()).toHaveLength(1)
      b.resetPendingWritesForTests()
    })
  })
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

it('a read-back mark another tab stored does not describe the caches of this one', async () => {
  await loadPendingWrites('A')
  await mutate(() => [op('mine', 1, { attempted: true, landed: { realId: 'r1' } }), op('theirs', 2, { attempted: true })])
  await dbRequest(STORE_PENDING_WRITES, 'readwrite', (s) => s.put({ ...op('theirs', 2, { attempted: true, landed: { realId: 'r2' } }), userId: 'A' }))
  const heard = new Promise<void>((resolve) => { const off = subscribe(() => { off(); resolve() }) })
  const otherTab = new BroadcastChannel('econumo-pending-writes')
  otherTab.postMessage('changed')
  await heard
  otherTab.close()
  expect(getOps().find((o) => o.key === 'mine')?.landed).toEqual({ realId: 'r1' })
  expect(getOps().find((o) => o.key === 'theirs')?.landed).toBeUndefined()
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
