import { useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { v7 as uuidv7 } from 'uuid'
import { toast } from 'sonner'
import * as budgetApi from '@/api/budget'
import type { BudgetCommentDto, BudgetDto, BudgetMetaDto, BudgetPlanDto, PlanCellDto } from '@/api/dto/budget'
import type { CurrentUserDto } from '@/api/dto/user'
import type { Id } from '@/api/types'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { apiErrorMessage } from '@/lib/apiError'
import { compareNames } from '@/lib/collate'
import { sub } from '@/lib/decimal'
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
  const invalidate = useInvalidateBudget()
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
      const meta = await budgetApi.createBudget(payload)
      // here, not in onSuccess: the dedupe above creates nothing
      if ((form.savingsAccountIds?.length ?? 0) > 0) {
        trackEvent(METRICS.BUDGET_SAVINGS_TOGGLE)
      }
      return meta
    },
    onSuccess: (meta) => {
      queryClient.setQueryData<BudgetMetaDto[]>(queryKeys.budgets, (prev) => {
        const items = prev ?? []
        return items.some((b) => b.id === meta.id) ? items : [...items, meta]
      })
      void queryClient.invalidateQueries({ queryKey: queryKeys.user })
      invalidate()
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
        const budgeted = form.amount === null ? '0' : form.amount
        const savings = prev.structure.savings
        return {
          ...prev,
          structure: {
            ...prev.structure,
            elements: prev.structure.elements.map((el) => (el.id === form.elementId ? { ...el, budgeted } : el)),
            // a savings row carries no carry-over: its available is planned minus saved
            ...(savings
              ? { savings: savings.map((row) => (row.id === form.elementId ? { ...row, budgeted, available: sub(budgeted, row.spent) } : row)) }
              : {}),
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

// A plan row's id is either an element's or, for a savings row, the savings
// account's; the two sets never collide, so one patch covers both arrays.
function patchPlanCells(
  plan: BudgetPlanDto | null | undefined,
  elementId: Id,
  patch: (cell: PlanCellDto, monthIndex: number) => PlanCellDto,
): BudgetPlanDto | null | undefined {
  if (!plan) {
    return plan
  }
  const patchRow = <T extends { id: Id; cells: PlanCellDto[] }>(row: T): T =>
    row.id === elementId ? { ...row, cells: row.cells.map(patch) } : row
  const { savings } = plan.structure
  return {
    ...plan,
    structure: {
      ...plan.structure,
      elements: plan.structure.elements.map(patchRow),
      ...(savings ? { savings: savings.map(patchRow) } : {}),
    },
  }
}

export function usePlanSetLimit(planKey: readonly unknown[]) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (form: { budgetId: Id; elementId: Id; period: string; amount: string | null; monthIndex: number }) =>
      budgetApi.setLimit({ budgetId: form.budgetId, elementId: form.elementId, period: form.period, amount: form.amount }),
    onMutate: async (form) => {
      await queryClient.cancelQueries({ queryKey: planKey })
      const previous = queryClient.getQueryData<BudgetPlanDto | null>(planKey)
      const planned = form.amount === null ? '' : form.amount
      queryClient.setQueryData<BudgetPlanDto | null>(planKey, (prev) =>
        patchPlanCells(prev, form.elementId, (c, i) => (i === form.monthIndex ? { ...c, planned } : c)),
      )
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

export function useFillPlannedCells(planKey: readonly unknown[]) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (form: { budgetId: Id; elementId: Id; amount: string; targets: { period: string; monthIndex: number }[] }) =>
      Promise.all(
        form.targets.map((t) =>
          budgetApi.setLimit({ budgetId: form.budgetId, elementId: form.elementId, period: t.period, amount: form.amount }),
        ),
      ),
    onMutate: async (form) => {
      await queryClient.cancelQueries({ queryKey: planKey })
      const covered = new Set(form.targets.map((t) => t.monthIndex))
      queryClient.setQueryData<BudgetPlanDto | null>(planKey, (prev) =>
        patchPlanCells(prev, form.elementId, (c, i) => (covered.has(i) ? { ...c, planned: form.amount } : c)),
      )
    },
    onSuccess: () => {
      trackEvent(METRICS.BUDGET_PLAN_FILL_RIGHT)
    },
    // No partial rollback: any failure means some months may have landed, so both a
    // success and a failure need the same resync — the budget-page caches for every
    // target month plus the plan cache — from the server rather than trusting the
    // optimistic patch. Invalidating here (not split across onSuccess/onError) also
    // means it happens exactly once regardless of outcome.
    onSettled: (_res, _err, form) => {
      for (const t of form.targets) {
        void queryClient.invalidateQueries({ queryKey: [...queryKeys.budget, form.budgetId, t.period] })
      }
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

function sameIdSet(a: Id[], b: Id[]): boolean {
  const set = new Set(a)
  return set.size === new Set(b).size && b.every((id) => set.has(id))
}

export function useUpdateBudgetDetail() {
  const queryClient = useQueryClient()
  const invalidate = useInvalidateBudget()
  return useMutation({
    // previousSavingsAccountIds is client-only: the set the dialog opened with,
    // so the metric fires only when the request actually changes a flag
    mutationFn: ({ previousSavingsAccountIds: _prev, ...form }: budgetApi.UpdateBudgetForm & { previousSavingsAccountIds?: Id[] }) =>
      budgetApi.updateBudget(form),
    onSuccess: (meta, variables) => {
      queryClient.setQueryData<BudgetMetaDto[]>(queryKeys.budgets, (prev) =>
        (prev ?? []).map((b) => (b.id === meta.id ? meta : b)),
      )
      invalidate()
      trackEvent(METRICS.BUDGET_UPDATE)
      if (variables.endDate !== undefined) trackEvent(METRICS.BUDGET_SET_END_DATE)
      if (variables.savingsAccountIds !== undefined && !sameIdSet(variables.savingsAccountIds, variables.previousSavingsAccountIds ?? [])) {
        trackEvent(METRICS.BUDGET_SAVINGS_TOGGLE)
      }
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
      void queryClient.invalidateQueries({ queryKey: queryKeys.budgetPlan })
      // access also unlocks the owner's classifications and custom currencies
      // referenced by the budget's elements
      void queryClient.invalidateQueries({ queryKey: queryKeys.categories })
      void queryClient.invalidateQueries({ queryKey: queryKeys.tags })
      void queryClient.invalidateQueries({ queryKey: queryKeys.currencies })
      void queryClient.invalidateQueries({ queryKey: queryKeys.currencyRates })
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

/** replaces the budget's meta row in the list cache (archive/unarchive return it) */
function useReplaceBudgetMeta() {
  const queryClient = useQueryClient()
  return (meta: BudgetMetaDto) => {
    queryClient.setQueryData<BudgetMetaDto[]>(queryKeys.budgets, (prev) =>
      (prev ?? []).map((b) => (b.id === meta.id ? meta : b)),
    )
    void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
  }
}

export function useArchiveBudget() {
  const replace = useReplaceBudgetMeta()
  return useMutation({
    mutationFn: (id: Id) => budgetApi.archiveBudget(id),
    onSuccess: (meta) => {
      replace(meta)
      trackEvent(METRICS.BUDGET_ARCHIVE)
    },
  })
}

export function useUnarchiveBudget() {
  const replace = useReplaceBudgetMeta()
  return useMutation({
    mutationFn: (id: Id) => budgetApi.unarchiveBudget(id),
    onSuccess: (meta) => {
      replace(meta)
      trackEvent(METRICS.BUDGET_UNARCHIVE)
    },
  })
}

export function useCloneBudget() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (form: budgetApi.CloneBudgetForm) => budgetApi.cloneBudget(form),
    onSuccess: (meta) => {
      queryClient.setQueryData<BudgetMetaDto[]>(queryKeys.budgets, (prev) => [...(prev ?? []), meta])
      void queryClient.invalidateQueries({ queryKey: queryKeys.budgets })
      void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
      trackEvent(METRICS.BUDGET_CLONE)
    },
  })
}

type CommentListData = { items: BudgetCommentDto[]; truncated: boolean }

/** Cache key for one cell's thread: element external id + first-of-month period. */
export function commentCellKey(elementId: Id, period: string): string {
  return `${elementId}|${period}`
}

export function useBudgetComments(budgetId: Id | null, from: string, months: number) {
  const key = [...queryKeys.budgetComments, budgetId ?? 'none', from, months] as const
  const query = useQuery({
    queryKey: key,
    queryFn: () => budgetApi.getCommentList({ budgetId: budgetId as Id, from, months }),
    enabled: budgetId !== null,
    staleTime: TEN_MINUTES,
  })
  const byCell = useMemo(() => {
    const map = new Map<string, BudgetCommentDto[]>()
    for (const item of query.data?.items ?? []) {
      const cell = commentCellKey(item.elementId, item.period)
      const bucket = map.get(cell)
      if (bucket) {
        bucket.push(item)
      } else {
        map.set(cell, [item])
      }
    }
    return map
  }, [query.data])
  return { ...query, items: query.data?.items ?? [], truncated: query.data?.truncated ?? false, byCell, commentsKey: key }
}

// this budget's cached comment lists, across every fetched window
function budgetCommentsFilter(budgetId: Id) {
  return { queryKey: queryKeys.budgetComments, predicate: (query: { queryKey: readonly unknown[] }) => query.queryKey[1] === budgetId }
}

export function useCreateComment(budgetId: Id) {
  const queryClient = useQueryClient()
  return useMutation({
    // `id` is the server's idempotency key: a caller that may resend the same
    // comment passes one id for every attempt so the server can dedupe it
    mutationFn: ({ id, ...form }: { id?: Id; elementId: Id; period: string; comment: string }) =>
      budgetApi.createComment({ id: id ?? uuidv7(), budgetId, ...form }),
    onMutate: async (form) => {
      const filter = budgetCommentsFilter(budgetId)
      await queryClient.cancelQueries(filter)
      const previous = queryClient.getQueriesData<CommentListData>(filter)
      // a cache peek, never a fetch: subscribing via useUserData() here would pull in
      // getUserData's side effect of probing get-identity-list on every comment created
      const user = queryClient.getQueryData<CurrentUserDto>(queryKeys.user)
      const now = new Date().toISOString().slice(0, 19).replace('T', ' ')
      const optimistic: BudgetCommentDto = {
        id: form.id ?? uuidv7(),
        elementId: form.elementId,
        period: form.period,
        comment: form.comment,
        author: user ? { id: user.id, avatar: user.avatar, name: user.name } : { id: '', avatar: '', name: '' },
        createdAt: now,
        updatedAt: now,
      }
      queryClient.setQueriesData<CommentListData>(filter, (data) =>
        data ? { ...data, items: [...data.items, optimistic] } : data,
      )
      return { previous }
    },
    onError: (err, _form, context) => {
      for (const [key, data] of context?.previous ?? []) {
        queryClient.setQueryData(key, data)
      }
      toast.error(apiErrorMessage(err), { id: 'budget-comment-error' })
    },
    onSuccess: () => {
      trackEvent(METRICS.BUDGET_CREATE_COMMENT)
      void queryClient.invalidateQueries({ queryKey: queryKeys.budgetComments })
    },
  })
}

export function useUpdateComment(budgetId: Id) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (form: { id: Id; comment: string }) => budgetApi.updateComment(form),
    onError: (err) => {
      toast.error(apiErrorMessage(err), { id: 'budget-comment-error' })
    },
    onSuccess: () => {
      trackEvent(METRICS.BUDGET_UPDATE_COMMENT)
      void queryClient.invalidateQueries(budgetCommentsFilter(budgetId))
    },
  })
}

export function useDeleteComment(budgetId: Id) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (form: { id: Id }) => budgetApi.deleteComment(form),
    onError: (err) => {
      toast.error(apiErrorMessage(err), { id: 'budget-comment-error' })
    },
    onSuccess: () => {
      trackEvent(METRICS.BUDGET_DELETE_COMMENT)
      void queryClient.invalidateQueries(budgetCommentsFilter(budgetId))
    },
  })
}
