import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { v7 as uuidv7 } from 'uuid'
import * as currencyApi from '@/api/currency'
import type { Id } from '@/api/types'
import { queryKeys, ONE_DAY } from '@/app/queryKeys'
import { METRICS, trackEvent } from '@/lib/metrics'

export function useCurrencies() {
  return useQuery({ queryKey: queryKeys.currencies, queryFn: currencyApi.getCurrencyList, staleTime: ONE_DAY })
}

export function useCurrencyRates() {
  return useQuery({ queryKey: queryKeys.currencyRates, queryFn: currencyApi.getCurrencyRateList, staleTime: ONE_DAY })
}

export function useCreateCurrency() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (form: { code: string; name: string; symbol?: string; fractionDigits?: number; rate: string }) =>
      currencyApi.createCurrency({ id: uuidv7(), ...form }),
    onSuccess: () => {
      trackEvent(METRICS.CURRENCY_CREATE)
      void queryClient.invalidateQueries({ queryKey: queryKeys.currencies })
      void queryClient.invalidateQueries({ queryKey: queryKeys.currencyRates })
    },
  })
}

export function useUpdateCurrency() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: currencyApi.updateCurrency,
    onSuccess: () => {
      trackEvent(METRICS.CURRENCY_UPDATE)
      void queryClient.invalidateQueries({ queryKey: queryKeys.currencies })
      // the fixed rate rides update-currency, so the rates view changes too
      void queryClient.invalidateQueries({ queryKey: queryKeys.currencyRates })
    },
  })
}

export function useDeleteCurrency() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: Id) => currencyApi.deleteCurrency(id),
    onSuccess: () => {
      trackEvent(METRICS.CURRENCY_DELETE)
      void queryClient.invalidateQueries({ queryKey: queryKeys.currencies })
    },
  })
}

export function useHideCurrency() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: Id) => currencyApi.hideCurrency(id),
    onSuccess: () => {
      trackEvent(METRICS.CURRENCY_DISABLE)
      void queryClient.invalidateQueries({ queryKey: queryKeys.currencies })
    },
  })
}

export function useShowCurrency() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: Id) => currencyApi.showCurrency(id),
    onSuccess: () => {
      trackEvent(METRICS.CURRENCY_ENABLE)
      void queryClient.invalidateQueries({ queryKey: queryKeys.currencies })
    },
  })
}

export function useHideAllCurrencies() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => currencyApi.hideAllCurrencies(),
    onSuccess: () => {
      trackEvent(METRICS.CURRENCY_DISABLE_ALL)
      void queryClient.invalidateQueries({ queryKey: queryKeys.currencies })
    },
  })
}

export function useShowAllCurrencies() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => currencyApi.showAllCurrencies(),
    onSuccess: () => {
      trackEvent(METRICS.CURRENCY_ENABLE_ALL)
      void queryClient.invalidateQueries({ queryKey: queryKeys.currencies })
    },
  })
}
