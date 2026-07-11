import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { SessionsPage } from './SessionsPage'
import { describeUserAgent, relativeTime } from './securityFormat'

const current = {
  id: '01890000-0000-7000-8000-00000000c001',
  userAgent: 'Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0',
  createdAt: '2026-07-01 10:00:00',
  lastUsedAt: '2026-07-10 09:00:00',
  isCurrent: true,
}
const other = {
  id: '01890000-0000-7000-8000-00000000c002',
  userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) Safari/604.1',
  createdAt: '2026-07-02 10:00:00',
  lastUsedAt: '2026-07-09 09:00:00',
  isCurrent: false,
}

function mockViewport() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

let navigatedToLogout = false

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/settings/profile/sessions', element: <SessionsPage /> },
      { path: '/logout', element: <div data-testid="logout-page" />, loader: () => { navigatedToLogout = true; return null } },
    ],
    { initialEntries: ['/settings/profile/sessions'] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  navigatedToLogout = false
  mockViewport()
  server.use(
    http.get('*/api/v1/user/get-session-list', () =>
      HttpResponse.json({ success: true, message: '', data: [current, other] }),
    ),
  )
})

it('lists sessions with parsed user agents and marks the current one', async () => {
  renderPage()
  expect(await screen.findByText(/Firefox on Linux/)).toBeInTheDocument()
  expect(screen.getByText(/Safari on iOS/)).toBeInTheDocument()
  expect(screen.getByText('Current')).toBeInTheDocument()
  // Current session gets "Sign out"; the other row gets "Revoke".
  expect(screen.getByRole('button', { name: 'Sign out' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Revoke' })).toBeInTheDocument()
})

it('revokes another session after confirmation', async () => {
  let revokedId: unknown
  server.use(
    http.post('*/api/v1/user/revoke-session', async ({ request }) => {
      revokedId = ((await request.json()) as { id: string }).id
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Revoke' }))
  await user.click(await screen.findByRole('button', { name: 'Revoke', hidden: false }))
  await waitFor(() => expect(revokedId).toBe(other.id))
  expect(navigatedToLogout).toBe(false)
})

it('signing out of the current session goes through the logout flow without revoke-session', async () => {
  // Revoking the presenting token first would make LogoutPage's logout-user
  // call 401 and surface the "session expired" banner; the logout endpoint
  // revokes the current session itself.
  let revokeCalled = false
  server.use(
    http.post('*/api/v1/user/revoke-session', () => {
      revokeCalled = true
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Sign out' }))
  // confirm dialog re-uses the same label
  const buttons = await screen.findAllByRole('button', { name: 'Sign out' })
  await user.click(buttons[buttons.length - 1])
  await waitFor(() => expect(screen.getByTestId('logout-page')).toBeInTheDocument())
  expect(revokeCalled).toBe(false)
})

it('signs out other devices', async () => {
  let called = false
  server.use(
    http.post('*/api/v1/user/revoke-other-sessions', () => {
      called = true
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Sign out other devices' }))
  // The dialog confirm button is deliberately short ("Sign out") so it fits its
  // half-width grid cell; the long label stays on the page-level button only.
  const confirm = await screen.findAllByRole('button', { name: 'Sign out' })
  await user.click(confirm[confirm.length - 1])
  await waitFor(() => expect(called).toBe(true))
})

it('securityFormat helpers parse UA and relative time', () => {
  expect(describeUserAgent(current.userAgent)).toBe('Firefox on Linux')
  expect(describeUserAgent(other.userAgent)).toBe('Safari on iOS')
  expect(describeUserAgent('')).toBe('')
  expect(describeUserAgent('weird-cli/1.0')).toBe('weird-cli/1.0')

  const now = new Date(Date.UTC(2026, 6, 10, 12, 0, 0))
  expect(relativeTime('2026-07-10 11:59:30', now)).toBe('just now')
  expect(relativeTime('2026-07-10 11:55:00', now)).toBe('5 minutes ago')
  expect(relativeTime('2026-07-10 09:00:00', now)).toBe('3 hours ago')
  expect(relativeTime('2026-07-07 12:00:00', now)).toBe('3 days ago')
})
