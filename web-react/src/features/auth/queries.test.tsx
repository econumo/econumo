import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { getToken } from '@/lib/storage'
import { useLogin } from './queries'

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
        success: true,
        message: '',
        data: { user: { id: 'u1', name: 'Ada', email: 'a@b', avatar: '', options: [], currency: 'USD', reportPeriod: 'month' }, token: 'fresh-jwt' },
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
