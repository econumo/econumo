import { useMemo, useState } from 'react'
import { v7 as uuidv7 } from 'uuid'
import type { AccountDto } from '@/api/dto/account'
import type { CreateRecurringDto, RecurringSchedule } from '@/api/dto/recurring'
import type { Id } from '@/api/types'
import type { TransactionType } from '@/api/dto/transaction'
import type { OpenRecurringParams } from '@/app/uiStore'
import { formatDateTime } from '@/lib/datetime'
import { normalizeNumber } from '@/lib/money'
import { evaluatedNumber } from '@/features/transactions/useTransactionForm'

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
  description: string
  schedule: RecurringSchedule
  nextPaymentAt: string
}

// unlike TransactionForm's seedAmount, this does NOT pad to the account's
// fraction digits — a recurring template's amount is re-entered on every
// post, so the prefill should echo the stored value verbatim (42.5 stays
// "42.5", not "42.50"), trimmed of float noise via normalizeNumber
const seedAmount = (value: number | null | undefined): string => (value === null || value === undefined ? '' : normalizeNumber(value))

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
      description: rt.description,
      schedule: rt.schedule,
      nextPaymentAt: rt.nextPaymentAt,
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
      description: tx.description,
      schedule: 'monthly',
      nextPaymentAt: formatDateTime(new Date()),
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
    description: '',
    schedule: 'monthly',
    nextPaymentAt: formatDateTime(new Date()),
  }
}

export function buildRecurringPayload(form: RecurringFormState): CreateRecurringDto {
  const isTransfer = form.type === 'transfer'
  return {
    id: form.id,
    type: form.type,
    accountId: form.accountId as Id,
    accountRecipientId: isTransfer ? form.accountRecipientId : null,
    amount: evaluatedNumber(form.amount),
    categoryId: isTransfer ? null : form.categoryId,
    payeeId: isTransfer ? null : form.payeeId,
    tagId: isTransfer ? null : form.tagId,
    description: form.description || '',
    schedule: form.schedule,
    nextPaymentAt: form.nextPaymentAt,
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

  return { form, patch, setType, account, accountRecipient }
}
