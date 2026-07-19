import { useMemo, useState } from 'react'
import { v7 as uuidv7 } from 'uuid'
import type { AccountDto } from '@/api/dto/account'
import type { CategoryDto } from '@/api/dto/category'
import type { CreateTransactionDto, TransactionType } from '@/api/dto/transaction'
import type { FolderDto } from '@/api/dto/folder'
import type { Id } from '@/api/types'
import type { OpenTransactionParams } from '@/app/uiStore'
import { formatDateTime } from '@/lib/datetime'
import { moneyFormat } from '@/lib/money'
import { evaluateFormula, sanitizeInput } from '@/lib/calculator'
import { normalize, tryNormalize } from '@/lib/decimal'

export interface TransactionFormState {
  id: Id
  isNew: boolean
  type: TransactionType
  accountId: Id | null
  accountRecipientId: Id | null
  amount: string
  amountRecipient: string
  categoryId: Id | null
  payeeId: Id | null
  tagId: Id | null
  description: string
  date: string
}

const seedAmount = (value: string | null | undefined, account: AccountDto | undefined): string => {
  if (value === null || value === undefined) {
    return ''
  }
  return moneyFormat(value, account?.currency, { showCurrency: false, useNativePrecision: false, useThousandSeparator: false })
}

export function initialFormState(params: OpenTransactionParams, accounts: AccountDto[], routeAccountId: Id | null): TransactionFormState {
  const tx = params.transaction
  if (tx) {
    const account = accounts.find((a) => a.id === tx.accountId)
    const recipient = accounts.find((a) => a.id === tx.accountRecipientId)
    return {
      id: tx.id,
      isNew: false,
      type: tx.type,
      accountId: tx.accountId,
      accountRecipientId: tx.accountRecipientId,
      amount: seedAmount(tx.amount, account),
      amountRecipient: seedAmount(tx.amountRecipient, recipient),
      categoryId: tx.categoryId,
      payeeId: tx.payeeId,
      tagId: tx.tagId,
      description: tx.description,
      date: tx.date,
    }
  }
  return {
    id: uuidv7(),
    isNew: true,
    type: params.type ?? 'expense',
    accountId: params.accountId ?? routeAccountId ?? accounts[0]?.id ?? null,
    accountRecipientId: null,
    amount: '',
    amountRecipient: '',
    categoryId: null,
    payeeId: null,
    tagId: null,
    description: '',
    date: formatDateTime(new Date()),
  }
}

// Plain decimal input skips the float-backed calculator so large amounts keep
// every digit; only actual formulas ("5+5") go through evaluation.
export const evaluatedAmount = (raw: string): string => {
  const sanitized = sanitizeInput(raw)
  if (/^-?\d+(\.\d+)?$/.test(sanitized)) {
    return normalize(sanitized)
  }
  return normalize(evaluateFormula(sanitized + '='))
}

export function buildPayload(form: TransactionFormState): CreateTransactionDto {
  const isTransfer = form.type === 'transfer'
  const amount = evaluatedAmount(form.amount)
  return {
    id: form.id,
    type: form.type,
    accountId: form.accountId as Id,
    accountRecipientId: isTransfer ? form.accountRecipientId : null,
    amount,
    amountRecipient: isTransfer
      ? form.amountRecipient === ''
        ? amount
        : (tryNormalize(sanitizeInput(form.amountRecipient)) ?? amount)
      : null,
    categoryId: isTransfer ? null : form.categoryId,
    description: form.description || '',
    payeeId: isTransfer ? null : form.payeeId,
    tagId: isTransfer ? null : form.tagId,
    date: form.date,
  }
}

export function accountOptions(accounts: AccountDto[], folders: FolderDto[], isNew: boolean): AccountDto[] {
  if (!isNew) {
    return accounts
  }
  // creation offers only accounts living in visible folders (Vue parity)
  const visibleFolderIds = new Set(folders.filter((f) => f.isVisible === 1).map((f) => f.id))
  return accounts.filter((a) => !a.folderId || visibleFolderIds.has(a.folderId))
}

export function categoryOptions(categories: CategoryDto[], type: TransactionType, ownerUserId: Id | undefined): CategoryDto[] {
  return categories.filter((c) => c.isArchived === 0 && c.type === type && (!ownerUserId || c.ownerUserId === ownerUserId))
}

export function canChangeAccountData(account: AccountDto | undefined, myUserId: Id | undefined): boolean {
  if (!account || !myUserId) {
    return false
  }
  if (account.owner.id === myUserId) {
    return true
  }
  return account.sharedAccess.some((access) => access.user.id === myUserId && access.role === 'admin')
}

export function useTransactionForm(params: OpenTransactionParams, accounts: AccountDto[], routeAccountId: Id | null) {
  const [form, setForm] = useState<TransactionFormState>(() => initialFormState(params, accounts, routeAccountId))
  const patch = (partial: Partial<TransactionFormState>) => setForm((prev) => ({ ...prev, ...partial }))

  const account = useMemo(() => accounts.find((a) => a.id === form.accountId), [accounts, form.accountId])
  const accountRecipient = useMemo(() => accounts.find((a) => a.id === form.accountRecipientId), [accounts, form.accountRecipientId])

  const setType = (type: TransactionType) => {
    // switching type clears the category (Vue parity)
    patch({ type, categoryId: null })
  }

  const recomputeRecipientAmount = (
    amount: string,
    from: AccountDto | undefined,
    to: AccountDto | undefined,
    exchangeFn: (fromId: string, toId: string, amount: string) => string,
  ): string => {
    if (!to || !from || amount === '' || tryNormalize(amount) === null) {
      return amount
    }
    if (from.currency.id === to.currency.id) {
      return amount
    }
    return exchangeFn(from.currency.id, to.currency.id, normalize(amount))
  }

  const swapAccounts = (exchangeFn: (fromId: string, toId: string, amount: string) => string) => {
    patch({
      accountId: form.accountRecipientId,
      accountRecipientId: form.accountId,
      // the entered amount now belongs to the other side — the saved recipient
      // amount is for the OLD direction, so re-derive it even when editing
      amountRecipient: recomputeRecipientAmount(form.amount, accountRecipient, account, exchangeFn),
    })
  }

  return { form, patch, setType, account, accountRecipient, recomputeRecipientAmount, swapAccounts }
}
