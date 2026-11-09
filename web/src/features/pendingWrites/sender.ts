import { notifyManager, onlineManager, type QueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { setLimit } from '@/api/budget'
import { createTransaction, deleteTransaction, updateTransaction } from '@/api/transaction'
import type { WriteOptions } from '@/api/client'
import type { TransactionItemDto } from '@/api/dto/transaction'
import type { Id } from '@/api/types'
import { queryKeys } from '@/app/queryKeys'
import { applyTransactionItem } from '@/features/transactions/queries'
import { apiErrorMessage } from '@/lib/apiError'
import { invalidateReports } from '@/lib/appQueryClient'
import { METRICS, trackEvent } from '@/lib/metrics'
import { getSessionUserId, hasToken } from '@/lib/storage'
import { classifyWriteError } from './classify'
import { markAttempted, nextSendable, resolveTempId, settleDone, settleRefused, settleUnresolved } from './queue'
import { getOps, loadPendingWrites, mutate, queueOwner } from './store'
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

function sessionUser(): string | null {
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

// The queue is not a mutation, so the MutationCache's report invalidation does not see it.
function acknowledge(queryClient: QueryClient, change: OpChange): void {
  invalidateReports(queryClient)
  trackEvent(CONFIRM_METRIC[change.kind])
}

/** The one place a write the server accepted reaches the caches and the analytics. */
export function confirmChange(queryClient: QueryClient, change: OpChange, result: unknown): void {
  if (change.kind === 'limit') {
    invalidateChange(queryClient, change)
  } else {
    applyTransactionItem(queryClient, result as TransactionItemDto, APPLY_MODE[change.kind])
  }
  acknowledge(queryClient, change)
}

function isQueued(op: PendingOp): boolean {
  return getOps().some((queued) => queued.key === op.key)
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
      return op.kind === 'tx-create' ? resolveTempId(remaining, op.txId, (result as TransactionItemDto).item.id) : remaining
    })
  })
}

// A replay proves an earlier attempt landed, but its response was lost, so all
// that is left is to read the outcome back from the server.
function settleReplayed(queryClient: QueryClient, op: PendingOp, err: unknown): Promise<void> {
  if (!isQueued(op)) {
    return Promise.resolve()
  }
  const written = mutate((ops) => {
    if (op.kind !== 'tx-create') {
      return settleDone(ops, op.key)
    }
    const realId = replayResourceId(err)
    return realId ? resolveTempId(settleDone(ops, op.key), op.txId, realId) : settleUnresolved(ops, op.key, apiErrorMessage(err))
  })
  if (op.kind !== 'limit') {
    void queryClient.invalidateQueries({ queryKey: queryKeys.transactions })
    void queryClient.invalidateQueries({ queryKey: queryKeys.accounts })
  }
  invalidateChange(queryClient, op)
  acknowledge(queryClient, op)
  return written
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
    blockedUntil = Date.now() + backoff(networkFailures)
    wakeIn(backoff(networkFailures))
    networkFailures++
    return false
  }
  networkFailures = 0
  if (outcome === 'pause') {
    return false
  }
  if (outcome === 'done') {
    await settleReplayed(queryClient, op, err)
  } else if (outcome === 'refused' || op.retries >= MAX_RETRIES) {
    await mutate((ops) => settleRefused(ops, op.key, apiErrorMessage(err)))
  } else {
    await mutate((ops) => retryLater(ops, op.key, op.retries + 1))
  }
  return true
}

async function sendNext(queryClient: QueryClient, op: PendingOp): Promise<boolean> {
  // Before the request leaves: a reload after this must not fold new values
  // under a key the server may already hold.
  await mutate((ops) => markAttempted(ops, op.key))
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
  const started = (async () => {
    do {
      rerun = false
      await drain()
    } while (rerun)
  })().catch((e: unknown) => {
    console.warn('Sending waiting changes failed', (e as Error | undefined)?.name)
    wakeIn(BACKOFF_MAX_MS)
  }).finally(() => {
    running = null
  })
  running = started
  return started
}

/** An explicit reason to try now (a reconnect, a new or retried change): any backoff is over. */
export function kickPendingWrites(): Promise<void> {
  networkFailures = 0
  blockedUntil = 0
  return run()
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
    unsubscribe = () => {
      offOnline()
      offUser()
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
