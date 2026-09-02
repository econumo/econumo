import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as importsApi from '@/api/imports'
import type { Id } from '@/api/types'
import type { ImportQueueDto, ImportQueuedEventPayload, ImportSourceDto, UpdateImportAccountDto } from '@/api/dto/imports'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { METRICS, trackEvent } from '@/lib/metrics'
import { useApplyTransactionItem } from '@/features/transactions/queries'

export function useImportSources() {
  return useQuery({ queryKey: queryKeys.importSources, queryFn: importsApi.getImportSourceList, staleTime: TEN_MINUTES })
}

export function useImportQueue() {
  return useQuery({ queryKey: queryKeys.importQueue, queryFn: importsApi.getImportQueue, staleTime: TEN_MINUTES })
}

export function useCreateImportSource() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ provider, name }: { provider: 'apple-wallet'; name: string }) => importsApi.createImportSource(provider, name),
    onSuccess: (item) => {
      queryClient.setQueryData<ImportSourceDto[]>(queryKeys.importSources, (prev) => {
        const items = prev ?? []
        return items.some((s) => s.id === item.id) ? items.map((s) => (s.id === item.id ? item : s)) : [...items, item]
      })
      trackEvent(METRICS.IMPORT_SOURCE_CONNECT, { provider: item.provider })
    },
  })
}

export function useDeleteImportSource() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: importsApi.deleteImportSource,
    onSuccess: (items) => {
      queryClient.setQueryData(queryKeys.importSources, items)
      void queryClient.invalidateQueries({ queryKey: queryKeys.importQueue })
    },
  })
}

// link/ignore/unlink all return the refreshed source; a link that drained a
// queue also created transactions, so the ledger caches are invalidated too.
function useApplyImportAccount() {
  const queryClient = useQueryClient()
  return (result: UpdateImportAccountDto) => {
    queryClient.setQueryData<ImportSourceDto[]>(queryKeys.importSources, (prev) =>
      (prev ?? []).map((s) => (s.id === result.item.id ? result.item : s)))
    void queryClient.invalidateQueries({ queryKey: queryKeys.importQueue })
    if (result.run) {
      void queryClient.invalidateQueries({ queryKey: queryKeys.transactions })
      void queryClient.invalidateQueries({ queryKey: queryKeys.accounts })
      void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
      void queryClient.invalidateQueries({ queryKey: queryKeys.budgetTransactions })
    }
  }
}

export function useLinkImportAccount() {
  const apply = useApplyImportAccount()
  return useMutation({
    mutationFn: ({ sourceId, externalAccountId, accountId }: { sourceId: Id; externalAccountId: string; accountId: Id }) =>
      importsApi.linkImportAccount(sourceId, externalAccountId, accountId),
    onSuccess: (result) => {
      apply(result)
      trackEvent(METRICS.IMPORT_ACCOUNT_LINK, { imported: result.run?.importedCount ?? 0, matched: result.run?.matchedCount ?? 0 })
    },
  })
}

export function useIgnoreImportAccount() {
  const apply = useApplyImportAccount()
  return useMutation({
    mutationFn: ({ sourceId, externalAccountId }: { sourceId: Id; externalAccountId: string }) =>
      importsApi.ignoreImportAccount(sourceId, externalAccountId),
    onSuccess: (result) => {
      apply(result)
      trackEvent(METRICS.IMPORT_ACCOUNT_IGNORE)
    },
  })
}

export function useUnlinkImportAccount() {
  const apply = useApplyImportAccount()
  return useMutation({
    mutationFn: ({ sourceId, externalAccountId }: { sourceId: Id; externalAccountId: string }) =>
      importsApi.unlinkImportAccount(sourceId, externalAccountId),
    onSuccess: apply,
  })
}

export function useImportQueuedEvent() {
  const queryClient = useQueryClient()
  const apply = useApplyTransactionItem()
  return useMutation({
    mutationFn: (payload: ImportQueuedEventPayload) => importsApi.importQueuedEvent(payload),
    onSuccess: (result, payload) => {
      apply(result, 'add')
      // the wire item carries isImported: 0 (the import link flips it after
      // the result is built), so the optimistic cache entry is stale until
      // the next refetch — invalidate rather than trust the returned badge.
      void queryClient.invalidateQueries({ queryKey: queryKeys.transactions })
      queryClient.setQueryData<ImportQueueDto>(queryKeys.importQueue, (prev) =>
        prev ? { ...prev, queued: prev.queued.filter((q) => q.linkId !== payload.linkId) } : prev)
      void queryClient.invalidateQueries({ queryKey: queryKeys.importSources })
      trackEvent(METRICS.IMPORT_QUEUE_IMPORT)
    },
  })
}

function useReplaceQueue() {
  const queryClient = useQueryClient()
  return (queue: ImportQueueDto) => {
    queryClient.setQueryData(queryKeys.importQueue, queue)
    void queryClient.invalidateQueries({ queryKey: queryKeys.importSources })
  }
}

export function useSkipQueuedEvent() {
  const replace = useReplaceQueue()
  return useMutation({
    mutationFn: importsApi.skipQueuedEvent,
    onSuccess: (queue) => {
      replace(queue)
      trackEvent(METRICS.IMPORT_QUEUE_SKIP)
    },
  })
}

export function useUnskipQueuedEvent() {
  const replace = useReplaceQueue()
  return useMutation({ mutationFn: importsApi.unskipQueuedEvent, onSuccess: replace })
}

export function useDiscardImportEvent() {
  const replace = useReplaceQueue()
  return useMutation({ mutationFn: importsApi.discardImportEvent, onSuccess: replace })
}

// A retry may create a transaction (status "created") or park the event
// somewhere else in the queue, so every cache it could have touched is refetched.
export function useRetryImportEvent() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: importsApi.retryImportEvent,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.importQueue })
      void queryClient.invalidateQueries({ queryKey: queryKeys.importSources })
      void queryClient.invalidateQueries({ queryKey: queryKeys.transactions })
      void queryClient.invalidateQueries({ queryKey: queryKeys.accounts })
      void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
    },
  })
}

export function useTransactionImportLinks(transactionId: Id, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.transactionImports(transactionId),
    queryFn: () => importsApi.getTransactionImportList(transactionId),
    enabled,
    staleTime: TEN_MINUTES,
  })
}
