import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { coerceBudgetFixture } from '@/test/coerceBudget'
import { fixtureDebtRow, fixtureWireBudget } from '@/test/fixtures'
import type { BudgetDebtElementDto, BudgetDto } from '@/api/dto/budget'
import { BudgetElementType } from '@/api/dto/budget'
import { useBudgetPeriodStore } from './budgetStore'
import { MonthFlows } from './MonthFlows'
import { DebtExtraNote } from './debtRow'
import { planHintClass } from './planHint'
import { currentPeriod } from './period'
import { useUiStore } from '@/app/uiStore'

const usd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const eur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 }

function renderFlows(debts: BudgetDebtElementDto[], adjust?: (b: BudgetDto) => void, userId = 'u1') {
  const budget = coerceBudgetFixture(fixtureWireBudget)
  budget.structure.savings = [
    { id: 'acc-s1', type: 5, name: 'Rainy day', icon: 'savings', currencyId: 'cur-usd', ownerUserId: 'u1', isArchived: 0, position: 0, budgeted: '100', spent: '20', available: '80' },
  ]
  budget.structure.debts = debts
  adjust?.(budget)
  const onShowTransactions = vi.fn()
  render(
    <MonthFlows
      budget={budget}
      currencies={[usd, eur]}
      planMonth={null}
      future={false}
      actionsColumn={false}
      renderPlanned={(_t, text, hint) => (
        <span data-testid="planned-text" className={planHintClass(hint)}>
          {text}
        </span>
      )}
      onShowTransactions={onShowTransactions}
      userId={userId}
    />,
  )
  return { onShowTransactions }
}

beforeEach(() => {
  useBudgetPeriodStore.setState({ unfoldedElements: {}, planFolds: {}, budgetFolds: {}, planUnfoldedElements: {} })
})

