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
}

// Bump when the stored shape changes without a release (dev builds keep the
// same version string).
export const FORMAT_VERSION = 1
const META_KEY = 'state'

export function storeBuster(): string {
  return `${FORMAT_VERSION}:${getVersion()}`
}

// Any IndexedDB failure (private mode, quota, a blocked upgrade) turns the
// store off for the rest of the session: sync then simply downloads
// everything, which is the behaviour without a store at all. An operation
// that never settles counts as a failure: the boot restore and the logout
// wait on this store, and IndexedDB can stall without ever reporting an error.
export const STORE_TIMEOUT_MS = 1500

let disabled = false
let warned = false
// Moved by every clear. A sync captures it when it starts, so its writes after
// a logout or a login (another user) are dropped instead of restoring the
// previous user's data.
let epoch = 0

export function storeEpoch(): number {
  return epoch
}

function isStale(at: number | undefined): boolean {
  return at !== undefined && at !== epoch
}

function withTimeout<T>(run: Promise<T>): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('IndexedDB did not answer in time')), STORE_TIMEOUT_MS)
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
    return await withTimeout(run())
  } catch (e) {
    disabled = true
    if (!warned) {
      warned = true
      console.warn('Transaction store unavailable; transactions will be downloaded in full', e)
    }
    return fallback
  }
}

function currentMeta(userId: string): Meta {
  return { buster: storeBuster(), userId }
}

/** `at`: the storeEpoch() its caller started from; a clear since then makes this a no-op. */
export function loadBuckets(userId: string, at?: number): Promise<Map<string, Bucket>> {
  if (isStale(at)) {
    return Promise.resolve(new Map())
  }
  return guarded(new Map<string, Bucket>(), async () => {
    const buckets = new Map<string, Bucket>()
    await dbTransaction([STORE_ACCOUNT_TRANSACTIONS, STORE_META], 'readwrite', (t) => {
      if (isStale(at)) {
        return
      }
      const meta = t.objectStore(STORE_META)
      const store = t.objectStore(STORE_ACCOUNT_TRANSACTIONS)
      const wanted = currentMeta(userId)
      const read = meta.get(META_KEY)
      read.onsuccess = () => {
        const stored = read.result as Meta | undefined
        if (stored?.buster === wanted.buster && stored.userId === wanted.userId) {
          const all = store.getAll()
          all.onsuccess = () => {
            for (const bucket of all.result as Bucket[]) {
              buckets.set(bucket.accountId, bucket)
            }
          }
          return
        }
        store.clear()
        meta.put(wanted, META_KEY)
      }
    })
    return buckets
  })
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
    const meta = t.objectStore(STORE_META)
    const store = t.objectStore(STORE_ACCOUNT_TRANSACTIONS)
    const wanted = currentMeta(userId)
    const read = meta.get(META_KEY)
    read.onsuccess = () => {
      const stored = read.result as Meta | undefined
      // a write before the first load must not adopt buckets another user or an older build left behind
      if (stored?.buster !== wanted.buster || stored.userId !== wanted.userId) {
        store.clear()
        meta.put(wanted, META_KEY)
      }
      for (const bucket of buckets) {
        store.put(bucket)
      }
    }
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
export function clearTransactions(): Promise<void> {
  epoch++
  return guarded(undefined, () => dbTransaction([STORE_ACCOUNT_TRANSACTIONS, STORE_META], 'readwrite', (t) => {
    t.objectStore(STORE_ACCOUNT_TRANSACTIONS).clear()
    t.objectStore(STORE_META).clear()
  }), true)
}

export function resetTransactionStoreForTests(): void {
  disabled = false
  warned = false
}
