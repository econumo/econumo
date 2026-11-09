import { notifyManager, onlineManager, type QueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { getAccountList } from '@/api/account'
import { setLimit } from '@/api/budget'
import { createTransaction, deleteTransaction, updateTransaction } from '@/api/transaction'
import type { WriteOptions } from '@/api/client'
import type { BudgetDto, BudgetPlanDto } from '@/api/dto/budget'
import type { TransactionItemDto } from '@/api/dto/transaction'
import type { Id } from '@/api/types'
import { queryKeys } from '@/app/queryKeys'
import { applyTransactionItem } from '@/features/transactions/queries'
import { consumeFullReload, transactionsQueryFn } from '@/features/transactions/revisionTracking'
import { apiErrorMessage } from '@/lib/apiError'
import { invalidateReports } from '@/lib/appQueryClient'
import { METRICS, trackEvent } from '@/lib/metrics'
import { getSessionUserId, hasToken } from '@/lib/storage'
import { classifyWriteError } from './classify'
import { applyLimitToBudget, applyLimitToPlan } from './overlay'
import { markLanded, nextSendable, resolveTempId, settleDone, settleRefused, settleUnresolved } from './queue'
import { claimForSend, getOps, isQueueLoaded, loadPendingWrites, mutate, queueOwner, refreshFromDisk, subscribe, subscribeAdopted, subscribeLeftElsewhere } from './store'
import { rememberRealId } from './tempIds'
import type { OpChange, PendingOp } from './types'

export const WRITE_TIMEOUT_MS = 10_000

const BACKOFF_MIN_MS = 2_000
const BACKOFF_MAX_MS = 60_000
const MAX_RETRIES = 5
const LOCK_NAME = 'econumo-pending-writes'
const LOCK_RETRY_MS = 2_000

const CONFIRM_METRIC = {
  'tx-create': METRICS.TRANSACTION_CREATE,
  'tx-update': METRICS.TRANSACTION_UPDATE,
  'tx-delete': METRICS.TRANSACTION_DELETE,
  limit: METRICS.BUDGET_UPDATE_ELEMENT_LIMIT,
} as const

type LimitChange = Extract<OpChange, { kind: 'limit' }>

const APPLY_MODE = { 'tx-create': 'add', 'tx-update': 'update', 'tx-delete': 'delete' } as const

let client: QueryClient | null = null
let unsubscribe: (() => void) | null = null
let running: Promise<void> | null = null
let rerun = false
let wakeTimer: ReturnType<typeof setTimeout> | null = null
let networkFailures = 0
let blockedUntil = 0

const backoff = (failures: number) => Math.min(BACKOFF_MIN_MS * 2 ** failures, BACKOFF_MAX_MS)

function wakeIn(ms: number): void {
  if (wakeTimer !== null) {
    clearTimeout(wakeTimer)
  }
  wakeTimer = setTimeout(() => {
    wakeTimer = null
    void run()
  }, ms)
}

export function sessionUser(): string | null {
  return hasToken() ? getSessionUserId() : null
}

function send(op: PendingOp): Promise<unknown> {
  const opts: WriteOptions = { idempotencyKey: op.key, timeout: WRITE_TIMEOUT_MS }
  switch (op.kind) {
    case 'tx-create': return createTransaction(op.payload, opts)
    case 'tx-update': return updateTransaction(op.payload, opts)
    case 'tx-delete': return deleteTransaction(op.txId, opts)
    case 'limit': return setLimit({ budgetId: op.budgetId, elementId: op.elementId, period: op.period, amount: op.amount }, opts)
  }
}

function invalidateChange(queryClient: QueryClient, change: OpChange): void {
  if (change.kind === 'limit') {
    void queryClient.invalidateQueries({ queryKey: [...queryKeys.budget, change.budgetId] })
    void queryClient.invalidateQueries({ queryKey: [...queryKeys.budgetPlan, change.budgetId] })
  } else {
    void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
    void queryClient.invalidateQueries({ queryKey: queryKeys.budgetPlan })
    void queryClient.invalidateQueries({ queryKey: queryKeys.budgetTransactions })
  }
}

// The overlay stops showing a limit the moment its op leaves, while queueing put
// the raw caches back to the old value: they take the new one first. Before any
// invalidation of them, since a cache write clears that flag.
function applyAcceptedLimit(queryClient: QueryClient, change: LimitChange): void {
  queryClient.setQueriesData<BudgetDto | null>(
    { queryKey: [...queryKeys.budget, change.budgetId, change.period] },
    (budget) => (budget ? applyLimitToBudget(budget, change) : budget),
  )
  queryClient.setQueriesData<BudgetPlanDto | null>(
    { queryKey: [...queryKeys.budgetPlan, change.budgetId] },
    (plan) => (plan ? applyLimitToPlan(plan, change) : plan),
  )
}

// The queue is not a mutation, so the MutationCache's report invalidation does not see it.
function acknowledge(queryClient: QueryClient, change: OpChange): void {
  invalidateReports(queryClient)
  trackEvent(CONFIRM_METRIC[change.kind])
}

/** The one place a write the server accepted reaches the caches and the analytics. */
export function confirmChange(queryClient: QueryClient, change: OpChange, result: unknown): void {
  if (change.kind === 'limit') {
    applyAcceptedLimit(queryClient, change)
    invalidateChange(queryClient, change)
  } else {
    applyTransactionItem(queryClient, result as TransactionItemDto, APPLY_MODE[change.kind])
  }
  acknowledge(queryClient, change)
}

// The tab that held the lock applied the server's answer to its own caches
// only; here the change just left the overlay, so its result must be read back.
function refreshAfterOtherTab(queryClient: QueryClient): void {
  for (const queryKey of [queryKeys.transactions, queryKeys.accounts, queryKeys.budget, queryKeys.budgetPlan, queryKeys.budgetTransactions]) {
    void queryClient.invalidateQueries({ queryKey })
  }
  invalidateReports(queryClient)
}

function isQueued(op: PendingOp): boolean {
  return getOps().some((queued) => queued.key === op.key)
}

function resolveAdded(ops: PendingOp[], tempId: Id, realId: Id): PendingOp[] {
  rememberRealId(tempId, realId)
  return resolveTempId(ops, tempId, realId)
}

function settleAccepted(queryClient: QueryClient, op: PendingOp, result: unknown): Promise<void> {
  if (!isQueued(op)) {
    return Promise.resolve()
  }
  // One synchronous step: observers never see the server's row beside the
  // temporary one, or neither of them.
  return notifyManager.batch(() => {
    confirmChange(queryClient, op, result)
    return mutate((ops) => {
      const remaining = settleDone(ops, op.key)
      return op.kind === 'tx-create' ? resolveAdded(remaining, op.txId, (result as TransactionItemDto).item.id) : remaining
    })
  })
}

// Accounts first: the transaction sync goes by their revisions, and an accounts
// fetch it would join may predate the write that landed. Their balances count
// the change, so they arrive in one step with the mark that stops the overlay
// counting it too. The transactions take the sync's own path, which alone keeps
// its revision bookkeeping right, while the mark hides the row beside the temporary one.
async function readTransactionsBack(queryClient: QueryClient, key: string, realId: Id | undefined): Promise<boolean> {
  let timer: ReturnType<typeof setTimeout> | undefined
  const timedOut = new Promise<never>((_, reject) => {
    timer = setTimeout(() => reject(new Error('Reading the replayed change back timed out')), WRITE_TIMEOUT_MS)
  })
  const read = (async () => {
    const owner = sessionUser()
    await queryClient.cancelQueries({ queryKey: queryKeys.accounts, exact: true })
    const accounts = await getAccountList()
    // the read ran outside the query lifecycle, so a sign-out's cache purge cannot discard it
    if (sessionUser() !== owner || !getOps().some((op) => op.key === key)) {
      throw new Error('The queue changed while reading the replayed change back')
    }
    notifyManager.batch(() => {
      void mutate((ops) => markLanded(ops, key, realId))
      queryClient.setQueryData(queryKeys.accounts, accounts)
    })
    // fetchQuery joins a sync already running, and that one may have read the revisions before the write
    await queryClient.cancelQueries({ queryKey: queryKeys.transactions, exact: true })
    await queryClient.fetchQuery({
      queryKey: queryKeys.transactions,
      queryFn: ({ signal }) => transactionsQueryFn(queryClient, { full: consumeFullReload(), signal }),
      staleTime: 0,
      retry: false,
    })
  })()
  try {
    await Promise.race([read, timedOut])
    return true
  } catch {
    return false
  } finally {
    clearTimeout(timer)
  }
}

function holdOff(): void {
  blockedUntil = Date.now() + backoff(networkFailures)
  wakeIn(backoff(networkFailures))
  networkFailures++
}

// A replay proves an earlier attempt landed, but its response was lost, so all
// that is left is to read the outcome back from the server. A transaction change
// stays in the overlay until that read has landed: settled sooner, an add would
// vanish and a delete or an edit revert in the meantime, for as long as the read
// cannot finish. False means it could not, and the attempt is repeated later.
async function settleReplayed(queryClient: QueryClient, op: PendingOp, err: unknown): Promise<boolean> {
  if (!isQueued(op)) {
    return true
  }
  if (op.kind === 'limit') {
    applyAcceptedLimit(queryClient, op)
  } else {
    if (!(await readTransactionsBack(queryClient, op.key, op.kind === 'tx-create' ? replayResourceId(err) : undefined))) {
      holdOff()
      return false
    }
    if (!isQueued(op)) {
      return true
    }
  }
  networkFailures = 0
  const written = mutate((ops) => {
    if (op.kind !== 'tx-create') {
      return settleDone(ops, op.key)
    }
    const realId = replayResourceId(err)
    return realId ? resolveAdded(settleDone(ops, op.key), op.txId, realId) : settleUnresolved(ops, op.key, apiErrorMessage(err))
  })
  invalidateChange(queryClient, op)
  acknowledge(queryClient, op)
  await written
  return true
}

function replayResourceId(err: unknown): Id | undefined {
  return isAxiosError<{ resourceId?: Id }>(err) ? err.response?.data?.resourceId || undefined : undefined
}

function retryLater(ops: PendingOp[], key: string, retries: number): PendingOp[] {
  return ops.map((op) => (op.key === key ? { ...op, retries, notBefore: Date.now() + backoff(retries - 1) } : op))
}

async function settleFailure(queryClient: QueryClient, op: PendingOp, err: unknown): Promise<boolean> {
  const outcome = classifyWriteError(err)
  if (outcome === 'wait') {
    holdOff()
    return false
  }
  if (outcome === 'done') {
    return settleReplayed(queryClient, op, err)
  }
  networkFailures = 0
  if (outcome === 'pause') {
    return false
  }
  if (outcome === 'refused' || op.retries >= MAX_RETRIES) {
    await mutate((ops) => settleRefused(ops, op.key, apiErrorMessage(err)))
  } else {
    await mutate((ops) => retryLater(ops, op.key, op.retries + 1))
  }
  return true
}

async function sendNext(queryClient: QueryClient, next: PendingOp): Promise<boolean> {
  // Before the request leaves: a reload after this, or another tab, must not
  // fold new values under a key the server may already hold.
  const op = await claimForSend(next.key)
  if (!op) {
    // settled or dropped in another tab, whose announcement has not arrived
    await refreshFromDisk()
    return true
  }
  let result: unknown
  try {
    result = await send(op)
  } catch (err) {
    return settleFailure(queryClient, op, err)
  }
  networkFailures = 0
  await settleAccepted(queryClient, op, result)
  return true
}

function wakeForNextRetry(): void {
  const now = Date.now()
  const later = getOps().filter((op) => op.status === 'waiting' && op.notBefore > now).map((op) => op.notBefore)
  if (later.length > 0) {
    wakeIn(Math.min(...later) - now)
  }
}

async function sendAll(queryClient: QueryClient): Promise<void> {
  while (client === queryClient && onlineManager.isOnline() && sessionUser() === queueOwner()) {
    const op = nextSendable(getOps(), Date.now())
    if (!op) {
      wakeForNextRetry()
      return
    }
    if (!(await sendNext(queryClient, op))) {
      return
    }
  }
}

// One tab sends at a time. Without Web Locks (they need a secure context, and a
// self-hosted instance may be plain http) tabs may overlap; the key makes the
// duplicate a harmless replay.
async function withSendingLock(run: () => Promise<void>): Promise<boolean> {
  if (!navigator.locks) {
    await run()
    return true
  }
  let held = false
  await navigator.locks.request(LOCK_NAME, { ifAvailable: true }, async (lock) => {
    if (lock) {
      held = true
      await run()
    }
  })
  return held
}

async function drain(): Promise<void> {
  const queryClient = client
  const user = sessionUser()
  if (!queryClient || !user) {
    return
  }
  if (queueOwner() !== user) {
    await loadPendingWrites(user)
  }
  // Stored rows are older than anything queued meanwhile, and the sender must not
  // get ahead of them. Their arrival runs this again.
  if (!isQueueLoaded(user)) {
    return
  }
  if (!onlineManager.isOnline()) {
    return
  }
  if (Date.now() < blockedUntil) {
    wakeIn(blockedUntil - Date.now())
    return
  }
  if (!nextSendable(getOps(), Date.now())) {
    wakeForNextRetry()
    return
  }
  if (!(await withSendingLock(() => sendAll(queryClient)))) {
    wakeIn(LOCK_RETRY_MS)
  }
}

function run(): Promise<void> {
  if (running) {
    rerun = true
    return running
  }
  running = (async () => {
    try {
      do {
        rerun = false
        await drain()
      } while (rerun)
    } catch (e) {
      console.warn('Sending waiting changes failed', (e as Error | undefined)?.name)
      wakeIn(BACKOFF_MAX_MS)
    } finally {
      // In the same step as the last look at rerun: a kick after it starts a new run.
      running = null
    }
  })()
  return running
}

/** An explicit reason to try now (a reconnect, a new or retried change): any backoff is over. */
export function kickPendingWrites(): Promise<void> {
  networkFailures = 0
  blockedUntil = 0
  return run()
}

const awaitsItsOutcome = (op: PendingOp) => op.attempted && op.status === 'waiting'

/**
 * The boot refresh goes after this: a change the previous run sent may be on the
 * server with its answer lost, and a read that brings its row and balance before
 * the sender has found that out shows the row twice and counts the money twice.
 * With nothing attempted there is nothing to wait for; never longer than one write.
 */
export async function attemptedWritesResolved(): Promise<void> {
  const user = sessionUser()
  if (!user || !onlineManager.isOnline()) {
    return
  }
  let timer: ReturnType<typeof setTimeout> | undefined
  let unsubscribeOps = () => {}
  await Promise.race([
    new Promise<void>((resolve) => { timer = setTimeout(resolve, WRITE_TIMEOUT_MS) }),
    (async () => {
      await loadPendingWrites(user)
      if (!getOps().some(awaitsItsOutcome)) {
        return
      }
      await new Promise<void>((resolve) => {
        unsubscribeOps = subscribe(() => {
          if (!getOps().some(awaitsItsOutcome)) {
            resolve()
          }
        })
        void kickPendingWrites().then(resolve)
      })
    })(),
  ])
  clearTimeout(timer)
  unsubscribeOps()
}

/**
 * Holding the refresh does not stop what a mount fetches on its own, and a restored
 * query past its staleTime would: its balances would count a change the overlay
 * still applies. So they read as fresh until the held refresh, which is bounded by
 * attemptedWritesResolved. Await this before the observers mount.
 */
export async function holdRestoredReads(queryClient: QueryClient): Promise<void> {
  const user = sessionUser()
  if (!user || !onlineManager.isOnline()) {
    return
  }
  await loadPendingWrites(user)
  if (!getOps().some(awaitsItsOutcome)) {
    return
  }
  const now = Date.now()
  for (const queryKey of [queryKeys.accounts, queryKeys.transactions]) {
    queryClient.getQueryCache().find({ queryKey, exact: true })?.setState({ dataUpdatedAt: now, isInvalidated: false })
  }
}

export function startPendingWrites(queryClient: QueryClient): () => void {
  if (client !== queryClient) {
    unsubscribe?.()
    client = queryClient
    const offOnline = onlineManager.subscribe((online) => {
      if (online) {
        void kickPendingWrites()
      }
    })
    // A sign-in on this page, or the id of a session from before it was kept.
    const offUser = queryClient.getQueryCache().subscribe((event) => {
      if (event.type === 'updated' && event.action.type === 'success' && event.query.queryKey[0] === queryKeys.user[0]) {
        void run()
      }
    })
    const offElsewhere = subscribeLeftElsewhere(() => refreshAfterOtherTab(queryClient))
    // The drain that asked for the load may have given up on it and found nothing to send.
    const offAdopted = subscribeAdopted(() => { void run() })
    unsubscribe = () => {
      offOnline()
      offUser()
      offElsewhere()
      offAdopted()
    }
  }
  void kickPendingWrites()
  return () => {
    unsubscribe?.()
    unsubscribe = null
    client = null
    if (wakeTimer !== null) {
      clearTimeout(wakeTimer)
      wakeTimer = null
    }
  }
}
