import { useState } from 'react'
import type { CreateDebtDto, DebtDto, UpdateDebtDto } from '@/api/dto/debt'
import type { DebtSchedule } from '@/api/dto/debt'
import type { OpenDebtParams } from '@/app/uiStore'
import { tryNormalize } from '@/lib/decimal'
import { moneyFormat, normalizeNumber } from '@/lib/money'
import type { CurrencyLike } from '@/lib/money'
import { isNotEmpty } from '@/lib/validation'
import { formatDate } from '@/lib/datetime'

export const SCHEDULE_ORDER: DebtSchedule[] = ['weekly', 'biweekly', 'monthly', 'quarterly', 'yearly', 'none']

/** the loan has no regular payment: the payment may be 0 and the last payment date reads "Return by" */
export const isNoFixed = (schedule: DebtSchedule): boolean => schedule === 'none'

export interface DebtCreateState {
  payment: string
  schedule: DebtSchedule
  /** Y-m-d, "" = none; asked only after the server says the loan cannot start
      before the end of the element's previous debt */
  startDate: string
  /** Return by a date (`none`): the whole amount and the day it is due back;
      the loan starts today, so the debt is created set up */
  amountBorrowed: string
  returnBy: string
}

export function initialCreateState(): DebtCreateState {
  return { payment: '', schedule: 'monthly', startDate: '', amountBorrowed: '', returnBy: '' }
}

/** Return by a date's own fields: the amount, and a return-by day after today */
export function returnByErrors(state: DebtCreateState, t: (key: string) => string): Record<string, string | undefined> {
  const required = t('common.validation.required_field')
  let lastPaymentDate = state.returnBy ? undefined : required
  if (state.returnBy && state.returnBy <= formatDate(new Date())) {
    lastPaymentDate = t('debts.form.return_by.after_today')
  }
  return {
    amountBorrowed: amountError(state.amountBorrowed, t, true),
    lastPaymentDate,
  }
}

/** A new debt carries no terms (the loan's facts come with Finish setup),
    except Return by a date, which is created set up in the one request. */
export function buildCreatePayload(state: DebtCreateState, params: OpenDebtParams): CreateDebtDto {
  if (isNoFixed(state.schedule)) {
    return {
      budgetId: params.budgetId,
      elementId: params.elementId,
      schedule: state.schedule,
      payment: '0',
      amountBorrowed: sentAmount(state.amountBorrowed),
      startDate: formatDate(new Date()),
      lastPaymentDate: state.returnBy,
    }
  }
  return {
    budgetId: params.budgetId,
    elementId: params.elementId,
    payment: sentPayment(state.payment, state.schedule),
    schedule: state.schedule,
    startDate: state.startDate || null,
  }
}

export function useDebtCreateForm() {
  const [form, setForm] = useState<DebtCreateState>(initialCreateState)
  const patch = (partial: Partial<DebtCreateState>) => setForm((prev) => ({ ...prev, ...partial }))
  return { form, patch }
}

/** everything the debt's Settings tab edits: its terms */
export interface DebtTermsState {
  /** the loan's facts: what was borrowed, from when, to when */
  amountBorrowed: string
  /** Y-m-d, "" until chosen */
  startDate: string
  /** Y-m-d, "" until chosen */
  lastPaymentDate: string
  payment: string
  schedule: DebtSchedule
}

export function initialTermsState(debt: DebtDto): DebtTermsState {
  return {
    amountBorrowed: debt.isSetUp === 1 ? normalizeNumber(debt.amountBorrowed) : '',
    startDate: debt.startDate,
    lastPaymentDate: debt.lastPaymentDate,
    payment: normalizeNumber(debt.payment),
    schedule: debt.schedule,
  }
}

const SPACES = /[\s\u00a0\u202f]/g

/** A typed amount as a plain decimal string, null when it is not one. It reads
    amounts the way the app shows them ("2,000.50") and the usual variants:
    spaces group thousands ("2 000"); "." is the decimal mark, and so is a lone
    "," ("1,5"), unless exactly three digits follow it ("1,200", as shown);
    with both marks, or several of one, the repeated one groups thousands
    ("2.000,50", "1,000,000"). */
