import { onlineManager, type QueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { v7 as uuidv7 } from 'uuid'
import type { WriteOptions } from '@/api/client'
import { METRICS, trackEvent } from '@/lib/metrics'
import { classifyWriteError } from './classify'
import { addAsEdit, addChange, discard, nextSeq, retryFailed, targetOf } from './queue'
import { WRITE_TIMEOUT_MS, confirmChange, kickPendingWrites, sessionUser } from './sender'
import { getOps, isQueueLoaded, loadPendingWrites, mutate } from './store'
import type { OpChange, PendingOp } from './types'

type AddChange = Extract<OpChange, { kind: 'tx-create' }>

// An add is keyed by its form's id, the one thing a retap and a restored draft
// share, so the server refuses a repeat of an add it already holds.
const keyOf = (change: OpChange) => (change.kind === 'tx-create' ? change.txId : uuidv7())

// Settles once the disk has the change, or after the store's time limit: the
// form must not close on a write that closing the app would still lose.
async function enqueue(kind: OpChange['kind'], apply: (ops: PendingOp[]) => PendingOp[]): Promise<{ queued: true }> {
  const persisted = mutate(apply)
  trackEvent(METRICS.WRITE_QUEUED, { kind })
  void kickPendingWrites()
  await persisted
  return { queued: true }
}

function enqueueChange(change: OpChange, key: string, attempted: boolean): Promise<{ queued: true }> {
  return enqueue(change.kind, (ops) => addChange(ops, change, { key, seq: nextSeq(ops) }, attempted))
}

// The server already holds this form's add and keeps the values of the attempt
// that committed, so the ones just submitted can only reach the row as an edit.
// An edit made while the request hung is newer than they are and stays last.
function enqueueReplayedAdd(add: AddChange, key: string): Promise<{ queued: true }> {
  return enqueue(add.kind, (ops) => {
    const editedMeanwhile = ops.some((op) => targetOf(op) === targetOf(add))
    const queued = addChange(ops, add, { key, seq: nextSeq(ops) }, true)
    return editedMeanwhile ? queued : addChange(queued, addAsEdit(add), { key: uuidv7(), seq: nextSeq(queued) })
  })
}

/**
 * Online with nothing unsent on the target, the request goes out and its
 * refusal reaches the caller. Otherwise the change waits in the queue, whose
 * order and folding a direct request must not overtake; that includes a queue
 * still on its way from the disk, which is why the load is waited for.
 */
export async function submitChange<T>(
  queryClient: QueryClient,
  change: OpChange,
  send: (opts: WriteOptions) => Promise<T>,
): Promise<{ queued: boolean; result?: T }> {
  const user = sessionUser()
  if (user && !isQueueLoaded(user)) {
    await loadPendingWrites(user)
    if (!isQueueLoaded(user)) {
      // A form's own key might name a stored add, whose row this would overwrite.
      return enqueueChange(change, uuidv7(), false)
    }
  }
  const target = targetOf(change)
  if (getOps().some((op) => targetOf(op) === target)) {
    // The same form again (a draft restored over its own queued add) would
    // otherwise hold a second op under the first one's key.
    return enqueueChange(change.kind === 'tx-create' ? addAsEdit(change) : change, uuidv7(), false)
  }
  const key = keyOf(change)
  if (!onlineManager.isOnline()) {
    return enqueueChange(change, key, false)
  }
  let result: T
  try {
    result = await send({ idempotencyKey: key, timeout: WRITE_TIMEOUT_MS })
  } catch (err) {
    const outcome = classifyWriteError(err)
    // A 5xx may have come after the commit, and then the server holds this key:
    // the retry must carry it too, or a delete answers "not found". Anything that
    // is not a request failure never left this client and rejects.
    if (outcome !== 'wait' && outcome !== 'done' && !(outcome === 'retry' && isAxiosError(err))) {
      throw err
    }
    // Another account signed in meanwhile: the queue is theirs now and this change
    // is not, and the form that asked for it is gone.
    if (sessionUser() !== user) {
      return { queued: true }
    }
    // 'done' is an earlier submit of this form that landed: the sender's own
    // attempt gets the same answer and reads the outcome back.
    return outcome === 'done' && change.kind === 'tx-create' ? enqueueReplayedAdd(change, key) : enqueueChange(change, key, true)
  }
  if (sessionUser() === user) {
    confirmChange(queryClient, change, result)
  }
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
