import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { v7 as uuidv7 } from 'uuid'
import * as currencyApi from '@/api/currency'
import type { Id } from '@/api/types'
import { queryKeys, ONE_DAY } from '@/app/queryKeys'

export function useCurrencies() {
  return useQuery({ queryKey: queryKeys.currencies, queryFn: currencyApi.getCurrencyList, staleTime: ONE_DAY })
}

export function useCurrencyRates() {
  return useQuery({ queryKey: queryKeys.currencyRates, queryFn: currencyApi.getCurrencyRateList, staleTime: ONE_DAY })
}

export function useCreateCurrency() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (form: { code: string; name: string; symbol?: string; fractionDigits?: number; rate?: string }) =>
      currencyApi.createCurrency({ id: uuidv7(), ...form }),
    onSuccess: () => {
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
      void queryClient.invalidateQueries({ queryKey: queryKeys.currencies })
    },
  })
}

export function useSetCurrencyRate() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: currencyApi.setCurrencyRate,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.currencies })
      void queryClient.invalidateQueries({ queryKey: queryKeys.currencyRates })
    },
  })
}

export function useArchiveCurrency() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: Id) => currencyApi.archiveCurrency(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.currencies })
    },
  })
}

export function useUnarchiveCurrency() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: Id) => currencyApi.unarchiveCurrency(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.currencies })
    },
  })
}

export function useDeleteCurrency() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: Id) => currencyApi.deleteCurrency(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.currencies })
    },
  })
}

export function useHideCurrency() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: Id) => currencyApi.hideCurrency(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.currencies })
    },
  })
}

export function useShowCurrency() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: Id) => currencyApi.showCurrency(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.currencies })
    },
  })
}
