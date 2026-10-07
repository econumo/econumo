import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { Id } from '@/api/types'
import { METRICS, trackEvent } from '@/lib/metrics'

/** the reporting-tags folder exists only in rendering: no folder row stands behind
 *  it, so its fold state is keyed by a reserved literal no element id (a UUID) can
 *  collide with; the table and the phone view share it */
export type BudgetMode = 'budget' | 'plan'

export const REPORTING_TAGS_FOLD_ID = '__reporting_tags__'

function firstOfCurrentMonth(): string {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-01`
}

export function normalizePeriod(date: string): string {
  // accept Y-m-d or Y-m-d H:i:s (legacy) and snap to the first of the month
  const match = /^(\d{4})-(\d{2})/.exec(date)
  if (!match) {
    return firstOfCurrentMonth()
  }
  return `${match[1]}-${match[2]}-01`
}

function addMonthsToPeriod(period: string, delta: number): string {
  const [y, m] = period.split('-').map(Number)
  const d = new Date(y, m - 1 + delta, 1)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-01`
}

interface BudgetPeriodState {
  /** the view last opened, so the main menu's link returns to it */
  lastMode: BudgetMode
  setLastMode: (mode: BudgetMode) => void
  selectedDate: string
  setPeriod: (date: string) => void
  /** element rows default folded; presence = unfolded (Vue semantics) */
  unfoldedElements: Record<Id, true>
  toggleElement: (id: Id) => void
  /** fold state belongs to one budget; reset when the budget id changes */
  foldBudgetId: Id | null
  resetFoldsFor: (budgetId: Id) => void
  /** the Plan grid's cursor walked off an edge: the window (and the strip) move */
  stepPeriod: (delta: number) => void
  /** folded plan sections: 'income', folder ids, 'archived' */
  planFolds: Record<string, true>
  togglePlanFold: (key: string) => void
  /** the Plan grid's hide-empty-rows filter: no control turns it on until the Plan
   *  view is reworked, so it is not persisted (a device that had it on is not stuck) */
  planHideEmpty: boolean
}

export const useBudgetPeriodStore = create<BudgetPeriodState>()(
  persist(
    (set, get) => ({
      lastMode: 'budget',
      setLastMode: (mode) => set({ lastMode: mode }),
      selectedDate: firstOfCurrentMonth(),
      setPeriod: (date) => {
        trackEvent(METRICS.BUDGET_CHANGE_DATE)
        set({ selectedDate: normalizePeriod(date) })
      },
      unfoldedElements: {},
      toggleElement: (id) =>
        set((state) => {
          const next = { ...state.unfoldedElements }
          if (next[id]) {
            delete next[id]
          } else {
            next[id] = true
          }
          return { unfoldedElements: next }
        }),
      foldBudgetId: null,
      resetFoldsFor: (budgetId) => {
        if (get().foldBudgetId !== budgetId) {
          set({ foldBudgetId: budgetId, unfoldedElements: {} })
        }
      },
      stepPeriod: (delta) => {
        trackEvent(METRICS.BUDGET_PLAN_CHANGE_WINDOW)
        set((s) => ({ selectedDate: addMonthsToPeriod(s.selectedDate, delta) }))
      },
      planFolds: {},
      togglePlanFold: (key) =>
        set((state) => {
          const next = { ...state.planFolds }
          if (next[key]) {
            delete next[key]
          } else {
            next[key] = true
          }
          return { planFolds: next }
        }),
      planHideEmpty: false,
    }),
    { name: 'budgetPeriod', partialize: ({ planHideEmpty: _hidden, ...rest }) => rest },
  ),
)
