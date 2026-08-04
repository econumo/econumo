import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { queryKeys } from '@/app/queryKeys'
import type { LabelDto } from '@/api/dto/label'
import type { TagDto } from '@/api/dto/tag'
import type { RecurringDto } from '@/api/dto/recurring'
import type { TransactionDto } from '@/api/dto/transaction'
import { useCreateLabel, useDeleteLabel, useLabels } from './queries'

function makeWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  return { queryClient, wrapper }
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('useLabels returns the labels from the API', async () => {
  server.use(
    http.get('*/api/v1/label/get-label-list', () =>
      HttpResponse.json({
        success: true,
        message: '',
        data: {
          items: [
            {
              id: 'l1', ownerUserId: 'u1', name: 'Kid A', icon: 'label', position: 0,
              isArchived: 0, createdAt: '2026-08-01 10:00:00', updatedAt: '2026-08-01 10:00:00',
            },
          ],
        },
      }),
    ),
  )
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useLabels(), { wrapper })
  await waitFor(() => expect(result.current.data).toHaveLength(1))
  expect(result.current.data?.[0].icon).toBe('label')
})

// Tags and labels have INDEPENDENT name namespaces on the backend, so the
// create-dedupe must not resolve a label to a same-named tag.
it('useCreateLabel does not dedupe against an existing tag of the same name', async () => {
  let apiCalled = false
  server.use(
    http.post('*/api/v1/label/create-label', () => {
      apiCalled = true
      return HttpResponse.json({
        success: true,
        message: '',
        data: {
          item: {
            id: 'l-new', ownerUserId: 'u1', name: 'Travel', icon: 'label', position: 0,
            isArchived: 0, createdAt: '2026-08-01 10:00:00', updatedAt: '2026-08-01 10:00:00',
          },
        },
      })
    }),
  )
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData<TagDto[]>(queryKeys.tags, [
    {
      id: 't1', ownerUserId: 'u1', name: 'Travel', icon: 'tag', position: 0,
      isArchived: 0, createdAt: '2026-08-01 10:00:00', updatedAt: '2026-08-01 10:00:00',
    },
  ])
  queryClient.setQueryData<LabelDto[]>(queryKeys.labels, [])

  const { result } = renderHook(() => useCreateLabel(), { wrapper })
  result.current.mutate({ name: 'Travel', ownerUserId: 'u1' })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))

  expect(apiCalled).toBe(true)
  expect(result.current.data?.id).toBe('l-new')
})


it('deleting a label strips its id from cached transactions and templates', async () => {
  server.use(http.post('*/api/v1/label/delete-label', () => HttpResponse.json({ success: true, message: '', data: {} })))
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData<LabelDto[]>(queryKeys.labels, [
    { id: 'l1', ownerUserId: 'u1', name: 'gone', icon: 'label', position: 0, isArchived: 0, createdAt: '', updatedAt: '' },
  ])
  queryClient.setQueryData(queryKeys.transactions, [
    { id: 't1', labelIds: ['l1', 'l2'] },
    { id: 't2', labelIds: ['l2'] },
  ])
  queryClient.setQueryData(queryKeys.recurring, [{ id: 'r1', labelIds: ['l1'] }])

  const { result } = renderHook(() => useDeleteLabel(), { wrapper })
  result.current.mutate('l1')
  await waitFor(() => expect(result.current.isSuccess).toBe(true))

  expect(queryClient.getQueryData<LabelDto[]>(queryKeys.labels)).toEqual([])
  // the server cascades the link rows away; a stale id left in the cache would
  // be resent by the next edit (a full replace) and rejected as unavailable
  const transactions = queryClient.getQueryData<TransactionDto[]>(queryKeys.transactions)!
  expect(transactions.map((tx) => tx.labelIds)).toEqual([['l2'], ['l2']])
  expect(queryClient.getQueryData<RecurringDto[]>(queryKeys.recurring)![0].labelIds).toEqual([])
})
