import { BudgetElementType, isIncomeType } from '@/api/dto/budget'
import { add, cmp, isZero } from '@/lib/decimal'
import { currentPeriod } from './period'

export type PlanHintKind = 'expense' | 'income' | 'savings'
export type PlanHint = { kind: 'short' } | null

export function planHintKind(type: BudgetElementType): PlanHintKind {
  if (type === BudgetElementType.SAVINGS) return 'savings'
  if (isIncomeType(type)) return 'income'
  return 'expense'
}

/** what the month needs: spent so far plus what is still due (absent = 0) */
export function planNeed(spent: string, upcoming: string | undefined): string {
  return upcoming === undefined ? spent : add(spent, upcoming)
}

/** planned: '' or a decimal (element currency); spent: real money so far; upcoming: still due (absent = 0).
 *  An empty plan stays empty: only an expense plan below what the month needs is marked. */
export function planHint(input: { planned: string; spent: string; upcoming?: string; kind: PlanHintKind }): PlanHint {
  const { planned, spent, upcoming, kind } = input
  if (kind !== 'expense' || upcoming === undefined || isZero(upcoming) || planned === '') return null
  return cmp(planned, planNeed(spent, upcoming)) < 0 ? { kind: 'short' } : null
}

/** whether a plan editor lists the cell's money still to come: never in a past month, nor with nothing spent or due.
 *  A savings cell's figures are net, so a deposit and a withdrawal can cancel out: it always asks. */
export function wantsUpcomingPanel(month: string, spent: string, upcoming: string | undefined, savings = false, startDay = 1): boolean {
  return month >= currentPeriod(startDay) && (savings || !isZero(spent) || (upcoming !== undefined && !isZero(upcoming)))
}

export function planHintClass(hint: PlanHint): string {
  return hint?.kind === 'short' ? 'text-expense' : ''
}
