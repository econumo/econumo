import { describe, expect, it } from 'vitest'
import type { BudgetFolderDto, BudgetPlanDto, PlanElementDto } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import { sub } from '@/lib/decimal'
import {
  PLAN_ACTIONS_COL_PX,
  PLAN_CURRENCY_COL_PX,
  PLAN_MIN_MONTH_COL_PX,
  PLAN_NAME_COL_PX,
  addMonths,
  balanceRow,
  bucketPlanRows,
  clampFirstMonth,
  fillTargetCol,
  folderSides,
  formatPlanMonth,
  makePlanExchange,
  monthDate,
  monthDiff,
  planInitialFirstMonth,
  planTotals,
  planVisibleCount,
} from './planMath'

const usd: CurrencyDto = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const eur: CurrencyDto = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 }

function mkEl(overrides: Partial<PlanElementDto> & Pick<PlanElementDto, 'id' | 'type' | 'name'>): PlanElementDto {
  return {
    icon: 'icon',
    currencyId: 'cur-usd',
    isArchived: 0,
    folderId: null,
    position: 0,
    ownerUserId: null,
    cells: [
      { actual: '0', planned: '' },
      { actual: '0', planned: '' },
    ],
    children: [],
    ...overrides,
  }
}

function mkPlan(overrides: Partial<BudgetPlanDto> = {}): BudgetPlanDto {
  return {
    meta: { id: 'b1', ownerUserId: 'u1', name: 'Plan', startedAt: '2026-01-01 00:00:00', currencyId: 'cur-usd', access: [] },
    months: ['2026-05-01', '2026-06-01'],
    openingBalances: [],
    currencyRates: [],
    structure: { folders: [], elements: [] },
    ...overrides,
  }
}

describe('window math', () => {
  it('addMonths crosses years both ways', () => {
    expect(addMonths('2026-01-01', -1)).toBe('2025-12-01')
    expect(addMonths('2026-11-01', 2)).toBe('2027-01-01')
  })

  it('monthDate builds a LOCAL date, immune to the host zone (F1: west-of-UTC month labels)', () => {
    // "new Date('2026-07-01')" parses as UTC midnight; formatting that directly in a
    // zone behind UTC renders the WRONG month (e.g. Jun 30). monthDate must build from
    // local y/m/d components instead — the invariant here is that it always agrees with
    // an explicitly-local `new Date(y, m - 1, 1)`, regardless of what zone the test runs in.
    expect(monthDate('2026-07-01').getTime()).toBe(new Date(2026, 6, 1).getTime())
    expect(monthDate('2026-01-01').getTime()).toBe(new Date(2026, 0, 1).getTime())
  })

  it('formatPlanMonth renders from the local-date path, not a raw UTC parse', () => {
    const label = formatPlanMonth('2026-07-01', 'en')
    expect(label).toBe(new Intl.DateTimeFormat('en', { month: 'short', year: '2-digit' }).format(new Date(2026, 6, 1)))
    expect(label).toContain('Jul')
  })

  it('monthDiff counts months from a to b', () => {
    expect(monthDiff('2026-01-01', '2026-01-01')).toBe(0)
    expect(monthDiff('2026-01-01', '2026-04-01')).toBe(3)
    expect(monthDiff('2026-04-01', '2026-01-01')).toBe(-3)
    expect(monthDiff('2025-11-01', '2026-02-01')).toBe(3)
  })

  it('planVisibleCount: 3..12 fit, collapse below 3, cap at 12', () => {
    // Derived from the constants so widening a fixed column cannot silently drift.
    // `fixed` is everything that is not a month: name + currency track + the row's
    // px-2, plus the leading gap; `month` is a month column plus its own gap.
    const fixed = PLAN_NAME_COL_PX + PLAN_CURRENCY_COL_PX + 16 + 4
    const month = PLAN_MIN_MONTH_COL_PX + 4
    expect(planVisibleCount(fixed + month * 2)).toBe(1) // only 2 fit -> mobile collapse
    expect(planVisibleCount(fixed + month * 3)).toBe(3)
    expect(planVisibleCount(fixed + month * 7 + 50)).toBe(7)
    expect(planVisibleCount(fixed + month * 40)).toBe(12)

    // edit mode widens the tail by the actions slot; months must not be measured
    // against space it takes, or they stretch and the window silently narrows
    expect(planVisibleCount(fixed + month * 8, true)).toBe(7)
    expect(planVisibleCount(fixed + PLAN_ACTIONS_COL_PX + month * 8, true)).toBe(8)
    expect(planVisibleCount(fixed + month * 8)).toBe(8)
  })

  it('planInitialFirstMonth anchors current month second, clamps at start, single-column starts current', () => {
    const now = new Date(2026, 7, 15) // August 2026 -> currentMonth '2026-08-01'
    const startedAt = '2026-01-01 00:00:00'
    // no persisted value, multi-column -> current month minus one
    expect(planInitialFirstMonth(null, startedAt, 3, now)).toBe('2026-07-01')
    // persisted value after the start month is used as-is
    expect(planInitialFirstMonth('2026-03-01', startedAt, 3, now)).toBe('2026-03-01')
    // persisted value before the start month is clamped to the start
    expect(planInitialFirstMonth('2025-11-01', startedAt, 5, now)).toBe('2026-01-01')
    // single visible column with no persisted value starts at the current month
    expect(planInitialFirstMonth(null, startedAt, 1, now)).toBe('2026-08-01')
  })

  it('clampFirstMonth never precedes the budget start month', () => {
    expect(clampFirstMonth('2025-12-01', '2026-01-01 00:00:00')).toBe('2026-01-01')
    expect(clampFirstMonth('2026-05-01', '2026-01-01 00:00:00')).toBe('2026-05-01')
    expect(clampFirstMonth('2026-01-01', '2026-01-01 00:00:00')).toBe('2026-01-01')
  })
})

