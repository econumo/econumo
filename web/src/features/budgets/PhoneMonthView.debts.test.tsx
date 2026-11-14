import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { coerceBudgetFixture } from '@/test/coerceBudget'
import { fixtureElementDebt, fixtureElementDebtNotSetUp, fixtureWireBudget } from '@/test/fixtures'
import type { BudgetDto, BudgetElementDto } from '@/api/dto/budget'
import { useUiStore } from '@/app/uiStore'
import { bucketElements, makeBudgetExchange } from './budgetMath'
import { useBudgetPeriodStore } from './budgetStore'
import { PhoneMonthView } from './PhoneMonthView'
import type { PhoneMonthViewProps } from './PhoneMonthView'

const usd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const eur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 }

const food = (b: BudgetDto) => b.structure.elements.find((el) => el.id === 'cat-food') as BudgetElementDto

function renderView(adjust: (b: BudgetDto) => void, canPlanDebts = true) {
  const budget = coerceBudgetFixture(fixtureWireBudget)
  adjust(budget)
  const props: PhoneMonthViewProps = {
    budget,
    buckets: bucketElements(budget, makeBudgetExchange(budget, [usd, eur])),
    currencies: [usd, eur],
    selectedDate: '2026-07-01',
    dataPeriod: '2026-07-01',
    planMonth: null,
    commentsByCell: new Map(),
    onOpenSheet: vi.fn(),
    onShowTransactions: vi.fn(),
    canPlanDebts,
  }
  render(<PhoneMonthView {...props} />)
  return props
}

beforeEach(() => {
  localStorage.clear()
  useUiStore.setState({ debtDetailsId: null, debtDetailsTab: 'overview' })
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: {}, foldBudgetId: null, planFolds: {}, budgetFolds: {}, planUnfoldedElements: {} })
})

it('no Debts card: the category row shows its debt under its own figures', async () => {
  const props = renderView((b) => {
    food(b).debt = fixtureElementDebt()
  })
  expect(screen.queryByTestId('phone-debts-card')).toBeNull()
  const row = screen.getByTestId('phone-row-cat-food')
  expect(row).toHaveTextContent(/Food.*45\.50/)
  expect(within(row).getByTestId('element-debt-line')).toHaveTextContent('24 payments left · Paid off by Oct 2028')
  // the figures still open the item sheet
  await userEvent.click(within(row).getByRole('button', { name: /^Food, budget .*, spent 45\.50$/ }))
  expect(props.onOpenSheet).toHaveBeenCalledWith({ kind: 'expense', element: expect.objectContaining({ id: 'cat-food' }) })
})

it('a debt not set up offers Finish setup to whoever plans, and nothing to anyone else', async () => {
  renderView((b) => {
    food(b).debt = fixtureElementDebtNotSetUp()
  })
  await userEvent.click(within(screen.getByTestId('phone-row-cat-food')).getByRole('button', { name: 'Finish setup' }))
  expect(useUiStore.getState().debtDetailsTab).toBe('settings')
})

it('a guest sees no line on a debt not set up', () => {
  renderView((b) => {
    food(b).debt = fixtureElementDebtNotSetUp()
  }, false)
  expect(within(screen.getByTestId('phone-row-cat-food')).queryByTestId('element-debt-line')).toBeNull()
})

it('the expense totals are the elements\' own', () => {
  renderView((b) => {
    food(b).debt = fixtureElementDebt()
  })
  // Food spent 45.50; the debt adds nothing
  expect(screen.getByTestId('phone-total-expenses')).toHaveTextContent('45.50')
  // Food 200 + Living 90 EUR (100 USD)
  expect(screen.getByTestId('phone-total-budget')).toHaveTextContent('300.00')
})
