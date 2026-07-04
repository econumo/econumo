import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as transactionApi from '@/api/transaction'
import type { TransactionDto, TransactionItemDto } from '@/api/dto/transaction'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'

export function useTransactions() {
  return useQuery({
    queryKey: queryKeys.transactions,
    queryFn: transactionApi.getTransactionList,
    staleTime: TEN_MINUTES,
    // the multi-account backend query has no ORDER BY; date desc is applied here
    select: (items) => [...items].sort((a, b) => (a.date < b.date ? 1 : a.date > b.date ? -1 : 0)),
  })
}

function useApplyTransactionItem() {
  const queryClient = useQueryClient()
  return (result: TransactionItemDto, mode: 'add' | 'update' | 'delete') => {
    queryClient.setQueryData(queryKeys.accounts, result.accounts)
    queryClient.setQueryData<TransactionDto[]>(queryKeys.transactions, (prev) => {
      const items = prev ?? []
      if (mode === 'add') {
        return [result.item, ...items]
      }
      if (mode === 'update') {
        return items.map((t) => (t.id === result.item.id ? result.item : t))
      }
      return items.filter((t) => t.id !== result.item.id)
    })
    void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
  }
}

export function useCreateTransaction() {
  const apply = useApplyTransactionItem()
  return useMutation({
    mutationFn: transactionApi.createTransaction,
    onSuccess: (result) => apply(result, 'add'),
  })
}

export function useUpdateTransaction() {
  const apply = useApplyTransactionItem()
  return useMutation({
    mutationFn: transactionApi.updateTransaction,
    onSuccess: (result) => apply(result, 'update'),
  })
}

export function useDeleteTransaction() {
  const apply = useApplyTransactionItem()
  return useMutation({
    mutationFn: transactionApi.deleteTransaction,
    onSuccess: (result) => apply(result, 'delete'),
  })
}
