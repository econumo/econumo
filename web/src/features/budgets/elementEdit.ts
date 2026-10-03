import type { AccountDto } from '@/api/dto/account'
import { BudgetElementType, UNCATEGORIZED_ID } from '@/api/dto/budget'
import type { Id } from '@/api/types'
import { hasAccountAdminAccess } from '@/features/connections/shared'

export const isEnvelopeType = (type: BudgetElementType): boolean =>
  type === BudgetElementType.ENVELOPE || type === BudgetElementType.INCOME_ENVELOPE

export interface EditableElement {
  id: Id
  type: BudgetElementType
  ownerUserId: Id | null
}

/**
 * Whether the caller may open an element's own edit dialog, by the right the
 * backend enforces on the matching update endpoint: the budget role for an
 * envelope, row ownership for a category or tag (update-category/update-tag
 * answer anyone else with NotFound), an owner or admin grant for a savings
 * account. null: there is nothing to edit (Uncategorized).
 */
export function elementEditAccess(el: EditableElement, userId: Id | undefined, budgetEditable: boolean, accounts: AccountDto[]): boolean | null {
  if (el.id === UNCATEGORIZED_ID) {
    return null
  }
  if (isEnvelopeType(el.type)) {
    return budgetEditable
  }
  if (!userId) {
    return false
  }
  if (el.type === BudgetElementType.SAVINGS) {
    // a deleted account is gone from the list and has no dialog to open
    const account = accounts.find((a) => a.id === el.id)
    return account ? hasAccountAdminAccess(account, userId) : false
  }
  return el.ownerUserId === userId
}
