import type { CurrencyDto, CurrencyRateDto } from '@/api/dto/currency'

// Port of the Vue useCurrency().exchange: convert through the base currency,
// falling back to the unconverted amount whenever a rate or currency is missing.
export function exchange(
  fromCurrencyId: string,
  toCurrencyId: string,
  amount: number | string,
  rates: CurrencyRateDto[],
  currencies: CurrencyDto[],
): number {
  const parsedAmount = parseFloat(amount.toString())
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
  if (fromRate === undefined) {
    return parsedAmount
  }
  if (fromCurrencyId !== fromRate.baseCurrencyId) {
    result = result / fromRate.rate
  }
  const toRate = rates.find((r) => r.currencyId === toCurrencyId)
  if (toRate === undefined) {
    return parsedAmount
  }
  if (toCurrencyId !== toRate.baseCurrencyId) {
    result = result * toRate.rate
  }
  return Number(result.toFixed(toCurrency.fractionDigits))
}
