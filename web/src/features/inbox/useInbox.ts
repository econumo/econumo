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
  /** queue AND sources have each resolved (success or error) at least once (no "All caught up" flash) */
  isLoaded: boolean
  /** the import queue or the source list request failed; either section may be showing stale/partial data */
  importsError: boolean
  retryImports: () => void
}

export function isSyncProblem(source: ImportSourceDto): boolean {
  // SimpleFIN is the only pull provider (syncs are manual, not scheduled). A
  // push provider (Apple Wallet) never syncs at all — its only "run" is a
  // card remap that leaves a leftover tap queued (e.g. no stored exchange
  // rate) and marks itself "partial"; that tap already surfaces under To
  // review, and the Apple Wallet page has no sync action to retry.
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
    isLoaded: (queue.data !== undefined || queue.isError) && (sources.data !== undefined || sources.isError),
    importsError: queue.isError || sources.isError,
    retryImports: () => {
      void queue.refetch()
      void sources.refetch()
    },
  }
}