describe('bucketPlanRows', () => {
  it('income above expenses: income envelope + income category + income uncat in income; folders derive side from members', () => {
    const nonZero = [
      { actual: '100', planned: '' },
      { actual: '0', planned: '' },
    ]
    const f1: BudgetFolderDto = { id: 'f1', name: 'Job', position: 0 }
    const incomeEnvelope = mkEl({ id: 'env-income', type: 4, name: 'Salary Envelope', folderId: 'f1', position: 0, cells: nonZero })
    const expenseCategory = mkEl({ id: 'cat-expense', type: 1, name: 'Groceries', position: 0, cells: nonZero })
    const incomeCategory = mkEl({ id: 'cat-income', type: 3, name: 'Bonus', position: 1, cells: nonZero })
    const uncatIncome = mkEl({ id: 'uncategorized', type: 3, name: 'Uncategorized', position: 2, cells: nonZero })
    const uncatExpense = mkEl({ id: 'uncategorized', type: 1, name: 'Uncategorized', position: 3, cells: nonZero })
    const plan = mkPlan({
      structure: { folders: [f1], elements: [incomeEnvelope, expenseCategory, incomeCategory, uncatIncome, uncatExpense] },
    })

    const rows = bucketPlanRows(plan, false)

    expect(rows.income.folders).toEqual([{ folder: f1, rows: [{ element: incomeEnvelope, hidden: false }] }])
    expect(rows.income.loose).toEqual([{ element: incomeCategory, hidden: false }])
    expect(rows.income.uncategorized).toEqual({ element: uncatIncome, hidden: false })
    expect(rows.expense.folders).toEqual([])
    expect(rows.expense.loose).toEqual([{ element: expenseCategory, hidden: false }])
    expect(rows.expense.uncategorized).toEqual({ element: uncatExpense, hidden: false })
  })

  it('neutral folder renders in the expense area', () => {
    const f2: BudgetFolderDto = { id: 'f2', name: 'Empty Folder', position: 0 }
    const plan = mkPlan({ structure: { folders: [f2], elements: [] } })

    const rows = bucketPlanRows(plan, false)

    expect(rows.expense.folders).toEqual([{ folder: f2, rows: [] }])
    expect(rows.income.folders).toEqual([])
  })

  it('hideEmpty removes all-empty rows and counts them per side; rows with any planned survive', () => {
    const hidden = mkEl({
      id: 'cat-hidden',
      type: 1,
      name: 'Hidden',
      position: 0,
      cells: [
        { actual: '0', planned: '' },
        { actual: '0', planned: '' },
      ],
    })
    // planned '0' (not empty) in month 0 keeps this row visible even though nothing is spent
    const surviving = mkEl({
      id: 'cat-surviving',
      type: 1,
      name: 'Surviving',
      position: 1,
      cells: [
        { actual: '0', planned: '0' },
        { actual: '0', planned: '' },
      ],
    })
    const plan = mkPlan({ structure: { folders: [], elements: [hidden, surviving] } })

    const shown = bucketPlanRows(plan, false)
    expect(shown.expense.loose).toEqual([
      { element: hidden, hidden: true },
      { element: surviving, hidden: false },
    ])
    expect(shown.expense.hiddenCount).toBe(1)

    const filtered = bucketPlanRows(plan, true)
    expect(filtered.expense.loose).toEqual([{ element: surviving, hidden: false }])
    expect(filtered.expense.hiddenCount).toBe(1)
  })

  it('archived rows leave the sections and sort by name', () => {
    const zebra = mkEl({ id: 'cat-zebra', type: 1, name: 'Zebra', isArchived: 1, position: 0, cells: [{ actual: '10', planned: '' }, { actual: '0', planned: '' }] })
    const apple = mkEl({ id: 'env-apple', type: 4, name: 'Apple', isArchived: 1, position: 1, cells: [{ actual: '20', planned: '' }, { actual: '0', planned: '' }] })
    const active = mkEl({ id: 'cat-active', type: 1, name: 'Active', position: 0, cells: [{ actual: '5', planned: '' }, { actual: '0', planned: '' }] })
    const plan = mkPlan({ structure: { folders: [], elements: [zebra, apple, active] } })

    const rows = bucketPlanRows(plan, false)

    expect(rows.archived.map((r) => r.element.id)).toEqual(['env-apple', 'cat-zebra'])
    expect(rows.expense.loose).toEqual([{ element: active, hidden: false }])
    expect(rows.income.loose).toEqual([])
    expect(rows.income.folders).toEqual([])
  })
})

