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
  /** null: nothing planned (blank, never 0), or a zero plan with nothing beside it */
  plan: string | null
  over: boolean
  /** a savings account: `actual` is its month-end balance (projected ahead), not what was saved */
  balance: boolean
}

/** A zero actual is only worth a mark next to a plan, as "— · 55"; on its own the
 *  month stays blank. `actual` is null where actuals do not show at all. */
export function shownActual(actual: string | null, hasPlan: boolean): Pick<PlanCellView, 'actual' | 'dash'> {
  if (actual === null || !isZero(actual)) {
    return { actual, dash: false }
  }
  return { actual: null, dash: hasPlan }
}

/** A zero plan only means something next to a figure: on its own it reads as noise,
 *  so a month with no actual (or a zero one) and a zero plan stays blank. */
function shownPlan(planned: string, actual: string | null): string | null {
  if (planned === '' || (isZero(planned) && (actual === null || isZero(actual)))) {
    return null
  }
  return planned
}

export function planCellView({ type, cell, month, selected }: PlanCellViewInput): PlanCellView {
  if (!cell) {
    return { actual: null, dash: false, plan: null, over: false, balance: false }
  }
  const past = month <= selected
  // where a savings account stands says more than what went in that month; ahead of
  // today it is the projection (unmet plans still to come), flat without a plan
  if (type === BudgetElementType.SAVINGS && cell.closingBalance !== undefined) {
    return { actual: cell.closingBalance, dash: false, plan: shownPlan(cell.planned, cell.closingBalance), over: false, balance: true }
  }
  const actual = past ? cell.actual : null
  const plan = shownPlan(cell.planned, actual)
  return {
    ...shownActual(actual, plan !== null),
    plan,
    over: past && type !== BudgetElementType.SAVINGS && isOverspent(type, cell),
    balance: false,
  }
}
