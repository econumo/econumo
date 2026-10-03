import type { ViewTransaction } from './useAccountTransactions'

export function canTouchTransaction(tx: ViewTransaction, canWrite: boolean): boolean {
  if (!canWrite) {
    return false
  }
  if (tx.type === 'transfer') {
    // a leg on an account the caller can't see stays untouchable; a row with NO
    // recipient at all (stored before the server required one, #261) is broken
    // rather than hidden and must stay deletable
    return !!tx.account && (tx.accountRecipientId === null || !!tx.accountRecipient)
  }
  return true
}
