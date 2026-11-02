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

export interface DebtFormState {
  name: string
  currencyId: Id | null
  categoryId: Id | null
  payeeId: Id | null
  tagId: Id | null
  /** Y-m-d, "" = none */
  startDate: string
  balance: string
  payment: string
  schedule: RecurringSchedule
  /** Y-m-d, "" until chosen */
  lastPaymentDate: string
}

export function initialDebtFormState(params: OpenDebtParams, categories: CategoryDto[], defaultCurrencyId: Id | null): DebtFormState {
  const debt = params.debt
  if (debt) {
    return {
      name: debt.name,
      currencyId: debt.currencyId,
      categoryId: debt.categoryId,
      payeeId: debt.payeeId,
      tagId: debt.tagId,
      startDate: debt.startDate,
      // what is owed right now: the stored snapshot's balance is as of its own date
      balance: normalizeNumber(debt.currentBalance),
      payment: normalizeNumber(debt.payment),
      schedule: debt.schedule,
      lastPaymentDate: debt.lastPaymentDate,
    }
  }
  return {
    name: categories.find((c) => c.id === params.categoryId)?.name ?? '',
    currencyId: params.currencyId ?? defaultCurrencyId,
    categoryId: params.categoryId ?? null,
    payeeId: null,
    tagId: null,
    startDate: '',
    balance: '',
    payment: '',
    schedule: 'monthly',
    lastPaymentDate: '',
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

export type Projection = { status: 'date'; date: string } | { status: 'never' } | { status: 'unknown' }

const DAY = /^\d{4}-\d{2}-(\d{2})/

/** The date the debt clears under the form's balance and payment, at the debt's
    own implied rate. 'unknown' while the inputs cannot be projected (a half-typed
    amount, a debt with no next due date, a negative balance, no payment, another
    schedule than the stored one); 'never'
    when the payment does not cover the interest. */
export function projectForm(form: DebtFormState, debt: DebtDto): Projection {
  const balance = amountOf(form.balance)
  const payment = amountOf(form.payment)
  const rate = Number(debt.periodRate)
  // the stored rate and next due date are per period of the stored schedule
  if (form.schedule !== debt.schedule) {
    return { status: 'unknown' }
  }
  if (balance === null || payment === null || !Number.isFinite(rate) || !DAY.test(debt.nextDueDate)) {
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

function withProjection(form: DebtFormState, initial: DebtFormState, debt: DebtDto | undefined): DebtFormState {
  if (!debt) {
    return form
  }
  // terms typed back to what they were are untouched: the stored date returns,
  // so saving writes no snapshot
  if (form.balance === initial.balance && form.payment === initial.payment && form.schedule === initial.schedule) {
    return { ...form, lastPaymentDate: initial.lastPaymentDate }
  }
  const projection = projectForm(form, debt)
  return projection.status === 'date' ? { ...form, lastPaymentDate: projection.date } : form
}

export function buildDebtPayload(form: DebtFormState, initial: DebtFormState, debt: DebtDto | undefined): CreateDebtDto | UpdateDebtDto {
  const base = {
    name: form.name.trim(),
    currencyId: form.currencyId as Id,
    categoryId: form.categoryId as Id,
    payeeId: form.payeeId,
    tagId: form.tagId,
    startDate: form.startDate || null,
  }
  if (!debt) {
    return {
      ...base,
      balance: evaluatedAmount(form.balance),
      payment: evaluatedAmount(form.payment),
      schedule: form.schedule,
      lastPaymentDate: form.lastPaymentDate,
    }
  }
  // The form shows what is owed now, the server compares against the stored
  // snapshot: when no term was touched, send the snapshot back so a rename does
  // not write a new one.
  const untouched =
    form.balance === initial.balance &&
    form.payment === initial.payment &&
    form.schedule === initial.schedule &&
    form.lastPaymentDate === initial.lastPaymentDate
  return {
    ...base,
    id: debt.id,
    ...(untouched
      ? { balance: debt.balance, payment: debt.payment, schedule: debt.schedule, lastPaymentDate: debt.lastPaymentDate }
      : {
          balance: evaluatedAmount(form.balance),
          payment: evaluatedAmount(form.payment),
          schedule: form.schedule,
          lastPaymentDate: form.lastPaymentDate,
        }),
  }
}

export function useDebtForm(params: OpenDebtParams, categories: CategoryDto[], defaultCurrencyId: Id | null) {
  const debt = params.debt
  const [initial] = useState(() => initialDebtFormState(params, categories, defaultCurrencyId))
  const [form, setForm] = useState<DebtFormState>(initial)
  const patch = (partial: Partial<DebtFormState>) => setForm((prev) => ({ ...prev, ...partial }))

  // Balance and payment re-fill the last payment date from the debt's own
  // rate; the date itself and the schedule recalculate nothing.
  const setBalance = (balance: string) => setForm((prev) => withProjection({ ...prev, balance }, initial, debt))
  const setPayment = (payment: string) => setForm((prev) => withProjection({ ...prev, payment }, initial, debt))

  const projection: Projection = debt ? projectForm(form, debt) : { status: 'unknown' }

  return { form, initial, patch, setBalance, setPayment, projection }
}
