import { act, render, screen, waitFor } from '@testing-library/react'
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

function pointerMove(dx: number) {
  const e = new Event('pointermove')
  Object.assign(e, { movementX: dx, movementY: 0 })
  act(() => {
    window.dispatchEvent(e)
  })
}

// Allow is armed by a real interaction with the page while it is focused
// (jsdom reports no focus by default); tests move the pointer.
async function clickAllow() {
  vi.spyOn(document, 'hasFocus').mockReturnValue(true)
  const allow = await screen.findByRole('button', { name: /allow/i })
  pointerMove(4)
  await waitFor(() => expect(allow).toBeEnabled())
  await userEvent.click(allow)
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
  await clickAllow()
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

it('a stale session keeps the consent URL for after the sign-in', async () => {
  vi.mocked(getAuthorizationRequest).mockRejectedValue(httpError(401, 'Invalid access token'))
  renderPage()
  await waitFor(() => expect(getAuthorizationRequest).toHaveBeenCalled())
  await new Promise((r) => setTimeout(r, 30))
  expect(takePostLoginRedirect()).toBe(`/oauth/authorize${SEARCH}`)
})

it('an invalid request drops the remembered URL', async () => {
  vi.mocked(getAuthorizationRequest).mockResolvedValue({ ...claude, errorRedirectUrl: 'https://claude.ai/cb?error=invalid_request' })
  renderPage()
  await screen.findByText(/sent an invalid request/i)
  expect(takePostLoginRedirect()).toBe('/')
})

it('never navigates to a non-http(s) redirect', async () => {
  vi.mocked(getAuthorizationRequest).mockResolvedValue(claude)
  vi.mocked(approveAuthorization).mockResolvedValue({ redirectUrl: 'javascript:alert(1)' })
  renderPage()
  await clickAllow()
  expect(await screen.findByText(/sent an invalid request/i)).toBeInTheDocument()
  expect(assign).not.toHaveBeenCalled()
  expect(trackEvent).not.toHaveBeenCalled()
})

it('offers no return button for a non-http(s) error redirect', async () => {
  vi.mocked(getAuthorizationRequest).mockResolvedValue({ ...claude, errorRedirectUrl: 'data:text/html,x' })
  renderPage()
  expect(await screen.findByText(/sent an invalid request/i)).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /return/i })).not.toBeInTheDocument()
  expect(assign).not.toHaveBeenCalled()
})

