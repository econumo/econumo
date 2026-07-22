import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { VerifyEmailDialog } from './VerifyEmailDialog'

function renderDialog(onClose = vi.fn()) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false } } })}>
      <VerifyEmailDialog open onClose={onClose} username="ada@example.test" password="pw12345678" />
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
  await user.type(screen.getByLabelText(/code/i), '123456789012')
  await user.click(screen.getByRole('button', { name: /verify and sign in/i }))
  await vi.waitFor(() => expect(loginBodies).toHaveLength(1))
  expect(confirmBodies[0]).toMatchObject({ username: 'ada@example.test', code: '123456789012' })
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
  await user.type(screen.getByLabelText(/code/i), '999999999999')
  await user.click(screen.getByRole('button', { name: /verify and sign in/i }))
  expect(await screen.findByText(/confirmation code is not valid/i)).toBeInTheDocument()
})

it('resend confirms and stays open', async () => {
  const user = userEvent.setup()
  server.use(
    http.post('*/api/v1/user/resend-verification-code', () =>
      HttpResponse.json({ success: true, message: '', data: {} }),
    ),
  )
  renderDialog()
  await user.click(screen.getByRole('button', { name: /resend code/i }))
  expect(await screen.findByText(/new code has been sent/i)).toBeInTheDocument()
})

it('is dismissible via Cancel', async () => {
  const user = userEvent.setup()
  const onClose = renderDialog()
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(onClose).toHaveBeenCalled()
})
