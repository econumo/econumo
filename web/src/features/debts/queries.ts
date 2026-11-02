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

// a debt write moves its budget row, the plan and every total that sums them,
// and the category/payee/tag it converts or pays through
function useInvalidateDebts() {
  const queryClient = useQueryClient()
  return () => {
    void queryClient.invalidateQueries({ queryKey: queryKeys.debts })
    void queryClient.invalidateQueries({ queryKey: queryKeys.budget })
    void queryClient.invalidateQueries({ queryKey: queryKeys.budgetPlan })
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

export function useUpdateDebt() {
  const invalidate = useInvalidateDebts()
  return useMutation({
    mutationFn: (form: UpdateDebtDto) => debtApi.updateDebt(form),
    onSuccess: () => {
      invalidate()
      trackEvent(METRICS.DEBT_UPDATE)
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
