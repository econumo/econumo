import type { CurrencyDto, CurrencyRateDto } from '@/api/dto/currency'
import { div, isZero, mul, normalize, round } from '@/lib/decimal'

// Convert through the base currency, falling back to the unconverted amount
// whenever a rate or currency is missing (or a rate is zero).
export function exchange(
  fromCurrencyId: string,
  toCurrencyId: string,
  amount: string,
  rates: CurrencyRateDto[],
  currencies: CurrencyDto[],
): string {
  const parsedAmount = normalize(amount)
  if (fromCurrencyId === toCurrencyId) {
    return parsedAmount
  }
  if (!Array.isArray(rates) || rates.length === 0) {
    return parsedAmount
  }
  const toCurrency = currencies.find((c) => c.id === toCurrencyId)
  if (!toCurrency) {
    return parsedAmount
  }
  let result = parsedAmount
  const fromRate = rates.find((r) => r.currencyId === fromCurrencyId)
  if (fromRate === undefined || isZero(fromRate.rate)) {
    return parsedAmount
  }
  if (fromCurrencyId !== fromRate.baseCurrencyId) {
    result = div(result, fromRate.rate)
  }
  const toRate = rates.find((r) => r.currencyId === toCurrencyId)
  if (toRate === undefined) {
    return parsedAmount
  }
  if (toCurrencyId !== toRate.baseCurrencyId) {
    result = mul(result, toRate.rate)
  }
  return round(result, toCurrency.fractionDigits)
}
