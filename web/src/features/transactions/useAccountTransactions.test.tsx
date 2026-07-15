import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { coreHandlers, fixtureOwner, fixtureTransactions } from '@/test/fixtures'
import { useAccountTransactions, transactionTitleInfo } from './useAccountTransactions'
import type { ViewTransaction } from './useAccountTransactions'

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>{children}</QueryClientProvider>
)

function sharedWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
}

const t = (key: string, params?: Record<string, string>) => {
  if (key.endsWith('transfer_from')) return `Transfer from ${params?.account}`
  if (key.endsWith('transfer_to')) return `Transfer to ${params?.account}`
  if (key.endsWith('name_hidden')) return '[Hidden Account]'
  return key
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('filters by account (both legs), groups by day desc with labels', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  vi.setSystemTime(new Date(2026, 6, 2, 12, 0, 0))
  server.use(
    ...coreHandlers({
      transactions: [
        { id: 'tx-today', author: fixtureOwner, type: 'expense', accountId: 'a1', accountRecipientId: null, amount: '1', amountRecipient: '1', categoryId: 'cat-food', description: '', payeeId: null, tagId: null, date: '2026-07-02 09:00:00' },
        { id: 'tx-yesterday', author: fixtureOwner, type: 'income', accountId: 'a1', accountRecipientId: null, amount: '2', amountRecipient: '2', categoryId: 'cat-salary', description: '', payeeId: null, tagId: null, date: '2026-07-01 09:00:00' },
        { id: 'tx-incoming-transfer', author: fixtureOwner, type: 'transfer', accountId: 'a2', accountRecipientId: 'a1', amount: '3', amountRecipient: '3', categoryId: null, description: '', payeeId: null, tagId: null, date: '2026-06-20 09:00:00' },
        { id: 'tx-other-account', author: fixtureOwner, type: 'expense', accountId: 'a2', accountRecipientId: null, amount: '4', amountRecipient: '4', categoryId: 'cat-food', description: '', payeeId: null, tagId: null, date: '2026-07-02 10:00:00' },
      ],
    }),
  )
  const { result } = renderHook(() => useAccountTransactions('a1', ''), { wrapper })
  await waitFor(() => expect(result.current.length).toBeGreaterThan(0))

  const kinds = result.current.map((e) => (e.kind === 'separator' ? `sep:${e.label}` : e.transaction.id))
  expect(kinds).toEqual(['sep:today', 'tx-today', 'sep:yesterday', 'tx-yesterday', 'sep:date', 'tx-incoming-transfer'])
  vi.useRealTimers()
})

it('search matches category, payee, description and amount terms', async () => {
  server.use(...coreHandlers())
  const { result, rerender } = renderHook(({ search }) => useAccountTransactions('a1', search), {
    wrapper,
    initialProps: { search: '' },
  })
  await waitFor(() => expect(result.current.length).toBeGreaterThan(0))

  rerender({ search: 'coffee' })
  expect(result.current.filter((e) => e.kind === 'transaction').map((e) => (e.kind === 'transaction' ? e.transaction.id : ''))).toEqual(['t1'])

  rerender({ search: 'salary' })
  expect(result.current.filter((e) => e.kind === 'transaction').map((e) => (e.kind === 'transaction' ? e.transaction.id : ''))).toEqual(['t2'])

  rerender({ search: '9.99' })
  expect(result.current.filter((e) => e.kind === 'transaction')).toHaveLength(1)

  rerender({ search: 'nothing-matches' })
  expect(result.current).toHaveLength(0)
})

it('title logic: transfer direction, source priority and suppression source', () => {
  const base = { id: 't', author: fixtureOwner, amount: 1, amountRecipient: null, categoryId: null, description: '', payeeId: null, tagId: null, date: '2026-07-01 00:00:00', accountRecipientId: null, isInFuture: false }
  const acc = (id: string, name: string) => ({ id, name }) as ViewTransaction['account']

  const outgoing = { ...base, type: 'transfer', accountId: 'page', accountRecipientId: 'other', account: acc('page', 'Mine'), accountRecipient: acc('other', 'Theirs') } as unknown as ViewTransaction
  expect(transactionTitleInfo(outgoing, 'page', t)).toEqual({ text: 'Transfer to Theirs', source: 'transfer' })

  const incoming = { ...outgoing, accountId: 'other', accountRecipientId: 'page', account: acc('other', 'Theirs'), accountRecipient: acc('page', 'Mine') } as unknown as ViewTransaction
  expect(transactionTitleInfo(incoming, 'page', t)).toEqual({ text: 'Transfer from Theirs', source: 'transfer' })

  const hidden = { ...incoming, account: undefined } as unknown as ViewTransaction
  expect(transactionTitleInfo(hidden, 'page', t).text).toBe('Transfer from [Hidden Account]')

  const categorized = { ...base, type: 'expense', accountId: 'page', category: { name: 'Food' }, description: 'x', payee: { name: 'P' } } as unknown as ViewTransaction
  expect(transactionTitleInfo(categorized, 'page', t)).toEqual({ text: 'Food', source: 'category' })

  const descOnly = { ...base, type: 'expense', accountId: 'page', description: 'lunch' } as unknown as ViewTransaction
  expect(transactionTitleInfo(descOnly, 'page', t)).toEqual({ text: 'lunch', source: 'description' })
})

