import { useMemo, useSyncExternalStore } from 'react'
import type { Id } from '@/api/types'
import { getOps, subscribe } from './store'
import type { PendingOp } from './types'

export function usePendingOps(): PendingOp[] {
  return useSyncExternalStore(subscribe, getOps)
}

// A target can hold a sent change and a later one: the later one is what the user sees.
function latest(ops: PendingOp[]): PendingOp | undefined {
  return ops.reduce<PendingOp | undefined>((found, op) => (!found || op.seq > found.seq ? op : found), undefined)
}

export function usePendingOpForTransaction(id: Id): PendingOp | undefined {
  return latest(usePendingOps().filter((op) => op.kind !== 'limit' && op.txId === id))
}

export function usePendingLimit(budgetId: Id, elementId: Id, period: string): PendingOp | undefined {
  return latest(usePendingOps().filter((op) => op.kind === 'limit' && op.budgetId === budgetId && op.elementId === elementId && op.period === period))
}

export function useFailedWrites(): PendingOp[] {
  const ops = usePendingOps()
  return useMemo(() => ops.filter((op) => op.status === 'failed'), [ops])
}

export function useWaitingCount(): number {
  return usePendingOps().filter((op) => op.status === 'waiting').length
}

/** Everything sign-out would throw away: changes still waiting and refused ones alike. */
export function useUnsentCount(): number {
  return usePendingOps().length
}
