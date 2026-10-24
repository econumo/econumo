import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { formatDateTime } from '@/lib/datetime'
import type { RecurringDto } from '@/api/dto/recurring'
import type { BudgetUpcomingDto } from '@/api/dto/budget'
import { ViewRecurringDialog } from './ViewRecurringDialog'

function mockMatchMedia() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

// a year out, so the "future" assumptions this fixture relies on never age into failures
const futurePaymentAt = formatDateTime(new Date(Date.now() + 365 * 24 * 3600 * 1000))

const fixtureRecurring: RecurringDto = {
  id: 'r1', ownerUserId: 'u1', type: 'expense', accountId: 'a1', accountRecipientId: null,
  amount: '50.5', categoryId: 'cat-food', payeeId: null, tagId: null, labelIds: [], description: 'rent',
  schedule: 'monthly', nextPaymentAt: futurePaymentAt,
}

function renderView(overrides: {
  onPost?: () => void
  canChange?: boolean
  recurring?: RecurringDto
} = {}) {
  server.use(...coreHandlers())
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const onClose = vi.fn()
  const onSkip = vi.fn()
  const onEdit = vi.fn()
  render(
    <QueryClientProvider client={queryClient}>
      <ViewRecurringDialog
        recurring={overrides.recurring ?? fixtureRecurring}
        onClose={onClose}
        onPost={overrides.onPost}
        onSkip={onSkip}
        onEdit={onEdit}
        canChange={overrides.canChange ?? true}
      />
    </QueryClientProvider>,
  )
  return { onClose, onSkip, onEdit }
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  mockMatchMedia()
})

it('settings context: skip only, no Post button', async () => {
  renderView({ onPost: undefined })
  expect(await screen.findByText('Recurring transaction')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Post' })).toBeNull()
  expect(screen.getByRole('button', { name: 'Skip' })).toBeInTheDocument()
})

it('account context: Post is the primary action', async () => {
  const onPost = vi.fn()
  renderView({ onPost })
  await userEvent.setup().click(await screen.findByRole('button', { name: 'Post' }))
  expect(onPost).toHaveBeenCalled()
})

it('disables mutating actions when canChange is false', async () => {
  renderView({ canChange: false })
  expect(await screen.findByRole('button', { name: 'Skip' })).toBeDisabled()
})

it('account context: the footer is hide | post | skip, in that order', async () => {
  renderView({ onPost: vi.fn() })
  const footer = (await screen.findByRole('button', { name: 'Post' })).parentElement!
  const labels = Array.from(footer.querySelectorAll('button')).map(
    (b) => b.getAttribute('aria-label') ?? b.textContent,
  )
  expect(labels).toEqual(['Cancel', 'Post', 'Skip'])
})

it('offers no delete action — templates are deleted from the settings list', async () => {
  renderView({ onPost: vi.fn() })
  await screen.findByRole('button', { name: 'Post' })
  expect(screen.queryByRole('button', { name: /Delete/i })).toBeNull()
})

it('the recurring row shows the period and leads to edit', async () => {
  const { onEdit } = renderView({ onPost: vi.fn() })
  const row = await screen.findByRole('button', { name: /Recurring transaction/i })
  expect(row).toHaveTextContent('Monthly')
  await userEvent.setup().click(row)
  expect(onEdit).toHaveBeenCalled()
})

it('drops the redundant Repeats and Next payment cards', async () => {
  // the schedule lives in the recurring row and the next-payment date sits under
  // the amount, so separate cards for both would just repeat them
  renderView({ onPost: vi.fn() })
  await screen.findByRole('button', { name: 'Post' })
  expect(screen.queryByText('Repeats')).toBeNull()
  expect(screen.queryByText('Next payment')).toBeNull()
})

it('colours the amount like a regular expense', async () => {
  renderView({ onPost: vi.fn() })
  await screen.findByRole('button', { name: 'Post' })
  // the hero amount is the one span carrying the type colour class, and it
  // renders with the expense sign exactly as the transaction preview does
  const amount = document.querySelector('.text-2xl.text-expense')
  expect(amount).not.toBeNull()
  expect(amount?.textContent).toContain('50.5')
  expect(amount?.textContent).toMatch(/^-/)
})

const partnerOccurrence: BudgetUpcomingDto = {
  id: 'r-partner', author: { id: 'u2', name: 'Partner', avatar: 'face:sky' }, currencyId: 'cur-usd', amount: '30',
  description: 'internet', category: { id: 'cat-bills', name: 'Bills', icon: 'receipt' }, payee: { id: 'p-isp', name: 'ISP' },
  tag: { id: 'tg-home', name: 'Home' }, labelIds: ['lb-partner'], spentAt: '2026-07-15 09:00:00',
  templateId: 'r-partner', schedule: 'weekly', accountId: 'acc-partner', accountName: 'Partner card',
  labelNames: ['household'], isNext: true, isOverdue: false, canEdit: false,
}

function renderReadOnly(overrides: { onEdit?: () => void; note?: string; occurrence?: BudgetUpcomingDto } = {}) {
  server.use(...coreHandlers())
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <ViewRecurringDialog readOnly occurrence={overrides.occurrence ?? partnerOccurrence} onClose={vi.fn()} onEdit={overrides.onEdit} note={overrides.note} />
    </QueryClientProvider>,
  )
}

it('read-only: shows the occurrence from its own names, with no Post or Skip', async () => {
  renderReadOnly()
  expect(await screen.findByText('Partner card')).toBeInTheDocument()
  expect(screen.getByText('Bills')).toBeInTheDocument()
  expect(screen.getByText('ISP')).toBeInTheDocument()
  expect(screen.getByText('Home')).toBeInTheDocument()
  expect(screen.getByText('household')).toBeInTheDocument()
  expect(screen.getByText('Weekly')).toBeInTheDocument()
  expect(screen.getByText('2026-07-15 09:00:00')).toBeInTheDocument()
  expect(document.querySelector('.text-2xl.text-expense')?.textContent).toContain('30')
  expect(screen.queryByRole('button', { name: 'Post' })).toBeNull()
  expect(screen.queryByRole('button', { name: 'Skip' })).toBeNull()
  expect(screen.queryByRole('button', { name: 'Edit' })).toBeNull()
})

it('read-only: offers Edit only when given, and shows the note', async () => {
  const onEdit = vi.fn()
  renderReadOnly({ onEdit, note: 'Can be posted after the earlier payment' })
  await userEvent.setup().click(await screen.findByRole('button', { name: 'Edit' }))
  expect(onEdit).toHaveBeenCalled()
  expect(screen.getByText('Can be posted after the earlier payment')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Post' })).toBeNull()
})

it('read-only transfer: shows the recipient account and its amount', async () => {
  renderReadOnly({
    occurrence: {
      ...partnerOccurrence, type: 'transfer', direction: 'in', category: null, payee: null, tag: null, labelNames: [],
      accountId: '', accountName: '', amount: '75',
      accountRecipientId: 'acc-pot', accountRecipientName: 'Holiday pot', amountRecipient: '75', currencyRecipientId: 'cur-usd',
    },
  })
  expect(await screen.findByText('Holiday pot')).toBeInTheDocument()
  expect(screen.getByText('Recipient')).toBeInTheDocument()
  expect(screen.getByText('[Hidden account]')).toBeInTheDocument()
})
