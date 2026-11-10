import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { onlineManager, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { server } from '@/test/msw'
import { coreHandlers, fixtureTransactions, fixtureWireBudget } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { useUiStore } from '@/app/uiStore'
import { METRICS, trackEvent } from '@/lib/metrics'
import { addChange } from '@/features/pendingWrites/queue'
import { getOps, loadPendingWrites, mutate, resetPendingWritesForTests } from '@/features/pendingWrites/store'
import type { OpChange } from '@/features/pendingWrites/types'
import { TransactionDialog } from '@/features/transactions/TransactionDialog'
import { InboxPage } from './InboxPage'

vi.mock('@/hooks/useIsCompact', () => ({ useIsCompact: () => false }))
vi.mock('@/lib/metrics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/metrics')>()),
  trackEvent: vi.fn(),
}))

const payload = (over = {}) => ({
  id: 't1', type: 'expense' as const, accountId: 'a1', accountRecipientId: null, amount: '77', amountRecipient: null,
  categoryId: 'cat-food', description: 'coffee beans', payeeId: 'p1', tagId: null, labelIds: [], date: '2026-07-02 09:30:00', ...over,
})

async function seedFailed(change: OpChange, error = 'Category is archived') {
  await mutate((ops) => {
    const added = addChange(ops, change, { key: `k-${ops.length + 1}`, seq: ops.length + 1 })
    return added.map((op) => ({ ...op, status: 'failed' as const, error }))
  })
}

function renderInbox() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, networkMode: 'always' }, mutations: { retry: false, networkMode: 'always' } } })
  const router = createMemoryRouter([{ path: '/inbox', element: <><InboxPage /><TransactionDialog /></> }], { initialEntries: ['/inbox'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return queryClient
}

beforeEach(async () => {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({ matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn() }))
  localStorage.clear()
  window.econumoConfig = {}
  vi.mocked(trackEvent).mockClear()
  useUiStore.setState({ transactionModal: null })
  server.use(...coreHandlers())
  resetPendingWritesForTests()
  onlineManager.setOnline(false)
  await loadPendingWrites('u1')
})

afterEach(() => {
  onlineManager.setOnline(true)
})

it('lists a failed edit first, with the server reason, and opens the dialog with the edited values', async () => {
  await seedFailed({ kind: 'tx-update', txId: 't1', payload: payload() })
  renderInbox()
  expect(await screen.findByText('Category is archived')).toBeInTheDocument()
  expect(screen.getByRole('heading', { level: 2, name: 'Not saved' })).toBeInTheDocument()
  const user = userEvent.setup()
  await user.click(await screen.findByText('coffee beans'))
  await waitFor(() => expect(useUiStore.getState().transactionModal?.transaction?.amount).toBe('77'))
})

it('saving the dialog on a failed edit makes it waiting again', async () => {
  await seedFailed({ kind: 'tx-update', txId: 't1', payload: payload() })
  renderInbox()
  const user = userEvent.setup()
  await user.click(await screen.findByText('coffee beans'))
  await screen.findByRole('heading', { name: 'Edit transaction' })
  await user.click(await screen.findByRole('button', { name: 'Update' }))
  await waitFor(() => expect(getOps()[0]?.status).toBe('waiting'))
  expect(getOps()).toHaveLength(1)
})

it('a failed delete names the transaction, retries and discards', async () => {
  await seedFailed({ kind: 'tx-delete', txId: 't1', previous: { ...fixtureTransactions[0], labelIds: [], recurringId: null, isImported: 0 } as never })
  renderInbox()
  expect(await screen.findByText('coffee beans')).toBeInTheDocument()
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Try again' }))
  expect(trackEvent).toHaveBeenCalledWith(METRICS.WRITE_RETRIED, { kind: 'tx-delete' })
  await waitFor(() => expect(getOps()[0]?.status).toBe('waiting'))
})

it('a failed delete with no stored row shows a generic label and can be discarded', async () => {
  await seedFailed({ kind: 'tx-delete', txId: 'gone', previous: null })
  renderInbox()
  expect(await screen.findByText('Deleted transaction')).toBeInTheDocument()
  await userEvent.setup().click(screen.getByRole('button', { name: 'Discard' }))
  expect(trackEvent).toHaveBeenCalledWith(METRICS.WRITE_DISCARDED, { kind: 'tx-delete' })
  await waitFor(() => expect(getOps()).toHaveLength(0))
  expect(screen.queryByText('Deleted transaction')).toBeNull()
})

it('a failed limit names the element, amount and month, and falls back without a cached budget', async () => {
  await seedFailed({ kind: 'limit', budgetId: 'b1', elementId: 'cat-food', period: '2026-07-01', amount: '250' })
  const queryClient = renderInbox()
  expect(await screen.findByText('Plan: 250 (July 2026)')).toBeInTheDocument()
  await act(async () => {
    queryClient.setQueryData([...queryKeys.budget, 'b1', '2026-07-01'], fixtureWireBudget)
  })
  const name = fixtureWireBudget.structure.elements.find((e) => e.id === 'cat-food')!.name
  expect(await screen.findByText(`Plan for ${name}: 250 (July 2026)`)).toBeInTheDocument()
  await userEvent.setup().click(screen.getByRole('button', { name: 'Try again' }))
  expect(trackEvent).toHaveBeenCalledWith(METRICS.WRITE_RETRIED, { kind: 'limit' })
})
