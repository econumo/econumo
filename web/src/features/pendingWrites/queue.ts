import type { Id } from '@/api/types'
import type { OpChange, PendingOp } from './types'

export function targetOf(change: OpChange): string {
  if (change.kind === 'limit') return `limit:${change.budgetId}:${change.elementId}:${change.period}`
  return `tx:${change.txId}`
}

function bySeq(ops: PendingOp[]): PendingOp[] {
  return [...ops].sort((a, b) => a.seq - b.seq)
}

// null means the two cancel out: an add the server never kept, then deleted.
function fold(earlier: OpChange, later: OpChange): OpChange | null {
  if (earlier.kind !== 'tx-create' || later.kind === 'limit') return later
  if (later.kind === 'tx-delete') return null
  return { ...later, kind: 'tx-create' }
}

export function addChange(
  ops: PendingOp[],
  change: OpChange,
  mint: { key: string; seq: number },
  attempted = false,
): PendingOp[] {
  const target = targetOf(change)
  const last = bySeq(ops).findLast((op) => targetOf(op) === target)
  const fresh = { attempted, status: 'waiting', retries: 0, notBefore: 0 } as const

  // An attempted key may already be stored by the server: a different payload
  // under it would be refused as a replay and the edit lost, so it queues behind.
  if (attempted || !last || (last.attempted && last.status !== 'failed')) {
    return [...ops, { ...change, ...mint, ...fresh }]
  }

  const folded = fold(last, change)
  if (!folded) return ops.filter((op) => op !== last)
  return ops.map((op) => (op === last ? { ...folded, key: op.key, seq: op.seq, ...fresh } : op))
}

export function markAttempted(ops: PendingOp[], key: string): PendingOp[] {
  return ops.map((op) => (op.key === key ? { ...op, attempted: true } : op))
}

export function settleDone(ops: PendingOp[], key: string): PendingOp[] {
  return ops.filter((op) => op.key !== key)
}

export function settleRefused(ops: PendingOp[], key: string, message: string): PendingOp[] {
  const refused = ops.find((op) => op.key === key)
  if (!refused) return ops

  // Nothing queues behind a failure: the change behind folds in, so the Inbox
  // shows the latest values under one operation.
  const target = targetOf(refused)
  const behind = bySeq(ops).find((op) => op.seq > refused.seq && targetOf(op) === target)
  const change = behind ? fold(refused, behind) : refused
  const remaining = ops.filter((op) => op !== behind)
  if (!change) return remaining.filter((op) => op !== refused)

  return remaining.map((op) =>
    op === refused
      ? {
          ...change,
          key: op.key,
          seq: op.seq,
          attempted: false,
          status: 'failed',
          error: message,
          retries: 0,
          notBefore: 0,
        }
      : op,
  )
}

export function retryFailed(ops: PendingOp[], key: string): PendingOp[] {
  return ops.map((op) => {
    if (op.key !== key || op.status !== 'failed') return op
    const { error: _error, ...rest } = op
    return { ...rest, status: 'waiting', retries: 0, notBefore: 0 }
  })
}

export function discard(ops: PendingOp[], key: string): PendingOp[] {
  return ops.filter((op) => op.key !== key)
}

export function resolveTempId(ops: PendingOp[], tempId: Id, realId: Id): PendingOp[] {
  return ops.map((op) => {
    if (op.kind === 'limit' || op.txId !== tempId) return op
    if (op.kind === 'tx-delete') return { ...op, txId: realId }
    return { ...op, txId: realId, payload: { ...op.payload, id: realId } }
  })
}

export function nextSendable(ops: PendingOp[], now: number): PendingOp | null {
  const blocked = new Set<string>()
  for (const op of bySeq(ops)) {
    const target = targetOf(op)
    if (!blocked.has(target) && op.status === 'waiting' && op.notBefore <= now) return op
    blocked.add(target)
  }
  return null
}
