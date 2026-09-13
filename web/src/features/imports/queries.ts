import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { v7 as uuidv7 } from 'uuid'
import * as importsApi from '@/api/imports'
import type { Id } from '@/api/types'
import type {
  ImportProvider,
  ImportQueueDto,
  ImportQueuedEventPayload,
  ImportRuleDto,
  ImportRuleScopeDto,
  ImportRuleSpecDto,
  ImportSourceDto,
  UpdateImportAccountDto,
} from '@/api/dto/imports'
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
    mutationFn: ({ provider, name, credentialCiphertext }: { provider: ImportProvider; name: string; credentialCiphertext?: string }) =>
      importsApi.createImportSource(provider, name, credentialCiphertext),
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
    mutationFn: ({ sourceId, externalAccountId, accountId, externalName }: { sourceId: Id; externalAccountId: string; accountId: Id; externalName?: string }) =>
      importsApi.linkImportAccount(sourceId, externalAccountId, accountId, externalName),
    onSuccess: (result) => {
      apply(result)
      trackEvent(METRICS.IMPORT_ACCOUNT_LINK, { imported: result.run?.importedCount ?? 0, matched: result.run?.matchedCount ?? 0 })
    },
  })
}

export function useIgnoreImportAccount() {
  const apply = useApplyImportAccount()
  return useMutation({
    mutationFn: ({ sourceId, externalAccountId, externalName }: { sourceId: Id; externalAccountId: string; externalName?: string }) =>
      importsApi.ignoreImportAccount(sourceId, externalAccountId, externalName),
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

export function useImportCredentialKey() {
  return useQuery({ queryKey: queryKeys.importCredentialKey, queryFn: importsApi.getImportCredentialKey, staleTime: TEN_MINUTES })
}

export function useSetImportCredentialKey() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: importsApi.setImportCredentialKey,
    onSuccess: (key) => queryClient.setQueryData(queryKeys.importCredentialKey, key),
  })
}

export function useClaimSetupToken() {
  return useMutation({ mutationFn: importsApi.claimSetupToken })
}

// The access URL is a per-request secret: it is the query's input, never
// part of the key (keys are visible in devtools and persisted caches).
export function useExternalAccounts(sourceId: Id, accessUrl: string | null) {
  return useQuery({
    queryKey: queryKeys.importExternalAccounts(sourceId),
    queryFn: () => importsApi.listExternalAccounts(sourceId, accessUrl ?? ''),
    enabled: accessUrl !== null,
    staleTime: TEN_MINUTES,
    gcTime: 0,
  })
}

export function useSyncImportSource() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ sourceId, accessUrl, startDate, endDate }: { sourceId: Id; accessUrl: string; startDate: string; endDate?: string }) =>
      importsApi.syncImportSource(sourceId, accessUrl, startDate, endDate),
    onSuccess: (result, vars) => {
      queryClient.setQueryData(queryKeys.importExternalAccounts(vars.sourceId), result.accounts)
      void queryClient.invalidateQueries({ queryKey: queryKeys.importSources })
      void queryClient.invalidateQueries({ queryKey: queryKeys.importQueue })
      void queryClient.invalidateQueries({ queryKey: ['importRuns'] })
      if (result.run.importedCount + result.run.matchedCount + result.run.amountsUpdatedCount > 0) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.transactions })
        void queryClient.invalidateQueries({ queryKey: queryKeys.accounts })
        void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
        void queryClient.invalidateQueries({ queryKey: queryKeys.budgetTransactions })
      }
      trackEvent(METRICS.IMPORT_SYNC, { trigger: result.run.trigger, imported: result.run.importedCount, matched: result.run.matchedCount })
    },
    // a failed sync still wrote a run row
    onError: () => void queryClient.invalidateQueries({ queryKey: ['importRuns'] }),
  })
}

export function useImportRuns(sourceId = '') {
  return useQuery({ queryKey: queryKeys.importRuns(sourceId), queryFn: () => importsApi.getImportRunList(sourceId || undefined), staleTime: TEN_MINUTES })
}

export function useImportRun(id: Id) {
  return useQuery({ queryKey: queryKeys.importRun(id), queryFn: () => importsApi.getImportRun(id), staleTime: TEN_MINUTES })
}

export function useImportRules() {
  return useQuery({ queryKey: queryKeys.importRules, queryFn: importsApi.getImportRuleList, staleTime: TEN_MINUTES })
}

export function useCreateImportRule() {
  const queryClient = useQueryClient()
  return useMutation({
    // The id is the create's idempotency key and the server requires it, so it
    // is minted HERE — the one choke point every call site goes through.
    mutationFn: ({ spec, id }: { spec: ImportRuleSpecDto; id?: Id }) => importsApi.createImportRule(spec, id ?? uuidv7()),
    onSuccess: (rule) => {
      queryClient.setQueryData<ImportRuleDto[]>(queryKeys.importRules, (prev = []) => [...prev.filter((r) => r.id !== rule.id), rule])
      trackEvent(METRICS.IMPORT_RULE_CREATE, { action: rule.action })
    },
  })
}

export function useUpdateImportRule() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, spec }: { id: Id; spec: ImportRuleSpecDto }) => importsApi.updateImportRule(id, spec),
    onSuccess: (rule) => {
      queryClient.setQueryData<ImportRuleDto[]>(queryKeys.importRules, (prev = []) => prev.map((r) => (r.id === rule.id ? rule : r)))
    },
  })
}

export function useDeleteImportRule() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: Id) => importsApi.deleteImportRule(id),
    onSuccess: (_void, id) => {
      queryClient.setQueryData<ImportRuleDto[]>(queryKeys.importRules, (prev = []) => prev.filter((r) => r.id !== id))
    },
  })
}

// A preview is a read that travels as a POST (the spec is the body); it is
// never cached — the count must reflect the value the user is typing.
export function usePreviewImportRule() {
  return useMutation({
    mutationFn: ({ spec, scope }: { spec: ImportRuleSpecDto; scope: ImportRuleScopeDto }) => importsApi.previewImportRule(spec, scope),
  })
}

export function useApplyImportRule() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ ruleId, scope, includeEdited }: { ruleId: Id; scope: ImportRuleScopeDto; includeEdited: boolean }) =>
      importsApi.applyImportRule(ruleId, scope, includeEdited),
    onSuccess: (result) => {
      trackEvent(METRICS.IMPORT_RULE_APPLY, { updated: result.updated, skipped: result.skipped })
      void queryClient.invalidateQueries({ queryKey: queryKeys.transactions })
      void queryClient.invalidateQueries({ queryKey: queryKeys.accounts })
      void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
      // an apply rewrote the rows' applied_* snapshots; the per-transaction
      // link caches hold them for ten minutes, and a stale snapshot is what
      // the post-edit rule prompt compares against
      void queryClient.invalidateQueries({ queryKey: ['transactionImports'] })
    },
  })
}

export function useSuggestImportRules() {
  return useMutation({
    mutationFn: (scope: ImportRuleScopeDto) => importsApi.suggestImportRules(scope),
    onSuccess: (items) => trackEvent(METRICS.IMPORT_RULES_SUGGEST, { count: items.length }),
  })
}
