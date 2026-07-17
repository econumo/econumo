import type { Id } from '../types'
import type { UserDto } from './user'
import type { AccountDto } from './account'

export type TransactionType = 'expense' | 'income' | 'transfer'

export interface CreateTransactionDto {
  id: Id
  type: TransactionType
  accountId: Id
  accountRecipientId: Id | null
  amount: number
  amountRecipient: number | null
  categoryId: Id | null
  description: string
  payeeId: Id | null
  tagId: Id | null
  date: string
}

export interface TransactionDto extends CreateTransactionDto {
  author: UserDto
}

// Prefill data for the transaction/recurring dialogs: accepts enriched view
// rows (e.g. ViewTransaction) whose author lookup may not have resolved yet,
// without requiring dialogs to fabricate one.
export interface TransactionPrefill extends Omit<TransactionDto, 'author'> {
  author?: UserDto
}

export interface TransactionItemDto {
  item: TransactionDto
  accounts: AccountDto[]
}
