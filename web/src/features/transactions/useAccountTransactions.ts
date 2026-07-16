import { useMemo } from 'react'
import type { AccountDto } from '@/api/dto/account'
import type { CategoryDto } from '@/api/dto/category'
import type { PayeeDto } from '@/api/dto/payee'
import type { TagDto } from '@/api/dto/tag'
import type { TransactionDto } from '@/api/dto/transaction'
import type { Id } from '@/api/types'
import { dayKey, formatDayHeading, isFuture, isToday, isYesterday } from '@/lib/datetime'
import { useAccounts } from '@/features/accounts/queries'
import { useCategories, usePayees, useTags } from '@/features/classifications/queries'
import { useTransactions } from './queries'

export interface ViewTransaction extends TransactionDto {
  account?: AccountDto
  accountRecipient?: AccountDto
  category?: CategoryDto
  payee?: PayeeDto
  tag?: TagDto
  isInFuture: boolean
}

export type DailyListEntry =
  | { kind: 'separator'; day: string; label: 'today' | 'yesterday' | 'date' }
  | { kind: 'transaction'; transaction: ViewTransaction }

export function separatorText(entry: { day: string; label: 'today' | 'yesterday' | 'date' }, t: (key: string) => string): string {
  if (entry.label === 'today') {
    return t('accounts.page.transaction_list.today')
  }
  if (entry.label === 'yesterday') {
    return t('accounts.page.transaction_list.yesterday')
  }
  return formatDayHeading(entry.day)
}

// Mirrors the Vue transactions-store search haystack: amounts, @author,
// category, description, payee, tag, sign, type, date — lowercased.
function haystack(tx: ViewTransaction): string {
  const sign = tx.type === 'expense' ? '-' : '+'
  return [
    tx.amount,
    tx.amountRecipient ?? '',
    `@${tx.author?.name ?? ''}`,
    tx.category?.name ?? '',
    tx.description,
    tx.payee?.name ?? '',
    tx.tag?.name ?? '',
    sign,
    tx.type,
    tx.date,
  ]
    .join('|')
    .toLowerCase()
}

export function useAccountTransactions(accountId: Id | undefined, search: string): DailyListEntry[] {
  const { data: transactions } = useTransactions()
  const { data: accounts } = useAccounts()
  const { data: categories } = useCategories()
  const { data: payees } = usePayees()
  const { data: tags } = useTags()

  return useMemo(() => {
    if (!transactions || !accountId) {
      return []
    }
    const enriched: ViewTransaction[] = transactions
      .filter((tx) => tx.accountId === accountId || tx.accountRecipientId === accountId)
      .map((tx) => ({
        ...tx,
        account: accounts?.find((a) => a.id === tx.accountId),
        accountRecipient: tx.accountRecipientId ? accounts?.find((a) => a.id === tx.accountRecipientId) : undefined,
        category: tx.categoryId ? categories?.find((c) => c.id === tx.categoryId) : undefined,
        payee: tx.payeeId ? payees?.find((p) => p.id === tx.payeeId) : undefined,
        tag: tx.tagId ? tags?.find((tg) => tg.id === tx.tagId) : undefined,
        isInFuture: isFuture(tx.date),
      }))

    const terms = search.toLowerCase().split(' ').filter(Boolean)
    const filtered = terms.length === 0 ? enriched : enriched.filter((tx) => {
      const hay = haystack(tx)
      return terms.every((term) => hay.includes(term))
    })

    // already date-desc from useTransactions' select; group by day
    const entries: DailyListEntry[] = []
    let currentDay: string | null = null
    for (const tx of filtered) {
      const day = dayKey(tx.date)
      if (day !== currentDay) {
        currentDay = day
        entries.push({ kind: 'separator', day, label: isToday(day) ? 'today' : isYesterday(day) ? 'yesterday' : 'date' })
      }
      entries.push({ kind: 'transaction', transaction: tx })
    }
    return entries
  }, [transactions, accounts, categories, payees, tags, accountId, search])
}

export interface TitleInfo {
  text: string
  source: 'transfer' | 'category' | 'description' | 'tag' | 'payee' | 'none'
}

export function transactionTitleInfo(
  tx: ViewTransaction,
  pageAccountId: Id,
  t: (key: string, params?: Record<string, string>) => string,
): TitleInfo {
  if (tx.type === 'transfer') {
    const incoming = tx.accountId !== pageAccountId
    const counterparty = incoming ? tx.account : tx.accountRecipient
    const name = counterparty?.name ?? t('accounts.account.name_hidden')
    return {
      text: t(incoming ? 'accounts.page.transaction_list.item.transfer_from' : 'accounts.page.transaction_list.item.transfer_to', {
        account: name,
      }),
      source: 'transfer',
    }
  }
  if (tx.category?.name) {
    return { text: tx.category.name, source: 'category' }
  }
  if (tx.description) {
    return { text: tx.description, source: 'description' }
  }
  if (tx.tag?.name) {
    return { text: tx.tag.name, source: 'tag' }
  }
  if (tx.payee?.name) {
    return { text: tx.payee.name, source: 'payee' }
  }
  return { text: '', source: 'none' }
}

export function isIncomeForAccount(tx: ViewTransaction, pageAccountId: Id): boolean {
  if (tx.type === 'transfer') {
    return tx.accountId !== pageAccountId
  }
  return tx.type !== 'expense'
}
