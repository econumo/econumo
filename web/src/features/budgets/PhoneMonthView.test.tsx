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
  income: {
    rows: [{ element: salaries, planned: '2000', actual: '400' }],
    groups: [{ kind: 'loose', id: '__no_folder__', name: null, rows: [{ element: salaries, planned: '2000', actual: '400' }], planned: '2000', received: '400', toReceive: '0' }],
    planned: '2000',
    received: '400',
    toReceive: '0',
  },
  balance: '4545',
  savingsBalance: null,
  transfersNet: '0',
}

// one folder-less income row, as both the flat list and its group
function incomeOf(row: { element: PlanElementDto; planned: string; actual: string }): PlanMonthFigures['income'] {
  return {
    rows: [row],
    groups: [{ kind: 'loose', id: '__no_folder__', name: null, rows: [row], planned: row.planned, received: row.actual, toReceive: '0' }],
    planned: row.planned,
    received: row.actual,
    toReceive: '0',
  }
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
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: {}, foldBudgetId: null, planFolds: {}, budgetFolds: {}, planUnfoldedElements: {} })
})

it('heads income and savings with one Planned · Actual row and the expenses with Expenses · Budget · Spent', () => {
  renderView()
  const flows = screen.getByTestId('phone-heading-flows')
  // the budget currency is named once, at the left of the top heading
  expect(flows).toHaveTextContent(/^USDPlannedActual$/)
  expect(screen.getAllByText('USD')).toHaveLength(1)
  const expenses = screen.getByTestId('phone-heading-expenses')
  expect(expenses).toHaveTextContent(/^ExpensesBudgetSpent$/)
  // a wider gap sets the expenses apart from the income/savings card
  expect(expenses.className).toContain('mt-3')
  // the headings sit right above their cards
  expect(flows.nextElementSibling).toBe(screen.getByTestId('phone-flows'))
  expect(screen.getByTestId('phone-flows').nextElementSibling).toBe(expenses)
  expect(expenses.nextElementSibling).toBe(screen.getByTestId('phone-folder-bf1'))
})

it('income and savings each get their own card, so Savings never reads as an income folder', () => {
  renderView({}, (b) => {
    b.structure.savings = [
      { id: 'acc-s1', type: 5, name: 'Rainy day', icon: 'savings', currencyId: 'cur-usd', ownerUserId: 'u1', isArchived: 0, position: 0, budgeted: '100', spent: '20', available: '80' } as never,
    ]
  })
  const income = screen.getByTestId('phone-income-card')
  expect(within(income).getByTestId('phone-income-summary')).toHaveTextContent(/^Income/)
  expect(within(income).queryByTestId('phone-savings-summary')).toBeNull()
  expect(within(screen.getByTestId('phone-savings-card')).getByTestId('phone-savings-summary')).toHaveTextContent(/^Savings/)
})

it('unfolded, Income is grouped like the Plan grid: a folder line with its sums, then No folder', async () => {
  const salariesEnvelope = {
    ...salaries,
    type: 4,
    children: [{ id: 'cat-salary', type: 3, name: 'Salary', icon: 'payments', isArchived: 0, ownerUserId: 'u1', cells: [{ actual: '0' }, { actual: '0' }, { actual: '350' }] }],
  } as PlanElementDto
  const freelance = { ...salaries, id: 'cat-freelance', type: 3, name: 'Freelance', children: [] } as PlanElementDto
  const grouped: PlanMonthFigures = {
    ...planMonth,
    income: {
      rows: [],
      groups: [
        { kind: 'folder', id: 'bf-inc', name: 'Work', rows: [{ element: salariesEnvelope, planned: '2000', actual: '350' }], planned: '2000', received: '350', toReceive: '0' },
        { kind: 'loose', id: '__no_folder__', name: null, rows: [{ element: freelance, planned: '500', actual: '50' }], planned: '500', received: '50', toReceive: '0' },
      ],
      planned: '2500',
      received: '400',
      toReceive: '0',
    },
  }
  renderView({ planMonth: grouped })
  await userEvent.click(screen.getByTestId('phone-income-summary'))
  const work = screen.getByTestId('phone-income-group-bf-inc')
  expect(work).toHaveTextContent(/^Work2,000\.00350\.00/)
  expect(within(work).getByTestId('phone-income-row-ie1')).toBeInTheDocument()
  const loose = screen.getByTestId('phone-income-group-__no_folder__')
  expect(loose).toHaveTextContent(/^No folder500\.0050\.00/)
  expect(within(loose).getByTestId('phone-income-row-cat-freelance')).toBeInTheDocument()

  // the envelope's name unfolds its categories, each with its own received amount
  expect(screen.queryByTestId('phone-child-cat-salary')).toBeNull()
  await userEvent.click(within(work).getByRole('button', { name: /Salaries/, expanded: false }))
  expect(screen.getByTestId('phone-child-cat-salary')).toHaveTextContent('350.00')
})

