import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import type { AccountDto } from '@/api/dto/account'
import type { TransactionDto } from '@/api/dto/transaction'
import { queryKeys } from '@/app/queryKeys'
import { IDBFactory } from 'fake-indexeddb'
import { loadBuckets, resetTransactionStoreForTests } from '@/lib/transactionStore'
import { STORE_ACCOUNT_TRANSACTIONS, STORE_META, dbRequest, resetDbForTests } from '@/lib/db'
import { clearPersistedQueryCache } from '@/lib/queryPersist'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts, fixtureOwner, fixtureTransactions } from '@/test/fixtures'
import { useAccountItemEffects } from '@/features/accounts/queries'
import { useApplyTransactionItem, useTransactions } from './queries'
import { installTransactionTracking, noteOwnWrite, requestFullReload, resetRevisions } from './revisionTracking'

// accountIds of every get-account-transaction-list request (null = all accounts)
let calls: (string | null)[] = []
let legacyCalls = 0
const uninstalls: (() => void)[] = []

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  calls = []
  legacyCalls = 0
  resetRevisions()
  // the analytics auth-method probe fired once the user data lands
  server.use(http.get('*/api/v1/oauth/get-identity-list', () => HttpResponse.json({ success: true, message: '', data: [] })))
  server.events.on('request:start', ({ request }) => {
    const url = new URL(request.url)
    if (url.pathname.endsWith('/get-account-transaction-list')) {
      calls.push(url.searchParams.get('accountIds'))
    }
    if (url.pathname.endsWith('/get-transaction-list')) {
      legacyCalls++
    }
  })
})

afterEach(() => {
  vi.useRealTimers()
  server.events.removeAllListeners()
  uninstalls.splice(0).forEach((u) => u())
  resetRevisions()
})

const wireAccounts = fixtureAccounts as unknown as AccountDto[]

function withRevisions(revs: Record<string, number>): AccountDto[] {
  return wireAccounts.map((a) => (a.id in revs ? { ...a, transactionsRevision: revs[a.id] } : a))
}

const created: TransactionDto = {
  id: 't-new', author: fixtureOwner, type: 'expense', accountId: 'a1', accountRecipientId: null,
  amount: '3', amountRecipient: '3', categoryId: 'cat-food', description: 'tea', payeeId: null, tagId: null,
  labelIds: [], date: '2026-07-03 10:00:00', recurringId: null, isImported: 0,
}

function setup(overrides: Partial<Record<string, unknown>> = {}) {
  server.use(...coreHandlers(overrides))
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  uninstalls.push(installTransactionTracking(queryClient))
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  // read during render, so the tracked-props observer re-renders on every change
  const { result } = renderHook(() => {
    const { data, isSuccess, isFetching } = useTransactions()
    return { list: { data, isSuccess, isFetching }, apply: useApplyTransactionItem(), accountEffects: useAccountItemEffects() }
  }, { wrapper })
  return { queryClient, result }
}

async function booted(overrides: Partial<Record<string, unknown>> = {}) {
  const ctx = setup(overrides)
  await waitFor(() => expect(ctx.result.current.list.isSuccess).toBe(true))
  return ctx
}

const settle = () => new Promise((r) => setTimeout(r, 50))

it('the first sync downloads every account in one request and stores the buckets', async () => {
  const { result } = await booted()
  expect(calls).toEqual([null])
  expect(result.current.list.data!.map((t) => t.id)).toEqual(['t1', 't2'])
  const stored = await loadBuckets('u1')
  expect([...stored.keys()].sort()).toEqual(['a-hidden', 'a1', 'a2', 'a3'])
  expect(stored.get('a1')!.items.map((t) => t.id)).toEqual(['t1', 't2'])
  expect(stored.get('a1')!.revision).toBe(1)
})

it('a refresh with unchanged revisions downloads no transactions', async () => {
  const { queryClient } = await booted()
  await act(() => queryClient.invalidateQueries())
  await settle()
  expect(calls).toEqual([null])
})