it('merges one virtual row per template at its next payment date', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  vi.setSystemTime(new Date(2026, 6, 2, 12, 0, 0))
  const rt = {
    id: 'r1', ownerUserId: 'u1', type: 'expense', accountId: 'a1', accountRecipientId: null,
    amount: 42.5, categoryId: 'cat-food', payeeId: null, tagId: null, description: 'rent',
    schedule: 'monthly', nextPaymentAt: '2026-07-05 00:00:00',
  }
  server.use(...coreHandlers({ recurring: [rt] }))
  const { result } = renderHook(() => useAccountTransactions('a1', ''), { wrapper })
  await waitFor(() => expect(result.current.some((e) => e.kind === 'transaction' && e.transaction.id === 'r1')).toBe(true))

  const kinds = result.current.map((e) => (e.kind === 'separator' ? `sep:${e.day}` : e.transaction.id))
  // 2026-07-05 is newer than every real row (today = 07-02), so the virtual
  // row and its separator sort first
  expect(kinds[0]).toBe('sep:2026-07-05')
  expect(kinds[1]).toBe('r1')

  const virtualEntry = result.current.find((e) => e.kind === 'transaction' && e.transaction.id === 'r1')
  expect(virtualEntry?.kind === 'transaction' && virtualEntry.transaction.recurring).toEqual(rt)
  vi.useRealTimers()
})

it('virtual transfer rows appear only on the source account', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  vi.setSystemTime(new Date(2026, 6, 2, 12, 0, 0))
  const rt = {
    id: 'r2', ownerUserId: 'u1', type: 'transfer', accountId: 'a1', accountRecipientId: 'a2',
    amount: 20, categoryId: null, payeeId: null, tagId: null, description: '',
    schedule: 'monthly', nextPaymentAt: '2026-07-10 00:00:00',
  }
  const txOnA2 = {
    id: 'tx-a2', author: fixtureOwner, type: 'expense', accountId: 'a2', accountRecipientId: null,
    amount: '5', amountRecipient: '5', categoryId: 'cat-food', description: '', payeeId: null, tagId: null,
    date: '2026-07-02 08:00:00',
  }
  server.use(...coreHandlers({ recurring: [rt], transactions: [...fixtureTransactions, txOnA2] }))
  const shared = sharedWrapper()

  const { result: resultA1 } = renderHook(() => useAccountTransactions('a1', ''), { wrapper: shared })
  await waitFor(() => expect(resultA1.current.some((e) => e.kind === 'transaction' && e.transaction.id === 'r2')).toBe(true))

  const { result: resultA2 } = renderHook(() => useAccountTransactions('a2', ''), { wrapper: shared })
  await waitFor(() => expect(resultA2.current.some((e) => e.kind === 'transaction' && e.transaction.id === 'tx-a2')).toBe(true))
  expect(resultA2.current.some((e) => e.kind === 'transaction' && e.transaction.id === 'r2')).toBe(false)
  vi.useRealTimers()
})

it('overdue templates surface at their past date', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  vi.setSystemTime(new Date(2026, 6, 2, 12, 0, 0))
  const rt = {
    id: 'r3', ownerUserId: 'u1', type: 'expense', accountId: 'a1', accountRecipientId: null,
    amount: 12, categoryId: 'cat-food', payeeId: null, tagId: null, description: 'overdue rent',
    schedule: 'monthly', nextPaymentAt: '2026-06-15 00:00:00',
  }
  server.use(...coreHandlers({ recurring: [rt] }))
  const { result } = renderHook(() => useAccountTransactions('a1', ''), { wrapper })
  await waitFor(() => expect(result.current.some((e) => e.kind === 'transaction' && e.transaction.id === 'r3')).toBe(true))

  const entries = result.current
  const idx = entries.findIndex((e) => e.kind === 'transaction' && e.transaction.id === 'r3')
  expect(entries[idx - 1]).toEqual({ kind: 'separator', day: '2026-06-15', label: 'date' })
  vi.useRealTimers()
})
