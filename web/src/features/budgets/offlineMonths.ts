import { useEffect, useState } from 'react'
import { useQueries, type UseQueryOptions } from '@tanstack/react-query'
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
  const first = meta.startedAt.slice(0, 7) + '-01'
  // the server refuses a month before the budget's first
  const periods = [addMonths(cur, -1), cur, addMonths(cur, 1)].filter((p) => p >= first)
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

// Keeps the default budget's previous, current and next period (Budget view and
// the plan window covering them) on the device, so they open offline though the
// device never showed them. The queries stay observed for as long as the app
// runs, so they are never garbage collected and every snapshot persists them; an
// observer only fetches when its entry is stale (staleTime, or invalidated by a
// write) and only while online. Its errors show nowhere but the sync icon.
export function useKeepBudgetMonths(): void {
  const { data: user } = useUserData()
  const { data: budgets } = useBudgets()
  const online = useOnline()
  const day = useLocalDay()
  const budgetId = userOption(user, UserOptions.BUDGET)
  const meta = budgetId ? budgets?.find((b) => b.id === budgetId) : undefined
  const kept = meta && day ? keptBudgetMonths(meta, new Date()) : null
  const shared = { enabled: online, refetchOnWindowFocus: false } as const
  const queries: UseQueryOptions<unknown, Error, unknown, readonly unknown[]>[] =
    kept && budgetId
      ? [
          ...kept.periods.map((p) => ({ ...budgetQueryOptions(budgetId, p), ...shared })),
          { ...budgetPlanQueryOptions(budgetId, kept.plan.from, kept.plan.months), ...shared },
        ]
      : []
  useQueries({ queries })
}
