import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as transactionApi from '@/api/transaction'
import type { TransactionDto, TransactionItemDto } from '@/api/dto/transaction'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { METRICS, trackEvent } from '@/lib/metrics'
import { consumeFullReload, noteOwnWrite, transactionsQueryFn } from './revisionTracking'

const byDateDesc = (items: TransactionDto[]) => [...items].sort((a, b) => (a.date < b.date ? 1 : a.date > b.date ? -1 : 0))

export function useTransactions() {
  const queryClient = useQueryClient()
  return useQuery({
    queryKey: queryKeys.transactions,
    queryFn: ({ signal }) => transactionsQueryFn(queryClient, { full: consumeFullReload(), signal }),
    staleTime: TEN_MINUTES,
    // the list is assembled from per-account buckets in no particular order
    select: byDateDesc,
  })
}

interface ApplyOptions {
  /** false when the cached item is known to differ from what the server stored */
  exact?: boolean
}

export function useApplyTransactionItem() {
  const queryClient = useQueryClient()
  return (result: TransactionItemDto, mode: 'add' | 'update' | 'delete', opts: ApplyOptions = {}) => {
    // a sync already in flight read the rows before this write; it re-runs if a revision still differs
    void queryClient.cancelQueries({ queryKey: queryKeys.transactions })
    const replaced = queryClient.getQueryData<TransactionDto[]>(queryKeys.transactions)?.find((t) => t.id === result.item.id)
    if (opts.exact !== false) {
      noteOwnWrite(result.accounts, [result.item.accountId, result.item.accountRecipientId, replaced?.accountId, replaced?.accountRecipientId])
    }
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
    void queryClient.invalidateQueries({ queryKey: queryKeys.budgetPlan })
    void queryClient.invalidateQueries({ queryKey: queryKeys.budgetTransactions })
  }
}

export function useCreateTransaction() {
  const apply = useApplyTransactionItem()
  return useMutation({
    mutationFn: transactionApi.createTransaction,
    onSuccess: (result) => {
      apply(result, 'add')
      trackEvent(METRICS.TRANSACTION_CREATE)
    },
  })
}

export function useUpdateTransaction() {
  const apply = useApplyTransactionItem()
  return useMutation({
    mutationFn: transactionApi.updateTransaction,
    onSuccess: (result) => {
      apply(result, 'update')
      trackEvent(METRICS.TRANSACTION_UPDATE)
    },
  })
}

export function useDeleteTransaction() {
  const apply = useApplyTransactionItem()
  return useMutation({
    mutationFn: transactionApi.deleteTransaction,
    onSuccess: (result) => {
      apply(result, 'delete')
      trackEvent(METRICS.TRANSACTION_DELETE)
    },
  })
}