it('a folder line folds its rows, keeps its sums, and the fold is the Plan grid\'s', async () => {
  renderView()
  const card = screen.getByTestId('phone-folder-bf1')
  const line = within(card).getByRole('button', { name: /Essentials/, expanded: true })
  expect(within(card).getAllByTestId(/^phone-row-/).length).toBeGreaterThan(0)
  await userEvent.click(line)
  expect(within(screen.getByTestId('phone-folder-bf1')).queryAllByTestId(/^phone-row-/)).toHaveLength(0)
  expect(within(screen.getByTestId('phone-folder-bf1')).getByRole('button', { name: /Essentials/, expanded: false })).toHaveTextContent('200.00')
  expect(useBudgetPeriodStore.getState().budgetFolds.bf1).toBe(true)
})

it('an income folder line folds its rows too', async () => {
  const freelance = { ...salaries, id: 'cat-freelance', type: 3, name: 'Freelance', children: [] } as PlanElementDto
  const grouped: PlanMonthFigures = {
    ...planMonth,
    income: {
      rows: [],
      groups: [{ kind: 'folder', id: 'bf-inc', name: 'Work', rows: [{ element: freelance, planned: '500', actual: '50' }], planned: '500', received: '50', toReceive: '0' }],
      planned: '500',
      received: '50',
      toReceive: '0',
    },
  }
  renderView({ planMonth: grouped })
  await userEvent.click(screen.getByTestId('phone-income-summary'))
  await userEvent.click(within(screen.getByTestId('phone-income-group-bf-inc')).getByRole('button', { name: /Work/, expanded: true }))
  expect(screen.queryByTestId('phone-income-row-cat-freelance')).toBeNull()
  expect(screen.getByTestId('phone-income-group-bf-inc')).toHaveTextContent('500.00')
  expect(useBudgetPeriodStore.getState().budgetFolds['bf-inc']).toBe(true)
})

it('without income folders the rows stay a plain list, no "No folder" line', async () => {
  renderView()
  await userEvent.click(screen.getByTestId('phone-income-summary'))
  expect(screen.getByTestId('phone-income-group-__no_folder__')).not.toHaveTextContent('No folder')
})

it('drops the income/savings card while the plan is not loaded and there are no savings rows', () => {
  renderView({ planMonth: null })
  expect(screen.queryByTestId('phone-heading-flows')).toBeNull()
  expect(screen.queryByTestId('phone-flows')).toBeNull()
  expect(screen.getByTestId('phone-heading-expenses')).toBeInTheDocument()
})

it('shows each expense row as one button with Budget and Spent, no symbols', async () => {
  const props = renderView()
  const food = screen.getByRole('button', { name: 'Food, budget 200.00 + 200.00, spent 45.50' })
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
  const bar = within(row).getByTestId('phone-progress').firstElementChild
  expect(bar).toHaveStyle({ width: '100%' })
  expect(bar?.className).toContain('bg-expense')
  expect(within(row).getByText('150.00').className).toContain('text-expense')
})

it('spending with no budget fills the bar in red', () => {
  renderView({}, (b) => {
    const food = b.structure.elements.find((el) => el.id === 'cat-food')!
    Object.assign(food, { budgeted: '0', spent: '40', budgetSpent: '40', available: '-40' })
  })
  const row = screen.getByTestId('phone-row-cat-food')
  const bar = within(row).getByTestId('phone-progress').firstElementChild
  expect(bar).toHaveStyle({ width: '100%' })
  expect(bar?.className).toContain('bg-expense')
  expect(within(row).getByText('40.00').className).toContain('text-expense')
})

