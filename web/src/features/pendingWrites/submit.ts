import { onlineManager, type QueryClient } from '@tanstack/react-query'
import { v7 as uuidv7 } from 'uuid'
import type { WriteOptions } from '@/api/client'
import { METRICS, trackEvent } from '@/lib/metrics'
import { classifyWriteError } from './classify'
import { addChange, discard, retryFailed, targetOf } from './queue'
import { WRITE_TIMEOUT_MS, confirmChange, kickPendingWrites } from './sender'
import { getOps, mutate } from './store'
import type { OpChange, PendingOp } from './types'

const nextSeq = (ops: PendingOp[]) => Math.max(0, ...ops.map((op) => op.seq)) + 1

// An add is keyed by its form's id, the one thing a retap and a restored draft
// share, so the server refuses a repeat of an add it already holds.
const keyOf = (change: OpChange) => (change.kind === 'tx-create' ? change.txId : uuidv7())

// Settles once the disk has the change, or after the store's time limit: the
// form must not close on a write that closing the app would still lose.
async function enqueue(change: OpChange, key: string, attempted: boolean): Promise<{ queued: true }> {
  const persisted = mutate((ops) => addChange(ops, change, { key, seq: nextSeq(ops) }, attempted))
  trackEvent(METRICS.WRITE_QUEUED, { kind: change.kind })
  void kickPendingWrites()
  await persisted
  return { queued: true }
}

/**
 * Online with nothing unsent on the target, the request goes out and its
 * refusal reaches the caller. Otherwise the change waits in the queue, whose
 * order and folding a direct request must not overtake.
 */
export async function submitChange<T>(
  queryClient: QueryClient,
  change: OpChange,
  send: (opts: WriteOptions) => Promise<T>,
): Promise<{ queued: boolean; result?: T }> {
  const target = targetOf(change)
  if (getOps().some((op) => targetOf(op) === target)) {
    // The same form again (a draft restored over its own queued add) would
    // otherwise hold a second op under the first one's key.
    const behind: OpChange = change.kind === 'tx-create' ? { kind: 'tx-update', txId: change.txId, payload: change.payload } : change
    return enqueue(behind, uuidv7(), false)
  }
  const key = keyOf(change)
  if (!onlineManager.isOnline()) {
    return enqueue(change, key, false)
  }
  let result: T
  try {
    result = await send({ idempotencyKey: key, timeout: WRITE_TIMEOUT_MS })
  } catch (err) {
    const outcome = classifyWriteError(err)
    // 'done' is an earlier submit of this form that landed: the sender's own
    // attempt gets the same answer and reads the outcome back.
    if (outcome === 'wait' || outcome === 'done') {
      return enqueue(change, key, true)
    }
    throw err
  }
  confirmChange(queryClient, change, result)
  return { queued: false, result }
}

function kindOf(key: string): PendingOp['kind'] | undefined {
  return getOps().find((op) => op.key === key)?.kind
}

export function retryWrite(key: string): void {
  const kind = kindOf(key)
  void mutate((ops) => retryFailed(ops, key))
  trackEvent(METRICS.WRITE_RETRIED, { kind })
  void kickPendingWrites()
}

export function discardWrite(key: string): void {
  const kind = kindOf(key)
  void mutate((ops) => discard(ops, key))
  trackEvent(METRICS.WRITE_DISCARDED, { kind })
}
