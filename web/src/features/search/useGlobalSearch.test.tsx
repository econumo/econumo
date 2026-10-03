import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import {
  coreHandlers, fixtureAccounts, fixtureCategories, fixtureLabels, fixtureOwner, fixturePayees, fixtureTags, fixtureUser, fixtureUsd,
} from '@/test/fixtures'
import { useGlobalSearch, type SearchScope } from './useGlobalSearch'

const ME = fixtureUser.id
const OTHER = 'u-other'
const ALL: SearchScope = { kind: 'all' }

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>{children}</QueryClientProvider>
)

const stamp = { createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' }

const categories = [
  { id: 'cat-fuel-old', ownerUserId: ME, name: 'Fuel old', position: 0, type: 'expense', icon: 'local_gas_station', isArchived: 1, ...stamp },
  { id: 'cat-other-fees', ownerUserId: OTHER, name: 'Fees', position: 1, type: 'expense', icon: 'receipt', isArchived: 0, ...stamp },
  ...fixtureCategories.filter((c) => c.id !== 'cat-archived'),
]
const payees = [
  { id: 'p-other', ownerUserId: OTHER, name: 'Grocer Other', position: 1, isArchived: 0, ...stamp },
  ...fixturePayees,
]
const tags = [{ id: 'tag-other', ownerUserId: OTHER, name: 'vacation other', icon: 'tag', position: 1, isArchived: 0, ...stamp }, ...fixtureTags]
const labels = [{ id: 'label-other', ownerUserId: OTHER, name: 'health other', icon: 'sell', position: 1, isArchived: 0, ...stamp }, ...fixtureLabels]

const sharedAccount = {
  id: 'a-shared', owner: { id: 'u2', avatar: 'pets:sky', name: 'Partner' }, folderId: 'f1', name: 'Partner wallet', position: 4,
  currency: fixtureUsd, balance: '1', type: 1, icon: 'wallet', sharedAccess: [],
}

const tx = (over: Record<string, unknown>) => ({
  author: fixtureOwner, type: 'expense', accountRecipientId: null, amountRecipient: null, categoryId: null,
  description: '', payeeId: null, tagId: null, labelIds: [], recurringId: null, isImported: 0, ...over,
})

// t-bank is the oldest and carries every classification; t-coffee and t-salary
// are on the Cash account.
const transactions = [
  tx({ id: 't-bank', accountId: 'a2', amount: '30', categoryId: 'cat-food', payeeId: 'p1', tagId: 'tag1', labelIds: ['label1'], description: 'weekly shop', date: '2026-06-15 10:00:00' }),
  tx({ id: 't-coffee', accountId: 'a1', amount: '9.99', categoryId: 'cat-food', payeeId: 'p1', description: 'coffee beans', date: '2026-07-02 09:30:00' }),
  tx({ id: 't-salary', type: 'income', accountId: 'a1', amount: '500', categoryId: 'cat-salary', description: '', date: '2026-07-01 08:00:00' }),
  tx({ id: 't-hidden', accountId: 'a-hidden', amount: '5', labelIds: ['label1'], description: 'loose change', date: '2026-07-01 08:00:00' }),
]
const TOTAL_TX = transactions.length

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(
    ...coreHandlers({
      transactions,
      categories,
      payees,
      tags,
      labels,
      accounts: [...fixtureAccounts, sharedAccount],
    }),
  )
})

const txIds = (result: { current: ReturnType<typeof useGlobalSearch> }) =>
  result.current.transactions.flatMap((e) => (e.kind === 'transaction' ? [e.transaction.id] : []))

it('empty query: every transaction, no entity groups', async () => {
  const { result } = renderHook(() => useGlobalSearch('', ALL), { wrapper })
  await waitFor(() => expect(result.current.transactionCount).toBe(TOTAL_TX))
  expect(result.current.accounts).toEqual([])
  expect(result.current.categories).toEqual([])
  expect(result.current.payees).toEqual([])
  expect(result.current.tags).toEqual([])
  expect(result.current.labels).toEqual([])
})

it('classifications are own-only and archived ones come last', async () => {
  const { result } = renderHook(() => useGlobalSearch('f', ALL), { wrapper })
  await waitFor(() => expect(result.current.categories.length).toBeGreaterThan(0))
  expect(result.current.categories.map((c) => c.id)).toEqual(['cat-food', 'cat-fuel-old'])
  expect(result.current.categories.every((c) => c.ownerUserId === ME)).toBe(true)
  const archivedIdx = result.current.categories.findIndex((c) => c.isArchived === 1)
  expect(result.current.categories.slice(archivedIdx).every((c) => c.isArchived === 1)).toBe(true)
})

