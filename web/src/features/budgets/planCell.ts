import type { PlanCellDto } from '@/api/dto/budget'
import { BudgetElementType } from '@/api/dto/budget'
import { isZero } from '@/lib/decimal'
import { isOverspent } from './planMath'

export interface PlanCellViewInput {
  type: BudgetElementType
  cell: PlanCellDto | undefined
  month: string
  /** the month selected in the strip: later months have no actuals worth showing */
  selected: string
}

export interface PlanCellView {
  /** null: no actual figure is shown in this month */
  actual: string | null
  /** nothing happened yet against a plan: a dash stands in the actual's place */
  dash: boolean
  /** null: nothing planned (blank, never 0) */
  plan: string | null
  over: boolean
}

/** A zero actual is only worth a mark next to a plan, as "— · 55"; on its own the
 *  month stays blank. `actual` is null where actuals do not show at all. */
export function shownActual(actual: string | null, hasPlan: boolean): Pick<PlanCellView, 'actual' | 'dash'> {
  if (actual === null || !isZero(actual)) {
    return { actual, dash: false }
  }
  return { actual: null, dash: hasPlan }
}

export function planCellView({ type, cell, month, selected }: PlanCellViewInput): PlanCellView {
  if (!cell) {
    return { actual: null, dash: false, plan: null, over: false }
  }
  const past = month <= selected
  const plan = cell.planned === '' ? null : cell.planned
  return {
    ...shownActual(past ? cell.actual : null, plan !== null),
    plan,
    over: past && type !== BudgetElementType.SAVINGS && isOverspent(type, cell),
  }
}
