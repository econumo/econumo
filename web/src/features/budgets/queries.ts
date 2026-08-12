import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as budgetApi from '@/api/budget'
import type { BudgetDto, BudgetMetaDto, BudgetPlanDto } from '@/api/dto/budget'
import type { Id } from '@/api/types'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { compareNames } from '@/lib/collate'
import { METRICS, trackEvent } from '@/lib/metrics'
import { applyMove } from '@/lib/ordering'
import type { ElementMoveItem } from './elementMove'
import { UserOptions } from '@/api/dto/user'
import { useUserData, userOption } from '@/features/user/queries'
import { useBudgetPeriodStore } from './budgetStore'
import { addMonths } from './planMath'

export function useBudgets() {
  const { i18n } = useTranslation()
  const { data: user } = useUserData()
  return useQuery({
    queryKey: queryKeys.budgets,
    queryFn: budgetApi.getBudgetList,
    staleTime: TEN_MINUTES,
    // the list is unsorted on the wire; the settings page shows name asc (Vue parity).
    // Invites I have not accepted are surfaced by the sharing-requests modal, not the list.
    select: (items) =>
      items
        .filter((b) => b.ownerUserId === user?.id || b.access.some((a) => a.user.id === user?.id && a.isAccepted === 1))
        .sort((a, b) => compareNames(a.name, b.name, i18n.language)),
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

export type BudgetRoleCheck = 'configure' | 'updateLimits' | 'deleteEnvelope'

export function myBudgetRole(meta: BudgetMetaDto | undefined | null, userId: Id | undefined): string | null {
  if (!meta || !userId) {
    return null
  }
  return meta.access.find((a) => a.user.id === userId && a.isAccepted === 1)?.role ?? null
}

export function canConfigureBudget(meta: BudgetMetaDto | undefined | null, userId: Id | undefined): boolean {
  const role = myBudgetRole(meta, userId)
  return role === 'owner' || role === 'admin'
}

// Mirrors the backend's canUpdate (update-budget): guest is read-only.
export function canEditBudget(meta: BudgetMetaDto | undefined | null, userId: Id | undefined): boolean {
  const role = myBudgetRole(meta, userId)
  return role === 'owner' || role === 'admin' || role === 'user'
}

export function canUpdateLimits(meta: BudgetMetaDto | undefined | null, userId: Id | undefined, selectedDate: string): boolean {
  const role = myBudgetRole(meta, userId)
  if (!(role === 'owner' || role === 'admin' || role === 'user')) {
    return false
  }
  // limits cannot be edited for months before the budget start
  return !!meta && meta.startedAt.slice(0, 7) <= selectedDate.slice(0, 7)
}

export function canDeleteEnvelope(meta: BudgetMetaDto | undefined | null, userId: Id | undefined): boolean {
  return canConfigureBudget(meta, userId)
}

// The user's default budget for the selected period. No default -> data: null
// (the Vue store rejects with 'Budget is not selected'; the page shows onboarding).
export function useBudget() {
  const { data: user } = useUserData()
  const selectedDate = useBudgetPeriodStore((s) => s.selectedDate)
  const resetFoldsFor = useBudgetPeriodStore((s) => s.resetFoldsFor)
  const budgetId = userOption(user, UserOptions.BUDGET)

  useEffect(() => {
    if (budgetId) {
      resetFoldsFor(budgetId)
    }
  }, [budgetId, resetFoldsFor])

  return useQuery<BudgetDto | null>({
    queryKey: [...queryKeys.budget, budgetId ?? 'none', selectedDate],
    queryFn: () => (budgetId ? budgetApi.getBudget(budgetId, selectedDate) : Promise.resolve(null)),
    enabled: user !== undefined,
    staleTime: TEN_MINUTES,
    // month switches keep showing the previous period instead of a blank page
    placeholderData: keepPreviousData,
  })
}

export function useSetLimit() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (form: { budgetId: Id; elementId: Id; period: string; amount: string | null }) =>
      budgetApi.setLimit(form),
    onMutate: async (form) => {
      // optimistic budgeted patch with rollback (Vue parity: instant cell feedback)
      const key = [...queryKeys.budget, form.budgetId, form.period]
      await queryClient.cancelQueries({ queryKey: key })
      const previous = queryClient.getQueryData<BudgetDto | null>(key)
      queryClient.setQueryData<BudgetDto | null>(key, (prev) => {
        if (!prev) {
          return prev
        }
        return {
          ...prev,
          structure: {
            ...prev.structure,
            elements: prev.structure.elements.map((el) =>
              el.id === form.elementId ? { ...el, budgeted: form.amount === null ? '0' : form.amount } : el,
            ),
          },
        }
      })
      return { previous, key }
    },
    onError: (_err, _form, context) => {
      if (context) {
        queryClient.setQueryData(context.key, context.previous)
      }
    },
    onSuccess: () => {
      trackEvent(METRICS.BUDGET_UPDATE_ELEMENT_LIMIT)
      // budget-mode edits patch only the budget-page cache above; the plan cache
      // (a different window/query key) must be invalidated too or the plan sheet
      // keeps showing the pre-edit limit until something else happens to refetch it
      void queryClient.invalidateQueries({ queryKey: queryKeys.budgetPlan })
    },
  })
}

