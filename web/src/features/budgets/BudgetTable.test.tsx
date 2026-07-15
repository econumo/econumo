import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers, fixtureWireBudget } from '@/test/fixtures'
import { coerceBudgetFixture } from '@/test/coerceBudget'
import { BudgetTable } from './BudgetTable'
import type { ElementRowExtras } from './BudgetTable'
import { bucketElements, makeBudgetExchange } from './budgetMath'
import { useBudgetPeriodStore } from './budgetStore'
import type { BudgetDto } from '@/api/dto/budget'

const usd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2, scope: 'global' as const, isArchived: 0 as const, isHidden: 0 as const }
const eur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2, scope: 'global' as const, isArchived: 0 as const, isHidden: 0 as const }

function renderTable(mutate?: (budget: BudgetDto) => void, extras: ElementRowExtras & { hideChildren?: boolean } = {}) {
  const budget = coerceBudgetFixture(fixtureWireBudget)
  mutate?.(budget)
  const buckets = bucketElements(budget, makeBudgetExchange(budget, [usd, eur]))
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <BudgetTable budget={budget} buckets={buckets} {...extras} />
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

it('renders column headers, folder, default and archived sections with aligned stat cells', async () => {
  renderTable()
  const headers = screen.getByTestId('column-headers')
  expect(headers).toHaveTextContent('Budget')
  expect(headers).toHaveTextContent('Spent')
  expect(headers).toHaveTextContent('Available')
  const essentials = await screen.findByTestId('budget-folder-Essentials')
  expect(within(essentials).getByText('Food')).toBeInTheDocument()
  await waitFor(() => expect(within(essentials).getByTestId('stat-line')).toHaveTextContent('200.00'))
  expect(within(essentials).getByTestId('stat-line')).toHaveTextContent('45.50')
  expect(within(essentials).getByTestId('stat-line')).toHaveTextContent('354.50')
  const noFolder = screen.getByTestId('budget-folder-Default folder')
  expect(within(noFolder).getByText('Living')).toBeInTheDocument()
  const archive = screen.getByTestId('budget-folder-Archived')
  expect(within(archive).getByText('zzz-archived')).toBeInTheDocument()
})

it('shows -spent and available+budgeted as a sign-colored pill', async () => {
  renderTable()
  const food = await screen.findByTestId('element-cat-food')
  await waitFor(() => expect(within(food).getByTestId('cell-spent')).toHaveTextContent('45.50'))
  expect(within(food).getByTestId('cell-available')).toHaveTextContent('354.50')
  expect(within(food).getByTestId('cell-available').className).toContain('text-income')
  expect(within(food).getByTestId('cell-available').className).toContain('rounded-full')
})

it('rounds float noise in cells to the currency precision', async () => {
  renderTable((budget) => {
    const food = budget.structure.elements[0]
    food.spent = -45.4999999934
    food.budgetSpent = -45.4999999934
  })
  const food = await screen.findByTestId('element-cat-food')
  await waitFor(() => expect(within(food).getByTestId('cell-spent')).toHaveTextContent('45.50'))
  expect(within(food).getByTestId('cell-spent')).not.toHaveTextContent('45.4999')
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

it('hideContents renders sections header-only (folder drag in progress)', async () => {
  renderTable(undefined, { hideContents: true } as never)
  const essentials = await screen.findByTestId('budget-folder-Essentials')
  expect(within(essentials).queryByText('Food')).not.toBeInTheDocument()
  expect(within(essentials).getByText('Essentials')).toBeInTheDocument()
})

it('hideChildren renders unfolded elements collapsed (element drag in progress)', async () => {
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: { 'env-1': true }, foldBudgetId: null })
  renderTable(undefined, { hideChildren: true })
  await screen.findByTestId('element-env-1')
  expect(screen.queryByTestId('child-cat-rent')).not.toBeInTheDocument()
})

it('clicking the name of a childless element does nothing', async () => {
  const user = userEvent.setup()
  const onSpentClick = vi.fn()
  renderTable(undefined, { onSpentClick })
  const food = await screen.findByTestId('element-cat-food')
  await user.click(within(food).getByText('Food'))
  expect(onSpentClick).not.toHaveBeenCalled()
  expect(useBudgetPeriodStore.getState().unfoldedElements['cat-food']).toBeUndefined()
})

it('clicking the spent amount reports the element as a transactions target', async () => {
  const user = userEvent.setup()
  const onSpentClick = vi.fn()
  renderTable(undefined, { onSpentClick })
  const food = await screen.findByTestId('element-cat-food')
  await user.click(within(food).getByRole('button', { name: 'transactions Food' }))
  expect(onSpentClick).toHaveBeenCalledWith({ id: 'cat-food', type: 1, name: 'Food', icon: 'restaurant', currencyId: null })
})

it('clicking a child spent amount reports the child category with the parent currency', async () => {
  const user = userEvent.setup()
  const onSpentClick = vi.fn()
  renderTable(undefined, { onSpentClick })
  const living = await screen.findByTestId('element-env-1')
  await user.click(within(living).getByText('Living'))
  await user.click(await screen.findByRole('button', { name: 'transactions Rent' }))
  expect(onSpentClick).toHaveBeenCalledWith({ id: 'cat-rent', type: 1, name: 'Rent', icon: 'house', currencyId: 'cur-eur' })
})

it('children show spent and the owner badge only in a multi-user budget', async () => {
  const user = userEvent.setup()
  renderTable((budget) => {
    budget.meta.access.push({ user: { id: 'u2', avatar: 'pets:sky', name: 'Partner' }, role: 'user', isAccepted: 1 })
    budget.structure.elements[1].children[0].spent = -12.5
    budget.structure.elements[1].children[0].ownerUserId = 'u2'
  })
  const living = await screen.findByTestId('element-env-1')
  await user.click(within(living).getByText('Living'))
  const child = await screen.findByTestId('child-cat-rent')
  await waitFor(() => expect(within(child).getByTestId('child-spent')).toHaveTextContent('12.50'))
  // the owner name is rendered (revealed on row hover via CSS)
  expect(within(child).getByText('Partner')).toBeInTheDocument()
})

it('children hide the owner badge in a single-user budget', async () => {
  const user = userEvent.setup()
  renderTable()
  const living = await screen.findByTestId('element-env-1')
  await user.click(within(living).getByText('Living'))
  const child = await screen.findByTestId('child-cat-rent')
  expect(within(child).queryByText('Ada')).not.toBeInTheDocument()
})

it('tapping the available pill reports the element when onAvailableClick is wired (compact set-limit path)', async () => {
  const user = userEvent.setup()
  const onAvailableClick = vi.fn()
  renderTable(undefined, { onAvailableClick })
  const food = await screen.findByTestId('element-cat-food')
  await user.click(within(food).getByRole('button', { name: 'limit Food' }))
  expect(onAvailableClick).toHaveBeenCalledWith(expect.objectContaining({ id: 'cat-food' }))
})

it('the available pill is not a button without onAvailableClick', async () => {
  renderTable()
  const food = await screen.findByTestId('element-cat-food')
  expect(within(food).queryByRole('button', { name: 'limit Food' })).not.toBeInTheDocument()
})

it('totals row sums all buckets in the budget currency', async () => {
  renderTable()
  const totals = await screen.findByTestId('budget-totals')
  expect(totals).toHaveTextContent('Total')
  await waitFor(() => expect(totals).toHaveTextContent('300.00'))
  expect(totals).toHaveTextContent('45.50')
  expect(totals).toHaveTextContent('554.50')
})

it('phone totals unfold into labeled budget/spent/available lines', async () => {
  renderTable()
  const totals = await screen.findByTestId('budget-totals-mobile')
  expect(totals).toHaveTextContent('Total')
  await waitFor(() => expect(totals).toHaveTextContent('300.00'))
  expect(totals).toHaveTextContent('Budget')
  expect(totals).toHaveTextContent('Spent')
  expect(totals).toHaveTextContent('Available')
  expect(totals).toHaveTextContent('45.50')
  expect(totals).toHaveTextContent('554.50')
})
