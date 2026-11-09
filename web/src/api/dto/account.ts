import type { Id } from '../types'
import type { UserDto } from './user'
import type { CurrencyDto } from './currency'
import type { TransactionDto } from './transaction'

export const AccountType = { CASH: 1, CREDIT_CARD: 2 } as const
export type AccountType = (typeof AccountType)[keyof typeof AccountType]

export type AccountRole = 'admin' | 'user' | 'guest'

export interface AccountAccessDto {
  user: UserDto
  role: AccountRole
  isAccepted: 0 | 1
}

export interface AccountDto {
  id: Id
  owner: UserDto
  folderId: Id | null
  name: string
  position: number
  currency: CurrencyDto
  /** decimal string (wire format, kept verbatim) */
  balance: string
  type: AccountType
  icon: string
  sharedAccess: AccountAccessDto[]
  /** moves by 1 per committed change to the account's transactions; absent on older servers */
  transactionsRevision?: number
  // how many rows get-account-transaction-list returns for it; absent from older servers
  transactionsCount?: number
}

export interface AccountItemDto {
  item: AccountDto
  /** opening-balance / correction transaction, when the backend wrote one */
  transaction: CorrectionDto | null
}

/** The correction's wire shape lacks the list's provenance fields; a correction never has any. */
export type CorrectionDto = Omit<TransactionDto, 'labelIds' | 'recurringId' | 'isImported'>
