import type { PlanCellDto } from '@/api/dto/budget'
import { BudgetElementType } from '@/api/dto/budget'
import { isOverspent } from './planMath'

export interface PlanCellViewInput {
  type: BudgetElementType
  cell: PlanCellDto | undefined
  month: string
  /** the month selected in the strip: later months have no actuals worth showing */
  selected: string
}

export interface PlanCellView {
  /** null: no actual is shown in this month */
  actual: string | null
  /** null: nothing planned (blank, never 0) */
  plan: string | null
  over: boolean
}

export function planCellView({ type, cell, month, selected }: PlanCellViewInput): PlanCellView {
  if (!cell) {
    return { actual: null, plan: null, over: false }
  }
  const past = month <= selected
  return {
    actual: past ? cell.actual : null,
    plan: cell.planned === '' ? null : cell.planned,
    over: past && type !== BudgetElementType.SAVINGS && isOverspent(type, cell),
  }
}
