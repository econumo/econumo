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

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('exchanges the handoff from the fragment, stores the token and goes home', async () => {
  let posted: unknown
  server.use(http.post('*/api/v1/oauth/exchange-handoff', async ({ request }) => {
    posted = await request.json()
    return HttpResponse.json({ token: 'eco_ses_ok', user: { id: 'u1', options: [], accessLevel: 'full', accessUntil: '', hasPassword: false } })
  }))
  renderAt('/oauth/callback#handoff=abc')
  expect(screen.getByText('Signing you in…')).toBeInTheDocument()
  await waitFor(() => expect(screen.getByTestId('home')).toBeInTheDocument())
  expect(posted).toEqual({ code: 'abc' })
  expect(localStorage.getItem('token')).toBe('eco_ses_ok')
})

it('lands on /login with an error when the exchange fails or the fragment is missing', async () => {
  server.use(http.post('*/api/v1/oauth/exchange-handoff', () =>
    HttpResponse.json({ success: false, message: 'x', code: 401, errors: {} }, { status: 401 })))
  const router = renderAt('/oauth/callback#handoff=bad')
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
  expect(router.state.location.search).toBe('?oauthError=invalid_state')
  const router2 = renderAt('/oauth/callback')
  await waitFor(() => expect(router2.state.location.search).toBe('?oauthError=invalid_state'))
})