it('spending with no budget on top of a debt from earlier months fills the bar in red', () => {
  renderView({}, (b) => {
    const food = b.structure.elements.find((el) => el.id === 'cat-food')!
    Object.assign(food, { budgeted: '0', spent: '40', budgetSpent: '40', available: '-70' })
  })
  const bar = within(screen.getByTestId('phone-row-cat-food')).getByTestId('phone-progress').firstElementChild
  expect(bar).toHaveStyle({ width: '100%' })
  expect(bar?.className).toContain('bg-expense')
})

it('an overspend that earlier months still cover stays gray', () => {
  renderView({}, (b) => {
    const food = b.structure.elements.find((el) => el.id === 'cat-food')!
    Object.assign(food, { budgeted: '100', spent: '150', budgetSpent: '150', available: '10' })
  })
  const row = screen.getByTestId('phone-row-cat-food')
  expect(within(row).getByText('150.00').className).not.toContain('text-expense')
  expect(within(row).getByTestId('phone-progress').firstElementChild?.className).toContain('bg-muted-foreground/40')
})

it('shows what earlier months left, read-only, before this month’s budget', () => {
  // Food: wire available 154.50 + spent 45.50 = 200.00 left from earlier months
  renderView()
  const row = screen.getByTestId('phone-row-cat-food')
  expect(within(row).getByTestId('phone-carry')).toHaveTextContent('200.00 +')
  expect(within(row).getByTestId('phone-carry').className).toContain('text-muted-foreground')
  // the bar measures spending against carry-over plus budget: 45.50 of 400
  expect(within(row).getByTestId('phone-progress').firstElementChild).toHaveStyle({ width: '11%' })
})

it('shows no carry-over when earlier months left nothing, and a negative one in red', () => {
  renderView({}, (b) => {
    const food = b.structure.elements.find((el) => el.id === 'cat-food')!
    Object.assign(food, { available: '-45.5' })
    const living = b.structure.elements.find((el) => el.id === 'env-1')!
    Object.assign(living, { available: '-30' })
  })
  expect(within(screen.getByTestId('phone-row-cat-food')).queryByTestId('phone-carry')).toBeNull()
  const debt = within(screen.getByTestId('phone-row-env-1')).getByTestId('phone-carry')
  expect(debt).toHaveTextContent('-30.00 +')
  expect(debt.className).toContain('text-expense')
})

it('spend within the budget stays gray, even when Available is negative from earlier months', () => {
  renderView({}, (b) => {
    const food = b.structure.elements.find((el) => el.id === 'cat-food')!
    Object.assign(food, { budgeted: '120', spent: '100', budgetSpent: '100', available: '-150' })
  })
  const row = screen.getByTestId('phone-row-cat-food')
  expect(within(row).getByText('100.00').className).not.toContain('text-expense')
  expect(within(row).getByTestId('phone-progress').firstElementChild?.className).toContain('bg-muted-foreground/40')
})

it('a row with no budget draws the empty gray track', () => {
  renderView({}, (b) => {
    const food = b.structure.elements.find((el) => el.id === 'cat-food')!
    Object.assign(food, { budgeted: '0', spent: '0', budgetSpent: '0', available: '0' })
  })
  const bar = within(screen.getByTestId('phone-row-cat-food')).getByTestId('phone-progress')
  expect(bar.className).toContain('bg-muted')
  expect(bar.firstElementChild).toHaveStyle({ width: '0%' })
})

it('a future month shows a dash for Spent, the empty gray track, and no colour', () => {
  renderView({ selectedDate: '2099-01-01' }, (b) => {
    const food = b.structure.elements.find((el) => el.id === 'cat-food')!
    Object.assign(food, { budgeted: '100', spent: '150', budgetSpent: '150', available: '-120' })
  })
  const row = screen.getByTestId('phone-row-cat-food')
  const bar = within(row).getByTestId('phone-progress').firstElementChild
  expect(bar).toHaveStyle({ width: '0%' })
  expect(bar?.className).toContain('bg-muted-foreground/40')
  expect(within(row).getByText('—').className).not.toContain('text-expense')
  expect(screen.getByRole('button', { name: 'Food, budget 30.00 + 100.00, spent —' })).toBeInTheDocument()
})

const rainyDay = { id: 'acc-s1', type: 5, name: 'Rainy day', icon: 'savings', currencyId: 'cur-usd', ownerUserId: 'u1', isArchived: 0, position: 0, budgeted: '100', spent: '40', available: '60', closingBalance: '1040' } as const

