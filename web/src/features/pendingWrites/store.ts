import { v7 as uuidv7 } from 'uuid'
import { STORE_PENDING_WRITES, dbTransaction } from '@/lib/db'
import { STORE_TIMEOUT_MS } from '@/lib/transactionStore'
import { changeBehindSent, markAttempted, nextSeq, replayBehind } from './queue'
import { realIdOf } from './tempIds'
import type { OpChange, PendingOp } from './types'

type Row = PendingOp & { userId: string }

const CHANNEL_NAME = 'econumo-pending-writes'

let ops: PendingOp[] = []
// Keys this page has read from the disk or written there. A key missing from the
// disk was settled in another tab only if it was ever there: a change queued
// before the owner was known simply has not been written yet.
let storedKeys = new Set<string>()
let owner: string | null = null
// The owner's stored rows are in memory, or will never come (a broken database).
let loaded = false
let disabled = false
let warned = false
// Moved by a clear and by a change of owner: a load already running must not land.
let epoch = 0
// Moved by every local change: a refresh that read before it read too early.
let generation = 0
let loading: Promise<void> | null = null
let channel: BroadcastChannel | null = null
const listeners = new Set<() => void>()
const leftElsewhereListeners = new Set<() => void>()
const adoptedListeners = new Set<() => void>()

function notify(): void {
  for (const listener of listeners) {
    listener()
  }
}

// IndexedDB can stall without ever reporting an error. Nothing that waits on it
// here (the boot, a send) may wait longer; the work stays queued and still lands.
function timed(run: Promise<unknown>): Promise<void> {
  return new Promise((resolve) => {
    const timer = setTimeout(resolve, STORE_TIMEOUT_MS)
    void run.finally(() => {
      clearTimeout(timer)
      resolve()
    })
  })
}

// A broken database turns persistence off for the session and leaves the queue
// in memory. Only the error's name is logged: the rows hold a user's finances.
async function guarded<T>(fallback: T, run: () => Promise<T>, force = false): Promise<T> {
  if (disabled && !force) {
    return fallback
  }
  try {
    return await run()
  } catch (e) {
    disabled = true
    if (!warned) {
      warned = true
      console.warn('Pending writes store unavailable; changes are kept in memory only', (e as Error | undefined)?.name)
    }
    return fallback
  }
}

function toOp({ userId: _userId, ...op }: Row): PendingOp {
  return op as PendingOp
}

// A row read from another tab's write: its mark says what that tab's caches
// hold, and this tab's caches only count the change if this tab marked it.
function withLandedMark({ landed: _stored, ...op }: PendingOp, mine: PendingOp['landed']): PendingOp {
  return (mine ? { ...op, landed: mine } : op) as PendingOp
}

async function readRows(): Promise<Row[]> {
  let rows: Row[] = []
  await dbTransaction([STORE_PENDING_WRITES], 'readonly', (t) => {
    const all = t.objectStore(STORE_PENDING_WRITES).getAll()
    all.onsuccess = () => {
      rows = all.result as Row[]
    }
  })
  return rows
}

/** Takes the disk's word for the queue: what another tab wrote, or a write here the disk turned down. */
export async function refreshFromDisk(): Promise<void> {
  const wanted = owner
  if (!wanted || disabled) {
    return
  }
  let rows: Row[] | null
  let seen: number
  do {
    seen = generation
    rows = await guarded<Row[] | null>(null, readRows)
  } while (rows && seen !== generation && owner === wanted)
  if (rows && owner === wanted) {
    const landed = new Map(ops.map((op) => [op.key, op.landed]))
    const next = rows.filter((r) => r.userId === wanted).map(toOp).map((op) => withLandedMark(op, landed.get(op.key)))
    const kept = new Set(next.map((op) => op.key))
    const left = ops.some((op) => !kept.has(op.key))
    ops = next
    storedKeys = kept
    notify()
    if (left) {
      for (const listener of leftElsewhereListeners) {
        listener()
      }
    }
  }
}

function openChannel(): BroadcastChannel | null {
  if (!channel && typeof BroadcastChannel !== 'undefined') {
    channel = new BroadcastChannel(CHANNEL_NAME)
    channel.onmessage = () => { void refreshFromDisk() }
  }
  return channel
}

function announce(): void {
  openChannel()?.postMessage('changed')
}