it('an accounts refresh with a moved revision downloads exactly that account, once', async () => {
  const { queryClient, result } = await booted()
  const moved = { ...fixtureTransactions[0], id: 't-a2', accountId: 'a2' }
  server.use(...coreHandlers({ accounts: withRevisions({ a2: 3 }), transactions: [...fixtureTransactions, moved] }))
  await act(() => queryClient.refetchQueries({ queryKey: queryKeys.accounts }))
  await waitFor(() => expect(result.current.list.data!.map((t) => t.id)).toContain('t-a2'))
  await settle()
  expect(calls).toEqual([null, 'a2'])
  expect((await loadBuckets('u1')).get('a2')!.revision).toBe(3)
})

it('a server revision ahead of the accounts list does not keep re-syncing', async () => {
  const { queryClient } = await booted()
  server.use(
    ...coreHandlers({ accounts: withRevisions({ a2: 2 }) }),
    http.get('*/api/v1/transaction/get-account-transaction-list', () =>
      HttpResponse.json({ success: true, message: '', data: { revisions: { a2: 3 }, items: [], authors: [] } })),
  )
  await act(() => queryClient.refetchQueries({ queryKey: queryKeys.accounts }))
  await waitFor(() => expect(calls).toEqual([null, 'a2']))
  await settle()
  expect(calls).toEqual([null, 'a2'])
})

it('an own write at exactly +1 needs no download and reaches IndexedDB with the new revision', async () => {
  const { result } = await booted()
  act(() => result.current.apply({ item: created, accounts: withRevisions({ a1: 2 }) }, 'add'))
  await waitFor(async () => {
    const a1 = (await loadBuckets('u1')).get('a1')!
    expect(a1.revision).toBe(2)
    expect(a1.items.map((t) => t.id)).toContain('t-new')
  }, { timeout: 2000 })
  expect(calls).toEqual([null])
  expect(result.current.list.data!.map((t) => t.id)).toContain('t-new')
})

it('an own write at +2 downloads that account', async () => {
  const { result } = await booted()
  server.use(...coreHandlers({ accounts: withRevisions({ a1: 3 }), transactions: [created, ...fixtureTransactions] }))
  act(() => result.current.apply({ item: created, accounts: withRevisions({ a1: 3 }) }, 'add'))
  await waitFor(() => expect(calls).toEqual([null, 'a1']))
  await settle()
  expect(calls).toEqual([null, 'a1'])
  expect((await loadBuckets('u1')).get('a1')!.revision).toBe(3)
})

it('a balance correction at exactly +1 needs no download', async () => {
  const { result } = await booted()
  // the wire correction carries none of the list's provenance fields
  const { labelIds: _l, recurringId: _r, isImported: _i, ...base } = created
  const correction = { ...base, id: 't-corr', type: 'income' as const, description: '' }
  const a1 = withRevisions({ a1: 2 }).find((a) => a.id === 'a1')!
  act(() => result.current.accountEffects({ item: { ...a1, balance: '200' }, transaction: correction }))
  await waitFor(async () => {
    const stored = (await loadBuckets('u1')).get('a1')!
    expect(stored.revision).toBe(2)
    expect(stored.items.map((t) => t.id)).toContain('t-corr')
  }, { timeout: 2000 })
  const row = (await loadBuckets('u1')).get('a1')!.items.find((t) => t.id === 't-corr')!
  expect(row.labelIds).toEqual([])
  expect(row.recurringId).toBeNull()
  expect(row.isImported).toBe(0)
  expect(calls).toEqual([null])
})

it('an update that moves a row to another account rewrites both buckets under their new revisions', async () => {
  const { result } = await booted()
  const moved = { ...fixtureTransactions[0], accountId: 'a2', labelIds: [], recurringId: null, isImported: 0 } as TransactionDto
  act(() => result.current.apply({ item: moved, accounts: withRevisions({ a1: 2, a2: 2 }) }, 'update'))
  await waitFor(async () => {
    const stored = await loadBuckets('u1')
    expect(stored.get('a1')!.revision).toBe(2)
    expect(stored.get('a1')!.items.map((t) => t.id)).toEqual(['t2'])
    expect(stored.get('a2')!.revision).toBe(2)
    expect(stored.get('a2')!.items.map((t) => t.id)).toEqual(['t1'])
  }, { timeout: 2000 })
  expect(calls).toEqual([null])
})

