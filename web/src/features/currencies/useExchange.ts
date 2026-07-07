import { useCallback } from 'react'
import { exchange } from '@/lib/exchange'
import { useCurrencies, useCurrencyRates } from './queries'

export function useExchange() {
  const { data: rates } = useCurrencyRates()
  const { data: currencies } = useCurrencies()
  return useCallback(
    (fromCurrencyId: string, toCurrencyId: string, amount: number | string): number =>
      exchange(fromCurrencyId, toCurrencyId, amount, rates ?? [], currencies ?? []),
    [rates, currencies],
  )
}
