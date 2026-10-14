import { IDBFactory } from 'fake-indexeddb'
import type { StoredTransaction } from '@/api/dto/transaction'
import { STORE_ACCOUNT_TRANSACTIONS, STORE_IMPORT_KEYS, dbRequest, resetDbForTests } from './db'
import { delayOpen, delayTransactionCompletion } from '@/test/idbDelay'
import { RELOAD_REVISION, STORE_TIMEOUT_MS, clearTransactions, deleteBuckets, loadBuckets, markForReload, putBuckets, resetTransactionStoreForTests, storeBuster, storeEpoch, tryLoadBuckets, tryLoadStored, type Bucket } from './transactionStore'

const tx = (id: string, accountId: string): StoredTransaction => ({
  id, type: 'expense', accountId, accountRecipientId: null, amount: '1', amountRecipient: null,
  categoryId: null, description: '', payeeId: null, tagId: null, labelIds: [], date: '2026-10-01 00:00:00',
  recurringId: null, isImported: 0, authorId: 'u1',
})

const bucket = (accountId: string, revision = 1): Bucket => ({
  accountId, revision, items: [tx(`t-${accountId}`, accountId)], authors: [{ id: 'u1', name: 'Ann', avatar: 'face:fuchsia' }],
})

beforeEach(() => {
  globalThis.indexedDB = new IDBFactory()
  resetDbForTests()
  resetTransactionStoreForTests()
  window.econumoConfig = { VERSION: '1.0.0' }
})

afterEach(() => {
  window.econumoConfig = {}
  vi.useRealTimers()
  vi.restoreAllMocks()
})

it('the buster combines the format and the app version', () => {
  expect(storeBuster()).toBe('1:1.0.0')
})

it('round-trips buckets', async () => {
  expect((await loadBuckets('u1')).size).toBe(0)
  await putBuckets('u1', [bucket('a', 3), bucket('b', 5)])
  const loaded = await loadBuckets('u1')
  expect([...loaded.keys()].sort()).toEqual(['a', 'b'])
  expect(loaded.get('a')).toEqual(bucket('a', 3))
})

it('putBuckets replaces a bucket and deleteBuckets removes only the named ones', async () => {
  await putBuckets('u1', [bucket('a', 1), bucket('b', 1)])
  await putBuckets('u1', [bucket('a', 2)])
  await deleteBuckets(['b', 'missing'])
  const loaded = await loadBuckets('u1')
  expect([...loaded.keys()]).toEqual(['a'])
  expect(loaded.get('a')?.revision).toBe(2)
})

it('another user gets an empty store and the previous data is gone', async () => {
  await loadBuckets('u1')
  await putBuckets('u1', [bucket('a')])
  expect((await loadBuckets('u2')).size).toBe(0)
  expect((await loadBuckets('u1')).size).toBe(0)
})

it('a new app version wipes the buckets', async () => {
  await loadBuckets('u1')
  await putBuckets('u1', [bucket('a')])
  window.econumoConfig = { VERSION: '1.1.0' }
  expect((await loadBuckets('u1')).size).toBe(0)
  await putBuckets('u1', [bucket('a')])
  expect((await loadBuckets('u1')).size).toBe(1)
})

it('a write before the first load by another user does not leak the previous user\'s buckets', async () => {
  await loadBuckets('u1')
  await putBuckets('u1', [bucket('a')])
  await putBuckets('u2', [bucket('b')])
  const loaded = await loadBuckets('u2')
  expect([...loaded.keys()]).toEqual(['b'])
})

it('a write before the first load after an upgrade drops the old build\'s buckets', async () => {
  await loadBuckets('u1')
  await putBuckets('u1', [bucket('a')])
  window.econumoConfig = { VERSION: '1.1.0' }
  await putBuckets('u1', [bucket('b')])
  expect([...(await loadBuckets('u1')).keys()]).toEqual(['b'])
})

it('a write before any load is kept by the next load for the same user', async () => {
  await putBuckets('u1', [bucket('a')])
  expect([...(await loadBuckets('u1')).keys()]).toEqual(['a'])
})

