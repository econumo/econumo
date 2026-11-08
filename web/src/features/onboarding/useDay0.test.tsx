import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { http, HttpResponse, delay } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser } from '@/test/fixtures'
import { useDay0 } from './useDay0'
import { loadDraft, saveDraft, emptyDraft, type AccountRow, type CategoryRow } from './draft'

const pendingUser = {
  ...fixtureUser,
  options: fixtureUser.options.map((o) => (o.name === 'onboarding' ? { ...o, value: '' } : o)),
}

const rows: AccountRow[] = [
  { key: 'r1', name: 'Cash', balance: '', currencyId: 'cur-usd', savings: false, icon: 'payments' },
  { key: 'r2', name: 'Savings', balance: '100', currencyId: 'cur-eur', savings: true, icon: 'savings' },
]
const cats: CategoryRow[] = [
  { key: 'c1', name: 'Food', type: 'expense', icon: 'restaurant', checked: true },
  { key: 'c2', name: 'Salary', type: 'income', icon: 'work', checked: true },
  { key: 'c3', name: 'Skipped', type: 'expense', icon: 'x', checked: false },
]

function setup() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  return renderHook(() => useDay0(), { wrapper })
}

const ok = (data: unknown = {}) => HttpResponse.json({ success: true, message: '', data })
const userEnvelope = () => ok({ user: pendingUser })
const locked = () => HttpResponse.json({ success: false, message: 'Operation is locked', code: 400, errors: {} }, { status: 400 })
const fail = (status = 500) => HttpResponse.json({ success: false, message: 'boom' }, { status })

type Faults = {
  accountResponse?: 'http500' | 'lost' // applied to the 2nd / 1st account create, once
  categoryLost?: boolean
  budgetLost?: boolean
  hideStatus?: number
  accountsDelayMs?: number
}

let calls: { path: string; body: any }[]
let srv: { accounts: any[]; categories: any[]; budgets: any[]; folders: any[]; ops: Set<string> }

function mockWrites(f: Faults = {}) {
  srv = { accounts: [], categories: [], budgets: [], folders: [], ops: new Set() }
  let accountCalls = 0
  let accountFault = false
  let categoryFault = false
  let budgetFault = false
  server.use(
    http.get('*/api/v1/account/get-account-list', async () => {
      if (f.accountsDelayMs) await delay(f.accountsDelayMs)
      return ok({ items: srv.accounts })
    }),
    http.get('*/api/v1/account/get-folder-list', () => ok({ items: srv.folders })),
    http.get('*/api/v1/category/get-category-list', () => ok({ items: srv.categories })),
    http.get('*/api/v1/budget/get-budget-list', () => ok({ items: srv.budgets })),
    ...coreHandlers({ user: pendingUser }),
    http.post('*/api/v1/account/create-account', async ({ request }) => {
      const body = (await request.json()) as any
      calls.push({ path: 'create-account', body })
      if (srv.ops.has(body.id)) return locked()
      if (!body.folderId && srv.folders.length > 0) {
        return HttpResponse.json({ success: false, message: 'invalid', code: 400, errors: { folderId: ['This value should not be blank.'] } }, { status: 400 })
      }
      accountCalls++
      if (f.accountResponse === 'http500' && accountCalls === 2 && !accountFault) {
        accountFault = true
        return fail()
      }
      srv.ops.add(body.id)
      if (!body.folderId) srv.folders.push({ id: 'folder-general', name: 'General' })
      const item = { id: `acc-${srv.accounts.length + 1}`, name: body.name, owner: { id: 'u1' }, folderId: body.folderId || 'folder-general' }
      srv.accounts.push(item)
      if (f.accountResponse === 'lost' && accountCalls === 1 && !accountFault) {
        accountFault = true
        return HttpResponse.error()
      }
      return ok({ item, transaction: null })
    }),
    http.post('*/api/v1/user/update-currency', async ({ request }) => {
      calls.push({ path: 'update-currency', body: await request.json() })
      return userEnvelope()
    }),
    http.post('*/api/v1/currency/hide-all-currencies', () => {
      calls.push({ path: 'hide-all', body: null })
      return f.hideStatus ? fail(f.hideStatus) : ok()
    }),
    http.post('*/api/v1/currency/show-currency', async ({ request }) => {
      calls.push({ path: 'show-currency', body: await request.json() })
      return ok()
    }),
    http.post('*/api/v1/category/create-category', async ({ request }) => {
      const body = (await request.json()) as any
      calls.push({ path: 'create-category', body })
      if (srv.ops.has(body.id)) return locked()
      srv.ops.add(body.id)
      const item = { id: `cat-${srv.categories.length + 1}`, name: body.name, type: body.type }
      srv.categories.push(item)
      if (f.categoryLost && !categoryFault) {
        categoryFault = true
        return HttpResponse.error()
      }
      return ok({ item })
    }),
    http.post('*/api/v1/budget/create-budget', async ({ request }) => {
      const body = (await request.json()) as any
      calls.push({ path: 'create-budget', body })
      if (srv.budgets.some((b) => b.id === body.id)) return HttpResponse.json({ success: false, message: 'in use', code: 400, errors: { id: ['x'] } }, { status: 400 })
      srv.budgets.push({ id: body.id, ownerUserId: 'u1', access: [] })
      if (f.budgetLost && !budgetFault) {
        budgetFault = true
        return HttpResponse.error()
      }
      return ok({ item: { meta: { id: body.id } } })
    }),
  )
}

