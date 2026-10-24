import type { BudgetDto, BudgetElementDto, BudgetPlanDto, PlanSavingsElementDto } from '@/api/dto/budget'
import { add } from '@/lib/decimal'
import { makePlanExchange, planTotals, savingsBalanceRow } from './planMath'
import { foldBudgetUpcoming, foldPlanUpcoming } from './upcoming'

const meta = {
  id: 'b1', ownerUserId: 'u1', name: 'B', startedAt: '2026-01-01 00:00:00', endedAt: '', currencyId: 'usd', isArchived: 0 as const, access: [],
}

function element(over: Partial<BudgetElementDto> = {}): BudgetElementDto {
  return {
    id: 'e1', type: 1, name: 'Food', icon: 'i', isArchived: 0, currencyId: 'usd', folderId: null, position: 0,
    budgeted: '200', available: '80', spent: '120', budgetSpent: '120', ownerUserId: 'u1', children: [], ...over,
  }
}

function budget(over: Partial<BudgetDto['structure']> = {}): BudgetDto {
  return {
    meta,
    filters: { periodStart: '2026-10-01 00:00:00', periodEnd: '2026-11-01 00:00:00' },
    balances: [],
    currencyRates: [],
    structure: { folders: [], elements: [], labels: [], savings: [], ...over },
  }
}

// the fixtures' selected month and plan window open in October 2026
const SEPTEMBER = '2026-09-01'
const OCTOBER = '2026-10-01'

it('foldBudgetUpcoming adds upcoming to spent and lowers available in a later month', () => {
  const folded = foldBudgetUpcoming(
    budget({
      elements: [
        element({
          upcoming: '50', budgetUpcoming: '45', upcomingBefore: '0',
          children: [{ id: 'c1', type: 1, name: 'C', icon: 'i', isArchived: 0, spent: '20', budgetSpent: '18', ownerUserId: 'u1', upcoming: '5', budgetUpcoming: '4.5' }],
        }),
        element({ id: 'e2', upcoming: '10', budgetUpcoming: '10', upcomingBefore: '30' }),
      ],
      labels: [{
        id: 'l1', name: 'L', icon: 'i', isArchived: 0, spent: '7', ownerUserId: 'u1', upcoming: '3',
        children: [
          { id: 'c1', type: 1, name: 'C', icon: 'i', isArchived: 0, spent: '7', budgetSpent: '7', ownerUserId: 'u1', upcoming: '1', budgetUpcoming: '1' },
          // an upcoming-only category under the label
          { id: 'c2', type: 1, name: 'D', icon: 'i', isArchived: 0, spent: '0', budgetSpent: '0', ownerUserId: 'u1', upcoming: '2', budgetUpcoming: '2' },
        ],
      }],
      savings: [{
        id: 's1', type: 5, name: 'S', icon: 'i', currencyId: 'usd', ownerUserId: 'u1', isArchived: 0, position: 0,
        budgeted: '100', spent: '40', available: '60', closingBalance: '900', upcoming: '25',
      }],
    }),
    SEPTEMBER,
  )
  const [food, second] = folded.structure.elements
  expect(food.spent).toBe('170')
  expect(food.budgetSpent).toBe('165')
  expect(food.available).toBe('30')
  expect(food.children[0].spent).toBe('25')
  expect(food.children[0].budgetSpent).toBe('22.5')
  expect(second.available).toBe('40')
  const label = folded.structure.labels![0]
  expect(label.spent).toBe('10')
  expect(label.children!.map((c) => [c.spent, c.budgetSpent])).toEqual([['8', '8'], ['2', '2']])
  const savings = folded.structure.savings![0]
  expect(savings.spent).toBe('65')
  expect(savings.available).toBe('35')
  // the server already counts upcoming money in a get-budget closing balance
  expect(savings.closingBalance).toBe('900')
})