it('an income row fills its bar toward the plan and stays gray until the plan is met', async () => {
  renderView()
  await userEvent.click(screen.getByTestId('phone-income-summary'))
  const bar = within(screen.getByTestId('phone-income-row-ie1')).getByTestId('phone-progress')
  expect(bar.firstElementChild).toHaveStyle({ width: '20%' })
  expect(bar.firstElementChild?.className).toContain('bg-muted-foreground/40')
})

it('an income row that received its plan turns its bar green', async () => {
  const met = { ...planMonth, income: incomeOf({ element: salaries, planned: '2000', actual: '2500' }) }
  renderView({ planMonth: met })
  await userEvent.click(screen.getByTestId('phone-income-summary'))
  const row = screen.getByTestId('phone-income-row-ie1')
  const bar = within(row).getByTestId('phone-progress')
  expect(bar.firstElementChild).toHaveStyle({ width: '100%' })
  expect(bar.firstElementChild?.className).toContain('bg-income')
  // only the bar carries the colour: receiving more than planned is no warning
  expect(within(row).getByText('2,500.00').className).not.toMatch(/text-(income|expense)/)
})

it('an income row with no plan draws the empty gray track', async () => {
  const unplanned = { ...planMonth, income: incomeOf({ element: salaries, planned: '0', actual: '300' }) }
  renderView({ planMonth: unplanned })
  await userEvent.click(screen.getByTestId('phone-income-summary'))
  const bar = within(screen.getByTestId('phone-income-row-ie1')).getByTestId('phone-progress')
  expect(bar.firstElementChild).toHaveStyle({ width: '0%' })
  expect(bar.firstElementChild?.className).toContain('bg-muted-foreground/40')
})

it('a savings row fills its bar toward the plan and turns green once the plan is saved', async () => {
  renderView({}, (b) => {
    b.structure.savings = [rainyDay, { ...rainyDay, id: 'acc-s2', name: 'House', position: 1, budgeted: '50', spent: '50' }]
  })
  await userEvent.click(screen.getByTestId('phone-savings-summary'))
  const partial = within(screen.getByTestId('phone-savings-row-acc-s1')).getByTestId('phone-progress').firstElementChild
  expect(partial).toHaveStyle({ width: '40%' })
  expect(partial?.className).toContain('bg-muted-foreground/40')
  const met = within(screen.getByTestId('phone-savings-row-acc-s2')).getByTestId('phone-progress').firstElementChild
  expect(met).toHaveStyle({ width: '100%' })
  expect(met?.className).toContain('bg-income')
})

it('a savings withdrawal draws the empty track', async () => {
  renderView({}, (b) => {
    b.structure.savings = [{ ...rainyDay, spent: '-30' }]
  })
  await userEvent.click(screen.getByTestId('phone-savings-summary'))
  const bar = within(screen.getByTestId('phone-savings-row-acc-s1')).getByTestId('phone-progress').firstElementChild
  expect(bar).toHaveStyle({ width: '0%' })
  expect(bar?.className).toContain('bg-muted-foreground/40')
})

