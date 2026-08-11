import type { BudgetFolderDto, BudgetPlanDto, PlanElementDto } from '@/api/dto/budget'
import { isIncomeType, UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'
import { compareNames } from '@/lib/collate'
import { add, cmp, isZero, sub } from '@/lib/decimal'
import { exchange } from '@/lib/exchange'

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

export const PLAN_NAME_COL_PX = 180
export const PLAN_MIN_MONTH_COL_PX = 110

export function planVisibleCount(containerWidthPx: number): number {
  const fit = Math.floor((containerWidthPx - PLAN_NAME_COL_PX) / PLAN_MIN_MONTH_COL_PX)
  return fit < 3 ? 1 : Math.min(fit, 12)
}

export function clampFirstMonth(firstMonth: string, startedAt: string): string {
  const startMonth = `${startedAt.slice(0, 7)}-01`
  return firstMonth < startMonth ? startMonth : firstMonth
}

export function planInitialFirstMonth(persisted: string | null, startedAt: string, visible: number, now?: Date): string {
  const base = persisted !== null ? persisted : visible === 1 ? currentMonth(now) : addMonths(currentMonth(now), -1)
  return clampFirstMonth(base, startedAt)
}

export interface PlanRow {
  element: PlanElementDto
  hidden: boolean
}
export interface PlanFolderSection {
  folder: BudgetFolderDto
  rows: PlanRow[]
}
export interface PlanRows {
  income: { folders: PlanFolderSection[]; loose: PlanRow[]; uncategorized: PlanRow | null; hiddenCount: number }
  expense: { folders: PlanFolderSection[]; loose: PlanRow[]; uncategorized: PlanRow | null; hiddenCount: number }
  archived: PlanRow[]
}

const isRowHidden = (el: PlanElementDto): boolean => el.cells.every((c) => isZero(c.actual) && c.planned === '')

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
      sides.set(folder.id, 'neutral')
    } else {
      sides.set(folder.id, inFolder.some((el) => sideOf(el) === 'income') ? 'income' : 'expense')
    }
  }
  return sides
}

export function bucketPlanRows(plan: BudgetPlanDto, hideEmpty: boolean): PlanRows {
  const folders = [...plan.structure.folders].sort((a, b) => a.position - b.position)
  const elements = plan.structure.elements

  const archived = elements
    .filter((el) => el.isArchived === 1)
    .map((el) => ({ element: el, hidden: false }))
    .sort((a, b) => compareNames(a.element.name, b.element.name))

  const active = elements.filter((el) => el.isArchived === 0 && el.id !== UNCATEGORIZED_ID)
  const uncategorized = elements.filter((el) => el.isArchived === 0 && el.id === UNCATEGORIZED_ID)
  const uncategorizedFor = (side: Side): PlanRow | null => {
    const el = uncategorized.find((e) => sideOf(e) === side)
    return el ? { element: el, hidden: false } : null
  }

  // A memberless (neutral) folder defaults to the expense area for display.
  const sides = folderSides(plan)
  const folderSide = new Map<string, Side>()
  for (const folder of folders) {
    folderSide.set(folder.id, sides.get(folder.id) === 'income' ? 'income' : 'expense')
  }

  const toRow = (el: PlanElementDto): PlanRow => ({ element: el, hidden: isRowHidden(el) })
  const keep = (rows: PlanRow[]): PlanRow[] => (hideEmpty ? rows.filter((r) => !r.hidden) : rows)
  const countHidden = (rows: PlanRow[]): number => rows.filter((r) => r.hidden).length

  const sectionsFor = (side: Side): { folders: PlanFolderSection[]; loose: PlanRow[]; hiddenCount: number } => {
    let hiddenCount = 0
    const folderSections = folders
      .filter((f) => folderSide.get(f.id) === side)
      .map((folder) => {
        const rows = active
          .filter((el) => el.folderId === folder.id)
          .sort((a, b) => a.position - b.position)
          .map(toRow)
        hiddenCount += countHidden(rows)
        return { folder, rows: keep(rows) }
      })
    const looseRows = active
      .filter((el) => el.folderId === null && sideOf(el) === side)
      .sort((a, b) => a.position - b.position)
      .map(toRow)
    hiddenCount += countHidden(looseRows)
    return { folders: folderSections, loose: keep(looseRows), hiddenCount }
  }

  const income = sectionsFor('income')
  const expense = sectionsFor('expense')

  return {
    income: { ...income, uncategorized: uncategorizedFor('income') },
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
  /** per-element-cell max(actual, planned): the Balance row's contribution */
  effectiveNet: string
}

export type MonthExchange = (fromCurrencyId: string, amount: string, monthIndex: number) => string

export function makePlanExchange(plan: BudgetPlanDto, currencies: CurrencyDto[]): MonthExchange {
  return (from, amount, i) => {
    const monthRates = plan.currencyRates[i]
    const rates = (monthRates?.rates ?? []).map((r) => ({ ...r, updatedAt: r.periodStart }))
    return exchange(from, plan.meta.currencyId, amount, rates, currencies)
  }
}

export function planTotals(plan: BudgetPlanDto, ex: MonthExchange, now?: Date): PlanMonthTotals[] {
  const cur = currentMonth(now)
  const rows = plan.structure.elements
  return plan.months.map((month, i) => {
    let incomeActual = '0'
    let incomePlanned = '0'
    let expenseActual = '0'
    let expensePlanned = '0'
    let effIncome = '0'
    let effExpense = '0'
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
      } else {
        expenseActual = add(expenseActual, actual)
        if (el.isArchived === 0) expensePlanned = add(expensePlanned, planned)
        effExpense = add(effExpense, el.isArchived === 0 ? effective : actual)
      }
    }
    return {
      incomeActual,
      incomePlanned,
      expenseActual,
      expensePlanned,
      netActual: sub(incomeActual, expenseActual),
      netPlanned: sub(incomePlanned, expensePlanned),
      effectiveNet: sub(effIncome, effExpense),
    }
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
