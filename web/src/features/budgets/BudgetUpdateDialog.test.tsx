import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers, fixtureOwner, fixtureWireBudget } from '@/test/fixtures'
import { coerceBudgetFixture } from '@/test/coerceBudget'
import type { BudgetDto } from '@/api/dto/budget'
import { BudgetUpdateDialog } from './BudgetUpdateDialog'

function renderDialog(mutate?: (budget: BudgetDto) => void) {
  const budget = coerceBudgetFixture(fixtureWireBudget)
  mutate?.(budget)
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}>
      <BudgetUpdateDialog open budget={budget} onClose={() => {}} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  server.use(...coreHandlers())
})

it('owner can edit: account switches and the update button are enabled', async () => {
  renderDialog()
  expect(await screen.findByLabelText('include Cash')).toBeEnabled()
  expect(screen.getByRole('button', { name: 'Update' })).toBeEnabled()
})

it('guest gets a read-only dialog: account switches and the update button are disabled', async () => {
  renderDialog((budget) => {
    budget.meta.ownerUserId = 'u9'
    budget.meta.access = [
      { user: { id: 'u9', avatar: 'face:sky', name: 'Owner' }, role: 'owner', isAccepted: 1 },
      { user: fixtureOwner, role: 'guest', isAccepted: 1 },
    ]
  })
  expect(await screen.findByLabelText('include Cash')).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Update' })).toBeDisabled()
})