it('a write during the first load on an older server reloads instead of becoming the whole list', async () => {
  const legacy = wireAccounts.map(({ transactionsRevision: _r, ...a }) => a)
  let release: () => void = () => {}
  const gate = new Promise<void>((r) => { release = r })
  let first = true
  server.use(
    http.get('*/api/v1/transaction/get-transaction-list', async () => {
      if (first) {
        first = false
        await gate
      }
      return undefined
    }),
    ...coreHandlers({ accounts: legacy, transactions: [created, ...fixtureTransactions] }),
  )
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  uninstalls.push(installTransactionTracking(queryClient))
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  const { result } = renderHook(() => {
    const { data } = useTransactions()
    return { data, apply: useApplyTransactionItem() }
  }, { wrapper })
  await waitFor(() => expect(legacyCalls).toBe(1))
  act(() => result.current.apply({ item: created, accounts: legacy as AccountDto[] }, 'add'))
  release()
  await waitFor(() => expect(result.current.data?.map((t) => t.id).sort()).toEqual(['t-new', 't1', 't2']))
  expect(legacyCalls).toBe(2)
})

it('a full reload cancelled by a local write runs again', async () => {
  const { queryClient, result } = await booted()
  let release: () => void = () => {}
  const gate = new Promise<void>((r) => { release = r })
  server.use(http.get('*/api/v1/transaction/get-account-transaction-list', async () => {
    await gate
    return undefined
  }))
  requestFullReload()
  void queryClient.invalidateQueries({ queryKey: queryKeys.transactions })
  await waitFor(() => expect(calls).toEqual([null, null]))
  act(() => result.current.apply({ item: created, accounts: withRevisions({ a1: 2 }) }, 'add'))
  release()
  await waitFor(() => expect(calls).toEqual([null, null, null]))
  await waitFor(() => expect(result.current.list.isFetching).toBe(false))
  await settle()
  expect(calls).toEqual([null, null, null])
})

it('noteOwnWrite ignores accounts it was not told were touched', async () => {
  const { queryClient } = await booted()
  noteOwnWrite(withRevisions({ a1: 2, a2: 2 }), ['a1'])
  server.use(...coreHandlers({ accounts: withRevisions({ a1: 2, a2: 2 }) }))
  act(() => queryClient.setQueryData(queryKeys.accounts, withRevisions({ a1: 2, a2: 2 })))
  await waitFor(() => expect(calls).toEqual([null, 'a2']))
})

it('an in-flight sync cancelled by a local write does not undo it', async () => {
  const { queryClient, result } = await booted()
  let release: () => void = () => {}
  const gate = new Promise<void>((r) => { release = r })
  const a2Row = { ...fixtureTransactions[0], id: 't-a2', accountId: 'a2' }
  server.use(
    ...coreHandlers({ accounts: withRevisions({ a2: 2 }), transactions: [created, ...fixtureTransactions, a2Row] }),
  )
  server.use(http.get('*/api/v1/transaction/get-account-transaction-list', async () => {
    await gate
    return undefined
  }))
  await act(() => queryClient.refetchQueries({ queryKey: queryKeys.accounts }))
  await waitFor(() => expect(calls).toEqual([null, 'a2']))
  act(() => result.current.apply({ item: created, accounts: withRevisions({ a1: 2, a2: 2 }) }, 'add'))
  release()
  await waitFor(() => expect(result.current.list.data!.map((t) => t.id)).toEqual(expect.arrayContaining(['t-new', 't-a2'])))
  await waitFor(async () => {
    const stored = await loadBuckets('u1')
    expect(stored.get('a1')!.revision).toBe(2)
    expect(stored.get('a1')!.items.map((t) => t.id)).toContain('t-new')
    expect(stored.get('a2')!.revision).toBe(2)
  }, { timeout: 2000 })
  // the cancelled request and the re-run; a1 is never downloaded
  expect(calls).toEqual([null, 'a2', 'a2'])
})

it('a local patch reaches IndexedDB after 500 ms, keeping the stored revision', async () => {
  const { queryClient } = await booted()
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
  act(() => {
    queryClient.setQueryData<TransactionDto[]>(queryKeys.transactions, (prev) =>
      prev!.map((t) => (t.categoryId === 'cat-food' ? { ...t, categoryId: 'cat-salary' } : t)))
  })
  await act(async () => { vi.advanceTimersByTime(499) })
  expect((await loadBuckets('u1')).get('a1')!.items.find((t) => t.id === 't1')!.categoryId).toBe('cat-food')
  await act(async () => { vi.advanceTimersByTime(1) })
  vi.useRealTimers()
  await waitFor(async () => {
    const a1 = (await loadBuckets('u1')).get('a1')!
    expect(a1.items.find((t) => t.id === 't1')!.categoryId).toBe('cat-salary')
    expect(a1.revision).toBe(1)
  })
  expect(calls).toEqual([null])
})

