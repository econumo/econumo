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
  // a previous user's persisted finances must not survive a new sign-in
  localStorage.setItem('econumo.query-cache', '{"stale":"finances"}')
  await user.type(screen.getByLabelText('Email'), 'ada@example.test')
  await user.type(screen.getByLabelText('Password'), 'secret')
  await user.click(screen.getByRole('button', { name: /sign in/i }))
  await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('/'))
  expect(localStorage.getItem('token')).toBe('jwt')
  expect(localStorage.getItem('econumo.query-cache')).toBeNull()
})

it('opens the verification dialog on a 403 login', async () => {
  const user = userEvent.setup()
  server.use(
    http.post('*/api/v1/user/login-user', () =>
      HttpResponse.json({ success: false, message: 'Please verify your email address.', code: 403, errors: {} }, { status: 403 }),
    ),
  )
  renderLogin()
  await user.type(screen.getByLabelText('Email'), 'ada@example.test')
  await user.type(screen.getByLabelText('Password'), 'pw12345678')
  await user.click(screen.getByRole('button', { name: /sign in/i }))
  expect(await screen.findByText(/verify your email/i)).toBeInTheDocument()
  expect(screen.queryByText(/sign-in failed/i)).not.toBeInTheDocument()
})

// The 403 carries the server's remaining wait on Retry-After, so the dialog's
// countdown is already correct when it opens — no extra round trip, no guess.
it('seeds the resend countdown from the 403 Retry-After header', async () => {
  const user = userEvent.setup()
  server.use(
    http.post('*/api/v1/user/login-user', () =>
      HttpResponse.json(
        { success: false, message: 'Please verify your email address.', code: 403, errors: {} },
        { status: 403, headers: { 'Retry-After': '23' } },
      ),
    ),
  )
  renderLogin()
  await user.type(screen.getByLabelText('Email'), 'ada@example.test')
  await user.type(screen.getByLabelText('Password'), 'pw12345678')
  await user.click(screen.getByRole('button', { name: /sign in/i }))
  const resend = await screen.findByRole('button', { name: /resend code in/i })
  expect(resend).toBeDisabled()
  expect(resend).toHaveTextContent('23s')
})

it('shows the failure dialog on invalid credentials', async () => {
  server.use(
    http.post('*/api/v1/user/login-user', () =>
      HttpResponse.json({ success: false, message: 'Invalid credentials.', code: 0, errors: {} }, { status: 401 }),
    ),
  )
  const user = userEvent.setup()
  renderLogin()
  await user.type(screen.getByLabelText('Email'), 'ada@example.test')
  await user.type(screen.getByLabelText('Password'), 'wrong')
  await user.click(screen.getByRole('button', { name: /sign in/i }))
  expect(await screen.findByRole('dialog')).toBeInTheDocument()
})

it('stores the email immediately when the box is checked with an email typed', async () => {
  const user = userEvent.setup()
  renderLogin()
  await user.type(screen.getByLabelText('Email'), 'ada@example.test')
  await user.click(screen.getByLabelText('Remember me'))
  expect(localStorage.getItem('rememberedEmail')).toBe(JSON.stringify('ada@example.test'))
})

it('keeps the stored email in sync while typing with the box checked', async () => {
  const user = userEvent.setup()
  renderLogin()
  await user.click(screen.getByLabelText('Remember me'))
  await user.type(screen.getByLabelText('Email'), 'ada@example.test')
  expect(localStorage.getItem('rememberedEmail')).toBe(JSON.stringify('ada@example.test'))
  await user.type(screen.getByLabelText('Email'), '.io')
  expect(localStorage.getItem('rememberedEmail')).toBe(JSON.stringify('ada@example.test.io'))
})

it('prefills the email and checks the box when an email is remembered', () => {
  localStorage.setItem('rememberedEmail', JSON.stringify('ada@example.test'))
  renderLogin()
  expect(screen.getByLabelText('Email')).toHaveValue('ada@example.test')
  expect(screen.getByLabelText('Remember me')).toBeChecked()
})

it('forgets the remembered email as soon as the box is unchecked', async () => {
  localStorage.setItem('rememberedEmail', JSON.stringify('ada@example.test'))
  const user = userEvent.setup()
  renderLogin()
  await user.click(screen.getByLabelText('Remember me'))
  expect(localStorage.getItem('rememberedEmail')).toBeNull()
  await user.type(screen.getByLabelText('Email'), 'x')
  expect(localStorage.getItem('rememberedEmail')).toBeNull()
})

it('does not store the email while typing with the box unchecked', async () => {
  const user = userEvent.setup()
  renderLogin()
  await user.type(screen.getByLabelText('Email'), 'ada@example.test')
  expect(localStorage.getItem('rememberedEmail')).toBeNull()
})

it('shows the session-expired notice when reason=expired', () => {
  renderLogin('/login?reason=expired')
  expect(screen.getByText(/session has expired/i)).toBeInTheDocument()
})

it('stores the server address as it is typed', async () => {
  window.econumoConfig = { ALLOW_CUSTOM_API: 'true' }
  const user = userEvent.setup()
  renderLogin()
  await user.click(screen.getByRole('button', { name: /custom server/i }))
  const host = screen.getByLabelText('Server address')
  await user.clear(host)
  await user.type(host, 'https://my.box.test')
  expect(localStorage.getItem('backendHost')).toBe(JSON.stringify('https://my.box.test'))
})

it('hides the custom-server option when custom API is not allowed', () => {
  window.econumoConfig = { ALLOW_CUSTOM_API: 'false' }
  renderLogin()
  expect(screen.queryByRole('button', { name: /custom server/i })).not.toBeInTheDocument()
})

it('persists the collapse and clears the server address', async () => {
  window.econumoConfig = { ALLOW_CUSTOM_API: 'true' }
  localStorage.setItem('selfHosted', 'true')
  localStorage.setItem('backendHost', JSON.stringify('https://old.example.test'))
  const user = userEvent.setup()
  renderLogin()
  expect(screen.getByLabelText('Server address')).toHaveValue('https://old.example.test')
  await user.click(screen.getByRole('button', { name: /custom server/i }))
  expect(localStorage.getItem('selfHosted')).toBe('false')
  expect(localStorage.getItem('backendHost')).toBeNull()
  await user.click(screen.getByRole('button', { name: /custom server/i }))
  expect(screen.getByLabelText('Server address')).toHaveValue(window.location.origin)
})

it('prefills the current origin when expanding with no custom server configured', async () => {
  window.econumoConfig = { ALLOW_CUSTOM_API: 'true' }
  const user = userEvent.setup()
  renderLogin()
  await user.click(screen.getByRole('button', { name: /custom server/i }))
  expect(screen.getByLabelText('Server address')).toHaveValue(window.location.origin)
})

it('reveals the server address field through the custom-server disclosure', async () => {
  window.econumoConfig = { ALLOW_CUSTOM_API: 'true' }
  const user = userEvent.setup()
  renderLogin()
  expect(screen.queryByLabelText('Server address')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: /custom server/i }))
  expect(screen.getByLabelText('Server address')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: /custom server/i }))
  expect(screen.queryByLabelText('Server address')).not.toBeInTheDocument()
})