describe('folderSides', () => {
  it('derives income/expense/neutral per folder from members, matching bucketPlanRows', () => {
    const f1: BudgetFolderDto = { id: 'f1', name: 'Job', position: 0 }
    const f2: BudgetFolderDto = { id: 'f2', name: 'Bills', position: 1 }
    const f3: BudgetFolderDto = { id: 'f3', name: 'Empty', position: 2 }
    const incomeEnvelope = mkEl({ id: 'env-income', type: 4, name: 'Salary', folderId: 'f1', position: 0 })
    const expenseCategory = mkEl({ id: 'cat-expense', type: 1, name: 'Rent', folderId: 'f2', position: 0 })
    const archivedIncome = mkEl({ id: 'cat-archived-income', type: 3, name: 'Old bonus', folderId: 'f2', isArchived: 1, position: 1 })
    const plan = mkPlan({ structure: { folders: [f1, f2, f3], elements: [incomeEnvelope, expenseCategory, archivedIncome] } })

    const sides = folderSides(plan)

    expect(sides.get('f1')).toBe('income')
    // an archived income member counts (matches the backend's folderSide), so
    // f2 (an active expense category + an archived income category) reads income
    expect(sides.get('f2')).toBe('income')
    expect(sides.get('f3')).toBe('neutral')
  })

  it('a folder holding only an archived member is still classified by that member (matches the backend rule)', () => {
    const fIncome: BudgetFolderDto = { id: 'f-income', name: 'Old salary', position: 0 }
    const fExpense: BudgetFolderDto = { id: 'f-expense', name: 'Old rent', position: 1 }
    const archivedIncomeOnly = mkEl({ id: 'cat-archived-income-only', type: 3, name: 'Old bonus', folderId: 'f-income', isArchived: 1, position: 0 })
    const archivedExpenseOnly = mkEl({ id: 'cat-archived-expense-only', type: 1, name: 'Old rent', folderId: 'f-expense', isArchived: 1, position: 0 })
    const plan = mkPlan({ structure: { folders: [fIncome, fExpense], elements: [archivedIncomeOnly, archivedExpenseOnly] } })

    const sides = folderSides(plan)

    expect(sides.get('f-income')).toBe('income')
    expect(sides.get('f-expense')).toBe('expense')
  })
})