it('payees, tags and labels exclude other users items', async () => {
  const payee = renderHook(() => useGlobalSearch('grocer', ALL), { wrapper })
  await waitFor(() => expect(payee.result.current.payees.length).toBeGreaterThan(0))
  expect(payee.result.current.payees.map((p) => p.id)).toEqual(['p1'])

  const tag = renderHook(() => useGlobalSearch('vacation', ALL), { wrapper })
  await waitFor(() => expect(tag.result.current.tags.length).toBeGreaterThan(0))
  expect(tag.result.current.tags.map((t) => t.id)).toEqual(['tag1'])

  const label = renderHook(() => useGlobalSearch('health', ALL), { wrapper })
  await waitFor(() => expect(label.result.current.labels.length).toBeGreaterThan(0))
  expect(label.result.current.labels.map((l) => l.id)).toEqual(['label1'])
})

it('accounts include hidden-folder and shared accounts', async () => {
  const hidden = renderHook(() => useGlobalSearch('mattress', ALL), { wrapper })
  await waitFor(() => expect(hidden.result.current.accounts.length).toBeGreaterThan(0))
  expect(hidden.result.current.accounts.map((a) => a.id)).toEqual(['a-hidden'])

  const shared = renderHook(() => useGlobalSearch('partner', ALL), { wrapper })
  await waitFor(() => expect(shared.result.current.accounts.length).toBeGreaterThan(0))
  expect(shared.result.current.accounts.map((a) => a.id)).toEqual(['a-shared'])
  expect(shared.result.current.accounts[0].owner.id).not.toBe(ME)
})

it('an account name finds its transactions', async () => {
  const { result } = renderHook(() => useGlobalSearch('bank', ALL), { wrapper })
  await waitFor(() => expect(result.current.transactionCount).toBeGreaterThan(0))
  expect(txIds(result)).toEqual(['t-bank'])
  expect(result.current.accounts.map((a) => a.id)).toEqual(['a2'])
})

it('a skipped character still finds a transaction by description', async () => {
  const { result } = renderHook(() => useGlobalSearch('cofee', ALL), { wrapper })
  await waitFor(() => expect(result.current.transactionCount).toBeGreaterThan(0))
  expect(txIds(result)).toEqual(['t-coffee'])
})

it.each([
  { kind: 'category', id: 'cat-food', expected: ['t-coffee', 't-bank'] },
  { kind: 'payee', id: 'p1', expected: ['t-coffee', 't-bank'] },
  { kind: 'tag', id: 'tag1', expected: ['t-bank'] },
  { kind: 'label', id: 'label1', expected: ['t-hidden', 't-bank'] },
] as const)('scope $kind narrows to that item', async ({ kind, id, expected }) => {
  const scope: SearchScope = { kind, id }
  const { result } = renderHook(() => useGlobalSearch('', scope), { wrapper })
  await waitFor(() => expect(result.current.transactionCount).toBeGreaterThan(0))
  expect(txIds(result)).toEqual(expected)
  for (const entry of result.current.transactions) {
    if (entry.kind !== 'transaction') continue
    const t = entry.transaction
    if (kind === 'label') expect(t.labelIds).toContain(id)
    else expect(t[`${kind}Id`]).toBe(id)
  }
  expect(result.current.accounts).toEqual([])
  expect(result.current.categories).toEqual([])
  expect(result.current.payees).toEqual([])
  expect(result.current.tags).toEqual([])
  expect(result.current.labels).toEqual([])
})

it('orders transactions newest first and groups by day', async () => {
  const { result } = renderHook(() => useGlobalSearch('', ALL), { wrapper })
  await waitFor(() => expect(result.current.transactionCount).toBe(TOTAL_TX))
  const entries = result.current.transactions
  expect(entries[0]).toMatchObject({ kind: 'separator', day: '2026-07-02' })
  expect(entries[1]).toMatchObject({ kind: 'transaction', transaction: { id: 't-coffee' } })
  expect(entries.map((e) => (e.kind === 'separator' ? `sep:${e.day}` : e.transaction.id))).toEqual([
    'sep:2026-07-02', 't-coffee',
    'sep:2026-07-01', 't-salary', 't-hidden',
    'sep:2026-06-15', 't-bank',
  ])
})
