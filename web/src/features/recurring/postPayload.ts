import { v7 as uuidv7 } from 'uuid'
import type { AccountDto } from '@/api/dto/account'
import type { PostRecurringPayload, RecurringDto } from '@/api/dto/recurring'
import { formatDateTime, isFuture } from '@/lib/datetime'
import { normalize } from '@/lib/decimal'

/**
 * The payload a due template posts as, without going through the transaction
 * form: a due template already describes the transaction the user means, so the
 * account list's Post acts outright. Mirrors buildPayload's transfer rules —
 * only a transfer carries a recipient, and only a non-transfer carries
 * category/payee/tag — so a template posted here and one posted through the
 * dialog write the same row.
 */
export function recurringPostPayload(
  recurring: RecurringDto,
  accounts: AccountDto[],
  exchangeFn: (fromCurrencyId: string, toCurrencyId: string, amount: string) => string,
): PostRecurringPayload {
  const isTransfer = recurring.type === 'transfer'
  const amount = normalize(recurring.amount)
  const from = accounts.find((a) => a.id === recurring.accountId)
  const to = recurring.accountRecipientId ? accounts.find((a) => a.id === recurring.accountRecipientId) : undefined
  // a template stores one amount, so a cross-currency transfer's recipient leg
  // has to be converted here as the form used to — passing the sender's number
  // through would silently credit the wrong sum
  const amountRecipient =
    isTransfer && from && to && from.currency.id !== to.currency.id ? exchangeFn(from.currency.id, to.currency.id, amount) : amount

  return {
    id: uuidv7(),
    recurringId: recurring.id,
    type: recurring.type,
    accountId: recurring.accountId,
    accountRecipientId: isTransfer ? recurring.accountRecipientId : null,
    amount,
    amountRecipient: isTransfer ? amountRecipient : null,
    categoryId: isTransfer ? null : recurring.categoryId,
    payeeId: isTransfer ? null : recurring.payeeId,
    tagId: isTransfer ? null : recurring.tagId,
    // labelIds is deliberately OMITTED: this path posts a due template outright,
    // with no chip row to edit, so it asks the server to inherit the template's
    // labels rather than echoing a possibly-stale cached copy of them
    description: recurring.description || '',
    // posting ahead of schedule means "pay it now": the money moves today, so a
    // future scheduled date is clamped to the present rather than written as-is
    date: isFuture(recurring.nextPaymentAt) ? formatDateTime(new Date()) : recurring.nextPaymentAt,
  }
}
