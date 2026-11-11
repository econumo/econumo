import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import type { CurrentUserDto } from '@/api/dto/user'
import { queryKeys } from '@/app/queryKeys'
import { METRICS, trackEvent } from '@/lib/metrics'
import { useDay0 } from './useDay0'
import { loadDraft, saveDraft, emptyDraft, type AccountRow, type CategoryRow } from './draft'
import { stepRoutes, userWith, type StepServer } from './onboardingRoutes.test-utils'

vi.mock('@/lib/metrics', async (importOriginal) => ({ ...(await importOriginal<typeof import('@/lib/metrics')>()), trackEvent: vi.fn() }))

const rows: AccountRow[] = [
  { key: 'r1', name: 'Cash', balance: '0', currencyId: 'cur-usd', icon: 'payments' },
  { key: 'r2', name: 'Savings', balance: '100', currencyId: 'cur-eur', icon: 'savings' },
]
const cats: CategoryRow[] = [
  { key: 'c1', name: 'Food', type: 'expense', icon: 'restaurant', labelKey: 'onboarding.categories.food', defaultKey: 'food' },
  { key: 'c2', name: 'Salary', type: 'income', icon: 'work' },
]

let srv: StepServer
let queryClient: QueryClient
let legacyCalls: string[]

function setup(option = '') {
  const routes = stepRoutes({ option })
  srv = routes.srv
  legacyCalls = []
  server.use(
    ...coreHandlers({ user: userWith(option) }),
    ...routes.handlers,
    // none of the per-row writes the step routes replaced may be called any more
    ...['category/create-category', 'category/sort-category-list', 'account/create-account', 'currency/hide-all-currencies', 'currency/show-currency', 'user/update-currency', 'budget/create-budget', 'budget/add-account', 'budget/set-limit', 'user/update-onboarding'].map((p) =>
      http.post(`*/api/v1/${p}`, () => {
        legacyCalls.push(p)
        return new Response(null, { status: 500 })
      }),
    ),
  )
  queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  return renderHook(() => useDay0(), { wrapper })
}

const ready = async (option = '') => {
  const hook = setup(option)
  await waitFor(() => expect(hook.result.current.ready).toBe(true))
  return hook
}
const calledTimes = (event: string) => vi.mocked(trackEvent).mock.calls.filter(([name]) => name === event).length
const onboardingOf = () => queryClient.getQueryData<CurrentUserDto>(queryKeys.user)?.options.find((o) => o.name === 'onboarding')?.value

beforeEach(() => {
  localStorage.clear()
  vi.mocked(trackEvent).mockClear()
})

afterEach(() => expect(legacyCalls).toEqual([]))

