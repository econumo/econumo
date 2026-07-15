import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { formatDateTime } from '@/lib/datetime'
import type { RecurringDto } from '@/api/dto/recurring'
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
  amount: 50.5, categoryId: 'cat-food', payeeId: null, tagId: null, description: 'rent',
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
  const onDelete = vi.fn()
  render(
    <QueryClientProvider client={queryClient}>
      <ViewRecurringDialog
        recurring={overrides.recurring ?? fixtureRecurring}
        onClose={onClose}
        onPost={overrides.onPost}
        onSkip={onSkip}
        onEdit={onEdit}
        onDelete={onDelete}
        canChange={overrides.canChange ?? true}
      />
    </QueryClientProvider>,
  )
  return { onClose, onSkip, onEdit, onDelete }
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  mockMatchMedia()
})

it('settings context: skip/edit/delete, no Post button', async () => {
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
