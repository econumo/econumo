export interface CurrencyLike {
  symbol: string
  fractionDigits: number
}

export function normalizeNumber(amount: number | string | null | undefined): string {
  if (amount === null || amount === undefined) {
    return '0'
  }
  const num = typeof amount === 'string' ? Number(amount) : amount
  if (!Number.isFinite(num)) {
    return '0'
  }
  if (Math.abs(num) < 0.000001) {
    return num.toFixed(8).replace(/\.?0+$/, '') || '0'
  }
  const str = num.toString()
  const [intPart, decPart = ''] = str.split('.')
  if (!decPart) {
    return intPart
  }
  return `${intPart}.${decPart.slice(0, 8)}`.replace(/\.?0+$/, '')
}

export function formatNumber(amount: number | string | null | undefined, digits: number, useFixedPrecision: boolean): string {
  if (amount === null || amount === undefined) {
    return '0'
  }
  const num = typeof amount === 'string' ? Number(amount) : amount
  if (!Number.isFinite(num)) {
    return '0'
  }
  if (digits === 0) {
    return Math.round(num).toString()
  }
  if (useFixedPrecision) {
    return num.toFixed(digits)
  }
  const actualDecimals = num.toString().split('.')[1]?.length ?? 0
  const useDigits = Math.max(digits, Math.min(actualDecimals, 8))
  return num.toFixed(useDigits)
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
}

export function moneyFormat(
  amount: number | string,
  currency?: CurrencyLike | null,
  opts: MoneyFormatOptions = {},
): string {
  const { showCurrency = true, useNativePrecision = true, useThousandSeparator = true } = opts
  const normalizedAmount = normalizeNumber(amount)
  const digits = useNativePrecision
    ? (currency?.fractionDigits ?? 8)
    : Number.isInteger(Number(normalizedAmount))
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
