import type { Id } from '@/api/types'
import { addMonths, currentMonth } from '@/features/budgets/planMath'
import type { ReportParams } from './reportTabs'

export interface NetWorthQueryArgs {
  budgetId: Id | null
  from?: string
}

function periodFrom(p: ReportParams, today: Date): string | undefined {
  switch (p.period) {
    case 'last12':
      return addMonths(currentMonth(today), -11)
    case 'last3y':
      return `${today.getFullYear() - 2}-01-01`
    case 'all':
      return undefined
  }
}

// Balances are month-end, so a period's starting balance is the close of the
// month before it.
export function netWorthQueryArgs(p: ReportParams, today: Date = new Date()): NetWorthQueryArgs {
  const from = periodFrom(p, today)
  return { budgetId: p.budgetId, from: from ? addMonths(from, -1) : undefined }
}

export type MonthRange = [string, string]

const utc = (m: string) => new Date(Date.UTC(Number(m.slice(0, 4)), Number(m.slice(5, 7)) - 1, 15))

export interface PeriodLabels {
  /** "Oct 2026" */
  month: (m: string) => string
  /** a month range: "2025", "Q3 2026", "Oct 2026", "Jan – Oct 2026", "Nov 2025 – Feb 2026" */
  range: (range: MonthRange) => string
}

// Everything goes through Intl in the UI language, so word order and month
// names follow the language; only the quarter has no Intl form and comes from
// the catalogue.
export function periodLabels(lang: string, quarter: (q: number, year: string) => string): PeriodLabels {
  const monthYear = new Intl.DateTimeFormat(lang, { month: 'short', year: 'numeric', timeZone: 'UTC' })
  const yearOnly = new Intl.DateTimeFormat(lang, { year: 'numeric', timeZone: 'UTC' })
  const range = ([from, to]: MonthRange) => {
    if (from === to) return monthYear.format(utc(from))
    const fm = Number(from.slice(5, 7))
    const tm = Number(to.slice(5, 7))
    if (from.slice(0, 4) === to.slice(0, 4)) {
      if (fm === 1 && tm === 12) return yearOnly.format(utc(from))
      if ((fm - 1) % 3 === 0 && tm === fm + 2) return quarter(Math.floor((fm - 1) / 3) + 1, yearOnly.format(utc(from)))
    }
    return monthYear.formatRange(utc(from), utc(to))
  }
  return { month: (m) => monthYear.format(utc(m)), range }
}
