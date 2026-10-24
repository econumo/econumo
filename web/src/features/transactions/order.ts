import type { TransactionDto } from '@/api/dto/transaction'
import type { Id } from '@/api/types'

type Ordered = Pick<TransactionDto, 'id' | 'date' | 'accountId' | 'accountRecipientId' | 'sortKey' | 'recipientSortKey'>

/** The row's place within its day on `accountId`: the recipient key when the
 *  account is a transfer's recipient, the source key otherwise (and for lists
 *  spanning several accounts). Undefined from a server without keys. */
export function sideKey(tx: Ordered, accountId?: Id): number | undefined {
  if (accountId && tx.accountRecipientId === accountId && tx.accountId !== accountId && tx.recipientSortKey != null) {
    return tx.recipientSortKey
  }
  return tx.sortKey ?? undefined
}

/** List order: newest day first, then the side key (higher first), then id.
 *  The time of day only orders rows when the server sent no keys. */
export function compareTransactions(a: Ordered, b: Ordered, accountId?: Id): number {
  const dayA = a.date.slice(0, 10)
  const dayB = b.date.slice(0, 10)
  if (dayA !== dayB) {
    return dayA < dayB ? 1 : -1
  }
  const keyA = sideKey(a, accountId)
  const keyB = sideKey(b, accountId)
  if (keyA !== undefined && keyB !== undefined) {
    if (keyA !== keyB) {
      return keyB - keyA
    }
  } else if (a.date !== b.date) {
    return a.date < b.date ? 1 : -1
  }
  return a.id < b.id ? -1 : a.id > b.id ? 1 : 0
}
