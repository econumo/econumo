import type { TFunction } from 'i18next'
import type { DebtDto, DebtRuleDto, DebtRuleFormDto } from '@/api/dto/debt'
import type { Id } from '@/api/types'

/** the filters that pick a debt's payments: a category, and optionally a payee, a tag and a label */
export interface PaymentsFrom {
  categoryId: Id | null
  payeeId: Id | null
  tagId: Id | null
  labelId: Id | null
}

/** a rule being edited; `id` is the stored rule it started from, sent back so
 *  the rule keeps its place (the debt's icon is its first rule's) */
export interface RuleDraft extends PaymentsFrom {
  id?: Id
}

export const MAX_RULES = 10

export const emptyRule = (): RuleDraft => ({ categoryId: null, payeeId: null, tagId: null, labelId: null })

export const ruleDraft = (rule: DebtRuleDto): RuleDraft => ({
  id: rule.id,
  categoryId: rule.categoryId,
  payeeId: rule.payeeId,
  tagId: rule.tagId,
  labelId: rule.labelId,
})

/** a rule as sent; a draft without a category is refused before sending */
export const ruleForm = (rule: RuleDraft): DebtRuleFormDto => ({
  ...(rule.id ? { id: rule.id } : {}),
  categoryId: rule.categoryId as Id,
  payeeId: rule.payeeId,
  tagId: rule.tagId,
  labelId: rule.labelId,
})

const isWhole = (rule: PaymentsFrom) => !rule.payeeId && !rule.tagId && !rule.labelId

/**
 * Why the whole category cannot become a rule of the picked debt, or null.
 * 'covered': the debt already counts payments in it. 'conflict': another debt
 * of the budget has a rule on the category that could match the same
 * payments; a whole rule overlaps every rule on its category, and only an
 * identical (whole) rule of a debt starting on another date is told apart, by
 * the start dates.
 */
export function addRuleBlocker(debts: DebtDto[], debtId: Id, categoryId: Id): 'covered' | 'conflict' | null {
  const picked = debts.find((d) => d.id === debtId)
  if (!picked) {
    return null
  }
  if (picked.rules.some((r) => r.categoryId === categoryId)) {
    return 'covered'
  }
  const conflicts = debts.some(
    (other) =>
      other.id !== debtId &&
      other.budgetId === picked.budgetId &&
      other.rules.some((r) => r.categoryId === categoryId && !(isWhole(r) && other.startDate !== picked.startDate)),
  )
  return conflicts ? 'conflict' : null
}

/** the rule's category, with its owner's name when it is not the viewer's ("Anna's Mortgage") */
export function ruleOwnerLabel(rule: DebtRuleDto, viewerId: Id | undefined, t: TFunction): string {
  return rule.ownerUserId === viewerId ? rule.categoryName : t('debts.rules.owned_category', { owner: rule.ownerName, category: rule.categoryName })
}

const sameKey = (a: PaymentsFrom, b: PaymentsFrom) =>
  a.categoryId === b.categoryId && a.payeeId === b.payeeId && a.tagId === b.tagId && a.labelId === b.labelId

/** whether the edited rules differ from the stored ones (in order) */
export function rulesChanged(before: DebtRuleDto[], after: PaymentsFrom[]): boolean {
  return before.length !== after.length || before.some((rule, i) => !sameKey(rule, after[i]))
}
