import { useState } from 'react'
import type { CategoryDto } from '@/api/dto/category'
import type { CreateDebtDto, DebtDto, UpdateDebtDto } from '@/api/dto/debt'
import type { RecurringSchedule } from '@/api/dto/recurring'
import type { Id } from '@/api/types'
import type { OpenDebtParams } from '@/app/uiStore'
import { tryNormalize } from '@/lib/decimal'
import { normalizeNumber } from '@/lib/money'
import { isNotEmpty } from '@/lib/validation'
import { formatDate } from '@/lib/datetime'
import { firstDueAfter, projectLastPaymentDate } from './debtMath'

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
    payment: sentAmount(state.payment),
    schedule: state.schedule,
  }
}

export function useDebtCreateForm(params: OpenDebtParams, categories: CategoryDto[], defaultCurrencyId: Id | null) {
  const [form, setForm] = useState<DebtCreateState>(() => initialCreateState(params, categories, defaultCurrencyId))
  const patch = (partial: Partial<DebtCreateState>) => setForm((prev) => ({ ...prev, ...partial }))
  return { form, patch }
}

export interface DebtTermsState {
  /** the balance and payment are read in this currency, the debt's own until changed */
  currencyId: Id
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
    currencyId: debt.currencyId,
    // what is owed right now: the stored snapshot's balance is as of its own date
    balance: debt.isSetUp === 1 ? normalizeNumber(debt.currentBalance) : '',
    payment: normalizeNumber(debt.payment),
    schedule: debt.schedule,
    lastPaymentDate: debt.lastPaymentDate,
    startDate: debt.startDate,
  }
}

const SPACES = /[\s\u00a0\u202f]/g

/** A typed amount as a plain decimal string, null when it is not one. Spaces
    group thousands ("2 000"); a lone "," or "." is the decimal mark ("1,5"),
    and with both, or with several of one, the repeated one groups thousands
    ("2,000.50", "2.000,50", "1,000,000"). */
export function plainAmount(raw: string): string | null {
  const s = raw.replace(SPACES, '')
  let canonical: string
  if (/^-?\d+([.,]\d+)?$/.test(s)) {
    canonical = s.replace(',', '.')
  } else if (/^-?\d{1,3}(,\d{3})+(\.\d+)?$/.test(s)) {
    canonical = s.replace(/,/g, '')
  } else if (/^-?\d{1,3}(\.\d{3})+(,\d+)?$/.test(s)) {
    canonical = s.replace(/\./g, '').replace(',', '.')
  } else {
    return null
  }
  return tryNormalize(canonical)
}

/** a typed amount as a number, null while it is empty, half-typed or not a number */
export function amountOf(raw: string): number | null {
  const plain = plainAmount(raw)
  const n = plain === null ? NaN : Number(plain)
  return Number.isFinite(n) ? n : null
}

/** a typed amount as sent: the plain decimal, or the text as typed for the server to refuse */
function sentAmount(raw: string): string {
  return plainAmount(raw) ?? raw.trim()
}

/** a required amount's error: blank, not a number, or below the floor */
export function amountError(raw: string, t: (key: string) => string, positive: boolean): string | undefined {
  if (!isNotEmpty(raw)) {
    return t('common.validation.required_field')
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
    does not cover the interest. `fractionDigits` is the debt currency's
    precision: the debt is paid off below half its smallest unit. New terms
    are a snapshot as of `today` (the caller's day), which pays only the dues
    after it, so the projection starts at the first of those. */
export function projectTerms(form: DebtTermsState, debt: DebtDto, fractionDigits?: number, today = formatDate(new Date())): Projection {
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
  const match = DAY.exec(debt.lastPaymentDate)
  const anchor = match ? Number(match[1]) : undefined
  const first = firstDueAfter(debt.nextDueDate, form.schedule, today, anchor)
  if (first === null) {
    return { status: 'unknown' }
  }
  const date = projectLastPaymentDate(balance, payment, rate, form.schedule, first, anchor, fractionDigits)
  if (date !== null) {
    return { status: 'date', date }
  }
  // past the projection's horizon is not "never"
  return payment <= balance * rate ? { status: 'never' } : { status: 'unknown' }
}

// "1 090" typed over a stored 1090 is the same amount
const sameAmount = (a: string, b: string): boolean => sentAmount(a) === sentAmount(b)

function termsUntouched(form: DebtTermsState, initial: DebtTermsState): boolean {
  return sameAmount(form.balance, initial.balance) && sameAmount(form.payment, initial.payment) && form.schedule === initial.schedule
}

function withProjection(form: DebtTermsState, initial: DebtTermsState, debt: DebtDto, fractionDigits?: number): DebtTermsState {
  // terms typed back to what they were are untouched: the stored date returns,
  // so saving writes no snapshot
  if (debt.isSetUp === 1 && termsUntouched(form, initial)) {
    return { ...form, lastPaymentDate: initial.lastPaymentDate }
  }
  const projection = projectTerms(form, debt, fractionDigits)
  return projection.status === 'date' ? { ...form, lastPaymentDate: projection.date } : form
}

/** the update that saves the terms and the currency; the debt's name and filters go back as stored */
export function buildTermsPayload(form: DebtTermsState, initial: DebtTermsState, debt: DebtDto): UpdateDebtDto {
  const base = {
    id: debt.id,
    name: debt.name,
    currencyId: form.currencyId,
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
      payment: sentAmount(form.payment),
      schedule: form.schedule,
      ...(terms ? { balance: sentAmount(form.balance), lastPaymentDate: form.lastPaymentDate } : {}),
    }
  }
  // The form shows what is owed now, not the stored snapshot: when no term was
  // touched, send none, so the server keeps the stored ones and a start date or
  // currency change writes no snapshot. (Sent back, the stored numbers would be
  // rounded to a new currency's digits and read as new terms.)
  if (termsUntouched(form, initial) && form.lastPaymentDate === initial.lastPaymentDate) {
    return base
  }
  return {
    ...base,
    balance: sentAmount(form.balance),
    payment: sentAmount(form.payment),
    schedule: form.schedule,
    lastPaymentDate: form.lastPaymentDate,
  }
}

/** `digitsOf` gives a currency's fraction digits: the projection rounds in the
    currency the form is in */
export function useDebtTermsForm(debt: DebtDto, digitsOf: (currencyId: Id) => number | undefined = () => undefined) {
  const [initial] = useState(() => initialTermsState(debt))
  const [form, setForm] = useState<DebtTermsState>(initial)
  const patch = (partial: Partial<DebtTermsState>) => setForm((prev) => ({ ...prev, ...partial }))

  // Balance and payment re-fill the last payment date from the debt's own
  // rate; the date itself and the schedule recalculate nothing.
  const reproject = (next: DebtTermsState) => withProjection(next, initial, debt, digitsOf(next.currencyId))
  const setBalance = (balance: string) => setForm((prev) => reproject({ ...prev, balance }))
  const setPayment = (payment: string) => setForm((prev) => reproject({ ...prev, payment }))

  const projection = projectTerms(form, debt, digitsOf(form.currencyId))

  return { form, initial, patch, setBalance, setPayment, projection }
}
