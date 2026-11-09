import type { ReactNode } from 'react'
import { http, HttpResponse } from 'msw'
import { QueryClientProvider, onlineManager, type QueryClient } from '@tanstack/react-query'
import { renderHook } from '@testing-library/react'
import { server } from '@/test/msw'
import { queryKeys } from '@/app/queryKeys'
import type { AccountDto } from '@/api/dto/account'
import type { CreateTransactionDto, TransactionDto } from '@/api/dto/transaction'
import { useDeleteTransaction, useMoveTransaction, useUpdateTransaction } from '@/features/transactions/queries'
import { createAppQueryClient } from '@/lib/appQueryClient'
import { removeToken, setToken } from '@/lib/storage'
import { useUnsentCount, usePendingOpForTransaction } from './hooks'
import { addChange } from './queue'
import { kickPendingWrites, startPendingWrites } from './sender'
import { getOps, loadPendingWrites, mutate } from './store'

vi.mock('@/app/routerRef', () => ({ navigateTo: vi.fn() }))

const me = { id: 'u1', name: 'Ann', avatar: 'face:fuchsia' }
const payload = (id: string, over: Partial<CreateTransactionDto> = {}): CreateTransactionDto => ({
  id, type: 'expense', accountId: 'a1', accountRecipientId: null, amount: '5', amountRecipient: null,
  categoryId: null, description: '', payeeId: null, tagId: null, labelIds: [], date: '2026-11-08 10:00:00', ...over,
})
const row = (id: string): TransactionDto => ({ ...payload(id), author: me, recurringId: null, isImported: 0 })
const accounts = [{ id: 'a1', transactionsRevision: 2 } as AccountDto]

type Seen = { path: string; body: Record<string, unknown> }
let seen: Seen[]
let client: QueryClient
let stop: () => void

const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>

// A dialog opened on the temporary row keeps that row while the add confirms.
async function landAdd(tempId: string): Promise<void> {
  await mutate((ops) => addChange(ops, { kind: 'tx-create', txId: tempId, payload: payload(tempId) }, { key: tempId, seq: 1 }))
  await kickPendingWrites()
  expect(getOps()).toEqual([])
  seen = []
}

beforeEach(async () => {
  seen = []
  setToken('tok', 'u1')
  onlineManager.setOnline(true)
  client = createAppQueryClient()
  client.setQueryData(queryKeys.accounts, [])
  client.setQueryData(queryKeys.transactions, [])
  await loadPendingWrites('u1')
  stop = startPendingWrites(client)
  server.use(
    http.post('*/api/v1/transaction/:action', async ({ request, params }) => {
      const r: Seen = { path: String(params.action), body: (await request.json()) as Record<string, unknown> }
      seen.push(r)
      if (r.path === 'move-transaction') return HttpResponse.json({ success: true, message: '', data: { items: [], accounts } })
      return HttpResponse.json({ success: true, message: '', data: { item: row(r.path === 'create-transaction' ? 'real' : String(r.body.id)), accounts } })
    }),
  )
})

afterEach(() => {
  stop()
  removeToken()
  onlineManager.setOnline(true)
})

it('an edit from a dialog opened before the add landed updates the real row', async () => {
  await landAdd('temp-edit')
  const { result } = renderHook(() => useUpdateTransaction(), { wrapper })
  await result.current.mutateAsync(payload('temp-edit', { amount: '9' }))
  expect(seen).toMatchObject([{ path: 'update-transaction', body: { id: 'real', amount: '9' } }])
})

it('a delete from a dialog opened before the add landed deletes the real row', async () => {
  await landAdd('temp-delete')
  const { result } = renderHook(() => useDeleteTransaction(), { wrapper })
  await result.current.mutateAsync('temp-delete')
  expect(seen).toMatchObject([{ path: 'delete-transaction', body: { id: 'real' } }])
})

it('a move names the real rows', async () => {
  await landAdd('temp-move')
  const { result } = renderHook(() => useMoveTransaction(), { wrapper })
  await result.current.mutateAsync({ id: 'temp-move', accountId: 'a1', fromDate: '2026-11-08', date: '2026-11-08', aboveId: 'temp-move' })
  expect(seen).toMatchObject([{ path: 'move-transaction', body: { id: 'real', aboveId: 'real' } }])
})

it('an edit queued offline after the add landed is queued against the real row', async () => {
  await landAdd('temp-offline')
  onlineManager.setOnline(false)
  const { result } = renderHook(() => ({ update: useUpdateTransaction(), op: usePendingOpForTransaction('temp-offline'), unsent: useUnsentCount() }), { wrapper })
  await result.current.update.mutateAsync(payload('temp-offline', { amount: '9' }))
  expect(getOps()).toMatchObject([{ kind: 'tx-update', txId: 'real', payload: { id: 'real' } }])
  await vi.waitFor(() => expect(result.current.op).toMatchObject({ kind: 'tx-update', txId: 'real' }))
  await mutate(() => [])
})

it('a row still waiting keeps its temporary id', async () => {
  onlineManager.setOnline(false)
  await mutate((ops) => addChange(ops, { kind: 'tx-create', txId: 'temp-waiting', payload: payload('temp-waiting') }, { key: 'temp-waiting', seq: 1 }))
  const { result } = renderHook(() => useUpdateTransaction(), { wrapper })
  await result.current.mutateAsync(payload('temp-waiting', { amount: '9' }))
  expect(getOps()).toMatchObject([{ kind: 'tx-create', txId: 'temp-waiting', payload: { amount: '9' } }])
  await mutate(() => [])
})
