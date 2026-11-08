import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser } from '@/test/fixtures'
import { listenForInstallPrompt, resetInstallStateForTests } from '@/features/install/installPrompt'
import * as metrics from '@/lib/metrics'
import { markGettingStartedDone } from './gettingStarted'
import { HOME_SCREEN_KEY, setHomeScreenState } from './homeScreen'
import { useOnboardingSteps } from './useOnboardingSteps'

const IPHONE = 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1'
const MAC_SAFARI = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15'
const ANDROID_FIREFOX = 'Mozilla/5.0 (Android 14; Mobile; rv:130.0) Gecko/130.0 Firefox/130.0'
const DESKTOP = 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36'

const userWith = (value: string | null) => ({
  ...fixtureUser,
  options: value === null ? fixtureUser.options : [...fixtureUser.options, { name: 'getting_started', value }],
})

function setDevice(ua: string, standalone = false) {
  vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue(ua)
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q.includes('standalone') ? standalone : false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function setup(user: unknown, extra: Record<string, unknown> = {}) {
  server.use(...coreHandlers({ user, ...extra }))
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  return { queryClient, ...renderHook(() => useOnboardingSteps(), { wrapper }) }
}

const keys = (steps: { key: string }[]) => steps.map((s) => s.key)

beforeAll(() => listenForInstallPrompt())

beforeEach(() => {
  resetInstallStateForTests()
  localStorage.clear()
  window.econumoConfig = {}
  setDevice(DESKTOP)
})

afterEach(() => vi.restoreAllMocks())

it('gives desktop Linux Chrome the four shared steps', async () => {
  const { result } = setup(userWith(''))
  await waitFor(() => expect(result.current.ready).toBe(true))
  expect(keys(result.current.steps)).toEqual(['import', 'categories', 'accounts', 'plan'])
  expect(result.current.current).toBe('import')
  expect(result.current.allResolved).toBe(false)
})

it('gives Mac Safari all six steps', async () => {
  setDevice(MAC_SAFARI)
  const { result } = setup(userWith(''))
  await waitFor(() => expect(result.current.ready).toBe(true))
  expect(keys(result.current.steps)).toEqual(['import', 'categories', 'accounts', 'plan', 'home_screen', 'apple_wallet'])
})

it('gives iPhone Safari all six steps', async () => {
  setDevice(IPHONE)
  const { result } = setup(userWith(''))
  await waitFor(() => expect(result.current.ready).toBe(true))
  expect(keys(result.current.steps)).toEqual(['import', 'categories', 'accounts', 'plan', 'home_screen', 'apple_wallet'])
})

it('leaves out the install step on an iPhone already running the installed app', async () => {
  setDevice(IPHONE, true)
  const { result } = setup(userWith(''))
  await waitFor(() => expect(result.current.ready).toBe(true))
  expect(keys(result.current.steps)).toEqual(['import', 'categories', 'accounts', 'plan', 'apple_wallet'])
})

it('gives an Android phone the install step but not Apple Wallet', async () => {
  setDevice(ANDROID_FIREFOX)
  const { result } = setup(userWith(''))
  await waitFor(() => expect(result.current.ready).toBe(true))
  expect(keys(result.current.steps)).toEqual(['import', 'categories', 'accounts', 'plan', 'home_screen'])
})

it('the current step is the first one still open', async () => {
  const { result } = setup(userWith('import,categories,accounts'))
  await waitFor(() => expect(result.current.ready).toBe(true))
  expect(result.current.current).toBe('plan')
})

it('reads a ! key as skipped', async () => {
  const { result } = setup(userWith('!import'))
  await waitFor(() => expect(result.current.ready).toBe(true))
  expect(result.current.steps[0]).toEqual({ key: 'import', state: 'skipped' })
  expect(result.current.current).toBe('categories')
})

it('is all resolved once every visible step is done or skipped', async () => {
  const { result } = setup(userWith('import,!categories,accounts,!plan'))
  await waitFor(() => expect(result.current.ready).toBe(true))
  expect(result.current.current).toBeNull()
  expect(result.current.allResolved).toBe(true)
})

it('marks Apple Wallet done once a Wallet source exists, and home screen from this device', async () => {
  setDevice(IPHONE)
  localStorage.setItem(HOME_SCREEN_KEY, 'skipped')
  const { result } = setup(userWith(''), { importSources: [{ id: 's1', provider: 'apple-wallet', name: 'Wallet', cards: [] }] })
  await waitFor(() => expect(result.current.steps.find((s) => s.key === 'apple_wallet')?.state).toBe('done'))
  expect(result.current.steps.find((s) => s.key === 'home_screen')?.state).toBe('skipped')
})

it('marks home screen done when the browser reports the install, keeps the step and remembers it', async () => {
  setDevice(ANDROID_FIREFOX)
  const { result } = setup(userWith(''))
  await waitFor(() => expect(result.current.ready).toBe(true))
  expect(result.current.steps.at(-1)).toEqual({ key: 'home_screen', state: 'open' })
  act(() => {
    window.dispatchEvent(new Event('appinstalled'))
  })
  expect(result.current.steps.at(-1)).toEqual({ key: 'home_screen', state: 'done' })
  expect(localStorage.getItem(HOME_SCREEN_KEY)).toBe('done')
})

it('a home screen step finished elsewhere updates every mounted hook', async () => {
  setDevice(IPHONE)
  const { result } = setup(userWith(''))
  await waitFor(() => expect(result.current.ready).toBe(true))
  act(() => setHomeScreenState('done'))
  expect(result.current.steps.find((s) => s.key === 'home_screen')?.state).toBe('done')
})

it('skipping home screen stays on this device, sends nothing and fires the skip metric', async () => {
  setDevice(IPHONE)
  const track = vi.spyOn(metrics, 'trackEvent')
  const posts: unknown[] = []
  server.use(http.post('*/api/v1/user/update-getting-started', async ({ request }) => {
    posts.push(await request.json())
    return HttpResponse.json({ success: true, message: '', data: { user: userWith('') } })
  }))
  const { result } = setup(userWith(''))
  await waitFor(() => expect(result.current.ready).toBe(true))
  await act(() => result.current.skip('home_screen'))
  expect(result.current.steps.find((s) => s.key === 'home_screen')?.state).toBe('skipped')
  expect(localStorage.getItem(HOME_SCREEN_KEY)).toBe('skipped')
  expect(posts).toEqual([])
  expect(track).toHaveBeenCalledWith(metrics.METRICS.ONBOARDING_STEP_SKIP, { step: 'home_screen' })
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

it('markDone writes the option and fires the done metric; a revisited skip turns into done', async () => {
  const track = vi.spyOn(metrics, 'trackEvent')
  const { result } = setup(userWith('!import'))
  const state = serveOption('!import')
  await waitFor(() => expect(result.current.ready).toBe(true))
  await act(() => result.current.markDone('import'))
  expect(state.posts).toEqual([{ value: 'import' }])
  expect(result.current.steps[0]).toEqual({ key: 'import', state: 'done' })
  expect(track).toHaveBeenCalledWith(metrics.METRICS.ONBOARDING_STEP_DONE, { step: 'import' })
})

it('skip on a revisited done step keeps it done and writes nothing', async () => {
  setDevice(IPHONE)
  localStorage.setItem(HOME_SCREEN_KEY, 'done')
  const track = vi.spyOn(metrics, 'trackEvent')
  const { result } = setup(userWith('import'))
  const state = serveOption('import')
  await waitFor(() => expect(result.current.ready).toBe(true))
  await act(() => result.current.skip('import'))
  await act(() => result.current.skip('home_screen'))
  expect(state.posts).toEqual([])
  expect(result.current.steps[0]).toEqual({ key: 'import', state: 'done' })
  expect(localStorage.getItem(HOME_SCREEN_KEY)).toBe('done')
  expect(track).not.toHaveBeenCalledWith(metrics.METRICS.ONBOARDING_STEP_SKIP, expect.anything())
})

it('a registered user without the option is in the flow and has the answer written', async () => {
  const registered = { ...fixtureUser, options: fixtureUser.options.map((o) => (o.name === 'onboarding' ? { ...o, value: 'started' } : o)) }
  const posts: unknown[] = []
  // the server appends the option on the first write
  let stored = registered
  const { result } = setup(registered)
  server.use(
    http.get('*/api/v1/user/get-user-data', () => HttpResponse.json({ success: true, message: '', data: { user: stored } })),
    http.post('*/api/v1/user/update-getting-started', async ({ request }) => {
      const body = (await request.json()) as { value: string }
      posts.push(body)
      stored = { ...registered, options: [...registered.options, { name: 'getting_started', value: body.value }] }
      return HttpResponse.json({ success: true, message: '', data: { user: stored } })
    }),
  )
  await waitFor(() => expect(result.current.ready).toBe(true))
  expect(result.current.current).toBe('import')
  await act(() => result.current.skip('import'))
  expect(posts).toEqual([{ value: '!import' }])
  await waitFor(() => expect(result.current.steps[0]).toEqual({ key: 'import', state: 'skipped' }))
})

it('two quick skips keep both', async () => {
  const { result } = setup(userWith(''))
  const state = serveOption('')
  await waitFor(() => expect(result.current.ready).toBe(true))
  act(() => {
    void result.current.skip('plan')
    void result.current.skip('import')
  })
  await waitFor(() => expect(state.posts).toEqual([{ value: '!plan' }, { value: '!plan,!import' }]))
  await waitFor(() => expect(state.stored).toBe('!plan,!import'))
  await new Promise((r) => setTimeout(r, 50))
  expect(result.current.steps.filter((s) => s.state === 'skipped').map((s) => s.key)).toEqual(['import', 'plan'])
})

it('a user fetch answering before the write lands does not bring the step back', async () => {
  const { result, queryClient } = setup(userWith(''))
  const state = serveOption('', 100)
  await waitFor(() => expect(result.current.ready).toBe(true))
  act(() => {
    markGettingStartedDone(queryClient, 'import')
    // what the CSV import does right after: refetch everything, the user included
    void queryClient.invalidateQueries()
  })
  await waitFor(() => expect(state.stored).toBe('import'))
  await waitFor(() => expect(result.current.steps[0]?.state).toBe('done'))
  await new Promise((r) => setTimeout(r, 100))
  expect(result.current.steps[0]?.state).toBe('done')
  expect(state.posts).toEqual([{ value: 'import' }])
})

it('answering a step already in that state fires no metric and writes nothing', async () => {
  setDevice(IPHONE)
  localStorage.setItem(HOME_SCREEN_KEY, 'skipped')
  const track = vi.spyOn(metrics, 'trackEvent')
  const { result } = setup(userWith('import,!plan'))
  const state = serveOption('import,!plan')
  await waitFor(() => expect(result.current.ready).toBe(true))
  await act(() => result.current.markDone('import'))
  await act(() => result.current.skip('plan'))
  await act(() => result.current.skip('home_screen'))
  expect(state.posts).toEqual([])
  expect(track).not.toHaveBeenCalledWith(metrics.METRICS.ONBOARDING_STEP_DONE, expect.anything())
  expect(track).not.toHaveBeenCalledWith(metrics.METRICS.ONBOARDING_STEP_SKIP, expect.anything())
})

it('is ready as soon as the import source list fails, without waiting for retries', async () => {
  server.use(...coreHandlers({ user: userWith('') }))
  server.use(http.get('*/api/v1/import/get-source-list', () => HttpResponse.json({ success: false, message: 'Not found', code: 404, errors: {} }, { status: 404 })))
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: 3 } } })
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  const { result } = renderHook(() => useOnboardingSteps(), { wrapper })
  // the first retry alone would wait a second
  await waitFor(() => expect(result.current.ready).toBe(true), { timeout: 800 })
  expect(result.current.current).toBe('import')
})
