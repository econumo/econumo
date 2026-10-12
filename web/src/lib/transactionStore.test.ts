import { IDBFactory } from 'fake-indexeddb'
import type { StoredTransaction } from '@/api/dto/transaction'
import { STORE_IMPORT_KEYS, dbRequest, resetDbForTests } from './db'
import { clearTransactions, deleteBuckets, loadBuckets, putBuckets, resetTransactionStoreForTests, storeBuster, type Bucket } from './transactionStore'

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
