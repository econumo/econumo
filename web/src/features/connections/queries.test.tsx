import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts, fixtureConnections } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import type { AccountDto } from '@/api/dto/account'
import type { ConnectionDto } from '@/api/dto/connection'
import { useAcceptInvite, useDeleteConnection, useRevokeAccountAccess, useSetAccountAccess } from './queries'

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
  server.use(...coreHandlers())
})

it('useAcceptInvite replaces the connections cache with the returned items', async () => {
  server.use(
    http.post('*/api/v1/connection/accept-invite', () =>
      HttpResponse.json({ success: true, message: '', data: { items: fixtureConnections } }),
    ),
  )
  const { queryClient, wrapper } = makeWrapper()
  const { result } = renderHook(() => useAcceptInvite(), { wrapper })
  result.current.mutate('aB3f9')
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(queryClient.getQueryData(queryKeys.connections)).toEqual(fixtureConnections)
})

it('useSetAccountAccess rewrites the accounts cache with the grant', async () => {
  server.use(
    http.post('*/api/v1/connection/set-account-access', () =>
      HttpResponse.json({ success: true, message: '', data: {} }),
    ),
  )
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData(queryKeys.accounts, fixtureAccounts)
  queryClient.setQueryData(queryKeys.connections, fixtureConnections)
  const { result } = renderHook(() => useSetAccountAccess(), { wrapper })
  result.current.mutate({ accountId: 'a1', userId: 'u2', role: 'user' })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  const accounts = queryClient.getQueryData<AccountDto[]>(queryKeys.accounts)!
  expect(accounts.find((a) => a.id === 'a1')!.sharedAccess).toEqual([{ user: fixtureConnections[0].user, role: 'user' }])
})

it('useRevokeAccountAccess drops the grant from the accounts cache', async () => {
  server.use(
    http.post('*/api/v1/connection/revoke-account-access', () =>
      HttpResponse.json({ success: true, message: '', data: {} }),
    ),
  )
  const { queryClient, wrapper } = makeWrapper()
  const shared = fixtureAccounts.map((a) =>
    a.id === 'a1' ? { ...a, sharedAccess: [{ user: fixtureConnections[0].user, role: 'user' }] } : a,
  )
  queryClient.setQueryData(queryKeys.accounts, shared)
  const { result } = renderHook(() => useRevokeAccountAccess(), { wrapper })
  result.current.mutate({ accountId: 'a1', userId: 'u2' })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  const accounts = queryClient.getQueryData<AccountDto[]>(queryKeys.accounts)!
  expect(accounts.find((a) => a.id === 'a1')!.sharedAccess).toEqual([])
})

it('useDeleteConnection removes the user from the connections cache', async () => {
  server.use(
    http.post('*/api/v1/connection/delete-connection', () =>
      HttpResponse.json({ success: true, message: '', data: {} }),
    ),
  )
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData(queryKeys.connections, fixtureConnections)
  const { result } = renderHook(() => useDeleteConnection(), { wrapper })
  result.current.mutate('u2')
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(queryClient.getQueryData<ConnectionDto[]>(queryKeys.connections)).toEqual([])
})
