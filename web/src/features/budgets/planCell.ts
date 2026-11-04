import type { PlanCellDto } from '@/api/dto/budget'
import { BudgetElementType, UNCATEGORIZED_ID } from '@/api/dto/budget'
import { isZero } from '@/lib/decimal'
import { isOverspent } from './planMath'
import type { PlanHint } from './planHint'
import { planHint, planHintKind } from './planHint'

export interface PlanCellViewInput {
  type: BudgetElementType
  /** Uncategorized cannot hold a plan, so it never carries a hint */
  id?: string
  cell: PlanCellDto | undefined
  month: string
  /** the caller's current month: later months have no actuals worth showing, earlier ones no plan hint */
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
  /** display only: an expense plan short of spent + the month's upcoming payments */
  hint: PlanHint
  /** debt rows only: payments left after the month, null where the debt does not run */
  paymentsLeft?: number | null
}

/** A zero actual is only worth a mark next to a plan, as "— · 55"; on its own the
 *  month stays blank. `actual` is null where actuals do not show at all. */
export function shownActual(actual: string | null, hasPlan: boolean): Pick<PlanCellView, 'actual' | 'dash'> {
  if (actual === null || !isZero(actual)) {
    return { actual, dash: false }
  }
  return { actual: null, dash: hasPlan }
}

/** An unset plan stays blank; a plan of 0 was set on purpose and shows. */
function shownPlan(planned: string): string | null {
  return planned === '' ? null : planned
}

export function planCellView({ type, id, cell, month, selected }: PlanCellViewInput): PlanCellView {
  if (!cell) {
    return { actual: null, dash: false, plan: null, over: false, balance: false, hint: null }
  }
  // a debt shows what is still owed at the month's end (projected ahead), as savings
  // shows its balance; outside its life, with nothing owed, only a payment made then
  // shows. Its one hint: a plan below the month's scheduled dues.
  if (type === BudgetElementType.DEBT) {
    const runs = cell.isPlannable !== 0 || !isZero(cell.closingBalance ?? '0')
    const plan = shownPlan(cell.planned)
    const debtHint = month < selected ? null : planHint({ planned: cell.planned, spent: cell.actual, scheduled: cell.scheduledTotal, month, kind: 'debt' })
    if (!runs || cell.closingBalance === undefined) {
      const paid = month <= selected ? cell.actual : null
      return { ...shownActual(paid, plan !== null), plan, over: false, balance: false, hint: debtHint, paymentsLeft: null }
    }
    return { actual: cell.closingBalance, dash: false, plan, over: false, balance: true, hint: debtHint, paymentsLeft: cell.paymentsLeft ?? null }
  }
  const past = month <= selected
  // the hint compares the plan with what the month needs (saved, for savings), never a balance
  const hint = month < selected || id === UNCATEGORIZED_ID ? null : planHint({ planned: cell.planned, spent: cell.actual, upcoming: cell.upcoming, kind: planHintKind(type) })
  // where a savings account stands says more than what went in that month; ahead of
  // today it is the projection (unmet plans still to come), flat without a plan
  if (type === BudgetElementType.SAVINGS && cell.closingBalance !== undefined) {
    return { actual: cell.closingBalance, dash: false, plan: shownPlan(cell.planned), over: false, balance: true, hint }
  }
  const actual = past ? cell.actual : null
  const plan = shownPlan(cell.planned)
  return {
    ...shownActual(actual, plan !== null),
    plan,
    over: past && type !== BudgetElementType.SAVINGS && isOverspent(type, cell),
    balance: false,
    hint,
  }
}
