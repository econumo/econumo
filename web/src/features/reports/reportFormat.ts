import type { Id } from '@/api/types'
import { useCurrencies } from '@/features/currencies/queries'
import { abs, isNegative, isZero } from '@/lib/decimal'
import { moneyFormat } from '@/lib/money'

export function useMoney(currencyId: Id | undefined) {
  const { data: currencies } = useCurrencies()
  const currency = currencies?.find((c) => c.id === currencyId)
  return (amount: string) =>
    moneyFormat(amount, currency ?? { symbol: '', fractionDigits: 2 }, { showCurrency: !!currency })
}

export function compactNumber(lang: string) {
  const fmt = new Intl.NumberFormat(lang, { notation: 'compact', maximumFractionDigits: 1 })
  return (v: number) => fmt.format(v)
}

export function percent(pct: number | null): string | null {
  if (pct === null) return null
  const r = Math.round(pct)
  return `${r > 0 ? '+' : r < 0 ? '−' : ''}${Math.abs(r)}%`
}

export function signedMoney(diff: string, money: (a: string) => string): string {
  if (isZero(diff)) return money('0')
  return `${isNegative(diff) ? '−' : '+'}${money(abs(diff))}`
}
