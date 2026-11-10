import { useCallback } from 'react'
import { useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query'
import * as transactionApi from '@/api/transaction'
import { toast } from 'sonner'
import type { Id } from '@/api/types'
import type { CreateTransactionDto, MoveTransactionDto, TransactionDto, TransactionItemDto } from '@/api/dto/transaction'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { apiErrorMessage } from '@/lib/apiError'
import { METRICS, trackEvent } from '@/lib/metrics'
import { usePendingOps } from '@/features/pendingWrites/hooks'
import { overlayTransactions } from '@/features/pendingWrites/overlay'
import { submitChange } from '@/features/pendingWrites/submit'
import { realIdOf } from '@/features/pendingWrites/tempIds'
import { useUserData } from '@/features/user/queries'
import { compareTransactions } from './order'
import { beginLocalWrite, consumeFullReload, noteOwnWrite, transactionsQueryFn } from './revisionTracking'

const byDateDesc = (items: TransactionDto[]) => [...items].sort((a, b) => compareTransactions(a, b))

export function useTransactions() {
  const queryClient = useQueryClient()
  const ops = usePendingOps()
  const { data: user } = useUserData()
  // the list is assembled from per-account buckets in no particular order; waiting changes go on
  // top of the server rows, and the overlay does not sort
  const select = useCallback(
    (items: TransactionDto[]) => byDateDesc(user ? overlayTransactions(items, ops, user) : items),
    [ops, user],
  )
  return useQuery({
    queryKey: queryKeys.transactions,
    queryFn: ({ signal }) => transactionsQueryFn(queryClient, { full: consumeFullReload(), signal }),
    staleTime: TEN_MINUTES,
    select,
  })
}

export function applyTransactionItem(queryClient: QueryClient, result: TransactionItemDto, mode: 'add' | 'update' | 'delete'): void {
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
        // a sync may have brought the row in while its response was on the way
        return [result.item, ...items.filter((t) => t.id !== result.item.id)]
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

export function useApplyTransactionItem() {
  const queryClient = useQueryClient()
  return (result: TransactionItemDto, mode: 'add' | 'update' | 'delete') => applyTransactionItem(queryClient, result, mode)
}

export function useCreateTransaction() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (form: CreateTransactionDto) =>
      submitChange(queryClient, { kind: 'tx-create', txId: form.id, payload: form }, (opts) => transactionApi.createTransaction(form, opts)),
  })
}

export function useUpdateTransaction() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (held: CreateTransactionDto) => {
      const form = { ...held, id: realIdOf(held.id) }
      return submitChange(queryClient, { kind: 'tx-update', txId: form.id, payload: form }, (opts) => transactionApi.updateTransaction(form, opts))
    },
  })
}

export function useDeleteTransaction() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (heldId: Id) => {
      const id = realIdOf(heldId)
      const previous = queryClient.getQueryData<TransactionDto[]>(queryKeys.transactions)?.find((t) => t.id === id) ?? null
      return submitChange(queryClient, { kind: 'tx-delete', txId: id, previous }, (opts) => transactionApi.deleteTransaction(id, opts))
    },
  })
}

/** A drag on an account page. The page shows the dropped arrangement itself
 *  until this settles, so the cache only ever holds server rows: the success
 *  merges the rows the move rewrote, and a refusal (usually a day another
 *  device changed) leaves the cache alone and syncs again. */
export function useMoveTransaction() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (req: MoveTransactionDto) =>
      transactionApi.moveTransaction({ ...req, id: realIdOf(req.id), aboveId: req.aboveId === null ? null : realIdOf(req.aboveId) }),
    onSuccess: (result, req: MoveTransactionDto) => {
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
      trackEvent(METRICS.TRANSACTION_MOVE, { dayChanged: req.fromDate !== req.date })
    },
    onError: (err) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.transactions })
      toast.error(apiErrorMessage(err))
    },
  })
}
