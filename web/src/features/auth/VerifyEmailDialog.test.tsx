import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { VerifyEmailDialog } from './VerifyEmailDialog'

function renderDialog(onClose = vi.fn(), cooldownSeconds?: number) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false } } })}>
      <VerifyEmailDialog open onClose={onClose} username="ada@example.test" password="pw12345678" cooldownSeconds={cooldownSeconds} />
    </QueryClientProvider>,
  )
  return onClose
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('confirms the code then silently re-logs-in', async () => {
  const user = userEvent.setup()
  const confirmBodies: Record<string, unknown>[] = []
  const loginBodies: Record<string, unknown>[] = []
  server.use(
    http.post('*/api/v1/user/confirm-email', async ({ request }) => {
      confirmBodies.push((await request.json()) as Record<string, unknown>)
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
    http.post('*/api/v1/user/login-user', async ({ request }) => {
      loginBodies.push((await request.json()) as Record<string, unknown>)
      return HttpResponse.json({ token: 'tok-verified', user: { id: 'u1', accessLevel: 'full', accessUntil: '' } })
    }),
  )
  renderDialog()
  await user.type(screen.getByLabelText(/code/i), '482913')
  await user.click(screen.getByRole('button', { name: /verify and sign in/i }))
  await vi.waitFor(() => expect(loginBodies).toHaveLength(1))
  expect(confirmBodies[0]).toMatchObject({ username: 'ada@example.test', code: '482913' })
  expect(loginBodies[0]).toMatchObject({ username: 'ada@example.test', password: 'pw12345678' })
})

it('shows the server message inline on an invalid code', async () => {
  const user = userEvent.setup()
  server.use(
    http.post('*/api/v1/user/confirm-email', () =>
      HttpResponse.json({ success: false, message: 'The confirmation code is not valid.', code: 400, errors: {} }, { status: 400 }),
    ),
  )
  renderDialog()
  await user.type(screen.getByLabelText(/code/i), '999999')
  await user.click(screen.getByRole('button', { name: /verify and sign in/i }))
  expect(await screen.findByText(/confirmation code is not valid/i)).toBeInTheDocument()
})

it('rejects a code that is not six digits before calling the API', async () => {
  const user = userEvent.setup()
  let called = false
  server.use(
    http.post('*/api/v1/user/confirm-email', () => {
      called = true
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  renderDialog()
  await user.type(screen.getByLabelText(/code/i), '12345')
  await user.click(screen.getByRole('button', { name: /verify and sign in/i }))
  expect(await screen.findByText(/valid code/i)).toBeInTheDocument()
  expect(called).toBe(false)
})

it('holds resend behind the cooldown, then enables it', async () => {
  vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'setTimeout', 'clearTimeout', 'Date'] })
  try {
    renderDialog()
    // A code was already emailed by the blocked login, so resend starts locked.
    const label = () => screen.getByRole('button', { name: /resend code/i })
    expect(label()).toBeDisabled()
    expect(label()).toHaveTextContent('60s')

    await act(async () => { await vi.advanceTimersByTimeAsync(30_000) })
    expect(label()).toBeDisabled()
    expect(label()).toHaveTextContent('30s')

    await act(async () => { await vi.advanceTimersByTimeAsync(30_000) })
    expect(label()).toBeEnabled()
  } finally {
    vi.useRealTimers()
  }
})

// The server owns the wait: a reload mid-cooldown re-opens the dialog with the
// real remaining time, not a fresh optimistic 60.
it('starts the countdown from the server-supplied wait', async () => {
  vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'setTimeout', 'clearTimeout', 'Date'] })
  try {
    renderDialog(vi.fn(), 17)
    const label = () => screen.getByRole('button', { name: /resend code/i })
    expect(label()).toHaveTextContent('17s')
    await act(async () => { await vi.advanceTimersByTimeAsync(17_000) })
    expect(label()).toBeEnabled()
  } finally {
    vi.useRealTimers()
  }
})

it('resend confirms, stays open, and re-locks using the returned wait', async () => {
  const user = userEvent.setup()
  server.use(
    http.post('*/api/v1/user/resend-verification-code', () =>
      HttpResponse.json({ success: true, message: '', data: {} }, { headers: { 'Retry-After': '45' } }),
    ),
  )
  // Zero cooldown so the click lands immediately; the gating itself is covered above.
  renderDialog(vi.fn(), 0)
  await user.click(screen.getByRole('button', { name: /resend code/i }))
  expect(await screen.findByText(/new code has been sent/i)).toBeInTheDocument()
  // The button re-locks at the server's number, not the local default.
  const resend = await screen.findByRole('button', { name: /resend code in/i })
  expect(resend).toBeDisabled()
  expect(resend).toHaveTextContent('45s')
})

// The attempt cap (429) is a longer wait than the per-send gap; the button must
// stay locked for the server's stated duration instead of re-enabling at once.
it('locks the button for the Retry-After of a 429', async () => {
  const user = userEvent.setup()
  server.use(
    http.post('*/api/v1/user/resend-verification-code', () =>
      HttpResponse.json(
        { success: false, message: 'Too many attempts. Try again later.', code: 429, errors: {} },
        { status: 429, headers: { 'Retry-After': '900' } },
      ),
    ),
  )
  renderDialog(vi.fn(), 0)
  await user.click(screen.getByRole('button', { name: /resend code/i }))
  expect(await screen.findByText(/too many attempts/i)).toBeInTheDocument()
  const resend = await screen.findByRole('button', { name: /resend code in/i })
  expect(resend).toBeDisabled()
  expect(resend).toHaveTextContent('900s')
})

it('is dismissible via Cancel', async () => {
  const user = userEvent.setup()
  const onClose = renderDialog()
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(onClose).toHaveBeenCalled()
})
