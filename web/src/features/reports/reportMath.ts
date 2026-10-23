import type { FlowReportDto, NetWorthAccountDto, NetWorthReportDto, ReportRowDto } from '@/api/dto/report'
import type { Id } from '@/api/types'
import { abs, add, cmp, div, isNegative, isZero, mul, sub } from '@/lib/decimal'
import { currentMonth, monthDiff } from '@/features/budgets/planMath'

export type Step = 'month' | 'quarter' | 'year'

export interface Bucket {
  key: string
  start: string
  months: number
  total: string
  partial: boolean
  projected: string
}

export interface Insight {
  key: string
  params: Record<string, string>
}

const STEP_LENGTH: Record<Step, number> = { month: 1, quarter: 3, year: 12 }

const sum = (xs: string[]): string => xs.reduce((acc, x) => add(acc, x), '0')

function bucketStart(month: string, step: Step): string {
  const [y, m] = month.split('-')
  if (step === 'month') return month
  if (step === 'year') return `${y}-01-01`
  const q = Math.floor((Number(m) - 1) / 3) * 3 + 1
  return `${y}-${String(q).padStart(2, '0')}-01`
}

function daysInMonth(month: string): number {
  const [y, m] = month.split('-').map(Number)
  return new Date(y, m, 0).getDate()
}

export function rollUp(months: string[], amounts: string[], step: Step, today: Date = new Date()): Bucket[] {
  const len = STEP_LENGTH[step]
  const cur = currentMonth(today)
  const groups = new Map<string, { months: string[]; amounts: string[] }>()
  months.forEach((m, i) => {
    const start = bucketStart(m, step)
    const g = groups.get(start) ?? { months: [], amounts: [] }
    g.months.push(m)
    g.amounts.push(amounts[i] ?? '0')
    groups.set(start, g)
  })
  return [...groups.entries()]
    .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
    .map(([start, g]) => {
      const total = sum(g.amounts)
      const containsCurrent = monthDiff(start, cur) >= 0 && monthDiff(start, cur) < len
      const partial = containsCurrent || g.months.length < len
      // elapsed months = whole + day/dim, kept as the exact fraction (whole*dim + day)/dim
      const dim = daysInMonth(cur)
      const whole = g.months.filter((m) => m < cur).length
      const elapsedDays = whole * dim + (g.months.includes(cur) ? today.getDate() : 0)
      const projected = partial && elapsedDays > 0 ? div(mul(total, len * dim), elapsedDays) : total
      return { key: start, start, months: g.months.length, total, partial, projected }
    })
}

export function topGroups(rows: ReportRowDto[], n = 5): { ids: Id[]; otherIds: Id[] } {
  const ranked = rows
    .map((r, i) => ({ id: r.id, total: sum(r.amounts), i }))
    .sort((a, b) => cmp(b.total, a.total) || a.i - b.i)
  return { ids: ranked.slice(0, n).map((r) => r.id), otherIds: ranked.slice(n).map((r) => r.id) }
}

export interface YearSlot {
  month: number
  earlier: string | null
  later: string | null
}

export function compareYears(
  months: string[],
  amounts: string[],
  earlier: number,
  later: number,
  today: Date = new Date(),
): { slots: YearSlot[]; earlierTotal: string; laterTotal: string; throughMonth: number } {
  const byMonth = new Map<string, string>()
  months.forEach((m, i) => byMonth.set(m.slice(0, 7), amounts[i] ?? '0'))
  const throughMonth = later === today.getFullYear() ? today.getMonth() + 1 : 12
  const at = (year: number, month: number) => byMonth.get(`${year}-${String(month).padStart(2, '0')}`) ?? null
  const slots: YearSlot[] = []
  for (let month = 1; month <= throughMonth; month++) {
    slots.push({ month, earlier: at(earlier, month), later: at(later, month) })
  }
  return {
    slots,
    earlierTotal: sum(slots.map((s) => s.earlier ?? '0')),
    laterTotal: sum(slots.map((s) => s.later ?? '0')),
    throughMonth,
  }
}

