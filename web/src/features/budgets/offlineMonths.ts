import { useEffect, useMemo, useState } from 'react'
import { hashKey, useQueries, useQueryClient } from '@tanstack/react-query'
import type { BudgetMetaDto } from '@/api/dto/budget'
import { UserOptions } from '@/api/dto/user'
import { useOnline } from '@/hooks/useOnline'
import { useUserData, userOption } from '@/features/user/queries'
import { addMonths } from './planMath'
import { budgetCurrentPeriod } from './period'
import { budgetPlanQueryOptions, budgetQueryOptions, keptPlanWindow, useBudgets } from './queries'
import { onWrite } from '@/lib/writeEvents'

/** the periods and the plan window of a budget the device keeps for offline use:
 *  the previous, current and next period, under the keys the Budget page asks for
 *  when it opens on them */
export function keptBudgetMonths(meta: BudgetMetaDto, now: Date): { periods: string[]; plan: { from: string; months: number } } {
  const cur = budgetCurrentPeriod(meta, now)
  // a month before the budget's first is browsable too (read-only), so it is kept
  const periods = [addMonths(cur, -1), cur, addMonths(cur, 1)]
  // around the kept periods, not today: a budget that has not started yet opens on its first period
  return { periods, plan: keptPlanWindow(cur) }
}

const localDay = (d: Date) => `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`

// The current period moves at a local midnight: the timer catches it in an open
// tab, the visibility check after a device slept through it.
function useLocalDay(): string {
  const [day, setDay] = useState(() => localDay(new Date()))
  useEffect(() => {
    const check = () => setDay(localDay(new Date()))
    const now = new Date()
    const midnight = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1, 0, 0, 1)
    const timer = setTimeout(check, midnight.getTime() - now.getTime())
    document.addEventListener('visibilitychange', check)
    return () => {
      clearTimeout(timer)
      document.removeEventListener('visibilitychange', check)
    }
  }, [day])
  return day
}

/** how long the kept entries wait after the last write before they refetch: a
 *  burst of writes (a queue of changes draining) refetches them once */
export const KEEP_REFETCH_DELAY_MS = 3000

// Keeps the default budget's previous, current and next period (Budget view and
// the plan window covering them) on the device, so they open offline though the
// device never showed them. The queries stay observed for as long as the app
// runs, so they are never garbage collected and every snapshot persists them.
// The observers are disabled: invalidating the budget queries (every transaction
// write) must not refetch them at once on every page. A Budget or Plan page on
// screen has its own enabled observer and refetches its month immediately as
// before (an invalidation cancels and restarts a fetch in flight there); the rest
// are fetched here, online only, at once when missing and `delayMs` after the
// last write when a write invalidated them. Errors show nowhere but the sync icon.
export function useKeepBudgetMonths(delayMs: number = KEEP_REFETCH_DELAY_MS): void {
  const queryClient = useQueryClient()
  const { data: user } = useUserData()
  const { data: budgets } = useBudgets()
  const online = useOnline()
  const day = useLocalDay()
  const budgetId = userOption(user, UserOptions.BUDGET)
  const meta = budgetId ? budgets?.find((b) => b.id === budgetId) : undefined
  const kept = useMemo(() => (meta && day ? keptBudgetMonths(meta, new Date()) : null), [meta, day])
  const options = useMemo(
    () =>
      kept && budgetId
        ? [
            ...kept.periods.map((p) => budgetQueryOptions(budgetId, p)),
            budgetPlanQueryOptions(budgetId, kept.plan.from, kept.plan.months),
          ]
        : [],
    [kept, budgetId],
  )
  useQueries({ queries: options.map((o) => ({ ...o, enabled: false })) })

  useEffect(() => {
    if (!online || options.length === 0) {
      return
    }
    const cache = queryClient.getQueryCache()
    const find = (o: (typeof options)[number]) => cache.find({ queryKey: o.queryKey, exact: true })
    const fetch = (o: (typeof options)[number]) => {
      queryClient.fetchQuery({ ...o, staleTime: 0 } as Parameters<typeof queryClient.fetchQuery>[0]).catch(() => {})
    }
    // Entries a write left behind. The 'invalidate' event alone would lose
    // writes: it fires only for a query not already invalidated, and a fetch
    // that started before the write clears the flag when it lands, with the
    // figures from before the write. So every write marks the entries it
    // invalidated, and a marked entry refetches after the last write, once no
    // fetch of it is in flight.
    const dirty = new Set<(typeof options)[number]>()
    let timer: ReturnType<typeof setTimeout> | undefined
    const flush = () => {
      timer = undefined
      for (const o of [...dirty]) {
        const query = find(o)
        if (query?.isActive()) {
          // a page on screen refetches it itself
          dirty.delete(o)
        } else if (query?.state.fetchStatus !== 'fetching') {
          dirty.delete(o)
          fetch(o)
        }
      }
      if (dirty.size > 0) {
        timer = setTimeout(flush, delayMs)
      }
    }
    const restart = () => {
      if (dirty.size > 0) {
        clearTimeout(timer)
        timer = setTimeout(flush, delayMs)
      }
    }
    const markInvalidated = () => {
      for (const o of options) {
        if (find(o)?.state.isInvalidated) {
          dirty.add(o)
        }
      }
      restart()
    }

    for (const o of options) {
      const query = find(o)
      if (!query || query.state.data === undefined) {
        if (query?.state.fetchStatus !== 'fetching') {
          fetch(o)
        }
      } else if (query.isStaleByTime(o.staleTime)) {
        dirty.add(o)
      }
    }
    restart()
    const hashes = new Set(options.map((o) => hashKey(o.queryKey)))
    const unsubscribeQueries = cache.subscribe((event) => {
      if (event.type === 'updated' && event.action.type === 'invalidate' && hashes.has(event.query.queryHash)) {
        markInvalidated()
      }
    })
    // a mutation's success runs its invalidations before this event
    const unsubscribeMutations = queryClient.getMutationCache().subscribe((event) => {
      if (event.type === 'updated' && event.action.type === 'success') {
        markInvalidated()
      }
    })
    const unsubscribeWrites = onWrite(markInvalidated)
    return () => {
      clearTimeout(timer)
      unsubscribeQueries()
      unsubscribeMutations()
      unsubscribeWrites()
    }
  }, [online, options, queryClient, delayMs])
}
