import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
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
  ImportSuggestionDto,
  UpdateImportAccountDto,
} from '@/api/dto/imports'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { METRICS, trackEvent } from '@/lib/metrics'
import { useApplyTransactionItem } from '@/features/transactions/queries'
import { retryPolicy, type OwnErrorHandling } from '@/lib/appQueryClient'
import { afterFailedPullSync, afterPullSync } from './sidebarSync'

// The queries reached from surfaces that exist with transaction import off
// (the Inbox, the transaction dialogs) never fetch then: the server does not
// mount the routes.
export function useImportSources() {
  return useQuery({ queryKey: queryKeys.importSources, queryFn: importsApi.getImportSourceList, staleTime: TEN_MINUTES })
}

export function useImportQueue() {
  return useQuery({ queryKey: queryKeys.importQueue, queryFn: importsApi.getImportQueue, staleTime: TEN_MINUTES })
}

// The server writes suggestions in the background after an import answers, and an Apple Wallet
// push reaches the server without this device knowing: a short staleTime plus a refetch on focus
// is how they show up.
export function useImportSuggestions() {
  return useQuery({
    queryKey: queryKeys.importSuggestions,
    queryFn: importsApi.getImportSuggestionList,
    staleTime: 60_000,
    refetchOnWindowFocus: true,
    retry: suggestionRetry,
  })
}

// A 404 is an older server without the route: asking again cannot help.
export function suggestionRetry(failureCount: number, error: unknown): boolean {
  return !(isAxiosError(error) && error.response?.status === 404) && retryPolicy(failureCount, error)
}

export { cancelSuggestionRefresh, scheduleSuggestionRefresh, SUGGESTION_REFRESH_DELAYS_MS } from './suggestionRefresh'

export interface SuggestionChanges {
  changedMatch: boolean
  changedCategory: boolean
  changedPayee: boolean
}

// The AI's match text for an edited import. An empty answer (no AI) and a
// failed request (an error with no data, a 429 included) both leave the
// heuristic in place; no error is surfaced.
export function useSuggestMatchText(linkId: Id, enabled: boolean) {
  return useQuery({
    queryKey: ['importMatchText', linkId],
    queryFn: () => importsApi.suggestImportMatchText(linkId),
    enabled,
    staleTime: Infinity,
    retry: false,
  })
}

// A refused accept or dismiss whose suggestion is no longer listed already
// happened: a retry after a lost answer, or another device. Answers the fresh
// list then, else null (the refusal stands).
async function listWithout(err: unknown, id: Id): Promise<ImportSuggestionDto[] | null> {
  if (!isAxiosError(err) || err.response === undefined || err.response.status >= 500) {
    return null
  }
  const items = await importsApi.getImportSuggestionList().catch(() => undefined)
  return items !== undefined && !items.some((s) => s.id === id) ? items : null
}

// suggestions an accept was sent for and not yet answered as done: only a retry may find its
// suggestion gone because its own first attempt landed; a first accept refused that way was
// settled elsewhere (a dismiss, the list's cleanup) and is an error. Module level, so Cancel and
// a reopen of the prompt keep it.
const acceptsSent = new Set<Id>()

export function useAcceptImportSuggestion() {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { handlesOwnErrors: true },
    mutationFn: async ({ id, ruleId }: { id: Id; ruleId: Id; changes: SuggestionChanges }): Promise<{ updated: number; alreadyGone?: true }> => {
      const retry = acceptsSent.has(id)
      acceptsSent.add(id)
      try {
        return await importsApi.acceptImportSuggestion(id, ruleId)
      } catch (err) {
        if (!retry || (await listWithout(err, id)) === null) throw err
        return { updated: 0, alreadyGone: true }
      }
    },
    onSuccess: (_result, { id, changes }) => {
      acceptsSent.delete(id)
      for (const queryKey of [queryKeys.importSuggestions, queryKeys.transactions, queryKeys.accounts, queryKeys.budget, queryKeys.budgetPlan, queryKeys.budgetTransactions]) {
        void queryClient.invalidateQueries({ queryKey })
      }
      void queryClient.invalidateQueries({ queryKey: ['transactionImports'] })
      trackEvent(METRICS.IMPORT_SUGGESTION_ACCEPT, { ...changes })
    },
  })
}

export function useDismissImportSuggestion() {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { handlesOwnErrors: true },
    mutationFn: async (id: Id) => {
      try {
        await importsApi.dismissImportSuggestion(id)
      } catch (err) {
        if ((await listWithout(err, id)) === null) throw err
      }
    },
    // Never replace the cache with the answer's list: two dismisses answered out of order would
    // bring the first one's row back. Drop this id and refetch.
    onSuccess: (_result, id) => {
      queryClient.setQueryData<ImportSuggestionDto[]>(queryKeys.importSuggestions, (prev) => prev?.filter((s) => s.id !== id))
      void queryClient.invalidateQueries({ queryKey: queryKeys.importSuggestions })
      trackEvent(METRICS.IMPORT_SUGGESTION_DISMISS)
    },
  })
}

