import { normalize, round, toFixedString } from '@/lib/decimal'

export interface CurrencyLike {
  symbol: string
  fractionDigits: number
}

export function normalizeNumber(amount: number | string | null | undefined): string {
  return normalize(amount)
}

export function formatNumber(amount: number | string | null | undefined, digits: number, useFixedPrecision: boolean): string {
  const normalized = normalize(amount)
  if (digits === 0) {
    return round(normalized, 0)
  }
  if (useFixedPrecision) {
    return toFixedString(normalized, digits)
  }
  const actualDecimals = normalized.split('.')[1]?.length ?? 0
  const useDigits = Math.max(digits, Math.min(actualDecimals, 8))
  return toFixedString(normalized, useDigits)
}

export function addThousandSeparators(formatted: string): string[] {
  const parts = formatted.split('.')
  parts[0] = parts[0].replace(/\B(?=(\d{3})+(?!\d))/g, ',')
  return parts
}

export interface MoneyFormatOptions {
  showCurrency?: boolean
  useNativePrecision?: boolean
  useThousandSeparator?: boolean
  /** round to at most this many decimals */
  maxPrecision?: number
}

export function moneyFormat(
  amount: number | string,
  currency?: CurrencyLike | null,
  opts: MoneyFormatOptions = {},
): string {
  const { showCurrency = true, useNativePrecision = true, useThousandSeparator = true, maxPrecision } = opts
  let normalizedAmount = normalize(amount)
  if (maxPrecision !== undefined) {
    normalizedAmount = normalize(round(normalizedAmount, maxPrecision))
  }
  const digits = useNativePrecision
    ? (currency?.fractionDigits ?? 8)
    : !normalizedAmount.includes('.')
      ? (currency?.fractionDigits ?? 0)
      : Math.max(currency?.fractionDigits ?? 0, Math.min((normalizedAmount.split('.')[1] || '').length, 8))
  const formattedNumber = formatNumber(normalizedAmount, digits, useNativePrecision)
  const parts = useThousandSeparator ? addThousandSeparators(formattedNumber) : formattedNumber.split('.')

  let result = parts[0]
  if (parts.length > 1) {
    result += '.' + parts[1]
  }
  if (showCurrency && currency) {
    result += ' ' + currency.symbol
  }
  return result
}
