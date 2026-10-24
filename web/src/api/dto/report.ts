import type { Id } from '../types'

export type GroupBy = 'category' | 'tag' | 'payee'

export interface ReportScope {
  budgetId: Id | null
}

export interface ReportChildRowDto {
  id: Id
  kind: string
  name: string
  icon: string
  isArchived: number
  amounts: string[]
}

export interface ReportRowDto {
  id: Id
  kind: string
  name: string
  icon: string
  isArchived: number
  ownerUserId: Id | null
  amounts: string[]
  /** budget scope only, "" = no plan; empty array in the my-accounts scope */
  planned: string[]
  children: ReportChildRowDto[]
}

export interface FlowReportDto {
  currencyId: Id
  months: string[]
  expense: ReportRowDto[]
  income: ReportRowDto[]
  transfersIn: string[]
  transfersOut: string[]
  savings: ReportRowDto[]
}

export interface NetWorthAccountDto {
  id: Id
  name: string
  icon: string
  currencyId: Id
  ownerUserId: Id
  isDeleted: number
  /** budget scope only: a savings member of the budget */
  isSavings?: number
  balances: string[]
}

export interface NetWorthReportDto {
  currencyId: Id
  months: string[]
  accounts: NetWorthAccountDto[]
}
