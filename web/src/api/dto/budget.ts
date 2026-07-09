import type { Id } from '../types'
import type { UserDto } from './user'

export type BudgetRole = 'owner' | 'admin' | 'user' | 'guest'

export interface BudgetAccessDto {
  user: UserDto
  role: BudgetRole
  isAccepted: 0 | 1
}

export interface BudgetMetaDto {
  id: Id
  ownerUserId: Id
  name: string
  /** full datetime Y-m-d H:i:s */
  startedAt: string
  currencyId: Id
  access: BudgetAccessDto[]
}

export const BudgetElementType = { ENVELOPE: 0, CATEGORY: 1, TAG: 2 } as const
export type BudgetElementType = (typeof BudgetElementType)[keyof typeof BudgetElementType]

export interface BudgetChildElementDto {
  id: Id
  type: BudgetElementType
  name: string
  icon: string
  isArchived: 0 | 1
  /** wire: decimal strings, coerced */
  spent: number
  budgetSpent: number
  ownerUserId: Id
}

export interface BudgetElementDto extends Omit<BudgetChildElementDto, 'ownerUserId'> {
  /** null = inherit the budget base currency */
  currencyId: Id | null
  folderId: Id | null
  position: number
  budgeted: number
  available: number
  ownerUserId: Id | null
  children: BudgetChildElementDto[]
}

export interface BudgetFolderDto {
  id: Id
  name: string
  position: number
}

// nullable by period phase: future month = all null except holdings; current month = endBalance null
export interface BudgetBalanceDto {
  currencyId: Id
  startBalance: number | null
  endBalance: number | null
  income: number | null
  expenses: number | null
  exchanges: number | null
  holdings: number | null
}

export interface BudgetRateDto {
  currencyId: Id
  baseCurrencyId: Id
  rate: number
  /** date-only Y-m-d */
  periodStart: string
  periodEnd: string
}

// budget/get-transaction-list has its own wire shape: spentAt (not date),
// embedded category/payee/tag refs and a per-transaction currencyId
export interface BudgetTransactionDto {
  id: Id
  author: UserDto
  currencyId: Id
  /** wire: decimal string, coerced */
  amount: number
  description: string
  category: { id: Id; name: string; icon: string } | null
  payee: { id: Id; name: string } | null
  tag: { id: Id; name: string } | null
  /** full datetime Y-m-d H:i:s */
  spentAt: string
}

export interface BudgetDto {
  meta: BudgetMetaDto
  filters: { periodStart: string; periodEnd: string; excludedAccountsIds: Id[] }
  balances: BudgetBalanceDto[]
  currencyRates: BudgetRateDto[]
  structure: { folders: BudgetFolderDto[]; elements: BudgetElementDto[] }
}
