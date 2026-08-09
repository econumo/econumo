import { useMemo, useState } from 'react'
import { v7 as uuidv7 } from 'uuid'
import type { AccountDto } from '@/api/dto/account'
import type { CreateRecurringDto, RecurringSchedule } from '@/api/dto/recurring'
import type { Id } from '@/api/types'
import type { TransactionType } from '@/api/dto/transaction'
import type { OpenRecurringParams } from '@/app/uiStore'
import { formatDate, formatDateTime, parseDateTime } from '@/lib/datetime'
import { normalizeNumber } from '@/lib/money'
import { evaluatedAmount } from '@/features/transactions/useTransactionForm'

export interface RecurringFormState {
  id: Id
  isNew: boolean
  type: TransactionType
  accountId: Id | null
  accountRecipientId: Id | null
  amount: string
  categoryId: Id | null
  payeeId: Id | null
  tagId: Id | null
  labelIds: Id[]
  description: string
  schedule: RecurringSchedule
  nextPaymentAt: string
  /** the source transaction's date when the template is created FROM one; the
      next payment derives from it (anchor + one interval) and re-derives when
      the schedule changes. Null once the user picks a date by hand — a manual
      choice must not be clobbered by a later schedule change — and for the
      from-scratch and edit flows, which have no anchor. */
  anchorDate: string | null
  /** the source transaction's id when the template is created FROM one; sent
      on create so the backend links that transaction to the new template */
  sourceTransactionId: Id | null
}

const SCHEDULE_STEP: Record<RecurringSchedule, { days: number } | { months: number }> = {
  weekly: { days: 7 },
  biweekly: { days: 14 },
  monthly: { months: 1 },
  quarterly: { months: 3 },
  yearly: { months: 12 },
}

// nextFromAnchor returns anchor + one schedule interval, day-granular. Month
// steps clamp to the target month's end (Jan 31 + 1 month = Feb 28), mirroring
// the backend's scheduled-day clamping.
export function nextFromAnchor(anchor: string, schedule: RecurringSchedule): string {
  const d = parseDateTime(anchor)
  const step = SCHEDULE_STEP[schedule]
  if ('days' in step) {
    d.setDate(d.getDate() + step.days)
  } else {
    const day = d.getDate()
    d.setDate(1)
    d.setMonth(d.getMonth() + step.months)
    const daysInMonth = new Date(d.getFullYear(), d.getMonth() + 1, 0).getDate()
    d.setDate(Math.min(day, daysInMonth))
  }
  return `${formatDate(d)} 00:00:00`
}

// unlike TransactionForm's seedAmount, this does NOT pad to the account's
// fraction digits — a recurring template's amount is re-entered on every
// post, so the prefill should echo the stored value verbatim (42.5 stays
// "42.5", not "42.50"), trimmed of float noise via normalizeNumber
const seedAmount = (value: string | null | undefined): string => (value === null || value === undefined ? '' : normalizeNumber(value))

export function initialRecurringFormState(params: OpenRecurringParams, accounts: AccountDto[]): RecurringFormState {
  const rt = params.recurring
  if (rt) {
    return {
      id: rt.id,
      isNew: false,
      type: rt.type,
      accountId: rt.accountId,
      accountRecipientId: rt.accountRecipientId,
      amount: seedAmount(rt.amount),
      categoryId: rt.categoryId,
      payeeId: rt.payeeId,
      tagId: rt.tagId,
      // update-recurring-transaction REPLACES the stored label set, so an edit
      // has to carry the current ids through or saving would detach them
      labelIds: rt.labelIds ?? [],
      description: rt.description,
      schedule: rt.schedule,
      nextPaymentAt: rt.nextPaymentAt,
      anchorDate: null,
      sourceTransactionId: null,
    }
  }
  const tx = params.fromTransaction
  if (tx) {
    return {
      id: uuidv7(),
      isNew: true,
      type: tx.type,
      accountId: tx.accountId,
      accountRecipientId: tx.accountRecipientId,
      amount: seedAmount(tx.amount),
      categoryId: tx.categoryId,
      payeeId: tx.payeeId,
      tagId: tx.tagId,
      labelIds: tx.labelIds ?? [],
      description: tx.description,
      schedule: 'monthly',
      // the source transaction anchors the schedule: the next payment is one
      // interval after it, not today
      nextPaymentAt: nextFromAnchor(tx.date, 'monthly'),
      anchorDate: tx.date,
      sourceTransactionId: tx.id,
    }
  }
  return {
    id: uuidv7(),
    isNew: true,
    type: 'expense',
    accountId: params.accountId ?? accounts[0]?.id ?? null,
    accountRecipientId: null,
    amount: '',
    categoryId: null,
    payeeId: null,
    tagId: null,
    labelIds: [],
    description: '',
    schedule: 'monthly',
    nextPaymentAt: formatDateTime(new Date()),
    anchorDate: null,
    sourceTransactionId: null,
  }
}

export function buildRecurringPayload(form: RecurringFormState): CreateRecurringDto {
  const isTransfer = form.type === 'transfer'
  return {
    id: form.id,
    type: form.type,
    accountId: form.accountId as Id,
    accountRecipientId: isTransfer ? form.accountRecipientId : null,
    amount: evaluatedAmount(form.amount),
    categoryId: isTransfer ? null : form.categoryId,
    payeeId: isTransfer ? null : form.payeeId,
    tagId: isTransfer ? null : form.tagId,
    // a transfer template carries no classification; [] (never null) so the
    // replace-everything write clears whatever the template held before
    labelIds: isTransfer ? [] : form.labelIds,
    description: form.description || '',
    schedule: form.schedule,
    nextPaymentAt: form.nextPaymentAt,
    // create-from-transaction only; edits never carry a source
    ...(form.sourceTransactionId ? { sourceTransactionId: form.sourceTransactionId } : {}),
  }
}

export function useRecurringForm(params: OpenRecurringParams, accounts: AccountDto[]) {
  const [form, setForm] = useState<RecurringFormState>(() => initialRecurringFormState(params, accounts))
  const patch = (partial: Partial<RecurringFormState>) => setForm((prev) => ({ ...prev, ...partial }))

  const account = useMemo(() => accounts.find((a) => a.id === form.accountId), [accounts, form.accountId])
  const accountRecipient = useMemo(() => accounts.find((a) => a.id === form.accountRecipientId), [accounts, form.accountRecipientId])

  const setType = (type: TransactionType) => {
    patch({ type, categoryId: null })
  }

  // schedule changes re-derive the next payment from the anchor (the source
  // transaction's date), so switching monthly -> weekly moves e.g. Sep 17 back
  // to Aug 24 for an Aug 17 transaction. Without an anchor the date is the
  // user's own and stays put.
  const setSchedule = (schedule: RecurringSchedule) =>
    setForm((prev) => ({
      ...prev,
      schedule,
      ...(prev.anchorDate ? { nextPaymentAt: nextFromAnchor(prev.anchorDate, schedule) } : {}),
    }))

  return { form, patch, setType, setSchedule, account, accountRecipient }
}
