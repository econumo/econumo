import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureOwner, fixtureUser } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { hasRealTransactions, markGettingStartedDone, withStepState } from './gettingStarted'
import { HOME_SCREEN_KEY, markHomeScreenDone, useGettingStarted } from './useGettingStarted'
import { listenForInstallPrompt, resetInstallStateForTests } from '@/features/install/installPrompt'

const IPHONE = 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1'
const IPAD = 'Mozilla/5.0 (iPad; CPU OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1'
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

beforeAll(() => listenForInstallPrompt())

beforeEach(() => {
  resetInstallStateForTests()
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

it('gives an iPad home screen but not Apple Wallet, which is iPhone only', async () => {
  setDevice(IPAD, true)
  const { result } = setup(userWith(''))
  await waitFor(() => expect(result.current.active).toBe(true))
  expect(keys(result.current.steps)).toEqual(['home_screen'])
  expect(keys(result.current.inboxSteps)).toEqual(['home_screen', 'plan', 'import'])
})

it('marks home screen done when the browser reports the install, and keeps it for the next launch', async () => {
  setDevice(ANDROID_FIREFOX, true)
  const { result } = setup(userWith(''))
  await waitFor(() => expect(result.current.active).toBe(true))
  expect(result.current.steps[0]).toEqual({ key: 'home_screen', done: false, dismissed: false })
  act(() => {
    window.dispatchEvent(new Event('appinstalled'))
  })
  expect(result.current.steps[0]).toEqual({ key: 'home_screen', done: true, dismissed: false })
  expect(localStorage.getItem(HOME_SCREEN_KEY)).toBe('done')
})

it('a home screen step finished elsewhere updates every mounted hook', async () => {
  setDevice(IPHONE, true)
  const { result } = setup(userWith(''))
  await waitFor(() => expect(result.current.active).toBe(true))
  act(() => markHomeScreenDone())
  expect(result.current.steps[0]).toEqual({ key: 'home_screen', done: true, dismissed: false })
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

// The server's copy of the option: get-user-data reads it, a POST replaces it
// (after an optional delay, so a user fetch can answer first).
function serveOption(initial: string, postDelayMs = 0) {
  const state = { stored: initial, posts: [] as unknown[] }
  server.use(
    http.get('*/api/v1/user/get-user-data', () => HttpResponse.json({ success: true, message: '', data: { user: userWith(state.stored) } })),
    http.post('*/api/v1/user/update-getting-started', async ({ request }) => {
      const body = (await request.json()) as { value: string }
      state.posts.push(body)
      await new Promise((r) => setTimeout(r, postDelayMs))
      state.stored = body.value
      return HttpResponse.json({ success: true, message: '', data: { user: userWith(body.value) } })
    }),
  )
  return state
}

it('two quick dismissals keep both', async () => {
  const { result } = setup(userWith(''))
  const state = serveOption('')
  await waitFor(() => expect(result.current.active).toBe(true))
  act(() => {
    result.current.dismiss('plan')
    result.current.dismiss('import')
  })
  await waitFor(() => expect(state.posts).toEqual([{ value: '!plan' }, { value: '!plan,!import' }]))
  await waitFor(() => expect(state.stored).toBe('!plan,!import'))
  await new Promise((r) => setTimeout(r, 50))
  expect(result.current.steps.every((s) => s.dismissed)).toBe(true)
})

it('a user fetch answering before the write lands does not bring the step back', async () => {
  const { result, queryClient } = setup(userWith(''))
  const state = serveOption('', 100)
  await waitFor(() => expect(result.current.active).toBe(true))
  act(() => {
    markGettingStartedDone(queryClient, 'import')
    // what the CSV import does right after: refetch everything, the user included
    void queryClient.invalidateQueries()
  })
  await waitFor(() => expect(state.stored).toBe('import'))
  await waitFor(() => expect(result.current.inboxSteps.find((s) => s.key === 'import')?.done).toBe(true))
  await new Promise((r) => setTimeout(r, 100))
  expect(result.current.inboxSteps.find((s) => s.key === 'import')?.done).toBe(true)
  expect(state.posts).toEqual([{ value: 'import' }])
})

it('hasData is false with only an opening correction and true after a categorized transaction', async () => {
  const { result } = setup(userWith(''))
  await waitFor(() => expect(result.current.active).toBe(true))
  await waitFor(() => expect(result.current.hasData).toBe(false))
  const real = { ...correction('t9', 'a1'), categoryId: 'cat-salary' }
  expect(hasRealTransactions([correction('c1', 'a1')] as never)).toBe(false)
  expect(hasRealTransactions([correction('c1', 'a1'), correction('c2', 'a2')] as never)).toBe(false)
  expect(hasRealTransactions([correction('c1', 'a1'), real] as never)).toBe(true)
})

it('a balance edit is not real data: its adjustment carries a description and may be the second on the account', () => {
  const adjustment = { ...correction('c2', 'a1'), type: 'expense', description: 'Balance adjustment' }
  expect(hasRealTransactions([correction('c1', 'a1'), adjustment] as never)).toBe(false)
  for (const field of ['payeeId', 'tagId', 'recurringId', 'accountRecipientId']) {
    expect(hasRealTransactions([{ ...adjustment, [field]: 'x' }] as never)).toBe(true)
  }
  expect(hasRealTransactions([{ ...adjustment, labelIds: ['l1'] }] as never)).toBe(true)
  expect(hasRealTransactions([{ ...adjustment, type: 'transfer' }] as never)).toBe(true)
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
