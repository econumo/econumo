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

it('verifies by re-submitting login with the code', async () => {
  const user = userEvent.setup()
  const bodies: Record<string, unknown>[] = []
  server.use(
    http.post('*/api/v1/user/login-user', async ({ request }) => {
      bodies.push((await request.json()) as Record<string, unknown>)
      return HttpResponse.json({ token: 'tok-verified', user: { id: 'u1', accessLevel: 'full', accessUntil: '' } })
    }),
  )
  renderDialog()
  await user.type(screen.getByLabelText(/code/i), '123456789012')
  await user.click(screen.getByRole('button', { name: /verify and sign in/i }))
  await vi.waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toMatchObject({ username: 'ada@example.test', password: 'pw12345678', code: '123456789012' })
})

it('shows the server message inline on an invalid code', async () => {
  const user = userEvent.setup()
  server.use(
    http.post('*/api/v1/user/login-user', () =>
      HttpResponse.json({ success: false, message: 'The confirmation code is not valid.', code: 403, errors: {} }, { status: 403 }),
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
    http.post('*/api/v1/user/login-user', () =>
      HttpResponse.json({ success: false, message: 'Please verify your email address.', code: 403, errors: {} }, { status: 403 }),
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
