import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { OAuthCallbackPage } from './OAuthCallbackPage'

function renderAt(entry: string) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/oauth/callback', element: <OAuthCallbackPage /> },
      { path: '/', element: <div data-testid="home" /> },
      { path: '/login', element: <div data-testid="login" /> },
    ],
    { initialEntries: [entry] },
  )
  render(<QueryClientProvider client={qc}><RouterProvider router={router} /></QueryClientProvider>)
  return router
}

const realLocation = window.location
let replace: ReturnType<typeof vi.fn>

beforeEach(() => {
  localStorage.clear()
  sessionStorage.clear()
  window.econumoConfig = {}
  sessionStorage.setItem('oauthFlow', 'f1')
  replace = vi.fn()
  Object.defineProperty(window, 'location', { value: { ...realLocation, replace }, writable: true })
})

afterEach(() => {
  Object.defineProperty(window, 'location', { value: realLocation, writable: true })
})

it('exchanges the handoff from the fragment, stores the token and enters the app in a fresh document', async () => {
  let posted: unknown
  server.use(http.post('*/api/v1/oauth/exchange-handoff', async ({ request }) => {
    posted = await request.json()
    return HttpResponse.json({ token: 'eco_ses_ok', user: { id: 'u1', options: [], accessLevel: 'full', accessUntil: '', hasPassword: false } })
  }))
  const router = renderAt('/oauth/callback#handoff=abc')
  expect(screen.getByText('Signing you in…')).toBeInTheDocument()
  // A router navigation would keep the document Apple's cross-site POST landed
  // in; only a new document clears its viewport in an iOS standalone PWA.
  await waitFor(() => expect(replace).toHaveBeenCalledWith('/'))
  expect(router.state.location.pathname).toBe('/oauth/callback')
  expect(posted).toEqual({ code: 'abc', flow: 'f1' })
  expect(localStorage.getItem('token')).toBe('eco_ses_ok')
  expect(sessionStorage.getItem('oauthFlow')).toBeNull()
})

it('lands on /login with an error when the exchange fails or the fragment is missing', async () => {
  server.use(http.post('*/api/v1/oauth/exchange-handoff', () =>
    HttpResponse.json({ success: false, message: 'x', code: 401, errors: {} }, { status: 401 })))
  const router = renderAt('/oauth/callback#handoff=bad')
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
  expect(router.state.location.search).toBe('?oauthError=invalid_state')
  sessionStorage.setItem('oauthFlow', 'f1')
  const router2 = renderAt('/oauth/callback')
  await waitFor(() => expect(router2.state.location.search).toBe('?oauthError=invalid_state'))
})

it('reports a provider error when the exchange fails with anything but a 401', async () => {
  server.use(http.post('*/api/v1/oauth/exchange-handoff', () =>
    HttpResponse.json({ success: false, message: 'boom', code: 0, exceptionType: 'x' }, { status: 500 })))
  const router = renderAt('/oauth/callback#handoff=abc')
  await waitFor(() => expect(router.state.location.search).toBe('?oauthError=provider_error'))
})

it('lands on /login when this browser holds no flow secret', async () => {
  sessionStorage.clear()
  let called = false
  server.use(http.post('*/api/v1/oauth/exchange-handoff', () => {
    called = true
    return HttpResponse.json({ token: 'eco_ses_ok', user: { id: 'u1', options: [], accessLevel: 'full', accessUntil: '' } })
  }))
  const router = renderAt('/oauth/callback#handoff=abc')
  await waitFor(() => expect(router.state.location.search).toBe('?oauthError=invalid_state'))
  expect(called).toBe(false)
})
