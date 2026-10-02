import { bucketElements, bucketStats, budgetTotals, periodRange, rowState, rowProgress, carryOver, overBudget, makeBudgetExchange, displayAvailable, savingsTotals, totalsWithSavings } from './budgetMath'
import { fixtureWireBudget } from '@/test/fixtures'
import { BudgetElementType } from '@/api/dto/budget'
import type { BudgetDto, BudgetElementDto } from '@/api/dto/budget'

const usd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const eur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 }

const budget: BudgetDto = JSON.parse(JSON.stringify(fixtureWireBudget))

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
    { ...zero, id: 'tag-carry', name: 'ccc-carry', available: '12' },
    { ...zero, id: 'tag-spent', name: 'aaa-spent', spent: '3' },
    { ...zero, id: 'tag-limit', name: 'bbb-limit', budgeted: '5', available: '-5' },
  )
  const buckets = bucketElements(mutated, exch)
  expect(buckets.archive.elements.map((e) => e.id)).toEqual(['tag-spent', 'tag-limit', 'tag-carry'])
})

it('drops the Uncategorized bucket when the selected month has no uncategorized spend', () => {
  const uncatEl: BudgetElementDto = {
    id: 'uncategorized', type: 1, name: 'Uncategorized', icon: 'question_mark', currencyId: null, isArchived: 0,
    folderId: null, position: 9, budgeted: '0', available: '0', spent: '0', budgetSpent: '0',
    ownerUserId: null, children: [],
  }

  // spent '0' (carried by prior-month spend server-side) -> the bucket is empty and
  // budgetTotals is unaffected
  const zeroBudget: BudgetDto = JSON.parse(JSON.stringify(budget))
  zeroBudget.structure.elements.push({ ...uncatEl })
  const zeroBuckets = bucketElements(zeroBudget, exch)
  expect(zeroBuckets.uncategorized.elements).toEqual([])
  expect(budgetTotals(zeroBuckets)).toEqual(budgetTotals(bucketElements(budget, exch)))

  // spent '5' -> present
  const spentBudget: BudgetDto = JSON.parse(JSON.stringify(budget))
  spentBudget.structure.elements.push({ ...uncatEl, spent: '5' })
  const spentBuckets = bucketElements(spentBudget, exch)
  expect(spentBuckets.uncategorized.elements.map((e) => e.id)).toEqual(['uncategorized'])

  // spent '-5' (a refund) -> still present: the filter is cmp !== 0, not > 0
  const refundBudget: BudgetDto = JSON.parse(JSON.stringify(budget))
  refundBudget.structure.elements.push({ ...uncatEl, spent: '-5' })
  const refundBuckets = bucketElements(refundBudget, exch)
  expect(refundBuckets.uncategorized.elements.map((e) => e.id)).toEqual(['uncategorized'])
})

it('zero folders puts every active element into the no-folder bucket', () => {
  const noFolders: BudgetDto = { ...budget, structure: { folders: [], elements: budget.structure.elements, labels: budget.structure.labels } }
  const buckets = bucketElements(noFolders, exch)
  expect(buckets.withFolder).toEqual([])
  expect(buckets.withoutFolder.elements.map((e) => e.id)).toEqual(['cat-food', 'env-1'])
})

it('stats exchange budgeted/available but never budgetSpent', () => {
  // env-1 is EUR: budgeted 90 EUR -> 100 USD (rate 0.9); budgetSpent stays raw
  const stats = bucketStats([budget.structure.elements[1]], budget, exch)
  expect(stats.budgeted).toBe('100')
  expect(stats.available).toBe('200') // (90 + 90) EUR -> USD
  expect(stats.spent).toBe('0')

  const usdStats = bucketStats([budget.structure.elements[0]], budget, exch)
  expect(usdStats.budgeted).toBe('200')
  expect(usdStats.spent).toBe('45.5')
  expect(usdStats.available).toBe('354.5')
})

it('totals sum all buckets', () => {
  const totals = budgetTotals(bucketElements(budget, exch))
  expect(totals.budgeted).toBe('300')
  expect(totals.spent).toBe('45.5')
  expect(totals.available).toBe('554.5')
})

