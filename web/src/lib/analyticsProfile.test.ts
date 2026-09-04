import { QueryClient } from '@tanstack/react-query'
import { queryKeys } from '@/app/queryKeys'
import { profileAttributes, setAnalyticsQueryClient } from './analyticsProfile'

const ME = 'u1'
const OWNER = { id: ME } as const
const OTHER_OWNER = { id: 'u9' } as const

// Minimal AccountDto shape isPendingForMe() actually reads: owner.id and
// sharedAccess entries. Every other AccountDto field is irrelevant here.
function account(overrides: {
  id: string
  folderId: string | null
  owner?: { id: string }
  sharedAccess?: { user: { id: string }; isAccepted: 0 | 1 }[]
}) {
  return { owner: OWNER, sharedAccess: [], ...overrides }
}

afterEach(() => {
  setAnalyticsQueryClient(null)
})

it('counts what the cache holds and omits what it does not', () => {
  const qc = new QueryClient()
  qc.setQueryData(queryKeys.folders, [
    { id: 'f1', name: 'Visible', position: 0, isVisible: 1 },
    { id: 'f2', name: 'Hidden', position: 1, isVisible: 0 },
  ])
  // useAccounts() caches the raw get-account-list response: AccountDto[],
  // flat, folderId directly on the item (not wrapped in AccountItemDto).
  qc.setQueryData(queryKeys.accounts, [
    account({ id: 'a1', folderId: 'f1' }),
    account({ id: 'a2', folderId: 'f2' }),
    account({ id: 'a3', folderId: null }),
  ])
  qc.setQueryData(queryKeys.categories, [
    { id: 'c1', isArchived: 0 },
    { id: 'c2', isArchived: 1 },
  ])
  qc.setQueryData(queryKeys.user, { id: ME, createdAt: '2026-02-17 08:30:00' })
  setAnalyticsQueryClient(qc)

  const attrs = profileAttributes()
  expect(attrs.accounts).toBe(3)
  expect(attrs.accounts_hidden).toBe(1) // unfoldered counts as visible
  expect(attrs.categories).toBe(1)
  expect(attrs.categories_archived).toBe(1)
  expect(attrs.signup_year).toBe(2026)
  expect(attrs.signup_month).toBe(2)
  // Never loaded, so never guessed at.
  expect('tags' in attrs).toBe(false)
  expect('connections' in attrs).toBe(false)
})

it('counts payees, tags and connections when loaded', () => {
  const qc = new QueryClient()
  qc.setQueryData(queryKeys.payees, [
    { id: 'p1', isArchived: 0 },
    { id: 'p2', isArchived: 0 },
    { id: 'p3', isArchived: 1 },
  ])
  qc.setQueryData(queryKeys.tags, [{ id: 't1', isArchived: 0 }])
  qc.setQueryData(queryKeys.connections, [{ user: { id: 'u2' } }, { user: { id: 'u3' } }])
  setAnalyticsQueryClient(qc)

  const attrs = profileAttributes()
  expect(attrs.payees).toBe(2)
  expect(attrs.payees_archived).toBe(1)
  expect(attrs.tags).toBe(1)
  expect(attrs.tags_archived).toBe(0)
  expect(attrs.connections).toBe(2)
})

it('excludes an un-accepted pending share from accounts', () => {
  const qc = new QueryClient()
  qc.setQueryData(queryKeys.accounts, [
    account({ id: 'a1', folderId: null }),
    account({
      id: 'a2-pending',
      folderId: null,
      owner: OTHER_OWNER,
      sharedAccess: [{ user: { id: ME }, isAccepted: 0 }],
    }),
  ])
  qc.setQueryData(queryKeys.user, { id: ME })
  setAnalyticsQueryClient(qc)

  const attrs = profileAttributes()
  expect(attrs.accounts).toBe(1)
})

it('omits accounts_hidden when folders have not loaded', () => {
  const qc = new QueryClient()
  qc.setQueryData(queryKeys.accounts, [account({ id: 'a1', folderId: null })])
  qc.setQueryData(queryKeys.user, { id: ME })
  setAnalyticsQueryClient(qc)

  const attrs = profileAttributes()
  expect(attrs.accounts).toBe(1)
  expect('accounts_hidden' in attrs).toBe(false)
})

it('omits accounts and accounts_hidden when the user has not loaded (pending shares cannot be filtered without "me")', () => {
  const qc = new QueryClient()
  qc.setQueryData(queryKeys.folders, [{ id: 'f1', name: 'Hidden', position: 0, isVisible: 0 }])
  qc.setQueryData(queryKeys.accounts, [account({ id: 'a1', folderId: 'f1' })])
  setAnalyticsQueryClient(qc)

  const attrs = profileAttributes()
  expect('accounts' in attrs).toBe(false)
  expect('accounts_hidden' in attrs).toBe(false)
})

it('omits the signup cohort when the user has not loaded', () => {
  const qc = new QueryClient()
  setAnalyticsQueryClient(qc)

  const attrs = profileAttributes()
  expect('signup_year' in attrs).toBe(false)
  expect('signup_month' in attrs).toBe(false)
})

it('is empty with no client', () => {
  setAnalyticsQueryClient(null)
  expect(profileAttributes()).toEqual({})
})
