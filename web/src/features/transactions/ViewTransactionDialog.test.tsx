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
  recurringSchedule?: 'monthly' | 'weekly'
  onOpenRecurring?: () => void
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
        recurringSchedule={overrides.recurringSchedule}
        onOpenRecurring={overrides.onOpenRecurring}
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

it('hides Make recurring for a transaction posted from a recurring template', async () => {
  // a posted instance already has a schedule behind it, so offering the action
  // again would invite duplicate templates
  renderView({
    onMakeRecurring: vi.fn(),
    transaction: { ...fixtureTransaction, recurringId: 'r1' } as ViewTransaction,
  })
  expect(await screen.findByRole('button', { name: 'Edit' })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /Make recurring/i })).toBeNull()
})

it('shows Make recurring for a hand-entered transaction (recurringId null)', async () => {
  renderView({
    onMakeRecurring: vi.fn(),
    transaction: { ...fixtureTransaction, recurringId: null } as ViewTransaction,
  })
  expect(await screen.findByRole('button', { name: /Make recurring/i })).toBeInTheDocument()
})

it('shows the schedule row for a posted transaction and opens the template on click', async () => {
  const onOpenRecurring = vi.fn()
  renderView({
    transaction: { ...fixtureTransaction, recurringId: 'r1' } as ViewTransaction,
    recurringSchedule: 'monthly',
    onOpenRecurring,
  })
  const row = await screen.findByRole('button', { name: /Recurring transaction/i })
  expect(row).toHaveTextContent('Monthly')
  await userEvent.setup().click(row)
  expect(onOpenRecurring).toHaveBeenCalled()
})

it('marks a posted transaction with a non-interactive recurring indicator', async () => {
  renderView({
    transaction: { ...fixtureTransaction, recurringId: 'r1' } as ViewTransaction,
    recurringSchedule: 'monthly',
    onOpenRecurring: vi.fn(),
  })
  // present as a labelled graphic...
  const indicators = await screen.findAllByLabelText('Recurring transaction')
  expect(indicators.length).toBeGreaterThan(0)
  // ...but never as a pressable control, unlike the hand-entered case
  expect(screen.queryByRole('button', { name: /Make recurring/i })).toBeNull()
  const heroIndicator = indicators.find((el) => el.closest('button') === null)
  expect(heroIndicator).toBeDefined()
})

it('shows the hero indicator even when the template is gone', async () => {
  // the transaction still came from a schedule, so the glyph stays; only the
  // clickable schedule row (which needs a resolvable template) disappears
  renderView({
    transaction: { ...fixtureTransaction, recurringId: 'r1' } as ViewTransaction,
    recurringSchedule: undefined,
    onOpenRecurring: undefined,
  })
  const indicators = await screen.findAllByLabelText('Recurring transaction')
  expect(indicators.some((el) => el.closest('button') === null)).toBe(true)
})

it('shows no recurring indicator on a hand-entered transaction', async () => {
  renderView({ transaction: { ...fixtureTransaction, recurringId: null } as ViewTransaction })
  expect(screen.queryByLabelText('Recurring transaction')).toBeNull()
})

it('shows no schedule row when the template could not be resolved', async () => {
  // deleted template, or one on an account the caller cannot see: the row would
  // be a dead end, so it is omitted entirely
  renderView({
    transaction: { ...fixtureTransaction, recurringId: 'r1' } as ViewTransaction,
    recurringSchedule: undefined,
    onOpenRecurring: undefined,
  })
  expect(await screen.findByRole('button', { name: 'Edit' })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /Recurring transaction/i })).toBeNull()
})

it('a categoryless non-transfer transaction shows Uncategorized as the hero name', async () => {
  renderView({ transaction: categorylessTx })
  expect(await screen.findByText('Uncategorized')).toBeInTheDocument()
})

it('shows a labels card with the resolved label name and heading when the transaction has labels', async () => {
  renderView({ transaction: { ...fixtureTransaction, labelIds: ['label1'] } as ViewTransaction })
  expect(await screen.findByText('health')).toBeInTheDocument()
  expect(screen.getByText('Reporting tag')).toBeInTheDocument()
  // the fixture's icon differs from DEFAULT_ICON.label on purpose, so this goes red
  // if the card ever renders the kind default instead of the row's stored icon
  expect(screen.getByText('sell')).toBeInTheDocument()
})

it('renders no labels card when the transaction has no labels', async () => {
  renderView({ transaction: { ...fixtureTransaction, labelIds: [] } as ViewTransaction })
  await screen.findByRole('button', { name: 'Edit' })
  expect(screen.queryByText('health')).toBeNull()
  expect(screen.queryByText('Label')).toBeNull()
})
