import type { Id } from '../types'
import type { RecurringSchedule } from './recurring'

/** amounts are decimal strings in the debt's currency, dates Y-m-d ("" when none).
 *  While isSetUp is 0 the terms and figures (balance .. periodRate, payment and
 *  schedule excepted) are "" and the counts 0. */
export interface DebtDto {
  id: Id
  /** the budget the debt belongs to */
  budgetId: Id
  budgetName: string
  /** 1 when the caller may plan in that budget (owner, admin or user), it is
   *  not archived and the debt is active; 0 reads only */
  canEdit: 0 | 1
  /** the element the debt is attached to: a top-level expense category or an envelope */
  elementId: Id
  elementType: 'category' | 'envelope'
  /** paid_off: ended (see endedAt), read-only until reopened */
  status: 'active' | 'paid_off'
  /** the last day payments count; "" while active */
  endedAt: string
  /** name, icon and currency are the element's */
  name: string
  icon: string
  currencyId: Id
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
  /** what the payments dated up to today went above the regular payment, due
   *  window by due window (a payment on the start date in full) */
  extraPaid: string
  payoffDate: string
  nextDueDate: string
  /** the rate per schedule period in force today, 10 decimals */
  periodRate: string
  /** the yearly percent in force today (up to 8 decimals); "" while not set up */
  annualRate: string
  /** the yearly percent the loan's own terms imply; "" while not set up */
  impliedAnnualRate: string
  isPaidOff: 0 | 1
  /** 1 once the debt has its amount borrowed, start date and last payment date */
  isSetUp: 0 | 1
}

/** one history entry: the origin (isOrigin 1, the amount borrowed as of the
 *  start date) or a statement balance after it */
export interface DebtSnapshotDto {
  id: Id
  isOrigin: 0 | 1
  /** what Econumo had worked out for asOf before the statement; "" for the origin */
  computedBalance: string
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

/** the payments still to come, soonest first; budgetName names the debt's
 *  budget when its planned extras are included, "" when only the regular payments count.
 *  paymentsLeft, payoffDate and overpaymentLeft are the debt's figures with
 *  those extras (its own when there are none), extraPlanned the planned extras
 *  they carry ("0" with none); 0 and "" while not set up */
export interface DebtForecastDto {
  items: DebtForecastItemDto[]
  budgetId: string
  budgetName: string
  paymentsLeft: number
  payoffDate: string
  overpaymentLeft: string
  extraPlanned: string
}

/** a stored change of the yearly rate and the regular payment, from `date` on */
export interface DebtRateChangeDto {
  id: Id
  date: string
  annualRate: string
  payment: string
}

export interface DebtDetailsDto {
  item: DebtDto
  snapshots: DebtSnapshotDto[]
  /** oldest first */
  rateChanges: DebtRateChangeDto[]
  payments: DebtPaymentDto[]
}

export interface CreateDebtDto {
  budgetId: Id
  /** a top-level expense category or an envelope of the budget */
  elementId: Id
  /** must be positive (the server refuses 0 and blank) */
  payment: string
  schedule: RecurringSchedule
  /** Y-m-d, asked only after debt.start_before_previous_end */
  startDate?: string | null
}

/** amountBorrowed and lastPaymentDate come as a pair; they, payment, schedule
 *  and (on a set-up debt) startDate left out keep the stored ones, one that
 *  differs corrects the loan's facts. A debt stays on its element */
export interface UpdateDebtDto {
  id: Id
  startDate?: string | null
  amountBorrowed?: string
  /** a statement balance (as of balanceDate, default today) */
  balance?: string
  balanceDate?: string
  payment?: string
  schedule?: RecurringSchedule
  lastPaymentDate?: string
}

/** date Y-m-d, omitted = today */
export interface MarkDebtPaidOffForm {
  id: Id
  date?: string
}

/** a statement balance as of balanceDate (Y-m-d; omitted = today), between the
 *  start date and today; a second one on the same date replaces the first */
export interface ReconcileDebtForm {
  id: Id
  balance: string
  balanceDate?: string
}

export interface DeleteDebtReconciliationForm {
  id: Id
  reconciliationId: Id
}

/** a new rate (yearly percent) and payment from `date` (Y-m-d); a second one on the same date replaces the first */
export interface AddDebtRateChangeForm {
  id: Id
  date: string
  annualRate: string
  payment: string
}

export interface DeleteDebtRateChangeForm {
  id: Id
  rateChangeId: Id
}

export interface DeleteDebtForm {
  id: Id
}
