import type { ImportFailedEventDto, ImportQueuedEventDto, ImportSourceDto } from '@/api/dto/imports'
import { usePendingInvites, type PendingInvite } from '@/features/connections/pendingInvites'
import { useImportQueue, useImportSources } from '@/features/imports/queries'

export interface Inbox {
  invites: PendingInvite[]
  syncProblems: ImportSourceDto[]
  failed: ImportFailedEventDto[]
  queued: ImportQueuedEventDto[]
  skipped: ImportQueuedEventDto[]
  /** invites + syncProblems + failed + queued; skipped is never counted */
  count: number
  /** queue AND sources have resolved at least once (no "All caught up" flash) */
  isLoaded: boolean
  /** the import queue request failed; imports sections are unknown */
  importsError: boolean
  retryImports: () => void
}

export function isSyncProblem(source: ImportSourceDto): boolean {
  // Only SimpleFIN sources pull on a schedule; a push provider's (Apple
  // Wallet) only run is a card remap that leaves taps queued (e.g. no stored
  // exchange rate) and marks itself "partial" — those taps already surface
  // under To review, and the Apple Wallet page has no sync action to retry.
  if (source.provider !== 'simplefin') return false
  return source.lastRunStatus === 'failed' || source.lastRunStatus === 'partial'
}

export function formatInboxCount(count: number): string {
  return count > 99 ? '99+' : String(count)
}

export function useInbox(): Inbox {
  const { invites } = usePendingInvites()
  const queue = useImportQueue()
  const sources = useImportSources()
  const syncProblems = (sources.data ?? []).filter(isSyncProblem)
  const queued = queue.data?.queued ?? []
  const skipped = queue.data?.skipped ?? []
  const failed = queue.data?.failed ?? []
  return {
    invites,
    syncProblems,
    failed,
    queued,
    skipped,
    count: invites.length + syncProblems.length + failed.length + queued.length,
    isLoaded: queue.data !== undefined && sources.data !== undefined,
    importsError: queue.isError,
    retryImports: () => void queue.refetch(),
  }
}
