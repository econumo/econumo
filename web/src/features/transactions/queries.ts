import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as transactionApi from '@/api/transaction'
import { toast } from 'sonner'
import type { MoveTransactionDto, TransactionDto, TransactionItemDto } from '@/api/dto/transaction'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { apiErrorMessage } from '@/lib/apiError'
import { METRICS, trackEvent } from '@/lib/metrics'
import { compareTransactions } from './order'
import { applyLocalMove } from './move'
import { beginLocalWrite, consumeFullReload, noteOwnWrite, patchTransactions, transactionsQueryFn } from './revisionTracking'

const byDateDesc = (items: TransactionDto[]) => [...items].sort((a, b) => compareTransactions(a, b))

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

export function useApplyTransactionItem() {
  const queryClient = useQueryClient()
  return (result: TransactionItemDto, mode: 'add' | 'update' | 'delete') => {
    const patch = beginLocalWrite(queryClient)
    if (patch) {
      const replaced = queryClient.getQueryData<TransactionDto[]>(queryKeys.transactions)?.find((t) => t.id === result.item.id)
      noteOwnWrite(result.accounts, [result.item.accountId, result.item.accountRecipientId, replaced?.accountId, replaced?.accountRecipientId])
    }
    queryClient.setQueryData(queryKeys.accounts, result.accounts)
    if (patch) {
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
    }
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

/** A drag on an account page. The list shows the drop at once; the server's
 *  rows replace the guess on success. A refusal puts back only the rows the
 *  drop touched (anything that arrived meanwhile stays) and syncs again: the
 *  usual cause is a day another device changed, whose rows this list lacks. */
export function useMoveTransaction() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: transactionApi.moveTransaction,
    onMutate: (req: MoveTransactionDto) => {
      const touched = new Set(req.transactionIds)
      const previous = (queryClient.getQueryData<TransactionDto[]>(queryKeys.transactions) ?? []).filter((t) => touched.has(t.id))
      const moved = previous.find((t) => t.id === req.id)
      patchTransactions(queryClient, (rows) => applyLocalMove(rows, req))
      return { previous, dayChanged: moved ? moved.date.slice(0, 10) !== req.date : false }
    },
    onSuccess: (result, _req, context) => {
      const patch = beginLocalWrite(queryClient)
      if (patch) {
        noteOwnWrite(result.accounts, result.items.flatMap((t) => [t.accountId, t.accountRecipientId]))
      }
      queryClient.setQueryData(queryKeys.accounts, result.accounts)
      if (patch) {
        const byId = new Map(result.items.map((t) => [t.id, t]))
        queryClient.setQueryData<TransactionDto[]>(queryKeys.transactions, (prev) => (prev ?? []).map((t) => byId.get(t.id) ?? t))
      }
      void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
      void queryClient.invalidateQueries({ queryKey: queryKeys.budgetPlan })
      void queryClient.invalidateQueries({ queryKey: queryKeys.budgetTransactions })
      trackEvent(METRICS.TRANSACTION_MOVE, { dayChanged: context?.dayChanged ?? false })
    },
    onError: (err, _req, context) => {
      const before = new Map((context?.previous ?? []).map((t) => [t.id, t]))
      queryClient.setQueryData<TransactionDto[]>(queryKeys.transactions, (rows) => rows?.map((t) => before.get(t.id) ?? t))
      void queryClient.invalidateQueries({ queryKey: queryKeys.transactions })
      toast.error(apiErrorMessage(err))
    },
  })
}