it('totals large amounts exactly', () => {
  // two elements of 9007199254740993 each (beyond Number.MAX_SAFE_INTEGER)
  // must sum to 18014398509481986, which float math cannot represent
  const bigEl = (id: string): BudgetElementDto => ({
    id,
    type: 1,
    name: id,
    icon: 'restaurant',
    currencyId: budget.meta.currencyId,
    isArchived: 0,
    folderId: null,
    position: 0,
    budgeted: '9007199254740993',
    available: '0',
    spent: '0',
    budgetSpent: '0',
    ownerUserId: null,
    children: [],
  })
  const stats = bucketStats([bigEl('big-1'), bigEl('big-2')], budget, (_f, _t, a) => a)
  expect(stats.budgeted).toBe('18014398509481986')
})

it('savingsTotals: savings is actual for a past month, max(planned, saved) per live row from the current month; balance sums closing balances', () => {
  const row = { type: BudgetElementType.SAVINGS, icon: 'savings', ownerUserId: 'u1', isArchived: 0 as const, position: 0, available: '0' }
  const withSavings = (savings: BudgetDto['structure']['savings']): BudgetDto => ({ ...budget, structure: { ...budget.structure, savings } })
  // a fake exchange that doubles anything not already in the budget currency
  const ex = (from: string, to: string, amount: string) => (from === to ? amount : String(Number(amount) * 2))
  const cur = budget.meta.currencyId
  const b = withSavings([
    // under plan: 500 planned, 200 saved
    { ...row, id: 's1', name: 'A', currencyId: cur, budgeted: '500', spent: '200', closingBalance: '100.5' },
    // over plan, other currency: 5 planned, 30 saved
    { ...row, id: 's2', name: 'B', currencyId: 'other', budgeted: '5', spent: '30', closingBalance: '10' },
    // a deleted account never expects its plan
    { ...row, id: 's3', name: 'C', currencyId: cur, isArchived: 1, budgeted: '70', spent: '0', closingBalance: '0' },
  ])
  expect(savingsTotals(b, ex, false)).toEqual({ savings: '260', balance: '120.5' })
  expect(savingsTotals(b, ex, true)).toEqual({ savings: '560', balance: '120.5' })
  // no savings rows: no lines; a server without balances: the savings line only
  expect(savingsTotals(withSavings([]), ex, true)).toBeNull()
  expect(savingsTotals(withSavings(undefined), ex, true)).toBeNull()
  expect(savingsTotals(withSavings([{ ...row, id: 's1', name: 'A', currencyId: cur, budgeted: '1', spent: '2' }]), ex, false)).toEqual({ savings: '2', balance: null })
})

