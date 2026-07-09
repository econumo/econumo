import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { Id } from '@/api/types'
import { METRICS, trackEvent } from '@/lib/metrics'

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

interface BudgetPeriodState {
  selectedDate: string
  setPeriod: (date: string) => void
  /** element rows default folded; presence = unfolded (Vue semantics) */
  unfoldedElements: Record<Id, true>
  toggleElement: (id: Id) => void
  /** fold state belongs to one budget; reset when the budget id changes */
  foldBudgetId: Id | null
  resetFoldsFor: (budgetId: Id) => void
}

export const useBudgetPeriodStore = create<BudgetPeriodState>()(
  persist(
    (set, get) => ({
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
    }),
    { name: 'budgetPeriod' },
  ),
)