it('markForReload keeps every bucket\'s rows and marks it for download', async () => {
  await putBuckets('u1', [bucket('a', 3), bucket('b', 5)])
  await markForReload()
  const loaded = await loadBuckets('u1')
  expect(loaded.get('a')).toEqual({ ...bucket('a'), revision: RELOAD_REVISION })
  expect(loaded.get('b')).toEqual({ ...bucket('b'), revision: RELOAD_REVISION })
})

it('a write a sync started before markForReload does not replace a mark', async () => {
  await putBuckets('u1', [bucket('a', 3)])
  const at = storeEpoch()
  await markForReload()
  await putBuckets('u1', [bucket('a', 4)], at)
  expect((await loadBuckets('u1')).get('a')?.revision).toBe(RELOAD_REVISION)
})

it('clearTransactions still clears after the store disabled itself', async () => {
  vi.spyOn(console, 'warn').mockImplementation(() => {})
  await loadBuckets('u1')
  await putBuckets('u1', [bucket('a')])
  const realOpen = globalThis.indexedDB.open.bind(globalThis.indexedDB)
  globalThis.indexedDB.open = () => { throw new Error('x') }
  await loadBuckets('u1')  // disables the store
  globalThis.indexedDB.open = realOpen
  resetDbForTests()
  await clearTransactions()
  resetTransactionStoreForTests()
  expect((await loadBuckets('u1')).size).toBe(0)
})

it('clearTransactions keeps the SimpleFIN key', async () => {
  await dbRequest(STORE_IMPORT_KEYS, 'readwrite', (s) => s.put({ key: 'k', tag: 't' }, 'dataKey'))
  await loadBuckets('u1')
  await putBuckets('u1', [bucket('a')])
  await clearTransactions()
  expect((await loadBuckets('u1')).size).toBe(0)
  expect(await dbRequest(STORE_IMPORT_KEYS, 'readonly', (s) => s.get('dataKey'))).toEqual({ key: 'k', tag: 't' })
})

it('an unusable IndexedDB makes the store a silent no-op with a single warning', async () => {
  const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
  globalThis.indexedDB.open = () => { throw new Error('x') }
  expect((await loadBuckets('u1')).size).toBe(0)
  await expect(putBuckets('u1', [bucket('a')])).resolves.toBeUndefined()
  await expect(deleteBuckets(['a'])).resolves.toBeUndefined()
  await expect(clearTransactions()).resolves.toBeUndefined()
  expect((await loadBuckets('u1')).size).toBe(0)
  expect(warn).toHaveBeenCalledTimes(1)
})

it('a slow write still completes and leaves the store on', async () => {
  const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
  await loadBuckets('u1')
  const restore = delayTransactionCompletion(STORE_TIMEOUT_MS + 1000, (_, mode) => mode === 'readwrite')
  try {
    await putBuckets('u1', [bucket('a')])
  } finally {
    restore()
  }
  expect((await loadBuckets('u1')).get('a')).toEqual(bucket('a'))
  expect(warn).not.toHaveBeenCalled()
}, 10_000)

it('a load that does not answer in time gives up without turning the store off', async () => {
  const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
  await putBuckets('u1', [bucket('a')])
  const restore = delayTransactionCompletion(null)
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
  const load = tryLoadBuckets('u1')
  await vi.advanceTimersByTimeAsync(STORE_TIMEOUT_MS + 100)
  expect(await load).toBeNull()
  vi.useRealTimers()
  restore()
  // still on: a write lands
  await putBuckets('u1', [bucket('b')])
  expect(await dbRequest(STORE_ACCOUNT_TRANSACTIONS, 'readonly', (s) => s.getAllKeys())).toEqual(['a', 'b'])
  expect(warn).not.toHaveBeenCalled()
})

it('a clear that does not answer in time lets its caller go on', async () => {
  await putBuckets('u1', [bucket('a')])
  const restore = delayTransactionCompletion(null)
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
  let done = false
  const clear = clearTransactions().then(() => { done = true })
  await vi.advanceTimersByTimeAsync(STORE_TIMEOUT_MS + 100)
  await clear
  expect(done).toBe(true)
  vi.useRealTimers()
  restore()
  // the clear itself still ran
  expect((await loadBuckets('u1')).size).toBe(0)
})

