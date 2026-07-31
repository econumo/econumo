import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { queryKeys } from '@/app/queryKeys'
import { useAcceptAccountAccess, useAccounts, useCreateAccount, useDeclineAccountAccess, useFolders } from './queries'

const wireOwner = { id: 'u1', avatar: '', name: 'Ada' }
const wireUser = { id: 'u1', name: 'Ada', email: 'ada@example.test', avatar: 'face:emerald', options: [] }
const wireAccount = {
  id: 'a-real', owner: wireOwner, folderId: 'f1', name: 'Cash', position: 0,
  currency: { id: 'c1', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 },
  balance: '100.5', type: 1, icon: 'wallet', sharedAccess: [],
}
const wirePartner = { id: 'u2', avatar: '', name: 'Partner' }
function pendingAccount(isAccepted: 0 | 1) {
  return {
    id: 'a-pending', owner: wirePartner, folderId: null, name: 'Shared wallet', position: 1,
    currency: { id: 'c1', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 },
    balance: '0', type: 1, icon: 'wallet',
    sharedAccess: [{ user: wireOwner, role: 'user', isAccepted }],
  }
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

it('accept-access invalidates the transactions cache (pending accounts have their transactions hidden)', async () => {
  server.use(
    http.post('*/api/v1/account/accept-access', () =>
      HttpResponse.json({ success: true, message: '', data: {} }),
    ),
  )
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData(queryKeys.transactions, [])

  const { result } = renderHook(() => useAcceptAccountAccess(), { wrapper })
  result.current.mutate({ accountId: 'a-pending', folderId: 'f1' })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))

  expect(queryClient.getQueryState(queryKeys.transactions)?.isInvalidated).toBe(true)
})

it('decline-access immediately drops the pending account from the cache', async () => {
  server.use(
    http.post('*/api/v1/account/decline-access', () =>
      HttpResponse.json({ success: true, message: '', data: {} }),
    ),
  )
  const { queryClient, wrapper } = makeWrapper()
  queryClient.setQueryData(queryKeys.accounts, [wireAccount, pendingAccount(0)])

  const { result } = renderHook(() => useDeclineAccountAccess(), { wrapper })
  result.current.mutate('a-pending')
  await waitFor(() => expect(result.current.isSuccess).toBe(true))

  expect(queryClient.getQueryData<{ id: string }[]>(queryKeys.accounts)!.map((a) => a.id)).toEqual(['a-real'])
})

it('useAccounts hides an account pending my acceptance, and shows it once accepted', async () => {
  server.use(
    http.get('*/api/v1/user/get-user-data', () => HttpResponse.json({ success: true, message: '', data: { user: wireUser } })),
    http.get('*/api/v1/account/get-account-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [wireAccount, pendingAccount(0)] } }),
    ),
  )
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useAccounts(), { wrapper })
  await waitFor(() => expect(result.current.isSuccess).toBe(true))
  expect(result.current.data?.map((a) => a.id)).toEqual(['a-real'])

  server.use(
    http.get('*/api/v1/account/get-account-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [wireAccount, pendingAccount(1)] } }),
    ),
  )
  await result.current.refetch()
  await waitFor(() => expect(result.current.data?.map((a) => a.id)).toEqual(['a-real', 'a-pending']))
})
