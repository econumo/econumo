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

export interface NetWorthQueryArgs {
  budgetId: Id | null
  from?: string
  to?: string
}

// Balances are month-end, so a period's starting balance is the close of the
// month before it: "this year" starts from December's close, like the report
// list's card.
export function netWorthQueryArgs(p: ReportParams, today: Date = new Date()): NetWorthQueryArgs {
  const from = periodFrom(p, today)
  const args: NetWorthQueryArgs = { budgetId: p.budgetId, from: from ? addMonths(from, -1) : undefined }
  if (p.period === 'custom' && p.to) args.to = p.to
  return args
}

export type MonthRange = [string, string]

export function lastMonthOf(b: Bucket): string {
  return b.last
}

/** the months a preset or custom period covers; null for all time, which has no fixed start */
export function periodRange(p: ReportParams, today: Date = new Date()): MonthRange | null {
  const from = periodFrom(p, today)
  if (!from) return null
  return [from, p.period === 'custom' && p.to ? p.to : currentMonth(today)]
}

/** the equal-length period just before `range` */
export function previousRange([from, to]: MonthRange): MonthRange {
  const n = monthDiff(from, to) + 1
  return [addMonths(from, -n), addMonths(from, -1)]
}

export function monthsInRange([from, to]: MonthRange): number {
  return monthDiff(from, to) + 1
}

const utc = (m: string) => new Date(Date.UTC(Number(m.slice(0, 4)), Number(m.slice(5, 7)) - 1, 15))

export interface PeriodLabels {
  /** "Oct 2026" */
  month: (m: string) => string
  /** "Jan" */
  monthShort: (month: number) => string
  /** a step bucket: "2026", "Q4 2026", "Oct 2026" */
  bucket: (start: string, step: Step) => string
  /** a month range: "2025", "Q3 2026", "Oct 2026", "Jan – Oct 2026", "Nov 2025 – Feb 2026" */
  range: (range: MonthRange) => string
  /** a range of months within one year, without the year: "Jan – Oct" */
  monthsOfYear: (from: number, to: number) => string
}

// Everything goes through Intl in the UI language, so word order and month
// names follow the language; only the quarter has no Intl form and comes from
// the catalogue.
export function periodLabels(lang: string, quarter: (q: number, year: string) => string): PeriodLabels {
  const monthYear = new Intl.DateTimeFormat(lang, { month: 'short', year: 'numeric', timeZone: 'UTC' })
  const monthOnly = new Intl.DateTimeFormat(lang, { month: 'short', timeZone: 'UTC' })
  const yearOnly = new Intl.DateTimeFormat(lang, { year: 'numeric', timeZone: 'UTC' })
  const quarterOf = (m: string) => Math.floor((Number(m.slice(5, 7)) - 1) / 3) + 1
  const bucket = (start: string, step: Step) => {
    if (step === 'year') return yearOnly.format(utc(start))
    if (step === 'quarter') return quarter(quarterOf(start), yearOnly.format(utc(start)))
    return monthYear.format(utc(start))
  }
  const range = ([from, to]: MonthRange) => {
    if (from === to) return monthYear.format(utc(from))
    const fm = Number(from.slice(5, 7))
    const tm = Number(to.slice(5, 7))
    if (from.slice(0, 4) === to.slice(0, 4)) {
      if (fm === 1 && tm === 12) return bucket(from, 'year')
      if ((fm - 1) % 3 === 0 && tm === fm + 2) return bucket(from, 'quarter')
    }
    return monthYear.formatRange(utc(from), utc(to))
  }
  const monthOf = (month: number) => utc(`2000-${String(month).padStart(2, '0')}-01`)
  return {
    month: (m) => monthYear.format(utc(m)),
    monthShort: (month) => monthOnly.format(monthOf(month)),
    bucket,
    range,
    monthsOfYear: (from, to) => (from === to ? monthOnly.format(monthOf(from)) : monthOnly.formatRange(monthOf(from), monthOf(to))),
  }
}