it('displayAvailable adds budgeted to available', () => {
  expect(displayAvailable({ available: '154.5', budgeted: '200' } as BudgetElementDto)).toBe('354.5')
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


it('periodRange marks months after the budget end month with afterEnd', () => {
  const range = periodRange('2026-07-01', '2026-01-01 00:00:00', 6, 6, 'en', '2026-08-01 00:00:00')
  expect(range.find((i) => i.value === '2026-08-01')!.afterEnd).toBe(false)
  expect(range.find((i) => i.value === '2026-09-01')!.afterEnd).toBe(true)
  // the start boundary is a separate flag and still applies in the same call
  expect(range.find((i) => i.value === '2026-01-01')!.outsideBudget).toBe(false)
  const wide = periodRange('2026-07-01', '2026-02-01 00:00:00', 6, 6, 'en', '2026-08-01 00:00:00')
  expect(wide.find((i) => i.value === '2026-01-01')!.outsideBudget).toBe(true)
  expect(wide.find((i) => i.value === '2026-01-01')!.afterEnd).toBe(false)
})

it('periodRange without an end month leaves later months inside', () => {
  const range = periodRange('2026-07-01', '2026-01-01 00:00:00', 6, 6, 'en', '')
  expect(range.find((i) => i.value === '2026-12-01')!.afterEnd).toBe(false)
  expect(range.find((i) => i.value === '2026-12-01')!.outsideBudget).toBe(false)
})

describe('rowState', () => {
  it('is none with no budget and no spending', () => {
    expect(rowState({ budgeted: '0', spent: '0', available: '0' })).toBe('none')
    // even with a carried-over balance: the spec's table checks this row first
    expect(rowState({ budgeted: '0', spent: '0', available: '-20' })).toBe('none')
  })
  it('is ok within budget', () => {
    expect(rowState({ budgeted: '700', spent: '650', available: '50' })).toBe('ok')
    expect(rowState({ budgeted: '700', spent: '700', available: '0' })).toBe('ok')
  })
  it('is covered when over this month but carry-over keeps Available non-negative', () => {
    expect(rowState({ budgeted: '700', spent: '801.37', available: '649.32' })).toBe('covered')
    expect(rowState({ budgeted: '0', spent: '10', available: '5' })).toBe('covered')
  })
  it('is over when Available is negative', () => {
    expect(rowState({ budgeted: '700', spent: '801.37', available: '-101.37' })).toBe('over')
    expect(rowState({ budgeted: '100', spent: '50', available: '-1' })).toBe('over')
  })
  it('is none for a future month whatever the figures', () => {
    expect(rowState({ budgeted: '700', spent: '801.37', available: '-101.37' }, true)).toBe('none')
  })
})

describe('rowProgress', () => {
  it('is spent over budget, capped at 1', () => {
    expect(rowProgress({ budgeted: '200', spent: '50' })).toBe(0.25)
    expect(rowProgress({ budgeted: '200', spent: '500' })).toBe(1)
  })
  it('is null with no budget or in a future month', () => {
    expect(rowProgress({ budgeted: '0', spent: '50' })).toBeNull()
    expect(rowProgress({ budgeted: '200', spent: '50' }, true)).toBeNull()
  })
  it('never goes below zero (refunds)', () => {
    expect(rowProgress({ budgeted: '200', spent: '-30' })).toBe(0)
  })
  it('measures against what earlier months left plus this month’s budget', () => {
    expect(rowProgress({ budgeted: '700', spent: '615', carry: '530' })).toBe(0.5)
    // only budget left over, no budget this month: still a bar
    expect(rowProgress({ budgeted: '0', spent: '50', carry: '200' })).toBe(0.25)
  })
  it('ignores a negative carry-over (earlier overspend) for the bar', () => {
    expect(rowProgress({ budgeted: '200', spent: '50', carry: '-120' })).toBe(0.25)
  })
})

describe('overBudget', () => {
  it('is true only when this month is over budget and earlier months do not cover it', () => {
    expect(overBudget({ budgeted: '100', spent: '150', available: '-20' })).toBe(true)
    // covered by earlier months: not over
    expect(overBudget({ budgeted: '100', spent: '150', available: '10' })).toBe(false)
    // an earlier overspend with this month within budget: not over
    expect(overBudget({ budgeted: '120', spent: '100', available: '-150' })).toBe(false)
    expect(overBudget({ budgeted: '100', spent: '100', available: '0' })).toBe(false)
  })
  it('is false in a future month', () => {
    expect(overBudget({ budgeted: '100', spent: '150', available: '-20' }, true)).toBe(false)
  })
})

it('carryOver is what earlier months left: displayed Available less this months budget less spent', () => {
  // the spec's worked example: Budget 700, Spent 801.37, Available 649.32 -> 750.69
  const el = { budgeted: '700', spent: '801.37', available: '-50.68' }
  expect(displayAvailable(el)).toBe('649.32')
  expect(carryOver(el)).toBe('750.69')
})

it('totals what earlier months left, in the budget currency, without Uncategorized or savings', () => {
  // Food 154.50 + 45.50 = 200 USD; Living 90 + 0 = 90 EUR at 0.9 = 100 USD
  const mutated: BudgetDto = JSON.parse(JSON.stringify(budget))
  mutated.structure.elements.push({
    ...mutated.structure.elements[0], id: 'uncategorized', name: 'Uncategorized', folderId: null,
    budgeted: '0', available: '0', spent: '12', budgetSpent: '12',
  })
  mutated.structure.savings = [
    { id: 'acc-s1', type: 5, name: 'Rainy day', icon: 'savings', currencyId: 'cur-usd', ownerUserId: 'u1', isArchived: 0, position: 0, budgeted: '100', spent: '40', available: '60' },
  ]
  const ex = makeBudgetExchange(mutated, [usd, eur])
  const buckets = bucketElements(mutated, ex)
  expect(buckets.withFolder[0].stats.carry).toBe('200')
  const totals = budgetTotals(buckets)
  expect(Number(totals.carry)).toBeCloseTo(300, 6)
  expect(Number(totalsWithSavings(totals, mutated, ex).carry)).toBeCloseTo(300, 6)
})
