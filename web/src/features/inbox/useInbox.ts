import type { AccountDto } from '@/api/dto/account'
import type { ImportFailedEventDto, ImportQueuedEventDto, ImportSourceDto } from '@/api/dto/imports'
import type { RecurringDto } from '@/api/dto/recurring'
import type { Id } from '@/api/types'
import { useAccounts } from '@/features/accounts/queries'
import { usePendingInvites, type PendingInvite } from '@/features/connections/pendingInvites'
import { canWriteToAccount } from '@/features/connections/shared'
import { useImportQueue, useImportSources } from '@/features/imports/queries'
import { useRecurring } from '@/features/recurring/queries'
import { useUserData } from '@/features/user/queries'
import { isFuture } from '@/lib/datetime'

export interface Inbox {
  /** templates due today or earlier on an account I can post to, oldest first */
  dueRecurring: RecurringDto[]
  invites: PendingInvite[]
  syncProblems: ImportSourceDto[]
  failed: ImportFailedEventDto[]
  queued: ImportQueuedEventDto[]
  skipped: ImportQueuedEventDto[]
  /** dueRecurring + invites + syncProblems + failed + queued; skipped is never counted */
  count: number
  /** queue, sources AND recurring have each resolved (success or error) at least once (no "All caught up" flash) */
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

// A partner's template on a joint account shows for both of us — either can
// post it, and it leaves both inboxes once one does. Read-only shares are left
// out: their preview has nothing to act on.
export function isDueRecurring(rt: RecurringDto, accounts: AccountDto[] | undefined, meId: Id | undefined): boolean {
  if (isFuture(rt.nextPaymentAt)) return false
  const account = accounts?.find((a) => a.id === rt.accountId)
  return !!account && canWriteToAccount(account, meId)
}

export function formatInboxCount(count: number): string {
  return count > 99 ? '99+' : String(count)
}

export function useInbox(): Inbox {
  const { invites } = usePendingInvites()
  const queue = useImportQueue()
  const sources = useImportSources()
  const recurring = useRecurring()
  const { data: accounts } = useAccounts()
  const { data: user } = useUserData()
  // useRecurring already sorts by next payment, so the oldest debt leads
  const dueRecurring = (recurring.data ?? []).filter((rt) => isDueRecurring(rt, accounts, user?.id))
  const syncProblems = (sources.data ?? []).filter(isSyncProblem)
  const queued = queue.data?.queued ?? []
  const skipped = queue.data?.skipped ?? []
  const failed = queue.data?.failed ?? []
  return {
    dueRecurring,
    invites,
    syncProblems,
    failed,
    queued,
    skipped,
    count: dueRecurring.length + invites.length + syncProblems.length + failed.length + queued.length,
    isLoaded:
      (queue.data !== undefined || queue.isError) &&
      (sources.data !== undefined || sources.isError) &&
      (recurring.data !== undefined || recurring.isError),
    importsError: queue.isError || sources.isError,
    retryImports: () => {
      void queue.refetch()
      void sources.refetch()
    },
  }
}
