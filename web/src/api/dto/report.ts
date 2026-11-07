import type { Id } from '../types'

export interface NetWorthAccountDto {
  id: Id
  name: string
  icon: string
  currencyId: Id
  ownerUserId: Id
  /** budget scope only: a savings member of the budget */
  isSavings?: number
  balances: string[]
}

export interface NetWorthReportDto {
  currencyId: Id
  months: string[]
  accounts: NetWorthAccountDto[]
}
