import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { queryKeys } from '@/app/queryKeys'
import { useCreateAccount, useFolders } from './queries'

const wireOwner = { id: 'u1', avatar: '', name: 'Ada' }
const wireAccount = {
  id: 'a-real', owner: wireOwner, folderId: 'f1', name: 'Cash', position: 0,
  currency: { id: 'c1', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 },
  balance: '100.5', type: 1, icon: 'wallet', sharedAccess: [],
}
const wireCorrection = {
  id: 't-corr', author: wireOwner, type: 'income', accountId: 'a-real', accountRecipientId: null,
  amount: '100.5', amountRecipient: '100.5', categoryId: null, description: '', payeeId: null, tagId: null,
  date: '2026-07-03 12:00:00',
}

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

it('create-account upserts the item and inserts the opening-balance transaction', async () => {
  server.use(
    http.post('*/api/v1/account/create-account', () =>
      HttpResponse.json({ success: true, message: '', data: { item: wireAccount, transaction: wireCorrection } }),
    ),
  )
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData(queryKeys.accounts, [])
  queryClient.setQueryData(queryKeys.transactions, [])
  queryClient.setQueryData(queryKeys.folders, [{ id: 'f1', name: 'General', position: 0, isVisible: 1 }])

  const { result } = renderHook(() => useCreateAccount(), { wrapper })
  result.current.mutate({ id: 'op1', name: 'Cash', currencyId: 'c1', balance: '100.5', icon: 'wallet', folderId: 'f1' })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))

  expect(queryClient.getQueryData<{ id: string }[]>(queryKeys.accounts)!.map((a) => a.id)).toEqual(['a-real'])
  expect(queryClient.getQueryData<{ id: string }[]>(queryKeys.transactions)!.map((t) => t.id)).toEqual(['t-corr'])
})

it('create-account with an empty folders cache refetches folders (first-account auto-folder)', async () => {
  let folderListHits = 0
  server.use(
    http.post('*/api/v1/account/create-account', () =>
      HttpResponse.json({ success: true, message: '', data: { item: wireAccount, transaction: null } }),
    ),
    http.get('*/api/v1/account/get-folder-list', () => {
      folderListHits++
      return HttpResponse.json({ success: true, message: '', data: { items: [{ id: 'f1', name: 'General', position: 0, isVisible: 1 }] } })
    }),
  )
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData(queryKeys.folders, [])
  // an active useFolders observer, so invalidation triggers a refetch
  const { result } = renderHook(() => ({ create: useCreateAccount(), folders: useFolders() }), { wrapper })
  result.current.create.mutate({ id: 'op1', name: 'Cash', currencyId: 'c1', balance: '0', icon: 'wallet', folderId: null })
  await waitFor(() => expect(result.current.create.isSuccess).toBe(true))
  await waitFor(() => expect(folderListHits).toBeGreaterThan(0))
})
