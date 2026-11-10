import { useEffect, useMemo, useState } from 'react'
import { hashKey, useQueries, useQueryClient } from '@tanstack/react-query'
import type { BudgetMetaDto } from '@/api/dto/budget'
import { UserOptions } from '@/api/dto/user'
import { useOnline } from '@/hooks/useOnline'
import { useUserData, userOption } from '@/features/user/queries'
import { addMonths } from './planMath'
import { budgetCurrentPeriod, currentPeriod, periodStartDayOf } from './period'
import { budgetPlanQueryOptions, budgetQueryOptions, monthPlanSpan, planFetchWindow, useBudgets } from './queries'

/** the periods and the plan window of a budget the device keeps for offline use:
 *  the previous, current and next period, under the keys the Budget page asks for
 *  when it opens on them */
export function keptBudgetMonths(meta: BudgetMetaDto, now: Date): { periods: string[]; plan: { from: string; months: number } } {
  const startDay = periodStartDayOf(meta)
  const cur = budgetCurrentPeriod(meta, now)
  // a month before the budget's first is browsable too (read-only), so it is kept
  const periods = [addMonths(cur, -1), cur, addMonths(cur, 1)]
  const span = monthPlanSpan(cur, currentPeriod(startDay, now))
  return { periods, plan: planFetchWindow(span.first, span.visible, startDay, now) }
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

/** how long the kept entries wait after an invalidation before they refetch: a
 *  burst of writes (a queue of changes draining) refetches them once */
export const KEEP_REFETCH_DELAY_MS = 3000

// Keeps the default budget's previous, current and next period (Budget view and
// the plan window covering them) on the device, so they open offline though the
// device never showed them. The queries stay observed for as long as the app
// runs, so they are never garbage collected and every snapshot persists them.
// The observers are disabled: invalidating the budget queries (every transaction
// write) must not refetch them at once on every page. A Budget or Plan page on
// screen has its own enabled observer and refetches its month immediately as
// before; the rest are fetched here, online only, at once when missing and
// after `delayMs` without another invalidation when stale. Errors show nowhere
// but the sync icon.
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
    const hashes = new Set(options.map((o) => hashKey(o.queryKey)))
    let timer: ReturnType<typeof setTimeout> | undefined
    const due = (missingOnly: boolean) =>
      options.filter((o) => {
        const query = cache.find({ queryKey: o.queryKey, exact: true })
        if (!query || query.state.data === undefined) {
          return query?.state.fetchStatus !== 'fetching'
        }
        return !missingOnly && query.state.fetchStatus !== 'fetching' && query.isStaleByTime(o.staleTime)
      })
    const fetch = (list: typeof options) => {
      for (const o of list) {
        queryClient.fetchQuery(o as Parameters<typeof queryClient.fetchQuery>[0]).catch(() => {})
      }
    }
    const later = () => {
      clearTimeout(timer)
      timer = setTimeout(() => fetch(due(false)), delayMs)
    }
    fetch(due(true))
    if (due(false).length > 0) {
      later()
    }
    const unsubscribe = cache.subscribe((event) => {
      if (event.type === 'updated' && event.action.type === 'invalidate' && hashes.has(event.query.queryHash)) {
        later()
      }
    })
    return () => {
      clearTimeout(timer)
      unsubscribe()
    }
  }, [online, options, queryClient, delayMs])
}
