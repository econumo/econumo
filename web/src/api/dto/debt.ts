import type { Id } from '../types'
import type { RecurringSchedule } from './recurring'

/** one rule of a debt: payments in a participant's category, narrowed by a
 *  payee and by a tag or a label (never both). The names are there for every
 *  reader, a partner's classifications included ("" when unset) */
export interface DebtRuleDto {
  id: Id
  categoryId: Id
  categoryName: string
  categoryIcon: string
  /** the category's owner, a participant of the debt's budget or a former one */
  ownerUserId: Id
  ownerName: string
  payeeId: Id | null
  payeeName: string
  tagId: Id | null
  tagName: string
  labelId: Id | null
  labelName: string
}

/** amounts are decimal strings in the debt's currency, dates Y-m-d ("" when none).
 *  While isSetUp is 0 the terms and figures (balance .. periodRate, payment and
 *  schedule excepted) are "" and the counts 0. */
export interface DebtDto {
  id: Id
  /** the budget the debt belongs to */
  budgetId: Id
  budgetName: string
  /** 1 when the caller may plan in that budget (owner, admin or user) and it is
   *  not archived; 0 reads only */
  canEdit: 0 | 1
  name: string
  /** the first rule's category icon */
  icon: string
  currencyId: Id
  /** 0..10 (0 only after a classification delete), oldest first */
  rules: DebtRuleDto[]
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
  /** implied rate per schedule period, 10 decimals */
  periodRate: string
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

export interface DebtDetailsDto {
  item: DebtDto
  snapshots: DebtSnapshotDto[]
  payments: DebtPaymentDto[]
}

/** a rule as sent; id names a rule the debt already has (one with an
 *  identical key keeps its id too) */
export interface DebtRuleFormDto {
  id?: Id
  categoryId: Id
  payeeId?: Id | null
  tagId?: Id | null
  labelId?: Id | null
}

export interface CreateDebtDto {
  budgetId: Id
  name: string
  /** left out = the budget's currency */
  currencyId?: Id
  /** 1..10 */
  rules: DebtRuleFormDto[]
  /** Y-m-d, the day the loan began; omitted or null = none (alone it only
   *  bounds which payments count) */
  startDate?: string | null
  /** the loan's facts, amountBorrowed, startDate and lastPaymentDate, come
   *  together: without them the debt is not set up */
  amountBorrowed?: string
  /** a statement balance (as of balanceDate, default today) over what the
   *  recorded payments give; the SPA states it through reconcile-debt */
  balance?: string
  balanceDate?: string
  /** must be positive (the server refuses 0 and blank) */
  payment: string
  schedule: RecurringSchedule
  lastPaymentDate?: string
}

/** currencyId "" keeps the stored currency, rules left out the stored rules
 *  (a list replaces them). amountBorrowed and lastPaymentDate come as a pair;
 *  they, payment, schedule and (on a set-up debt) startDate left out keep the
 *  stored ones, one that differs corrects the loan's facts. A debt stays in its
 *  budget */
export interface UpdateDebtDto extends Omit<CreateDebtDto, 'budgetId' | 'currencyId' | 'rules' | 'payment' | 'schedule'> {
  id: Id
  currencyId: Id | ''
  rules?: DebtRuleFormDto[]
  payment?: string
  schedule?: RecurringSchedule
}

/** "Add to a debt": one more rule on an existing debt */
export interface AddDebtRuleDto {
  id: Id
  rule: DebtRuleFormDto
}

export interface DebtRuleOptionDto {
  id: Id
  name: string
  icon: string
}

/** a participant of the budget with the classifications a rule may name:
 *  active expense categories, payees, tags and labels */
export interface DebtRuleParticipantDto {
  userId: Id
  name: string
  categories: DebtRuleOptionDto[]
  payees: DebtRuleOptionDto[]
  tags: DebtRuleOptionDto[]
  labels: DebtRuleOptionDto[]
}

/** the caller first, then the other participants by name */
export interface DebtRuleOptionsDto {
  participants: DebtRuleParticipantDto[]
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

export interface DeleteDebtForm {
  id: Id
  /** required when the debt still carries plans or comments */
  confirmDataRemoval?: boolean
}
