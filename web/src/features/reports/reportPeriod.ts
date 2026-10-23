import type { GroupBy } from '@/api/dto/report'
import type { Id } from '@/api/types'
import { addMonths, currentMonth, monthDiff } from '@/features/budgets/planMath'
import type { Bucket, Step } from './reportMath'
import type { ReportParams } from './reportTabs'

export interface FlowQueryArgs {
  budgetId: Id | null
  from?: string
  to?: string
  groupBy?: GroupBy
}

export function compareYearsOf(p: ReportParams, today: Date = new Date()): [number, number] {
  const y = today.getFullYear()
  return p.compare ?? [y - 1, y]
}

export function periodFrom(p: ReportParams, today: Date = new Date()): string | undefined {
  const y = today.getFullYear()
  switch (p.period) {
    case 'thisYear':
      return `${y}-01-01`
    case 'last12':
      return addMonths(currentMonth(today), -11)
    case 'last3y':
      return `${y - 2}-01-01`
    case 'all':
      return undefined
    case 'custom':
      return p.from
  }
}

// The server ends every window at the current month unless told otherwise, so
// only a custom range sends `to`. Group-by is a my-accounts switch: the budget
// scope always groups by the budget's own rows. Category is the server's
// default and is left off so the report list's figures share the cache entry.
export function flowQueryArgs(p: ReportParams, today: Date = new Date()): FlowQueryArgs {
  const groupBy = p.budgetId === null && p.groupBy !== 'category' ? p.groupBy : undefined
  if (p.mode === 'compare') {
    return { budgetId: p.budgetId, from: `${compareYearsOf(p, today)[0]}-01-01`, groupBy }
  }
  const args: FlowQueryArgs = { budgetId: p.budgetId, from: periodFrom(p, today), groupBy }
  if (p.period === 'custom' && p.to) args.to = p.to
  return args
}

export type MonthRange = [string, string]

export function lastMonthOf(b: Bucket): string {
  return addMonths(b.start, b.months - 1)
}

const STEP_LENGTH: Record<Step, number> = { month: 1, quarter: 3, year: 12 }

/**
 * The two ranges the Over time headline and table compare: the latest bucket
 * against the one before it, like-for-like. A latest bucket that is cut short
 * (the current year so far, or a custom range ending mid-quarter) is compared
 * with the same number of months of the bucket before. When the latest bucket
 * holds nothing but the unfinished current month, a months-granular comparison
 * would pit a few days against a whole month, so the two complete buckets
 * before it are compared instead. A previous bucket cut by the window start
 * has no like-for-like counterpart: no comparison then.
 */
export function comparisonRanges(buckets: Bucket[], step: Step, today: Date = new Date()): { a: MonthRange; b: MonthRange } | null {
  const len = STEP_LENGTH[step]
  const cur = currentMonth(today)
  let list = buckets
  const last = list[list.length - 1]
  if (last && last.months === 1 && last.start === cur) {
    list = list.slice(0, -1)
  }
  if (list.length < 2) return null
  const later = list[list.length - 1]
  const earlier = list[list.length - 2]
  if (earlier.months < len) return null
  const n = Math.min(later.months, len)
  return {
    a: [earlier.start, addMonths(earlier.start, n - 1)],
    b: [later.start, addMonths(later.start, n - 1)],
  }
}

const monthFmt = (lang: string, month: 'short' | 'long' = 'short') => new Intl.DateTimeFormat(lang, { month, timeZone: 'UTC' })
const monthName = (m: string, lang: string) => monthFmt(lang).format(new Date(`${m.slice(0, 7)}-15T00:00:00Z`))

export function monthLabel(m: string, lang: string): string {
  return `${monthName(m, lang)} ${m.slice(0, 4)}`
}

export function bucketLabel(start: string, step: Step, lang: string): string {
  if (step === 'year') return start.slice(0, 4)
  if (step === 'quarter') return `Q${Math.floor((Number(start.slice(5, 7)) - 1) / 3) + 1} ${start.slice(0, 4)}`
  return monthLabel(start, lang)
}

/** "2025", "Q3 2026", "Oct 2026", "Jan–Oct 2026", "Nov 2025 – Feb 2026" */
export function rangeLabel([from, to]: MonthRange, lang: string): string {
  const fy = from.slice(0, 4)
  const ty = to.slice(0, 4)
  const fm = Number(from.slice(5, 7))
  const tm = Number(to.slice(5, 7))
  if (from === to) return monthLabel(from, lang)
  if (fy === ty) {
    if (fm === 1 && tm === 12) return fy
    if ((fm - 1) % 3 === 0 && tm === fm + 2) return bucketLabel(from, 'quarter', lang)
    return `${monthName(from, lang)}–${monthName(to, lang)} ${fy}`
  }
  return `${monthLabel(from, lang)} – ${monthLabel(to, lang)}`
}

export function monthsInRange([from, to]: MonthRange): number {
  return monthDiff(from, to) + 1
}

export function shortMonth(month: number, lang: string): string {
  return monthName(`2000-${String(month).padStart(2, '0')}-01`, lang)
}
