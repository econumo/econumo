import { useMemo } from 'react'
import { useAccounts } from '@/features/accounts/queries'
import { useCurrencies } from '@/features/currencies/queries'
import { useUserData } from '@/features/user/queries'
import { abs, isNegative } from '@/lib/decimal'
import { moneyFormat, type CurrencyLike } from '@/lib/money'
import type { PreparedRow, WizardAnswers } from './prepare'
import { nameKey, writableAccounts } from './wizard'

export interface AccountView {
  name: string | null
  icon: string
  currency: (CurrencyLike & { id: string; code?: string }) | null
}

/** the account (name, icon, currency) a prepared row lands in under the current answers */
export function useAccountView(answers: WizardAnswers): (row: PreparedRow | null) => AccountView {
  const { data: user } = useUserData()
  const { data: accounts = [] } = useAccounts()
  const { data: currencies = [] } = useCurrencies()
  const writable = useMemo(() => writableAccounts(accounts, user?.id), [accounts, user?.id])
  const { account } = answers

  return (row) => {
    if (account?.kind === 'existing') {
      const found = accounts.find((a) => a.id === account.accountId)
      return { name: found?.name ?? null, icon: found?.icon || 'account_balance_wallet', currency: found?.currency ?? null }
    }
    if (account?.kind === 'new') {
      return { name: account.name, icon: 'account_balance_wallet', currency: currencies.find((c) => c.id === account.currencyId) ?? null }
    }
    if (account?.kind === 'column' && row && row.account !== null) {
      const key = nameKey(row.account)
      const found = writable.find((a) => nameKey(a.name) === key)
      if (found) return { name: found.name, icon: found.icon || 'account_balance_wallet', currency: found.currency }
      const currencyId = Object.hasOwn(account.currencies, key) ? account.currencies[key] : account.currencyId
      return { name: row.account, icon: 'account_balance_wallet', currency: currencies.find((c) => c.id === currencyId) ?? null }
    }
    return { name: null, icon: 'account_balance_wallet', currency: null }
  }
}

const isoParts = (date: string) => date.split('-').map(Number)

// the date as the UI language writes it ("3 Nov 2026"); the stored value stays ISO
export function formatDay(date: string, language: string): string {
  const [y, m, d] = isoParts(date)
  if (!y || !m || !d) return date
  return new Intl.DateTimeFormat(language, { dateStyle: 'medium', timeZone: 'UTC' }).format(new Date(Date.UTC(y, m - 1, d)))
}

/** day and month only ("3 Nov"), for a list whose year is clear from the totals */
export function formatShortDay(date: string, language: string): string {
  const [y, m, d] = isoParts(date)
  if (!y || !m || !d) return date
  return new Intl.DateTimeFormat(language, { day: 'numeric', month: 'short', timeZone: 'UTC' }).format(new Date(Date.UTC(y, m - 1, d)))
}

/** a signed amount as it will be added: "−4.50 €", income with a plus */
export function formatSigned(amount: string, currency: CurrencyLike | null): { income: boolean; text: string } {
  const income = !isNegative(amount)
  const value = moneyFormat(abs(amount), currency, { showCurrency: false, useNativePrecision: false })
  return { income, text: `${income ? '+' : '−'}${value}${currency ? ` ${currency.symbol}` : ''}` }
}
