import type { RecurringDto } from '@/api/dto/recurring'
import type { AccountDto } from '@/api/dto/account'
import type { CategoryDto } from '@/api/dto/category'
import type { PayeeDto } from '@/api/dto/payee'
import type { TagDto } from '@/api/dto/tag'
import type { ViewTransaction } from '@/features/transactions/useAccountTransactions'

interface Lookups {
  accounts?: AccountDto[]
  categories?: CategoryDto[]
  payees?: PayeeDto[]
  tags?: TagDto[]
}

/**
 * Shape a template like the transaction it will become, so the recurring UI can
 * render it through the transaction preview and list row instead of maintaining
 * a parallel layout. Mirrors the mapping useAccountTransactions already applies
 * to build the account list's unposted virtual rows.
 */
export function recurringAsTransaction(recurring: RecurringDto, { accounts, categories, payees, tags }: Lookups): ViewTransaction {
  return {
    id: recurring.id,
    type: recurring.type,
    accountId: recurring.accountId,
    accountRecipientId: recurring.accountRecipientId,
    amount: recurring.amount,
    // a template carries one amount; the recipient leg mirrors it
    amountRecipient: recurring.type === 'transfer' ? recurring.amount : null,
    categoryId: recurring.categoryId,
    payeeId: recurring.payeeId,
    tagId: recurring.tagId,
    description: recurring.description,
    // the next payment IS this row's date, which is why the recurring surfaces
    // need no separate "next payment" field
    date: recurring.nextPaymentAt,
    account: accounts?.find((a) => a.id === recurring.accountId),
    accountRecipient: recurring.accountRecipientId
      ? accounts?.find((a) => a.id === recurring.accountRecipientId)
      : undefined,
    category: recurring.categoryId ? categories?.find((c) => c.id === recurring.categoryId) : undefined,
    payee: recurring.payeeId ? payees?.find((p) => p.id === recurring.payeeId) : undefined,
    tag: recurring.tagId ? tags?.find((tg) => tg.id === recurring.tagId) : undefined,
    // `recurring` marks the template itself; isInFuture is left false so the row
    // is dimmed by that flag alone rather than twice over
    isInFuture: false,
    recurringId: recurring.id,
    recurring,
  } as unknown as ViewTransaction
}
