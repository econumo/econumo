import { AxiosError, type AxiosResponse } from 'axios'
import { hashKey, QueryClient } from '@tanstack/react-query'
import { IDBFactory } from 'fake-indexeddb'
import { fixtureAccounts, fixtureOwner, fixtureUser } from '@/test/fixtures'
import { installTransactionTracking, resetRevisions } from '@/features/transactions/revisionTracking'
import { delayTransactionCompletion } from '@/test/idbDelay'
import { createAppQueryClient } from './appQueryClient'
import { resetDbForTests } from './db'
import { QUERY_CACHE_KEY, clearPersistedQueryCache, createPersistOptions } from './queryPersist'
import { STORE_TIMEOUT_MS, loadBuckets, putBuckets, resetTransactionStoreForTests, type Bucket } from './transactionStore'

function defaultRetry(): (failureCount: number, error: unknown) => boolean {
  const retry = createAppQueryClient().getDefaultOptions().queries?.retry
  if (typeof retry !== 'function') {
    throw new Error('expected the default query retry to be a function')
  }
  return retry as (failureCount: number, error: unknown) => boolean
}

it('retries without limit while the backend is unreachable (network error, no response)', () => {
  const retry = defaultRetry()
  const networkError = new AxiosError('Network Error', 'ERR_NETWORK')
  expect(retry(1, networkError)).toBe(true)
  expect(retry(50, networkError)).toBe(true)
})

it('keeps the three-attempt default for HTTP errors', () => {
  const retry = defaultRetry()
  const httpError = new AxiosError('Server Error', 'ERR_BAD_RESPONSE', undefined, undefined, {
    status: 500,
  } as AxiosResponse)
  expect(retry(2, httpError)).toBe(true)
  expect(retry(3, httpError)).toBe(false)
})

it('keeps the three-attempt default for non-axios errors', () => {
  const retry = defaultRetry()
  const plainError = new Error('boom')
  expect(retry(2, plainError)).toBe(true)
  expect(retry(3, plainError)).toBe(false)
})

it('never persists an authorization request', () => {
  const should = createPersistOptions().dehydrateOptions.shouldDehydrateQuery
  expect(should({ queryKey: ['authserver', 'authorization-request', {}], state: { status: 'success' } })).toBe(false)
  expect(should({ queryKey: ['accounts'], state: { status: 'success' } })).toBe(true)
  expect(should({ queryKey: ['accounts'], state: { status: 'error' } })).toBe(false)
})

it('never persists the transactions list: it lives in IndexedDB', () => {
  const should = createPersistOptions().dehydrateOptions.shouldDehydrateQuery
  expect(should({ queryKey: ['transactions'], state: { status: 'success' } })).toBe(false)
})

