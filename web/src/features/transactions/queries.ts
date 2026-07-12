import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef, useState } from 'react'
import * as transactionApi from '@/api/transaction'
import type { TransactionDto, TransactionItemDto } from '@/api/dto/transaction'
import type { Id } from '@/api/types'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { METRICS, trackEvent } from '@/lib/metrics'
import {
  advancePage,
  buildPagesFromBoot,
  mergeTransactions,
  PAGE_LIMIT,
  PER_ACCOUNT_LIMIT,
  type TransactionPagesMap,
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
  const state = accountId ? pages?.[accountId] : undefined
  const [isFetching, setIsFetching] = useState(false)
  const inFlight = useRef(false)

  const fetchPage = useCallback(
    async (cursor: string | undefined) => {
      if (!accountId || inFlight.current) {
        return
      }
      inFlight.current = true
      setIsFetching(true)
      try {
        const res = await transactionApi.getTransactionList({ accountId, limit: PAGE_LIMIT, cursor })
        queryClient.setQueryData<TransactionDto[]>(queryKeys.transactions, (prev) =>
          mergeTransactions(prev ?? [], res.items),
        )
        queryClient.setQueryData<TransactionPagesMap>(queryKeys.transactionPages, (prev) => {
          const current = prev?.[accountId] ?? { nextCursor: null, hasMore: false, oldestLoaded: null }
          return { ...(prev ?? {}), [accountId]: advancePage(current, res.page, res.items) }
        })
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
  // excluded from boot) gets its first page on demand.
  useEffect(() => {
    if (accountId && pages && !pages[accountId]) {
      void fetchPage(undefined)
    }
  }, [accountId, pages, fetchPage])

  return { hasMore: state?.hasMore ?? false, isFetching, fetchNext }
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
