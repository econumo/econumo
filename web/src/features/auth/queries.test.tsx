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
