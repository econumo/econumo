import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { coreHandlers, fixtureConnections } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import type { ConnectionDto } from '@/api/dto/connection'
import { useAcceptInvite, useDeleteConnection } from './queries'

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
