import { render, waitFor } from '@testing-library/react'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { setToken } from '@/lib/storage'
import { navigateTo } from '@/app/routerRef'
import i18n from '@/app/i18n'
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

function renderPage() {
  Object.defineProperty(window, 'location', { value: { ...window.location, assign }, writable: true })
  const router = createMemoryRouter([{ path: '/logout', element: <LogoutPage /> }], { initialEntries: ['/logout'] })
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