it('fold is identity without upcoming fields', () => {
  const raw = budget({ elements: [element()], savings: undefined, labels: undefined })
  expect(foldBudgetUpcoming(raw, SEPTEMBER)).toEqual(raw)

  const plan: BudgetPlanDto = {
    meta,
    months: ['2026-10-01'],
    openingBalances: [{ currencyId: 'usd', amount: '500' }],
    currencyRates: [],
    transfers: [{ period: '2026-10-01', items: [{ currencyId: 'usd', in: '1', out: '2' }] }],
    structure: {
      folders: [],
      elements: [{ id: 'e1', type: 1, name: 'F', icon: 'i', currencyId: 'usd', isArchived: 0, folderId: null, position: 0, ownerUserId: 'u1', cells: [{ actual: '5', planned: '' }], children: [] }],
    },
  }
  expect(foldPlanUpcoming(plan, SEPTEMBER)).toEqual(plan)
})

it('fold results are memoized per data object', () => {
  const raw = budget({ elements: [element({ upcoming: '1', budgetUpcoming: '1', upcomingBefore: '0' })] })
  expect(foldBudgetUpcoming(raw, SEPTEMBER)).toBe(foldBudgetUpcoming(raw, SEPTEMBER))
  const plan = planWith()
  expect(foldPlanUpcoming(plan, OCTOBER)).toBe(foldPlanUpcoming(plan, OCTOBER))
  // a new month folds again
  expect(foldPlanUpcoming(plan, SEPTEMBER)).not.toBe(foldPlanUpcoming(plan, OCTOBER))
})

it('the current month is not folded: spent, available and labels stay the fact, and the upcoming fields stay readable', () => {
  const raw = budget({
    elements: [
      element({
        upcoming: '50', budgetUpcoming: '45', upcomingBefore: '0',
        children: [{ id: 'c1', type: 1, name: 'C', icon: 'i', isArchived: 0, spent: '0', budgetSpent: '0', ownerUserId: 'u1', upcoming: '5', budgetUpcoming: '4.5' }],
      }),
    ],
    labels: [{ id: 'l1', name: 'L', icon: 'i', isArchived: 0, spent: '7', ownerUserId: 'u1', upcoming: '3' }],
    savings: [{
      id: 's1', type: 5, name: 'S', icon: 'i', currencyId: 'usd', ownerUserId: 'u1', isArchived: 0, position: 0,
      budgeted: '100', spent: '40', available: '60', closingBalance: '900', upcoming: '25',
    }],
  })
  const folded = foldBudgetUpcoming(raw, OCTOBER)
  const [food] = folded.structure.elements
  expect([food.spent, food.budgetSpent, food.available]).toEqual(['120', '120', '80'])
  expect(food.upcoming).toBe('50')
  expect(food.children[0].spent).toBe('0')
  expect(food.children[0].upcoming).toBe('5')
  expect(folded.structure.labels![0].spent).toBe('7')
  const savings = folded.structure.savings![0]
  expect([savings.spent, savings.available, savings.closingBalance]).toEqual(['40', '60', '900'])
  // an earlier month carries no upcoming money and is left alone as well
  expect(foldBudgetUpcoming(raw, '2026-11-01')).toEqual(raw)
})

it('a later month still carries the current month\'s upcoming money over (upcomingBefore)', () => {
  const raw = budget({ elements: [element({ budgeted: '200', available: '80', spent: '0', budgetSpent: '0', upcoming: '20', budgetUpcoming: '20', upcomingBefore: '50' })] })
  const [food] = foldBudgetUpcoming(raw, SEPTEMBER).structure.elements
  expect(food.spent).toBe('20')
  expect(food.available).toBe('10')
})

