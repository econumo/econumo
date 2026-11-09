import { useMemo, useSyncExternalStore } from 'react'
import { useTranslation } from 'react-i18next'
import type { Id } from '@/api/types'
import { pluralPick } from '@/lib/plural'
import { getOps, subscribe } from './store'
import { realIdOf } from './tempIds'
import type { PendingOp } from './types'

export function usePendingOps(): PendingOp[] {
  return useSyncExternalStore(subscribe, getOps)
}

// A target can hold a sent change and a later one: the later one is what the user sees.
function latest(ops: PendingOp[]): PendingOp | undefined {
  return ops.reduce<PendingOp | undefined>((found, op) => (!found || op.seq > found.seq ? op : found), undefined)
}

export function usePendingOpForTransaction(id: Id): PendingOp | undefined {
  const current = realIdOf(id)
  return latest(usePendingOps().filter((op) => op.kind !== 'limit' && op.txId === current))
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

/** A sign-out confirm's `question`, followed by what signing out would lose when there is anything. */
export function useSignOutQuestion(question: string): string {
  const { t, i18n } = useTranslation()
  const unsent = useUnsentCount()
  return unsent > 0 ? `${question} ${pluralPick(t('settings.logout.pending_warning'), unsent, i18n.language)}` : question
}
