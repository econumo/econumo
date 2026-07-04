import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers, fixtureWireBudget } from '@/test/fixtures'
import { coerceBudgetFixture } from '@/test/coerceBudget'
import { BudgetTable } from './BudgetTable'
import { bucketElements, makeBudgetExchange } from './budgetMath'
import { useBudgetPeriodStore } from './budgetStore'

const usd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const eur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 }

function renderTable() {
  const budget = coerceBudgetFixture(fixtureWireBudget)
  const buckets = bucketElements(budget, makeBudgetExchange(budget, [usd, eur]))
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <BudgetTable budget={budget} buckets={buckets} />
    </QueryClientProvider>,
  )
  return budget
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: {}, foldBudgetId: null })
})

it('renders folder, default and archived sections with rows and stat lines', async () => {
  renderTable()
  const essentials = await screen.findByTestId('budget-folder-Essentials')
  expect(within(essentials).getByText('Food')).toBeInTheDocument()
  expect(within(essentials).getByTestId('stat-line')).toHaveTextContent('Budget 200')
  const noFolder = screen.getByTestId('budget-folder-Default folder')
  expect(within(noFolder).getByText('Living')).toBeInTheDocument()
  const archive = screen.getByTestId('budget-folder-Archived')
  expect(within(archive).getByText('zzz-archived')).toBeInTheDocument()
})

it('shows -spent and available+budgeted with sign colors', async () => {
  renderTable()
  const food = await screen.findByTestId('element-cat-food')
  await waitFor(() => expect(within(food).getByTestId('cell-spent')).toHaveTextContent('45.50'))
  expect(within(food).getByTestId('cell-available')).toHaveTextContent('354.50')
  expect(within(food).getByTestId('cell-available').className).toContain('text-green-600')
})

it('fold toggle expands children and persists in the store', async () => {
  const user = userEvent.setup()
  renderTable()
  const living = await screen.findByTestId('element-env-1')
  expect(screen.queryByTestId('child-cat-rent')).not.toBeInTheDocument()
  await user.click(within(living).getByText('Living'))
  expect(await screen.findByTestId('child-cat-rent')).toBeInTheDocument()
  expect(useBudgetPeriodStore.getState().unfoldedElements['env-1']).toBe(true)
})

it('totals row sums all buckets in the budget currency', async () => {
  renderTable()
  const totals = await screen.findByTestId('budget-totals')
  expect(totals).toHaveTextContent('Total')
  await waitFor(() => expect(totals).toHaveTextContent('300.00'))
  expect(totals).toHaveTextContent('45.50')
  expect(totals).toHaveTextContent('554.50')
})
