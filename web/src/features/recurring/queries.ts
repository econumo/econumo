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
  const queryClient = useQueryClient()
  const replace = useReplaceRecurring()
  return useMutation({
    mutationFn: recurringApi.createRecurring,
    onSuccess: (item, form) => {
      replace(item)
      // created FROM a transaction: the backend linked the source to the new
      // template, so mirror that here and the recurring sign shows up on the
      // account list without a refetch
      if (form.sourceTransactionId) {
        queryClient.setQueryData<TransactionDto[]>(queryKeys.transactions, (prev) =>
          prev?.map((tx) => (tx.id === form.sourceTransactionId ? { ...tx, recurringId: item.id } : tx)),
        )
      }
    },
  })
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
      // mirror the backend's ON DELETE SET NULL: posted transactions lose their
      // link too, so the recurring sign leaves the account list without a
      // refetch
      queryClient.setQueryData<TransactionDto[]>(queryKeys.transactions, (prev) =>
        prev?.map((tx) => (tx.recurringId === id ? { ...tx, recurringId: null } : tx)),
      )
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
