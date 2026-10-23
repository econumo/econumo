import { keepPreviousData, useQuery } from '@tanstack/react-query'
import * as reportApi from '@/api/report'
import type { FlowReportDto, GroupBy, NetWorthReportDto } from '@/api/dto/report'
import type { Id } from '@/api/types'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'

interface ReportQueryParams {
  budgetId: Id | null
  from?: string
  to?: string
}

export function useFlowReport(params: ReportQueryParams & { groupBy?: GroupBy }) {
  const { budgetId, from, to, groupBy } = params
  return useQuery<FlowReportDto>({
    queryKey: [...queryKeys.reportFlow, budgetId ?? 'mine', from ?? '', to ?? '', groupBy ?? ''],
    queryFn: () => reportApi.getFlowReport({ budgetId, from, to, groupBy }),
    staleTime: TEN_MINUTES,
    placeholderData: keepPreviousData,
  })
}

export function useNetWorthReport(params: ReportQueryParams) {
  const { budgetId, from, to } = params
  return useQuery<NetWorthReportDto>({
    queryKey: [...queryKeys.reportNetWorth, budgetId ?? 'mine', from ?? '', to ?? '', ''],
    queryFn: () => reportApi.getNetWorthReport({ budgetId, from, to }),
    staleTime: TEN_MINUTES,
    placeholderData: keepPreviousData,
  })
}
