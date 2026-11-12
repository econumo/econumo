import { onlineManager, type QueryClient } from '@tanstack/react-query'
import * as importsApi from '@/api/imports'
import type { Id } from '@/api/types'
import type { SyncImportSourceResultDto } from '@/api/dto/imports'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { isSimpleFINImportEnabled } from '@/lib/config'
import { decryptCredential, loadStoredKey } from '@/lib/importCrypto'
import { METRICS, trackEvent } from '@/lib/metrics'
import { syncStartDate } from './syncWindow'

// The sidebar's sync button also pulls SimpleFIN, but at most once per
// interval: every sync is a real round trip to the bank bridge. Any sync from
// this device counts (the SimpleFIN page's button too, which has no limit),
// so a tap right after a manual sync does not sync again.
export const SIDEBAR_SYNC_INTERVAL_MS = 10 * 60 * 1000
const LAST_SYNC_KEY = 'econumo.simplefinSyncAt'

export function notePullSync(now = Date.now()) {
  try {
    localStorage.setItem(LAST_SYNC_KEY, String(now))
  } catch {
    // no storage: the button may sync on every tap, which the server's own rate limit still caps
  }
}

function lastPullSync(): number {
  try {
    return Number(localStorage.getItem(LAST_SYNC_KEY)) || 0
  } catch {
    return 0
  }
}

// What a sync changes, refreshed the same way whichever surface ran it.
export function afterPullSync(queryClient: QueryClient, sourceId: Id, result: SyncImportSourceResultDto, from: 'settings' | 'sidebar') {
  notePullSync()
  queryClient.setQueryData(queryKeys.importExternalAccounts(sourceId), result.accounts)
  void queryClient.invalidateQueries({ queryKey: queryKeys.importSources })
  void queryClient.invalidateQueries({ queryKey: queryKeys.importQueue })
  void queryClient.invalidateQueries({ queryKey: ['importRuns'] })
  if (result.run.importedCount + result.run.matchedCount + result.run.amountsUpdatedCount > 0) {
    void queryClient.invalidateQueries({ queryKey: queryKeys.transactions })
    void queryClient.invalidateQueries({ queryKey: queryKeys.accounts })
    void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
    void queryClient.invalidateQueries({ queryKey: queryKeys.budgetTransactions })
  }
  trackEvent(METRICS.IMPORT_SYNC, { trigger: result.run.trigger, imported: result.run.importedCount, matched: result.run.matchedCount, from })
}

// a failed sync still wrote a run row, and the source's lastRunStatus must
// reach the Inbox badge immediately
export function afterFailedPullSync(queryClient: QueryClient) {
  void queryClient.invalidateQueries({ queryKey: ['importRuns'] })
  void queryClient.invalidateQueries({ queryKey: queryKeys.importSources })
}

let running = false

export interface SidebarSyncTotals {
  imported: number
  matched: number
  failed: number
}

// Syncs every connected SimpleFIN source when it is due and this device can
// decrypt the access URL; otherwise does nothing and resolves null. Silent on
// failure: a failing sync shows up in the Inbox.
export async function syncPullSourcesIfDue(queryClient: QueryClient, now = Date.now()): Promise<SidebarSyncTotals | null> {
  if (running || !window.isSecureContext || !isSimpleFINImportEnabled() || !onlineManager.isOnline()) {
    return null
  }
  if (now - lastPullSync() < SIDEBAR_SYNC_INTERVAL_MS) {
    return null
  }
  running = true
  try {
    const sources = await queryClient.fetchQuery({ queryKey: queryKeys.importSources, queryFn: importsApi.getImportSourceList, staleTime: TEN_MINUTES })
    const pull = sources.filter((s) => s.provider === 'simplefin' && s.credentialCiphertext)
    if (pull.length === 0) {
      return null
    }
    const [wrapped, local] = await Promise.all([
      queryClient.fetchQuery({ queryKey: queryKeys.importCredentialKey, queryFn: importsApi.getImportCredentialKey, staleTime: TEN_MINUTES }),
      loadStoredKey(),
    ])
    if (!wrapped || local?.tag !== wrapped.wrappedDataKey) {
      return null // locked on this device: only the SimpleFIN page asks for the passphrase
    }
    notePullSync(now)
    const totals: SidebarSyncTotals = { imported: 0, matched: 0, failed: 0 }
    // one source at a time, like the Data page: each sync holds the single SQLite writer
    for (const source of pull) {
      try {
        const accessUrl = await decryptCredential(source.credentialCiphertext)
        const result = await importsApi.syncImportSource(source.id, accessUrl, syncStartDate(source))
        afterPullSync(queryClient, source.id, result, 'sidebar')
        totals.imported += result.run.importedCount
        totals.matched += result.run.matchedCount
        if (result.run.status !== 'completed') {
          totals.failed += 1
        }
      } catch {
        totals.failed += 1
        afterFailedPullSync(queryClient)
      }
    }
    return totals
  } catch {
    return null
  } finally {
    running = false
  }
}