it('a read-only user is told why and can send the app back with a refusal', async () => {
  vi.mocked(getAuthorizationRequest).mockResolvedValue(claude)
  vi.mocked(approveAuthorization).mockRejectedValue(httpError(402, 'Read-only access. Write operations are disabled.'))
  vi.mocked(declineAuthorization).mockResolvedValue({ redirectUrl: 'https://claude.ai/api/mcp/auth_callback?error=access_denied&state=s' })
  renderPage()
  await clickAllow()
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
  await clickAllow()
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

describe('double-clickjacking guard', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  async function renderConsent() {
    vi.mocked(getAuthorizationRequest).mockResolvedValue(claude)
    renderPage()
    await screen.findByText('Claude')
  }

  it('keeps Allow disabled until the page has been visible and focused for a moment', async () => {
    const focused = vi.spyOn(document, 'hasFocus').mockReturnValue(false)
    await renderConsent()
    vi.useFakeTimers()
    const allow = screen.getByRole('button', { name: /allow/i })
    expect(allow).toBeDisabled()
    expect(screen.getByRole('button', { name: /deny/i })).toBeEnabled()

    act(() => {
      vi.advanceTimersByTime(2000)
    })
    expect(allow).toBeDisabled() // never focused: the clock does not run

    focused.mockReturnValue(true)
    act(() => {
      window.dispatchEvent(new Event('focus'))
    })
    act(() => {
      vi.advanceTimersByTime(300)
    })
    expect(allow).toBeDisabled()
    focused.mockReturnValue(false)
    act(() => {
      window.dispatchEvent(new Event('blur'))
    })
    focused.mockReturnValue(true)
    act(() => {
      window.dispatchEvent(new Event('focus'))
    })
    act(() => {
      vi.advanceTimersByTime(300)
    })
    expect(allow).toBeDisabled() // losing focus restarted the wait
    act(() => {
      vi.advanceTimersByTime(250)
    })
    expect(allow).toBeEnabled()
  })

  it('arms on a keypress or a moving pointer while focused, not a motionless one', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true)
    await renderConsent()
    const allow = screen.getByRole('button', { name: /allow/i })
    pointerMove(0)
    expect(allow).toBeDisabled()
    act(() => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab' }))
    })
    expect(allow).toBeEnabled()
  })

  it('arms on pointer movement while focused', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true)
    await renderConsent()
    const allow = screen.getByRole('button', { name: /allow/i })
    expect(allow).toBeDisabled()
    pointerMove(-3)
    expect(allow).toBeEnabled()
  })

  it('does not arm on pointer movement or a keypress while the window is not focused', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(false)
    await renderConsent()
    const allow = screen.getByRole('button', { name: /allow/i })
    pointerMove(5)
    act(() => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab' }))
    })
    expect(allow).toBeDisabled()
  })

  it('disarms when the window loses focus and re-arms only after a fresh interval', async () => {
    const focused = vi.spyOn(document, 'hasFocus').mockReturnValue(true)
    await renderConsent()
    vi.useFakeTimers()
    const allow = screen.getByRole('button', { name: /allow/i })
    pointerMove(4)
    expect(allow).toBeEnabled()

    focused.mockReturnValue(false)
    act(() => {
      window.dispatchEvent(new Event('blur'))
    })
    expect(allow).toBeDisabled()

    focused.mockReturnValue(true)
    act(() => {
      window.dispatchEvent(new Event('focus'))
    })
    expect(allow).toBeDisabled()
    act(() => {
      vi.advanceTimersByTime(499)
    })
    expect(allow).toBeDisabled()
    act(() => {
      vi.advanceTimersByTime(2)
    })
    expect(allow).toBeEnabled()
  })

  it('cancels the pending timer when focus is lost before it fires', async () => {
    const focused = vi.spyOn(document, 'hasFocus').mockReturnValue(true)
    await renderConsent()
    vi.useFakeTimers()
    const allow = screen.getByRole('button', { name: /allow/i })
    act(() => {
      window.dispatchEvent(new Event('focus'))
      vi.advanceTimersByTime(300)
    })
    focused.mockReturnValue(false)
    act(() => {
      window.dispatchEvent(new Event('blur'))
      vi.advanceTimersByTime(2000)
    })
    expect(allow).toBeDisabled()
  })

  it('disarms when the page becomes hidden and re-arms after a fresh interval once visible', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true)
    const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
    await renderConsent()
    vi.useFakeTimers()
    const allow = screen.getByRole('button', { name: /allow/i })
    act(() => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab' }))
    })
    expect(allow).toBeEnabled()

    visibility.mockReturnValue('hidden')
    act(() => {
      document.dispatchEvent(new Event('visibilitychange'))
    })
    expect(allow).toBeDisabled()
    pointerMove(5)
    expect(allow).toBeDisabled() // hidden pages cannot arm either

    visibility.mockReturnValue('visible')
    act(() => {
      document.dispatchEvent(new Event('visibilitychange'))
    })
    expect(allow).toBeDisabled()
    act(() => {
      vi.advanceTimersByTime(499)
    })
    expect(allow).toBeDisabled()
    act(() => {
      vi.advanceTimersByTime(2)
    })
    expect(allow).toBeEnabled()
  })

  it('leaves Deny usable before the guard arms', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(false)
    vi.mocked(declineAuthorization).mockResolvedValue({ redirectUrl: 'https://claude.ai/api/mcp/auth_callback?error=access_denied&state=s' })
    await renderConsent()
    expect(screen.getByRole('button', { name: /allow/i })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: /deny/i }))
    await waitFor(() => expect(assign).toHaveBeenCalledWith('https://claude.ai/api/mcp/auth_callback?error=access_denied&state=s'))
  })
})
