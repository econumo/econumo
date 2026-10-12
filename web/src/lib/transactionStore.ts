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
// everything, which is the behaviour without a store at all.
let disabled = false
let warned = false

async function guarded<T>(fallback: T, run: () => Promise<T>): Promise<T> {
  if (disabled) {
    return fallback
  }
  try {
    return await run()
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

export function loadBuckets(userId: string): Promise<Map<string, Bucket>> {
  return guarded(new Map<string, Bucket>(), async () => {
    const buckets = new Map<string, Bucket>()
    await dbTransaction([STORE_ACCOUNT_TRANSACTIONS, STORE_META], 'readwrite', (t) => {
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

export function putBuckets(userId: string, buckets: Bucket[]): Promise<void> {
  if (buckets.length === 0) {
    return Promise.resolve()
  }
  return guarded(undefined, () => dbTransaction([STORE_ACCOUNT_TRANSACTIONS, STORE_META], 'readwrite', (t) => {
    const store = t.objectStore(STORE_ACCOUNT_TRANSACTIONS)
    for (const bucket of buckets) {
      store.put(bucket)
    }
    t.objectStore(STORE_META).put(currentMeta(userId), META_KEY)
  }))
}

export function deleteBuckets(accountIds: string[]): Promise<void> {
  if (accountIds.length === 0) {
    return Promise.resolve()
  }
  return guarded(undefined, () => dbTransaction([STORE_ACCOUNT_TRANSACTIONS], 'readwrite', (t) => {
    const store = t.objectStore(STORE_ACCOUNT_TRANSACTIONS)
    for (const id of accountIds) {
      store.delete(id)
    }
  }))
}

export function clearTransactions(): Promise<void> {
  return guarded(undefined, () => dbTransaction([STORE_ACCOUNT_TRANSACTIONS, STORE_META], 'readwrite', (t) => {
    t.objectStore(STORE_ACCOUNT_TRANSACTIONS).clear()
    t.objectStore(STORE_META).clear()
  }))
}

export function resetTransactionStoreForTests(): void {
  disabled = false
  warned = false
}
