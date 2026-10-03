import { useMemo } from 'react'
import type { AccountDto } from '@/api/dto/account'
import type { CategoryDto } from '@/api/dto/category'
import type { LabelDto } from '@/api/dto/label'
import type { PayeeDto } from '@/api/dto/payee'
import type { TagDto } from '@/api/dto/tag'
import type { Id } from '@/api/types'
import { dayKey } from '@/lib/datetime'
import { moneyFormat, type CurrencyLike } from '@/lib/money'
import { matchesTerms, rankByName, type ClassificationType, type TermFields } from '@/lib/search'
import { useTransactions } from '@/features/transactions/queries'
import {
  enrichTransaction,
  groupByDay,
  useTransactionLookups,
  type DailyListEntry,
  type ViewTransaction,
} from '@/features/transactions/useAccountTransactions'
import { useUserData } from '@/features/user/queries'

export type SearchScope = { kind: 'all' } | { kind: ClassificationType; id: Id }

export interface GlobalSearchResult {
  accounts: AccountDto[]
  categories: CategoryDto[]
  payees: PayeeDto[]
  tags: TagDto[]
  labels: LabelDto[]
  transactions: DailyListEntry[]
  transactionCount: number
}

// the wire value ("1250") plus the row's display ("1,250.00") with and without
// separators, so whatever the user reads off a row finds it
function amountForms(amount: string | null | undefined, currency: CurrencyLike | undefined): string[] {
  if (amount === null || amount === undefined || amount === '') {
    return []
  }
  const shown = moneyFormat(amount, currency, { showCurrency: false, useNativePrecision: false })
  return [amount, shown, shown.replaceAll(',', '')]
}

export function transactionFields(tx: ViewTransaction): TermFields {
  return {
    text: [
      tx.description,
      tx.account?.name ?? '',
      tx.accountRecipient?.name ?? '',
      tx.category?.name ?? '',
      tx.payee?.name ?? '',
      tx.tag?.name ?? '',
      ...(tx.labels ?? []).map((l) => l.name),
      `@${tx.author?.name ?? ''}`,
      tx.type,
    ],
    exact: [
      ...amountForms(tx.amount, tx.account?.currency),
      ...amountForms(tx.amountRecipient, tx.accountRecipient?.currency),
      tx.date,
      // transfers are listed unsigned
      tx.type === 'expense' ? '-' : tx.type === 'income' ? '+' : '',
    ],
  }
}

// Shared classifications belong to another user and are not the caller's to
// manage or pick, so they are not offered as entity results.
function ownActiveFirst<T extends { ownerUserId: Id; isArchived: 0 | 1; name: string }>(
  items: T[] | undefined,
  me: Id | undefined,
  query: string,
): T[] {
  if (!query.trim() || !items || !me) {
    return []
  }
  const ranked = rankByName(items.filter((i) => i.ownerUserId === me), (i) => i.name, query)
  return [...ranked.filter((i) => i.isArchived === 0), ...ranked.filter((i) => i.isArchived === 1)]
}

function inScope(tx: ViewTransaction, scope: SearchScope): boolean {
  switch (scope.kind) {
    case 'all':
      return true
    case 'category':
      return tx.categoryId === scope.id
    case 'payee':
      return tx.payeeId === scope.id
    case 'tag':
      return tx.tagId === scope.id
    case 'label':
      return (tx.labelIds ?? []).includes(scope.id)
  }
}

/** `scope` must be referentially stable across renders (keep it in state). */
export function useGlobalSearch(query: string, scope: SearchScope): GlobalSearchResult {
  const { data: transactions } = useTransactions()
  const { data: user } = useUserData()
  const lookups = useTransactionLookups()

  // fields are built once per list, not per keystroke: amounts go through moneyFormat
  const enriched = useMemo(
    () =>
      (transactions ?? []).map((raw) => {
        const tx = enrichTransaction(raw, lookups)
        return { tx, fields: transactionFields(tx) }
      }),
    [transactions, lookups],
  )

  return useMemo(() => {
    const groups = scope.kind === 'all'
    const me = user?.id
    const matched = enriched
      .filter(({ tx, fields }) => inScope(tx, scope) && matchesTerms(fields, query))
      .map(({ tx }, index) => ({ tx, index }))
      .sort((a, b) => (a.tx.date < b.tx.date ? 1 : a.tx.date > b.tx.date ? -1 : a.index - b.index))
      .map(({ tx }) => ({ tx, groupDay: dayKey(tx.date) }))
    return {
      accounts: groups && query.trim() ? rankByName(lookups.accounts ?? [], (a) => a.name, query) : [],
      categories: groups ? ownActiveFirst(lookups.categories, me, query) : [],
      payees: groups ? ownActiveFirst(lookups.payees, me, query) : [],
      tags: groups ? ownActiveFirst(lookups.tags, me, query) : [],
      labels: groups ? ownActiveFirst(lookups.labels, me, query) : [],
      transactions: groupByDay(matched),
      transactionCount: matched.length,
    }
  }, [enriched, lookups, user?.id, query, scope])
}