const ready = async () => {
  const hook = setup()
  await waitFor(() => expect(hook.result.current.ready).toBe(true))
  return hook
}
const paths = () => calls.map((c) => c.path)

beforeEach(() => {
  localStorage.clear()
  calls = []
})

describe('useDay0', () => {
  it('saveAccounts creates accounts and sets currencies', async () => {
    mockWrites()
    const { result } = await ready()
    await act(() => result.current.saveAccounts(rows, [], 'cur-usd'))
    expect(paths()).toEqual(['create-account', 'create-account', 'update-currency', 'hide-all', 'show-currency', 'show-currency'])
    expect(calls[0].body).toMatchObject({ name: 'Cash', balance: '0', folderId: null, currencyId: 'cur-usd' })
    expect(calls[2].body).toEqual({ currency: 'USD' })
    const draft = loadDraft('u1')!
    expect(draft.accountIds).toEqual(['acc-1', 'acc-2'])
    expect(draft.savingsAccountIds).toEqual(['acc-2'])
    expect(draft.mainCurrencyId).toBe('cur-usd')
  })

  it('saveAccounts never creates a row that stands for an existing account', async () => {
    mockWrites()
    const { result } = await ready()
    const existing: AccountRow = { key: 'e1', name: 'Imported', balance: '', currencyId: 'cur-eur', savings: true, icon: 'savings', accountId: 'acc-imp' }
    await act(() => result.current.saveAccounts([existing, rows[0]], [], 'cur-usd'))
    expect(calls.filter((c) => c.path === 'create-account').map((c) => c.body.name)).toEqual(['Cash'])
    expect(calls.filter((c) => c.path === 'show-currency').map((c) => c.body.id).sort()).toEqual(['cur-eur', 'cur-usd'])
    const d = loadDraft('u1')!
    expect(d.accountIds).toEqual(['acc-imp', 'acc-1'])
    expect(d.savingsAccountIds).toEqual(['acc-imp'])
  })

  it('a second save with the same currencies does not apply them again, a changed main does', async () => {
    mockWrites()
    const { result } = await ready()
    await act(() => result.current.saveAccounts(rows, [], 'cur-usd'))
    calls = []
    await act(() => result.current.saveAccounts(rows, [], 'cur-usd'))
    expect(paths()).toEqual([])
    await act(() => result.current.saveAccounts(rows, [], 'cur-eur'))
    expect(paths()).toEqual(['update-currency', 'hide-all', 'show-currency', 'show-currency'])
  })

  it('three accounts in one save reuse the folder created by the first', async () => {
    mockWrites()
    const { result } = await ready()
    const three = [...rows, { key: 'r3', name: 'Card', balance: '5', currencyId: 'cur-usd', savings: false, icon: 'credit_card' }]
    await act(() => result.current.saveAccounts(three, [], 'cur-usd'))
    const creates = calls.filter((c) => c.path === 'create-account').map((c) => c.body.folderId)
    expect(creates).toEqual([null, 'folder-general', 'folder-general'])
    expect(loadDraft('u1')!.accountIds).toEqual(['acc-1', 'acc-2', 'acc-3'])
  })

  it('resuming after the first account was created still sends the folder id', async () => {
    mockWrites({ accountResponse: 'http500' })
    const { result } = await ready()
    await act(async () => {
      await expect(result.current.saveAccounts(rows, [], 'cur-usd')).rejects.toBeTruthy()
    })
    calls = []
    await act(() => result.current.saveAccounts(rows, [], 'cur-usd'))
    expect(calls.filter((c) => c.path === 'create-account').map((c) => c.body.folderId)).toEqual(['folder-general'])
  })

  it('retry after a 500 mid-accounts completes without duplicates', async () => {
    mockWrites({ accountResponse: 'http500' })
    const { result } = await ready()
    await act(async () => {
      await expect(result.current.saveAccounts(rows, [], 'cur-usd')).rejects.toBeTruthy()
    })
    expect(Object.keys(loadDraft('u1')!.created)).toEqual(['r1'])
    await act(() => result.current.saveAccounts(rows, [], 'cur-usd'))
    expect(srv.accounts.map((a) => a.name)).toEqual(['Cash', 'Savings'])
    expect(calls.filter((c) => c.path === 'create-account')).toHaveLength(3)
    expect(loadDraft('u1')!.accountIds).toEqual(['acc-1', 'acc-2'])
  })

  it('retry after a lost response reconciles by name without duplicates', async () => {
    mockWrites({ accountResponse: 'lost' })
    const { result } = await ready()
    await act(async () => {
      await expect(result.current.saveAccounts(rows, [], 'cur-usd')).rejects.toBeTruthy()
    })
    await act(() => result.current.saveAccounts(rows, [], 'cur-usd'))
    expect(srv.accounts.map((a) => a.name)).toEqual(['Cash', 'Savings'])
    const d = loadDraft('u1')!
    expect(d.created).toEqual({ r1: 'acc-1', r2: 'acc-2' })
    expect(d.savingsAccountIds).toEqual(['acc-2'])
  })

  it('a reloaded hook reuses the persisted rows and keys', async () => {
    mockWrites({ accountResponse: 'http500' })
    const first = await ready()
    await act(async () => {
      await expect(first.result.current.saveAccounts(rows, [], 'cur-usd')).rejects.toBeTruthy()
    })
    first.unmount()
    const second = await ready()
    expect(second.result.current.draft?.rows.map((r) => r.key)).toEqual(['r1', 'r2'])
    await act(() => second.result.current.saveAccounts(second.result.current.draft!.rows, [], 'cur-usd'))
    expect(srv.accounts).toHaveLength(2)
  })

  it('ignores a 400 but not a 5xx from hide-all-currencies', async () => {
    mockWrites({ hideStatus: 400 })
    const a = await ready()
    await act(() => a.result.current.saveAccounts(rows, [], 'cur-usd'))
    expect(calls.filter((c) => c.path === 'show-currency').map((c) => c.body.id).sort()).toEqual(['cur-eur', 'cur-usd'])
    a.unmount()
    localStorage.clear()
    for (const status of [401, 404, 429, 500]) {
      mockWrites({ hideStatus: status })
      const b = await ready()
      await act(async () => {
        await expect(b.result.current.saveAccounts(rows, [], 'cur-usd')).rejects.toBeTruthy()
      })
      b.unmount()
      localStorage.clear()
    }
  })

  it('saveCategories creates only the checked categories and touches nothing else', async () => {
    mockWrites()
    const { result } = await ready()
    await act(() => result.current.saveCategories(cats))
    expect(paths()).toEqual(['create-category', 'create-category'])
    expect(calls.map((c) => c.body.name)).toEqual(['Food', 'Salary'])
    expect(loadDraft('u1')!.createdCategories).toEqual({ c1: 'cat-1', c2: 'cat-2' })
  })

  it('saveCategories persists the submitted rows before its first request', async () => {
    mockWrites()
    server.use(http.post('*/api/v1/category/create-category', () => fail()))
    const { result } = await ready()
    await act(async () => {
      await result.current.saveCategories(cats).catch(() => undefined)
    })
    expect(loadDraft('u1')?.categoryRows).toEqual(cats)
    expect(result.current.error).toBeTruthy()
  })

  it('a lost category response reconciles on retry without duplicates', async () => {
    mockWrites({ categoryLost: true })
    const { result } = await ready()
    await act(async () => {
      await expect(result.current.saveCategories(cats)).rejects.toBeTruthy()
    })
    await act(() => result.current.saveCategories(cats))
    expect(srv.categories).toHaveLength(2)
    expect(loadDraft('u1')!.createdCategories).toEqual({ c1: 'cat-1', c2: 'cat-2' })
  })

  it('a stored draft with a category already created resumes without creating it again', async () => {
    mockWrites()
    saveDraft('u1', { ...emptyDraft(), ops: { 'category:c1': 'op-c1' }, categoryRows: cats, createdCategories: { c1: 'cat-x' } })
    const { result } = await ready()
    await act(() => result.current.saveCategories(cats))
    expect(calls.map((c) => c.body.name)).toEqual(['Salary'])
  })

  it('ensureBudget creates one budget over every owned account, savings from the draft', async () => {
    mockWrites()
    srv.accounts.push(
      { id: 'acc-imp', name: 'Imported', owner: { id: 'u1' } },
      { id: 'acc-sav', name: 'Savings', owner: { id: 'u1' } },
      { id: 'acc-other', name: 'Shared', owner: { id: 'u2' } },
    )
    saveDraft('u1', { ...emptyDraft(), savingsAccountIds: ['acc-sav', 'acc-gone'], mainCurrencyId: 'cur-eur' })
    const { result } = await ready()
    let id = ''
    await act(async () => {
      id = await result.current.ensureBudget()
    })
    expect(paths()).toEqual(['create-budget'])
    expect(calls[0].body).toMatchObject({
      id,
      name: 'My budget',
      startDate: '',
      currencyId: 'cur-eur',
      accountIds: ['acc-imp', 'acc-sav'],
      savingsAccountIds: ['acc-sav'],
    })
    let again = ''
    await act(async () => {
      again = await result.current.ensureBudget()
    })
    expect(again).toBe(id)
    expect(calls.filter((c) => c.path === 'create-budget')).toHaveLength(1)
  })

  it('ensureBudget returns the default budget the user already has', async () => {
    mockWrites()
    srv.budgets.push({ id: 'b-old', ownerUserId: 'u1', access: [] }, { id: 'b-default', ownerUserId: 'u1', access: [] })
    server.use(
      ...coreHandlers({
        user: { ...pendingUser, options: pendingUser.options.map((o) => (o.name === 'budget' ? { ...o, value: 'b-default' } : o)) },
        budgets: srv.budgets,
        accounts: srv.accounts,
      }),
    )
    const { result } = await ready()
    let id = ''
    await act(async () => {
      id = await result.current.ensureBudget()
    })
    expect(id).toBe('b-default')
    expect(paths()).toEqual([])
  })

  it('ensureBudget falls back to the main currency option and refuses without accounts', async () => {
    mockWrites()
    const { result } = await ready()
    await act(async () => {
      await expect(result.current.ensureBudget()).rejects.toBeTruthy()
    })
    expect(paths()).toEqual([])
    srv.accounts.push({ id: 'acc-1', name: 'Cash', owner: { id: 'u1' } })
    await act(async () => {
      await result.current.ensureBudget()
    })
    expect(calls[0].body).toMatchObject({ currencyId: 'cur-usd', accountIds: ['acc-1'] })
  })

  it('a lost budget response reconciles on retry without a second budget', async () => {
    mockWrites({ budgetLost: true })
    srv.accounts.push({ id: 'acc-1', name: 'Cash', owner: { id: 'u1' } })
    const { result } = await ready()
    await act(async () => {
      await expect(result.current.ensureBudget()).rejects.toBeTruthy()
    })
    let id = ''
    await act(async () => {
      id = await result.current.ensureBudget()
    })
    expect(srv.budgets).toHaveLength(1)
    expect(srv.budgets[0].id).toBe(id)
    expect(calls.filter((c) => c.path === 'create-budget')).toHaveLength(1)
  })

  it('ready waits for the accounts query', async () => {
    mockWrites({ accountsDelayMs: 300 })
    const { result } = setup()
    await new Promise((r) => setTimeout(r, 100))
    expect(result.current.ready).toBe(false)
    await waitFor(() => expect(result.current.ready).toBe(true))
  })
})