export const PLAN_BUFFER = 2

export function planFetchWindow(firstMonth: string, visibleMonths: number): { from: string; months: number } {
  // buffer both sides so arrow navigation renders instantly; the server caps months at 24
  const months = Math.min(visibleMonths + 2 * PLAN_BUFFER, 24)
  return { from: addMonths(firstMonth, -PLAN_BUFFER), months }
}

export function useBudgetPlan(budgetId: Id | null, firstMonth: string, visibleMonths: number) {
  const { from, months } = planFetchWindow(firstMonth, visibleMonths)
  const planKey = [...queryKeys.budgetPlan, budgetId ?? 'none', from, months] as const
  const query = useQuery<BudgetPlanDto | null>({
    queryKey: planKey,
    queryFn: () => (budgetId ? budgetApi.getBudgetPlan(budgetId, from, months) : Promise.resolve(null)),
    enabled: budgetId !== null,
    staleTime: TEN_MINUTES,
    // shifting months keeps showing the previous window instead of a blank sheet
    placeholderData: keepPreviousData,
  })
  return { ...query, fetchFrom: from, planKey }
}

export function usePlanSetLimit(planKey: readonly unknown[]) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (form: { budgetId: Id; elementId: Id; period: string; amount: string | null; monthIndex: number }) =>
      budgetApi.setLimit({ budgetId: form.budgetId, elementId: form.elementId, period: form.period, amount: form.amount }),
    onMutate: async (form) => {
      await queryClient.cancelQueries({ queryKey: planKey })
      const previous = queryClient.getQueryData<BudgetPlanDto | null>(planKey)
      queryClient.setQueryData<BudgetPlanDto | null>(planKey, (prev) => {
        if (!prev) {
          return prev
        }
        return {
          ...prev,
          structure: {
            ...prev.structure,
            elements: prev.structure.elements.map((el) =>
              el.id === form.elementId
                ? {
                    ...el,
                    cells: el.cells.map((c, i) =>
                      i === form.monthIndex ? { ...c, planned: form.amount === null ? '' : form.amount } : c,
                    ),
                  }
                : el,
            ),
          },
        }
      })
      return { previous }
    },
    onError: (_err, _form, context) => {
      if (context) {
        queryClient.setQueryData(planKey, context.previous)
      }
    },
    onSuccess: (_res, form) => {
      trackEvent(METRICS.BUDGET_UPDATE_ELEMENT_LIMIT)
      // the budget-page cache for that month is now stale; the plan cache resyncs too
      void queryClient.invalidateQueries({ queryKey: [...queryKeys.budget, form.budgetId, form.period] })
      void queryClient.invalidateQueries({ queryKey: planKey })
    },
  })
}

function useInvalidateBudget() {
  const queryClient = useQueryClient()
  return () => {
    void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
    void queryClient.invalidateQueries({ queryKey: queryKeys.budgetPlan })
  }
}

export function useCreateEnvelope() {
  const invalidate = useInvalidateBudget()
  return useMutation({
    mutationFn: budgetApi.createEnvelope,
    onSuccess: () => {
      invalidate()
      trackEvent(METRICS.BUDGET_ENVELOPE_CREATE)
    },
  })
}

export function useUpdateEnvelope() {
  const invalidate = useInvalidateBudget()
  return useMutation({
    mutationFn: budgetApi.updateEnvelope,
    onSuccess: () => {
      invalidate()
      trackEvent(METRICS.BUDGET_ENVELOPE_UPDATE)
    },
  })
}

export function useDeleteEnvelope() {
  const invalidate = useInvalidateBudget()
  return useMutation({
    mutationFn: ({ budgetId, id }: { budgetId: Id; id: Id }) => budgetApi.deleteEnvelope(budgetId, id),
    onSuccess: () => {
      invalidate()
      trackEvent(METRICS.BUDGET_ENVELOPE_DELETE)
    },
  })
}

export function useCreateBudgetFolder() {
  const invalidate = useInvalidateBudget()
  return useMutation({
    mutationFn: budgetApi.createBudgetFolder,
    onSuccess: () => {
      invalidate()
      trackEvent(METRICS.BUDGET_FOLDER_CREATE)
    },
  })
}

export function useUpdateBudgetFolder() {
  const invalidate = useInvalidateBudget()
  return useMutation({
    mutationFn: budgetApi.updateBudgetFolder,
    onSuccess: () => {
      invalidate()
      trackEvent(METRICS.BUDGET_FOLDER_UPDATE)
    },
  })
}

