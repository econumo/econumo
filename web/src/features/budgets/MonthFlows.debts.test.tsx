import { render, screen } from '@testing-library/react'
import { coerceBudgetFixture } from '@/test/coerceBudget'
import { fixtureElementDebt, fixtureWireBudget } from '@/test/fixtures'
import { useBudgetPeriodStore } from './budgetStore'
import { MonthFlows } from './MonthFlows'
import { DebtExtraNote } from './debtRow'

const usd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const eur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 }

beforeEach(() => {
  useBudgetPeriodStore.setState({ unfoldedElements: {}, planFolds: {}, budgetFolds: {}, planUnfoldedElements: {} })
})

it('there is no Debts block: a debt shows on its element', () => {
  const budget = coerceBudgetFixture(fixtureWireBudget)
  budget.structure.elements[0].debt = fixtureElementDebt()
  budget.structure.savings = [
    { id: 'acc-s1', type: 5, name: 'Rainy day', icon: 'savings', currencyId: 'cur-usd', ownerUserId: 'u1', isArchived: 0, position: 0, budgeted: '100', spent: '20', available: '80' },
  ]
  render(
    <MonthFlows
      budget={budget}
      dataPeriod="2026-07-01"
      currencies={[usd, eur]}
      planMonth={null}
      future={false}
      actionsColumn={false}
      renderPlanned={(_t, text) => <span>{text}</span>}
    />,
  )
  expect(screen.getByTestId('month-savings')).toBeInTheDocument()
  expect(screen.queryByTestId('month-debts')).toBeNull()
  expect(screen.queryByText('Debts')).toBeNull()
})

describe('the extra payment note', () => {
  it('says how much earlier the debt is paid off and how much overpayment it saves', () => {
    render(<DebtExtraNote row={fixtureElementDebt({ payoffDate: '2027-08-05', overpaymentLeft: '1500' })} currency={usd} />)
    // base payoff 2028-10-05, 2,000 overpayment left
    expect(screen.getByTestId('debt-extra-effect')).toHaveTextContent('Paid off 14 months earlier, 500.00 $ less overpayment')
  })

  it('an interest-free debt only gets paid off earlier', () => {
    render(<DebtExtraNote row={fixtureElementDebt({ payoffDate: '2028-09-05', overpaymentLeft: '0', baseOverpaymentLeft: '0' })} currency={usd} />)
    expect(screen.getByTestId('debt-extra-effect')).toHaveTextContent(/^Paid off 1 month earlier$/)
  })

  it('says nothing while the plans change nothing', () => {
    render(<DebtExtraNote row={fixtureElementDebt()} currency={usd} />)
    expect(screen.queryByTestId('debt-extra-effect')).not.toBeInTheDocument()
  })
})
