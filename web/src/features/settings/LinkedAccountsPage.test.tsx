import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { Toaster } from '@/components/ui/sonner'
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
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  mockViewport()
  server.use(
    http.get('*/api/v1/oauth/get-provider-list', () => HttpResponse.json({ success: true, message: '', data: providers })),
    http.get('*/api/v1/oauth/get-identity-list', () => HttpResponse.json({ success: true, message: '', data: identities })),
  )
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

it('shows the linked toast from ?linked=', async () => {
  mockUser(true)
  renderPage('/settings/profile/linked-accounts?linked=google')
  expect(await screen.findByText('Google account linked.')).toBeInTheDocument()
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

it('points the disabled Unlink button at the hint that explains it', async () => {
  mockUser(false)
  renderPage()
  const btn = await screen.findByRole('button', { name: 'Unlink' })
  expect(btn).toHaveAttribute('aria-describedby', 'linked-accounts-last-identity-hint')
  expect(screen.getByText('Set a password before unlinking your only sign-in method.')).toHaveAttribute('id', 'linked-accounts-last-identity-hint')
})
