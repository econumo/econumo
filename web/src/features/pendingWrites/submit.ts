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

function enqueue(change: OpChange, key: string, attempted: boolean): { queued: true } {
  void mutate((ops) => addChange(ops, change, { key, seq: nextSeq(ops) }, attempted))
  trackEvent(METRICS.WRITE_QUEUED, { kind: change.kind })
  void kickPendingWrites()
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
  if (!onlineManager.isOnline() || getOps().some((op) => targetOf(op) === target)) {
    return enqueue(change, uuidv7(), false)
  }
  const key = uuidv7()
  let result: T
  try {
    result = await send({ idempotencyKey: key, timeout: WRITE_TIMEOUT_MS })
  } catch (err) {
    if (classifyWriteError(err) === 'wait') {
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
