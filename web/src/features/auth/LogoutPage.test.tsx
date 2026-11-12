import { render, waitFor } from '@testing-library/react'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { setToken } from '@/lib/storage'
import { navigateTo } from '@/app/routerRef'
import i18n from '@/app/i18n'
import { IDBFactory } from 'fake-indexeddb'
import { resetDbForTests } from '@/lib/db'
import { resetTransactionStoreForTests } from '@/lib/transactionStore'
import { addChange } from '@/features/pendingWrites/queue'
import { loadPendingWrites, mutate, resetPendingWritesForTests } from '@/features/pendingWrites/store'
import { APP_LOCK_ATTEMPTS_KEY, APP_LOCK_CONFIG_KEY } from '@/features/appLock/keys'
import { LogoutPage } from './LogoutPage'

// Tracks real call order (not just "was called") so a refactor that moves
// resetAnalyticsIdentity relative to the logout event or the token removal
// fails these tests, even though each function still behaves normally.
const order: string[] = []

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return {
    ...actual,
    trackEvent: vi.fn((...args: Parameters<typeof actual.trackEvent>) => {
      order.push('trackEvent')
      return actual.trackEvent(...args)
    }),
  }
})

vi.mock('@/lib/analytics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/analytics')>()
  return {
    ...actual,
    resetAnalyticsIdentity: vi.fn(() => {
      order.push('resetAnalyticsIdentity')
      return actual.resetAnalyticsIdentity()
    }),
  }
})

vi.mock('@/lib/storage', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/storage')>()
  return {
    ...actual,
    removeToken: vi.fn(() => {
      order.push('removeToken')
      return actual.removeToken()
    }),
  }
})

vi.mock('@/app/routerRef', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/app/routerRef')>()
  return {
    ...actual,
    navigateTo: vi.fn(actual.navigateTo),
  }
})

const assign = vi.fn()

function renderPage(state?: unknown) {
  Object.defineProperty(window, 'location', { value: { ...window.location, assign }, writable: true })
  const router = createMemoryRouter([{ path: '/logout', element: <LogoutPage /> }], { initialEntries: [{ pathname: '/logout', state }] })
  return render(<RouterProvider router={router} />)
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  order.length = 0
  assign.mockClear()
  vi.mocked(navigateTo).mockClear()
})

it('calls logout, purges the token and redirects to /login', async () => {
  let called = false
  server.use(
    http.post('*/api/v1/user/logout-user', () => {
      called = true
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  setToken('tok')
  localStorage.setItem('econumo.query-cache', '{"stale":"finances"}')
  renderPage()
  await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('/login'))
  expect(called).toBe(true)
  expect(localStorage.getItem('token')).toBeNull()
  expect(localStorage.getItem('econumo.query-cache')).toBeNull()
  // The logout event must flush under the outgoing user's identity, so the
  // identity reset has to land strictly between the tracked event and the
  // token removal that follows it.
  expect(order).toEqual(['trackEvent', 'resetAnalyticsIdentity', 'removeToken'])
})

it('still redirects to /login when IndexedDB never answers', async () => {
  globalThis.indexedDB = { open: () => ({}) } as unknown as IDBFactory
  resetDbForTests()
  resetTransactionStoreForTests()
  vi.spyOn(console, 'warn').mockImplementation(() => {})
  try {
    server.use(http.post('*/api/v1/user/logout-user', () => HttpResponse.json({ success: true, message: '', data: {} })))
    setToken('tok')
    renderPage()
    await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('/login'), { timeout: 4000 })
  } finally {
    vi.restoreAllMocks()
    globalThis.indexedDB = new IDBFactory()
    resetDbForTests()
    resetTransactionStoreForTests()
  }
})

describe('the queue of unsent changes', () => {
  const queueOne = async () => {
    await loadPendingWrites('u1')
    await mutate((ops) => addChange(ops, { kind: 'limit', budgetId: 'b', elementId: 'e', period: '2026-11-01', amount: '1' }, { key: 'k', seq: 1 }))
    server.use(http.post('*/api/v1/user/logout-user', () => HttpResponse.json({ success: true, message: '', data: {} })))
    setToken('tok', 'u1')
  }
  const stored = async () => {
    resetPendingWritesForTests()
    return loadPendingWrites('u1')
  }

  it('is cleared by a sign-out', async () => {
    await queueOne()
    renderPage()
    await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('/login'))
    expect(await stored()).toEqual([])
  })

  it('stays for the same user to sign back in when the sign-out asks to keep it', async () => {
    await queueOne()
    renderPage({ keepPendingWrites: true })
    await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('/login'))
    expect(localStorage.getItem('token')).toBeNull()
    expect(await stored()).toHaveLength(1)
  })

  it('stays when the session is already gone too', async () => {
    await queueOne()
    localStorage.removeItem('token')
    renderPage({ keepPendingWrites: true })
    await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('/login'))
    expect(await stored()).toHaveLength(1)
  })
})

