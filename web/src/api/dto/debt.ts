import type { Id } from '../types'
import type { RecurringSchedule } from './recurring'

/** amounts are decimal strings in the debt's currency, dates Y-m-d ("" when none) */
export interface DebtDto {
  id: Id
  name: string
  icon: string
  currencyId: Id
  categoryId: Id
  payeeId: Id | null
  tagId: Id | null
  /** "" when the debt has no start date */
  startDate: string
  /** the latest snapshot's terms */
  balance: string
  payment: string
  schedule: RecurringSchedule
  lastPaymentDate: string
  currentBalance: string
  paymentsLeft: number
  moneyLeft: string
  overpaymentLeft: string
  overpaidSoFar: string
  payoffDate: string
  nextDueDate: string
  /** implied rate per schedule period, 10 decimals */
  periodRate: string
  isPaidOff: 0 | 1
}

export interface DebtSnapshotDto {
  asOf: string
  balance: string
  payment: string
  schedule: RecurringSchedule
  lastPaymentDate: string
}

export interface DebtPaymentDto {
  transactionId: Id
  date: string
  amount: string
  interest: string
  principal: string
}

export interface DebtDetailsDto {
  item: DebtDto
  snapshots: DebtSnapshotDto[]
  payments: DebtPaymentDto[]
}

export interface CreateDebtDto {
  name: string
  currencyId: Id
  categoryId: Id
  payeeId?: Id | null
  tagId?: Id | null
  /** Y-m-d; omitted or null = none */
  startDate?: string | null
  balance: string
  /** must be positive (the server refuses 0 and blank) */
  payment: string
  schedule: RecurringSchedule
  lastPaymentDate: string
}

/** the currency never changes after creation; the server ignores currencyId */
export interface UpdateDebtDto extends CreateDebtDto {
  id: Id
}

export interface DeleteDebtForm {
  id: Id
  /** required when the debt still carries plans or comments */
  confirmDataRemoval?: boolean
}
