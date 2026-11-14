import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers, fixtureElementDebt, fixtureElementDebtNotSetUp, fixtureWireBudget } from '@/test/fixtures'
import { coerceBudgetFixture } from '@/test/coerceBudget'
import type { BudgetDto, BudgetElementDto } from '@/api/dto/budget'
import { useUiStore } from '@/app/uiStore'
import { BudgetTable } from './BudgetTable'
import { bucketElements, makeBudgetExchange } from './budgetMath'
import { useBudgetPeriodStore } from './budgetStore'
import { currentPeriod } from './period'

const usd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const eur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 }

function renderTable(adjust: (budget: BudgetDto) => void, canPlanDebts = true) {
  const budget = coerceBudgetFixture(fixtureWireBudget)
  adjust(budget)
  const buckets = bucketElements(budget, makeBudgetExchange(budget, [usd, eur]))
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <BudgetTable budget={budget} buckets={buckets} dataPeriod={budget.filters.periodStart.slice(0, 10)} canPlanDebts={canPlanDebts} />
    </QueryClientProvider>,
  )
  return budget
}

const food = (b: BudgetDto) => b.structure.elements.find((el) => el.id === 'cat-food') as BudgetElementDto
const thisMonth = (b: BudgetDto) => {
  b.filters.periodStart = `${currentPeriod(1)} 00:00:00`
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  useUiStore.setState({ debtDetailsId: null, debtDetailsTab: 'overview' })
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: {}, foldBudgetId: null, planFolds: {}, budgetFolds: {}, planUnfoldedElements: {} })
})

it('a category carrying a debt keeps its own columns and adds a quiet debt line', async () => {
  renderTable((b) => {
    food(b).debt = fixtureElementDebt()
  })
  const row = screen.getByTestId('element-cat-food')
  // the element's own Budget, Spent and Available, unchanged by the debt
  expect(within(row).getByTestId('cell-budgeted')).toHaveTextContent('200 +200')
  expect(within(row).getByTestId('cell-spent')).toHaveTextContent('45.5')
  expect(within(row).getByTestId('cell-available')).toHaveTextContent('354.5')
  const line = within(row).getByTestId('element-debt-line')
  expect(line).toHaveTextContent('24 payments left · Paid off by Oct 2028')
  // the line opens the debt's review
  await userEvent.click(within(line).getByRole('button'))
  expect(useUiStore.getState().debtDetailsId).toBe('debt-1')
  expect(useUiStore.getState().debtDetailsTab).toBe('overview')
  // an element with no debt has no line
  expect(within(screen.getByTestId('element-env-1')).queryByTestId('element-debt-line')).toBeNull()
})

it('an envelope carries its debt the same way', () => {
  renderTable((b) => {
    b.structure.elements.find((el) => el.id === 'env-1')!.debt = fixtureElementDebt({ paymentsLeft: 1 })
  })
  expect(within(screen.getByTestId('element-env-1')).getByTestId('element-debt-line')).toHaveTextContent('1 payment left')
})

it('a paid-off debt says so', () => {
  renderTable((b) => {
    food(b).debt = fixtureElementDebt({ isPaidOff: 1, paymentsLeft: 0, closingBalance: '0' })
  })
  expect(within(screen.getByTestId('element-cat-food')).getByTestId('element-debt-line')).toHaveTextContent(/^Paid off$/)
})

it('a debt not set up offers whoever plans in the budget Finish setup, on Settings', async () => {
  renderTable((b) => {
    food(b).debt = fixtureElementDebtNotSetUp()
  })
  const line = within(screen.getByTestId('element-cat-food')).getByTestId('element-debt-line')
  expect(line).not.toHaveTextContent('payments left')
  await userEvent.click(within(line).getByRole('button', { name: 'Finish setup' }))
  expect(useUiStore.getState().debtDetailsId).toBe('debt-1')
  expect(useUiStore.getState().debtDetailsTab).toBe('settings')
})

it('anyone else sees nothing extra on a debt not set up', () => {
  renderTable((b) => {
    food(b).debt = fixtureElementDebtNotSetUp()
  }, false)
  const row = screen.getByTestId('element-cat-food')
  expect(within(row).queryByTestId('element-debt-line')).toBeNull()
  expect(within(row).queryByRole('button', { name: 'Finish setup' })).toBeNull()
})

describe('the plan hint on a debt-carrying row', () => {
  it('is red when the plan is below the scheduled amount', () => {
    renderTable((b) => {
      thisMonth(b)
      Object.assign(food(b), { budgeted: '500', spent: '0', upcoming: '0', debt: fixtureElementDebt({ scheduledTotal: '1000' }) })
    })
    expect(within(screen.getByTestId('element-cat-food')).getByTestId('plan-figure').className).toContain('text-expense')
  })

  it('replaces the expense hint: what is still due on the category does not count', () => {
    renderTable((b) => {
      thisMonth(b)
      // without the debt, 500 below 0 spent + 900 due would be red
      Object.assign(food(b), { budgeted: '500', spent: '0', upcoming: '900', debt: fixtureElementDebt({ scheduledTotal: '400' }) })
    })
    expect(within(screen.getByTestId('element-cat-food')).getByTestId('plan-figure').className).not.toContain('text-expense')
  })

  it('a debt paid off in the period gives the expense hint back', () => {
    renderTable((b) => {
      thisMonth(b)
      // the debt alone would not mark 500 (scheduled 400); 0 spent + 900 due does
      Object.assign(food(b), { budgeted: '500', spent: '0', upcoming: '900', debt: fixtureElementDebt({ scheduledTotal: '400', isPaidOff: 1, paymentsLeft: 0 }) })
    })
    expect(within(screen.getByTestId('element-cat-food')).getByTestId('plan-figure').className).toContain('text-expense')
  })

  it('a past month is never red', () => {
    renderTable((b) => {
      b.filters.periodStart = '2020-01-01 00:00:00'
      Object.assign(food(b), { budgeted: '500', debt: fixtureElementDebt({ scheduledTotal: '1000' }) })
    })
    expect(within(screen.getByTestId('element-cat-food')).getByTestId('plan-figure').className).not.toContain('text-expense')
  })
})
