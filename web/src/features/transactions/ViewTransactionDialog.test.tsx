import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers, fixtureOwner } from '@/test/fixtures'
import type { ViewTransaction } from './useAccountTransactions'
import { ViewTransactionDialog } from './ViewTransactionDialog'

function mockMatchMedia() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

const fixtureTransaction = {
  id: 't1',
  type: 'expense',
  accountId: 'a1',
  accountRecipientId: null,
  amount: '50.5',
  amountRecipient: '50.5',
  categoryId: 'cat1',
  description: 'test transaction',
  payeeId: null,
  tagId: null,
  date: '2026-07-15 10:00:00',
  account: { id: 'a1', name: 'Checking', icon: 'account', currency: { code: 'USD', fraction: 2 }, ownerUserId: 'u1', folder: null, folderName: '', sharedAccess: [], isArchived: 0 },
  accountRecipient: undefined,
  category: { id: 'cat1', name: 'Food', icon: 'fastfood', ownerUserId: 'u1', type: 'expense', isArchived: 0, position: 0, description: '' },
  payee: undefined,
  tag: undefined,
  author: undefined,
  recurring: undefined,
  isInFuture: false,
} as unknown as ViewTransaction

const categorylessAccount = { id: 'a1', owner: fixtureOwner, folderId: null, name: 'Cash', position: 0, currency: { id: 'usd', code: 'USD', symbol: '$', fractionDigits: 2 }, balance: '0', type: 1, icon: 'wallet', sharedAccess: [] } as unknown as ViewTransaction['account']

const categorylessTx = {
  id: 't1',
  author: fixtureOwner,
  type: 'expense',
  accountId: 'a1',
  accountRecipientId: null,
  amount: '9.99',
  amountRecipient: null,
  categoryId: null,
  description: '',
  payeeId: null,
  tagId: null,
  date: '2026-07-01 10:00:00',
  account: categorylessAccount,
  isInFuture: false,
} as unknown as ViewTransaction

function renderView(overrides: {
  transaction?: ViewTransaction
  onMakeRecurring?: () => void
  canChange?: boolean
} = {}) {
  server.use(...coreHandlers())
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const onClose = vi.fn()
  const onEdit = vi.fn()
  const onDelete = vi.fn()
  const onMakeRecurring = overrides.onMakeRecurring
  render(
    <QueryClientProvider client={queryClient}>
      <ViewTransactionDialog
        transaction={overrides.transaction ?? fixtureTransaction}
        onClose={onClose}
        onEdit={onEdit}
        onDelete={onDelete}
        onMakeRecurring={onMakeRecurring}
        canChange={overrides.canChange ?? true}
        isShared={false}
      />
    </QueryClientProvider>,
  )
  return { onClose, onEdit, onDelete, onMakeRecurring }
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  mockMatchMedia()
})

it('renders Make recurring button when onMakeRecurring is provided', async () => {
  renderView({ onMakeRecurring: vi.fn() })
  expect(await screen.findByRole('button', { name: /Make recurring/i })).toBeInTheDocument()
})

it('fires onMakeRecurring callback when button is clicked', async () => {
  const onMakeRecurring = vi.fn()
  renderView({ onMakeRecurring })
  await userEvent.setup().click(await screen.findByRole('button', { name: /Make recurring/i }))
  expect(onMakeRecurring).toHaveBeenCalled()
})

it('does not render Make recurring button when onMakeRecurring is not provided', async () => {
  renderView({ onMakeRecurring: undefined })
  expect(await screen.findByRole('button', { name: 'Edit' })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /Make recurring/i })).toBeNull()
})

it('disables Make recurring button when canChange is false', async () => {
  const onMakeRecurring = vi.fn()
  renderView({ onMakeRecurring, canChange: false })
  const button = await screen.findByRole('button', { name: /Make recurring/i })
  expect(button).toBeDisabled()
})

it('a categoryless non-transfer transaction shows Uncategorized as the hero name', async () => {
  renderView({ transaction: categorylessTx })
  expect(await screen.findByText('Uncategorized')).toBeInTheDocument()
})
