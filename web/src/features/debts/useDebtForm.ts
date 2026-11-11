import { useState } from 'react'
import type { CategoryDto } from '@/api/dto/category'
import type { CreateDebtDto, DebtDto, UpdateDebtDto } from '@/api/dto/debt'
import type { RecurringSchedule } from '@/api/dto/recurring'
import type { Id } from '@/api/types'
import type { OpenDebtParams } from '@/app/uiStore'
import { tryNormalize } from '@/lib/decimal'
import { moneyFormat, normalizeNumber } from '@/lib/money'
import type { CurrencyLike } from '@/lib/money'
import { isNotEmpty } from '@/lib/validation'

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
  /** Y-m-d, "" = none; asked only once another debt has the same filters,
      and then the day the loan began */
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

/** a new debt carries no terms: the loan's facts come with Finish setup */
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

/** everything the debt's Settings tab edits: its name, which payments count, its terms */
export interface DebtTermsState extends PaymentsFrom {
  name: string
  /** the amounts are read in this currency, the debt's own until changed */
  currencyId: Id
  /** the loan's facts: what was borrowed, from when, to when */
  amountBorrowed: string
  /** Y-m-d, "" until chosen */
  startDate: string
  /** Y-m-d, "" until chosen */
  lastPaymentDate: string
  payment: string
  schedule: RecurringSchedule
}

export function initialTermsState(debt: DebtDto): DebtTermsState {
  return {
    name: debt.name,
    categoryId: debt.categoryId,
    payeeId: debt.payeeId,
    tagId: debt.tagId,
    labelId: debt.labelId,
    currencyId: debt.currencyId,
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
    !sameAmount(form.payment, initial.payment) ||
    form.schedule !== initial.schedule ||
    form.lastPaymentDate !== initial.lastPaymentDate
  )
}

/** the loan's facts are asked together: once one is typed, or the debt is set up */
export function factsAsked(form: DebtTermsState, debt: DebtDto): boolean {
  return debt.isSetUp === 1 || isNotEmpty(form.amountBorrowed) || form.lastPaymentDate !== ''
}

/** The one update the Settings tab saves. A currency left as loaded goes as
    none, so the server keeps the stored one (another device may have changed it
    since); the loan's facts likewise go only when one was touched, all of them
    then, so the server reads them as one correction, and a set-up debt's start
    date only when changed (left out, the server keeps it). A debt not set up
    always sends its start date: it says which payments count, and null clears
    it. */
export function buildTermsPayload(form: DebtTermsState, initial: DebtTermsState, debt: DebtDto): UpdateDebtDto {
  const setUp = debt.isSetUp === 1
  const base = {
    id: debt.id,
    name: form.name.trim(),
    currencyId: form.currencyId === initial.currencyId ? '' : form.currencyId,
    // the category is required: its picker has no "any" row
    categoryId: form.categoryId ?? debt.categoryId,
    payeeId: form.payeeId,
    tagId: form.tagId,
    labelId: form.labelId,
    ...(!setUp || form.startDate !== initial.startDate ? { startDate: form.startDate || null } : {}),
  }
  const facts = { amountBorrowed: sentAmount(form.amountBorrowed), lastPaymentDate: form.lastPaymentDate }
  if (!setUp) {
    // without the loan's facts the save only updates payment, repeats and start date
    const complete = isNotEmpty(form.amountBorrowed) && form.lastPaymentDate !== ''
    return { ...base, payment: sentAmount(form.payment), schedule: form.schedule, ...(complete ? facts : {}) }
  }
  if (!factsTouched(form, initial)) {
    return base
  }
  return { ...base, ...facts, payment: sentAmount(form.payment), schedule: form.schedule }
}

export function useDebtTermsForm(debt: DebtDto) {
  const [initial] = useState(() => initialTermsState(debt))
  const [form, setForm] = useState<DebtTermsState>(initial)
  const patch = (partial: Partial<DebtTermsState>) => setForm((prev) => ({ ...prev, ...partial }))
  const reset = () => setForm(initial)
  const dirty = (Object.keys(initial) as (keyof DebtTermsState)[]).some((key) => form[key] !== initial[key])
  return { form, initial, patch, reset, dirty }
}
