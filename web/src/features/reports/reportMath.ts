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
      const sinceStart = monthDiff(start, cur)
      const containsCurrent = sinceStart >= 0 && sinceStart < len
      const partial = containsCurrent || g.months.length < len
      // Only the bucket holding the current month is projected, and only when the
      // window covers it from its first month: scaling a window-cut slice to the
      // full step would invent spending outside the chosen period.
      const whole = g.months.filter((m) => m < cur).length
      const projectable = containsCurrent && g.months.includes(cur) && whole === sinceStart
      // elapsed months = whole + day/dim, kept as the exact fraction (whole*dim + day)/dim
      const dim = daysInMonth(cur)
      const elapsedDays = whole * dim + today.getDate()
      const projected = projectable ? div(mul(total, len * dim), elapsedDays) : total
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

export interface BucketChange {
  earlier: Bucket | null
  later: Bucket
  rows: ChangeRow[]
}

/**
 * The Over time table: the last bucket against the one before it. A later
 * quarter or year still in progress is compared through its projection (its own value
 * stays actual-to-date), so an unfinished year does not read as a drop. An
 * earlier bucket cut by the window start has nothing like-for-like to offer,
 * so it is left out and only the later bucket is shown.
 */
export function bucketChanges(rows: ReportRowDto[], allMonths: string[], step: Step, today: Date = new Date()): BucketChange | null {
  // in Month step the running month is too short to compare, even projected
  // (a bill paid on the 2nd would read as a jump), so the table takes the
  // last two complete months; the chart still shows the running one
  const cut = step === 'month' && allMonths[allMonths.length - 1] === currentMonth(today) ? 1 : 0
  const months = cut ? allMonths.slice(0, -1) : allMonths
  if (cut) rows = rows.map((r) => ({ ...r, amounts: r.amounts.slice(0, months.length) }))
  const totals = rollUp(months, months.map((_, i) => sum(rows.map((r) => r.amounts[i] ?? '0'))), step, today)
  if (totals.length === 0) return null
  const later = totals[totals.length - 1]
  const prev = totals.length > 1 ? totals[totals.length - 2] : null
  const earlier = prev && prev.months === STEP_LENGTH[step] ? prev : null
  const list = rows
    .map((r) => {
      const b = rollUp(months, r.amounts, step, today)
      const lb = b[b.length - 1]
      const eb = earlier ? b[b.length - 2] : undefined
      const av = eb?.total ?? '0'
      const diff = sub(lb.projected, av)
      const pct = earlier === null || isZero(av) ? null : Number(div(mul(diff, 100), abs(av)))
      return {
        id: r.id, name: r.name, kind: r.kind, a: av, b: lb.total, diff, pct,
        monthlyAvg: div(lb.total, Math.max(1, lb.months)),
      }
    })
    .sort((x, y) => (earlier ? cmp(abs(y.diff), abs(x.diff)) : cmp(y.b, x.b)))
  return { earlier, later, rows: list }
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

export interface NetWorthGrouping {
  me: Id
  /** my accounts' folder, from my account list; deleted accounts are not in it */
  folderOf: Record<Id, Id | null>
  folders: { id: Id; name: string }[]
  /** budget participants' names by user id */
  owners: Record<Id, string>
  names: { noFolder: string; deleted: string; otherOwner: string }
}

// My accounts sit in my folders; anyone else's account is listed under its
// owner's name, since their folders are theirs and are never shown.
export function netWorthGroupOf(g: NetWorthGrouping) {
  const folderName = new Map(g.folders.map((f) => [f.id, f.name]))
  return (a: NetWorthAccountDto): { id: string; name: string } => {
    if (a.ownerUserId !== g.me) return { id: `owner:${a.ownerUserId}`, name: g.owners[a.ownerUserId] ?? g.names.otherOwner }
    if (a.isDeleted) return { id: 'deleted', name: g.names.deleted }
    const folderId = g.folderOf[a.id]
    const name = folderId ? folderName.get(folderId) : undefined
    return folderId && name !== undefined ? { id: `folder:${folderId}`, name } : { id: 'nofolder', name: g.names.noFolder }
  }
}

const EMPTY: Insight = { key: 'reports.insight.empty', params: {} }

export function spendingInsight(rows: ReportRowDto[]): Insight {
  const totals = rows.map((r) => ({ id: r.id, name: r.name, total: sum(r.amounts) }))
  const all = sum(totals.map((t) => t.total))
  if (totals.length === 0 || cmp(all, 0) <= 0) return EMPTY
  const top = totals.reduce((best, t) => (cmp(t.total, best.total) > 0 ? t : best))
  const share = Math.round(Number(div(mul(top.total, 100), all)))
  return { key: 'reports.insight.spending_top', params: { id: top.id, name: top.name, amount: top.total, share: String(share) } }
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


export const FLOW_HUB = '__income'
export const FLOW_BALANCE = '__balance'
export const FLOW_SAVED = '__saved'
export const FLOW_TO_SAVINGS = '__toSavings'
export const FLOW_STAYED = '__stayed'
export const FLOW_OTHER_INCOME = '__otherIncome'
export const FLOW_OTHER_EXPENSE = '__otherExpense'
export const FLOW_TRANSFER_IN = '__transferIn'
export const FLOW_TRANSFER_OUT = '__transferOut'
const FLOW_TOP_INCOME = 5
const FLOW_TOP_EXPENSE = 7

export interface FlowNode {
  id: string
  kind: 'income' | 'expense' | 'special'
  /** the row's own name and kind; special nodes are named by the caller */
  name: string
  rowKind: string
  value: string
}

export interface FlowGraph {
  /** false when transfers in or a balance draw feed the hub, so it is more than income */
  hubIncomeOnly: boolean
  nodes: FlowNode[]
  links: { source: number; target: number; value: string }[]
}

const positive = (v: string) => (cmp(v, 0) > 0 ? v : '0')

function flowSide(rows: ReportRowDto[], n: number, kind: FlowNode['kind'], otherId: string): FlowNode[] {
  const ranked = rows
    .map((r, i) => ({ r, total: sum(r.amounts), i }))
    .sort((a, b) => cmp(b.total, a.total) || a.i - b.i)
  const top = ranked.slice(0, n).filter((x) => cmp(x.total, 0) > 0)
  // a row netting below zero (refunds over spending) lowers Other rather than
  // vanishing, so the flow stays as close to the headline as a Sankey allows
  const rest = positive(sum(ranked.filter((x) => !top.includes(x)).map((x) => x.total)))
  const nodes: FlowNode[] = top.map((x) => ({ id: x.r.id, kind, name: x.r.name, rowKind: x.r.kind, value: x.total }))
  if (cmp(rest, 0) > 0) nodes.push({ id: otherId, kind, name: '', rowKind: '', value: rest })
  return nodes
}

/**
 * The Flow view: income rows and transfers in -> the hub -> expense rows,
 * transfers out and what was left. In a budget with savings members, what
 * was left splits into what moved to them (Plan's Saved, never below zero)
 * and what stayed in everyday accounts. Whenever the outflows exceed the
 * inflows, "From balance" covers the gap, so the hub's two sides always add
 * up.
 */
export function flowGraph(report: FlowReportDto, budget: boolean): FlowGraph {
  const income = flowSide(report.income, FLOW_TOP_INCOME, 'income', FLOW_OTHER_INCOME)
  const expense = flowSide(report.expense, FLOW_TOP_EXPENSE, 'expense', FLOW_OTHER_EXPENSE)
  const special = (id: string, value: string): FlowNode[] => (cmp(value, 0) > 0 ? [{ id, kind: 'special', name: '', rowKind: '', value }] : [])
  const transferIn = special(FLOW_TRANSFER_IN, positive(sum(report.transfersIn)))
  const transferOut = special(FLOW_TRANSFER_OUT, positive(sum(report.transfersOut)))
  const split = budget && report.savings.length > 0
  const moved = split ? positive(sum(report.savings.flatMap((r) => r.amounts))) : '0'
  const inflow = sum([...income, ...transferIn].map((x) => x.value))
  const outflow = sum([...expense, ...transferOut].map((x) => x.value))
  const left = sub(inflow, add(outflow, moved))
  const balance = special(FLOW_BALANCE, positive(sub('0', left)))
  const sources = [...income, ...transferIn, ...balance]
  const outputs = [
    ...expense,
    ...transferOut,
    ...(split ? [...special(FLOW_TO_SAVINGS, moved), ...special(FLOW_STAYED, positive(left))] : special(FLOW_SAVED, positive(left))),
  ]
  if (sources.length === 0 || outputs.length === 0) return { hubIncomeOnly: true, nodes: [], links: [] }
  const hubValue = sum(sources.map((x) => x.value))
  const nodes = [...sources, { id: FLOW_HUB, kind: 'special' as const, name: '', rowKind: '', value: hubValue }, ...outputs]
  const hub = sources.length
  return {
    hubIncomeOnly: transferIn.length === 0 && balance.length === 0,
    nodes,
    links: [
      ...sources.map((x, i) => ({ source: i, target: hub, value: x.value })),
      ...outputs.map((x, i) => ({ source: hub, target: hub + 1 + i, value: x.value })),
    ],
  }
}
