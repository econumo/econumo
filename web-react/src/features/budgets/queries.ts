import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as budgetApi from '@/api/budget'
import type { BudgetMetaDto } from '@/api/dto/budget'
import type { Id } from '@/api/types'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { METRICS, trackEvent } from '@/lib/metrics'

export function useBudgets() {
  return useQuery({
    queryKey: queryKeys.budgets,
    queryFn: budgetApi.getBudgetList,
    staleTime: TEN_MINUTES,
    // the list is unsorted on the wire; the settings page shows name asc (Vue parity)
    select: (items) => [...items].sort((a, b) => a.name.localeCompare(b.name)),
  })
}

export function useCreateBudget() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (form: budgetApi.CreateBudgetForm & { ownerUserId?: Id }) => {
      // Vue guard: a same-name own budget resolves without an API call
      const existing = queryClient
        .getQueryData<BudgetMetaDto[]>(queryKeys.budgets)
        ?.find((b) => b.name.toLowerCase() === form.name.toLowerCase() && (!form.ownerUserId || b.ownerUserId === form.ownerUserId))
      if (existing) {
        return existing
      }
      const { ownerUserId: _owner, ...payload } = form
      return budgetApi.createBudget(payload)
    },
    onSuccess: (meta) => {
      queryClient.setQueryData<BudgetMetaDto[]>(queryKeys.budgets, (prev) => {
        const items = prev ?? []
        return items.some((b) => b.id === meta.id) ? items : [...items, meta]
      })
      void queryClient.invalidateQueries({ queryKey: queryKeys.user })
      trackEvent(METRICS.BUDGET_CREATE)
    },
  })
}

export function useDeleteBudget() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: Id) => budgetApi.deleteBudget(id),
    onSuccess: (_r, id) => {
      queryClient.setQueryData<BudgetMetaDto[]>(queryKeys.budgets, (prev) => (prev ?? []).filter((b) => b.id !== id))
      void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
      void queryClient.invalidateQueries({ queryKey: queryKeys.user })
      trackEvent(METRICS.BUDGET_DELETE)
    },
  })
}