it('a future month draws the empty gray track on income and savings rows, even for a met plan', async () => {
  renderView({ selectedDate: '2099-01-01' }, (b) => {
    b.structure.savings = [{ ...rainyDay, spent: '100' }]
  })
  await userEvent.click(screen.getByTestId('phone-income-summary'))
  await userEvent.click(screen.getByTestId('phone-savings-summary'))
  for (const testId of ['phone-income-row-ie1', 'phone-savings-row-acc-s1']) {
    const bar = within(screen.getByTestId(testId)).getByTestId('phone-progress').firstElementChild
    expect(bar).toHaveStyle({ width: '0%' })
    expect(bar?.className).toContain('bg-muted-foreground/40')
  }
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

// jsdom has no layout, so pin the geometry that keeps the columns aligned: a row's
// figures subgrid insets only its last track, so the other lines must keep their
// tracks flush right and pad just the last cell, or their Budget column shifts left
it('headings, section summaries and folder headers pad only their last cell, like the rows', () => {
  renderView()
  const lines = [
    screen.getByTestId('phone-heading-expenses'),
    screen.getByTestId('phone-folder-bf1').firstElementChild as HTMLElement,
  ]
  for (const line of lines) {
    expect(line.className).not.toMatch(/(^|\s)(px|pr)-\d/)
    expect((line.lastElementChild as HTMLElement).className).toContain('pr-2')
  }
})

it('the name of an expandable row folds it; only the figures open the sheet', async () => {
  const props = renderView()
  const living = screen.getByTestId('phone-row-env-1')
  const name = within(living).getByRole('button', { expanded: false })
  expect(name).toHaveAttribute('aria-expanded', 'false')
  await userEvent.click(name)
  expect(name).toHaveAttribute('aria-expanded', 'true')
  expect(props.onOpenSheet).not.toHaveBeenCalled()
  expect(screen.getByTestId('phone-child-cat-rent')).toBeInTheDocument()
  await userEvent.click(name)
  expect(screen.queryByTestId('phone-child-cat-rent')).toBeNull()
  await userEvent.click(within(living).getByRole('button', { name: /^Living, budget/ }))
  expect(props.onOpenSheet).toHaveBeenCalledWith({ kind: 'expense', element: expect.objectContaining({ id: 'env-1' }) })
})

it('the name of a row without children does nothing', async () => {
  const props = renderView()
  const food = screen.getByTestId('phone-row-cat-food')
  await userEvent.click(within(food).getByText('Food'))
  expect(props.onOpenSheet).not.toHaveBeenCalled()
  expect(within(food).getAllByRole('button')).toHaveLength(1)
})

it('a child opens its transactions from its Spent, not its name', async () => {
  const props = renderView()
  await userEvent.click(within(screen.getByTestId('phone-row-env-1')).getByRole('button', { expanded: false }))
  const child = screen.getByTestId('phone-child-cat-rent')
  await userEvent.click(within(child).getByText('Rent'))
  expect(props.onShowTransactions).not.toHaveBeenCalled()
  await userEvent.click(within(child).getByRole('button', { name: 'Rent, spent 0.00' }))
  expect(props.onShowTransactions).toHaveBeenCalledWith(expect.objectContaining({ id: 'cat-rent', parent: { id: 'env-1', type: 0 } }))
})

it('collapses income into one summary row that unfolds into income rows', async () => {
  const props = renderView()
  const summary = screen.getByTestId('phone-income-summary')
  // a folder-style header: name, planned under Budget, received under Spent
  expect(within(summary).getByTestId('phone-income-planned')).toHaveTextContent('2,000.00')
  expect(within(summary).getByTestId('phone-income-received')).toHaveTextContent('400.00')
  expect(summary).toHaveTextContent(/^Income2,000\.00400\.00$/)
  expect(summary).toHaveAttribute('aria-expanded', 'false')
  expect(screen.queryByTestId('phone-income-row-ie1')).toBeNull()
  await userEvent.click(summary)
  await userEvent.click(screen.getByRole('button', { name: 'Salaries, planned 2,000.00, received 400.00' }))
  expect(props.onOpenSheet).toHaveBeenCalledWith({ kind: 'plan', cell: planMonth.income.rows[0] })
})

const totalLines = () =>
  Array.from(screen.getByTestId('phone-totals').querySelectorAll('[data-testid^="phone-total-"]')).map((el) => el.getAttribute('data-testid'))

it('leaves income and the plan lines out while the plan is not loaded', () => {
  renderView({ planMonth: null })
  expect(screen.queryByTestId('phone-income-summary')).toBeNull()
  expect(totalLines()).toEqual(['phone-total-budget', 'phone-total-expenses'])
})

it('the Total card reads Budget, Income, Expenses and Balance; Transfers only when non-zero', () => {
  renderView()
  expect(totalLines()).toEqual(['phone-total-budget', 'phone-total-income', 'phone-total-expenses', 'phone-total-balance'])
  // what earlier months left + this month's budget: 200 + 90 EUR → 300 USD left
  const budgetLine = screen.getByTestId('phone-total-budget')
  expect(budgetLine).toHaveTextContent('Budget')
  expect(budgetLine).toHaveTextContent('300.00 + 300.00')
  // expenses only: Food 354.50 + Living 180 EUR → 200 USD
  expect(within(budgetLine).getByTestId('phone-budget-available')).toHaveTextContent('554.50 available')
  expect(screen.getByTestId('phone-total-income')).toHaveTextContent(/^Income400\.00$/)
  expect(screen.getByTestId('phone-total-expenses')).toHaveTextContent('Expenses45.50')
  expect(screen.getByTestId('phone-total-balance')).toHaveTextContent('Balance at month end4,545.00')
})

it('Budget shows only this month\'s budget when earlier months left nothing, and a negative Available in red', () => {
  renderView({}, (b) => {
    // nothing left from earlier months (carry 0) and Food overspent by 145.50
    Object.assign(b.structure.elements.find((el) => el.id === 'cat-food')!, { spent: '345.5', budgetSpent: '345.5', available: '-345.5' })
    Object.assign(b.structure.elements.find((el) => el.id === 'env-1')!, { available: '0' })
  })
  const budgetLine = screen.getByTestId('phone-total-budget')
  expect(budgetLine).not.toHaveTextContent('+')
  const available = within(budgetLine).getByTestId('phone-budget-available')
  // -145.50 (Food) + 100.00 (Living, 90 EUR)
  expect(available).toHaveTextContent('-45.50 available')
  expect(available.className).toContain('text-expense')
})

it('a future month shows dashes for what has not happened yet', () => {
  renderView({ selectedDate: '2099-01-01' })
  expect(screen.getByTestId('phone-income-received')).toHaveTextContent('—')
  expect(screen.getByTestId('phone-total-expenses')).toHaveTextContent('Expenses—')
  expect(screen.getByTestId('phone-total-income')).toHaveTextContent(/^Income—$/)
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
  // savings is the second line of the shared card, under income
  const flows = screen.getByTestId('phone-flows')
  const lines = within(flows).getAllByRole('button', { expanded: false })
  expect(lines.map((b) => b.getAttribute('data-testid'))).toEqual(['phone-income-summary', 'phone-savings-summary'])
  // collapsed by default to one line: planned and saved, budget currency
  const summary = screen.getByTestId('phone-savings-summary')
  expect(summary).toHaveAttribute('aria-expanded', 'false')
  expect(summary).toHaveTextContent(/^Savings100\.0040\.00$/)
  expect(screen.queryByTestId('phone-savings-row-acc-s1')).toBeNull()
  await userEvent.click(summary)
  expect(summary).toHaveAttribute('aria-expanded', 'true')
  await userEvent.click(screen.getByRole('button', { name: 'Rainy day, planned 100.00, saved 40.00' }))
  expect(props.onOpenSheet).toHaveBeenCalledWith({ kind: 'savings', row: expect.objectContaining({ id: 'acc-s1' }) })
  expect(screen.getByTestId('phone-total-savings')).toHaveTextContent(/^Savings40\.00$/)
  expect(screen.getByTestId('phone-total-savings-balance')).toHaveTextContent('1,040.00')
  expect(totalLines()).toEqual([
    'phone-total-budget', 'phone-total-income', 'phone-total-expenses', 'phone-total-savings', 'phone-total-savings-balance', 'phone-total-balance',
  ])
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

it('puts the reporting tags card below the Archived card', () => {
  renderView({}, (b) => {
    b.structure.labels = [{ id: 'label-kid-a', name: 'kid-A', icon: 'label', isArchived: 0, spent: '50.00', ownerUserId: 'u1', children: [] }]
    const archived = b.structure.elements.find((e) => e.id === 'tag-old')!
    archived.spent = '12'
  })
  const archive = screen.getByTestId('phone-folder-__archive__')
  const labels = screen.getByTestId('phone-labels')
  expect(archive.compareDocumentPosition(labels) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})

it('tapping a reporting tag opens its sheet', async () => {
  const label = { id: 'label-kid-a', name: 'kid-A', icon: 'label', isArchived: 0 as const, spent: '50.00', ownerUserId: 'u1', children: [] }
  const props = renderView({}, (b) => {
    b.structure.labels = [label]
  })
  await userEvent.click(screen.getByRole('button', { name: 'Reporting tags' }))
  await userEvent.click(screen.getByRole('button', { name: 'kid-A, spent 50.00' }))
  expect(props.onOpenSheet).toHaveBeenCalledWith({ kind: 'label', label })
  expect(props.onShowTransactions).not.toHaveBeenCalled()
})

it('an empty income folder reads as dashes in its Planned and Received', async () => {
  renderView({
    planMonth: {
      ...planMonth,
      income: {
        ...planMonth.income,
        groups: [...planMonth.income.groups, { kind: 'folder', id: 'bf-later', name: 'Later', rows: [], planned: '0', received: '0', toReceive: '0' }],
      },
    },
  })
  await userEvent.click(screen.getByTestId('phone-income-summary'))
  expect(screen.getByTestId('phone-income-group-bf-later')).toHaveTextContent(/^Later——$/)
})
