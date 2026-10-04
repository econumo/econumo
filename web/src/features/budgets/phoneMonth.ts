import { isIncomeType, isPlannedType } from '@/api/dto/budget'
import type { BudgetElementType } from '@/api/dto/budget'
import type { BudgetElementDto, BudgetPlanDto, BudgetSavingsElementDto, LabelSpendDto, PlanElementDto } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'
import { add, isZero } from '@/lib/decimal'
import {
  balanceRow,
  bucketPlanRows,
  everydayBalanceRow,
  makePlanExchange,
  planHasSavingsData,
  planTotals,
  savingsBalanceRow,
} from './planMath'

export interface PlanCellFigures {
  element: PlanElementDto
  planned: string
  actual: string
  closingBalance?: string
}

/** One group of income rows, in the Plan grid's order: each folder, then the
 *  folder-less rows, Uncategorized, and archived rows with money this month. Only
 *  folders carry a name; each view labels the other kinds itself. */
export interface IncomeGroup {
  kind: 'folder' | 'loose' | 'uncategorized' | 'archived'
  id: string
  name: string | null
  rows: PlanCellFigures[]
  /** in the budget currency */
  planned: string
  received: string
}

export interface PlanMonthFigures {
  month: string
  index: number
  income: {
    /** every listed row in order (the phone's flat list) */
    rows: PlanCellFigures[]
    /** the same rows grouped as the Plan grid groups them; empty groups are left out */
    groups: IncomeGroup[]
    planned: string
    received: string
  }
  balance: string
  savingsBalance: string | null
  transfersNet: string
}

export function planCellFigures(element: PlanElementDto, index: number): PlanCellFigures {
  const cell = element.cells[index]
  return {
    element,
    planned: cell && cell.planned !== '' ? cell.planned : '0',
    actual: cell?.actual ?? '0',
    ...(cell?.closingBalance !== undefined ? { closingBalance: cell.closingBalance } : {}),
  }
}

/** One month of the Plan view's figures for the phone view: the same functions, so
 *  its income, Balance and Total savings always agree with the Plan view's lines. */
export function planMonthFigures(plan: BudgetPlanDto, currencies: CurrencyDto[], month: string, now?: Date): PlanMonthFigures | null {
  const index = plan.months.indexOf(month)
  if (index === -1) {
    return null
  }
  const ex = makePlanExchange(plan, currencies)
  const totals = planTotals(plan, ex, now)
  const combined = balanceRow(plan, totals, ex, now)
  const savings = planHasSavingsData(plan) ? savingsBalanceRow(plan, totals, ex, now) : null
  const balance = savings ? everydayBalanceRow(combined, savings) : combined

  const buckets = bucketPlanRows(plan, false)
  const income = buckets.income
  const received = (el: PlanElementDto) => !isZero(el.cells[index]?.actual ?? '0')
  const inBase = (cells: PlanCellFigures[], pick: (c: PlanCellFigures) => string) =>
    cells.reduce((sum, c) => add(sum, ex(c.element.currencyId, pick(c), index)), '0')
  const group = (kind: IncomeGroup['kind'], id: string, name: string | null, elements: PlanElementDto[]): IncomeGroup => {
    const cells = elements.map((el) => planCellFigures(el, index))
    return { kind, id, name, rows: cells, planned: inBase(cells, (c) => c.planned), received: inBase(cells, (c) => c.actual) }
  }
  const uncategorized = income.uncategorized?.element
  const groups = [
    ...income.folders.map((f) => group('folder', f.folder.id, f.folder.name, f.rows.map((r) => r.element))),
    group('loose', '__no_folder__', null, income.loose.map((r) => r.element)),
    group('uncategorized', '__uncategorized__', null, uncategorized && received(uncategorized) ? [uncategorized] : []),
    // the received total counts archived rows too, so the ones with money this month must be listed
    group(
      'archived',
      '__archive__',
      null,
      buckets.archived.filter(({ element }) => isIncomeType(element.type) && received(element)).map(({ element }) => element),
    ),
  ].filter((g) => g.rows.length > 0)
  const rows = groups.flatMap((g) => g.rows)

  return {
    month,
    index,
    income: { rows, groups, planned: totals[index].incomePlanned, received: totals[index].incomeActual },
    balance: balance[index],
    savingsBalance: savings ? savings[index] : null,
    transfersNet: totals[index].transfersNet,
  }
}

export type SheetTarget =
  | { kind: 'expense'; element: BudgetElementDto }
  | { kind: 'savings'; row: BudgetSavingsElementDto }
  | { kind: 'plan'; cell: PlanCellFigures }
  | { kind: 'label'; label: LabelSpendDto }

export interface SheetCell {
  id: Id
  name: string
  currencyId: Id
  amount: string
}

export function sheetCell(target: SheetTarget, baseCurrencyId: Id): SheetCell {
  switch (target.kind) {
    case 'expense':
      return { id: target.element.id, name: target.element.name, currencyId: target.element.currencyId ?? baseCurrencyId, amount: target.element.budgeted }
    case 'savings':
      return { id: target.row.id, name: target.row.name, currencyId: target.row.currencyId, amount: target.row.budgeted }
    case 'plan':
      return { id: target.cell.element.id, name: target.cell.element.name, currencyId: target.cell.element.currencyId, amount: target.cell.planned }
    case 'label':
      // a reporting tag has no amount to set; its spend is already in the budget currency
      return { id: target.label.id, name: target.label.name, currencyId: baseCurrencyId, amount: '0' }
  }
}

export function sheetSetsPlan(target: SheetTarget): boolean {
  switch (target.kind) {
    case 'expense':
      return false
    case 'savings':
      return true
    case 'plan':
      return isPlannedType(target.cell.element.type)
    case 'label':
      return false
  }
}

/** the element a sheet describes, in the shape its edit dialog takes */
export interface SheetElement {
  id: Id
  type: BudgetElementType
  name: string
  icon: string
  ownerUserId: Id | null
  currencyId: Id | null
  isArchived: 0 | 1
  children: { id: Id }[]
}

export function sheetIcon(target: SheetTarget): string {
  switch (target.kind) {
    case 'expense':
      return target.element.icon
    case 'savings':
      return target.row.icon
    case 'plan':
      return target.cell.element.icon
    case 'label':
      return target.label.icon
  }
}

/** a reporting tag is not a budget element: it edits through its own dialog */
export function sheetElement(target: Exclude<SheetTarget, { kind: 'label' }>): SheetElement {
  switch (target.kind) {
    case 'expense':
      return target.element
    case 'savings':
      return { ...target.row, children: [] }
    case 'plan':
      return target.cell.element
  }
}
