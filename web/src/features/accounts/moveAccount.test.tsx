import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { delay, http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { queryKeys } from '@/app/queryKeys'
import type { AccountDto } from '@/api/dto/account'
import { useMoveAccount } from './queries'

function makeWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  return { queryClient, wrapper }
}

const owner = { id: 'u1', avatar: '', name: 'Ada' }
const usd = { id: 'usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const account = (id: string, position: number): AccountDto => ({
  id, owner, folderId: null, name: id, position, currency: usd, balance: '0', type: 1, icon: 'wallet', sharedAccess: [],
})

// The RAW accounts cache holds get-account-list's response, which is REVERSED
// (highest position first) -- useAccounts sorts a derived view, never the
// cache. The optimistic move must not assume the cache is in position order:
// applyMove re-stamps positions by array index, so applied to the reversed
// array it flips the entire list until the server echo lands.
it('optimistic account move keeps visual order on the reversed cache', async () => {
  server.use(
    http.post('*/api/v1/account/move-account', async () => {
      await delay('infinite')
      return HttpResponse.json({ success: true, message: '', data: { items: [] } })
    }),
  )
  const { queryClient, wrapper } = makeWrapper()
  // Visual order A(0), B(1), C(2) -- stored reversed, as the server sends it.
  queryClient.setQueryData(queryKeys.accounts, [account('C', 2), account('B', 1), account('A', 0)])

  const { result } = renderHook(() => useMoveAccount(), { wrapper })
  result.current.mutate({ id: 'C', afterId: 'A', folderId: null })

  await waitFor(() => {
    const cached = queryClient.getQueryData<AccountDto[]>(queryKeys.accounts)!
    const visual = [...cached].sort((a, b) => a.position - b.position).map((a) => a.id)
    expect(visual).toEqual(['A', 'C', 'B'])
  })
})
