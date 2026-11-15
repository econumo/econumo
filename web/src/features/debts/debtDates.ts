import { formatDate } from '@/lib/datetime'
import type { DebtDetailsDto } from '@/api/dto/debt'

// debt dates are bare Y-m-d (a payment's carries a time); read as local days
function localDay(date: string): Date {
  const [y, m, d] = date.slice(0, 10).split('-').map(Number)
  return new Date(y, m - 1, d || 1)
}

/** "Jul 2050" */
export function debtMonth(date: string, lang: string): string {
  return new Intl.DateTimeFormat(lang, { month: 'short', year: 'numeric' }).format(localDay(date))
}

/** "Jul 5, 2050" */
export function debtDay(date: string, lang: string): string {
  return new Intl.DateTimeFormat(lang, { day: 'numeric', month: 'short', year: 'numeric' }).format(localDay(date))
}

/** the Y-m-d `days` after `date` (before when negative) */
export function shiftDay(date: string, days: number): string {
  const d = localDay(date)
  d.setDate(d.getDate() + days)
  return formatDate(d)
}

/** The days a debt can be marked paid off on: after the start date, no earlier
    than the latest statement (one after the origin) and no later than today.
    `min` is "" when the details hold no start date. */
export function paidOffBounds(details: DebtDetailsDto | undefined, today: string): { min: string; max: string; start: string; statement: string } {
  const start = details?.item.startDate ?? ''
  const statements = (details?.snapshots ?? []).filter((s) => s.isOrigin !== 1).map((s) => s.asOf.slice(0, 10))
  const statement = statements.length > 0 ? statements.reduce((a, b) => (b > a ? b : a)) : ''
  const afterStart = start ? shiftDay(start, 1) : ''
  const min = statement > afterStart ? statement : afterStart
  return { min, max: today, start, statement }
}

export type PaidOffProblem = 'future' | 'before_start' | 'before_statement'

/** why `date` cannot be a paid-off date, null when nothing client-side rules it out */
export function paidOffProblem(date: string, bounds: ReturnType<typeof paidOffBounds>): PaidOffProblem | null {
  if (date > bounds.max) {
    return 'future'
  }
  if (bounds.start && date <= bounds.start) {
    return 'before_start'
  }
  if (bounds.statement && date < bounds.statement) {
    return 'before_statement'
  }
  return null
}
