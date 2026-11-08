import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser, fixtureUsd, fixtureEur, fixtureAccounts } from '@/test/fixtures'
import { useDay0 } from './useDay0'
import { loadDraft, saveDraft, type AccountRow, type CategoryRow } from './draft'

const pendingUser = {
  ...fixtureUser,
  options: fixtureUser.options.map((o) => (o.name === 'onboarding' ? { ...o, value: '' } : o)),
}

const rows: AccountRow[] = [
  { key: 'r1', name: 'Cash', balance: '', currencyId: 'cur-usd', savings: false, icon: 'payments' },
  { key: 'r2', name: 'Savings', balance: '100', currencyId: 'cur-eur', savings: true, icon: 'savings' },
]

function setup() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  return renderHook(() => useDay0(), { wrapper })
}

const ok = (data: unknown = {}) => HttpResponse.json({ success: true, message: '', data })
const userEnvelope = () => ok({ user: pendingUser })

let calls: { path: string; body: any }[]

function mockWrites(opts: { failSecondAccountOnce?: boolean; hideStatus?: number } = {}) {
  let accountCalls = 0
  server.use(
    ...coreHandlers({ user: pendingUser, accounts: [] }),
    http.post('*/api/v1/account/create-account', async ({ request }) => {
      const body = (await request.json()) as any
      calls.push({ path: 'create-account', body })
      accountCalls++
      if (opts.failSecondAccountOnce && accountCalls === 2) {
        return HttpResponse.json({ success: false, message: 'boom' }, { status: 500 })
      }
      return ok({ item: { id: `acc-${body.name}`, name: body.name }, transaction: null })
    }),
    http.post('*/api/v1/user/update-currency', async ({ request }) => {
      calls.push({ path: 'update-currency', body: await request.json() })
      return userEnvelope()
    }),
    http.post('*/api/v1/currency/hide-all-currencies', () => {
      calls.push({ path: 'hide-all', body: null })
      return opts.hideStatus ? HttpResponse.json({ success: false, message: 'no' }, { status: opts.hideStatus }) : ok()
    }),
    http.post('*/api/v1/currency/show-currency', async ({ request }) => {
      calls.push({ path: 'show-currency', body: await request.json() })
      return ok()
    }),
    http.post('*/api/v1/category/create-category', async ({ request }) => {
      calls.push({ path: 'create-category', body: await request.json() })
      return ok({ item: {} })
    }),
    http.post('*/api/v1/budget/create-budget', async ({ request }) => {
      calls.push({ path: 'create-budget', body: await request.json() })
      return ok({ item: { meta: {} } })
    }),
    http.post('*/api/v1/user/update-getting-started', async ({ request }) => {
      calls.push({ path: 'update-getting-started', body: await request.json() })
      return userEnvelope()
    }),
    http.post('*/api/v1/user/complete-onboarding', () => {
      calls.push({ path: 'complete-onboarding', body: null })
      return userEnvelope()
    }),
  )
}

beforeEach(() => {
  localStorage.clear()
  calls = []
})

describe('useDay0', () => {
  it('saveAccounts creates accounts, sets currencies, moves to categories', async () => {
    mockWrites()
    const { result } = setup()
    await waitFor(() => expect(result.current.ready).toBe(true))
    await act(() => result.current.saveAccounts(rows, [], 'cur-usd'))
    expect(calls.map((c) => c.path)).toEqual(['create-account', 'create-account', 'update-currency', 'hide-all', 'show-currency', 'show-currency'])
    expect(calls[0].body).toMatchObject({ name: 'Cash', balance: '0', folderId: null, currencyId: 'cur-usd' })
    expect(calls[2].body).toEqual({ currency: 'USD' })
    const draft = loadDraft('u1')!
    expect(draft.step).toBe('categories')
    expect(draft.accountIds).toEqual(['acc-Cash', 'acc-Savings'])
    expect(draft.savingsAccountIds).toEqual(['acc-Savings'])
    expect(draft.mainCurrencyId).toBe('cur-usd')
  })

  it('a retry after a failure re-sends the same operation ids', async () => {
    mockWrites({ failSecondAccountOnce: true })
    const { result } = setup()
    await waitFor(() => expect(result.current.ready).toBe(true))
    await act(async () => {
      await expect(result.current.saveAccounts(rows, [], 'cur-usd')).rejects.toBeTruthy()
    })
    const first = calls.filter((c) => c.path === 'create-account').map((c) => c.body.id)
    expect(first).toHaveLength(2)
    calls.length = 0
    await act(() => result.current.saveAccounts(rows, [], 'cur-usd'))
    const second = calls.filter((c) => c.path === 'create-account').map((c) => c.body.id)
    expect(second).toEqual(first)
  })

  it('ignores a 4xx from hide-all-currencies', async () => {
    mockWrites({ hideStatus: 400 })
    const { result } = setup()
    await waitFor(() => expect(result.current.ready).toBe(true))
    await act(() => result.current.saveAccounts(rows, [], 'cur-usd'))
    expect(calls.filter((c) => c.path === 'show-currency').map((c) => c.body.id).sort()).toEqual(['cur-eur', 'cur-usd'])
  })

  it('finish creates categories, then the budget, then completes', async () => {
    mockWrites()
    saveDraft('u1', { step: 'categories', ops: {}, accountIds: ['a1', 'a2'], savingsAccountIds: ['a2'], mainCurrencyId: 'cur-usd' })
    const { result } = setup()
    await waitFor(() => expect(result.current.ready).toBe(true))
    const cats: CategoryRow[] = [
      { key: 'c1', name: 'Food', type: 'expense', icon: 'restaurant', checked: true },
      { key: 'c2', name: 'Salary', type: 'income', icon: 'work', checked: true },
      { key: 'c3', name: 'Skipped', type: 'expense', icon: 'x', checked: false },
    ]
    await act(() => result.current.finish(cats))
    expect(calls.map((c) => c.path)).toEqual(['create-category', 'create-category', 'create-budget', 'update-getting-started', 'complete-onboarding'])
    const budget = calls[2].body
    expect(budget).toMatchObject({ name: 'My budget', startDate: '', currencyId: 'cur-usd', accountIds: ['a1', 'a2'], savingsAccountIds: ['a2'] })
    expect(calls[3].body).toEqual({ value: '' })
    expect(loadDraft('u1')).toBeNull()
  })

  it('skip clears getting started and completes onboarding', async () => {
    mockWrites()
    const { result } = setup()
    await waitFor(() => expect(result.current.ready).toBe(true))
    await act(() => result.current.skip())
    expect(calls.map((c) => c.path)).toEqual(['update-getting-started', 'complete-onboarding'])
    expect(loadDraft('u1')).toBeNull()
  })

  it('resumes at categories when accounts exist but there is no draft', async () => {
    mockWrites()
    server.use(...coreHandlers({ user: pendingUser, accounts: fixtureAccounts, currencies: [fixtureUsd, fixtureEur] }))
    const { result } = setup()
    await waitFor(() => expect(result.current.draft?.step).toBe('categories'))
    const owned = fixtureAccounts.filter((a) => a.owner.id === 'u1').map((a) => a.id)
    expect(result.current.draft?.accountIds).toEqual(owned)
  })
})
