import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as debtApi from '@/api/debt'
import type { CreateDebtDto, UpdateDebtDto, DeleteDebtForm } from '@/api/dto/debt'
import type { Id } from '@/api/types'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { METRICS, trackEvent } from '@/lib/metrics'

export function useDebts() {
  return useQuery({ queryKey: queryKeys.debts, queryFn: debtApi.getDebtList, staleTime: TEN_MINUTES })
}

export function useDebt(id: Id | undefined) {
  return useQuery({
    queryKey: queryKeys.debt(id ?? ''),
    queryFn: () => debtApi.getDebt(id as Id),
    enabled: !!id,
    staleTime: TEN_MINUTES,
  })
}

/** Loads the debt afresh for Edit: the form pre-fills the balance with the
 *  current one, and a cached balance from before a payment would store a
 *  snapshot that loses the payment's principal. */
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
  return useMutation({
    mutationFn: (form: UpdateDebtDto) => debtApi.updateDebt(form),
    onSuccess: (saved) => {
      invalidate()
      trackEvent(finishingSetup && saved.isSetUp === 1 ? METRICS.DEBT_FINISH_SETUP : METRICS.DEBT_UPDATE)
    },
  })
}

export function useDeleteDebt() {
  const invalidate = useInvalidateDebts()
  return useMutation({
    mutationFn: (form: DeleteDebtForm) => debtApi.deleteDebt(form),
    onSuccess: () => {
      invalidate()
      trackEvent(METRICS.DEBT_DELETE)
    },
  })
}

export function useMarkDebtPaidOff() {
  const invalidate = useInvalidateDebts()
  return useMutation({
    mutationFn: (id: Id) => debtApi.markDebtPaidOff(id),
    onSuccess: () => {
      invalidate()
      trackEvent(METRICS.DEBT_MARK_PAID_OFF)
    },
  })
}