describe('totals + balance', () => {
  it('planTotals reports uncategorized as actual expense less actual income, ignoring plans', () => {
    const uncatExpense = mkEl({
      id: 'uncategorized',
      type: 1,
      name: 'Uncategorized',
      cells: [
        { actual: '300', planned: '999' },
        { actual: '50', planned: '' },
      ],
    })
    const uncatIncome = mkEl({
      id: 'uncategorized',
      type: 3,
      name: 'Uncategorized',
      cells: [
        { actual: '120', planned: '888' },
        { actual: '80', planned: '' },
      ],
    })
    // a normal categorized pair must not leak into the uncategorized figure
    const normalExpense = mkEl({ id: 'cat-x', type: 1, name: 'X', cells: [{ actual: '1000', planned: '1000' }, { actual: '7', planned: '7' }] })
    const plan = mkPlan({
      months: ['2026-07-01', '2026-08-01'],
      structure: { folders: [], elements: [uncatExpense, uncatIncome, normalExpense] },
    })
    const ex = makePlanExchange(plan, [usd, eur])
    const totals = planTotals(plan, ex, new Date(2027, 0, 1))

    // 300 spend - 120 income = 180; the 999/888 plans are ignored
    expect(totals[0].uncategorizedActual).toBe('180')
    // 50 - 80 = -30: more unassigned income than spend reads negative
    expect(totals[1].uncategorizedActual).toBe('-30')
  })

  it('planTotals converts each month with its own rates (2:1 then 4:1)', () => {
    const eurExpense = mkEl({
      id: 'cat-eur',
      type: 1,
      name: 'Eur Expense',
      currencyId: 'cur-eur',
      cells: [
        { actual: '100', planned: '50' },
        { actual: '200', planned: '100' },
      ],
    })
    const plan = mkPlan({
      months: ['2026-07-01', '2026-08-01'],
      currencyRates: [
        {
          period: '2026-07-01',
          rates: [
            { currencyId: 'cur-usd', baseCurrencyId: 'cur-usd', rate: '1', periodStart: '2026-07-01', periodEnd: '2026-08-01' },
            { currencyId: 'cur-eur', baseCurrencyId: 'cur-usd', rate: '2', periodStart: '2026-07-01', periodEnd: '2026-08-01' },
          ],
        },
        {
          period: '2026-08-01',
          rates: [
            { currencyId: 'cur-usd', baseCurrencyId: 'cur-usd', rate: '1', periodStart: '2026-08-01', periodEnd: '2026-09-01' },
            { currencyId: 'cur-eur', baseCurrencyId: 'cur-usd', rate: '4', periodStart: '2026-08-01', periodEnd: '2026-09-01' },
          ],
        },
      ],
      structure: { folders: [], elements: [eurExpense] },
    })
    const ex = makePlanExchange(plan, [usd, eur])
    const now = new Date(2027, 0, 1) // both months are fully elapsed; irrelevant here, only actual/planned checked

    const totals = planTotals(plan, ex, now)

    // month 0: rate 2:1 -> 100 EUR / 2 = 50 USD actual; 50 EUR / 2 = 25 USD planned
    expect(totals[0].expenseActual).toBe('50')
    expect(totals[0].expensePlanned).toBe('25')
    // month 1: rate 4:1 -> 200 EUR / 4 = 50 USD actual; 100 EUR / 4 = 25 USD planned
    expect(totals[1].expenseActual).toBe('50')
    expect(totals[1].expensePlanned).toBe('25')
  })

  it('effectiveNet: past month = actual net; current/future = per-cell max', () => {
    const over = mkEl({
      id: 'exp-over',
      type: 1,
      name: 'Over',
      cells: [
        { actual: '50', planned: '50' },
        { actual: '120', planned: '100' }, // overspend: actual 120 > planned 100 -> counts 120
      ],
    })
    const under = mkEl({
      id: 'exp-under',
      type: 1,
      name: 'Under',
      cells: [
        { actual: '10', planned: '10' },
        { actual: '10', planned: '50' }, // underspend: actual 10 < planned 50 -> counts 50
      ],
    })
    const income = mkEl({
      id: 'inc-1',
      type: 3,
      name: 'Income',
      cells: [
        { actual: '500', planned: '500' },
        { actual: '900', planned: '1000' }, // max is per cell, not on the totals: counts 1000
      ],
    })
    const plan = mkPlan({
      months: ['2026-07-01', '2026-08-01'],
      structure: { folders: [], elements: [over, under, income] },
    })
    const ex = makePlanExchange(plan, [usd])
    const now = new Date(2026, 7, 15) // currentMonth '2026-08-01' -> month 0 is past, month 1 is current

    const totals = planTotals(plan, ex, now)

    // month 0 (past): effective = actual. income 500 - (50 + 10) = 440
    expect(totals[0].netActual).toBe('440')
    expect(totals[0].effectiveNet).toBe('440')
    // month 1 (current): netActual = 900 - (120 + 10) = 770; netPlanned = 1000 - (100 + 50) = 850
    expect(totals[1].netActual).toBe('770')
    expect(totals[1].netPlanned).toBe('850')
    // effectiveNet = max(900,1000) - (max(120,100) + max(10,50)) = 1000 - (120 + 50) = 830
    expect(totals[1].effectiveNet).toBe('830')
  })

  it('exposes effectiveIncome/effectiveExpense; net is their difference', () => {
    const over = mkEl({
      id: 'exp-over',
      type: 1,
      name: 'Over',
      cells: [
        { actual: '50', planned: '50' },
        { actual: '120', planned: '100' }, // overspend: actual 120 > planned 100 -> counts 120
      ],
    })
    const under = mkEl({
      id: 'exp-under',
      type: 1,
      name: 'Under',
      cells: [
        { actual: '10', planned: '10' },
        { actual: '10', planned: '50' }, // underspend: actual 10 < planned 50 -> counts 50
      ],
    })
    const income = mkEl({
      id: 'inc-1',
      type: 3,
      name: 'Income',
      cells: [
        { actual: '500', planned: '500' },
        { actual: '900', planned: '1000' }, // underspend: actual 900 < planned 1000 -> counts 1000
      ],
    })
    const plan = mkPlan({
      months: ['2026-07-01', '2026-08-01'],
      structure: { folders: [], elements: [over, under, income] },
    })
    const ex = makePlanExchange(plan, [usd])
    const now = new Date(2026, 7, 15) // currentMonth '2026-08-01' -> month 0 is past, month 1 is current

    const totals = planTotals(plan, ex, now)

    // month 0 (past): effective = actual for both sides
    expect(totals[0].effectiveIncome).toBe('500')
    expect(totals[0].effectiveExpense).toBe('60')
    // month 1 (current): per-cell max sums -> income max(900,1000)=1000; expense max(120,100)+max(10,50)=170
    expect(totals[1].effectiveIncome).toBe('1000')
    expect(totals[1].effectiveExpense).toBe('170')

    for (const t of totals) {
      expect(sub(t.effectiveIncome, t.effectiveExpense)).toBe(t.effectiveNet)
    }
  })

  it('balanceRow chains: seed(+FX) + effectiveNet cumulative', () => {
    const over = mkEl({
      id: 'exp-over',
      type: 1,
      name: 'Over',
      cells: [
        { actual: '50', planned: '50' },
        { actual: '120', planned: '100' },
      ],
    })
    const under = mkEl({
      id: 'exp-under',
      type: 1,
      name: 'Under',
      cells: [
        { actual: '10', planned: '10' },
        { actual: '10', planned: '50' },
      ],
    })
    const income = mkEl({
      id: 'inc-1',
      type: 3,
      name: 'Income',
      cells: [
        { actual: '500', planned: '500' },
        { actual: '900', planned: '1000' },
      ],
    })
    const plan = mkPlan({
      months: ['2026-07-01', '2026-08-01'],
      openingBalances: [
        { currencyId: 'cur-usd', amount: '500' },
        { currencyId: 'cur-eur', amount: '100' },
      ],
      currencyRates: [
        {
          period: '2026-07-01',
          rates: [
            { currencyId: 'cur-usd', baseCurrencyId: 'cur-usd', rate: '1', periodStart: '2026-07-01', periodEnd: '2026-08-01' },
            { currencyId: 'cur-eur', baseCurrencyId: 'cur-usd', rate: '2', periodStart: '2026-07-01', periodEnd: '2026-08-01' },
          ],
        },
        {
          period: '2026-08-01',
          rates: [{ currencyId: 'cur-usd', baseCurrencyId: 'cur-usd', rate: '1', periodStart: '2026-08-01', periodEnd: '2026-09-01' }],
        },
      ],
      structure: { folders: [], elements: [over, under, income] },
    })
    const ex = makePlanExchange(plan, [usd, eur])
    const now = new Date(2026, 7, 15)
    const totals = planTotals(plan, ex, now) // effectiveNet: [440, 830] (see previous test's arithmetic)

    const balances = balanceRow(plan, totals, ex, now)

    // seed = 500 USD + (100 EUR / 2 rate) = 500 + 50 = 550
    // balance[0] = 550 + 440 = 990
    // balance[1] = 990 + 830 = 1820
    expect(balances).toEqual(['990', '1820'])
  })

  it('empty planned counts as zero everywhere; archived rows count in actuals only', () => {
    const archived = mkEl({
      id: 'exp-archived',
      type: 1,
      name: 'Archived Expense',
      isArchived: 1,
      cells: [
        { actual: '0', planned: '' },
        { actual: '30', planned: '999' }, // archived: actual counts, planned never counts
      ],
    })
    const normal = mkEl({
      id: 'exp-normal',
      type: 1,
      name: 'Normal',
      cells: [
        { actual: '0', planned: '' },
        { actual: '0', planned: '' }, // empty planned counts as '0'
      ],
    })
    const plan = mkPlan({
      months: ['2026-07-01', '2026-08-01'],
      structure: { folders: [], elements: [archived, normal] },
    })
    const ex = makePlanExchange(plan, [usd])
    const now = new Date(2026, 7, 15) // month 1 ('2026-08-01') is the current month, not past

    const totals = planTotals(plan, ex, now)

    expect(totals[0]).toEqual({
      incomeActual: '0',
      incomePlanned: '0',
      expenseActual: '0',
      expensePlanned: '0',
      netActual: '0',
      netPlanned: '0',
      effectiveIncome: '0',
      effectiveExpense: '0',
      effectiveNet: '0',
      uncategorizedActual: '0',
    })
    // actual includes the archived row's 30; planned excludes it entirely (normal's empty planned is '0')
    expect(totals[1].expenseActual).toBe('30')
    expect(totals[1].expensePlanned).toBe('0')
    // effectiveNet uses the archived row's actual (30), never max(actual, planned) = max(30, 999)
    expect(totals[1].effectiveNet).toBe('-30')
  })
})

describe('fillTargetCol', () => {
  it('rounds the pointer delta to whole columns', () => {
    expect(fillTargetCol(1, 0, 110, 5)).toBe(1)
    expect(fillTargetCol(1, 54, 110, 5)).toBe(1) // < half a column
    expect(fillTargetCol(1, 56, 110, 5)).toBe(2) // past half
    expect(fillTargetCol(1, 275, 110, 5)).toBe(4) // 2.5 -> round -> 3 cols right
  })
  it('never goes left of the source and clamps at the last visible column', () => {
    expect(fillTargetCol(2, -500, 110, 5)).toBe(2)
    expect(fillTargetCol(2, 5000, 110, 5)).toBe(5)
  })
  it('degrades to the source column on zero/negative width', () => {
    expect(fillTargetCol(1, 300, 0, 5)).toBe(1)
    expect(fillTargetCol(1, 300, -10, 5)).toBe(1)
  })
})
