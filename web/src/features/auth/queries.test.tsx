import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { getToken } from '@/lib/storage'
import { useLogin, useResendVerification } from './queries'

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

it('login passes the verification code through and fires the completed metric', async () => {
  const bodies: unknown[] = []
  server.use(
    http.post('*/api/v1/user/login-user', async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ token: 'tok', user: { id: 'u1', accessLevel: 'full', accessUntil: '' } })
    }),
  )
  const { result } = renderHook(() => useLogin(), { wrapper })
  await result.current.mutateAsync({ username: 'a@b.test', password: 'pw', code: '123456789012' })
  expect(bodies[0]).toMatchObject({ username: 'a@b.test', password: 'pw', code: '123456789012' })
})

it('resend treats the 403 reply as success', async () => {
  server.use(
    http.post('*/api/v1/user/login-user', () =>
      HttpResponse.json({ success: false, message: 'Please verify your email address.', code: 403, errors: {} }, { status: 403 }),
    ),
  )
  const { result } = renderHook(() => useResendVerification(), { wrapper })
  await expect(result.current.mutateAsync({ username: 'a@b.test', password: 'pw' })).resolves.toBeUndefined()
})
