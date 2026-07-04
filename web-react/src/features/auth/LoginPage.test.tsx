import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { LoginPage } from './LoginPage'

function renderLogin(path = '/login') {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  const router = createMemoryRouter([{ path: '/login', element: <LoginPage /> }], { initialEntries: [path] })
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false } } })}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('logs in and stores the token', async () => {
  const assign = vi.fn()
  Object.defineProperty(window, 'location', { value: { ...window.location, assign }, writable: true })
  server.use(
    http.post('*/api/v1/user/login-user', () =>
      HttpResponse.json({
        user: { id: 'u1', name: 'Ada', email: 'a@b', avatar: '', options: [], currency: 'USD', reportPeriod: 'month' },
        token: 'jwt',
      }),
    ),
  )
  const user = userEvent.setup()
  renderLogin()
  await user.type(screen.getByLabelText(/e-?mail/i), 'ada@example.test')
  await user.type(screen.getByLabelText(/password/i), 'secret')
  await user.click(screen.getByRole('button', { name: /sign in/i }))
  await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('/'))
  expect(localStorage.getItem('token')).toBe('jwt')
})

it('shows the failure dialog on invalid credentials', async () => {
  server.use(
    http.post('*/api/v1/user/login-user', () =>
      HttpResponse.json({ success: false, message: 'Invalid credentials.', code: 0, errors: {} }, { status: 401 }),
    ),
  )
  const user = userEvent.setup()
  renderLogin()
  await user.type(screen.getByLabelText(/e-?mail/i), 'ada@example.test')
  await user.type(screen.getByLabelText(/password/i), 'wrong')
  await user.click(screen.getByRole('button', { name: /sign in/i }))
  expect(await screen.findByRole('dialog')).toBeInTheDocument()
})

it('shows the session-expired notice when reason=expired', () => {
  renderLogin('/login?reason=expired')
  expect(screen.getByText(/session has expired/i)).toBeInTheDocument()
})

it('hides the self-hosted section when custom API is not allowed', () => {
  window.econumoConfig = { ALLOW_CUSTOM_API: 'false' }
  renderLogin()
  expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
})

it('shows the self-hosted section when custom API is allowed', () => {
  window.econumoConfig = { ALLOW_CUSTOM_API: 'true' }
  renderLogin()
  expect(screen.getByRole('checkbox')).toBeInTheDocument()
})
