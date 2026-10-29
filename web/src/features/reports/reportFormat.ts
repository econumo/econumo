import type { Id } from '@/api/types'
import { useCurrencies } from '@/features/currencies/queries'
import { abs, isNegative, isZero, round } from '@/lib/decimal'
import { moneyFormat } from '@/lib/money'

/** `whole` rounds to whole units, for narrow columns on phones */
export function useMoney(currencyId: Id | undefined, { whole = false }: { whole?: boolean } = {}) {
  const { data: currencies } = useCurrencies()
  const currency = currencies?.find((c) => c.id === currencyId)
  const shape = { symbol: currency?.symbol ?? '', fractionDigits: whole ? 0 : (currency?.fractionDigits ?? 2) }
  return (amount: string) => moneyFormat(whole ? round(amount, 0) : amount, shape, { showCurrency: !!currency })
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
