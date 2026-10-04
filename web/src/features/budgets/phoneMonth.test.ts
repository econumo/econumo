import { coerceBudgetFixture } from '@/test/coerceBudget'
import { fixtureWireBudget, fixtureWirePlan } from '@/test/fixtures'
import type { BudgetPlanDto } from '@/api/dto/budget'
import { cmp } from '@/lib/decimal'
import { planCellFigures, planMonthFigures, sheetCell } from './phoneMonth'

const usd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const eur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 }

// the fixture minus its EUR row, so every figure below is plain USD arithmetic
function usdPlan(): BudgetPlanDto {
  const plan = JSON.parse(JSON.stringify(fixtureWirePlan)) as BudgetPlanDto
  plan.structure.elements = plan.structure.elements.filter((el) => el.id !== 'env-eur')
  return plan
}
const past = new Date(2026, 11, 1)

it('reads the income rows and totals of the selected month', () => {
  const f = planMonthFigures(usdPlan(), [usd, eur], '2026-07-01', past)!
  expect(f.index).toBe(2)
  expect(f.income.rows.map((r) => [r.element.id, r.planned, r.actual])).toEqual([
    ['ie1', '2000', '0'],
    ['cat-freelance', '500', '400'],
  ])
  expect(cmp(f.income.planned, '2500')).toBe(0)
  expect(cmp(f.income.received, '400')).toBe(0)
})

// Freelance moved into an income folder, the way the Plan grid would show it
function planWithIncomeFolder(): BudgetPlanDto {
  const plan = usdPlan()
  plan.structure.folders = [...plan.structure.folders, { id: 'bf-inc', name: 'Side gigs', position: 1 }]
  plan.structure.elements = plan.structure.elements.map((el) => (el.id === 'cat-freelance' ? { ...el, folderId: 'bf-inc' } : el))
  return plan
}

it('groups the income rows as the Plan grid does: folders with their sums, then the folder-less rows', () => {
  const f = planMonthFigures(planWithIncomeFolder(), [usd, eur], '2026-07-01', past)!
  expect(f.income.folders.map((g) => [g.id, g.name, g.rows.map((r) => r.element.id)])).toEqual([['bf-inc', 'Side gigs', ['cat-freelance']]])
  expect(cmp(f.income.folders[0].planned, '500')).toBe(0)
  expect(cmp(f.income.folders[0].received, '400')).toBe(0)
  expect(f.income.loose.map((r) => r.element.id)).toEqual(['ie1'])
  expect(f.income.uncategorized).toBeNull()
  // the flat list keeps the same rows, folders first
  expect(f.income.rows.map((r) => r.element.id)).toEqual(['cat-freelance', 'ie1'])
  expect(planMonthFigures(planWithIncomeFolder(), [usd, eur], '2026-06-01', past)!.income.uncategorized?.element.id).toBe('uncategorized')
})

it('lists the income Uncategorized row only in a month it received something', () => {
  expect(planMonthFigures(usdPlan(), [usd, eur], '2026-06-01', past)!.income.rows.map((r) => r.element.id)).toContain('uncategorized')
  expect(planMonthFigures(usdPlan(), [usd, eur], '2026-07-01', past)!.income.rows.map((r) => r.element.id)).not.toContain('uncategorized')
})

it('an unplanned cell counts as a zero plan', () => {
  const ie1 = usdPlan().structure.elements.find((el) => el.id === 'ie1')!
  expect(planCellFigures(ie1, 0)).toEqual({ element: ie1, planned: '0', actual: '2000' })
  expect(planCellFigures(ie1, 9)).toEqual({ element: ie1, planned: '0', actual: '0' })
})

it('carries a savings cell\'s closing balance', () => {
  const row = { id: 'acc-s1', type: 0, name: 'Rainy day', icon: 'savings', currencyId: 'cur-usd', isArchived: 0, folderId: null, position: 0, ownerUserId: 'u1',
    cells: [{ actual: '40', planned: '100', closingBalance: '1040' }], children: [] } as unknown as Parameters<typeof planCellFigures>[0]
  expect(planCellFigures(row, 0).closingBalance).toBe('1040')
})

