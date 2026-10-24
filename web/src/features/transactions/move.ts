import type { MoveTransactionDto, TransactionDto } from '@/api/dto/transaction'

/** The list as the server will have it after `req`: the day's rows renumbered
 *  top-down on the page account's side, the moved row on the new date with its
 *  time kept. Rows the move does not touch keep their identity. */
export function applyLocalMove(rows: TransactionDto[], req: MoveTransactionDto): TransactionDto[] {
  const position = new Map(req.transactionIds.map((id, i) => [id, req.transactionIds.length - i]))
  return rows.map((tx) => {
    const key = position.get(tx.id)
    if (key === undefined) {
      return tx
    }
    const next = { ...tx }
    if (tx.id === req.id && tx.date.slice(0, 10) !== req.date) {
      next.date = req.date + tx.date.slice(10)
    }
    if (tx.accountRecipientId === req.accountId && tx.accountId !== req.accountId) {
      next.recipientSortKey = key
    } else {
      next.sortKey = key
    }
    return next
  })
}