it('a load for the same user and build only reads', async () => {
  await putBuckets('u1', [bucket('a')])
  const spy = vi.spyOn(IDBDatabase.prototype, 'transaction')
  expect([...(await loadBuckets('u1')).keys()]).toEqual(['a'])
  expect(spy.mock.calls.map((c) => c[1])).toEqual(['readonly'])
})

it('a database that opens after the limit is no failure: the load gives up, the store stays on', async () => {
  const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
  const restore = delayOpen(STORE_TIMEOUT_MS + 300)
  try {
    expect(await tryLoadBuckets('u1')).toBeNull()
    await putBuckets('u1', [bucket('a')])
  } finally {
    restore()
  }
  expect((await loadBuckets('u1')).get('a')).toEqual(bucket('a'))
  expect(warn).not.toHaveBeenCalled()
}, 10_000)

it('while a load that ran out of time is still pending, the next one does not wait again', async () => {
  await putBuckets('u1', [bucket('a')])
  const restore = delayTransactionCompletion(STORE_TIMEOUT_MS + 300, (_, mode) => mode === 'readonly')
  try {
    expect(await tryLoadBuckets('u1')).toBeNull()
    const started = Date.now()
    expect(await tryLoadBuckets('u1')).toBeNull()
    expect(Date.now() - started).toBeLessThan(STORE_TIMEOUT_MS / 3)
  } finally {
    restore()
  }
  // the first load answers in the end; after that a load reads again
  await new Promise((r) => setTimeout(r, 600))
  expect((await loadBuckets('u1')).get('a')).toEqual(bucket('a'))
}, 10_000)

it('a clear held up by a slow open lets its caller go on and still runs once the database opens', async () => {
  await putBuckets('u1', [bucket('a')])
  // drop the open connection, so the clear has to open the database again
  resetDbForTests()
  const restore = delayOpen(STORE_TIMEOUT_MS + 300)
  try {
    const started = Date.now()
    await clearTransactions()
    expect(Date.now() - started).toBeLessThan(STORE_TIMEOUT_MS + 250)
  } finally {
    restore()
  }
  expect(await dbRequest(STORE_ACCOUNT_TRANSACTIONS, 'readonly', (s) => s.count())).toBe(0)
}, 10_000)

describe('tryLoadStored', () => {
  it('returns the buckets with their owner and when they were last written', async () => {
    vi.spyOn(Date, 'now').mockReturnValue(5_000)
    await putBuckets('u1', [bucket('a'), bucket('b')])
    vi.mocked(Date.now).mockReturnValue(9_000)
    await putBuckets('u1', [bucket('b', 2)])
    const stored = await tryLoadStored()
    expect(stored?.userId).toBe('u1')
    expect(stored?.savedAt).toBe(9_000)
    expect([...stored!.buckets.keys()].sort()).toEqual(['a', 'b'])
    expect(stored!.buckets.get('b')?.revision).toBe(2)
  })

  it('is null for an empty store and for another build\'s data, which it leaves alone', async () => {
    expect(await tryLoadStored()).toBeNull()
    await putBuckets('u1', [bucket('a')])
    window.econumoConfig = { VERSION: '1.1.0' }
    expect(await tryLoadStored()).toBeNull()
    window.econumoConfig = { VERSION: '1.0.0' }
    expect([...(await tryLoadStored())!.buckets.keys()]).toEqual(['a'])
  })

  it('tolerates the meta of a build that kept no write time', async () => {
    await putBuckets('u1', [bucket('a')])
    await dbRequest('meta', 'readwrite', (s) => s.put({ buster: storeBuster(), userId: 'u1' }, 'state'))
    const stored = await tryLoadStored()
    expect(stored?.savedAt).toBeUndefined()
    expect(stored?.buckets.size).toBe(1)
  })

  it('gives up after the limit', async () => {
    await putBuckets('u1', [bucket('a')])
    const restore = delayTransactionCompletion(null)
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    const load = tryLoadStored()
    await vi.advanceTimersByTimeAsync(STORE_TIMEOUT_MS + 100)
    expect(await load).toBeNull()
    vi.useRealTimers()
    restore()
  })
})