function planWith(over: Partial<BudgetPlanDto> = {}, savings?: PlanSavingsElementDto[]): BudgetPlanDto {
  return {
    meta,
    months: ['2026-10-01', '2026-11-01', '2026-12-01'],
    openingBalances: [{ currencyId: 'usd', amount: '500' }],
    savingsOpeningBalances: [{ currencyId: 'usd', amount: '1000' }],
    currencyRates: [],
    transfers: [
      { period: '2026-10-01', items: [{ currencyId: 'usd', in: '10', out: '5' }] },
      { period: '2026-11-01', items: [] },
      { period: '2026-12-01', items: [] },
    ],
    savingsFlows: [{ month: '2026-10-01', currencyId: 'usd', amount: '40' }],
    savingsIncomeExpense: [],
    structure: {
      folders: [],
      elements: [{
        id: 'e1', type: 0, name: 'Env', icon: 'i', currencyId: 'usd', isArchived: 0, folderId: null, position: 0, ownerUserId: null,
        cells: [{ actual: '5', planned: '10', upcoming: '20' }, { actual: '0', planned: '', upcoming: '7.5' }, { actual: '0', planned: '' }],
        children: [{ id: 'c1', type: 1, name: 'C', icon: 'i', isArchived: 0, ownerUserId: 'u1', cells: [{ actual: '5', upcoming: '20' }, { actual: '0', upcoming: '7.5' }, { actual: '0' }] }],
      }],
      savings: savings ?? [],
    },
    ...over,
  }
}

it('foldPlanUpcoming adds cell upcoming and merges transfers by currency', () => {
  const folded = foldPlanUpcoming(
    planWith({
      upcomingOpeningBalances: [{ currencyId: 'usd', amount: '15' }, { currencyId: 'eur', amount: '3' }],
      upcomingSavingsOpeningBalances: [{ currencyId: 'usd', amount: '0' }],
      upcomingTransfers: [
        { period: '2026-10-01', items: [{ currencyId: 'usd', in: '100', out: '0' }, { currencyId: 'eur', in: '0', out: '9' }] },
        { period: '2026-11-01', items: [{ currencyId: 'usd', in: '0', out: '4' }] },
        { period: '2026-12-01', items: [] },
      ],
      upcomingSavingsFlows: [{ month: '2026-10-01', currencyId: 'usd', amount: '60' }, { month: '2026-11-01', currencyId: 'usd', amount: '60' }],
      upcomingSavingsIncomeExpense: [{ month: '2026-11-01', currencyId: 'usd', amount: '2' }],
    }),
    SEPTEMBER,
  )
  const el = folded.structure.elements[0]
  expect(el.cells.map((c) => c.actual)).toEqual(['25', '7.5', '0'])
  expect(el.cells[0].planned).toBe('10')
  expect(el.children[0].cells.map((c) => c.actual)).toEqual(['25', '7.5', '0'])
  expect(folded.openingBalances).toEqual([{ currencyId: 'usd', amount: '515' }, { currencyId: 'eur', amount: '3' }])
  expect(folded.savingsOpeningBalances).toEqual([{ currencyId: 'usd', amount: '1000' }])
  expect(folded.transfers).toEqual([
    { period: '2026-10-01', items: [{ currencyId: 'usd', in: '110', out: '5' }, { currencyId: 'eur', in: '0', out: '9' }] },
    { period: '2026-11-01', items: [{ currencyId: 'usd', in: '0', out: '4' }] },
    { period: '2026-12-01', items: [] },
  ])
  expect(folded.savingsFlows).toEqual([
    { month: '2026-10-01', currencyId: 'usd', amount: '100' },
    { month: '2026-11-01', currencyId: 'usd', amount: '60' },
  ])
  expect(folded.savingsIncomeExpense).toEqual([{ month: '2026-11-01', currencyId: 'usd', amount: '2' }])
})

it('savings closing adds cumulative upcoming', () => {
  const folded = foldPlanUpcoming(
    planWith({}, [{
      id: 's1', type: 5, name: 'S', icon: 'i', currencyId: 'usd', ownerUserId: 'u1', isArchived: 0, position: 0,
      cells: [
        { actual: '40', planned: '100', closingBalance: '1040', upcoming: '60' },
        { actual: '0', planned: '100', closingBalance: '1040', upcoming: '60' },
        { actual: '0', planned: '', closingBalance: '1040', upcoming: '0' },
      ],
    }]),
    SEPTEMBER,
  )
  const cells = folded.structure.savings![0].cells
  expect(cells.map((c) => c.actual)).toEqual(['100', '60', '0'])
  expect(cells.map((c) => c.closingBalance)).toEqual(['1100', '1160', '1160'])
})