export function useCreateImportSource({ handlesOwnErrors = false }: OwnErrorHandling = {}) {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { handlesOwnErrors },
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

export function useDeleteImportSource({ handlesOwnErrors = false }: OwnErrorHandling = {}) {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { handlesOwnErrors },
    mutationFn: importsApi.deleteImportSource,
    onSuccess: (items, id) => {
      const source = queryClient.getQueryData<ImportSourceDto[]>(queryKeys.importSources)?.find((item) => item.id === id)
      queryClient.setQueryData(queryKeys.importSources, items)
      void queryClient.invalidateQueries({ queryKey: queryKeys.importQueue })
      void queryClient.invalidateQueries({ queryKey: queryKeys.importSuggestions })
      trackEvent(METRICS.IMPORT_SOURCE_DISCONNECT, source ? { provider: source.provider } : {})
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
    void queryClient.invalidateQueries({ queryKey: queryKeys.importSuggestions })
    if (result.run) {
      void queryClient.invalidateQueries({ queryKey: queryKeys.transactions })
      void queryClient.invalidateQueries({ queryKey: queryKeys.accounts })
      void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
      void queryClient.invalidateQueries({ queryKey: queryKeys.budgetTransactions })
    }
  }
}

// Read before the response replaces the source in the cache. A card the cache
// does not hold yet (a SimpleFIN account listed but never stored) is unmapped.
function useCardBefore() {
  const queryClient = useQueryClient()
  return (sourceId: Id, externalAccountId: string) =>
    queryClient.getQueryData<ImportSourceDto[]>(queryKeys.importSources)
      ?.find((s) => s.id === sourceId)?.cards.find((c) => c.externalAccountId === externalAccountId)
}

export function useLinkImportAccount() {
  const apply = useApplyImportAccount()
  const cardBefore = useCardBefore()
  return useMutation({
    meta: { handlesOwnErrors: true },
    mutationFn: ({ sourceId, externalAccountId, accountId, externalName }: { sourceId: Id; externalAccountId: string; accountId: Id; externalName?: string }) =>
      importsApi.linkImportAccount(sourceId, externalAccountId, accountId, externalName),
    onSuccess: (result, { sourceId, externalAccountId, accountId }) => {
      const before = cardBefore(sourceId, externalAccountId)
      apply(result)
      const imported = result.run?.importedCount ?? 0
      const matched = result.run?.matchedCount ?? 0
      trackEvent(METRICS.IMPORT_ACCOUNT_LINK, { imported, matched })
      const provider = result.item.provider
      if (before?.state === 'mapped') {
        if (before.accountId !== accountId) {
          trackEvent(METRICS.IMPORT_ACCOUNT_REMAP, { provider, imported, matched })
        }
      } else {
        trackEvent(METRICS.IMPORT_ACCOUNT_MAP, { provider, from: before?.state ?? 'unmapped', imported, matched })
      }
    },
  })
}

export function useIgnoreImportAccount() {
  const apply = useApplyImportAccount()
  return useMutation({
    meta: { handlesOwnErrors: true },
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
    meta: { handlesOwnErrors: true },
    mutationFn: ({ sourceId, externalAccountId }: { sourceId: Id; externalAccountId: string }) =>
      importsApi.unlinkImportAccount(sourceId, externalAccountId),
    onSuccess: (result) => {
      apply(result)
      trackEvent(METRICS.IMPORT_ACCOUNT_UNMAP, { provider: result.item.provider })
    },
  })
}

export function useImportQueuedEvent() {
  const queryClient = useQueryClient()
  const apply = useApplyTransactionItem()
  return useMutation({
    mutationFn: (payload: ImportQueuedEventPayload) => importsApi.importQueuedEvent(payload),
    onSuccess: (result, payload) => {
      apply(result, 'add')
      // a skipped row can be imported too (from the import history)
      queryClient.setQueryData<ImportQueueDto>(queryKeys.importQueue, (prev) =>
        prev
          ? { ...prev, queued: prev.queued.filter((q) => q.linkId !== payload.linkId), skipped: prev.skipped.filter((q) => q.linkId !== payload.linkId) }
          : prev)
      void queryClient.invalidateQueries({ queryKey: queryKeys.importSources })
      void queryClient.invalidateQueries({ queryKey: ['importSourceHistory'] })
      void queryClient.invalidateQueries({ queryKey: queryKeys.importSuggestions })
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
    meta: { handlesOwnErrors: true },
    mutationFn: importsApi.skipQueuedEvent,
    onSuccess: (queue) => {
      replace(queue)
      trackEvent(METRICS.IMPORT_QUEUE_SKIP)
    },
  })
}

export function useUnskipQueuedEvent() {
  const replace = useReplaceQueue()
  return useMutation({ meta: { handlesOwnErrors: true }, mutationFn: importsApi.unskipQueuedEvent, onSuccess: replace })
}

export function useDiscardImportEvent({ handlesOwnErrors = false }: OwnErrorHandling = {}) {
  const replace = useReplaceQueue()
  return useMutation({ meta: { handlesOwnErrors }, mutationFn: importsApi.discardImportEvent, onSuccess: replace })
}

// A retry may create a transaction (status "created") or park the event
// somewhere else in the queue, so every cache it could have touched is refetched.
export function useRetryImportEvent() {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { handlesOwnErrors: true },
    mutationFn: importsApi.retryImportEvent,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.importQueue })
      void queryClient.invalidateQueries({ queryKey: queryKeys.importSuggestions })
      void queryClient.invalidateQueries({ queryKey: queryKeys.importSources })
      void queryClient.invalidateQueries({ queryKey: queryKeys.transactions })
      void queryClient.invalidateQueries({ queryKey: queryKeys.accounts })
      void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
    },
  })
}

