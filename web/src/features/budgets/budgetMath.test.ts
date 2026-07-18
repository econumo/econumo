import { bucketElements, bucketStats, budgetTotals, periodRange, widgetMath, makeBudgetExchange, displayAvailable } from './budgetMath'
import { fixtureWireBudget } from '@/test/fixtures'
import type { BudgetDto, BudgetElementDto } from '@/api/dto/budget'

const usd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const eur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 }

// coerced version of the wire fixture
const budget: BudgetDto = JSON.parse(JSON.stringify(fixtureWireBudget))
budget.balances = budget.balances.map((b) => ({
  ...b,
  startBalance: b.startBalance === null ? null : Number(b.startBalance),
  endBalance: b.endBalance === null ? null : Number(b.endBalance),
  income: b.income === null ? null : Number(b.income),
  expenses: b.expenses === null ? null : Number(b.expenses),
  exchanges: b.exchanges === null ? null : Number(b.exchanges),
  holdings: b.holdings === null ? null : Number(b.holdings),
}))
budget.currencyRates = budget.currencyRates.map((r) => ({ ...r, rate: Number(r.rate) }))
budget.structure.elements = budget.structure.elements.map((el) => ({
  ...el,
  spent: Number(el.spent),
  budgetSpent: Number(el.budgetSpent),
  budgeted: Number(el.budgeted),
  available: Number(el.available),
  children: el.children.map((c) => ({ ...c, spent: Number(c.spent), budgetSpent: Number(c.budgetSpent) })),
}))

const exch = makeBudgetExchange(budget, [usd, eur])

it('buckets: folder, folderless, archived (all-zero archived rows hidden)', () => {
  const buckets = bucketElements(budget, exch)
  expect(buckets.withFolder).toHaveLength(1)
  expect(buckets.withFolder[0].folder!.name).toBe('Essentials')
  expect(buckets.withFolder[0].elements.map((e) => e.id)).toEqual(['cat-food'])
  expect(buckets.withoutFolder.elements.map((e) => e.id)).toEqual(['env-1'])
  // tag-old has zero budget, spent and available -> nothing to show in archive
  expect(buckets.archive.elements).toEqual([])
})

it('archive keeps elements with a nonzero budget, spent or available, name-sorted', () => {
  const mutated: BudgetDto = JSON.parse(JSON.stringify(budget))
  const zero = mutated.structure.elements.find((el) => el.id === 'tag-old')!
  mutated.structure.elements.push(
    { ...zero, id: 'tag-carry', name: 'ccc-carry', available: 12 },
    { ...zero, id: 'tag-spent', name: 'aaa-spent', spent: 3 },
    { ...zero, id: 'tag-limit', name: 'bbb-limit', budgeted: 5, available: -5 },
  )
  const buckets = bucketElements(mutated, exch)
  expect(buckets.archive.elements.map((e) => e.id)).toEqual(['tag-spent', 'tag-limit', 'tag-carry'])
})

it('zero folders puts every active element into the no-folder bucket', () => {
  const noFolders: BudgetDto = { ...budget, structure: { folders: [], elements: budget.structure.elements } }
  const buckets = bucketElements(noFolders, exch)
  expect(buckets.withFolder).toEqual([])
  expect(buckets.withoutFolder.elements.map((e) => e.id)).toEqual(['cat-food', 'env-1'])
})

it('stats exchange budgeted/available but never budgetSpent', () => {
  // env-1 is EUR: budgeted 90 EUR -> 100 USD (rate 0.9); budgetSpent stays raw
  const stats = bucketStats([budget.structure.elements[1]], budget, exch)
  expect(stats.budgeted).toBe(100)
  expect(stats.available).toBe(200) // (90 + 90) EUR -> USD
  expect(stats.spent).toBe(0)

  const usdStats = bucketStats([budget.structure.elements[0]], budget, exch)
  expect(usdStats.budgeted).toBe(200)
  expect(usdStats.spent).toBe(45.5)
  expect(usdStats.available).toBe(354.5)
})

it('totals sum all buckets', () => {
  const totals = budgetTotals(bucketElements(budget, exch))
  expect(totals.budgeted).toBe(300)
  expect(totals.spent).toBe(45.5)
  expect(totals.available).toBe(554.5)
})

it('displayAvailable adds budgeted to available', () => {
  expect(displayAvailable({ available: 154.5, budgeted: 200 } as BudgetElementDto)).toBe(354.5)
})

it('periodRange spans 47 months with year-aware labels and start marking', () => {
  const range = periodRange('2026-07-01', '2026-01-01 00:00:00')
  expect(range).toHaveLength(47)
  const active = range.find((i) => i.isActive)!
  expect(active.value).toBe('2026-07-01')
  const before = range.find((i) => i.value === '2025-12-01')!
  expect(before.outsideBudget).toBe(true)
  expect(before.label).toBe('Dec 2025')
  const inside = range.find((i) => i.value === '2026-01-01')!
  expect(inside.outsideBudget).toBe(false)
})

it('widget math folds signed exchanges/holdings and clamps progress', () => {
  const m = widgetMath({ currencyId: 'x', startBalance: 100, endBalance: null, income: 400, expenses: -450, exchanges: -25, holdings: 30 })
  expect(m.spent).toBe(475)
  expect(m.total).toBe(530)
  expect(m.progress).toBeCloseTo(475 / 530)
  expect(m.overspent).toBe(false)

  const nulls = widgetMath({ currencyId: 'x', startBalance: null, endBalance: null, income: null, expenses: null, exchanges: null, holdings: 10 })
  expect(nulls.spent).toBe(0)
  expect(nulls.total).toBe(10)
  expect(nulls.progress).toBe(0)

  const over = widgetMath({ currencyId: 'x', startBalance: 0, endBalance: null, income: 100, expenses: -150, exchanges: 0, holdings: 0 })
  expect(over.overspent).toBe(true)
  expect(over.progress).toBe(1)
})
