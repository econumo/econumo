import { useQuery } from '@tanstack/react-query'
import * as currencyApi from '@/api/currency'
import { queryKeys, ONE_DAY } from '@/app/queryKeys'

export function useCurrencies() {
  return useQuery({ queryKey: queryKeys.currencies, queryFn: currencyApi.getCurrencyList, staleTime: ONE_DAY })
}

export function useCurrencyRates() {
  return useQuery({ queryKey: queryKeys.currencyRates, queryFn: currencyApi.getCurrencyRateList, staleTime: ONE_DAY })
}
