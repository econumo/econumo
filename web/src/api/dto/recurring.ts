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

/** labelIds is inherited but has NO effect here: post-recurring-transaction
 *  always copies the TEMPLATE's labels onto the created transaction and
 *  overwrites whatever the client sent. Seed the form from the template so the
 *  chips show what will actually be saved. */
export interface PostRecurringPayload extends CreateTransactionDto {
  recurringId: Id
}

export interface PostRecurringResult {
  item: TransactionDto
  accounts: AccountDto[]
  nextPaymentAt: string
}
