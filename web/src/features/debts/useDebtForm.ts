import { useState } from 'react'
import type { CategoryDto } from '@/api/dto/category'
import type { CreateDebtDto, DebtDto, UpdateDebtDto } from '@/api/dto/debt'
import type { RecurringSchedule } from '@/api/dto/recurring'
import type { Id } from '@/api/types'
import type { OpenDebtParams } from '@/app/uiStore'
import { tryNormalize } from '@/lib/decimal'
import { normalizeNumber } from '@/lib/money'
import { isNotEmpty, isValidFormula } from '@/lib/validation'
import { evaluatedAmount } from '@/features/transactions/useTransactionForm'
import { projectLastPaymentDate } from './debtMath'

export const SCHEDULE_ORDER: RecurringSchedule[] = ['weekly', 'biweekly', 'monthly', 'quarterly', 'yearly']

/** the filters that pick a debt's payments: a category, and optionally a payee, a tag and a label */
export interface PaymentsFrom {
  categoryId: Id | null
  payeeId: Id | null
  tagId: Id | null
  labelId: Id | null
}

export interface DebtCreateState extends PaymentsFrom {
  name: string
  currencyId: Id | null
  payment: string
  schedule: RecurringSchedule
  /** Y-m-d, "" = none; asked only once another debt has the same filters */
  startDate: string
}

export function initialCreateState(params: OpenDebtParams, categories: CategoryDto[], defaultCurrencyId: Id | null): DebtCreateState {
  return {
    name: categories.find((c) => c.id === params.categoryId)?.name ?? '',
    currencyId: params.currencyId ?? defaultCurrencyId,
    categoryId: params.categoryId ?? null,
    payeeId: null,
    tagId: null,
    labelId: null,
    payment: '',
    schedule: 'monthly',
    startDate: '',
  }
}

/** a new debt carries no terms: balance and last payment date come with Finish setup */
export function buildCreatePayload(state: DebtCreateState): CreateDebtDto {
  return {
    name: state.name.trim(),
    currencyId: state.currencyId as Id,
    categoryId: state.categoryId as Id,
    payeeId: state.payeeId,
    tagId: state.tagId,
    labelId: state.labelId,
    startDate: state.startDate || null,
    payment: evaluatedAmount(state.payment),
    schedule: state.schedule,
  }
}

export function useDebtCreateForm(params: OpenDebtParams, categories: CategoryDto[], defaultCurrencyId: Id | null) {
  const [form, setForm] = useState<DebtCreateState>(() => initialCreateState(params, categories, defaultCurrencyId))
  const patch = (partial: Partial<DebtCreateState>) => setForm((prev) => ({ ...prev, ...partial }))
  return { form, patch }
}

export interface DebtTermsState {
  balance: string
  payment: string
  schedule: RecurringSchedule
  /** Y-m-d, "" until chosen */
  lastPaymentDate: string
  /** Y-m-d, "" = none */
  startDate: string
}

export function initialTermsState(debt: DebtDto): DebtTermsState {
  return {
    // what is owed right now: the stored snapshot's balance is as of its own date
    balance: debt.isSetUp === 1 ? normalizeNumber(debt.currentBalance) : '',
    payment: normalizeNumber(debt.payment),
    schedule: debt.schedule,
    lastPaymentDate: debt.lastPaymentDate,
    startDate: debt.startDate,
  }
}

/** a typed amount as a number, null while it is empty, half-typed or a broken formula */
export function amountOf(raw: string): number | null {
  if (!isNotEmpty(raw) || !isValidFormula(raw)) {
    return null
  }
  const normalized = tryNormalize(evaluatedAmount(raw))
  const n = normalized === null ? NaN : Number(normalized)
  return Number.isFinite(n) ? n : null
}

/** a required amount's error: blank, a broken formula, not a number, or below the floor */
export function amountError(raw: string, t: (key: string) => string, positive: boolean): string | undefined {
  if (!isNotEmpty(raw)) {
    return t('common.validation.required_field')
  }
  if (!isValidFormula(raw)) {
    return t('common.validation.invalid_formula')
  }
  const n = amountOf(raw)
  if (n === null || n < 0 || (positive && n === 0)) {
    return t('common.validation.invalid_number')
  }
  return undefined
}

export type Projection = { status: 'date'; date: string } | { status: 'never' } | { status: 'unknown' }

const DAY = /^\d{4}-\d{2}-(\d{2})/