// "Import again" on a row whose transaction was deleted: it may create a
// transaction or adopt an existing one (a transfer entered by hand), and the
// row gets a new id in its run and history.
export function useReimportImportLink() {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { handlesOwnErrors: true },
    mutationFn: importsApi.reimportLink,
    onSuccess: (res) => {
      for (const key of [['importRun'], ['importSourceHistory'], queryKeys.importQueue, queryKeys.importSources, queryKeys.transactions, queryKeys.accounts, queryKeys.budget, queryKeys.budgetTransactions]) {
        void queryClient.invalidateQueries({ queryKey: key })
      }
      trackEvent(METRICS.IMPORT_REIMPORT, { status: res.status })
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
    meta: { handlesOwnErrors: true },
    mutationFn: importsApi.setImportCredentialKey,
    onSuccess: (key) => queryClient.setQueryData(queryKeys.importCredentialKey, key),
  })
}

export function useClaimSetupToken() {
  return useMutation({ meta: { handlesOwnErrors: true, skipInvalidate: true }, mutationFn: importsApi.claimSetupToken })
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
    meta: { handlesOwnErrors: true },
    mutationFn: ({ sourceId, accessUrl, startDate, endDate }: { sourceId: Id; accessUrl: string; startDate: string; endDate?: string }) =>
      importsApi.syncImportSource(sourceId, accessUrl, startDate, endDate),
    onSuccess: (result, vars) => afterPullSync(queryClient, vars.sourceId, result, 'settings'),
    onError: () => afterFailedPullSync(queryClient),
  })
}

// Runs are listed per source: each provider shows its own history.
export function useImportRuns(sourceId: Id | undefined) {
  return useQuery({
    queryKey: queryKeys.importRuns(sourceId ?? ''),
    queryFn: () => importsApi.getImportRunList(sourceId),
    staleTime: TEN_MINUTES,
    enabled: Boolean(sourceId),
  })
}

export function useImportRun(id: Id) {
  return useQuery({ queryKey: queryKeys.importRun(id), queryFn: () => importsApi.getImportRun(id), staleTime: TEN_MINUTES })
}

// Taps keep arriving while the page is closed, so it refetches every time it
// opens rather than trusting a cached copy.
export function useImportSourceHistory(sourceId: Id | undefined) {
  return useInfiniteQuery({
    queryKey: queryKeys.importSourceHistory(sourceId ?? ''),
    queryFn: ({ pageParam }) => importsApi.getImportSourceHistory(sourceId ?? '', pageParam),
    initialPageParam: 0,
    getNextPageParam: (last, pages) => (last.hasMore ? pages.reduce((n, p) => n + p.items.length, 0) : undefined),
    enabled: Boolean(sourceId),
  })
}

export function useImportRules() {
  return useQuery({ queryKey: queryKeys.importRules, queryFn: importsApi.getImportRuleList, staleTime: TEN_MINUTES })
}

export function useCreateImportRule() {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { handlesOwnErrors: true },
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
    meta: { handlesOwnErrors: true },
    mutationFn: ({ id, spec }: { id: Id; spec: ImportRuleSpecDto }) => importsApi.updateImportRule(id, spec),
    onSuccess: (rule) => {
      queryClient.setQueryData<ImportRuleDto[]>(queryKeys.importRules, (prev = []) => prev.map((r) => (r.id === rule.id ? rule : r)))
    },
  })
}

export function useDeleteImportRule() {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { handlesOwnErrors: true },
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
    meta: { skipInvalidate: true },
    mutationFn: ({ spec, scope }: { spec: ImportRuleSpecDto; scope: ImportRuleScopeDto }) => importsApi.previewImportRule(spec, scope),
  })
}

export function useApplyImportRule() {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { handlesOwnErrors: true },
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
