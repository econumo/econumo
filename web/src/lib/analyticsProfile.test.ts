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

function budget(id: string, ownerUserId: string, access: { user: { id: string }; isAccepted: 0 | 1 }[] = [], isArchived: 0 | 1 = 0) {
  return { id, ownerUserId, access, isArchived }
}

afterEach(() => {
  setAnalyticsQueryClient(null)
  vi.useRealTimers()
})

it('counts every item the cache holds, hidden and archived included, and omits what it does not', () => {
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
  expect(attrs.categories).toBe(2)
  expect(attrs.signup_year).toBe(2026)
  // Never loaded, so never guessed at.
  for (const key of ['tags', 'labels', 'budgets', 'connections']) {
    expect(key in attrs).toBe(false)
  }
})

it('counts payees, tags, labels and connections when loaded', () => {
  const qc = new QueryClient()
  qc.setQueryData(queryKeys.payees, [
    { id: 'p1', isArchived: 0 },
    { id: 'p2', isArchived: 0 },
    { id: 'p3', isArchived: 1 },
  ])
  qc.setQueryData(queryKeys.tags, [{ id: 't1', isArchived: 0 }])
  qc.setQueryData(queryKeys.labels, [
    { id: 'l1', isArchived: 1 },
    { id: 'l2', isArchived: 0 },
  ])
  qc.setQueryData(queryKeys.connections, [{ user: { id: 'u2' } }, { user: { id: 'u3' } }])
  setAnalyticsQueryClient(qc)

  const attrs = profileAttributes()
  expect(attrs.payees).toBe(3)
  expect(attrs.tags).toBe(1)
  expect(attrs.labels).toBe(2)
  expect(attrs.connections).toBe(2)
})

it('no longer sends the hidden and archived breakdowns', () => {
  const qc = new QueryClient()
  qc.setQueryData(queryKeys.folders, [{ id: 'f1', name: 'Hidden', position: 0, isVisible: 0 }])
  qc.setQueryData(queryKeys.accounts, [account({ id: 'a1', folderId: 'f1' })])
  qc.setQueryData(queryKeys.categories, [{ id: 'c1', isArchived: 1 }])
  qc.setQueryData(queryKeys.payees, [{ id: 'p1', isArchived: 1 }])
  qc.setQueryData(queryKeys.tags, [{ id: 't1', isArchived: 1 }])
  qc.setQueryData(queryKeys.user, { id: ME, createdAt: '2026-02-17 08:30:00' })
  setAnalyticsQueryClient(qc)

  const attrs = profileAttributes()
  for (const key of ['accounts_hidden', 'categories_archived', 'payees_archived', 'tags_archived', 'signup_month']) {
    expect(key in attrs).toBe(false)
  }
})

it('counts owned and accepted budgets, archived included, but not an unaccepted invite', () => {
  const qc = new QueryClient()
  qc.setQueryData(queryKeys.budgets, [
    budget('b1', ME),
    budget('b2', ME, [], 1),
    budget('b3', 'u9', [{ user: { id: ME }, isAccepted: 1 }]),
    budget('b4-invite', 'u9', [{ user: { id: ME }, isAccepted: 0 }]),
  ])
  qc.setQueryData(queryKeys.user, { id: ME })
  setAnalyticsQueryClient(qc)

  expect(profileAttributes().budgets).toBe(3)
})

it('omits budgets when the user has not loaded (invites cannot be filtered without "me")', () => {
  const qc = new QueryClient()
  qc.setQueryData(queryKeys.budgets, [budget('b1', ME)])
  setAnalyticsQueryClient(qc)

  expect('budgets' in profileAttributes()).toBe(false)
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

  expect(profileAttributes().accounts).toBe(1)
})

it('omits accounts when the user has not loaded (pending shares cannot be filtered without "me")', () => {
  const qc = new QueryClient()
  qc.setQueryData(queryKeys.accounts, [account({ id: 'a1', folderId: null })])
  setAnalyticsQueryClient(qc)

  expect('accounts' in profileAttributes()).toBe(false)
})

describe('months_since_signup', () => {
  function monthsOn(now: string, createdAt: string): number | undefined {
    vi.useFakeTimers()
    vi.setSystemTime(new Date(now))
    const qc = new QueryClient()
    qc.setQueryData(queryKeys.user, { id: ME, createdAt })
    setAnalyticsQueryClient(qc)
    return profileAttributes().months_since_signup
  }

  it.each([
    ['the signup month itself', '2026-02-28T00:00:00Z', 0],
    ['one day short of a full month', '2026-03-17T08:29:59Z', 0],
    ['exactly one month', '2026-03-17T08:30:00Z', 1],
    ['across a year boundary', '2027-01-20T00:00:00Z', 11],
    ['a year and a half', '2027-08-18T00:00:00Z', 18],
  ])('%s', (_, now, expected) => {
    expect(monthsOn(now, '2026-02-17 08:30:00')).toBe(expected)
  })

  // createdAt is UTC wall clock: a local-time parse would shift the boundary.
  it('reads createdAt as UTC', () => {
    expect(monthsOn('2026-03-31T23:30:00Z', '2026-02-28 23:59:59')).toBe(1)
    expect(monthsOn('2026-03-28T23:59:58Z', '2026-02-28 23:59:59')).toBe(0)
  })

  it('never goes negative when the clock is behind the server', () => {
    expect(monthsOn('2026-02-17T08:00:00Z', '2026-02-17 08:30:00')).toBe(0)
  })

  it('is omitted when the user has not loaded', () => {
    setAnalyticsQueryClient(new QueryClient())
    const attrs = profileAttributes()
    expect('signup_year' in attrs).toBe(false)
    expect('months_since_signup' in attrs).toBe(false)
  })
})

it('is empty with no client', () => {
  setAnalyticsQueryClient(null)
  expect(profileAttributes()).toEqual({})
})
