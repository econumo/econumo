import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureOwner, fixtureWireBudget } from '@/test/fixtures'
import { coerceBudgetFixture } from '@/test/coerceBudget'
import { BudgetElementType } from '@/api/dto/budget'
import { useUiStore } from '@/app/uiStore'
import { BudgetTransactionsDialog } from './BudgetTransactionsDialog'
import { useBudgetPeriodStore } from './budgetStore'

const target = { id: 'cat-food', type: BudgetElementType.CATEGORY, name: 'Food', icon: 'restaurant', currencyId: null }

const wireItems = [
  {
    // same id as fixtureTransactions t1 -> previewable
    id: 't1', author: fixtureOwner, currencyId: 'cur-usd', amount: '9.99', description: 'coffee beans',
    category: { id: 'cat-food', name: 'Food', icon: 'restaurant' }, payee: null, tag: null,
    spentAt: '2026-07-02 09:30:00',
  },
  {
    // a partner's transaction the user cannot see in the plain list -> read-only row
    id: 'tx-foreign', author: fixtureOwner, currencyId: 'cur-usd', amount: '5', description: 'partner spend',
    category: { id: 'cat-food', name: 'Food', icon: 'restaurant' }, payee: null, tag: null,
    spentAt: '2026-07-02 10:00:00',
  },
]

function renderDialog() {
  const budget = coerceBudgetFixture(fixtureWireBudget)
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}>
      <BudgetTransactionsDialog budget={budget} element={target} onClose={() => {}} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: {}, foldBudgetId: null })
  useUiStore.setState({ transactionModal: null })
  server.use(
    ...coreHandlers(),
    http.get('*/api/v1/budget/get-transaction-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: wireItems } }),
    ),
  )
})

it('every row opens the preview; a foreign transaction shows disabled Edit/Delete', async () => {
  const user = userEvent.setup()
  renderDialog()
  const own = await screen.findByTestId('budget-tx-t1')
  expect(own.tagName).toBe('BUTTON')
  expect(own).toHaveTextContent('coffee beans')
  const foreign = screen.getByTestId('budget-tx-tx-foreign')
  expect(foreign.tagName).toBe('BUTTON')
  await user.click(foreign)
  const edit = await screen.findByRole('button', { name: 'Edit' })
  expect(edit).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Delete' })).toBeDisabled()
  // synthesized preview still carries the wire details (list row + preview card)
  expect(screen.getAllByText('partner spend').length).toBeGreaterThan(1)
})

it('clicking a transaction opens the preview; Edit hands off to the transaction modal', async () => {
  const user = userEvent.setup()
  renderDialog()
  await user.click(await screen.findByTestId('budget-tx-t1'))
  // preview hero shows the category and the editable footer
  const edit = await screen.findByRole('button', { name: 'Edit' })
  await user.click(edit)
  expect(useUiStore.getState().transactionModal?.transaction?.id).toBe('t1')
})

it('delete flows through the confirm dialog', async () => {
  let deletedId: unknown
  server.use(
    http.post('*/api/v1/transaction/delete-transaction', async ({ request }) => {
      deletedId = ((await request.json()) as { id: string }).id
      return HttpResponse.json({ success: true, message: '', data: { item: wireItems[0], accounts: [] } })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  await user.click(await screen.findByTestId('budget-tx-t1'))
  await user.click(await screen.findByRole('button', { name: 'Delete' }))
  // the confirm dialog takes over
  await user.click(await screen.findByRole('button', { name: 'Delete' }))
  await vi.waitFor(() => expect(deletedId).toBe('t1'))
})
