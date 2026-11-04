import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { coerceBudgetFixture } from '@/test/coerceBudget'
import { fixtureDebtRow, fixtureWireBudget } from '@/test/fixtures'
import type { BudgetDebtElementDto, BudgetDto } from '@/api/dto/budget'
import { bucketElements, makeBudgetExchange } from './budgetMath'
import { useBudgetPeriodStore } from './budgetStore'
import { PhoneMonthView } from './PhoneMonthView'
import type { PhoneMonthViewProps } from './PhoneMonthView'

const usd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const eur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 }

function renderView(debts: BudgetDebtElementDto[], adjust?: (b: BudgetDto) => void) {
  const budget = coerceBudgetFixture(fixtureWireBudget)
  budget.structure.debts = debts
  adjust?.(budget)
  const props: PhoneMonthViewProps = {
    budget,
    buckets: bucketElements(budget, makeBudgetExchange(budget, [usd, eur])),
    currencies: [usd, eur],
    selectedDate: '2026-07-01',
    planMonth: null,
    commentsByCell: new Map(),
    onOpenSheet: vi.fn(),
    onShowTransactions: vi.fn(),
  }
  render(<PhoneMonthView {...props} />)
  return props
}

beforeEach(() => {
  localStorage.clear()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: {}, foldBudgetId: null, planFolds: {}, budgetFolds: {}, planUnfoldedElements: {} })
})

it('a Debts card sums Planned · Paid and unfolds to its rows', async () => {
  const props = renderView([fixtureDebtRow(), fixtureDebtRow({ id: 'debt-2', name: 'Car', position: 1, budgeted: '', isBudgeted: 0, spent: '100', budgetSpent: '100' })])
  const card = screen.getByTestId('phone-debts-card')
  const summary = within(card).getByTestId('phone-debts-summary')
  expect(summary).toHaveTextContent(/Debts\s*1,500\.00\s*1,000\.00/)
  expect(within(card).queryByTestId('phone-debts-row-debt-1')).not.toBeInTheDocument()
  await userEvent.click(summary)
  const row = within(card).getByTestId('phone-debts-row-debt-1')
  expect(row).toHaveTextContent(/Mortgage\s*1,500\.00\s*900\.00/)
  // an empty plan reads as a dash
  expect(within(card).getByTestId('phone-debts-row-debt-2')).toHaveTextContent(/Car\s*—\s*100\.00/)
  await userEvent.click(within(row).getByRole('button', { name: 'Mortgage, planned 1,500.00, paid 900.00' }))
  expect(props.onOpenSheet).toHaveBeenCalledWith({ kind: 'debt', row: fixtureDebtRow() })
})

it('expense totals include debt rows', () => {
  renderView([fixtureDebtRow()])
  // Food spent 45.50, the debt row paid 900
  expect(screen.getByTestId('phone-total-expenses')).toHaveTextContent('945.50')
  // Food 200 + Living 90 EUR (100 USD) + the debt's 1,500
  expect(screen.getByTestId('phone-total-budget')).toHaveTextContent('1,800.00')
})
