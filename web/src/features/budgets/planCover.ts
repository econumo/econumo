import type { QueryClient } from '@tanstack/react-query'
import type { BudgetPlanDto, PlanCellDto } from '@/api/dto/budget'
import type { Id } from '@/api/types'
import { queryKeys } from '@/app/queryKeys'

/** a cached window cut back to end at `last`. The start stays: the opening
 *  balances are the balances before the first month, and every running figure
 *  (Balance, the savings split) adds up from there, so dropping leading months
 *  would need their effect folded into the openings. Readers look months up by
 *  `months.indexOf`, never by position, so extra leading months are harmless. */
export function coveredPlan(plan: BudgetPlanDto, last: string): BudgetPlanDto {
  const end = plan.months.indexOf(last) + 1
  if (end === 0 || end === plan.months.length) {
    return plan
  }
  const cut = <T,>(cells: T[]) => cells.slice(0, end)
  const within = <T extends { month: string }>(list: T[] | undefined) => list?.filter((f) => f.month <= last)
  const { savings } = plan.structure
  return {
    ...plan,
    months: cut(plan.months),
    currencyRates: cut(plan.currencyRates),
    transfers: cut(plan.transfers),
    structure: {
      ...plan.structure,
      elements: plan.structure.elements.map((el) => ({
        ...el,
        cells: cut<PlanCellDto>(el.cells),
        children: el.children.map((c) => ({ ...c, cells: cut(c.cells) })),
      })),
      ...(savings ? { savings: savings.map((s) => ({ ...s, cells: cut<PlanCellDto>(s.cells) })) } : {}),
    },
    ...(plan.savingsFlows ? { savingsFlows: within(plan.savingsFlows) } : {}),
    ...(plan.savingsIncomeExpense ? { savingsIncomeExpense: within(plan.savingsIncomeExpense) } : {}),
  }
}

/** the newest cached plan window of the budget holding every month from `first`
 *  to `last`, or undefined */
export function findCoveringPlan(client: QueryClient, budgetId: Id, first: string, last: string): BudgetPlanDto | undefined {
  let best: { plan: BudgetPlanDto; at: number } | undefined
  for (const query of client.getQueryCache().findAll({ queryKey: [...queryKeys.budgetPlan, budgetId] })) {
    const plan = query.state.data as BudgetPlanDto | null | undefined
    if (!plan || plan.months.length === 0) {
      continue
    }
    if (plan.months[0] <= first && plan.months.includes(last) && (!best || query.state.dataUpdatedAt > best.at)) {
      best = { plan, at: query.state.dataUpdatedAt }
    }
  }
  return best?.plan
}
