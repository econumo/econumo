import type { BudgetTransactionsParams } from '@/api/budget'
import { BudgetElementType, UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { Id } from '@/api/types'
import type { BudgetTransactionsTarget } from './BudgetTransactionsDialog'

/** the budget get-transaction-list selector for one row of the budget */
export function budgetTransactionsParams(budgetId: Id, target: BudgetTransactionsTarget, periodStart: string): BudgetTransactionsParams {
  // A category listed under a tag displays the tag-and-category intersection.
  // Sending categoryId alone puts the backend on its "tag_id IS NULL" branch,
  // which returns the complement of what the row shows.
  const isUncategorized = target.id === UNCATEGORIZED_ID
  const parentTagId = target.parent?.type === BudgetElementType.TAG ? target.parent.id : undefined
  // a category listed under a reporting label narrows the same way; labelId is
  // the one filter the backend accepts alongside categoryId/uncategorized
  const parentLabelId = target.parent?.type === 'label' ? target.parent.id : undefined
  return {
    budgetId,
    periodStart,
    // the parent tag only ever applies to a CATEGORY-typed element, so it
    // is folded into that branch rather than spread last, which would
    // silently clobber a TAG-typed element's own tagId
    //
    // The Uncategorized element is presentation-only and has no id the
    // backend can look up, so it is selected by flag instead; it composes
    // with the parent tag exactly as a real category does. Never send
    // uncategorized together with categoryId -- the backend rejects that.
    ...(target.type === BudgetElementType.CATEGORY
      ? {
          ...(isUncategorized ? { uncategorized: true } : { categoryId: target.id }),
          ...(parentTagId ? { tagId: parentTagId } : {}),
          ...(parentLabelId ? { labelId: parentLabelId } : {}),
        }
      : {}),
    ...(target.type === BudgetElementType.TAG ? { tagId: target.id } : {}),
    // labelId is mutually exclusive with categoryId/tagId/envelopeId/uncategorized
    // on the backend (400 if combined); 'label' never equals a real
    // BudgetElementType, so this branch can never fire alongside another one
    ...(target.type === 'label' ? { labelId: target.id } : {}),
    ...(target.type === BudgetElementType.ENVELOPE ? { envelopeId: target.id } : {}),
    // transfers composes with nothing on the backend; like 'label' it can
    // never coincide with another branch here
    ...(target.type === 'transfers' ? { transfers: true } : {}),
    // a savings row's id is its account id; composes with nothing
    ...(target.type === BudgetElementType.SAVINGS ? { accountId: target.id } : {}),
    ...(target.type === BudgetElementType.INCOME_CATEGORY ? { income: true, categoryId: target.id } : {}),
    ...(target.type === BudgetElementType.INCOME_ENVELOPE ? { income: true, envelopeId: target.id } : {}),
  }
}