/** The date the debt clears under the form's balance and payment, at the debt's
    own implied rate. 'unknown' while the inputs cannot be projected (a half-typed
    amount, a debt with no rate or next due date yet, a negative balance, no
    payment, another schedule than the stored one); 'never' when the payment
    does not cover the interest. */
export function projectTerms(form: DebtTermsState, debt: DebtDto): Projection {
  // a debt not set up has no rate to project with: its last payment date is typed
  if (debt.isSetUp !== 1) {
    return { status: 'unknown' }
  }
  const balance = amountOf(form.balance)
  const payment = amountOf(form.payment)
  const rate = Number(debt.periodRate)
  // the stored rate and next due date are per period of the stored schedule
  if (form.schedule !== debt.schedule) {
    return { status: 'unknown' }
  }
  if (balance === null || payment === null || debt.periodRate === '' || !Number.isFinite(rate) || !DAY.test(debt.nextDueDate)) {
    return { status: 'unknown' }
  }
  // a negative balance or a payment of nothing is an input error, not a debt
  // that never pays off
  if (balance < 0 || payment <= 0 || rate < 0) {
    return { status: 'unknown' }
  }
  // the server anchors month-based schedules on the last payment date's day
  const anchor = DAY.exec(debt.lastPaymentDate)
  const date = projectLastPaymentDate(balance, payment, rate, form.schedule, debt.nextDueDate, anchor ? Number(anchor[1]) : undefined)
  if (date !== null) {
    return { status: 'date', date }
  }
  // past the projection's horizon is not "never"
  return payment <= balance * rate ? { status: 'never' } : { status: 'unknown' }
}

function termsUntouched(form: DebtTermsState, initial: DebtTermsState): boolean {
  return form.balance === initial.balance && form.payment === initial.payment && form.schedule === initial.schedule
}

function withProjection(form: DebtTermsState, initial: DebtTermsState, debt: DebtDto): DebtTermsState {
  // terms typed back to what they were are untouched: the stored date returns,
  // so saving writes no snapshot
  if (debt.isSetUp === 1 && termsUntouched(form, initial)) {
    return { ...form, lastPaymentDate: initial.lastPaymentDate }
  }
  const projection = projectTerms(form, debt)
  return projection.status === 'date' ? { ...form, lastPaymentDate: projection.date } : form
}

/** the update that saves the terms; the debt's name and filters go back as stored */
export function buildTermsPayload(form: DebtTermsState, initial: DebtTermsState, debt: DebtDto): UpdateDebtDto {
  const base = {
    id: debt.id,
    name: debt.name,
    currencyId: debt.currencyId,
    categoryId: debt.categoryId,
    payeeId: debt.payeeId,
    tagId: debt.tagId,
    labelId: debt.labelId,
    startDate: form.startDate || null,
  }
  if (debt.isSetUp !== 1) {
    // without both terms the save only updates payment, repeats and start date
    const terms = isNotEmpty(form.balance) && form.lastPaymentDate !== ''
    return {
      ...base,
      payment: evaluatedAmount(form.payment),
      schedule: form.schedule,
      ...(terms ? { balance: evaluatedAmount(form.balance), lastPaymentDate: form.lastPaymentDate } : {}),
    }
  }
  // The form shows what is owed now, the server compares against the stored
  // snapshot: when no term was touched, send the snapshot back so a start date
  // change does not write a new one.
  if (termsUntouched(form, initial) && form.lastPaymentDate === initial.lastPaymentDate) {
    return { ...base, balance: debt.balance, payment: debt.payment, schedule: debt.schedule, lastPaymentDate: debt.lastPaymentDate }
  }
  return {
    ...base,
    balance: evaluatedAmount(form.balance),
    payment: evaluatedAmount(form.payment),
    schedule: form.schedule,
    lastPaymentDate: form.lastPaymentDate,
  }
}

export function useDebtTermsForm(debt: DebtDto) {
  const [initial] = useState(() => initialTermsState(debt))
  const [form, setForm] = useState<DebtTermsState>(initial)
  const patch = (partial: Partial<DebtTermsState>) => setForm((prev) => ({ ...prev, ...partial }))

  // Balance and payment re-fill the last payment date from the debt's own
  // rate; the date itself and the schedule recalculate nothing.
  const setBalance = (balance: string) => setForm((prev) => withProjection({ ...prev, balance }, initial, debt))
  const setPayment = (payment: string) => setForm((prev) => withProjection({ ...prev, payment }, initial, debt))

  const projection = projectTerms(form, debt)

  return { form, initial, patch, setBalance, setPayment, projection }
}
