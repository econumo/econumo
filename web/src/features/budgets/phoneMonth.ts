import { isIncomeType, isPlannedType } from '@/api/dto/budget'
import type { BudgetElementType } from '@/api/dto/budget'
import type { BudgetElementDto, BudgetPlanDto, BudgetSavingsElementDto, PlanElementDto } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'
import { isZero } from '@/lib/decimal'
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

export interface PlanMonthFigures {
  month: string
  index: number
  income: { rows: PlanCellFigures[]; planned: string; received: string }
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
  const rows = [...income.folders.flatMap((f) => f.rows), ...income.loose].map((r) => planCellFigures(r.element, index))
  const uncategorized = income.uncategorized?.element
  if (uncategorized && received(uncategorized)) {
    rows.push(planCellFigures(uncategorized, index))
  }
  // the received total counts archived rows too, so the ones with money this month must be listed
  for (const { element } of buckets.archived) {
    if (isIncomeType(element.type) && received(element)) {
      rows.push(planCellFigures(element, index))
    }
  }

  return {
    month,
    index,
    income: { rows, planned: totals[index].incomePlanned, received: totals[index].incomeActual },
    balance: balance[index],
    savingsBalance: savings ? savings[index] : null,
    transfersNet: totals[index].transfersNet,
  }
}

export type SheetTarget =
  | { kind: 'expense'; element: BudgetElementDto }
  | { kind: 'savings'; row: BudgetSavingsElementDto }
  | { kind: 'plan'; cell: PlanCellFigures }

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

export function sheetElement(target: SheetTarget): SheetElement {
  switch (target.kind) {
    case 'expense':
      return target.element
    case 'savings':
      return { ...target.row, children: [] }
    case 'plan':
      return target.cell.element
  }
}