// Everything below runs inside one readwrite transaction, and those serialize
// across tabs: the row read here is the latest, and nothing lands between the
// read and the write. A row a tab still took for unsent may have been taken by
// another tab's sender since; overwritten, the newer values would be lost to a
// replay of its key, so they queue behind it. A row gone from the disk was settled
// over there: its key is spent, and rewritten it would be answered as a replay, so
// the newer values take a fresh one.
function underRealId(change: OpChange): OpChange {
  if (change.kind === 'limit') {
    return change
  }
  const txId = realIdOf(change.txId)
  return change.kind === 'tx-delete' ? { ...change, txId } : { ...change, txId, payload: { ...change.payload, id: txId } }
}

function writeRows(store: IDBObjectStore, stored: Row[], changed: PendingOp[], removed: PendingOp[], previous: Map<string, PendingOp>, userId: string): boolean {
  const rows = new Map(stored.map((row) => [row.key, row]))
  let seq = nextSeq([...stored, ...changed])
  let turnedDown = false
  const takenElsewhere = (written: PendingOp | undefined, before: PendingOp | undefined, row: Row | undefined): row is Row =>
    Boolean(row?.attempted && before && !before.attempted && !written?.attempted)
  const settledElsewhere = (before: PendingOp | undefined, row: Row | undefined): before is PendingOp =>
    Boolean(before && !row && storedKeys.has(before.key))
  const queueBehind = (change: OpChange | null) => {
    if (change) {
      store.put({ ...underRealId(change), key: uuidv7(), seq: seq++, attempted: false, status: 'waiting', retries: 0, notBefore: 0, userId })
    }
  }
  for (const op of changed) {
    const row = rows.get(op.key)
    const before = previous.get(op.key)
    if (takenElsewhere(op, before, row)) {
      turnedDown = true
      queueBehind(changeBehindSent(toOp(row), op, before as PendingOp))
    } else if (settledElsewhere(before, row)) {
      turnedDown = true
      storedKeys.delete(op.key)
      if (!before.attempted) {
        queueBehind(changeBehindSent(before, op, before))
      }
    } else {
      store.put({ ...op, userId })
      storedKeys.add(op.key)
    }
  }
  for (const op of removed) {
    const row = rows.get(op.key)
    if (takenElsewhere(undefined, op, row)) {
      turnedDown = true
      queueBehind(changeBehindSent(toOp(row), undefined, op))
    } else {
      // an add cancelled here that the other tab had already sent still needs its delete
      if (settledElsewhere(op, row) && !op.attempted) {
        const behind = changeBehindSent(op, undefined, op)
        turnedDown ||= behind !== null
        queueBehind(behind)
      }
      store.delete(op.key)
      storedKeys.delete(op.key)
    }
  }
  return turnedDown
}

function persist(before: PendingOp[], after: PendingOp[]): Promise<void> {
  const userId = owner
  const previous = new Map(before.map((op) => [op.key, op]))
  const kept = new Set(after.map((op) => op.key))
  const changed = after.filter((op) => previous.get(op.key) !== op)
  const removed = before.filter((op) => !kept.has(op.key))
  if (!userId || disabled || changed.length + removed.length === 0) {
    return Promise.resolve()
  }
  let turnedDown = false
  const write = guarded(undefined, () => dbTransaction([STORE_PENDING_WRITES], 'readwrite', (t) => {
    const store = t.objectStore(STORE_PENDING_WRITES)
    const all = store.getAll()
    all.onsuccess = () => {
      turnedDown = writeRows(store, all.result as Row[], changed, removed, previous, userId)
    }
  })).then(() => (turnedDown ? refreshFromDisk() : undefined)).then(announce)
  return timed(write)
}

function announceLoaded(): void {
  loaded = true
  for (const listener of adoptedListeners) {
    listener()
  }
}

// What this page queued before the stored rows arrived goes behind them, and
// reaches the disk now that the owner is known.
function adopt(stored: PendingOp[]): void {
  const inMemory = new Map(ops.map((op) => [op.key, op]))
  const onDisk = new Set(stored.map((op) => op.key))
  storedKeys = new Set(onDisk)
  const restored = stored.map((op) => inMemory.get(op.key) ?? op)
  const local = ops.filter((op) => !onDisk.has(op.key))
  ops = replayBehind(restored, local)
  generation++
  notify()
  void persist([...restored, ...local], ops)
  announceLoaded()
}

function startLoad(userId: string): Promise<void> {
  const at = epoch
  return guarded(undefined, async () => {
    const rows = await readRows()
    const foreign = rows.filter((r) => r.userId !== userId)
    if (foreign.length > 0 && at === epoch) {
      await dbTransaction([STORE_PENDING_WRITES], 'readwrite', (t) => {
        for (const row of foreign) {
          t.objectStore(STORE_PENDING_WRITES).delete(row.key)
        }
      })
    }
    if (at === epoch) {
      adopt(rows.filter((r) => r.userId === userId).map(toOp))
    }
  }).finally(() => {
    if (at === epoch && !loaded) {
      announceLoaded()
    }
  })
}