export function plainAmount(raw: string): string | null {
  const s = raw.replace(SPACES, '')
  let canonical: string
  if (/^-?[1-9]\d{0,2}(,\d{3})+(\.\d+)?$/.test(s)) {
    canonical = s.replace(/,/g, '')
  } else if (/^-?\d+([.,]\d+)?$/.test(s)) {
    canonical = s.replace(',', '.')
  } else if (/^-?[1-9]\d{0,2}(\.\d{3})+(,\d+)?$/.test(s)) {
    canonical = s.replace(/\./g, '').replace(',', '.')
  } else {
    return null
  }
  return tryNormalize(canonical)
}

/** a typed amount as a number, null while it is empty, half-typed or not a number */
function amountOf(raw: string): number | null {
  const plain = plainAmount(raw)
  const n = plain === null ? NaN : Number(plain)
  return Number.isFinite(n) ? n : null
}

/** How a typed amount with a "," or "." is read, in the display format ("= 1,500.00 $"):
    "1,500" and "1.500" read differently, and this shows which. null without a
    separator or when it is no amount. */
export function amountEcho(raw: string, currency: CurrencyLike | undefined): string | null {
  const plain = /[.,]/.test(raw) ? plainAmount(raw) : null
  return plain === null ? null : `= ${moneyFormat(plain, currency)}`
}

/** a typed amount as sent: the plain decimal, or the text as typed for the server to refuse */
function sentAmount(raw: string): string {
  return plainAmount(raw) ?? raw.trim()
}

/** the payment as sent: no fixed payments take a blank one as 0 */
function sentPayment(raw: string, schedule: DebtSchedule): string {
  return isNoFixed(schedule) && raw.trim() === '' ? '0' : sentAmount(raw)
}

/** the payment's error: blank and 0 are fine without fixed payments, else it must be positive */
export function paymentError(raw: string, schedule: DebtSchedule, t: (key: string) => string): string | undefined {
  return isNoFixed(schedule) && raw.trim() === '' ? undefined : amountError(raw, t, !isNoFixed(schedule))
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

// "1 090" typed over a stored 1090 is the same amount
const sameAmount = (a: string, b: string): boolean => sentAmount(a) === sentAmount(b)

/** a fact of the loan (other than its start date, always sent) typed differently */
function factsTouched(form: DebtTermsState, initial: DebtTermsState): boolean {
  return (
    !sameAmount(form.amountBorrowed, initial.amountBorrowed) ||
    sentPayment(form.payment, form.schedule) !== sentPayment(initial.payment, initial.schedule) ||
    form.schedule !== initial.schedule ||
    form.lastPaymentDate !== initial.lastPaymentDate
  )
}

/** the loan's facts are asked together: once one is typed, or the debt is set up */
export function factsAsked(form: DebtTermsState, debt: DebtDto): boolean {
  return debt.isSetUp === 1 || isNotEmpty(form.amountBorrowed) || form.lastPaymentDate !== ''
}

/** The one update the Settings tab saves. The loan's facts go only when one
    was touched, all of them then, so the server reads them as one correction,
    and a set-up debt's start date only when changed (left out, the server
    keeps it). A debt not set up always sends its start date: it says which
    payments count, and null clears it. */
export function buildTermsPayload(form: DebtTermsState, initial: DebtTermsState, debt: DebtDto): UpdateDebtDto {
  const setUp = debt.isSetUp === 1
  const base = {
    id: debt.id,
    ...(!setUp || form.startDate !== initial.startDate ? { startDate: form.startDate || null } : {}),
  }
  const facts = { amountBorrowed: sentAmount(form.amountBorrowed), lastPaymentDate: form.lastPaymentDate }
  if (!setUp) {
    // without the loan's facts the save only updates payment, repeats and start date
    const complete = isNotEmpty(form.amountBorrowed) && form.lastPaymentDate !== ''
    return { ...base, payment: sentPayment(form.payment, form.schedule), schedule: form.schedule, ...(complete ? facts : {}) }
  }
  if (!factsTouched(form, initial)) {
    return base
  }
  return { ...base, ...facts, payment: sentPayment(form.payment, form.schedule), schedule: form.schedule }
}

export function useDebtTermsForm(debt: DebtDto) {
  const [initial] = useState(() => initialTermsState(debt))
  const [form, setForm] = useState<DebtTermsState>(initial)
  const patch = (partial: Partial<DebtTermsState>) => setForm((prev) => ({ ...prev, ...partial }))
  const reset = () => setForm(initial)
  const dirty = (Object.keys(initial) as (keyof DebtTermsState)[]).some((key) => form[key] !== initial[key])
  return { form, initial, patch, reset, dirty }
}
