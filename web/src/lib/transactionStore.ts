import type { UserDto } from '@/api/dto/user'
import type { StoredTransaction } from '@/api/dto/transaction'
import { getVersion } from '@/lib/config'
import { STORE_ACCOUNT_TRANSACTIONS, STORE_META, dbTransaction } from '@/lib/db'

export interface Bucket {
  accountId: string
  revision: number
  items: StoredTransaction[]
  authors: UserDto[]
}

interface Meta {
  buster: string
  userId: string
  // absent in a store written before it was kept
  savedAt?: number
}

export interface StoredTransactions {
  userId: string
  savedAt: number | undefined
  buckets: Map<string, Bucket>
}

// Bump when the stored shape changes without a release (dev builds keep the
// same version string).
export const FORMAT_VERSION = 1
const META_KEY = 'state'

export function storeBuster(): string {
  return `${FORMAT_VERSION}:${getVersion()}`
}

// Any IndexedDB failure (private mode, quota, a blocked upgrade) turns the
// store off for the rest of the session; the sync then keeps working from the
// list it holds in memory. IndexedDB can also stall without ever reporting an
// error, so the waits that hold the boot or a sign-in/out (a load, a clear)
// give up after the limit. Giving up is no failure: a database that opens or
// reads slowly stays on, and the writes queued behind it still land.
export const STORE_TIMEOUT_MS = 1500

let disabled = false
let warned = false
// Moved by every clear. A sync captures it when it starts, so its writes after
// a logout or a login (another user) are dropped instead of restoring the
// previous user's data.
let epoch = 0
// A load that ran out of time is still queued in the database; a second one
// would only wait out another full limit behind it.
let stalled = false

export function storeEpoch(): number {
  return epoch
}

function isStale(at: number | undefined): boolean {
  return at !== undefined && at !== epoch
}

const TIMED_OUT = Symbol('timed out')

function within<T>(run: Promise<T>): Promise<T | typeof TIMED_OUT> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => resolve(TIMED_OUT), STORE_TIMEOUT_MS)
    run.then(
      (value) => { clearTimeout(timer); resolve(value) },
      (e: unknown) => { clearTimeout(timer); reject(e) },
    )
  })
}

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
      console.warn('Transaction store unavailable; transactions are kept in memory only', e)
    }
    return fallback
  }
}

function currentMeta(userId: string): Meta {
  return { buster: storeBuster(), userId }
}

function isOwnedBy(stored: Meta | undefined, userId: string): boolean {
  const wanted = currentMeta(userId)
  return stored?.buster === wanted.buster && stored.userId === wanted.userId
}

// Inside a readwrite transaction: drops whatever another user or an older
// build left behind, then runs `next`.
function adopt(t: IDBTransaction, userId: string, next: (store: IDBObjectStore) => void = () => {}): void {
  const meta = t.objectStore(STORE_META)
  const store = t.objectStore(STORE_ACCOUNT_TRANSACTIONS)
  const read = meta.get(META_KEY)
  read.onsuccess = () => {
    if (!isOwnedBy(read.result as Meta | undefined, userId)) {
      store.clear()
      meta.put(currentMeta(userId), META_KEY)
    }
    next(store)
  }
}

export async function loadBuckets(userId: string, at?: number): Promise<Map<string, Bucket>> {
  return (await tryLoadBuckets(userId, at)) ?? new Map()
}

async function readBuckets(userId: string, at: number | undefined): Promise<Map<string, Bucket>> {
  const buckets = new Map<string, Bucket>()
  let foreign = false
  // readonly: loads never queue behind each other, only behind writes
  await dbTransaction([STORE_ACCOUNT_TRANSACTIONS, STORE_META], 'readonly', (t) => {
    if (isStale(at)) {
      return
    }
    const read = t.objectStore(STORE_META).get(META_KEY)
    read.onsuccess = () => {
      if (!isOwnedBy(read.result as Meta | undefined, userId)) {
        foreign = true
        return
      }
      const all = t.objectStore(STORE_ACCOUNT_TRANSACTIONS).getAll()
      all.onsuccess = () => {
        for (const bucket of all.result as Bucket[]) {
          buckets.set(bucket.accountId, bucket)
        }
      }
    }
  })
  if (foreign && !isStale(at)) {
    // Not awaited: a write the caller starts later is created after this one
    // and runs after it.
    void guarded(undefined, () => dbTransaction([STORE_ACCOUNT_TRANSACTIONS, STORE_META], 'readwrite', (t) => adopt(t, userId)))
  }
  return buckets
}

