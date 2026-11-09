import { STORE_PENDING_WRITES, dbTransaction } from '@/lib/db'
import { STORE_TIMEOUT_MS } from '@/lib/transactionStore'
import { replayBehind } from './queue'
import type { PendingOp } from './types'

type Row = PendingOp & { userId: string }

const CHANNEL_NAME = 'econumo-pending-writes'

let ops: PendingOp[] = []
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

async function refreshFromDisk(): Promise<void> {
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
    const next = rows.filter((r) => r.userId === wanted).map(toOp)
    const kept = new Set(next.map((op) => op.key))
    const left = ops.some((op) => !kept.has(op.key))
    ops = next
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

function persist(before: PendingOp[], after: PendingOp[]): Promise<void> {
  const userId = owner
  const previous = new Map(before.map((op) => [op.key, op]))
  const kept = new Set(after.map((op) => op.key))
  const changed = after.filter((op) => previous.get(op.key) !== op)
  const removed = before.filter((op) => !kept.has(op.key))
  if (!userId || disabled || changed.length + removed.length === 0) {
    return Promise.resolve()
  }
  const write = guarded(undefined, () => dbTransaction([STORE_PENDING_WRITES], 'readwrite', (t) => {
    const store = t.objectStore(STORE_PENDING_WRITES)
    for (const op of changed) {
      store.put({ ...op, userId })
    }
    for (const op of removed) {
      store.delete(op.key)
    }
  })).then(announce)
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

// Attempted even while the store is disabled: a sign-out must not leave data on disk.
export async function clearPendingWrites(): Promise<void> {
  epoch++
  generation++
  owner = null
  loaded = false
  loading = null
  ops = []
  notify()
  await timed(guarded(undefined, () => dbTransaction([STORE_PENDING_WRITES], 'readwrite', (t) => {
    t.objectStore(STORE_PENDING_WRITES).clear()
  }), true).then(announce))
}

export function resetPendingWritesForTests(): void {
  channel?.close()
  channel = null
  ops = []
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
