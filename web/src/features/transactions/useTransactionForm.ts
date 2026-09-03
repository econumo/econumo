import { useMemo, useState } from 'react'
import { v7 as uuidv7 } from 'uuid'
import type { AccountDto } from '@/api/dto/account'
import type { CategoryDto } from '@/api/dto/category'
import type { CreateTransactionDto, TransactionType } from '@/api/dto/transaction'
import type { FolderDto } from '@/api/dto/folder'
import type { Id } from '@/api/types'
import type { OpenTransactionParams } from '@/app/uiStore'
import type { ClassificationKind } from '@/lib/classificationKind'
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
  labelIds: Id[]
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
  const rt = params.postRecurring
  if (rt) {
    const account = accounts.find((a) => a.id === rt.accountId)
    return {
      id: uuidv7(),
      isNew: true,
      type: rt.type,
      accountId: rt.accountId,
      accountRecipientId: rt.accountRecipientId,
      amount: seedAmount(rt.amount, account),
      amountRecipient: '',
      categoryId: rt.categoryId,
      payeeId: rt.payeeId,
      tagId: rt.tagId,
      // the post dialog sends its own labelIds, so the chips start as the
      // template's set and any toggle the user makes lands on the transaction
      labelIds: rt.labelIds ?? [],
      description: rt.description,
      date: rt.nextPaymentAt,
    }
  }
  const iq = params.importQueued
  if (iq) {
    // an unmapped card's row carries accountId: '' — fall back to the first
    // account exactly like a fresh, unprefilled transaction would, so the
    // account select is never left blank (a blank select silently blocks submit)
    const accountId = iq.accountId || accounts[0]?.id || null
    const account = accounts.find((a) => a.id === accountId)
    return {
      id: uuidv7(),
      isNew: true,
      type: iq.type,
      accountId,
      accountRecipientId: null,
      amount: seedAmount(iq.amount, account),
      amountRecipient: '',
      categoryId: null,
      payeeId: null,
      tagId: null,
      labelIds: [],
      // the bank's merchant string is free text; payees are the user's own
      // entities, so it lands in the description for the user to reclassify
      description: iq.payee,
      date: iq.date,
    }
  }
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
      // update-transaction REPLACES the stored label set, so the edit form has
      // to carry the current ids through untouched or saving would detach them
      labelIds: tx.labelIds ?? [],
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
    labelIds: [],
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

interface OwnedRow {
  id: Id
  ownerUserId: Id
}

interface ClassificationLists {
  categories: OwnedRow[]
  payees: OwnedRow[]
  tags: OwnedRow[]
  labels: OwnedRow[]
}

interface ClassificationSelection {
  categoryId: Id | null
  payeeId: Id | null
  tagId: Id | null
  labelIds: Id[]
}

/**
 * Drop classifications the ACCOUNT OWNER does not own.
 *
 * The server accepts only the owner's category/payee/tag/labels on a shared
 * account. Rows written under an older, laxer rule can name the CALLER's own
 * instead, and those transactions then fail every save with
 * "This transaction is not available for this operation." Clearing them here
 * lets the edit go through: an unowned category falls back to uncategorized
 * (null), the rest simply unselect.
 *
 * An id absent from its list is KEPT, not dropped — the lists arrive from
 * async queries, and treating "not loaded yet" as "foreign" would wipe a
 * perfectly good selection on a slow connection.
 */
export function scrubForeignClassifications(
  selection: ClassificationSelection,
  lists: ClassificationLists,
  ownerUserId: Id | undefined,
): ClassificationSelection {
  if (!ownerUserId) {
    return selection
  }
  const keep = (rows: OwnedRow[], id: Id | null) => {
    if (!id) {
      return null
    }
    const row = rows.find((r) => r.id === id)
    return !row || row.ownerUserId === ownerUserId ? id : null
  }
  return {
    categoryId: keep(lists.categories, selection.categoryId),
    payeeId: keep(lists.payees, selection.payeeId),
    tagId: keep(lists.tags, selection.tagId),
    labelIds: selection.labelIds.filter((id) => keep(lists.labels, id) !== null),
  }
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
    // a transfer carries no classification; [] (never null/undefined) so the
    // replace-everything write clears whatever the row held before
    labelIds: isTransfer ? [] : form.labelIds,
    date: form.date,
  }
}

// A tag is radio-like (one per transaction, re-picking clears it) while labels
// are a free multi-select; both live on the same chip row, so the two
// transitions are shared with the recurring form via a structural type.
export function toggleTag<T extends { tagId: Id | null }>(state: T, id: Id): T {
  return { ...state, tagId: state.tagId === id ? null : id }
}

export function toggleLabel<T extends { labelIds: Id[] }>(state: T, id: Id): T {
  return {
    ...state,
    labelIds: state.labelIds.includes(id) ? state.labelIds.filter((labelId) => labelId !== id) : [...state.labelIds, id],
  }
}

export function toggleClassification<T extends { tagId: Id | null; labelIds: Id[] }>(state: T, kind: ClassificationKind, id: Id): T {
  return kind === 'tag' ? toggleTag(state, id) : toggleLabel(state, id)
}

export interface ClassificationChip {
  kind: ClassificationKind
  id: Id
  name: string
  icon: string
  checked: boolean
}

interface ClassificationRow {
  id: Id
  ownerUserId: Id
  name: string
  icon: string
  isArchived: 0 | 1
}

// Offered rows are the account OWNER's live ones (labels/tags belong to the
// owner, not the caller — the server rejects anything else on a shared
// account). An attached row is appended even when ARCHIVED, since dropping the
// chip would drop its id from the form and the write replaces the whole set.
// An attached row owned by someone ELSE is deliberately NOT appended: rows
// written under an older, laxer server rule can name the caller's own
// classifications, and keeping them would fail every save.
function kindChips(rows: ClassificationRow[], kind: ClassificationKind, attached: Id[], ownerUserId: Id | undefined): ClassificationChip[] {
  const ownedByAccount = (row: ClassificationRow) => !ownerUserId || row.ownerUserId === ownerUserId
  const offered = rows.filter((row) => row.isArchived === 0 && ownedByAccount(row))
  const extras = attached
    .map((id) => rows.find((row) => row.id === id))
    .filter((row): row is ClassificationRow => !!row && ownedByAccount(row) && !offered.some((shown) => shown.id === row.id))
  return [...offered, ...extras].map((row) => ({
    kind,
    id: row.id,
    name: row.name,
    icon: row.icon,
    checked: attached.includes(row.id),
  }))
}

export function classificationChips(
  tags: ClassificationRow[],
  labels: ClassificationRow[],
  selection: { tagId: Id | null; labelIds: Id[] },
  ownerUserId: Id | undefined,
): ClassificationChip[] {
  return [
    ...kindChips(tags, 'tag', selection.tagId ? [selection.tagId] : [], ownerUserId),
    ...kindChips(labels, 'label', selection.labelIds, ownerUserId),
  ]
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