export function useDeleteBudgetFolder() {
  const invalidate = useInvalidateBudget()
  return useMutation({
    mutationFn: ({ budgetId, id }: { budgetId: Id; id: Id }) => budgetApi.deleteBudgetFolder(budgetId, id),
    onSuccess: () => {
      invalidate()
      trackEvent(METRICS.BUDGET_FOLDER_DELETE)
    },
  })
}

export function useMoveBudgetFolder() {
  const queryClient = useQueryClient()
  const invalidate = useInvalidateBudget()
  const selectedDate = useBudgetPeriodStore((s) => s.selectedDate)
  return useMutation({
    mutationFn: ({ budgetId, id, afterId }: { budgetId: Id; id: Id; afterId: Id | null }) =>
      budgetApi.moveBudgetFolder(budgetId, id, afterId),
    // optimistic folder positions with rollback — a dropped folder must land
    // instantly, not after the server round-trip
    onMutate: async ({ budgetId, id, afterId }) => {
      const key = [...queryKeys.budget, budgetId, selectedDate]
      await queryClient.cancelQueries({ queryKey: key })
      const previous = queryClient.getQueryData<BudgetDto | null>(key)
      queryClient.setQueryData<BudgetDto | null>(key, (prev) => {
        if (!prev) {
          return prev
        }
        const ordered = [...prev.structure.folders].sort((a, b) => a.position - b.position)
        return {
          ...prev,
          structure: { ...prev.structure, folders: applyMove(ordered, id, afterId) },
        }
      })
      return { previous, key }
    },
    onError: (_err, _form, context) => {
      if (context) {
        queryClient.setQueryData(context.key, context.previous)
      }
    },
    onSuccess: () => {
      invalidate()
      trackEvent(METRICS.BUDGET_FOLDER_CHANGE_ORDER)
    },
  })
}

export function useMoveElement() {
  const invalidate = useInvalidateBudget()
  return useMutation({
    mutationFn: ({ budgetId, item }: { budgetId: Id; item: ElementMoveItem }) =>
      budgetApi.moveElement(budgetId, item.id, item.folderId, item.afterId),
    onSuccess: () => {
      invalidate()
      trackEvent(METRICS.BUDGET_CHANGE_ORDER_ELEMENT)
    },
  })
}

export function useChangeElementCurrency() {
  const invalidate = useInvalidateBudget()
  return useMutation({
    mutationFn: budgetApi.changeElementCurrency,
    onSuccess: () => {
      invalidate()
      trackEvent(METRICS.BUDGET_ELEMENT_CHANGE_CURRENCY)
    },
  })
}

export function useUpdateBudgetDetail() {
  const queryClient = useQueryClient()
  const invalidate = useInvalidateBudget()
  return useMutation({
    mutationFn: budgetApi.updateBudget,
    onSuccess: (meta) => {
      queryClient.setQueryData<BudgetMetaDto[]>(queryKeys.budgets, (prev) =>
        (prev ?? []).map((b) => (b.id === meta.id ? meta : b)),
      )
      invalidate()
      trackEvent(METRICS.BUDGET_UPDATE)
    },
  })
}

export function useBudgetTransactions(params: budgetApi.BudgetTransactionsParams | null) {
  return useQuery({
    queryKey: [...queryKeys.budgetTransactions, params],
    queryFn: () => budgetApi.getBudgetTransactions(params as budgetApi.BudgetTransactionsParams),
    enabled: params !== null,
  })
}

export function useGrantBudgetAccess() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (form: { budgetId: Id; userId: Id; role: string }) => budgetApi.grantAccess(form),
    onSuccess: (items) => {
      queryClient.setQueryData(queryKeys.budgets, items)
      trackEvent(METRICS.BUDGET_GRANT_ACCESS)
    },
  })
}

export function useRevokeBudgetAccess() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (form: { budgetId: Id; userId: Id }) => budgetApi.revokeAccess(form),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.budgets })
      trackEvent(METRICS.BUDGET_REVOKE_ACCESS)
    },
  })
}

export function useAcceptBudgetAccess() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (budgetId: Id) => budgetApi.acceptAccess(budgetId),
    onSuccess: (items) => {
      queryClient.setQueryData(queryKeys.budgets, items)
      // accepting can change the default budget option + budget visibility
      void queryClient.invalidateQueries({ queryKey: queryKeys.user })
      void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
      trackEvent(METRICS.BUDGET_ACCEPT_ACCESS)
    },
  })
}

export function useDeclineBudgetAccess() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (budgetId: Id) => budgetApi.declineAccess(budgetId),
    onSuccess: (_r, budgetId) => {
      // drop it synchronously so the invite disappears before the refetch lands
      queryClient.setQueryData<BudgetMetaDto[]>(queryKeys.budgets, (prev) => (prev ?? []).filter((b) => b.id !== budgetId))
      void queryClient.invalidateQueries({ queryKey: queryKeys.budgets })
      trackEvent(METRICS.BUDGET_DECLINE_ACCESS)
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
