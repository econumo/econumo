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
  /** the day the loan began (always there once set up); "" when none */
  startDate: string
  /** the loan's amount as of startDate; "" while not set up */
  amountBorrowed: string
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
  /** 1 once the debt has its amount borrowed, start date and last payment date */
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
  /** what was owed after the payment; "" while the debt is not set up */
  balance: string
}

/** a projected payment; balance is what is owed after it */
export interface DebtForecastItemDto {
  date: string
  payment: string
  interest: string
  principal: string
  balance: string
}

/** the payments still to come, soonest first; budgetName names the budget whose
 *  planned extras are included, "" when only the regular payments count */
export interface DebtForecastDto {
  items: DebtForecastItemDto[]
  budgetId: string
  budgetName: string
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
  /** Y-m-d, the day the loan began; omitted or null = none (alone it only
   *  bounds which payments count) */
  startDate?: string | null
  /** the loan's facts, amountBorrowed, startDate and lastPaymentDate, come
   *  together: without them the debt is not set up */
  amountBorrowed?: string
  /** owed now, optional: a snapshot as of today over what the recorded
   *  payments give */
  balance?: string
  /** must be positive (the server refuses 0 and blank) */
  payment: string
  schedule: RecurringSchedule
  lastPaymentDate?: string
}

/** currencyId "" keeps the stored currency. A set-up debt needs its startDate;
 *  amountBorrowed, lastPaymentDate, payment or schedule left out keep the stored
 *  ones, one that differs corrects the loan's facts; a balance alone adds what
 *  is owed now */
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
