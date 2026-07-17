import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as recurringApi from '@/api/recurring'
import type { CreateRecurringDto, PostRecurringPayload, RecurringDto } from '@/api/dto/recurring'
import type { Id } from '@/api/types'
import type { TransactionDto } from '@/api/dto/transaction'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'

export function useRecurring() {
  return useQuery({
    queryKey: queryKeys.recurring,
    queryFn: recurringApi.getRecurringList,
    staleTime: TEN_MINUTES,
    select: (items) => [...items].sort((a, b) => (a.nextPaymentAt < b.nextPaymentAt ? -1 : a.nextPaymentAt > b.nextPaymentAt ? 1 : 0)),
  })
}

function useReplaceRecurring() {
  const queryClient = useQueryClient()
  return (item: RecurringDto) => {
    queryClient.setQueryData<RecurringDto[]>(queryKeys.recurring, (prev) => {
      const items = prev ?? []
      return items.some((r) => r.id === item.id) ? items.map((r) => (r.id === item.id ? item : r)) : [...items, item]
    })
  }
}

export function useCreateRecurring() {
  const replace = useReplaceRecurring()
  return useMutation({ mutationFn: recurringApi.createRecurring, onSuccess: replace })
}

export function useUpdateRecurring() {
  const replace = useReplaceRecurring()
  return useMutation({
    mutationFn: (form: CreateRecurringDto) => recurringApi.updateRecurring(form),
    onSuccess: replace,
  })
}

export function useDeleteRecurring() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: Id) => recurringApi.deleteRecurring(id),
    onSuccess: (_res, id) => {
      queryClient.setQueryData<RecurringDto[]>(queryKeys.recurring, (prev) => (prev ?? []).filter((r) => r.id !== id))
    },
  })
}

export function useSkipRecurring() {
  const replace = useReplaceRecurring()
  return useMutation({ mutationFn: (id: Id) => recurringApi.skipRecurring(id), onSuccess: replace })
}

export function usePostRecurring() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (payload: PostRecurringPayload) => recurringApi.postRecurring(payload),
    onSuccess: (result, payload) => {
      queryClient.setQueryData(queryKeys.accounts, result.accounts)
      queryClient.setQueryData<TransactionDto[]>(queryKeys.transactions, (prev) => [result.item, ...(prev ?? [])])
      queryClient.setQueryData<RecurringDto[]>(queryKeys.recurring, (prev) =>
        (prev ?? []).map((r) => (r.id === payload.recurringId ? { ...r, nextPaymentAt: result.nextPaymentAt } : r)),
      )
      void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
      void queryClient.invalidateQueries({ queryKey: queryKeys.budgetTransactions })
    },
  })
}
