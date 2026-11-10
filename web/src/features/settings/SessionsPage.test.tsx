import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider, onlineManager } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { mutate } from '@/features/pendingWrites/store'
import type { PendingOp } from '@/features/pendingWrites/types'
import { SessionsPage } from './SessionsPage'
import { describeUserAgent, relativeTime } from './securityFormat'

const current = {
  id: '01890000-0000-7000-8000-00000000c001',
  userAgent: 'Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0',
  createdAt: '2026-07-01 10:00:00',
  lastUsedAt: '2026-07-10 09:00:00',
  isCurrent: true,
  provider: '',
}
const other = {
  id: '01890000-0000-7000-8000-00000000c002',
  userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) Safari/604.1',
  createdAt: '2026-07-02 10:00:00',
  lastUsedAt: '2026-07-09 09:00:00',
  isCurrent: false,
  provider: '',
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
    http.get('*/api/v1/oauth/get-provider-list', () =>
      HttpResponse.json({ success: true, message: '', data: [{ id: 'google', name: 'Google' }, { id: 'apple', name: 'Apple' }, { id: 'oidc', name: 'Authentik' }] }),
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

const waiting: PendingOp = {
  key: 'k1', seq: 1, attempted: false, status: 'waiting', retries: 0, notBefore: 0,
  kind: 'limit', budgetId: 'b', elementId: 'e', period: '2026-11-01', amount: '1',
}

it('warns that signing out of this device loses unsent changes, only when there are some', async () => {
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Sign out' }))
  expect(within(await screen.findByRole('dialog')).getByText('Sign out of this device?')).toBeInTheDocument()

  await act(() => mutate(() => [waiting]))
  expect(within(screen.getByRole('dialog')).getByText('Sign out of this device? 1 change has not been sent yet and will be lost.')).toBeInTheDocument()
})

it('does not warn about unsent changes when revoking another device', async () => {
  await mutate(() => [waiting])
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Revoke' }))
  expect((await screen.findByRole('dialog')).textContent).not.toContain('will be lost')
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

it('shows a "via {provider}" badge for a session opened through OAuth', async () => {
  server.use(
    http.get('*/api/v1/user/get-session-list', () =>
      HttpResponse.json({ success: true, message: '', data: [{ ...current, provider: 'google' }, other] }),
    ),
  )
  renderPage()
  expect(await screen.findByText('via Google')).toBeInTheDocument()
})

it('shows the configured custom-provider name instead of the catalogue "SSO"', async () => {
  server.use(
    http.get('*/api/v1/user/get-session-list', () =>
      HttpResponse.json({ success: true, message: '', data: [{ ...current, provider: 'oidc' }, other] }),
    ),
  )
  renderPage()
  expect(await screen.findByText('via Authentik')).toBeInTheDocument()
})

it('securityFormat helpers parse UA and relative time', () => {
  expect(describeUserAgent(current.userAgent)).toBe('Firefox on Linux')
  expect(describeUserAgent(other.userAgent)).toBe('Safari on iOS')
  expect(describeUserAgent('')).toBe('')
  expect(describeUserAgent('weird-cli/1.0')).toBe('weird-cli/1.0')

  const now = new Date(Date.UTC(2026, 6, 10, 12, 0, 0))
  expect(relativeTime('2026-07-10 11:59:30', { now })).toBe('just now')
  expect(relativeTime('2026-07-10 11:55:00', { now })).toBe('5 minutes ago')
  expect(relativeTime('2026-07-10 09:00:00', { now })).toBe('3 hours ago')
  expect(relativeTime('2026-07-07 12:00:00', { now })).toBe('3 days ago')
  expect(relativeTime('2026-07-10 11:59:30', { now, justNow: 'только что' })).toBe('только что')
  expect(relativeTime('2026-07-07 12:00:00', { now, lang: 'ru' })).toBe('3 дня назад')
})

it('offline: revoking another session is disabled, signing out still works', async () => {
  const user = userEvent.setup()
  renderPage()
  await screen.findByText(/Firefox on Linux/)
  act(() => onlineManager.setOnline(false))
  onTestFinished(() => act(() => onlineManager.setOnline(true)))
  expect(screen.getByRole('button', { name: 'Revoke' })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Sign out' }))
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByRole('button', { name: 'Sign out' }))
  await waitFor(() => expect(screen.getByTestId('logout-page')).toBeInTheDocument())
})

it('offline with nothing loaded on this device, the session list says so instead of an empty or loading page', async () => {
  onlineManager.setOnline(false)
  onTestFinished(() => onlineManager.setOnline(true))
  renderPage()
  expect(await screen.findByTestId('offline-unavailable')).toHaveTextContent('Not saved on this device')
})
