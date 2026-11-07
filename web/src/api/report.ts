import { api, apiUrl } from './client'
import type { Id } from './types'
import type { NetWorthReportDto } from './dto/report'

interface Envelope<T> {
  data: T
}

export interface ReportParams {
  budgetId: Id | null
  from?: string
  to?: string
}

function query(entries: Record<string, string | null | undefined>): string {
  const qs = new URLSearchParams()
  for (const [k, v] of Object.entries(entries)) {
    if (v) qs.set(k, v)
  }
  const s = qs.toString()
  return s ? `?${s}` : ''
}

export async function getNetWorthReport(p: ReportParams): Promise<NetWorthReportDto> {
  const response = await api.get<Envelope<{ item: NetWorthReportDto }>>(
    apiUrl(`/api/v1/report/get-net-worth-report${query({ budgetId: p.budgetId, from: p.from, to: p.to })}`),
  )
  return response.data.data.item
}
