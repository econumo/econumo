import type { QueryClient } from '@tanstack/react-query'
import type { BudgetDto, BudgetPlanDto } from '@/api/dto/budget'
import type { Id } from '@/api/types'
import { queryKeys } from '@/app/queryKeys'
import { isZero } from '@/lib/decimal'

/** How many budgets the SPA already holds in its query cache plan savings for
 *  this account: a monthly budget with a non-zero `budgeted` savings row, or a
 *  plan window with any non-empty `planned` savings cell. Counts distinct
 *  budget ids. Budgets the user cannot see are not counted — the confirmation
 *  wording ("will be removed") does not promise a precise total. */
export function countBudgetsPlanningSavings(queryClient: QueryClient, accountId: Id): number {
  const budgetIds = new Set<Id>()
  for (const [, budget] of queryClient.getQueriesData<BudgetDto | null>({ queryKey: queryKeys.budget })) {
    const row = budget?.structure.savings?.find((s) => s.id === accountId)
    if (budget && row && row.budgeted !== '' && !isZero(row.budgeted)) {
      budgetIds.add(budget.meta.id)
    }
  }
  for (const [, plan] of queryClient.getQueriesData<BudgetPlanDto | null>({ queryKey: queryKeys.budgetPlan })) {
    const row = plan?.structure.savings?.find((s) => s.id === accountId)
    if (plan && row && row.cells.some((c) => c.planned !== '')) {
      budgetIds.add(plan.meta.id)
    }
  }
  return budgetIds.size
}
