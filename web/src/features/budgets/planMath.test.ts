import { describe, expect, it } from 'vitest'
import type { BudgetFolderDto, BudgetPlanDto, PlanElementDto, PlanSavingsElementDto } from '@/api/dto/budget'
import { BudgetElementType } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import { add, sub } from '@/lib/decimal'
import { fixtureWirePlan } from '@/test/fixtures'
import {
  PLAN_ACTIONS_COL_PX,
  PLAN_MIN_MONTH_COL_PX,
  PLAN_NAME_COL_PX,
  addMonths,
  balanceRow,
  bucketPlanRows,
  clampFirstMonth,
  everydayBalanceRow,
  fillTargetCol,
  folderSides,
  formatPlanMonth,
  isOverspent,
  makePlanExchange,
  monthDate,
  monthDiff,
  planHasSavingsData,
  planInitialFirstMonth,
  planMonthExchange,
  planTotals,
  planVisibleCount,
  projectSavingsClosings,
  savingsBalanceRow,
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

function mkSavingsEl(overrides: Partial<PlanSavingsElementDto> & Pick<PlanSavingsElementDto, 'id' | 'name'>): PlanSavingsElementDto {
  return {
    type: BudgetElementType.SAVINGS,
    icon: 'icon',
    currencyId: 'cur-eur',
    ownerUserId: 'u1',
    isArchived: 0,
    position: 0,
    cells: [
      { actual: '0', planned: '' },
      { actual: '0', planned: '' },
    ],
    ...overrides,
  }
}

function mkPlan(overrides: Partial<BudgetPlanDto> = {}): BudgetPlanDto {
  return {
    meta: { id: 'b1', ownerUserId: 'u1', name: 'Plan', startedAt: '2026-01-01 00:00:00', endedAt: '', currencyId: 'cur-usd', isArchived: 0, access: [] },
    months: ['2026-05-01', '2026-06-01'],
    openingBalances: [],
    currencyRates: [],
    transfers: [],
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

  it('formatPlanMonth uses the period-strip wording: bare month this year, "Mon YYYY" otherwise', () => {
    const now = new Date(2026, 7, 16)
    // built from local components, not a raw UTC parse of "YYYY-MM-01"
    expect(formatPlanMonth('2026-07-01', 'en', now)).toBe('July')
    expect(formatPlanMonth('2025-12-01', 'en', now)).toBe('Dec 2025')
    expect(formatPlanMonth('2027-01-01', 'en', now)).toBe('Jan 2027')
    expect(formatPlanMonth('2026-07-01', 'ru', now)).toBe(new Intl.DateTimeFormat('ru', { month: 'long' }).format(new Date(2026, 6, 1)))
  })

  it('monthDiff counts months from a to b', () => {
    expect(monthDiff('2026-01-01', '2026-01-01')).toBe(0)
    expect(monthDiff('2026-01-01', '2026-04-01')).toBe(3)
    expect(monthDiff('2026-04-01', '2026-01-01')).toBe(-3)
    expect(monthDiff('2025-11-01', '2026-02-01')).toBe(3)
  })

  it('planVisibleCount: 3..12 fit, collapse below 3, cap at 12', () => {
    // Derived from the constants so widening a fixed column cannot silently drift.
    // `fixed` is everything that is not a month: name + the row's px-2, plus the
    // leading gap; `month` is a month column plus its own gap.
    const fixed = PLAN_NAME_COL_PX + 16 + 4
    const month = PLAN_MIN_MONTH_COL_PX + 4
    expect(planVisibleCount(fixed + month * 2)).toBe(1) // only 2 fit -> mobile collapse
    expect(planVisibleCount(fixed + month * 3)).toBe(3)
    expect(planVisibleCount(fixed + month * 7 + 50)).toBe(7)
    expect(planVisibleCount(fixed + month * 40)).toBe(12)

    // edit mode adds the actions slot; months must not be measured against space it
    // takes, or they stretch and the window silently narrows
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

  it('clampFirstMonth never starts past the budget end month', () => {
    expect(clampFirstMonth('2026-09-01', '2026-01-01 00:00:00', '2026-06-01 00:00:00')).toBe('2026-06-01')
    // inside the range is untouched, and an absent end month is unbounded
    expect(clampFirstMonth('2026-03-01', '2026-01-01 00:00:00', '2026-06-01 00:00:00')).toBe('2026-03-01')
    expect(clampFirstMonth('2026-09-01', '2026-01-01 00:00:00', '')).toBe('2026-09-01')
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

  it('neutral folders get their own bucket, in position order, joining neither side', () => {
    const f2: BudgetFolderDto = { id: 'f2', name: 'Empty Folder', position: 3 }
    const f3: BudgetFolderDto = { id: 'f3', name: 'Another Empty', position: 1 }
    const plan = mkPlan({ structure: { folders: [f2, f3], elements: [] } })

    const rows = bucketPlanRows(plan, false)

    expect(rows.neutral).toEqual([
      { folder: f3, rows: [] },
      { folder: f2, rows: [] },
    ])
    expect(rows.expense.folders).toEqual([])
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

  it('transfers: in − out per month (each currency at its month rate) folds into Net and Balance', () => {
    const income = mkEl({
      id: 'inc-1',
      type: 3,
      name: 'Income',
      cells: [
        { actual: '1000', planned: '1000' },
        { actual: '0', planned: '1000' },
      ],
    })
    const expense = mkEl({
      id: 'exp-1',
      type: 1,
      name: 'Expense',
      cells: [
        { actual: '300', planned: '300' },
        { actual: '0', planned: '400' },
      ],
    })
    const plan = mkPlan({
      months: ['2026-07-01', '2026-08-01'],
      openingBalances: [{ currencyId: 'cur-usd', amount: '100' }],
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
      transfers: [
        // July: 200 USD out to savings, 40 EUR (= 20 USD at 2:1) back in
        {
          period: '2026-07-01',
          items: [
            { currencyId: 'cur-usd', in: '0', out: '200' },
            { currencyId: 'cur-eur', in: '40', out: '0' },
          ],
        },
        // August: 50 USD in — a future month, still counted (transfers are never planned)
        { period: '2026-08-01', items: [{ currencyId: 'cur-usd', in: '50', out: '0' }] },
      ],
      structure: { folders: [], elements: [income, expense] },
    })
    const ex = makePlanExchange(plan, [usd, eur])
    const now = new Date(2026, 7, 15) // July past, August current

    const totals = planTotals(plan, ex, now)

    expect(totals[0].transfersIn).toBe('20')
    expect(totals[0].transfersOut).toBe('200')
    expect(totals[0].transfersNet).toBe('-180')
    expect(totals[1].transfersNet).toBe('50')

    // Net = income − expenses + transfers, on both the actual and the effective figures;
    // planned never includes transfers
    expect(totals[0].netActual).toBe('520') // 1000 − 300 − 180
    expect(totals[0].effectiveNet).toBe('520')
    expect(totals[0].netPlanned).toBe('700')
    expect(totals[1].effectiveNet).toBe('650') // max(0,1000) − max(0,400) + 50
    expect(totals[1].netPlanned).toBe('600')

    // Balance chains on the transfer-inclusive Net: 100 + 520, then + 650
    expect(balanceRow(plan, totals, ex, now)).toEqual(['620', '1270'])
  })

  it('transfers: a missing or empty month entry counts as zero', () => {
    const plan = mkPlan({
      months: ['2026-07-01', '2026-08-01'],
      transfers: [{ period: '2026-07-01', items: [] }],
    })
    const totals = planTotals(plan, makePlanExchange(plan, [usd]), new Date(2027, 0, 1))
    expect(totals.map((t) => t.transfersNet)).toEqual(['0', '0'])
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
      transfersIn: '0',
      transfersOut: '0',
      transfersNet: '0',
      savingsActual: '0',
      savingsPlanned: '0',
      effectiveSavings: '0',
    })
    // actual includes the archived row's 30; planned excludes it entirely (normal's empty planned is '0')
    expect(totals[1].expenseActual).toBe('30')
    expect(totals[1].expensePlanned).toBe('0')
    // effectiveNet uses the archived row's actual (30), never max(actual, planned) = max(30, 999)
    expect(totals[1].effectiveNet).toBe('-30')
  })
})

describe('savings + net + balance split', () => {
  const months = ['2026-06-01', '2026-07-01', '2026-08-01']
  const now = new Date(2026, 6, 15) // July 2026 -> currentMonth '2026-07-01'; month0 past, month1 current, month2 future

  const eurRate = (period: string) => ({
    period,
    rates: [
      { currencyId: 'cur-usd', baseCurrencyId: 'cur-usd', rate: '1', periodStart: period, periodEnd: period },
      { currencyId: 'cur-eur', baseCurrencyId: 'cur-usd', rate: '2', periodStart: period, periodEnd: period },
    ],
  })

  function buildPlan(): BudgetPlanDto {
    const expense = mkEl({
      id: 'exp-1',
      type: 1,
      name: 'Expense',
      cells: [
        { actual: '200', planned: '200' },
        { actual: '200', planned: '200' },
        { actual: '200', planned: '200' },
      ],
    })
    const savingsA = mkSavingsEl({
      id: 'sav-a',
      name: 'Vacation Fund',
      cells: [
        { actual: '100', planned: '150' }, // past: effective = actual (100), even though planned is higher
        { actual: '300', planned: '400' }, // current: effective = max = 400
        { actual: '0', planned: '400' }, // future: effective = planned = 400 (actual is 0)
      ],
    })
    const savingsArchived = mkSavingsEl({
      id: 'sav-archived',
      name: 'Closed Fund',
      isArchived: 1,
      cells: [
        { actual: '20', planned: '999' },
        { actual: '10', planned: '999' },
        { actual: '0', planned: '999' },
      ],
    })
    return mkPlan({
      months,
      openingBalances: [{ currencyId: 'cur-usd', amount: '2000' }],
      currencyRates: months.map(eurRate),
      structure: { folders: [], elements: [expense], savings: [savingsA, savingsArchived] },
      savingsOpeningBalances: [{ currencyId: 'cur-usd', amount: '1000' }],
      savingsFlows: [{ month: '2026-06-01', currencyId: 'cur-usd', amount: '303' }],
    })
  }

  it('planTotals: savingsActual/savingsPlanned convert EUR->USD per month; archived rows are excluded from savingsPlanned; effectiveSavings follows past=actual / current+future=max, archived always actual', () => {
    const plan = buildPlan()
    const ex = makePlanExchange(plan, [usd, eur])
    const totals = planTotals(plan, ex, now)

    // month 0 (past): savingsA 100/2=50, archived 20/2=10 -> savingsActual 60
    expect(totals[0].savingsActual).toBe('60')
    // savingsPlanned excludes the archived row's 999 plan: 150/2 = 75
    expect(totals[0].savingsPlanned).toBe('75')
    // effectiveSavings past = actual for both rows: 50 + 10 = 60
    expect(totals[0].effectiveSavings).toBe('60')

    // month 1 (current): savingsA 300/2=150, archived 10/2=5 -> savingsActual 155
    expect(totals[1].savingsActual).toBe('155')
    expect(totals[1].savingsPlanned).toBe('200') // 400/2
    // effectiveSavings: max(150,200)=200 + archived actual 5 = 205
    expect(totals[1].effectiveSavings).toBe('205')

    // month 2 (future): savingsA actual 0, archived actual 0 -> savingsActual 0
    expect(totals[2].savingsActual).toBe('0')
    expect(totals[2].savingsPlanned).toBe('200') // 400/2
    // effectiveSavings: planned (actual 0 < planned) = 200 + archived actual 0 = 200
    expect(totals[2].effectiveSavings).toBe('200')
  })

  it('netActual/netPlanned subtract savings; effectiveNet is unchanged (combined contribution, same with or without savings rows)', () => {
    const plan = buildPlan()
    const ex = makePlanExchange(plan, [usd, eur])
    const totals = planTotals(plan, ex, now)

    // netActual = income(0) - expense(200) + transfers(0) - savingsActual
    expect(totals[0].netActual).toBe('-260') // -200 - 60
    expect(totals[1].netActual).toBe('-355') // -200 - 155
    expect(totals[2].netActual).toBe('-200') // -200 - 0

    // netPlanned = income(0) - expensePlanned(200) - savingsPlanned
    expect(totals[0].netPlanned).toBe('-275') // -200 - 75
    expect(totals[1].netPlanned).toBe('-400') // -200 - 200
    expect(totals[2].netPlanned).toBe('-400') // -200 - 200

    const withoutSavings = mkPlan({ ...plan, structure: { ...plan.structure, savings: [] } })
    const totalsWithoutSavings = planTotals(withoutSavings, ex, now)
    expect(totals.map((t) => t.effectiveNet)).toEqual(totalsWithoutSavings.map((t) => t.effectiveNet))
  })

  it('savingsIncomeExpense (income/expense booked on savings accounts, kept out of the category rows) is added back to netActual and effectiveNet', () => {
    const plan = buildPlan()
    const ex = makePlanExchange(plan, [usd, eur])
    const base = planTotals(plan, ex, now)
    const withInterest = mkPlan({
      ...plan,
      savingsIncomeExpense: [
        { month: '2026-06-01', currencyId: 'cur-usd', amount: '12' },
        { month: '2026-07-01', currencyId: 'cur-eur', amount: '-10' },
      ],
    })
    const totals = planTotals(withInterest, ex, now)

    expect(totals[0].netActual).toBe('-248') // -260 + 12
    expect(totals[1].netActual).toBe('-360') // -355 - 10/2
    expect(totals[2].netActual).toBe('-200')
    expect(totals[0].effectiveNet).toBe(add(base[0].effectiveNet, '12'))
    expect(totals[1].effectiveNet).toBe(sub(base[1].effectiveNet, '5'))
    expect(totals[2].effectiveNet).toBe(base[2].effectiveNet)
    // income/expense rows themselves are untouched
    expect(totals.map((t) => t.incomeActual)).toEqual(base.map((t) => t.incomeActual))
    expect(totals.map((t) => t.expenseActual)).toEqual(base.map((t) => t.expenseActual))
  })

  it('savingsBalanceRow: opening + flows(past), + flows + (effectiveSavings - savingsActual)(current and future)', () => {
    const plan = buildPlan()
    const ex = makePlanExchange(plan, [usd, eur])
    const totals = planTotals(plan, ex, now)

    const savings = savingsBalanceRow(plan, totals, ex, now)

    // opening 1000 USD + flows 303 (past month) = 1303
    expect(savings[0]).toBe('1303')
    // + flows 0 + (effectiveSavings 205 - savingsActual 155) = 50 -> 1353
    // (the archived row's 5 is in both terms, so it adds nothing)
    expect(savings[1]).toBe('1353')
    // future: + flows 0 + (effectiveSavings 200 - savingsActual 0) = 200 -> 1553
    expect(savings[2]).toBe('1553')
  })

  it('savingsBalanceRow current month: the gap is per row, so it agrees with the Savings line when one row is under plan and another over', () => {
    // TFSA planned 500, saved 0; RRSP planned 0, saved 300 (all USD, current month).
    // Savings line = max(0, 500) + max(300, 0) = 800.
    // Balance = opening 0 + flows 300 + per-row gap (500 - 0) + (300 - 300) = 800.
    // The aggregate gap max(0, 500 - 300) = 200 would give 500, 300 short.
    const tfsa = mkSavingsEl({
      id: 'sav-tfsa',
      name: 'TFSA',
      currencyId: 'cur-usd',
      cells: [
        { actual: '0', planned: '' },
        { actual: '0', planned: '500' },
        { actual: '0', planned: '' },
      ],
    })
    const rrsp = mkSavingsEl({
      id: 'sav-rrsp',
      name: 'RRSP',
      currencyId: 'cur-usd',
      cells: [
        { actual: '0', planned: '' },
        { actual: '300', planned: '0' },
        { actual: '0', planned: '' },
      ],
    })
    const plan = mkPlan({
      months,
      openingBalances: [{ currencyId: 'cur-usd', amount: '1000' }],
      currencyRates: months.map(eurRate),
      structure: { folders: [], elements: [], savings: [tfsa, rrsp] },
      savingsFlows: [{ month: '2026-07-01', currencyId: 'cur-usd', amount: '300' }],
    })
    const ex = makePlanExchange(plan, [usd, eur])
    const totals = planTotals(plan, ex, now)
    expect(totals[1].effectiveSavings).toBe('800')

    const savings = savingsBalanceRow(plan, totals, ex, now)
    expect(savings).toEqual(['0', '800', '800'])
    // combined stays at the opening 1000 (savings never change the combined total)
    expect(everydayBalanceRow(balanceRow(plan, totals, ex, now), savings)).toEqual(['1000', '200', '200'])
  })

  it('savingsBalanceRow future month: a future-dated everyday->savings transfer above plan counts once, at its booked amount', () => {
    // future month: planned 50, an everyday->savings transfer of 120 already booked.
    // The transfer is both the row's actual (120 -> effective 120) and the account's
    // flow (+120), so the month adds flows 120 + (effective 120 - actual 120) = 120,
    // not 240. It moves nothing out of the budget: combined stays 1000, and the
    // everyday Balance drops by exactly the 120 the everyday account sent.
    const fund = mkSavingsEl({
      id: 'sav-fund',
      name: 'Fund',
      currencyId: 'cur-usd',
      cells: [
        { actual: '0', planned: '' },
        { actual: '0', planned: '' },
        { actual: '120', planned: '50' },
      ],
    })
    const plan = mkPlan({
      months,
      openingBalances: [{ currencyId: 'cur-usd', amount: '1000' }],
      currencyRates: months.map(eurRate),
      structure: { folders: [], elements: [], savings: [fund] },
      savingsFlows: [{ month: '2026-08-01', currencyId: 'cur-usd', amount: '120' }],
    })
    const ex = makePlanExchange(plan, [usd, eur])
    const totals = planTotals(plan, ex, now)
    expect(totals[2].effectiveSavings).toBe('120')
    const savings = savingsBalanceRow(plan, totals, ex, now)
    expect(savings).toEqual(['0', '0', '120'])
    expect(everydayBalanceRow(balanceRow(plan, totals, ex, now), savings)).toEqual(['1000', '1000', '880'])
  })

  it('savingsBalanceRow future month: a planned month with no activity adds the plan', () => {
    // future month: planned 250, nothing booked, no flow.
    // flows 0 + (effective max(0, 250) = 250 - actual 0) = 250 on top of the opening 400.
    const fund = mkSavingsEl({
      id: 'sav-fund',
      name: 'Fund',
      currencyId: 'cur-usd',
      cells: [
        { actual: '0', planned: '' },
        { actual: '0', planned: '' },
        { actual: '0', planned: '250' },
      ],
    })
    const plan = mkPlan({
      months,
      openingBalances: [{ currencyId: 'cur-usd', amount: '1000' }],
      currencyRates: months.map(eurRate),
      structure: { folders: [], elements: [], savings: [fund] },
      savingsOpeningBalances: [{ currencyId: 'cur-usd', amount: '400' }],
    })
    const ex = makePlanExchange(plan, [usd, eur])
    const totals = planTotals(plan, ex, now)
    const savings = savingsBalanceRow(plan, totals, ex, now)
    expect(savings).toEqual(['400', '400', '650'])
    // combined 1000 throughout; everyday = 1000 - savings
    expect(everydayBalanceRow(balanceRow(plan, totals, ex, now), savings)).toEqual(['600', '600', '350'])
  })

  it('savingsBalanceRow future month: a savings->external transfer moves the Savings balance, not the everyday Balance', () => {
    // future month: the savings account is planned 100 and also sends 20 to an
    // account outside the budget. That transfer is not "saved" (actual stays 0),
    // but it leaves the budget: transfersNet -20, so combined 1000 -> 980, and the
    // savings account's flow is -20.
    //   savings: opening 400, future + flows -20 + (effective 100 - actual 0) = 480
    //   everyday: 980 - 480 = 500
    // Without the transfer: savings 400 + 100 = 500, combined 1000, everyday 500.
    // Same everyday 500 both ways; the 20 comes off the Savings balance only.
    const fund = mkSavingsEl({
      id: 'sav-fund',
      name: 'Fund',
      currencyId: 'cur-usd',
      cells: [
        { actual: '0', planned: '' },
        { actual: '0', planned: '' },
        { actual: '0', planned: '100' },
      ],
    })
    const base = {
      months,
      openingBalances: [{ currencyId: 'cur-usd', amount: '1000' }],
      currencyRates: months.map(eurRate),
      structure: { folders: [], elements: [], savings: [fund] },
      savingsOpeningBalances: [{ currencyId: 'cur-usd', amount: '400' }],
    }
    const without = mkPlan(base)
    const withTransfer = mkPlan({
      ...base,
      transfers: [{ period: '2026-08-01', items: [{ currencyId: 'cur-usd', in: '0', out: '20' }] }],
      savingsFlows: [{ month: '2026-08-01', currencyId: 'cur-usd', amount: '-20' }],
    })
    const split = (plan: BudgetPlanDto) => {
      const ex = makePlanExchange(plan, [usd, eur])
      const totals = planTotals(plan, ex, now)
      const savings = savingsBalanceRow(plan, totals, ex, now)
      return { savings, everyday: everydayBalanceRow(balanceRow(plan, totals, ex, now), savings) }
    }

    expect(split(without)).toEqual({ savings: ['400', '400', '500'], everyday: ['600', '600', '500'] })
    expect(split(withTransfer)).toEqual({ savings: ['400', '400', '480'], everyday: ['600', '600', '500'] })
  })

  it('savingsBalanceRow converts a non-budget-currency flow with that month\'s rate', () => {
    // EUR rate 2 in June, 4 in July: a 400 EUR flow in July is 100 USD (not 200 at June's rate)
    const july = {
      period: '2026-07-01',
      rates: [
        { currencyId: 'cur-usd', baseCurrencyId: 'cur-usd', rate: '1', periodStart: '2026-07-01', periodEnd: '2026-07-01' },
        { currencyId: 'cur-eur', baseCurrencyId: 'cur-usd', rate: '4', periodStart: '2026-07-01', periodEnd: '2026-07-01' },
      ],
    }
    const rates = [eurRate('2026-06-01'), july, eurRate('2026-08-01')]
    const plan = mkPlan({
      months,
      currencyRates: rates,
      structure: { folders: [], elements: [], savings: [] },
      savingsFlows: [{ month: '2026-07-01', currencyId: 'cur-eur', amount: '400' }],
    })
    const ex = makePlanExchange(plan, [usd, eur])
    const totals = planTotals(plan, ex, now)
    expect(savingsBalanceRow(plan, totals, ex, now)).toEqual(['0', '100', '100'])
  })

  it('everydayBalanceRow splits the combined balance into everyday + savings', () => {
    const plan = buildPlan()
    const ex = makePlanExchange(plan, [usd, eur])
    const totals = planTotals(plan, ex, now)

    const combined = balanceRow(plan, totals, ex, now)
    const savings = savingsBalanceRow(plan, totals, ex, now)
    const everyday = everydayBalanceRow(combined, savings)

    expect(everyday).toEqual(combined.map((c, i) => sub(c, savings[i])))
    // combined: opening 2000 + effectiveNet(-200 each month, unchanged by savings) = 1800, 1600, 1400
    expect(combined).toEqual(['1800', '1600', '1400'])
    // savings from the previous test: 1303, 1353, 1553
    expect(everyday).toEqual(['497', '247', '-153'])
  })

  it('a plan without savings/savingsFlows/savingsOpeningBalances (older server) reads savings totals and balance as zero', () => {
    const plan = mkPlan({ months, structure: { folders: [], elements: [] } })
    const ex = makePlanExchange(plan, [usd])
    const totals = planTotals(plan, ex, now)

    for (const t of totals) {
      expect(t.savingsActual).toBe('0')
      expect(t.savingsPlanned).toBe('0')
      expect(t.effectiveSavings).toBe('0')
    }
    expect(savingsBalanceRow(plan, totals, ex, now)).toEqual(['0', '0', '0'])
  })
})

describe('planHasSavingsData', () => {
  it('is false for a plan with no savings rows, no opening balances and no flows', () => {
    expect(planHasSavingsData(mkPlan())).toBe(false)
    expect(planHasSavingsData(mkPlan({ structure: { folders: [], elements: [], savings: [] } }))).toBe(false)
  })

  it('is true when a savings row exists, even with all-zero cells', () => {
    const savings = mkSavingsEl({ id: 'sav-1', name: 'Rainy day' })
    expect(planHasSavingsData(mkPlan({ structure: { folders: [], elements: [], savings: [savings] } }))).toBe(true)
  })

  it('is true when a savings opening balance is non-zero', () => {
    expect(planHasSavingsData(mkPlan({ savingsOpeningBalances: [{ currencyId: 'cur-usd', amount: '1000' }] }))).toBe(true)
  })

  it('is false when every savings opening balance is zero', () => {
    expect(
      planHasSavingsData(
        mkPlan({ savingsOpeningBalances: [{ currencyId: 'cur-usd', amount: '0' }, { currencyId: 'cur-eur', amount: '0' }] }),
      ),
    ).toBe(false)
  })

  it('is true when a savings flow is non-zero', () => {
    expect(
      planHasSavingsData(mkPlan({ savingsFlows: [{ month: '2026-06-01', currencyId: 'cur-usd', amount: '125' }] })),
    ).toBe(true)
  })

  it('is false when every savings flow is zero', () => {
    expect(
      planHasSavingsData(mkPlan({ savingsFlows: [{ month: '2026-06-01', currencyId: 'cur-usd', amount: '0' }] })),
    ).toBe(false)
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

describe('isOverspent', () => {
  const { CATEGORY, INCOME_CATEGORY } = BudgetElementType

  it('is true when an expense actual clears a set plan', () => {
    expect(isOverspent(CATEGORY, { actual: '222.93', planned: '0' })).toBe(true)
    expect(isOverspent(CATEGORY, { actual: '150.01', planned: '150' })).toBe(true)
  })

  it('treats an unset plan as 0', () => {
    expect(isOverspent(CATEGORY, { actual: '50', planned: '' })).toBe(true)
    expect(isOverspent(CATEGORY, { actual: '0', planned: '' })).toBe(false)
  })

  it('is false at exactly the plan', () => {
    expect(isOverspent(CATEGORY, { actual: '150', planned: '150' })).toBe(false)
  })

  it('never flags an income row', () => {
    expect(isOverspent(INCOME_CATEGORY, { actual: '3000', planned: '2000' })).toBe(false)
    expect(isOverspent(INCOME_CATEGORY, { actual: '300', planned: '' })).toBe(false)
  })

  it('is false for a missing cell', () => {
    expect(isOverspent(CATEGORY, undefined)).toBe(false)
  })

  it('never flags a savings row', () => {
    expect(isOverspent(BudgetElementType.SAVINGS, { actual: '600', planned: '500' })).toBe(false)
  })
})

describe('projectSavingsClosings', () => {
  const months = ['2026-06-01', '2026-07-01', '2026-08-01', '2026-09-01']
  const now = new Date(2026, 6, 15) // current month July

  it('keeps the booked closings of past months, then adds each month\'s unmet plan from the current month on', () => {
    const row = mkSavingsEl({
      id: 'sav',
      name: 'TFSA',
      cells: [
        { actual: '100', planned: '150', closingBalance: '1100' }, // past: a missed plan never arrives
        { actual: '300', planned: '400', closingBalance: '1400' }, // current: 100 still expected
        { actual: '0', planned: '400', closingBalance: '1400' }, // future: 400 more expected
        { actual: '0', planned: '', closingBalance: '1400' },
      ],
    })
    const got = projectSavingsClosings(row, months, now).cells.map((c) => c.closingBalance)
    expect(got.map(Number)).toEqual([1100, 1500, 1900, 1900])
  })

  it('an over-met plan adds nothing; a deleted account and a server without balances are left as sent', () => {
    const over = mkSavingsEl({
      id: 'over',
      name: 'Over',
      cells: months.map(() => ({ actual: '500', planned: '100', closingBalance: '10' })),
    })
    expect(projectSavingsClosings(over, months, now).cells.map((c) => Number(c.closingBalance))).toEqual([10, 10, 10, 10])

    const deleted = mkSavingsEl({
      id: 'gone',
      name: 'Gone',
      isArchived: 1,
      cells: months.map(() => ({ actual: '0', planned: '100', closingBalance: '5' })),
    })
    expect(projectSavingsClosings(deleted, months, now)).toEqual(deleted)

    const legacy = mkSavingsEl({ id: 'old', name: 'Old', cells: months.map(() => ({ actual: '0', planned: '100' })) })
    expect(projectSavingsClosings(legacy, months, now)).toEqual(legacy)
  })
})

it('planMonthExchange converts at the given month\'s rates', () => {
  const plan = JSON.parse(JSON.stringify(fixtureWirePlan)) as BudgetPlanDto
  const usd = { id: 'cur-usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
  const eur = { id: 'cur-eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 }
  const may = planMonthExchange(plan, [usd, eur], 0)
  const aug = planMonthExchange(plan, [usd, eur], 3)
  // the fixture's EUR rate moves from 0.90 (May) to 0.93 (Aug): the two months must differ
  expect(may('cur-eur', 'cur-usd', '100')).not.toBe(aug('cur-eur', 'cur-usd', '100'))
  expect(may('cur-usd', 'cur-usd', '100')).toBe('100')
})
