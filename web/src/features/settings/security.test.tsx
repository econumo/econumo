import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import {
  useCreatePersonalToken,
  usePersonalTokens,
  useRevokeOtherSessions,
  useRevokePersonalToken,
  useRevokeSession,
  useSessions,
} from './security'

const session = {
  id: '01890000-0000-7000-8000-0000000000s1',
  userAgent: 'Firefox on Linux',
  createdAt: '2026-07-01 10:00:00',
  lastUsedAt: '2026-07-10 09:00:00',
  isCurrent: true,
}

const pat = {
  id: '01890000-0000-7000-8000-0000000000p1',
  name: 'CI export',
  createdAt: '2026-07-01 10:00:00',
  lastUsedAt: '2026-07-10 09:00:00',
  expiresAt: null,
}

function makeWrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
  return { client, wrapper }
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('useSessions unwraps the sessions list', async () => {
  server.use(
    http.get('*/api/v1/user/get-session-list', () =>
      HttpResponse.json({ success: true, message: '', data: [session] }),
    ),
  )
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useSessions(), { wrapper })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(result.current.data).toEqual([session])
})

it('useRevokeSession posts the id and invalidates the sessions list', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/revoke-session', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
    http.get('*/api/v1/user/get-session-list', () =>
      HttpResponse.json({ success: true, message: '', data: [] }),
    ),
  )
  const { client, wrapper } = makeWrapper()
  const invalidate = vi.spyOn(client, 'invalidateQueries')
  const { result } = renderHook(() => useRevokeSession(), { wrapper })
  result.current.mutate(session.id)
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(body).toEqual({ id: session.id })
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ['sessions'] })
})

it('useRevokeOtherSessions posts without a body and invalidates', async () => {
  let called = false
  server.use(
    http.post('*/api/v1/user/revoke-other-sessions', () => {
      called = true
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const { client, wrapper } = makeWrapper()
  const invalidate = vi.spyOn(client, 'invalidateQueries')
  const { result } = renderHook(() => useRevokeOtherSessions(), { wrapper })
  result.current.mutate()
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(called).toBe(true)
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ['sessions'] })
})

it('usePersonalTokens unwraps the token list', async () => {
  server.use(
    http.get('*/api/v1/user/get-personal-token-list', () =>
      HttpResponse.json({ success: true, message: '', data: [pat] }),
    ),
  )
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => usePersonalTokens(), { wrapper })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(result.current.data).toEqual([pat])
})

it('useCreatePersonalToken sends name + expiresAt ("" for never) and returns the token once', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/create-personal-token', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({
        success: true,
        message: '',
        data: { ...pat, token: 'eco_pat_secret-value' },
      })
    }),
  )
  const { client, wrapper } = makeWrapper()
  const invalidate = vi.spyOn(client, 'invalidateQueries')
  const { result } = renderHook(() => useCreatePersonalToken(), { wrapper })
  result.current.mutate({ name: 'CI export', expiresAt: null })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(body).toEqual({ name: 'CI export', expiresAt: '' })
  expect(result.current.data?.token).toBe('eco_pat_secret-value')
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ['personalTokens'] })
})

it('useCreatePersonalToken passes an explicit expiry through', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/create-personal-token', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({
        success: true,
        message: '',
        data: { ...pat, token: 'eco_pat_x', expiresAt: '2030-01-01 00:00:00' },
      })
    }),
  )
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useCreatePersonalToken(), { wrapper })
  result.current.mutate({ name: 'Short lived', expiresAt: '2030-01-01 00:00:00' })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(body).toEqual({ name: 'Short lived', expiresAt: '2030-01-01 00:00:00' })
})

it('useRevokePersonalToken posts the id and invalidates the token list', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/revoke-personal-token', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const { client, wrapper } = makeWrapper()
  const invalidate = vi.spyOn(client, 'invalidateQueries')
  const { result } = renderHook(() => useRevokePersonalToken(), { wrapper })
  result.current.mutate(pat.id)
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(body).toEqual({ id: pat.id })
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ['personalTokens'] })
})
