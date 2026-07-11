import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { fixtureUser } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { useUpdateName, useUpdateAvatar, useUpdateDefaultBudget, useUpdatePassword } from './queries'

function makeWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  return { queryClient, wrapper }
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('update-name replaces the user cache with the echoed user', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/update-name', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { user: { ...fixtureUser, name: 'Renamed' } } })
    }),
  )
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData(queryKeys.user, fixtureUser)
  const { result } = renderHook(() => useUpdateName(), { wrapper })
  result.current.mutate('Renamed')
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(body).toEqual({ name: 'Renamed' })
  expect(queryClient.getQueryData<{ name: string }>(queryKeys.user)!.name).toBe('Renamed')
})

it('update-avatar replaces the user cache and invalidates avatar-embedding lists', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/update-avatar', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { user: { ...fixtureUser, avatar: 'pets' } } })
    }),
  )
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData(queryKeys.user, fixtureUser)
  const spy = vi.spyOn(queryClient, 'invalidateQueries')
  const { result } = renderHook(() => useUpdateAvatar(), { wrapper })
  result.current.mutate({ icon: 'pets', color: 'teal' })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(body).toEqual({ icon: 'pets', color: 'teal' })
  expect(queryClient.getQueryData<{ avatar: string }>(queryKeys.user)!.avatar).toBe('pets')
  expect(spy).toHaveBeenCalled()
})

it('update-budget posts {value} and invalidates the budget cache', async () => {
  server.use(
    http.post('*/api/v1/user/update-budget', async ({ request }) => {
      expect(await request.json()).toEqual({ value: 'b1' })
      return HttpResponse.json({ success: true, message: '', data: { user: fixtureUser } })
    }),
  )
  const { queryClient, wrapper } = makeWrapper()
  const spy = vi.spyOn(queryClient, 'invalidateQueries')
  const { result } = renderHook(() => useUpdateDefaultBudget(), { wrapper })
  result.current.mutate('b1')
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(spy).toHaveBeenCalledWith({ queryKey: queryKeys.budget })
})

it('update-password rejects on the generic 400 envelope', async () => {
  server.use(
    http.post('*/api/v1/user/update-password', () =>
      HttpResponse.json({ success: false, message: 'Form validation error', code: 400, errors: {} }, { status: 400 }),
    ),
  )
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useUpdatePassword(), { wrapper })
  result.current.mutate({ oldPassword: 'bad', newPassword: 'newpass' })
  await waitFor(() => expect(result.current.isError).toBe(true))
})

it('isOnboardingCompleted treats an absent option as completed (Vue parity)', async () => {
  const { isOnboardingCompleted } = await import('./queries')
  const base = { id: 'u1', name: 'A', email: 'a@x.test', avatar: '', currency: 'USD', reportPeriod: 'monthly' }
  expect(isOnboardingCompleted(undefined)).toBe(false)
  expect(isOnboardingCompleted({ ...base, options: [] })).toBe(true)
  expect(isOnboardingCompleted({ ...base, options: [{ name: 'onboarding', value: 'completed' }] })).toBe(true)
  expect(isOnboardingCompleted({ ...base, options: [{ name: 'onboarding', value: '' }] })).toBe(false)
})
