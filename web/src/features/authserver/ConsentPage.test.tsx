import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AxiosError } from 'axios'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { approveAuthorization, declineAuthorization, getAuthorizationRequest } from '@/api/authserver'
import { METRICS, trackEvent } from '@/lib/metrics'
import { takePostLoginRedirect } from './postLoginRedirect'
import { ConsentPage } from './ConsentPage'

vi.mock('@/api/authserver', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/api/authserver')>()),
  getAuthorizationRequest: vi.fn(),
  approveAuthorization: vi.fn(),
  declineAuthorization: vi.fn(),
}))
vi.mock('@/lib/metrics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/metrics')>()),
  trackEvent: vi.fn(),
}))

const SEARCH =
  '?client_id=abc&redirect_uri=https%3A%2F%2Fclaude.ai%2Fapi%2Fmcp%2Fauth_callback&response_type=code&code_challenge=x&code_challenge_method=S256&state=s'
const CALLBACK = 'https://claude.ai/api/mcp/auth_callback?code=c&state=s'

const claude = { clientName: 'Claude', redirectHost: 'claude.ai', isLoopback: false, errorRedirectUrl: '' }

function httpError(status: number, message: string) {
  return new AxiosError('failed', String(status), undefined, undefined, {
    status,
    data: { success: false, message, code: status, errors: {} },
  } as never)
}

function renderPage(search = SEARCH) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/oauth/authorize', element: <ConsentPage /> },
      { path: '/logout', element: <div data-testid="logout" /> },
    ],
    { initialEntries: [`/oauth/authorize${search}`] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return router
}

let assign: ReturnType<typeof vi.fn>

beforeEach(() => {
  localStorage.clear()
  sessionStorage.clear()
  window.econumoConfig = {}
  assign = vi.fn()
  Object.defineProperty(window, 'location', { value: { ...window.location, assign }, writable: true })
  vi.mocked(getAuthorizationRequest).mockReset()
  vi.mocked(approveAuthorization).mockReset()
  vi.mocked(declineAuthorization).mockReset()
  vi.mocked(trackEvent).mockClear()
  server.use(
    http.get('*/api/v1/user/get-user-data', () =>
      HttpResponse.json({
        success: true,
        message: '',
        data: { user: { id: 'u1', name: 'Ada', email: 'ada@example.test', avatar: 'face:fuchsia', options: [], accessLevel: 'full', accessUntil: '', hasPassword: true } },
      }),
    ),
  )
})

it('shows the client and where it will send the user, then approves', async () => {
  vi.mocked(getAuthorizationRequest).mockResolvedValue(claude)
  vi.mocked(approveAuthorization).mockResolvedValue({ redirectUrl: CALLBACK })
  renderPage()
  expect(await screen.findByText('Claude')).toBeInTheDocument()
  expect(screen.getByText(/claude\.ai/)).toBeInTheDocument()
  expect(await screen.findByText(/ada@example\.test/)).toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: /allow/i }))
  await waitFor(() => expect(assign).toHaveBeenCalledWith(CALLBACK))
  expect(approveAuthorization).toHaveBeenCalledWith(
    expect.objectContaining({ clientId: 'abc', state: 's', redirectUri: 'https://claude.ai/api/mcp/auth_callback', codeChallengeMethod: 'S256' }),
  )
  expect(trackEvent).toHaveBeenCalledWith(METRICS.CONNECTED_APP_APPROVE, {})
  expect(takePostLoginRedirect()).toBe('/')
})

it('reads a loopback client as an app on this computer', async () => {
  vi.mocked(getAuthorizationRequest).mockResolvedValue({ clientName: 'Codex', redirectHost: '127.0.0.1', isLoopback: true, errorRedirectUrl: '' })
  renderPage()
  expect(await screen.findByText('Codex')).toBeInTheDocument()
  expect(screen.getByText(/app on this computer/)).toBeInTheDocument()
  expect(screen.queryByText(/127\.0\.0\.1/)).not.toBeInTheDocument()
})

