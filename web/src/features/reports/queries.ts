import { keepPreviousData, useQuery } from '@tanstack/react-query'
import * as reportApi from '@/api/report'
import type { NetWorthReportDto } from '@/api/dto/report'
import type { Id } from '@/api/types'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { useBudgets } from '@/features/budgets/queries'

export function useNetWorthReport(params: { budgetId: Id | null; from?: string; to?: string }) {
  const { budgetId, from, to } = params
  return useQuery<NetWorthReportDto>({
    queryKey: [...queryKeys.reportNetWorth, budgetId ?? 'mine', from ?? '', to ?? '', ''],
    queryFn: () => reportApi.getNetWorthReport({ budgetId, from, to }),
    staleTime: TEN_MINUTES,
    placeholderData: keepPreviousData,
  })
}

/** Budgets a report can be scoped to: archived ones are left out. With none
 *  left the scope picker disappears and every report is "My accounts". */
export function useReportBudgets() {
  const query = useBudgets()
  return { ...query, data: query.data?.filter((b) => b.isArchived !== 1) }
}
