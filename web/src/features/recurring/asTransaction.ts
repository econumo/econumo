import type { RecurringDto } from '@/api/dto/recurring'
import type { AccountDto } from '@/api/dto/account'
import type { BudgetUpcomingDto } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'
import type { CategoryDto } from '@/api/dto/category'
import type { LabelDto } from '@/api/dto/label'
import type { PayeeDto } from '@/api/dto/payee'
import type { TagDto } from '@/api/dto/tag'
import type { ViewTransaction } from '@/features/transactions/useAccountTransactions'

interface Lookups {
  accounts?: AccountDto[]
  categories?: CategoryDto[]
  payees?: PayeeDto[]
  tags?: TagDto[]
  labels?: LabelDto[]
}

/**
 * Shape a template like the transaction it will become, so the recurring UI can
 * render it through the transaction preview and list row instead of maintaining
 * a parallel layout. Mirrors the mapping useAccountTransactions already applies
 * to build the account list's unposted virtual rows.
 */
export function recurringAsTransaction(recurring: RecurringDto, { accounts, categories, payees, tags, labels }: Lookups): ViewTransaction {
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
    labelIds: recurring.labelIds ?? [],
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
    // ordered by the owner's label list, like the account list's rows; an id
    // that no longer resolves is dropped rather than rendered blank
    labels: (labels ?? []).filter((l) => (recurring.labelIds ?? []).includes(l.id)),
    // `recurring` marks the template itself; isInFuture is left false so the row
    // is dimmed by that flag alone rather than twice over
    isInFuture: false,
    recurringId: recurring.id,
    isImported: 0,
    recurring,
  } as unknown as ViewTransaction
}

/**
 * One projected occurrence from the budget list, shaped for the read-only
 * preview. The template may sit on a partner's account the caller cannot see,
 * so the occurrence's own names stand in for whatever does not resolve locally.
 */
export function upcomingAsTransaction(
  occurrence: BudgetUpcomingDto,
  { accounts, currencies }: { accounts?: AccountDto[]; currencies?: CurrencyDto[] },
): ViewTransaction {
  // an account outside the budget comes with a blank id and name: the stand-in
  // then has no name, so the preview shows it as hidden
  const resolve = (id: Id, name: string, currencyId: Id): AccountDto =>
    (id ? accounts?.find((a) => a.id === id) : undefined) ??
    ({ id, name: name || undefined, icon: '', currency: currencies?.find((c) => c.id === currencyId), sharedAccess: [] } as unknown as AccountDto)
  const type = occurrence.type ?? (occurrence.direction ? 'transfer' : 'expense')
  const isTransfer = type === 'transfer' && Boolean(occurrence.amountRecipient)
  // amount/currencyId are the listed side's: on a transfer in, the recipient's
  const hasSource = isTransfer && Boolean(occurrence.amountSource && occurrence.currencySourceId)
  const amount = hasSource ? occurrence.amountSource : occurrence.amount
  const sourceCurrencyId = hasSource ? occurrence.currencySourceId! : occurrence.currencyId
  return {
    id: occurrence.templateId,
    author: occurrence.author,
    type,
    accountId: occurrence.accountId,
    accountRecipientId: isTransfer && occurrence.accountRecipientId ? occurrence.accountRecipientId : null,
    amount,
    amountRecipient: isTransfer ? occurrence.amountRecipient : null,
    categoryId: occurrence.category?.id ?? null,
    payeeId: occurrence.payee?.id ?? null,
    tagId: occurrence.tag?.id ?? null,
    labelIds: occurrence.labelIds ?? [],
    description: occurrence.description,
    date: occurrence.spentAt,
    account: resolve(occurrence.accountId, occurrence.accountName, sourceCurrencyId),
    accountRecipient: isTransfer
      ? resolve(occurrence.accountRecipientId ?? '', occurrence.accountRecipientName ?? '', occurrence.currencyRecipientId ?? '')
      : undefined,
    category: occurrence.category ? (occurrence.category as CategoryDto) : undefined,
    payee: occurrence.payee ? (occurrence.payee as PayeeDto) : undefined,
    tag: occurrence.tag ? (occurrence.tag as TagDto) : undefined,
    // the preview resolves labelIds against the caller's own labels and falls
    // back to these names, which is all a partner's labels can offer
    labels: occurrence.labelNames.map((name, i) => ({ id: `${occurrence.templateId}-label-${i}`, name, icon: 'label' }) as LabelDto),
    isInFuture: false,
    recurringId: occurrence.templateId,
    isImported: 0,
  } as unknown as ViewTransaction
}
