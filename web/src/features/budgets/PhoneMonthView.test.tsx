import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { coerceBudgetFixture } from '@/test/coerceBudget'
import { fixtureWireBudget } from '@/test/fixtures'
import type { BudgetCommentDto, BudgetDto, PlanElementDto } from '@/api/dto/budget'
import { bucketElements, makeBudgetExchange } from './budgetMath'
import { useBudgetPeriodStore } from './budgetStore'
import type { PlanMonthFigures } from './phoneMonth'
import { PhoneMonthView } from './PhoneMonthView'
import type { PhoneMonthViewProps } from './PhoneMonthView'

const usd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const eur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 }

const salaries = { id: 'ie1', type: 4, name: 'Salaries', icon: 'payments', currencyId: 'cur-usd', isArchived: 0, folderId: null, position: 3, ownerUserId: null, cells: [], children: [] } as PlanElementDto
const planMonth: PlanMonthFigures = {
  month: '2026-07-01',
  index: 2,
  income: { rows: [{ element: salaries, planned: '2000', actual: '400' }], planned: '2000', received: '400' },
  balance: '4545',
  savingsBalance: null,
  transfersNet: '0',
}

function renderView(overrides: Partial<PhoneMonthViewProps> = {}, mutate?: (b: BudgetDto) => void) {
  const budget = coerceBudgetFixture(fixtureWireBudget)
  mutate?.(budget)
  const props: PhoneMonthViewProps = {
    budget,
    buckets: bucketElements(budget, makeBudgetExchange(budget, [usd, eur])),
    currencies: [usd, eur],
    selectedDate: '2026-07-01',
    planMonth,
    commentsByCell: new Map(),
    onOpenSheet: vi.fn(),
    onShowTransactions: vi.fn(),
    ...overrides,
  }
  render(<PhoneMonthView {...props} />)
  return props
}

beforeEach(() => {
  localStorage.clear()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: {}, foldBudgetId: null })
})

it('names the budget currency once in the heading row, above Budget and Spent', () => {
  renderView()
  const heading = screen.getByTestId('phone-heading')
  expect(heading).toHaveTextContent('USD')
  expect(heading).toHaveTextContent('Budget')
  expect(heading).toHaveTextContent('Spent')
  expect(heading).not.toHaveTextContent('Available')
})

it('shows each expense row as one button with Budget and Spent, no symbols', async () => {
  const props = renderView()
  const food = screen.getByRole('button', { name: 'Food, budget 200.00, spent 45.50' })
  expect(within(screen.getByTestId('phone-row-cat-food')).queryByText(/\$/)).toBeNull()
  await userEvent.click(food)
  expect(props.onOpenSheet).toHaveBeenCalledWith({ kind: 'expense', element: expect.objectContaining({ id: 'cat-food' }) })
})

it('tags a foreign-currency element with its code and keeps its amounts unconverted', () => {
  renderView()
  const living = screen.getByTestId('phone-row-env-1')
  expect(within(living).getByTestId('phone-currency-tag')).toHaveTextContent('EUR')
  expect(within(living).getByText('90.00')).toBeInTheDocument()
  expect(within(screen.getByTestId('phone-row-cat-food')).queryByTestId('phone-currency-tag')).toBeNull()
})

it('draws the progress bar and colours Spent by row state', () => {
  renderView({}, (b) => {
    const food = b.structure.elements.find((el) => el.id === 'cat-food')!
    Object.assign(food, { budgeted: '100', spent: '150', budgetSpent: '150', available: '-120' })
  })
  const row = screen.getByTestId('phone-row-cat-food')
  expect(within(row).getByTestId('phone-progress').firstElementChild).toHaveStyle({ width: '100%' })
  expect(within(row).getByText('150.00').className).toContain('text-expense')
})

it('amber when carry-over covers the overspend', () => {
  renderView({}, (b) => {
    const food = b.structure.elements.find((el) => el.id === 'cat-food')!
    Object.assign(food, { budgeted: '100', spent: '150', budgetSpent: '150', available: '10' })
  })
  expect(within(screen.getByTestId('phone-row-cat-food')).getByText('150.00').className).toContain('text-amber-600')
})

it('a future month shows a dash for Spent, no bar, and no colour', () => {
  renderView({ selectedDate: '2099-01-01' }, (b) => {
    const food = b.structure.elements.find((el) => el.id === 'cat-food')!
    Object.assign(food, { budgeted: '100', spent: '150', budgetSpent: '150', available: '-120' })
  })
  const row = screen.getByTestId('phone-row-cat-food')
  expect(within(row).queryByTestId('phone-progress')).toBeNull()
  expect(screen.getByRole('button', { name: 'Food, budget 100.00, spent —' })).toBeInTheDocument()
})

