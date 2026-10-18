import { IDBFactory } from 'fake-indexeddb'
import { STORE_ACCOUNT_TRANSACTIONS, STORE_IMPORT_KEYS, STORE_META, dbRequest, dbTransaction, openDb, resetDbForTests } from './db'
import { loadStoredKey } from './importCrypto'

beforeEach(() => {
  globalThis.indexedDB = new IDBFactory()
  resetDbForTests()
})

const dbNames = async () => (await indexedDB.databases()).map((d) => d.name)

function seedLegacy(record: unknown): Promise<void> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open('econumo-import', 1)
    req.onupgradeneeded = () => req.result.createObjectStore('keys')
    req.onsuccess = () => {
      const db = req.result
      const t = db.transaction('keys', 'readwrite')
      t.objectStore('keys').put(record, 'dataKey')
      t.oncomplete = () => { db.close(); resolve() }
      t.onerror = () => reject(t.error)
    }
    req.onerror = () => reject(req.error)
  })
}

const newKey = () => crypto.subtle.generateKey({ name: 'AES-GCM', length: 256 }, false, ['encrypt', 'decrypt'])

it('a fresh open creates the three stores', async () => {
  const db = await openDb()
  expect(db.name).toBe('econumo')
  expect([...db.objectStoreNames].sort()).toEqual([STORE_ACCOUNT_TRANSACTIONS, STORE_IMPORT_KEYS, STORE_META].sort())
  expect(db.transaction(STORE_ACCOUNT_TRANSACTIONS).objectStore(STORE_ACCOUNT_TRANSACTIONS).keyPath).toBe('accountId')
})

it('openDb reuses one connection', async () => {
  expect(await openDb()).toBe(await openDb())
})

it('moves the key from the old econumo-import database and drops it', async () => {
  await seedLegacy({ key: await newKey(), tag: 't' })
  await openDb()
  const stored = await loadStoredKey()
  expect(stored?.tag).toBe('t')
  expect(stored?.key).toBeInstanceOf(CryptoKey)
  expect(await dbNames()).not.toContain('econumo-import')
})

it('moves a bare pre-tag CryptoKey record too', async () => {
  await seedLegacy(await newKey())
  await openDb()
  expect((await loadStoredKey())?.tag).toBe('')
})

it('does not overwrite a key that is already in importKeys', async () => {
  const db = await openDb()
  await dbRequest(STORE_IMPORT_KEYS, 'readwrite', (s) => s.put({ key: 'kept', tag: 'new' }, 'dataKey'))
  db.close()
  resetDbForTests()
  await seedLegacy({ key: await newKey(), tag: 'old' })
  await openDb()
  expect(await dbRequest(STORE_IMPORT_KEYS, 'readonly', (s) => s.get('dataKey'))).toEqual({ key: 'kept', tag: 'new' })
  expect(await dbNames()).not.toContain('econumo-import')
})

it('leaves no econumo-import database behind when there was none', async () => {
  await openDb()
  expect(await dbNames()).toEqual(['econumo'])
})

it('a failing migration is swallowed with one warning and the database still opens', async () => {
  await seedLegacy({ key: await newKey(), tag: 't' })
  const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
  const realOpen = indexedDB.open.bind(indexedDB)
  indexedDB.open = ((name: string, version?: number) => {
    if (name === 'econumo-import') throw new Error('boom')
    return realOpen(name, version)
  }) as typeof indexedDB.open
  const db = await openDb()
  expect(db.name).toBe('econumo')
  expect(warn).toHaveBeenCalledTimes(1)
  warn.mockRestore()
})

it('a failing put into importKeys leaves econumo-import intact', async () => {
  await seedLegacy({ key: await newKey(), tag: 't' })
  const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
  const realPut = IDBObjectStore.prototype.put
  const put = vi.spyOn(IDBObjectStore.prototype, 'put').mockImplementation(function (this: IDBObjectStore, ...args: Parameters<IDBObjectStore['put']>) {
    if (this.name === STORE_IMPORT_KEYS) throw new Error('quota')
    return realPut.apply(this, args)
  })
  await openDb()
  put.mockRestore()
  expect(warn).toHaveBeenCalledTimes(1)
  warn.mockRestore()
  expect(await dbNames()).toContain('econumo-import')
  expect(await loadStoredKey()).toBeNull()
})

it('a synchronous throw inside dbTransaction aborts the requests already queued', async () => {
  await expect(dbTransaction([STORE_META], 'readwrite', (t) => {
    t.objectStore(STORE_META).put({ a: 1 }, 'state')
    throw new Error('clone')
  })).rejects.toThrow('clone')
  expect(await dbRequest(STORE_META, 'readonly', (s) => s.get('state'))).toBeUndefined()
})

it('a stale handle closing does not drop the cache of a newer connection', async () => {
  const first = await openDb()
  resetDbForTests()
  const second = await openDb()
  first.onclose?.(new Event('close'))
  expect(await openDb()).toBe(second)
})

it('dbTransaction resolves on commit and rejects on abort', async () => {
  await dbTransaction([STORE_META], 'readwrite', (t) => { t.objectStore(STORE_META).put({ a: 1 }, 'state') })
  expect(await dbRequest(STORE_META, 'readonly', (s) => s.get('state'))).toEqual({ a: 1 })
  await expect(dbTransaction([STORE_META], 'readwrite', (t) => { t.abort() })).rejects.toBeDefined()
})

it('a versionchange from another tab closes the cached connection instead of blocking it', async () => {
  const db = await openDb()
  const upgraded = await new Promise<IDBDatabase>((resolve, reject) => {
    const req = indexedDB.open('econumo', 2)
    req.onblocked = () => reject(new Error('blocked by the cached connection'))
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
  expect(() => db.transaction(STORE_META)).toThrow()  // closed
  upgraded.close()
})

it('after the database is deleted elsewhere, openDb reopens a fresh one', async () => {
  const first = await openDb()
  await new Promise<void>((resolve, reject) => {
    const del = indexedDB.deleteDatabase('econumo')
    del.onblocked = () => reject(new Error('blocked by the cached connection'))
    del.onsuccess = () => resolve()
    del.onerror = () => reject(del.error)
  })
  const second = await openDb()
  expect(second).not.toBe(first)
  expect([...second.objectStoreNames]).toHaveLength(3)
})
