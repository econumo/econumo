import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as debtApi from '@/api/debt'
import type { AddDebtRuleDto, CreateDebtDto, DebtDetailsDto, DebtDto, UpdateDebtDto, DeleteDebtForm, DeleteDebtReconciliationForm, ReconcileDebtForm } from '@/api/dto/debt'
import type { Id } from '@/api/types'
import { isDebtForecastKey, queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { METRICS, trackEvent } from '@/lib/metrics'
import type { Metric } from '@/lib/metrics'
import { canEditBudget, useBudgets } from '@/features/budgets/queries'
import { useUserData } from '@/features/user/queries'

export function useDebts({ enabled = true }: { enabled?: boolean } = {}) {
  return useQuery({ queryKey: queryKeys.debts, queryFn: debtApi.getDebtList, staleTime: TEN_MINUTES, enabled })
}

export function useDebt(id: Id | undefined) {
  return useQuery({
    queryKey: queryKeys.debt(id ?? ''),
    queryFn: () => debtApi.getDebt(id as Id),
    enabled: !!id,
    staleTime: TEN_MINUTES,
  })
}

/** The payments still to come and the figures they give (payments left,
 *  payoff, overpayment), with the planned extras of the debt's own budget,
 *  loaded while `enabled` and never served stale: a plan saved in that budget
 *  moves them (the budget's plan writes invalidate every forecast too) */
export function useDebtForecast(id: Id, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.debtForecast(id),
    queryFn: () => debtApi.getDebtForecast(id),
    enabled,
    staleTime: 0,
  })
}

/** What a rule of a debt in the budget may name: each participant's active
 *  expense categories, payees, tags and labels (the only place a partner's
 *  are listed). Refetched on every form opening: a category made meanwhile
 *  must be offered */
export function useDebtRuleOptions(budgetId: Id | null) {
  return useQuery({
    queryKey: queryKeys.debtRuleOptions(budgetId ?? ''),
    queryFn: () => debtApi.getDebtRuleOptions(budgetId as Id),
    enabled: !!budgetId,
    staleTime: 0,
  })
}

/** the budgets a new debt can go into: the user plans in them (owner, admin
 *  or user) and they are not archived. undefined while loading */
export function usePlannableBudgets() {
  const { data: user } = useUserData()
  const { data: budgets } = useBudgets()
  return user && budgets ? budgets.filter((b) => b.isArchived === 0 && canEditBudget(b, user.id)) : undefined
}

/** Loads the debt afresh for the Settings tab: the form starts from the loan's
 *  facts as stored now (amountBorrowed and lastPaymentDate, which go together,
 *  payment, repeats, start date), so a cached copy cannot send back facts
 *  another device has corrected since. */
export function useFetchDebtForEdit() {
  const queryClient = useQueryClient()
  return (id: Id) => queryClient.fetchQuery({ queryKey: queryKeys.debt(id), queryFn: () => debtApi.getDebt(id), staleTime: 0 })
}

// a debt write moves its budget row, the plan and every total that sums them,
// the budget's transaction drill-downs, which list a debt row's payments, and
// (converting or deleting a whole-category debt) comments between the rows
function useInvalidateDebts() {
  const queryClient = useQueryClient()
  return () => {
    void queryClient.invalidateQueries({ queryKey: queryKeys.debts })
    void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
    void queryClient.invalidateQueries({ queryKey: queryKeys.budgetPlan })
    void queryClient.invalidateQueries({ queryKey: queryKeys.budgetTransactions })
    void queryClient.invalidateQueries({ queryKey: queryKeys.budgetComments })
  }
}

// The debt a write returns is the debt as stored now. It goes into both caches at
// once, so the Overview shows it before the refetch lands.
function useStoreSavedDebt() {
  const queryClient = useQueryClient()
  return (saved: DebtDto) => {
    queryClient.setQueryData<DebtDetailsDto>(queryKeys.debt(saved.id), (old) => (old ? { ...old, item: saved } : old))
    queryClient.setQueryData<DebtDto[]>(queryKeys.debts, (old) => old?.map((d) => (d.id === saved.id ? saved : d)))
  }
}

export function useCreateDebt({ fromBudget = false }: { fromBudget?: boolean } = {}) {
  const invalidate = useInvalidateDebts()
  return useMutation({
    mutationFn: (form: CreateDebtDto) => debtApi.createDebt(form),
    onSuccess: () => {
      invalidate()
      trackEvent(fromBudget ? METRICS.DEBT_CONVERT : METRICS.DEBT_CREATE)
    },
  })
}

/** `finishingSetup`: the debt being saved is not set up yet, so a save that
 *  comes back set up is the debt's setup finished, not an edit */
export function useUpdateDebt({ finishingSetup = false }: { finishingSetup?: boolean } = {}) {
  const invalidate = useInvalidateDebts()
  const storeSaved = useStoreSavedDebt()
  return useMutation({
    mutationFn: (form: UpdateDebtDto) => debtApi.updateDebt(form),
    onSuccess: (saved) => {
      storeSaved(saved)
      invalidate()
      trackEvent(finishingSetup && saved.isSetUp === 1 ? METRICS.DEBT_FINISH_SETUP : METRICS.DEBT_UPDATE)
    },
  })
}

/** "Add to a debt": its caller shows a refusal inline */
export function useAddDebtRule() {
  const invalidate = useInvalidateDebts()
  return useMutation({
    meta: { handlesOwnErrors: true },
    mutationFn: (form: AddDebtRuleDto) => debtApi.addDebtRule(form),
    onSuccess: () => {
      invalidate()
      trackEvent(METRICS.DEBT_RULE_ADD)
    },
  })
}

export function useDeleteDebt() {
  const invalidate = useInvalidateDebts()
  return useMutation({
    // useDebtDangerActions shows every failure itself (inline or a toast)
    meta: { handlesOwnErrors: true },
    mutationFn: (form: DeleteDebtForm) => debtApi.deleteDebt(form),
    onSuccess: () => {
      invalidate()
      trackEvent(METRICS.DEBT_DELETE)
    },
  })
}

export function useMarkDebtPaidOff() {
  const invalidate = useInvalidateDebts()
  const storeSaved = useStoreSavedDebt()
  return useMutation({
    meta: { handlesOwnErrors: true },
    mutationFn: (id: Id) => debtApi.markDebtPaidOff(id),
    onSuccess: (saved) => {
      storeSaved(saved)
      invalidate()
      trackEvent(METRICS.DEBT_MARK_PAID_OFF)
    },
  })
}

// A statement written or removed moves the debt's history, every split after
// its date, the figures and the projected payments.
function useReconciliationWrite<T>(write: (form: T) => Promise<DebtDto>, metric: Metric) {
  const queryClient = useQueryClient()
  const invalidate = useInvalidateDebts()
  const storeSaved = useStoreSavedDebt()
  return useMutation({
    mutationFn: write,
    onSuccess: (saved) => {
      storeSaved(saved)
      invalidate()
      void queryClient.invalidateQueries({ queryKey: queryKeys.debt(saved.id) })
      void queryClient.invalidateQueries({ predicate: (q) => isDebtForecastKey(q.queryKey) })
      trackEvent(metric)
    },
  })
}

export function useReconcileDebt() {
  return useReconciliationWrite((form: ReconcileDebtForm) => debtApi.reconcileDebt(form), METRICS.DEBT_RECONCILE)
}

export function useDeleteDebtReconciliation() {
  return useReconciliationWrite((form: DeleteDebtReconciliationForm) => debtApi.deleteDebtReconciliation(form), METRICS.DEBT_RECONCILIATION_DELETE)
}
