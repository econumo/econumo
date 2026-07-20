import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef, useState } from 'react'
import * as transactionApi from '@/api/transaction'
import type { TransactionDto, TransactionItemDto } from '@/api/dto/transaction'
import type { Id } from '@/api/types'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { useAccounts } from '@/features/accounts/queries'
import { METRICS, trackEvent } from '@/lib/metrics'
import {
  advancePage,
  buildPagesFromBoot,
  mergeTransactions,
  PAGE_LIMIT,
  PER_ACCOUNT_LIMIT,
  type TransactionPagesMap,
  type TxKey,
  widenHorizon,
} from './window'

export function useTransactions() {
  const queryClient = useQueryClient()
  return useQuery({
    queryKey: queryKeys.transactions,
    // Boot loads a window per visible account. A refetch (10-min stale, focus,
    // restore) REPLACES the flat array and resets every window: scrolled-in
    // history is dropped so server-side deletions cannot linger as ghosts.
    queryFn: async () => {
      const res = await transactionApi.getTransactionList({ perAccountLimit: PER_ACCOUNT_LIMIT })
      queryClient.setQueryData<TransactionPagesMap>(
        queryKeys.transactionPages,
        buildPagesFromBoot(res.items, res.accounts ?? []),
      )
      return res.items
    },
    staleTime: TEN_MINUTES,
    // the backend orders per mode; date desc with id tie-break is re-applied here
    select: (items) => [...items].sort((a, b) => (a.date < b.date ? 1 : a.date > b.date ? -1 : a.id < b.id ? -1 : a.id > b.id ? 1 : 0)),
  })
}

// Reactive view of the per-account pagination map. The map is written
// imperatively (boot queryFn, pager) — this query never fetches.
export function useTransactionPages() {
  return useQuery<TransactionPagesMap>({
    queryKey: queryKeys.transactionPages,
    queryFn: () => ({}),
    enabled: false,
    staleTime: Infinity,
  })
}

export function useAccountTransactionPager(accountId: Id | undefined) {
  const queryClient = useQueryClient()
  const { data: pages } = useTransactionPages()
  const { data: accounts } = useAccounts()
  const state = accountId ? pages?.[accountId] : undefined
  const [isFetching, setIsFetching] = useState(false)
  const [isError, setIsError] = useState(false)
  const inFlight = useRef(false)

  const fetchPage = useCallback(
    async (cursor: string | undefined) => {
      if (!accountId || inFlight.current) {
        return
      }
      inFlight.current = true
      setIsFetching(true)
      setIsError(false)
      try {
        const res = await transactionApi.getTransactionList({ accountId, limit: PAGE_LIMIT, cursor })
        queryClient.setQueryData<TransactionDto[]>(queryKeys.transactions, (prev) =>
          mergeTransactions(prev ?? [], res.items),
        )
        queryClient.setQueryData<TransactionPagesMap>(queryKeys.transactionPages, (prev) => {
          const current = prev?.[accountId] ?? { nextCursor: null, hasMore: false, oldestLoaded: null }
          return { ...(prev ?? {}), [accountId]: advancePage(current, res.page, res.items) }
        })
      } catch {
        // No toast/logger infra exists in this app; surface via isError and
        // leave inFlight reset below so the sentinel retries on next intersection.
        setIsError(true)
      } finally {
        inFlight.current = false
        setIsFetching(false)
      }
    },
    [accountId, queryClient],
  )

  const fetchNext = useCallback(() => {
    if (state?.hasMore && state.nextCursor) {
      void fetchPage(state.nextCursor)
    }
  }, [state, fetchPage])

  // Ensure-window: an account absent from the map (hidden-folder accounts are
  // excluded from boot) gets its first page on demand. Gated on the account
  // being known to the client — a foreign/deleted/mistyped id in the route
  // would otherwise fire a guaranteed-403 fetch on every pages-map change.
  useEffect(() => {
    if (accountId && pages && !pages[accountId] && accounts?.some((a) => a.id === accountId)) {
      void fetchPage(undefined)
    }
  }, [accountId, pages, accounts, fetchPage])

  return { hasMore: state?.hasMore ?? false, isFetching, isError, fetchNext }
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
    if (mode === 'add') {
      // A backdated create may land older than the account's loaded horizon;
      // widen it so the new row isn't hidden as if it were unfetched history.
      const key: TxKey = { date: result.item.date, id: result.item.id }
      const affected = [result.item.accountId, result.item.accountRecipientId].filter((id): id is string => !!id)
      queryClient.setQueryData<TransactionPagesMap>(queryKeys.transactionPages, (prev) => {
        if (!prev) {
          return prev
        }
        let next = prev
        for (const acc of affected) {
          if (next[acc]) {
            next = { ...next, [acc]: widenHorizon(next[acc], key) }
          }
        }
        return next
      })
    }
    void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
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
