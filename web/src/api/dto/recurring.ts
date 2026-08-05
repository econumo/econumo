import type { Id } from '../types'
import type { AccountDto } from './account'
import type { CreateTransactionDto, TransactionDto, TransactionType } from './transaction'

export type RecurringSchedule = 'weekly' | 'biweekly' | 'monthly' | 'quarterly' | 'yearly'

export interface CreateRecurringDto {
  id: Id
  type: TransactionType
  accountId: Id
  accountRecipientId: Id | null
  amount: string
  categoryId: Id | null
  payeeId: Id | null
  tagId: Id | null
  /** full replacement set, exactly as CreateTransactionDto.labelIds */
  labelIds: Id[]
  description: string
  schedule: RecurringSchedule
  nextPaymentAt: string
  /** create only: an existing transaction the template is made FROM — the
      backend links it to the new template (its recurringId) in the same write */
  sourceTransactionId?: Id
}

export interface RecurringDto extends CreateRecurringDto {
  ownerUserId: Id
}

/** labelIds is optional here, and absent differs from empty:
 *  omitted  -> the created transaction inherits the TEMPLATE's labels
 *  a list   -> it gets exactly that list
 *  []       -> it gets none
 *  So a surface that lets the user edit the chips must always send the field
 *  (even empty), and one that does not must omit it rather than guess. */
export interface PostRecurringPayload extends Omit<CreateTransactionDto, 'labelIds'> {
  recurringId: Id
  labelIds?: Id[]
}

export interface PostRecurringResult {
  item: TransactionDto
  accounts: AccountDto[]
  nextPaymentAt: string
}
