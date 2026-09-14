import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { Toaster } from '@/components/ui/sonner'
import { useOAuthInFlight } from '@/features/auth/oauthQueries'
import { LinkedAccountsPage } from './LinkedAccountsPage'

const providers = [{ id: 'google', name: 'Google' }, { id: 'apple', name: 'Apple' }]
const identities = [{ provider: 'google', email: 'me@gmail.test', createdAt: '2026-09-01 10:00:00' }]

function mockUser(hasPassword: boolean) {
  server.use(http.get('*/api/v1/user/get-user-data', () =>
    HttpResponse.json({ success: true, message: '', data: { user: { id: 'u1', name: 'Me', email: 'me@example.test', avatar: 'face:sky', options: [], accessLevel: 'full', accessUntil: '', createdAt: '2026-01-01 00:00:00', currency: 'USD', reportPeriod: 'month', hasPassword } } })))
}

function mockViewport() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderPage(initialEntry = '/settings/profile/linked-accounts') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/settings/profile/linked-accounts', element: <LinkedAccountsPage /> },
    ],
    { initialEntries: [initialEntry] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <Toaster />
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return router
}

beforeEach(() => {
  localStorage.clear()
  sessionStorage.clear()
  window.econumoConfig = {}
  delete (window as { Capacitor?: unknown }).Capacitor
  mockViewport()
  server.use(
    http.get('*/api/v1/oauth/get-provider-list', () => HttpResponse.json({ success: true, message: '', data: providers })),
    http.get('*/api/v1/oauth/get-identity-list', () => HttpResponse.json({ success: true, message: '', data: identities })),
  )
})

afterEach(() => {
  useOAuthInFlight.setState({ inFlight: false })
})

it('lists linked identities and offers Link for the rest', async () => {
  mockUser(true)
  renderPage()
  expect(await screen.findByText('me@gmail.test')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Unlink' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Link' })).toBeInTheDocument() // apple
})

it('unlinks after confirmation and fires the metric', async () => {
  mockUser(true)
  let posted: unknown
  server.use(http.post('*/api/v1/oauth/unlink-identity', async ({ request }) => {
    posted = await request.json()
    return HttpResponse.json({ success: true, message: '', data: {} })
  }))
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Unlink' }))
  await user.click(await screen.findByRole('button', { name: 'Unlink', hidden: false }))
  await waitFor(() => expect(posted).toEqual({ provider: 'google' }))
})

it('disables Unlink with a hint for a passwordless user with one identity', async () => {
  mockUser(false)
  renderPage()
  const btn = await screen.findByRole('button', { name: 'Unlink' })
  expect(btn).toBeDisabled()
  expect(screen.getByText('Set a password before unlinking your only sign-in method.')).toBeInTheDocument()
})

it('completes the link from #linkHandoff= and shows the toast', async () => {
  mockUser(true)
  sessionStorage.setItem('oauthFlow', 'f1')
  let posted: unknown
  server.use(http.post('*/api/v1/oauth/complete-link', async ({ request }) => {
    posted = await request.json()
    return HttpResponse.json({ success: true, message: '', data: { provider: 'google' } })
  }))
  renderPage('/settings/profile/linked-accounts#linkHandoff=abc')
  expect(await screen.findByText('Google account linked.')).toBeInTheDocument()
  expect(posted).toEqual({ code: 'abc', flow: 'f1' })
})

// In the app the deep link navigates back into this SAME mounted page, so a
// second link must not be swallowed by the first one's latch.
it('completes a second link on the same mounted page', async () => {
  mockUser(true)
  sessionStorage.setItem('oauthFlow', 'f1')
  const codes: string[] = []
  server.use(http.post('*/api/v1/oauth/complete-link', async ({ request }) => {
    const body = (await request.json()) as { code: string }
    codes.push(body.code)
    return HttpResponse.json({ success: true, message: '', data: { provider: 'apple' } })
  }))
  const router = renderPage('/settings/profile/linked-accounts#linkHandoff=first')
  await waitFor(() => expect(codes).toEqual(['first']))

  sessionStorage.setItem('oauthFlow', 'f2')
  await router.navigate('/settings/profile/linked-accounts#linkHandoff=second')
  await waitFor(() => expect(codes).toEqual(['first', 'second']))
})

// The flow secret is what proves this client started the link; without it the
// callback's code is worthless, and the server is never asked.
it('refuses to complete a link this client did not start', async () => {
  mockUser(true)
  let called = false
  server.use(http.post('*/api/v1/oauth/complete-link', () => {
    called = true
    return HttpResponse.json({ success: true, message: '', data: { provider: 'google' } })
  }))
  renderPage('/settings/profile/linked-accounts#linkHandoff=abc')
  expect(await screen.findByText('The sign-in attempt expired or was already used. Please try again.')).toBeInTheDocument()
  expect(called).toBe(false)
})

it('shows the server message when completing the link fails', async () => {
  mockUser(true)
  sessionStorage.setItem('oauthFlow', 'f1')
  server.use(http.post('*/api/v1/oauth/complete-link', () =>
    HttpResponse.json({ success: false, message: 'Linking session is invalid or has expired', code: 400, errors: {} }, { status: 400 })))
  renderPage('/settings/profile/linked-accounts#linkHandoff=abc')
  expect(await screen.findByText('Linking session is invalid or has expired')).toBeInTheDocument()
})

it('shows the link error from ?oauthError= and clears the parameter', async () => {
  mockUser(true)
  renderPage('/settings/profile/linked-accounts?oauthError=identity_taken')
  expect(await screen.findByText('This external account is already linked to another Econumo account.')).toBeInTheDocument()
})

it('falls back to the generic provider error for an unknown code', async () => {
  mockUser(true)
  renderPage('/settings/profile/linked-accounts?oauthError=made_up')
  expect(await screen.findByText('The sign-in provider returned an error. Please try again.')).toBeInTheDocument()
})

it('keeps the Link button disabled after the start mutation settles, and re-enables it once the browser sheet finishes', async () => {
  mockUser(true)
  const listeners: Record<string, () => void> = {}
  const browserOpen = vi.fn().mockResolvedValue(undefined)
  window.Capacitor = { isNativePlatform: () => true, Plugins: { Browser: {
    open: browserOpen,
    addListener: vi.fn((ev: string, cb: () => void) => { listeners[ev] = cb }),
  } } }
  server.use(http.post('*/api/v1/oauth/start-link', () =>
    HttpResponse.json({ success: true, message: '', data: { url: 'https://idp/apple', flow: 'f1' } })))
  renderPage()
  const button = await screen.findByRole('button', { name: 'Link' })
  await userEvent.click(button)
  await waitFor(() => expect(browserOpen).toHaveBeenCalled())
  await act(async () => {})
  expect(button).toBeDisabled()
  listeners.browserFinished()
  await waitFor(() => expect(button).not.toBeDisabled())
})

it('points the disabled Unlink button at the hint that explains it', async () => {
  mockUser(false)
  renderPage()
  const btn = await screen.findByRole('button', { name: 'Unlink' })
  expect(btn).toHaveAttribute('aria-describedby', 'linked-accounts-last-identity-hint')
  expect(screen.getByText('Set a password before unlinking your only sign-in method.')).toHaveAttribute('id', 'linked-accounts-last-identity-hint')
})
