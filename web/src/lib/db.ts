export const STORE_IMPORT_KEYS = 'importKeys'
export const STORE_ACCOUNT_TRANSACTIONS = 'accountTransactions'
export const STORE_META = 'meta'

const DB_NAME = 'econumo'
const DB_VERSION = 1
const LEGACY_DB_NAME = 'econumo-import'
const LEGACY_STORE = 'keys'
const DATA_KEY_RECORD = 'dataKey'

let cached: Promise<IDBDatabase> | null = null

function upgrade(db: IDBDatabase): void {
  db.createObjectStore(STORE_IMPORT_KEYS)
  db.createObjectStore(STORE_ACCOUNT_TRANSACTIONS, { keyPath: 'accountId' })
  db.createObjectStore(STORE_META)
}

function openRaw(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION)
    req.onupgradeneeded = () => upgrade(req.result)
    req.onsuccess = () => {
      const db = req.result
      // Another tab upgrading (or deleting) the database must never wait on this connection.
      db.onversionchange = () => { db.close(); cached = null }
      db.onclose = () => { cached = null }
      resolve(db)
    }
    req.onerror = () => reject(req.error)
  })
}

// The SimpleFIN key used to live in its own database. A CryptoKey survives
// structured clone, so moving it means nobody has to unlock again.
async function migrateLegacyKey(db: IDBDatabase): Promise<void> {
  const legacy = await new Promise<IDBDatabase | null>((resolve, reject) => {
    const req = indexedDB.open(LEGACY_DB_NAME)
    req.onupgradeneeded = () => {
      // it did not exist: undo the creation instead of leaving an empty database behind
      req.transaction?.abort()
      resolve(null)
    }
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => (req.error?.name === 'AbortError' ? resolve(null) : reject(req.error))
  })
  if (!legacy) {
    return
  }
  let record: unknown
  try {
    record = legacy.objectStoreNames.contains(LEGACY_STORE)
      ? await new Promise<unknown>((resolve, reject) => {
        const req = legacy.transaction(LEGACY_STORE, 'readonly').objectStore(LEGACY_STORE).get(DATA_KEY_RECORD)
        req.onsuccess = () => resolve(req.result)
        req.onerror = () => reject(req.error)
      })
      : undefined
  } finally {
    legacy.close()
  }
  if (record !== undefined) {
    await new Promise<void>((resolve, reject) => {
      const t = db.transaction(STORE_IMPORT_KEYS, 'readwrite')
      const store = t.objectStore(STORE_IMPORT_KEYS)
      const existing = store.get(DATA_KEY_RECORD)
      existing.onsuccess = () => {
        if (existing.result === undefined) {
          store.put(record, DATA_KEY_RECORD)
        }
      }
      t.oncomplete = () => resolve()
      t.onerror = () => reject(t.error)
      t.onabort = () => reject(t.error)
    })
  }
  await new Promise<void>((resolve) => {
    const del = indexedDB.deleteDatabase(LEGACY_DB_NAME)
    // blocked: another tab still holds it; the next page load retries
    del.onsuccess = del.onerror = del.onblocked = () => resolve()
  })
}

let migrated = false

export function openDb(): Promise<IDBDatabase> {
  if (cached) {
    return cached
  }
  const opening = openRaw().then(async (db) => {
    if (!migrated) {
      migrated = true
      try {
        await migrateLegacyKey(db)
      } catch (e) {
        console.warn('Could not move the SimpleFIN key into the new database; unlock it again if asked', e)
      }
    }
    return db
  })
  cached = opening
  opening.catch(() => { if (cached === opening) cached = null })
  return opening
}

export function dbRequest<T>(store: string, mode: IDBTransactionMode, run: (s: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  return openDb().then((db) => new Promise<T>((resolve, reject) => {
    const t = db.transaction(store, mode)
    const req = run(t.objectStore(store))
    t.oncomplete = () => resolve(req.result)
    t.onerror = () => reject(t.error)
    t.onabort = () => reject(t.error)
  }))
}

export function dbTransaction(stores: string[], mode: IDBTransactionMode, run: (t: IDBTransaction) => void): Promise<void> {
  return openDb().then((db) => new Promise<void>((resolve, reject) => {
    const t = db.transaction(stores, mode)
    t.oncomplete = () => resolve()
    t.onerror = () => reject(t.error)
    t.onabort = () => reject(t.error)
    run(t)
  }))
}

export function resetDbForTests(): void {
  const previous = cached
  cached = null
  migrated = false
  previous?.then((db) => db.close(), () => {})
}