describe('restoring the stored transactions', () => {
  beforeEach(() => {
    localStorage.clear()
    window.econumoConfig = {}
    globalThis.indexedDB = new IDBFactory()
    resetDbForTests()
    resetTransactionStoreForTests()
    resetRevisions()
  })

  afterEach(() => resetRevisions())

  const row = (id: string, accountId: string) => ({
    id, type: 'expense', accountId, accountRecipientId: null, amount: '1', amountRecipient: '1',
    categoryId: null, description: id, payeeId: null, tagId: null, labelIds: [], date: '2026-07-01 00:00:00',
    recurringId: null, isImported: 0, authorId: fixtureOwner.id,
  })
  const bucket = (accountId: string, ids: string[]): Bucket =>
    ({ accountId, revision: 1, items: ids.map((id) => row(id, accountId)), authors: [fixtureOwner] }) as Bucket

  function snapshot(data: { user?: unknown; accounts?: unknown }, timestamp = Date.now()) {
    const queries = Object.entries(data).map(([key, value]) => ({
      queryKey: [key],
      queryHash: hashKey([key]),
      state: { data: value, dataUpdatedAt: timestamp, status: 'success' },
    }))
    localStorage.setItem(QUERY_CACHE_KEY, JSON.stringify({
      buster: createPersistOptions().buster,
      timestamp,
      clientState: { queries, mutations: [] },
    }))
  }

  async function restoredTransactions() {
    const restored = await createPersistOptions().persister.restoreClient()
    const query = restored?.clientState.queries.find((q) => q.queryHash === hashKey(['transactions']))
    return query?.state as { data: { id: string }[]; dataUpdatedAt: number; status: string } | undefined
  }

  it('adds the buckets of the snapshot accounts as a restored query', async () => {
    await putBuckets(fixtureUser.id, [bucket('a1', ['t1']), bucket('a2', ['t2']), bucket('gone', ['t3'])])
    snapshot({ user: fixtureUser, accounts: fixtureAccounts }, 1_000_000)
    vi.spyOn(Date, 'now').mockReturnValue(1_000_500)
    try {
      const state = await restoredTransactions()
      expect(state?.status).toBe('success')
      expect(state?.dataUpdatedAt).toBe(1_000_000)
      expect(state?.data.map((t) => t.id).sort()).toEqual(['t1', 't2'])
    } finally {
      vi.restoreAllMocks()
    }
  })

  it('drops the stored buckets of accounts the snapshot no longer lists', async () => {
    await putBuckets(fixtureUser.id, [bucket('a1', ['t1']), bucket('gone', ['t3'])])
    snapshot({ user: fixtureUser, accounts: fixtureAccounts })
    expect((await restoredTransactions())?.data.map((t) => t.id)).toEqual(['t1'])
    await vi.waitFor(async () => expect([...(await loadBuckets(fixtureUser.id)).keys()]).toEqual(['a1']))
  })

  it('leaves out an account whose share invite is still pending', async () => {
    const pending = {
      ...fixtureAccounts[0], id: 'a-invite', owner: { id: 'u9', name: 'Bo', avatar: 'face:emerald' },
      sharedAccess: [{ user: { id: fixtureUser.id, name: 'Ada', avatar: 'face:emerald' }, role: 'user', isAccepted: 0 }],
    }
    await putBuckets(fixtureUser.id, [bucket('a1', ['t1']), bucket('a-invite', ['t9'])])
    snapshot({ user: fixtureUser, accounts: [...fixtureAccounts, pending] })
    expect((await restoredTransactions())?.data.map((t) => t.id)).toEqual(['t1'])
  })

  it('seeds nothing without the user or the accounts in the snapshot', async () => {
    await putBuckets(fixtureUser.id, [bucket('a1', ['t1'])])
    snapshot({ accounts: fixtureAccounts })
    expect(await restoredTransactions()).toBeUndefined()
    snapshot({ user: fixtureUser })
    expect(await restoredTransactions()).toBeUndefined()
  })

  it('seeds nothing into a snapshot that is about to be discarded', async () => {
    await putBuckets(fixtureUser.id, [bucket('a1', ['t1'])])
    snapshot({ user: fixtureUser, accounts: fixtureAccounts }, Date.now() - 25 * 60 * 60 * 1000)
    expect(await restoredTransactions()).toBeUndefined()
  })

  it('a clear forgets the restored revisions, so the next user adopts none of them', async () => {
    await putBuckets(fixtureUser.id, [bucket('a1', ['t1'])])
    snapshot({ user: fixtureUser, accounts: fixtureAccounts })
    expect(await restoredTransactions()).toBeDefined()

    await clearPersistedQueryCache()

    // u2 signs in on the same tab and edits before the first sync lands
    const queryClient = new QueryClient()
    const uninstall = installTransactionTracking(queryClient)
    try {
      queryClient.setQueryData(['user'], { ...fixtureUser, id: 'u2' })
      queryClient.setQueryData(['transactions'], [])
      queryClient.setQueryData(['transactions'], [{ ...row('t-u2', 'a1'), author: fixtureOwner }])
      await new Promise((r) => setTimeout(r, 700))
      expect((await loadBuckets('u2')).size).toBe(0)
    } finally {
      uninstall()
    }
  })

  it('a stalled IndexedDB neither holds the restore nor the clear', async () => {
    globalThis.indexedDB = { open: () => ({}) } as unknown as IDBFactory
    resetDbForTests()
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    try {
      snapshot({ user: fixtureUser, accounts: fixtureAccounts })
      const restored = await createPersistOptions().persister.restoreClient()
      expect(restored?.clientState.queries.map((q) => q.queryKey)).toEqual([['user'], ['accounts']])
      await clearPersistedQueryCache()
      expect(localStorage.getItem(QUERY_CACHE_KEY)).toBeNull()
      // a stall is no failure: the store stays on in case the database answers later
      expect(warn).not.toHaveBeenCalled()
    } finally {
      warn.mockRestore()
    }
  }, 10_000)

  it('a restore whose load does not answer in time goes on without transactions', async () => {
    await putBuckets(fixtureUser.id, [bucket('a1', ['t1'])])
    snapshot({ user: fixtureUser, accounts: fixtureAccounts })
    const restore = delayTransactionCompletion(STORE_TIMEOUT_MS + 300)
    try {
      expect(await restoredTransactions()).toBeUndefined()
    } finally {
      restore()
    }
    await new Promise((r) => setTimeout(r, 600))
    // the store stayed on: once the slow load is done, a restore seeds again
    expect((await restoredTransactions())?.data.map((t) => t.id)).toEqual(['t1'])
  }, 10_000)
})
