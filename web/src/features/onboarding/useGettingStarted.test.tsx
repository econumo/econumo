import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureOwner, fixtureUser } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { hasRealTransactions, markGettingStartedDone, withStepState } from './gettingStarted'
import { HOME_SCREEN_KEY, useGettingStarted } from './useGettingStarted'

const IPHONE = 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1'
const ANDROID_FIREFOX = 'Mozilla/5.0 (Android 14; Mobile; rv:130.0) Gecko/130.0 Firefox/130.0'
const ANDROID_CHROME = 'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Mobile Safari/537.36'
const DESKTOP = 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36'

const userWith = (value: string | null) => ({
  ...fixtureUser,
  options: value === null ? fixtureUser.options : [...fixtureUser.options, { name: 'getting_started', value }],
})

const correction = (id: string, accountId: string) => ({
  id, author: fixtureOwner, type: 'income', accountId, accountRecipientId: null, amount: '100', amountRecipient: '100',
  categoryId: null, description: '', payeeId: null, tagId: null, labelIds: [], recurringId: null, date: '2026-07-01 00:00:00',
})

function setDevice(ua: string, compact: boolean) {
  vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue(ua)
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q.includes('max-width') ? compact : false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function setup(user: unknown, extra: Record<string, unknown> = {}) {
  server.use(...coreHandlers({ user, transactions: [correction('c1', 'a1')], ...extra }))
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  return { queryClient, ...renderHook(() => useGettingStarted(), { wrapper }) }
}

const keys = (steps: { key: string }[]) => steps.map((s) => s.key)

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  setDevice(DESKTOP, false)
})

afterEach(() => vi.restoreAllMocks())

it('is inactive without the getting_started option', async () => {
  const { result } = setup(userWith(null))
  await waitFor(() => expect(result.current.hasData).toBe(false))
  expect(result.current.active).toBe(false)
})

it('gives desktop the plan and import cards', async () => {
  const { result } = setup(userWith(''))
  await waitFor(() => expect(result.current.active).toBe(true))
  expect(keys(result.current.steps)).toEqual(['plan', 'import'])
  expect(keys(result.current.inboxSteps)).toEqual(['plan', 'import'])
})

it('gives an iPhone home screen and Apple Wallet, with plan and import in the Inbox', async () => {
  setDevice(IPHONE, true)
  const { result } = setup(userWith(''))
  await waitFor(() => expect(result.current.active).toBe(true))
  expect(keys(result.current.steps)).toEqual(['home_screen', 'apple_wallet'])
  expect(keys(result.current.inboxSteps)).toEqual(['home_screen', 'apple_wallet', 'plan', 'import'])
})

it('gives an Android phone home screen only', async () => {
  setDevice(ANDROID_FIREFOX, true)
  const { result } = setup(userWith(''))
  await waitFor(() => expect(result.current.active).toBe(true))
  expect(keys(result.current.steps)).toEqual(['home_screen'])
})

it('leaves out home screen where the browser cannot install', async () => {
  setDevice(ANDROID_CHROME, true)
  const { result } = setup(userWith(''))
  await waitFor(() => expect(result.current.active).toBe(true))
  expect(keys(result.current.steps)).toEqual([])
  expect(keys(result.current.inboxSteps)).toEqual(['plan', 'import'])
})

it('reads done and dismissed from the option', async () => {
  const { result } = setup(userWith('plan,!import'))
  await waitFor(() => expect(result.current.active).toBe(true))
  expect(result.current.steps).toEqual([
    { key: 'plan', done: true, dismissed: false },
    { key: 'import', done: false, dismissed: true },
  ])
})

it('marks Apple Wallet done once a Wallet source exists, and home screen from this device', async () => {
  setDevice(IPHONE, true)
  localStorage.setItem(HOME_SCREEN_KEY, 'dismissed')
  const { result } = setup(userWith(''), { importSources: [{ id: 's1', provider: 'apple-wallet', name: 'Wallet', cards: [] }] })
  await waitFor(() => expect(result.current.steps[1]?.done).toBe(true))
  expect(result.current.steps[0]).toEqual({ key: 'home_screen', done: false, dismissed: true })
})

it('dismissing home screen stays on this device and sends nothing', async () => {
  setDevice(IPHONE, true)
  const posts: unknown[] = []
  server.use(http.post('*/api/v1/user/update-getting-started', async ({ request }) => {
    posts.push(await request.json())
    return HttpResponse.json({ success: true, message: '', data: { user: userWith('') } })
  }))
  const { result } = setup(userWith(''))
  await waitFor(() => expect(result.current.active).toBe(true))
  act(() => result.current.dismiss('home_screen'))
  expect(result.current.steps[0]?.dismissed).toBe(true)
  expect(localStorage.getItem(HOME_SCREEN_KEY)).toBe('dismissed')
  expect(posts).toEqual([])
})

it('two quick dismissals keep both', async () => {
  const posts: unknown[] = []
  server.use(http.post('*/api/v1/user/update-getting-started', async ({ request }) => {
    posts.push(await request.json())
    return HttpResponse.json({ success: true, message: '', data: { user: userWith('') } })
  }))
  const { result } = setup(userWith(''))
  await waitFor(() => expect(result.current.active).toBe(true))
  act(() => {
    result.current.dismiss('plan')
    result.current.dismiss('import')
  })
  await waitFor(() => expect(posts).toEqual([{ value: '!plan' }, { value: '!plan,!import' }]))
  expect(result.current.steps.every((s) => s.dismissed)).toBe(true)
})

it('hasData is false with only an opening correction and true after a categorized transaction', async () => {
  const { result } = setup(userWith(''))
  await waitFor(() => expect(result.current.active).toBe(true))
  await waitFor(() => expect(result.current.hasData).toBe(false))
  const real = { ...correction('t9', 'a1'), categoryId: 'cat-salary' }
  expect(hasRealTransactions([correction('c1', 'a1')] as never)).toBe(false)
  expect(hasRealTransactions([correction('c1', 'a1'), correction('c2', 'a2')] as never)).toBe(false)
  expect(hasRealTransactions([correction('c1', 'a1'), correction('c2', 'a1')] as never)).toBe(true)
  expect(hasRealTransactions([correction('c1', 'a1'), real] as never)).toBe(true)
  expect(hasRealTransactions([{ ...correction('c1', 'a1'), description: 'x' }] as never)).toBe(true)
})

it('withStepState replaces an earlier answer for the same key', () => {
  expect(withStepState('', 'plan', 'done')).toBe('plan')
  expect(withStepState('!plan,import', 'plan', 'done')).toBe('import,plan')
  expect(withStepState(null, 'import', 'dismissed')).toBe('!import')
})

it('markGettingStartedDone sends nothing outside the flow or once answered', async () => {
  const posts: unknown[] = []
  server.use(http.post('*/api/v1/user/update-getting-started', async ({ request }) => {
    posts.push(await request.json())
    return HttpResponse.json({ success: true, message: '', data: { user: userWith('plan') } })
  }))
  const queryClient = new QueryClient()
  queryClient.setQueryData(queryKeys.user, userWith(null))
  markGettingStartedDone(queryClient, 'plan')
  queryClient.setQueryData(queryKeys.user, userWith('!plan'))
  markGettingStartedDone(queryClient, 'plan')
  queryClient.setQueryData(queryKeys.user, userWith(''))
  markGettingStartedDone(queryClient, 'plan')
  markGettingStartedDone(queryClient, 'plan')
  await waitFor(() => expect(posts).toEqual([{ value: 'plan' }]))
})
