import type { Id } from '../types'
import type { AccountDto } from './account'
import type { CreateTransactionDto, TransactionDto, TransactionType } from './transaction'

export type RecurringSchedule = 'weekly' | 'biweekly' | 'monthly' | 'quarterly' | 'yearly'

export interface CreateRecurringDto {
  id: Id
  type: TransactionType
  accountId: Id
  accountRecipientId: Id | null
  amount: number
  categoryId: Id | null
  payeeId: Id | null
  tagId: Id | null
  description: string
  schedule: RecurringSchedule
  nextPaymentAt: string
}

export interface RecurringDto extends CreateRecurringDto {
  ownerUserId: Id
}

export interface PostRecurringPayload extends CreateTransactionDto {
  recurringId: Id
}

export interface PostRecurringResult {
  item: TransactionDto
  accounts: AccountDto[]
  nextPaymentAt: string
}