// Null when the store did not answer in time.
async function timedLoad<T>(fallback: T, read: () => Promise<T>): Promise<T | null> {
  if (stalled) {
    return null
  }
  const load = guarded(fallback, read)
  const result = await within(load)
  if (result !== TIMED_OUT) {
    return result
  }
  stalled = true
  void load.then(() => { stalled = false })
  return null
}

/**
 * Null when the store did not answer in time: what is on disk is then
 * unknown. `at`: the storeEpoch() its caller started from; a clear since then
 * makes this a no-op.
 */
export async function tryLoadBuckets(userId: string, at?: number): Promise<Map<string, Bucket> | null> {
  if (isStale(at)) {
    return new Map()
  }
  return timedLoad(new Map<string, Bucket>(), () => readBuckets(userId, at))
}

async function readStored(): Promise<StoredTransactions | null> {
  let out: StoredTransactions | null = null
  await dbTransaction([STORE_ACCOUNT_TRANSACTIONS, STORE_META], 'readonly', (t) => {
    const read = t.objectStore(STORE_META).get(META_KEY)
    read.onsuccess = () => {
      const meta = read.result as Meta | undefined
      if (!meta || meta.buster !== storeBuster()) {
        return
      }
      const all = t.objectStore(STORE_ACCOUNT_TRANSACTIONS).getAll()
      all.onsuccess = () => {
        const buckets = new Map((all.result as Bucket[]).map((b) => [b.accountId, b]))
        out = { userId: meta.userId, savedAt: meta.savedAt, buckets }
      }
    }
  })
  return out
}

/**
 * Whatever this build stored, for the boot restore, which knows no user yet:
 * the caller checks the owner. Never wipes; another build's data reads as
 * null and is dropped by the next sync. Null also when the store is empty or
 * did not answer in time.
 */
export async function tryLoadStored(): Promise<StoredTransactions | null> {
  return timedLoad<StoredTransactions | null>(null, readStored)
}

export function putBuckets(userId: string, buckets: Bucket[], at?: number): Promise<void> {
  if (buckets.length === 0 || isStale(at)) {
    return Promise.resolve()
  }
  return guarded(undefined, () => dbTransaction([STORE_ACCOUNT_TRANSACTIONS, STORE_META], 'readwrite', (t) => {
    // checked again when the transaction starts: a clear may have run in between
    if (isStale(at)) {
      return
    }
    // a write before the first load must not adopt buckets another user or an older build left behind
    adopt(t, userId, (store) => {
      for (const bucket of buckets) {
        store.put(bucket)
      }
      t.objectStore(STORE_META).put({ ...currentMeta(userId), savedAt: Date.now() }, META_KEY)
    })
  }))
}

export function deleteBuckets(accountIds: string[], at?: number): Promise<void> {
  if (accountIds.length === 0 || isStale(at)) {
    return Promise.resolve()
  }
  return guarded(undefined, () => dbTransaction([STORE_ACCOUNT_TRANSACTIONS], 'readwrite', (t) => {
    if (isStale(at)) {
      return
    }
    const store = t.objectStore(STORE_ACCOUNT_TRANSACTIONS)
    for (const id of accountIds) {
      store.delete(id)
    }
  }))
}

// Attempted even while the store is disabled: a logout must not leave data on disk.
// After the time limit the caller (logout, sign-in) goes on; the clear still
// runs once the database opens, before any write started after it.
export async function clearTransactions(): Promise<void> {
  epoch++
  await within(guarded(undefined, () => dbTransaction([STORE_ACCOUNT_TRANSACTIONS, STORE_META], 'readwrite', (t) => {
    t.objectStore(STORE_ACCOUNT_TRANSACTIONS).clear()
    t.objectStore(STORE_META).clear()
  }), true))
}

export function resetTransactionStoreForTests(): void {
  disabled = false
  warned = false
  stalled = false
}
