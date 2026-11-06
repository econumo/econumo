import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { Id } from '@/api/types'
import { METRICS, trackEvent } from '@/lib/metrics'
import { budgetCurrentPeriod, currentPeriod } from './period'

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

type FollowState = Pick<BudgetPeriodState, 'periodChosen' | 'selectedDate' | 'adoptedPeriod' | 'chosenPeriod'>

/** the month shown is a default, not a pick: the calendar month or one adopted earlier,
 *  unless the user picked exactly that month (also before a reload) */
export function followsCurrentPeriod(s: FollowState, now: Date = new Date()): boolean {
  if (s.periodChosen || s.selectedDate === s.chosenPeriod) {
    return false
  }
  return s.selectedDate === currentPeriod(1, now) || s.selectedDate === s.adoptedPeriod
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
  /** the user picked a month this session (not persisted) */
  periodChosen: boolean
  /** the month followCurrentPeriod last moved the default to; persisted, so a reload
   *  still knows the stored month was a default and not a pick */
  adoptedPeriod: string | null
  /** the month the user last picked; persisted, so a picked month that happens to be
   *  the calendar month is not taken for a default after a reload */
  chosenPeriod: string | null
  /** an unpicked default month moves to the opened budget's current period: a
   *  shifted budget can still be in last month's period. A budget that has not
   *  started yet (its first month, startedAt, is later) opens on its first month. */
  followCurrentPeriod: (budgetId: Id, startDay: number, startedAt?: string | null, now?: Date) => void
  /** the budget followCurrentPeriod last ran for this session (not persisted): until
   *  it names the open budget a default month may still move (another budget can
   *  start its months on another day), so the budget page holds its period fetches */
  periodFollowedFor: Id | null
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
        const selectedDate = normalizePeriod(date)
        set({ selectedDate, periodChosen: true, adoptedPeriod: null, chosenPeriod: selectedDate })
      },
      periodChosen: false,
      adoptedPeriod: null,
      chosenPeriod: null,
      followCurrentPeriod: (budgetId, startDay, startedAt, now = new Date()) => {
        if (get().periodFollowedFor !== budgetId) {
          set({ periodFollowedFor: budgetId })
        }
        if (!followsCurrentPeriod(get(), now)) {
          return
        }
        const target = startedAt
          ? budgetCurrentPeriod({ periodStartDay: startDay, startedAt }, now)
          : currentPeriod(startDay, now)
        // A default that is the calendar month of a day-1 budget needs no mark: marking
        // it would move a month a day-1 user kept across sessions, which main never
        // did. Any other default is marked so it still reads as one later: a shifted
        // period equal to the calendar month for now (a day-5 budget opened on 10 Nov
        // must move to December on 6 Dec), or a not-yet-started budget's first month.
        const adopted = startDay !== 1 || target !== currentPeriod(1, now) ? target : null
        const s = get()
        if (target !== s.selectedDate) {
          set({ selectedDate: target, adoptedPeriod: adopted, chosenPeriod: null })
        } else if (s.adoptedPeriod !== adopted) {
          set({ adoptedPeriod: adopted, chosenPeriod: null })
        }
      },
      periodFollowedFor: null,
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
        set((s) => {
          const selectedDate = addMonthsToPeriod(s.selectedDate, delta)
          return { selectedDate, periodChosen: true, adoptedPeriod: null, chosenPeriod: selectedDate }
        })
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
      partialize: ({ periodChosen: _chosen, periodFollowedFor: _followed, ...rest }): Partial<BudgetPeriodState> => rest,
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
