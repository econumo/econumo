import { BudgetElementType, isIncomeType } from '@/api/dto/budget'
import { add, cmp, isZero } from '@/lib/decimal'

export type PlanHintKind = 'expense' | 'income' | 'savings'
export type PlanHint = { kind: 'suggest'; amount: string } | { kind: 'short' } | null

export function planHintKind(type: BudgetElementType): PlanHintKind {
  if (type === BudgetElementType.SAVINGS) return 'savings'
  if (isIncomeType(type)) return 'income'
  return 'expense'
}

export const PLAN_SUGGEST_CLASS = 'italic text-muted-foreground'

/** planned: '' or a decimal (element currency); spent: real money so far; upcoming: still due (absent = 0) */
export function planHint(input: {
  planned: string
  spent: string
  upcoming?: string
  kind: PlanHintKind
}): PlanHint {
  const { planned, spent, upcoming, kind } = input
  if (upcoming === undefined || isZero(upcoming)) return null
  if (kind === 'savings' && cmp(upcoming, '0') <= 0) return null

  const need = add(spent, upcoming)
  if (isZero(planned)) {
    return cmp(need, '0') > 0 ? { kind: 'suggest', amount: need } : null
  }
  return kind === 'expense' && cmp(planned, need) < 0 ? { kind: 'short' } : null
}

export function planHintClass(hint: PlanHint): string {
  if (hint?.kind === 'suggest') return PLAN_SUGGEST_CLASS
  if (hint?.kind === 'short') return 'text-expense'
  return ''
}

/** the figure a plan spot shows: the suggestion while no plan is set */
export function planHintAmount(planned: string, hint: PlanHint): string {
  return hint?.kind === 'suggest' ? hint.amount : planned
}
