import type {
  BudgetFolderDto,
  BudgetPlanDto,
  PlanCellDto,
  PlanElementDto,
  PlanSavingsElementDto,
  PlanSavingsFlowDto,
} from '@/api/dto/budget'
import { BudgetElementType, isIncomeType, UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'
import { compareNames } from '@/lib/collate'
import { add, cmp, isZero, sub } from '@/lib/decimal'
import { exchange } from '@/lib/exchange'
import { periodLabeler } from './budgetMath'

export function addMonths(month: string, delta: number): string {
  const [y, m] = month.split('-').map(Number)
  const d = new Date(y, m - 1 + delta, 1)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-01`
}

export function monthDiff(a: string, b: string): number {
  const [ay, am] = a.split('-').map(Number)
  const [by, bm] = b.split('-').map(Number)
  return (by - ay) * 12 + (bm - am)
}

export function currentMonth(now: Date = new Date()): string {
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-01`
}

// A plain "YYYY-MM-01" parses as UTC midnight; formatting that directly in the
// caller's local zone can render the WRONG month west of UTC (e.g. Jun 30 for
// a "2026-07-01" input). Build the Date from local components instead, mirroring
// budgetMath's periodRange, so month formatting is zone-safe everywhere.
export function monthDate(m: string): Date {
  const [y, mo] = m.split('-').map(Number)
  return new Date(y, mo - 1, 1)
}

export function formatPlanMonth(m: string, lang: string, now?: Date): string {
  return periodLabeler(lang, now)(monthDate(m))
}

// the name column's default width; the user can drag it between the bounds below
export const PLAN_NAME_COL_PX = 224
export const PLAN_NAME_COL_MIN_PX = 160
export const PLAN_NAME_COL_MAX_PX = 480

export function clampPlanNameWidth(px: number): number {
  return Math.min(PLAN_NAME_COL_MAX_PX, Math.max(PLAN_NAME_COL_MIN_PX, Math.round(px)))
}
// wide enough for `12,345.67 · 12,345.67` with the cell's padding
export const PLAN_MIN_MONTH_COL_PX = 150
/** PLAN_LINE's pl-2 + pr-2.5; its month cells carry their own padding and no gap */
const PLAN_LINE_PADDING_PX = 18

export function planVisibleCount(containerWidthPx: number, nameWidthPx: number = PLAN_NAME_COL_PX): number {
  const fit = Math.floor((containerWidthPx - nameWidthPx - PLAN_LINE_PADDING_PX) / PLAN_MIN_MONTH_COL_PX)
  return fit < 3 ? 1 : Math.min(fit, 12)
}

/** Excel fill: the column the drag currently targets. Right-only — never
 *  before startCol; clamped to the last visible column; a degenerate
 *  colWidth (<= 0) stays on the source. */
export function fillTargetCol(startCol: number, deltaX: number, colWidth: number, lastCol: number): number {
  if (colWidth <= 0) {
    return startCol
  }
  const target = startCol + Math.round(deltaX / colWidth)
  return Math.min(Math.max(target, startCol), lastCol)
}

/** The Plan grid's first month: the selected month with one month of history before
 *  it, then the future, kept inside the budget's start and end months. */
export function planWindow(selected: string, visible: number, startedAt: string, endedAt?: string | null): string {
  const start = `${startedAt.slice(0, 7)}-01`
  let first = visible > 1 ? addMonths(selected, -1) : selected
  if (endedAt) {
    const lastFirst = addMonths(`${endedAt.slice(0, 7)}-01`, -(visible - 1))
    if (first > lastFirst) {
      first = lastFirst
    }
  }
  if (first < start) {
    first = start
  }
  return first
}

export interface PlanRow {
  element: PlanElementDto
}
export interface PlanFolderSection {
  folder: BudgetFolderDto
  rows: PlanRow[]
}
export interface PlanRows {
  income: { folders: PlanFolderSection[]; loose: PlanRow[]; uncategorized: PlanRow | null }
  /** member-less folders: they belong to neither side yet, so they sit between the
   *  two bands (header-only) until a move gives them one */
  neutral: PlanFolderSection[]
  expense: { folders: PlanFolderSection[]; loose: PlanRow[]; uncategorized: PlanRow | null }
  archived: PlanRow[]
}

// A savings row is never in a folder and has no breakdown, so presenting it as an
// element lets the grid's row, cell editor, fill, keyboard navigation and comment
// marker serve it unchanged.
export function savingsAsPlanElement(s: PlanSavingsElementDto): PlanElementDto {
  return { ...s, folderId: null, children: [] }
}

/** A savings row's closing balances as the plan shows them. The server sends what
 *  is booked; from the current month on, each month's plan not yet met by its actual
 *  is still expected to arrive, so it closes that month and every later one higher.
 *  That is the per-row gap the Savings balance row adds, so the rows and that row
 *  agree. A deleted account plans nothing, and a server that sends no balances is
 *  left alone. */
export function projectSavingsClosings(s: PlanSavingsElementDto, months: string[], now?: Date): PlanSavingsElementDto {
  if (s.isArchived !== 0 || s.cells.some((c) => c.closingBalance === undefined)) {
    return s
  }
  const cur = currentMonth(now)
  let expected = '0'
  const cells = s.cells.map((cell, i) => {
    if ((months[i] ?? '') >= cur) {
      const planned = cell.planned === '' ? '0' : cell.planned
      if (cmp(planned, cell.actual) > 0) {
        expected = add(expected, sub(planned, cell.actual))
      }
    }
    return { ...cell, closingBalance: add(cell.closingBalance ?? '0', expected) }
  })
  return { ...s, cells }
}

/** the overspend highlight: an expense actual past its plan, in ANY month — an
 *  unset plan reads as 0 everywhere else in the grid, so it counts as 0 here too */
export function isOverspent(type: BudgetElementType, cell: PlanCellDto | undefined): boolean {
  if (!cell || isIncomeType(type)) {
    return false
  }
  return cmp(cell.actual, cell.planned === '' ? '0' : cell.planned) > 0
}

type Side = 'income' | 'expense'
const sideOf = (el: PlanElementDto): Side => (isIncomeType(el.type) ? 'income' : 'expense')

export type FolderSide = Side | 'neutral'

// A folder's side follows its members: any income member -> income, any
// expense member -> expense, no members -> neutral. Archived members count
// too, matching the backend's folderSide (internal/budget/move.go) — a
// folder holding only an archived income category is still income-sided.
// Shared with the row-menu "Move to folder…" target list so the two can't
// diverge, and so plan-view bucketing agrees with what the server will accept.
export function folderSides(plan: BudgetPlanDto): Map<Id, FolderSide> {
  const members = plan.structure.elements.filter((el) => el.id !== UNCATEGORIZED_ID)
  const sides = new Map<Id, FolderSide>()
  for (const folder of plan.structure.folders) {
    const inFolder = members.filter((el) => el.folderId === folder.id)
    if (inFolder.length === 0) {
      // an empty folder keeps the side it was created in; only a server older than
      // the stored side leaves it open to both
      sides.set(folder.id, folder.side ?? 'neutral')
    } else {
      sides.set(folder.id, inFolder.some((el) => sideOf(el) === 'income') ? 'income' : 'expense')
    }
  }
  return sides
}

export function bucketPlanRows(plan: BudgetPlanDto): PlanRows {
  const folders = [...plan.structure.folders].sort((a, b) => a.position - b.position)
  const elements = plan.structure.elements

  const archived = elements
    .filter((el) => el.isArchived === 1)
    .map((element) => ({ element }))
    .sort((a, b) => compareNames(a.element.name, b.element.name))

  const active = elements.filter((el) => el.isArchived === 0 && el.id !== UNCATEGORIZED_ID)
  const uncategorized = elements.filter((el) => el.isArchived === 0 && el.id === UNCATEGORIZED_ID)
  const uncategorizedFor = (side: Side): PlanRow | null => {
    const el = uncategorized.find((e) => sideOf(e) === side)
    return el ? { element: el } : null
  }

  const folderSide = folderSides(plan)

  const toRow = (element: PlanElementDto): PlanRow => ({ element })

  const sectionsFor = (side: Side): { folders: PlanFolderSection[]; loose: PlanRow[] } => {
    const folderSections = folders
      .filter((f) => folderSide.get(f.id) === side)
      .map((folder) => ({
        folder,
        rows: active
          .filter((el) => el.folderId === folder.id)
          .sort((a, b) => a.position - b.position)
          .map(toRow),
      }))
    const loose = active
      .filter((el) => el.folderId === null && sideOf(el) === side)
      .sort((a, b) => a.position - b.position)
      .map(toRow)
    return { folders: folderSections, loose }
  }

  const income = sectionsFor('income')
  const expense = sectionsFor('expense')
  const neutral = folders.filter((f) => folderSide.get(f.id) === 'neutral').map((folder) => ({ folder, rows: [] }))

  return {
    income: { ...income, uncategorized: uncategorizedFor('income') },
    neutral,
    expense: { ...expense, uncategorized: uncategorizedFor('expense') },
    archived,
  }
}

export interface PlanMonthTotals {
  incomeActual: string
  incomePlanned: string
  expenseActual: string
  expensePlanned: string
  netActual: string
  netPlanned: string
  /** per-cell max(actual, planned) summed; past months = actual */
  effectiveIncome: string
  /** same accumulation as effectiveIncome, expense side */
  effectiveExpense: string
  /** per-element-cell max(actual, planned): the Balance row's contribution */
  effectiveNet: string
  /** actual uncategorized expense less actual uncategorized income — unassigned
   *  money is only ever real spend, so this line ignores plans entirely */
  uncategorizedActual: string
  /** transfers that crossed the budget boundary this month, in budget currency:
   *  in = moved into included accounts, out = moved out. Never planned. */
  transfersIn: string
  transfersOut: string
  /** in − out: the Transfers line, and a term of Net / Balance */
  transfersNet: string
  /** Σ savings actual, budget currency */
  savingsActual: string
  /** Σ non-archived savings planned, budget currency */
  savingsPlanned: string
  /** per-row max(actual, planned) summed; past months = actual; archived rows = actual */
  effectiveSavings: string
}

export type MonthExchange = (fromCurrencyId: string, amount: string, monthIndex: number) => string

export function makePlanExchange(plan: BudgetPlanDto, currencies: CurrencyDto[]): MonthExchange {
  return (from, amount, i) => {
    const monthRates = plan.currencyRates[i]
    const rates = (monthRates?.rates ?? []).map((r) => ({ ...r, updatedAt: r.periodStart }))
    return exchange(from, plan.meta.currencyId, amount, rates, currencies)
  }
}

export function planMonthExchange(plan: BudgetPlanDto, currencies: CurrencyDto[], monthIndex: number): (from: Id, to: Id, amount: string) => string {
  const rates = (plan.currencyRates[monthIndex]?.rates ?? []).map((r) => ({ ...r, updatedAt: r.periodStart }))
  return (from, to, amount) => exchange(from, to, amount, rates, currencies)
}

export function planTotals(plan: BudgetPlanDto, ex: MonthExchange, now?: Date): PlanMonthTotals[] {
  const cur = currentMonth(now)
  const rows = plan.structure.elements
  const transfersByMonth = new Map((plan.transfers ?? []).map((t) => [t.period, t.items]))
  return plan.months.map((month, i) => {
    // income/expense booked on savings accounts: no category row counts it, but
    // the combined balance moved by it
    const savingsIncomeExpense = (plan.savingsIncomeExpense ?? [])
      .filter((f) => f.month === month)
      .reduce((acc, f) => add(acc, ex(f.currencyId, f.amount, i)), '0')
    let transfersIn = '0'
    let transfersOut = '0'
    for (const tr of transfersByMonth.get(month) ?? []) {
      transfersIn = add(transfersIn, ex(tr.currencyId, tr.in, i))
      transfersOut = add(transfersOut, ex(tr.currencyId, tr.out, i))
    }
    const transfersNet = sub(transfersIn, transfersOut)
    let incomeActual = '0'
    let incomePlanned = '0'
    let expenseActual = '0'
    let expensePlanned = '0'
    let effIncome = '0'
    let effExpense = '0'
    let uncatIncome = '0'
    let uncatExpense = '0'
    for (const el of rows) {
      const cell = el.cells[i]
      if (!cell) {
        continue
      }
      const actual = ex(el.currencyId, cell.actual, i)
      const planned = ex(el.currencyId, cell.planned === '' ? '0' : cell.planned, i)
      const isPast = month < cur
      // per-cell effective value: overspend keeps its actual, underspend keeps its plan
      const effective = isPast ? actual : cmp(actual, planned) >= 0 ? actual : planned
      if (isIncomeType(el.type)) {
        incomeActual = add(incomeActual, actual)
        if (el.isArchived === 0) incomePlanned = add(incomePlanned, planned)
        effIncome = add(effIncome, el.isArchived === 0 ? effective : actual)
        if (el.id === UNCATEGORIZED_ID) uncatIncome = add(uncatIncome, actual)
      } else {
        expenseActual = add(expenseActual, actual)
        if (el.isArchived === 0) expensePlanned = add(expensePlanned, planned)
        effExpense = add(effExpense, el.isArchived === 0 ? effective : actual)
        if (el.id === UNCATEGORIZED_ID) uncatExpense = add(uncatExpense, actual)
      }
    }
    let savingsActual = '0'
    let savingsPlanned = '0'
    let effSavings = '0'
    for (const el of plan.structure.savings ?? []) {
      const cell = el.cells[i]
      if (!cell) {
        continue
      }
      const actual = ex(el.currencyId, cell.actual, i)
      const planned = ex(el.currencyId, cell.planned === '' ? '0' : cell.planned, i)
      const isPast = month < cur
      const effective = isPast ? actual : cmp(actual, planned) >= 0 ? actual : planned
      savingsActual = add(savingsActual, actual)
      // a deleted account's plan arrives only for months it had activity, and
      // counts there as in the monthly Total
      savingsPlanned = add(savingsPlanned, planned)
      effSavings = add(effSavings, el.isArchived === 0 ? effective : actual)
    }
    // Net carries the boundary transfers so the Balance row (which chains on
    // effectiveNet) reflects money that really left or entered the budget's
    // accounts — and Balance[m] − Balance[m−1] stays exactly the Net line.
    // Planned figures never include them: a transfer has no plan. Savings
    // actual/planned are subtracted from Net (money set aside is no longer
    // available in the everyday split), but NOT from effectiveNet: that line
    // is the COMBINED balance's per-month contribution, and an everyday->
    // savings transfer moves nothing between accounts still inside the total.
    return {
      incomeActual,
      incomePlanned,
      expenseActual,
      expensePlanned,
      netActual: sub(add(add(sub(incomeActual, expenseActual), transfersNet), savingsIncomeExpense), savingsActual),
      netPlanned: sub(sub(incomePlanned, expensePlanned), savingsPlanned),
      effectiveIncome: effIncome,
      effectiveExpense: effExpense,
      effectiveNet: add(add(sub(effIncome, effExpense), transfersNet), savingsIncomeExpense),
      uncategorizedActual: sub(uncatExpense, uncatIncome),
      transfersIn,
      transfersOut,
      transfersNet,
      savingsActual,
      savingsPlanned,
      effectiveSavings: effSavings,
    }
  })
}

/** A section's or folder's per-month sums in budget currency, so a line reads as the
 *  sum of the rows listed under it. As in planTotals, every row's actual counts but
 *  only a live row's plan: an archived row or deleted account plans nothing. */
export function planGroupSums(
  rows: PlanElementDto[],
  months: string[],
  monthIndex: (m: string) => number,
  ex: MonthExchange,
  /** 'balance': savings rows add their month-end balance in place of what was saved */
  figure: 'actual' | 'balance' = 'actual',
): { actual: string; planned: string }[] {
  return months.map((m) => {
    const i = monthIndex(m)
    let actual = '0'
    let planned = '0'
    if (i >= 0) {
      for (const el of rows) {
        const cell = el.cells[i]
        if (cell) {
          const figureValue = figure === 'balance' && cell.closingBalance !== undefined ? cell.closingBalance : cell.actual
          actual = add(actual, ex(el.currencyId, figureValue, i))
          // a deleted savings account keeps its plan, as in the Savings total
          if (el.isArchived === 0 || el.type === BudgetElementType.SAVINGS) {
            planned = add(planned, ex(el.currencyId, cell.planned === '' ? '0' : cell.planned, i))
          }
        }
      }
    }
    return { actual, planned }
  })
}

export function balanceRow(plan: BudgetPlanDto, totals: PlanMonthTotals[], ex: MonthExchange, now?: Date): string[] {
  void now
  let running = plan.openingBalances.reduce((acc, b) => add(acc, ex(b.currencyId, b.amount, 0)), '0')
  return totals.map((t) => {
    running = add(running, t.effectiveNet)
    return running
  })
}

/** The savings side of the balance split: opening balance plus, per month, what
 *  actually happened (savingsFlows) in the past; in the current and future months
 *  the flows booked so far plus each row's own gap to plan,
 *  Σ(max(actual, planned) − actual). Future months take the flows too: a future-dated
 *  savings→outside transfer leaves the combined balance through the boundary
 *  transfers, so it must leave the Savings balance as well, or the everyday Balance
 *  would drop for money no everyday account sent. An everyday→savings transfer is
 *  in both the flows and the row's actual, so it still counts once. The gap is per
 *  row so an over-saved row cannot cover another row's shortfall — the balance must
 *  move by exactly what the Savings line shows. */
export function savingsBalanceRow(plan: BudgetPlanDto, totals: PlanMonthTotals[], ex: MonthExchange, now?: Date): string[] {
  const cur = currentMonth(now)
  const flowsByMonth = new Map<string, PlanSavingsFlowDto[]>()
  for (const f of plan.savingsFlows ?? []) {
    const arr = flowsByMonth.get(f.month) ?? []
    arr.push(f)
    flowsByMonth.set(f.month, arr)
  }
  let running = (plan.savingsOpeningBalances ?? []).reduce((acc, b) => add(acc, ex(b.currencyId, b.amount, 0)), '0')
  return plan.months.map((month, i) => {
    const flows = (flowsByMonth.get(month) ?? []).reduce((acc, f) => add(acc, ex(f.currencyId, f.amount, i)), '0')
    const t = totals[i]
    running = add(running, flows)
    if (month >= cur) {
      running = add(running, sub(t.effectiveSavings, t.savingsActual))
    }
    return running
  })
}

/** The everyday side of the balance split: the combined balance minus the savings side. */
export function everydayBalanceRow(combined: string[], savings: string[]): string[] {
  return combined.map((c, i) => sub(c, savings[i] ?? '0'))
}

/** Whether the plan carries ANY savings money, independent of whether a savings row is
 *  currently on screen: a DELETED savings account (still a flagged member, but with no
 *  plan and no activity in the fetched window) drops its row from `structure.savings`,
 *  yet its pre-window balance still arrives in `savingsOpeningBalances` and must not be counted as everyday
 *  money. The balance split therefore keys off this — a row, or a non-zero opening
 *  balance, or a non-zero flow — while the Savings section and its totals line stay tied
 *  to rows alone (there is nothing to list or total without one). */
export function planHasSavingsData(plan: BudgetPlanDto): boolean {
  if ((plan.structure.savings ?? []).length > 0) {
    return true
  }
  if ((plan.savingsOpeningBalances ?? []).some((b) => !isZero(b.amount))) {
    return true
  }
  return (plan.savingsFlows ?? []).some((f) => !isZero(f.amount))
}
