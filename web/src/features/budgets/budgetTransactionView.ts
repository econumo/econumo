import type { AccountDto } from '@/api/dto/account'
import type { BudgetTransactionDto } from '@/api/dto/budget'
import type { CategoryDto } from '@/api/dto/category'
import type { PayeeDto } from '@/api/dto/payee'
import type { TagDto } from '@/api/dto/tag'
import type { TransactionDto } from '@/api/dto/transaction'
import type { ViewTransaction } from '@/features/transactions/useAccountTransactions'
import { isFuture } from '@/lib/datetime'

export interface BudgetTransactionLookups {
  transactions?: TransactionDto[]
  accounts?: AccountDto[]
  categories?: CategoryDto[]
  payees?: PayeeDto[]
  tags?: TagDto[]
}

// full ViewTransaction when the caller can see the transaction in their own
// list; otherwise (a partner's row in a shared budget) a read-only shape is
// synthesized from the budget wire — enough for the preview, never editable
export function budgetTransactionView(wireTx: BudgetTransactionDto, l: BudgetTransactionLookups): ViewTransaction {
  const tx = l.transactions?.find((item) => item.id === wireTx.id)
  if (tx) {
    return {
      ...tx,
      account: l.accounts?.find((a) => a.id === tx.accountId),
      accountRecipient: tx.accountRecipientId ? l.accounts?.find((a) => a.id === tx.accountRecipientId) : undefined,
      category: tx.categoryId ? l.categories?.find((c) => c.id === tx.categoryId) : undefined,
      payee: tx.payeeId ? l.payees?.find((p) => p.id === tx.payeeId) : undefined,
      tag: tx.tagId ? l.tags?.find((tg) => tg.id === tx.tagId) : undefined,
      isInFuture: isFuture(tx.date),
    }
  }
  return {
    id: wireTx.id,
    author: wireTx.author,
    type: wireTx.type ?? (wireTx.direction ? 'transfer' : 'expense'),
    accountId: '',
    accountRecipientId: null,
    amount: wireTx.amount,
    amountRecipient: null,
    categoryId: wireTx.category?.id ?? null,
    description: wireTx.description,
    payeeId: wireTx.payee?.id ?? null,
    tagId: wireTx.tag?.id ?? null,
    // the budget wire carries the row's reporting labels; the preview
    // resolves them itself from the owner's label list
    labelIds: wireTx.labelIds ?? [],
    date: wireTx.spentAt,
    category: wireTx.category ? (wireTx.category as CategoryDto) : undefined,
    payee: wireTx.payee ? (wireTx.payee as PayeeDto) : undefined,
    tag: wireTx.tag ? (wireTx.tag as TagDto) : undefined,
    isInFuture: isFuture(wireTx.spentAt),
    // the budget wire carries no provenance; this synthesized shape is
    // read-only anyway, so no recurring action is offered on it
    recurringId: null,
    isImported: 0,
  }
}