function rangeSum(row: ReportRowDto, months: string[], range: [string, string]): string {
  return sum(months.map((m, i) => (m >= range[0] && m <= range[1] ? (row.amounts[i] ?? '0') : '0')))
}

export interface ChangeRow {
  id: Id
  name: string
  kind: string
  a: string
  b: string
  diff: string
  pct: number | null
  monthlyAvg: string
}

export function changeRows(
  rows: ReportRowDto[],
  months: string[],
  a: [string, string],
  b: [string, string],
): ChangeRow[] {
  const bMonths = Math.max(1, monthDiff(b[0], b[1]) + 1)
  return rows
    .map((r) => {
      const av = rangeSum(r, months, a)
      const bv = rangeSum(r, months, b)
      const diff = sub(bv, av)
      const pct = isZero(av) ? null : Number(div(mul(diff, 100), abs(av)))
      return { id: r.id, name: r.name, kind: r.kind, a: av, b: bv, diff, pct, monthlyAvg: div(bv, bMonths) }
    })
    .sort((x, y) => cmp(abs(y.diff), abs(x.diff)))
}

export interface CashFlowMonth {
  month: string
  income: string
  spent: string
  saved: string
  transfersIn: string
  transfersOut: string
}

export function cashFlow(report: FlowReportDto): CashFlowMonth[] {
  return report.months.map((month, i) => {
    const income = sum(report.income.map((r) => r.amounts[i] ?? '0'))
    const spent = sum(report.expense.map((r) => r.amounts[i] ?? '0'))
    return {
      month,
      income,
      spent,
      saved: sub(income, spent),
      transfersIn: report.transfersIn[i] ?? '0',
      transfersOut: report.transfersOut[i] ?? '0',
    }
  })
}

export function netWorthSeries(
  report: NetWorthReportDto,
  groupOf: (a: NetWorthAccountDto) => { id: string; name: string },
): { groups: { id: string; name: string; balances: string[] }[]; total: string[] } {
  const zeros = () => report.months.map(() => '0')
  const groups = new Map<string, { id: string; name: string; balances: string[] }>()
  const total = zeros()
  for (const acc of report.accounts) {
    const { id, name } = groupOf(acc)
    const g = groups.get(id) ?? { id, name, balances: zeros() }
    groups.set(id, g)
    report.months.forEach((_, i) => {
      const v = acc.balances[i] ?? '0'
      g.balances[i] = add(g.balances[i], v)
      total[i] = add(total[i], v)
    })
  }
  return { groups: [...groups.values()], total }
}

const EMPTY: Insight = { key: 'reports.insight.empty', params: {} }

export function spendingInsight(rows: ReportRowDto[]): Insight {
  const totals = rows.map((r) => ({ name: r.name, total: sum(r.amounts) }))
  const all = sum(totals.map((t) => t.total))
  if (totals.length === 0 || cmp(all, 0) <= 0) return EMPTY
  const top = totals.reduce((best, t) => (cmp(t.total, best.total) > 0 ? t : best))
  const share = Math.round(Number(div(mul(top.total, 100), all)))
  return { key: 'reports.insight.spending_top', params: { name: top.name, amount: top.total, share: String(share) } }
}

export function cashFlowInsight(months: CashFlowMonth[]): Insight {
  if (months.length === 0) return EMPTY
  const income = sum(months.map((m) => m.income))
  const saved = sum(months.map((m) => m.saved))
  if (isNegative(saved)) return { key: 'reports.insight.cashflow_overspent', params: { amount: abs(saved) } }
  const rate = isZero(income) ? 0 : Math.round(Number(div(mul(saved, 100), income)))
  return { key: 'reports.insight.cashflow_saved', params: { amount: saved, rate: String(rate) } }
}

export function netWorthInsight(total: string[]): Insight {
  if (total.length < 2) return EMPTY
  const change = sub(total[total.length - 1], total[0])
  if (isZero(change)) return { key: 'reports.insight.networth_flat', params: {} }
  return {
    key: isNegative(change) ? 'reports.insight.networth_down' : 'reports.insight.networth_up',
    params: { amount: abs(change) },
  }
}