describe('useDay0', () => {
  it('saveCategories posts one save-categories with the rows and the order, and takes the user it answers with', async () => {
    const { result } = await ready()
    const order = ['c1', 'old-1', 'c2']
    await act(() => result.current.saveCategories(cats, order, ['food', 'rent']))
    expect(srv.calls).toHaveLength(1)
    expect(srv.calls[0].path).toBe('save-categories')
    expect(srv.calls[0].body).toEqual({
      id: expect.any(String),
      categories: [
        { key: 'c1', name: 'Food', type: 'expense', icon: 'restaurant' },
        { key: 'c2', name: 'Salary', type: 'income', icon: 'work' },
      ],
      order,
    })
    expect(onboardingOf()).toBe('categories')
    const draft = loadDraft('u1')!
    expect(draft.budgetId).toBe('b-onb')
    expect(draft.ops.categories).toBeUndefined()
    expect(draft.categoryOrder).toEqual(order)
  })

  it('saveCategories sends no order when the list was not reordered, and works with no new rows', async () => {
    const { result } = await ready()
    await act(() => result.current.saveCategories([]))
    expect(srv.calls[0].body).toEqual({ id: expect.any(String), categories: [] })
  })

  it('saveCategories fires the analytics events after the route succeeds', async () => {
    const { result } = await ready()
    await act(() => result.current.saveCategories(cats, undefined, ['food', 'rent']))
    expect(trackEvent).toHaveBeenCalledWith(METRICS.ONBOARDING_CATEGORY_REMOVED, { category: 'rent' })
    expect(trackEvent).toHaveBeenCalledWith(METRICS.ONBOARDING_CATEGORIES_COMPLETE, { renamed: '', custom: 1, offered: 2, archived: 0, restored: 0 })
    expect(calledTimes(METRICS.ONBOARDING_CATEGORIES_COMPLETE)).toBe(1)
    expect(calledTimes(METRICS.ONBOARDING_CATEGORY_REMOVED)).toBe(1)
  })

  it('a failed save-categories fires nothing, and the retry re-sends the same operation id', async () => {
    const { result } = await ready()
    srv.failNext.categories = 'http500'
    await act(async () => {
      await expect(result.current.saveCategories(cats)).rejects.toBeTruthy()
    })
    expect(trackEvent).not.toHaveBeenCalledWith(METRICS.ONBOARDING_CATEGORIES_COMPLETE, expect.anything())
    const first = loadDraft('u1')!
    expect(first.ops.categories).toBe(srv.calls[0].body.id)
    expect(first.categoryRows).toEqual(cats)
    await act(() => result.current.saveCategories(cats))
    expect(srv.calls.map((c) => c.body.id)).toEqual([first.ops.categories, first.ops.categories])
    expect(loadDraft('u1')!.ops.categories).toBeUndefined()
  })

  it('a response lost after the server committed is retried under the same id and answers with the same ids', async () => {
    const { result } = await ready()
    srv.failNext.categories = 'lost'
    await act(async () => {
      await expect(result.current.saveCategories(cats)).rejects.toBeTruthy()
    })
    await act(() => result.current.saveCategories(cats))
    expect(srv.calls[1].body.id).toBe(srv.calls[0].body.id)
    expect(loadDraft('u1')!.ops.categories).toBeUndefined()
  })

  it('a failed save refetches every query: the step may have committed with its answer lost', async () => {
    const { result } = await ready()
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
    srv.failNext.categories = 'lost'
    await act(async () => {
      await expect(result.current.saveCategories(cats)).rejects.toBeTruthy()
    })
    expect(invalidate).toHaveBeenCalledWith()
  })

  it('an operation id another write claimed is dropped, so the next Continue mints a fresh one', async () => {
    const { result } = await ready()
    server.use(
      http.post('*/api/v1/onboarding/save-categories', async ({ request }) => {
        srv.calls.push({ path: 'save-categories', body: await request.json() })
        return HttpResponse.json(
          { success: false, message: 'Validation failed', code: 400, errors: { id: ['The value you selected is not a valid choice.'] } },
          { status: 400 },
        )
      }, { once: true }),
    )
    await act(async () => {
      await expect(result.current.saveCategories(cats)).rejects.toBeTruthy()
    })
    expect(loadDraft('u1')!.ops.categories).toBeUndefined()
    await act(() => result.current.saveCategories(cats))
    expect(srv.calls).toHaveLength(2)
    expect(srv.calls[1].body.id).not.toBe(srv.calls[0].body.id)
    expect(onboardingOf()).toBe('categories')
  })

  it('a reloaded hook replays the operation id persisted by the failed submit', async () => {
    saveDraft('u1', { ...emptyDraft(), ops: { categories: 'op-cats' }, categoryRows: cats })
    const { result } = await ready()
    await act(() => result.current.saveCategories(cats))
    expect(srv.calls[0].body.id).toBe('op-cats')
  })

  it('a draft from the per-row flow drops its per-row ids', async () => {
    localStorage.setItem('econumo-day0:u1', JSON.stringify({ ops: { 'category:c1': 'op-c1', budget: 'b-old' }, categoryRows: cats }))
    expect(loadDraft('u1')!.ops).toEqual({})
  })

  it('saveAccounts posts the new rows with every visible currency and the main one', async () => {
    const { result } = await ready('categories')
    const existing: AccountRow = { key: 'e1', name: 'Imported', balance: '', currencyId: 'cur-chf', icon: 'savings', accountId: 'acc-imp' }
    await act(() => result.current.saveAccounts([existing, ...rows], ['cur-gbp'], 'cur-usd'))
    expect(srv.calls).toHaveLength(1)
    expect(srv.calls[0]).toEqual({
      path: 'save-accounts',
      body: {
        id: expect.any(String),
        accounts: [
          { key: 'r1', name: 'Cash', balance: '0', currencyId: 'cur-usd', icon: 'payments' },
          { key: 'r2', name: 'Savings', balance: '100', currencyId: 'cur-eur', icon: 'savings' },
        ],
        currencyIds: ['cur-gbp', 'cur-chf', 'cur-usd', 'cur-eur'],
        mainCurrencyId: 'cur-usd',
      },
    })
    expect(onboardingOf()).toBe('categories,accounts')
    const draft = loadDraft('u1')!
    expect(draft.rows).toHaveLength(3)
    expect(draft.mainCurrencyId).toBe('cur-usd')
    expect(draft.budgetId).toBe('b-onb')
    expect(trackEvent).toHaveBeenCalledWith(METRICS.ONBOARDING_ACCOUNTS_COMPLETE, { accounts: 2, currencies: 4, hidden: 0, shown: 0, currencyChanged: 0, balanceChanged: 0 })
    expect(calledTimes(METRICS.ONBOARDING_ACCOUNTS_COMPLETE)).toBe(1)
  })

  it('a network failure on save-accounts keeps the id, and the retry re-sends it', async () => {
    const { result } = await ready()
    srv.failNext.accounts = 'lost'
    await act(async () => {
      await expect(result.current.saveAccounts(rows, [], 'cur-usd')).rejects.toBeTruthy()
    })
    expect(trackEvent).not.toHaveBeenCalledWith(METRICS.ONBOARDING_ACCOUNTS_COMPLETE, expect.anything())
    await act(() => result.current.saveAccounts(rows, [], 'cur-usd'))
    expect(srv.calls[1].body.id).toBe(srv.calls[0].body.id)
    expect(loadDraft('u1')!.ops.accounts).toBeUndefined()
  })

  it('savePlan posts the amounts for the budget and takes the user it answers with', async () => {
    const { result } = await ready('categories,accounts')
    await act(() => result.current.savePlan('b1', [{ categoryId: 'cat-1', amount: '300' }]))
    expect(srv.calls).toEqual([{ path: 'save-plan', body: { id: expect.any(String), budgetId: 'b1', amounts: [{ categoryId: 'cat-1', amount: '300' }] } }])
    expect(onboardingOf()).toBe('categories,accounts,plan')
    expect(trackEvent).toHaveBeenCalledWith(METRICS.ONBOARDING_PLAN_COMPLETE, { rows: 1 })
    expect(calledTimes(METRICS.ONBOARDING_PLAN_COMPLETE)).toBe(1)
  })

  it('a step the server already marked answers with the current state and changes nothing', async () => {
    const { result } = await ready('categories')
    await act(() => result.current.saveCategories(cats))
    expect(onboardingOf()).toBe('categories')
    expect(loadDraft('u1')!.budgetId).toBeNull()
  })

  it('finishImport marks only the import step, keeps the budget for the plan step and invalidates the lists', async () => {
    const { result } = await ready('')
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
    await act(() => result.current.finishImport())
    expect(srv.calls).toEqual([{ path: 'finish-import', body: { id: expect.any(String) } }])
    expect(onboardingOf()).toBe('import')
    expect(loadDraft('u1')!.budgetId).toBe('b-onb')
    expect(loadDraft('u1')!.ops.import).toBeUndefined()
    expect(invalidate).toHaveBeenCalledWith({ predicate: expect.any(Function) })
    expect(trackEvent).toHaveBeenCalledWith(METRICS.ONBOARDING_IMPORT_FINISHED, { budget: true })
  })

  it('a lost finish-import answer is retried under the same operation id', async () => {
    const { result } = await ready('')
    srv.failNext.import = 'lost'
    await act(() => expect(result.current.finishImport()).rejects.toBeDefined())
    const first = loadDraft('u1')!.ops.import
    expect(first).toEqual(expect.any(String))
    expect(trackEvent).not.toHaveBeenCalledWith(METRICS.ONBOARDING_IMPORT_FINISHED, expect.anything())
    await act(() => result.current.finishImport())
    expect(srv.calls.map((c) => c.body.id)).toEqual([first, first])
    expect(onboardingOf()).toBe('import')
  })

  it('saveCategories and saveAccounts send the review diffs and count them', async () => {
    const { result } = await ready('import')
    await act(() => result.current.saveCategories([], undefined, [], { archive: ['cat-1', 'cat-2'], restore: ['cat-3'] }))
    await act(() => result.current.saveAccounts([], [], 'cur-usd', { hide: ['a1'], unhide: [] }))
    expect(srv.calls[0].body).toMatchObject({ categories: [], archive: ['cat-1', 'cat-2'], restore: ['cat-3'] })
    expect(srv.calls[1].body).toMatchObject({ accounts: [], hide: ['a1'] })
    expect(srv.calls[1].body.unhide).toBeUndefined()
    expect(trackEvent).toHaveBeenCalledWith(METRICS.ONBOARDING_CATEGORIES_COMPLETE, expect.objectContaining({ archived: 2, restored: 1 }))
    expect(trackEvent).toHaveBeenCalledWith(METRICS.ONBOARDING_ACCOUNTS_COMPLETE, expect.objectContaining({ hidden: 1, shown: 0 }))
  })

  it('ready waits for the accounts query', async () => {
    const hook = setup()
    expect(hook.result.current.ready).toBe(false)
    await waitFor(() => expect(hook.result.current.ready).toBe(true))
  })
})