it('deny navigates to the access_denied redirect', async () => {
  vi.mocked(getAuthorizationRequest).mockResolvedValue(claude)
  vi.mocked(declineAuthorization).mockResolvedValue({ redirectUrl: 'https://claude.ai/api/mcp/auth_callback?error=access_denied&state=s' })
  renderPage()
  await userEvent.click(await screen.findByRole('button', { name: /deny/i }))
  await waitFor(() => expect(assign).toHaveBeenCalledWith('https://claude.ai/api/mcp/auth_callback?error=access_denied&state=s'))
  expect(declineAuthorization).toHaveBeenCalledWith(expect.objectContaining({ clientId: 'abc', state: 's' }))
  expect(approveAuthorization).not.toHaveBeenCalled()
  expect(trackEvent).not.toHaveBeenCalled()
})

it('an invalid request shows an error card and only navigates when the user asks', async () => {
  vi.mocked(getAuthorizationRequest).mockResolvedValue({
    ...claude,
    errorRedirectUrl: 'https://claude.ai/api/mcp/auth_callback?error=invalid_request&state=s',
  })
  renderPage()
  expect(await screen.findByText(/sent an invalid request/i)).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /allow/i })).not.toBeInTheDocument()
  await new Promise((r) => setTimeout(r, 30))
  expect(assign).not.toHaveBeenCalled()
  await userEvent.click(screen.getByRole('button', { name: 'Return to claude.ai' }))
  expect(assign).toHaveBeenCalledWith('https://claude.ai/api/mcp/auth_callback?error=invalid_request&state=s')
})

it('an invalid request from a loopback client offers a return to the app', async () => {
  vi.mocked(getAuthorizationRequest).mockResolvedValue({
    clientName: 'Codex', redirectHost: '127.0.0.1', isLoopback: true,
    errorRedirectUrl: 'http://127.0.0.1:1455/cb?error=invalid_request',
  })
  renderPage()
  await userEvent.click(await screen.findByRole('button', { name: 'Return to the app' }))
  expect(assign).toHaveBeenCalledWith('http://127.0.0.1:1455/cb?error=invalid_request')
})

it('an unknown client shows the server message and never navigates', async () => {
  vi.mocked(getAuthorizationRequest).mockRejectedValue(httpError(400, 'This app is not registered.'))
  renderPage()
  expect(await screen.findByText('This app is not registered.')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /allow|deny|return/i })).not.toBeInTheDocument()
  expect(assign).not.toHaveBeenCalled()
  expect(takePostLoginRedirect()).toBe('/')
})

it('a read-only user is told why and can send the app back with a refusal', async () => {
  vi.mocked(getAuthorizationRequest).mockResolvedValue(claude)
  vi.mocked(approveAuthorization).mockRejectedValue(httpError(402, 'Read-only access. Write operations are disabled.'))
  vi.mocked(declineAuthorization).mockResolvedValue({ redirectUrl: 'https://claude.ai/api/mcp/auth_callback?error=access_denied&state=s' })
  renderPage()
  await userEvent.click(await screen.findByRole('button', { name: /allow/i }))
  expect(await screen.findByText(/read-only/i)).toBeInTheDocument()
  expect(assign).not.toHaveBeenCalled()
  expect(trackEvent).not.toHaveBeenCalled()
  expect(screen.queryByRole('button', { name: /allow/i })).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: 'Return to the app' }))
  await waitFor(() => expect(assign).toHaveBeenCalledWith('https://claude.ai/api/mcp/auth_callback?error=access_denied&state=s'))
})

it('shows another failure inline and keeps the buttons usable', async () => {
  vi.mocked(getAuthorizationRequest).mockResolvedValue(claude)
  vi.mocked(approveAuthorization).mockRejectedValue(httpError(500, 'Something broke'))
  renderPage()
  await userEvent.click(await screen.findByRole('button', { name: /allow/i }))
  expect(await screen.findByText('Something broke')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /allow/i })).toBeEnabled()
})

it('remembers its URL for after login and links to switch account', async () => {
  vi.mocked(getAuthorizationRequest).mockResolvedValue(claude)
  renderPage()
  await screen.findByText('Claude')
  expect(takePostLoginRedirect()).toBe(`/oauth/authorize${SEARCH}`)
  expect(screen.getByRole('link', { name: /switch account/i })).toHaveAttribute('href', '/logout')
})
