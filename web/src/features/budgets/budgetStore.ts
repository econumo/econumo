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
  /** element rows default folded; presence = unfolded (Vue semantics). The Budget
   *  view (and the phone) and the Plan grid each keep their own fold state, so
   *  folding a line in one view leaves the other as it was. */
  unfoldedElements: Record<Id, true>
  toggleElement: (id: Id) => void
  planUnfoldedElements: Record<Id, true>
  togglePlanElement: (id: Id) => void
  /** element fold state belongs to one budget; reset when the budget id changes */
  foldBudgetId: Id | null
  resetFoldsFor: (budgetId: Id) => void
  /** the Plan grid's cursor walked off an edge: the window (and the strip) move */
  stepPeriod: (delta: number) => void
  /** the Plan grid's name column width in px, as the user dragged it; null = default */
  planNameWidth: number | null
  setPlanNameWidth: (px: number | null) => void
  /** Plan grid section/folder lines (by fold key) whose sums the user turned on with Σ */
  planSumsShown: Record<string, true>
  togglePlanSums: (key: string) => void
  /** folded sections and folders, by fold key ('income', folder ids, 'archived'):
   *  the Budget view's and the Plan grid's */
  budgetFolds: Record<string, true>
  toggleBudgetFold: (key: string) => void
  planFolds: Record<string, true>
  togglePlanFold: (key: string) => void
}

function toggleKey<K extends string>(map: Record<K, true>, key: K): Record<K, true> {
  const next = { ...map }
  if (next[key]) {
    delete next[key]
  } else {
    next[key] = true
  }
  return next
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
      toggleElement: (id) => set((state) => ({ unfoldedElements: toggleKey(state.unfoldedElements, id) })),
      planUnfoldedElements: {},
      togglePlanElement: (id) => set((state) => ({ planUnfoldedElements: toggleKey(state.planUnfoldedElements, id) })),
      foldBudgetId: null,
      resetFoldsFor: (budgetId) => {
        if (get().foldBudgetId !== budgetId) {
          set({ foldBudgetId: budgetId, unfoldedElements: {}, planUnfoldedElements: {} })
        }
      },
      stepPeriod: (delta) => {
        trackEvent(METRICS.BUDGET_PLAN_CHANGE_WINDOW)
        set((s) => ({ selectedDate: addMonthsToPeriod(s.selectedDate, delta) }))
      },
      planNameWidth: null,
      setPlanNameWidth: (px) => set({ planNameWidth: px }),
      planSumsShown: {},
      togglePlanSums: (key) => {
        trackEvent(METRICS.BUDGET_PLAN_TOGGLE_SUMS)
        set((state) => ({ planSumsShown: toggleKey(state.planSumsShown, key) }))
      },
      budgetFolds: {},
      toggleBudgetFold: (key) => set((state) => ({ budgetFolds: toggleKey(state.budgetFolds, key) })),
      planFolds: {},
      togglePlanFold: (key) => set((state) => ({ planFolds: toggleKey(state.planFolds, key) })),
    }),
    {
      name: 'budgetPeriod',
      version: 1,
      // version 0 shared one fold state between both views: each view starts from it
      migrate: (persisted, version) => {
        const state = (persisted ?? {}) as Partial<BudgetPeriodState>
        if (version < 1) {
          return { ...state, planUnfoldedElements: { ...state.unfoldedElements }, budgetFolds: { ...state.planFolds } }
        }
        return state
      },
    },
  ),
)