it('chains Balance at month end from the opening balance, past months at actuals', () => {
  // opening 500; May +2300 -200; June +2050 -215 -100 transfers; July +400 -190
  const f = planMonthFigures(usdPlan(), [usd, eur], '2026-07-01', past)!
  expect(cmp(f.balance, '4545')).toBe(0)
  expect(f.savingsBalance).toBeNull()
  expect(cmp(f.transfersNet, '0')).toBe(0)
  expect(cmp(planMonthFigures(usdPlan(), [usd, eur], '2026-06-01', past)!.transfersNet, '-100')).toBe(0)
})

it('projects the current month at the larger of plan and actual, as the Plan view does', () => {
  // July current: income 2000 + 500, expenses 250 + 125 + 50 + 5
  const f = planMonthFigures(usdPlan(), [usd, eur], '2026-07-01', new Date(2026, 6, 15))!
  expect(cmp(f.balance, '6405')).toBe(0)
})

it('is null for a month outside the fetched window', () => {
  expect(planMonthFigures(usdPlan(), [usd, eur], '2027-01-01')).toBeNull()
})

it('splits Total savings off Balance when the plan carries savings data', () => {
  const plan = usdPlan()
  plan.structure.savings = [
    {
      id: 'acc-s1', type: 5, name: 'Rainy day', icon: 'savings', currencyId: 'cur-usd', ownerUserId: 'u1', isArchived: 0, position: 0,
      cells: plan.months.map(() => ({ actual: '0', planned: '' })),
    },
  ]
  plan.savingsOpeningBalances = [{ currencyId: 'cur-usd', amount: '1000' }]
  plan.openingBalances = [{ currencyId: 'cur-usd', amount: '1500' }]
  const f = planMonthFigures(plan, [usd, eur], '2026-07-01', past)!
  expect(cmp(f.savingsBalance!, '1000')).toBe(0)
  // combined 1500 + 4045 of activity, minus the 1000 held in savings
  expect(cmp(f.balance, '4545')).toBe(0)
})

it('sheetCell names the id, currency and settable amount of each target kind', () => {
  const budget = coerceBudgetFixture(fixtureWireBudget)
  const food = budget.structure.elements.find((el) => el.id === 'cat-food')!
  expect(sheetCell({ kind: 'expense', element: food }, 'cur-usd')).toEqual({ id: 'cat-food', name: 'Food', currencyId: 'cur-usd', amount: '200' })
  const ie1 = usdPlan().structure.elements.find((el) => el.id === 'ie1')!
  expect(sheetCell({ kind: 'plan', cell: { element: ie1, planned: '2000', actual: '0' } }, 'cur-usd')).toEqual({
    id: 'ie1', name: 'Salaries', currencyId: 'cur-usd', amount: '2000',
  })
  const saving = { id: 'acc-s1', type: 5 as const, name: 'Rainy day', icon: 'savings', currencyId: 'cur-eur', ownerUserId: 'u1', isArchived: 0 as const, position: 0, budgeted: '50', spent: '0', available: '50' }
  expect(sheetCell({ kind: 'savings', row: saving }, 'cur-usd')).toEqual({ id: 'acc-s1', name: 'Rainy day', currencyId: 'cur-eur', amount: '50' })
})

it('lists an archived income row only in a month it received something, so the rows add up to the total', () => {
  const plan = usdPlan()
  plan.structure.elements.push({
    id: 'cat-old-gig', type: 3, name: 'Old gig', icon: 'work', currencyId: 'cur-usd', isArchived: 1,
    folderId: null, position: 9, ownerUserId: 'u1',
    cells: [{ actual: '0', planned: '' }, { actual: '0', planned: '' }, { actual: '70', planned: '' }, { actual: '0', planned: '' }],
    children: [],
  } as unknown as BudgetPlanDto['structure']['elements'][number])
  const july = planMonthFigures(plan, [usd, eur], '2026-07-01', past)!
  expect(july.income.rows.map((r) => [r.element.id, r.actual])).toContainEqual(['cat-old-gig', '70'])
  expect(cmp(july.income.received, '470')).toBe(0)
  expect(planMonthFigures(plan, [usd, eur], '2026-06-01', past)!.income.rows.map((r) => r.element.id)).not.toContain('cat-old-gig')
})