it('navigates to the IdP end-session url when logout returns one', async () => {
  localStorage.setItem('token', 'eco_ses_x')
  server.use(http.post('*/api/v1/user/logout-user', () =>
    HttpResponse.json({ success: true, message: '', data: { result: 'test', logoutUrl: 'https://idp/end?x=1', provider: 'oidc' } })))
  renderPage()
  await waitFor(() => expect(assign).toHaveBeenCalledWith('https://idp/end?x=1'))
  expect(localStorage.getItem('token')).toBeNull()
})

it('redirects to /login for a provider session without an end-session url', async () => {
  localStorage.setItem('token', 'eco_ses_x')
  server.use(
    http.post('*/api/v1/user/logout-user', () =>
      HttpResponse.json({ success: true, message: '', data: { result: 'test', logoutUrl: '', provider: 'google' } })),
  )
  renderPage()
  await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('/login'))
  expect(localStorage.getItem('token')).toBeNull()
})

// Google and Apple publish no end_session_endpoint, so no url comes back and
// there is nothing to tell the user about: the local logout is the whole exit.
it('renders nothing and does not fetch the provider list', async () => {
  localStorage.setItem('token', 'eco_ses_x')
  let providerListCalled = false
  server.use(
    http.post('*/api/v1/user/logout-user', () =>
      HttpResponse.json({ success: true, message: '', data: { result: 'test', logoutUrl: '', provider: 'google' } })),
    http.get('*/api/v1/oauth/get-provider-list', () => {
      providerListCalled = true
      return HttpResponse.json({ success: true, message: '', data: [{ id: 'google', name: 'Google' }] })
    }),
  )
  const { container } = renderPage()
  await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('/login'))
  expect(providerListCalled).toBe(false)
  expect(container).toBeEmptyDOMElement()
})

// A language switch must not re-run the effect and issue a second logout.
it('a language change does not re-run the effect', async () => {
  localStorage.setItem('token', 'eco_ses_x')
  let logoutCalls = 0
  server.use(
    http.post('*/api/v1/user/logout-user', () => {
      logoutCalls += 1
      return HttpResponse.json({ success: true, message: '', data: { result: 'test', logoutUrl: '', provider: 'google' } })
    }),
  )
  renderPage()
  await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('/login'))
  await i18n.changeLanguage('de')
  await i18n.changeLanguage('en')
  expect(logoutCalls).toBe(1)
})

it('in the app ignores the end-session url and logs out locally', async () => {
  window.Capacitor = { isNativePlatform: () => true }
  localStorage.setItem('token', 'eco_ses_x')
  server.use(
    http.post('*/api/v1/user/logout-user', () =>
      HttpResponse.json({ success: true, message: '', data: { result: 'test', logoutUrl: 'https://idp/end', provider: 'oidc' } })),
  )
  renderPage()
  await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('/login'))
  expect(assign).not.toHaveBeenCalledWith('https://idp/end')
  delete (window as { Capacitor?: unknown }).Capacitor
})

describe('the app lock', () => {
  const configure = () => {
    server.use(http.post('*/api/v1/user/logout-user', () => HttpResponse.json({ success: true, message: '', data: {} })))
    setToken('tok', 'u1')
    localStorage.setItem(APP_LOCK_CONFIG_KEY, '{"v":1}')
    localStorage.setItem(APP_LOCK_ATTEMPTS_KEY, '{"failedAttempts":3,"lockedUntil":null}')
  }

  it('is removed by a sign-out, after the token', async () => {
    configure()
    const removeItem = vi.spyOn(Storage.prototype, 'removeItem')
    try {
      renderPage()
      await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('/login'))
      expect(localStorage.getItem(APP_LOCK_CONFIG_KEY)).toBeNull()
      expect(localStorage.getItem(APP_LOCK_ATTEMPTS_KEY)).toBeNull()
      const keys = removeItem.mock.calls.map(([key]) => key)
      expect(keys.indexOf('token')).toBeLessThan(keys.indexOf(APP_LOCK_CONFIG_KEY))
    } finally {
      removeItem.mockRestore()
    }
  })

  it('is removed when the session is already gone', async () => {
    configure()
    localStorage.removeItem('token')
    renderPage()
    await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('/login'))
    expect(localStorage.getItem(APP_LOCK_CONFIG_KEY)).toBeNull()
  })

  it('stays when the same user signs back in after setting a password', async () => {
    configure()
    renderPage({ keepPendingWrites: true })
    await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('/login'))
    expect(localStorage.getItem('token')).toBeNull()
    expect(localStorage.getItem(APP_LOCK_CONFIG_KEY)).toBe('{"v":1}')
  })

  it('is removed by Forgot PIN and the attempts limit, which keep the queue', async () => {
    configure()
    renderPage({ keepPendingWrites: true, resetAppLock: true })
    await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('/login'))
    expect(localStorage.getItem(APP_LOCK_CONFIG_KEY)).toBeNull()
    expect(localStorage.getItem(APP_LOCK_ATTEMPTS_KEY)).toBeNull()
  })
})