it('a window starting after the current month: each savings row opens on its upcoming balance, and the rows add up to the savings balance line', () => {
  const row = (id: string, closing: string, opening: string, upcoming: string[]): PlanSavingsElementDto => ({
    id, type: 5, name: id, icon: 'i', currencyId: 'usd', ownerUserId: 'u1', isArchived: 0, position: 0,
    upcomingOpeningBalance: opening,
    cells: upcoming.map((u) => ({ actual: '0', planned: '', closingBalance: closing, upcoming: u })),
  })
  const folded = foldPlanUpcoming(
    planWith(
      {
        savingsFlows: [],
        savingsOpeningBalances: [{ currencyId: 'usd', amount: '1000' }],
        upcomingSavingsOpeningBalances: [{ currencyId: 'usd', amount: '150' }],
        upcomingSavingsFlows: [{ month: '2026-10-01', currencyId: 'usd', amount: '150' }, { month: '2026-11-01', currencyId: 'usd', amount: '150' }],
      },
      [row('s1', '600', '150', ['150', '150', '0']), row('s2', '400', '0', ['0', '0', '0'])],
    ),
    SEPTEMBER,
  )
  const rows = folded.structure.savings!
  expect(rows[0].cells.map((c) => c.closingBalance)).toEqual(['900', '1050', '1050'])
  expect(rows[1].cells.map((c) => c.closingBalance)).toEqual(['400', '400', '400'])

  const now = new Date(2026, 8, 15)
  const ex = makePlanExchange(folded, [{ id: 'usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }])
  const line = savingsBalanceRow(folded, planTotals(folded, ex, now), ex, now)
  expect(line).toEqual(folded.months.map((_, i) => rows.reduce((acc, r) => add(acc, r.cells[i].closingBalance!), '0')))
})

it('an upcoming-only currency joins the opening balances by the server order rule', () => {
  const base = planWith({ openingBalances: [{ currencyId: 'eur', amount: '5' }] })
  const folded = foldPlanUpcoming(
    {
      ...base,
      upcomingOpeningBalances: [{ currencyId: 'gbp', amount: '1' }, { currencyId: 'usd', amount: '2' }, { currencyId: 'eur', amount: '0' }],
    },
    SEPTEMBER,
  )
  expect(folded.openingBalances).toEqual([
    { currencyId: 'usd', amount: '2' },
    { currencyId: 'eur', amount: '5' },
    { currencyId: 'gbp', amount: '1' },
  ])
})

it('the plan\'s current month keeps its actuals real; later months fold, and the projections still count the current month', () => {
  const folded = foldPlanUpcoming(
    planWith(
      {
        upcomingTransfers: [{ period: '2026-10-01', items: [{ currencyId: 'usd', in: '100', out: '0' }] }],
        upcomingSavingsFlows: [{ month: '2026-10-01', currencyId: 'usd', amount: '60' }],
      },
      [{
        id: 's1', type: 5, name: 'S', icon: 'i', currencyId: 'usd', ownerUserId: 'u1', isArchived: 0, position: 0,
        cells: [
          { actual: '40', planned: '100', closingBalance: '1040', upcoming: '60' },
          { actual: '0', planned: '100', closingBalance: '1040', upcoming: '60' },
          { actual: '0', planned: '', closingBalance: '1040', upcoming: '0' },
        ],
      }],
    ),
    OCTOBER,
  )
  const el = folded.structure.elements[0]
  expect(el.cells.map((c) => c.actual)).toEqual(['5', '7.5', '0'])
  expect(el.cells[0].upcoming).toBe('20')
  expect(el.children[0].cells.map((c) => c.actual)).toEqual(['5', '7.5', '0'])
  const savings = folded.structure.savings![0].cells
  expect(savings.map((c) => c.actual)).toEqual(['40', '60', '0'])
  // a closing balance is a projection: it counts the current month's upcoming money
  expect(savings.map((c) => c.closingBalance)).toEqual(['1100', '1160', '1160'])
  expect(folded.transfers[0].items).toEqual([{ currencyId: 'usd', in: '110', out: '5' }])
  expect(folded.savingsFlows).toEqual([{ month: '2026-10-01', currencyId: 'usd', amount: '100' }])
})
