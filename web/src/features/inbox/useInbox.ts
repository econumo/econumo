import type { AccountDto } from '@/api/dto/account'
import type { ImportFailedEventDto, ImportQueuedEventDto, ImportSourceDto } from '@/api/dto/imports'
import type { RecurringDto } from '@/api/dto/recurring'
import type { Id } from '@/api/types'
import { useAccounts, useFolders } from '@/features/accounts/queries'
import { usePendingInvites, type PendingInvite } from '@/features/connections/pendingInvites'
import { canWriteToAccount } from '@/features/connections/shared'
import { useImportQueue, useImportSources } from '@/features/imports/queries'
import { useRecurring } from '@/features/recurring/queries'
import { useFailedWrites, useStoredQueueSettled } from '@/features/pendingWrites/hooks'
import type { PendingOp } from '@/features/pendingWrites/types'
import { hiddenFolderAccountIds } from '@/features/transactions/useTransactionForm'
import { useUserData } from '@/features/user/queries'
import { isFuture } from '@/lib/datetime'

export interface Inbox {
  /** changes made on this device that the server refused */
  failedWrites: PendingOp[]
  /** templates due today or earlier on an account I can post to, oldest first */
  dueRecurring: RecurringDto[]
  invites: PendingInvite[]
  syncProblems: ImportSourceDto[]
  failed: ImportFailedEventDto[]
  queued: ImportQueuedEventDto[]
  skipped: ImportQueuedEventDto[]
  /** failedWrites + dueRecurring + invites + syncProblems + failed + queued; skipped is never counted */
  count: number
  /** the changes stored on this device, the import queue, sources AND recurring have each resolved (success or error) at least once (no "All caught up" flash) */
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
  const failedWrites = useFailedWrites()
  const storedQueueSettled = useStoredQueueSettled()
  const { invites } = usePendingInvites()
  const queue = useImportQueue()
  const sources = useImportSources()
  const recurring = useRecurring()
  const { data: accounts } = useAccounts()
  const folders = useFolders()
  const foldersKnown = folders.data !== undefined || folders.isError
  const { data: user } = useUserData()
  // a hidden folder's accounts are out of the way on purpose, so they don't ask for attention
  const hiddenAccountIds = hiddenFolderAccountIds(accounts ?? [], folders.data ?? [])
  // useRecurring already sorts by next payment, so the oldest debt leads
  const dueRecurring = foldersKnown
    ? (recurring.data ?? []).filter((rt) => !hiddenAccountIds.has(rt.accountId) && isDueRecurring(rt, accounts, user?.id))
    : []
  const syncProblems = (sources.data ?? []).filter(isSyncProblem)
  const queued = queue.data?.queued ?? []
  const skipped = queue.data?.skipped ?? []
  const failed = queue.data?.failed ?? []
  return {
    failedWrites,
    dueRecurring,
    invites,
    syncProblems,
    failed,
    queued,
    skipped,
    count: failedWrites.length + dueRecurring.length + invites.length + syncProblems.length + failed.length + queued.length,
    isLoaded:
      storedQueueSettled &&
      (queue.data !== undefined || queue.isError) &&
      (sources.data !== undefined || sources.isError) &&
      (recurring.data !== undefined || recurring.isError) &&
      foldersKnown,
    importsError: queue.isError || sources.isError,
    retryImports: () => {
      void queue.refetch()
      void sources.refetch()
    },
  }
}
