import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { usePendingInvites } from './pendingInvites'

const owner = { id: 'u2', avatar: 'pets:sky', name: 'Partner' }

const pendingAccount = {
  id: 'a-pending', owner, folderId: null, name: 'Shared cash', position: 0,
  currency: { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 },
  balance: '0', type: 1, icon: 'wallet',
  sharedAccess: [{ user: { id: 'u1', avatar: 'face:emerald', name: 'Ada' }, role: 'user', isAccepted: 0 }],
}

const pendingBudget = {
  id: 'b-pending', ownerUserId: 'u2', name: 'Shared budget', startedAt: '2026-01-01 00:00:00', currencyId: 'cur-usd',
  access: [
    { user: owner, role: 'owner', isAccepted: 1 },
    { user: { id: 'u1', avatar: 'face:emerald', name: 'Ada' }, role: 'admin', isAccepted: 0 },
  ],
}

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

it('collects pending account and budget invites for the current user', async () => {
  server.use(...coreHandlers({ accounts: [pendingAccount], budgets: [pendingBudget] }))
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => usePendingInvites(), { wrapper })

  await waitFor(() => expect(result.current.count).toBe(2))
  const accountInvite = result.current.invites.find((i) => i.kind === 'account')
  const budgetInvite = result.current.invites.find((i) => i.kind === 'budget')
  expect(accountInvite).toEqual({ kind: 'account', id: 'a-pending', name: 'Shared cash', owner, role: 'user' })
  expect(budgetInvite).toEqual({ kind: 'budget', id: 'b-pending', name: 'Shared budget', owner, role: 'admin' })
})

it('reports zero when there are no pending invites', async () => {
  server.use(...coreHandlers())
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => usePendingInvites(), { wrapper })

  await waitFor(() => expect(result.current.count).toBe(0))
  expect(result.current.invites).toEqual([])
})
