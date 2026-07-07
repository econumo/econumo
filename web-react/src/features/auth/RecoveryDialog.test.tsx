import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { RecoveryDialog } from './RecoveryDialog'

function renderDialog(onClose = vi.fn()) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false } } })}>
      <RecoveryDialog open onClose={onClose} />
    </QueryClientProvider>,
  )
  return onClose
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('walks the two-step recovery flow', async () => {
  const user = userEvent.setup()
  const calls: string[] = []
  server.use(
    http.post('*/api/v1/user/remind-password', () => {
      calls.push('remind')
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
    http.post('*/api/v1/user/reset-password', () => {
      calls.push('reset')
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const onClose = renderDialog()

  await user.type(screen.getByLabelText(/e-?mail/i), 'ada@example.test')
  await user.click(screen.getByRole('button', { name: /send code/i }))
  expect(await screen.findByLabelText(/code/i)).toBeInTheDocument()

  await user.type(screen.getByLabelText(/code/i), '123456789012')
  await user.type(screen.getByLabelText('Password'), 'newpass1')
  await user.click(screen.getByRole('button', { name: /reset password/i }))

  await vi.waitFor(() => expect(onClose).toHaveBeenCalled())
  expect(calls).toEqual(['remind', 'reset'])
})

it('validates the email before sending', async () => {
  const user = userEvent.setup()
  renderDialog()
  await user.type(screen.getByLabelText(/e-?mail/i), 'not-an-email')
  await user.click(screen.getByRole('button', { name: /send code/i }))
  expect(await screen.findByText(/enter a valid email/i)).toBeInTheDocument()
})
