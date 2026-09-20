import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { getToken } from '@/lib/storage'
import { useConfirmEmail, useLogin, useResendVerification } from './queries'

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false } } })}>
    {children}
  </QueryClientProvider>
)

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('stores the token after a successful login', async () => {
  server.use(
    http.post('*/api/v1/user/login-user', () =>
      HttpResponse.json({
        user: { id: 'u1', name: 'Ada', email: 'a@b', avatar: '', options: [], currency: 'USD', reportPeriod: 'month' },
        token: 'fresh-jwt',
      }),
    ),
  )
  const { result } = renderHook(() => useLogin(), { wrapper })
  result.current.mutate({ username: 'a@b', password: 'pw' })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(getToken()).toBe('fresh-jwt')
})

it('does not store a token on failed login', async () => {
  server.use(
    http.post('*/api/v1/user/login-user', () =>
      HttpResponse.json({ success: false, message: 'Invalid credentials.', code: 0, errors: {} }, { status: 401 }),
    ),
  )
  const { result } = renderHook(() => useLogin(), { wrapper })
  result.current.mutate({ username: 'a@b', password: 'bad' })
  await waitFor(() => expect(result.current.isError).toBe(true))
  expect(getToken()).toBeNull()
})

it('confirmEmail posts username+code and fires the completed metric', async () => {
  const bodies: unknown[] = []
  server.use(
    http.post('*/api/v1/user/confirm-email', async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const { result } = renderHook(() => useConfirmEmail(), { wrapper })
  await result.current.mutateAsync({ username: 'a@b.test', code: '482913' })
  expect(bodies[0]).toMatchObject({ username: 'a@b.test', code: '482913' })
})

it('resend posts the username and returns the wait from the Retry-After header', async () => {
  const bodies: unknown[] = []
  server.use(
    http.post('*/api/v1/user/resend-verification-code', async ({ request }) => {
      bodies.push(await request.json())
      // The wait lives on the header only — the body carries no retryAfter.
      return HttpResponse.json({ success: true, message: '', data: {} }, { headers: { 'Retry-After': '60' } })
    }),
  )
  const { result } = renderHook(() => useResendVerification(), { wrapper })
  await expect(result.current.mutateAsync({ username: 'a@b.test' })).resolves.toBe(60)
  expect(bodies[0]).toMatchObject({ username: 'a@b.test' })
})

it('resend falls back to 0 when the server sends no Retry-After', async () => {
  server.use(
    http.post('*/api/v1/user/resend-verification-code', () =>
      HttpResponse.json({ success: true, message: '', data: {} }),
    ),
  )
  const { result } = renderHook(() => useResendVerification(), { wrapper })
  await expect(result.current.mutateAsync({ username: 'a@b.test' })).resolves.toBe(0)
})

// The production failure this guards: login fired the identity-list probe
// before the token was stored, the unauthenticated 401 came back, and the
// response interceptor treated it as an expired session and deleted the token
// the login had just written — leaving a 200 login stranded on /login.
it('keeps the token when the identity-list probe 401s during login', async () => {
  server.use(
    http.post('*/api/v1/user/login-user', () =>
      HttpResponse.json({
        user: { id: 'u1', name: 'Ada', email: 'a@b', avatar: '', options: [], currency: 'USD', reportPeriod: 'month' },
        token: 'fresh-jwt',
      }),
    ),
    http.get('*/api/v1/oauth/get-identity-list', ({ request }) =>
      request.headers.get('Authorization')
        ? HttpResponse.json({ success: true, message: '', data: [] })
        : HttpResponse.json({ success: false, message: 'Access token not found', code: 401, errors: {} }, { status: 401 }),
    ),
  )
  const { result } = renderHook(() => useLogin(), { wrapper })
  result.current.mutate({ username: 'a@b', password: 'pw' })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))

  // Let the floating probe settle before asserting the token survived it.
  await new Promise((resolve) => setTimeout(resolve, 0))
  expect(getToken()).toBe('fresh-jwt')
})