export function queueOwner(): string | null {
  return owner
}

/** Until this holds, getOps() may be missing older changes that are on their way from the disk. */
export function isQueueLoaded(userId: string): boolean {
  return owner === userId && loaded
}

export function getOps(): PendingOp[] {
  return ops
}

export function subscribe(fn: () => void): () => void {
  listeners.add(fn)
  return () => { listeners.delete(fn) }
}

/** Another tab settled or dropped a change: this tab's caches never saw the outcome. */
export function subscribeLeftElsewhere(fn: () => void): () => void {
  leftElsewhereListeners.add(fn)
  return () => { leftElsewhereListeners.delete(fn) }
}

/** The stored rows reached memory, which can be long after a timed-out load stopped being waited for. */
export function subscribeAdopted(fn: () => void): () => void {
  adoptedListeners.add(fn)
  return () => { adoptedListeners.delete(fn) }
}

export async function loadPendingWrites(userId: string): Promise<PendingOp[]> {
  openChannel()
  if (owner !== userId) {
    if (owner !== null) {
      ops = []
      notify()
    }
    storedKeys = new Set()
    owner = userId
    loaded = false
    epoch++
    loading = null
  }
  if (!loading) {
    const started = startLoad(userId).finally(() => {
      if (loading === started) {
        loading = null
      }
    })
    loading = started
  }
  await timed(loading)
  return ops
}

/** Memory changes at once; the promise settles when the disk has it, or after the time limit. */
export function mutate(fn: (ops: PendingOp[]) => PendingOp[]): Promise<void> {
  const before = ops
  const after = fn(before)
  if (after === before) {
    return Promise.resolve()
  }
  ops = after
  generation++
  notify()
  return persist(before, after)
}

/**
 * Marks a row attempted and returns it as stored, which is what must be sent:
 * another tab may have folded newer values into it since this tab last looked.
 * Null means the row is no longer there to send. With no word from the database
 * (broken, stalled) this tab's copy goes out.
 */
export async function claimForSend(key: string): Promise<PendingOp | null> {
  const local = ops.find((op) => op.key === key)
  if (!local) {
    return null
  }
  // Memory at once: an edit in this tab while the disk answers queues behind it.
  ops = markAttempted(ops, key)
  generation++
  notify()
  const marked = ops.find((op) => op.key === key)!
  const userId = owner
  if (!userId || disabled) {
    return marked
  }
  const at = epoch
  let verdict: Row | null | undefined
  const claimed = guarded<Row | null | undefined>(undefined, async () => {
    let row: Row | null = null
    await dbTransaction([STORE_PENDING_WRITES], 'readwrite', (t) => {
      const store = t.objectStore(STORE_PENDING_WRITES)
      const found = store.get(key)
      found.onsuccess = () => {
        const current = found.result as Row | undefined
        if (current?.userId === userId && current.status === 'waiting') {
          row = { ...current, attempted: true }
          store.put(row)
        }
      }
    })
    return row
  })
  await timed(claimed.then((result) => {
    verdict = result
    if (result) {
      announce()
    }
  }))
  if (verdict === undefined) {
    return marked
  }
  if (verdict === null || at !== epoch) {
    return null
  }
  storedKeys.add(key)
  const taken = withLandedMark(toOp(verdict), ops.find((op) => op.key === key)?.landed)
  ops = ops.map((op) => (op.key === key ? taken : op))
  generation++
  notify()
  return taken
}

// Attempted even while the store is disabled: a sign-out must not leave data on disk.
export async function clearPendingWrites(): Promise<void> {
  epoch++
  generation++
  owner = null
  loaded = false
  loading = null
  ops = []
  storedKeys = new Set()
  notify()
  await timed(guarded(undefined, () => dbTransaction([STORE_PENDING_WRITES], 'readwrite', (t) => {
    t.objectStore(STORE_PENDING_WRITES).clear()
  }), true).then(announce))
}

export function resetPendingWritesForTests(): void {
  channel?.close()
  channel = null
  ops = []
  storedKeys = new Set()
  owner = null
  loaded = false
  disabled = false
  warned = false
  loading = null
  epoch++
  generation++
  listeners.clear()
  leftElsewhereListeners.clear()
  adoptedListeners.clear()
}