it('a sync starts from unsaved local patches, not from the older stored rows', async () => {
  const { queryClient, result } = await booted()
  act(() => {
    queryClient.setQueryData<TransactionDto[]>(queryKeys.transactions, (prev) =>
      prev!.map((t) => (t.id === 't1' ? { ...t, description: 'patched' } : t)))
  })
  await act(() => queryClient.invalidateQueries({ queryKey: queryKeys.transactions }))
  await waitFor(() => expect(result.current.list.isFetching).toBe(false))
  expect(result.current.list.data!.find((t) => t.id === 't1')!.description).toBe('patched')
  expect(calls).toEqual([null])
})

it('includes the rows of a hidden-folder account', async () => {
  const hidden = { ...fixtureTransactions[0], id: 't-hidden', accountId: 'a-hidden' }
  const { result } = await booted({ transactions: [...fixtureTransactions, hidden] })
  expect(result.current.list.data!.map((t) => t.id)).toContain('t-hidden')
})

it('falls back to get-transaction-list and stores nothing when accounts carry no revision', async () => {
  const accounts = wireAccounts.map(({ transactionsRevision: _r, ...a }) => a)
  const { result } = await booted({ accounts })
  expect(legacyCalls).toBe(1)
  expect(calls).toEqual([])
  expect(result.current.list.data!.map((t) => t.id)).toEqual(['t1', 't2'])
  expect((await loadBuckets('u1')).size).toBe(0)
})

it('never requests an account whose invite is still pending', async () => {
  const pending = {
    ...wireAccounts[1], id: 'a-pending', owner: { id: 'u2', name: 'Bob', avatar: 'face:sky' },
    sharedAccess: [{ user: fixtureOwner, role: 'user', isAccepted: 0 }],
  }
  const { queryClient } = setup({ accounts: [...wireAccounts, pending] })
  // like the real server, the response omits an account the caller cannot read yet
  server.use(http.get('*/api/v1/transaction/get-account-transaction-list', () =>
    HttpResponse.json({ success: true, message: '', data: { revisions: { a1: 1, a2: 1, a3: 1, 'a-hidden': 1 }, items: [], authors: [] } })))
  await waitFor(() => expect(queryClient.getQueryState(queryKeys.transactions)?.status).toBe('success'))
  await act(() => queryClient.invalidateQueries())
  await settle()
  expect(calls).toEqual([null])
})

it('a full reload request downloads every account once', async () => {
  const { queryClient } = await booted()
  requestFullReload()
  await act(() => queryClient.invalidateQueries({ queryKey: queryKeys.transactions }))
  await act(() => queryClient.invalidateQueries({ queryKey: queryKeys.transactions }))
  expect(calls).toEqual([null, null])
})

it('a sync that lands after a clear leaves nothing on disk', async () => {
  globalThis.indexedDB = new IDBFactory()
  resetDbForTests()
  resetTransactionStoreForTests()
  let release!: () => void
  const gate = new Promise<void>((r) => { release = r })
  const ctx = setup()
  server.use(http.get('*/api/v1/transaction/get-account-transaction-list', async () => {
    await gate
    return HttpResponse.json({
      success: true, message: '',
      data: {
        revisions: Object.fromEntries(wireAccounts.map((a) => [a.id, a.transactionsRevision])),
        items: fixtureTransactions.map(({ author, ...rest }) => ({ ...rest, authorId: author.id })),
        authors: [fixtureOwner],
      },
    })
  }))
  await waitFor(() => expect(calls).toEqual([null]))
  await clearPersistedQueryCache()
  release()
  await waitFor(() => expect(ctx.result.current.list.isFetching).toBe(false))
  await settle()
  expect(await dbRequest(STORE_ACCOUNT_TRANSACTIONS, 'readonly', (st) => st.count())).toBe(0)
  expect(await dbRequest(STORE_META, 'readonly', (st) => st.count())).toBe(0)
})