it('marks a commented row with a non-interactive indicator', () => {
  const comment = { id: 'c1', elementId: 'cat-food', period: '2026-07-01', comment: 'x', author: { id: 'u1', avatar: 'face:emerald', name: 'Ada' }, createdAt: '2026-07-01 09:00:00', updatedAt: '2026-07-01 09:00:00' } as BudgetCommentDto
  renderView({ commentsByCell: new Map([['cat-food|2026-07-01', [comment]]]) })
  const indicator = within(screen.getByTestId('phone-row-cat-food')).getByTestId('phone-comment-indicator')
  expect(indicator).toHaveAttribute('aria-hidden', 'true')
  expect(within(screen.getByTestId('phone-row-env-1')).queryByTestId('phone-comment-indicator')).toBeNull()
})

it('folders show their Budget and Spent sums; the unfoldered bucket reads "No folder"', () => {
  renderView()
  const essentials = screen.getByTestId('phone-folder-bf1')
  expect(essentials).toHaveTextContent('Essentials')
  expect(essentials).toHaveTextContent('200.00')
  expect(screen.getByTestId('phone-folder-__no_folder__')).toHaveTextContent('No folder')
})

it('the chevron unfolds children, the row opens the sheet, and a child opens its transactions', async () => {
  const props = renderView()
  const living = screen.getByTestId('phone-row-env-1')
  await userEvent.click(within(living).getByRole('button', { name: 'Expand' }))
  expect(props.onOpenSheet).not.toHaveBeenCalled()
  await userEvent.click(screen.getByTestId('phone-child-cat-rent'))
  expect(props.onShowTransactions).toHaveBeenCalledWith(expect.objectContaining({ id: 'cat-rent', parent: { id: 'env-1', type: 0 } }))
})

it('collapses income into one summary row that unfolds into income rows', async () => {
  const props = renderView()
  const summary = screen.getByTestId('phone-income-summary')
  expect(summary).toHaveTextContent('Income · 400.00 of 2,000.00')
  expect(summary).toHaveAttribute('aria-expanded', 'false')
  expect(screen.queryByTestId('phone-income-row-ie1')).toBeNull()
  await userEvent.click(summary)
  await userEvent.click(screen.getByRole('button', { name: 'Salaries, planned 2,000.00, received 400.00' }))
  expect(props.onOpenSheet).toHaveBeenCalledWith({ kind: 'plan', cell: planMonth.income.rows[0] })
})

it('leaves income and the plan lines out while the plan is not loaded', () => {
  renderView({ planMonth: null })
  expect(screen.queryByTestId('phone-income')).toBeNull()
  expect(screen.queryByTestId('phone-total-balance')).toBeNull()
  expect(screen.getByTestId('phone-total-expenses')).toHaveTextContent('45.50 of 300.00')
})

it('the totals card lists Expenses, Available and Balance; Transfers only when non-zero', () => {
  renderView()
  expect(screen.getByTestId('phone-total-available')).toBeInTheDocument()
  expect(screen.getByTestId('phone-total-balance')).toHaveTextContent('4,545.00')
  expect(screen.queryByTestId('phone-total-transfers')).toBeNull()
  expect(screen.queryByTestId('phone-total-savings')).toBeNull()
  expect(screen.queryByTestId('phone-total-savings-balance')).toBeNull()
})

it('shows Transfers when money crossed the budget boundary', () => {
  renderView({ planMonth: { ...planMonth, transfersNet: '-100' } })
  expect(screen.getByTestId('phone-total-transfers')).toHaveTextContent('-100.00')
})

it('lists savings rows with Planned and Saved, and the totals card adds the savings lines', async () => {
  const props = renderView({ planMonth: { ...planMonth, savingsBalance: '1040' } }, (b) => {
    b.structure.savings = [
      { id: 'acc-s1', type: 5, name: 'Rainy day', icon: 'savings', currencyId: 'cur-usd', ownerUserId: 'u1', isArchived: 0, position: 0, budgeted: '100', spent: '40', available: '60', closingBalance: '1040' },
    ]
  })
  const savings = screen.getByTestId('phone-savings')
  expect(savings).toHaveTextContent('Planned')
  expect(savings).toHaveTextContent('Saved')
  await userEvent.click(screen.getByRole('button', { name: 'Rainy day, planned 100.00, saved 40.00' }))
  expect(props.onOpenSheet).toHaveBeenCalledWith({ kind: 'savings', row: expect.objectContaining({ id: 'acc-s1' }) })
  expect(screen.getByTestId('phone-total-savings')).toHaveTextContent('40.00 of 100.00')
  expect(screen.getByTestId('phone-total-savings-balance')).toHaveTextContent('1,040.00')
})

it('shows the uncategorized row without a budget, and it opens the sheet', async () => {
  const props = renderView({}, (b) => {
    b.structure.elements.push({
      id: 'uncategorized', type: 1, name: 'Uncategorized', icon: 'question_mark', currencyId: null, isArchived: 0, folderId: null,
      position: 99, budgeted: '0', available: '0', spent: '12', budgetSpent: '12', ownerUserId: null, children: [],
    })
  })
  const row = screen.getByTestId('phone-row-uncategorized')
  expect(row).toHaveTextContent('—')
  await userEvent.click(within(row).getByRole('button'))
  expect(props.onOpenSheet).toHaveBeenCalledWith({ kind: 'expense', element: expect.objectContaining({ id: 'uncategorized' }) })
})
