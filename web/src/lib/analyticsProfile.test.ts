import { QueryClient } from '@tanstack/react-query'
import { queryKeys } from '@/app/queryKeys'
import { profileAttributes, setAnalyticsQueryClient } from './analyticsProfile'

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
    { id: 'a1', folderId: 'f1' },
    { id: 'a2', folderId: 'f2' },
    { id: 'a3', folderId: null },
  ])
  qc.setQueryData(queryKeys.categories, [
    { id: 'c1', isArchived: 0 },
    { id: 'c2', isArchived: 1 },
  ])
  qc.setQueryData(queryKeys.user, { id: 'u1', createdAt: '2026-02-17 08:30:00' })
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

it('omits accounts_hidden when folders have not loaded', () => {
  const qc = new QueryClient()
  qc.setQueryData(queryKeys.accounts, [{ id: 'a1', folderId: null }])
  setAnalyticsQueryClient(qc)

  const attrs = profileAttributes()
  expect(attrs.accounts).toBe(1)
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
