import { useEffect, useMemo, useState, useSyncExternalStore } from 'react'
import { useTranslation } from 'react-i18next'
import type { Id } from '@/api/types'
import { pluralPick } from '@/lib/plural'
import { getSessionUserId, hasToken } from '@/lib/storage'
import { getOps, isQueueLoaded, loadPendingWrites, subscribe, subscribeAdopted } from './store'
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

const subscribeQueueState = (fn: () => void) => {
  const offOps = subscribe(fn)
  const offAdopted = subscribeAdopted(fn)
  return () => {
    offOps()
    offAdopted()
  }
}

// A session with no user id has never had a queue on the disk: the sender and the
// store need that id. Otherwise a count of 0 means nothing until the rows arrived.
function useStoredQueueState(): 'loaded' | 'loading' | 'unknown' {
  const user = hasToken() ? getSessionUserId() : null
  const loaded = useSyncExternalStore(subscribeQueueState, () => !user || isQueueLoaded(user))
  const [gaveUpOn, setGaveUpOn] = useState<string | null>(null)
  useEffect(() => {
    if (!user || loaded) {
      return
    }
    let current = true
    // bounded by the store's own time limit; rows arriving later still flip `loaded`
    void loadPendingWrites(user).then(() => {
      if (current) {
        setGaveUpOn(user)
      }
    })
    return () => {
      current = false
    }
  }, [user, loaded])
  if (loaded) {
    return 'loaded'
  }
  return gaveUpOn === user ? 'unknown' : 'loading'
}

/** The stored rows are in memory, or the store's time limit passed without them. */
export function useStoredQueueSettled(): boolean {
  return useStoredQueueState() !== 'loading'
}

export interface SignOutConfirm {
  /** The confirm's question, followed by what signing out would lose. */
  question: string
  /** The stored queue is still on its way, so the question is not final. */
  confirmDisabled: boolean
  /** A link straight to sign-out must stop and ask. */
  mustAsk: boolean
}

export function useSignOutConfirm(question: string): SignOutConfirm {
  const { t, i18n } = useTranslation()
  const unsent = useUnsentCount()
  const queue = useStoredQueueState()
  const warning = unsent > 0
    ? pluralPick(t('settings.logout.pending_warning'), unsent, i18n.language)
    : queue === 'unknown' ? t('settings.logout.pending_unknown') : ''
  return {
    question: warning ? `${question} ${warning}` : question,
    confirmDisabled: queue === 'loading',
    mustAsk: unsent > 0 || queue !== 'loaded',
  }
}