it('renders the Debts section after Savings with Planned/Paid/Left', () => {
  renderFlows([fixtureDebtRow()])
  const savings = screen.getByTestId('month-savings')
  const debts = screen.getByTestId('month-debts')
  expect(savings.compareDocumentPosition(debts) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(within(debts).getByTestId('month-debts-header')).toHaveTextContent(/Debts.*Planned.*Paid.*Left/)
  const row = within(debts).getByTestId('month-debts-row-debt-1')
  expect(row).toHaveTextContent('Mortgage')
  expect(within(row).getByTestId('flow-planned')).toHaveTextContent('1,500.00')
  expect(within(row).getByTestId('flow-actual')).toHaveTextContent('900.00')
  expect(within(row).getByTestId('flow-third')).toHaveTextContent('23,000.00')
})

const notSetUpRow = () =>
  fixtureDebtRow({ closingBalance: '', paymentsLeft: 0, scheduledTotal: '0', upcoming: '0', payoffDate: '', basePayoffDate: '', overpaymentLeft: '', baseOverpaymentLeft: '', isSetUp: 0 })

it('debt row offers its owner Finish setup in Left while not set up', async () => {
  useUiStore.setState({ debtDetailsId: null, debtDetailsTab: 'overview' })
  renderFlows([notSetUpRow()])
  const row = screen.getByTestId('month-debts-row-debt-1')
  const left = within(row).getByTestId('flow-third')
  expect(left).not.toHaveTextContent('payments left')
  await userEvent.click(within(left).getByRole('button', { name: 'Finish setup' }))
  expect(useUiStore.getState().debtDetailsId).toBe('debt-1')
  // straight to the form the setup is finished in
  expect(useUiStore.getState().debtDetailsTab).toBe('settings')
  // Paid still shows; the section's Left sums only the debts set up
  expect(within(row).getByTestId('flow-actual')).toHaveTextContent('900.00')
})

it('debt row shows a dash for a partner when not set up', () => {
  renderFlows([notSetUpRow()], undefined, 'u2')
  const left = within(screen.getByTestId('month-debts-row-debt-1')).getByTestId('flow-third')
  expect(left).toHaveTextContent('—')
  expect(within(left).queryByRole('button')).toBeNull()
  expect(left).not.toHaveTextContent('Finish setup')
})

it('the Left total counts only the debts set up', () => {
  // a folded section shows its sums in the header
  useBudgetPeriodStore.setState({ budgetFolds: { debts: true } })
  renderFlows([fixtureDebtRow(), { ...notSetUpRow(), id: 'debt-2', position: 1 }])
  expect(screen.getByTestId('month-debts-header')).toHaveTextContent('23,000.00')
})

it('folds like the other sections', async () => {
  renderFlows([fixtureDebtRow()])
  await userEvent.click(within(screen.getByTestId('month-debts-header')).getByRole('button', { name: /Debts/ }))
  expect(screen.queryByTestId('month-debts-row-debt-1')).not.toBeInTheDocument()
  expect(useBudgetPeriodStore.getState().budgetFolds.debts).toBe(true)
})

it('shows payments left', () => {
  renderFlows([fixtureDebtRow(), fixtureDebtRow({ id: 'debt-2', name: 'Car', position: 1, paymentsLeft: 1 })])
  expect(within(screen.getByTestId('month-debts-row-debt-1')).getByTestId('flow-third')).toHaveTextContent('24 payments left')
  expect(within(screen.getByTestId('month-debts-row-debt-2')).getByTestId('flow-third')).toHaveTextContent('1 payment left')
})

it('a paid-off debt says so instead of counting payments', () => {
  renderFlows([fixtureDebtRow({ paymentsLeft: 0, closingBalance: '0', isPaidOff: 1 })])
  expect(within(screen.getByTestId('month-debts-row-debt-1')).getByTestId('flow-third')).toHaveTextContent('Paid off')
})

it('a plan below the scheduled amount is red', () => {
  const thisMonth = `${currentPeriod(1)} 00:00:00`
  renderFlows(
    [
      fixtureDebtRow({ budgeted: '500' }),
      // the plan covers the scheduled amount: no hint
      fixtureDebtRow({ id: 'debt-2', name: 'Car', position: 1, budgeted: '1000' }),
    ],
    (b) => {
      b.filters.periodStart = thisMonth
    },
  )
  expect(within(screen.getByTestId('month-debts-row-debt-1')).getByTestId('planned-text').className).toContain('text-expense')
  expect(within(screen.getByTestId('month-debts-row-debt-2')).getByTestId('planned-text').className).not.toContain('text-expense')
})

it('a past month is never red', () => {
  renderFlows([fixtureDebtRow({ budgeted: '500' })], (b) => {
    b.filters.periodStart = '2020-01-01 00:00:00'
  })
  expect(within(screen.getByTestId('month-debts-row-debt-1')).getByTestId('planned-text').className).not.toContain('text-expense')
})

it('Paid opens the debt\'s payments', async () => {
  const { onShowTransactions } = renderFlows([fixtureDebtRow()])
  await userEvent.click(within(screen.getByTestId('month-debts-row-debt-1')).getByRole('button', { name: 'transactions Mortgage' }))
  expect(onShowTransactions).toHaveBeenCalledWith({ id: 'debt-1', type: BudgetElementType.DEBT, name: 'Mortgage', icon: 'home', currencyId: 'cur-usd' })
})

it('no debts, no section', () => {
  renderFlows([])
  expect(screen.queryByTestId('month-debts')).not.toBeInTheDocument()
})

describe('the extra payment note', () => {
  it('says how much earlier the debt is paid off and how much overpayment it saves', () => {
    render(<DebtExtraNote row={fixtureDebtRow({ payoffDate: '2027-08-05', overpaymentLeft: '1500' })} currency={usd} />)
    // base payoff 2028-10-05, 2,000 overpayment left
    expect(screen.getByTestId('debt-extra-effect')).toHaveTextContent('Paid off 14 months earlier, 500.00 $ less overpayment')
  })

  it('an interest-free debt only gets paid off earlier', () => {
    render(<DebtExtraNote row={fixtureDebtRow({ payoffDate: '2028-09-05', overpaymentLeft: '0', baseOverpaymentLeft: '0' })} currency={usd} />)
    expect(screen.getByTestId('debt-extra-effect')).toHaveTextContent(/^Paid off 1 month earlier$/)
  })

  it('says nothing while the plans change nothing', () => {
    render(<DebtExtraNote row={fixtureDebtRow()} currency={usd} />)
    expect(screen.queryByTestId('debt-extra-effect')).not.toBeInTheDocument()
  })
})
