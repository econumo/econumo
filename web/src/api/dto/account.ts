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
  /** wire: decimal string, coerced */
  balance: number
  type: AccountType
  icon: string
  sharedAccess: AccountAccessDto[]
}

export interface AccountItemDto {
  item: AccountDto
  /** opening-balance / correction transaction, when the backend wrote one */
  transaction: TransactionDto | null
}
