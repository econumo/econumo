import type { BudgetDto, BudgetElementDto, BudgetPlanDto, PlanSavingsElementDto } from '@/api/dto/budget'
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

it('foldBudgetUpcoming adds upcoming to spent and lowers available', () => {
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
  expect(foldBudgetUpcoming(raw)).toEqual(raw)

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
  expect(foldPlanUpcoming(plan)).toEqual(plan)
})

it('fold results are memoized per data object', () => {
  const raw = budget({ elements: [element({ upcoming: '1', budgetUpcoming: '1', upcomingBefore: '0' })] })
  expect(foldBudgetUpcoming(raw)).toBe(foldBudgetUpcoming(raw))
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
  )
  const cells = folded.structure.savings![0].cells
  expect(cells.map((c) => c.actual)).toEqual(['100', '60', '0'])
  expect(cells.map((c) => c.closingBalance)).toEqual(['1100', '1160', '1160'])
})

it('an upcoming-only currency joins the opening balances by the server order rule', () => {
  const base = planWith({ openingBalances: [{ currencyId: 'eur', amount: '5' }] })
  const folded = foldPlanUpcoming({
    ...base,
    upcomingOpeningBalances: [{ currencyId: 'gbp', amount: '1' }, { currencyId: 'usd', amount: '2' }, { currencyId: 'eur', amount: '0' }],
  })
  expect(folded.openingBalances).toEqual([
    { currencyId: 'usd', amount: '2' },
    { currencyId: 'eur', amount: '5' },
    { currencyId: 'gbp', amount: '1' },
  ])
})
