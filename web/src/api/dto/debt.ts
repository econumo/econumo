import type { Id } from '../types'
import type { RecurringSchedule } from './recurring'

/** amounts are decimal strings in the debt's currency, dates Y-m-d ("" when none).
 *  While isSetUp is 0 the terms and figures (balance .. periodRate, payment and
 *  schedule excepted) are "" and the counts 0. */
export interface DebtDto {
  id: Id
  name: string
  icon: string
  currencyId: Id
  categoryId: Id
  payeeId: Id | null
  tagId: Id | null
  labelId: Id | null
  /** "" when the debt has no start date */
  startDate: string
  /** the latest snapshot's terms */
  balance: string
  payment: string
  schedule: RecurringSchedule
  lastPaymentDate: string
  currentBalance: string
  paymentsLeft: number
  /** due periods up to today that received a payment */
  periodsPaid: number
  moneyLeft: string
  overpaymentLeft: string
  overpaidSoFar: string
  payoffDate: string
  nextDueDate: string
  /** implied rate per schedule period, 10 decimals */
  periodRate: string
  isPaidOff: 0 | 1
  /** 1 once the debt has a balance and a last payment date */
  isSetUp: 0 | 1
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
  labelId?: Id | null
  /** Y-m-d; omitted or null = none */
  startDate?: string | null
  /** the optional terms, both or neither: without them the debt is not set up */
  balance?: string
  /** must be positive (the server refuses 0 and blank) */
  payment: string
  schedule: RecurringSchedule
  lastPaymentDate?: string
}

/** the currency never changes after creation; the server ignores currencyId.
 *  Without balance and lastPaymentDate the stored terms stay, and a payment or
 *  schedule left out keeps the stored one (one that differs from a set-up debt's
 *  terms needs balance and lastPaymentDate too) */
export interface UpdateDebtDto extends Omit<CreateDebtDto, 'payment' | 'schedule'> {
  id: Id
  payment?: string
  schedule?: RecurringSchedule
}

export interface DeleteDebtForm {
  id: Id
  /** required when the debt still carries plans or comments */
  confirmDataRemoval?: boolean
}
