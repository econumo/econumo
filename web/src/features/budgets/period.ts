import type { BudgetMetaDto } from '@/api/dto/budget'

// The one period rule of the SPA, mirroring the server's model.PeriodWindow /
// model.PeriodOf. A period is named by its month key "YYYY-MM-01": from day 2 to
// 15 after the month it starts in, from day 16 on after the month it ends in.
// Dates are built from local components, never parsed as UTC (see monthDate).

const MIN_DAY = 1
const MAX_DAY = 28

const validDay = (d: unknown): d is number => typeof d === 'number' && Number.isInteger(d) && d >= MIN_DAY && d <= MAX_DAY

const clampDay = (d: number): number => (validDay(d) ? d : MIN_DAY)

const isoDay = (d: Date): string =>
  `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`

const keyOf = (year: number, monthIndex: number): string => isoDay(new Date(year, monthIndex, 1))

/** a "YYYY-MM-DD" day (any time part ignored) as a local Date */
export function localDate(day: string): Date {
  const [y, m, d] = day.slice(0, 10).split('-').map(Number)
  return new Date(y, m - 1, d)
}

/** a server older than the period setting sends no day: calendar months */
export function periodStartDayOf(meta: Pick<BudgetMetaDto, 'periodStartDay'>): number {
  return validDay(meta.periodStartDay) ? meta.periodStartDay : MIN_DAY
}

/** the dates the period named by `key` covers, "YYYY-MM-DD", end exclusive */
export function periodWindow(key: string, startDay: number): { start: string; end: string } {
  const d = clampDay(startDay)
  const [y, m] = key.split('-').map(Number)
  const monthIndex = d >= 16 ? m - 2 : m - 1
  return { start: isoDay(new Date(y, monthIndex, d)), end: isoDay(new Date(y, monthIndex + 1, d)) }
}

/** the key of the period holding `day` ("YYYY-MM-DD", any time part ignored) */
export function periodOf(day: string, startDay: number): string {
  const d = clampDay(startDay)
  const shifted = localDate(day)
  shifted.setDate(shifted.getDate() - (d - 1))
  return keyOf(shifted.getFullYear(), shifted.getMonth() + (d >= 16 ? 1 : 0))
}

/** the period holding the local date of `now` */
export function currentPeriod(startDay: number, now: Date = new Date()): string {
  return periodOf(isoDay(now), startDay)
}

/** the dates a shifted period covers, last day inclusive ("Oct 28 – Nov 27" in
 *  `lang`); null for a calendar month, which needs no range */
export function periodDatesLabel(key: string, startDay: number, lang: string): string | null {
  if (clampDay(startDay) === MIN_DAY) {
    return null
  }
  const { start, end } = periodWindow(key, startDay)
  const last = localDate(end)
  last.setDate(last.getDate() - 1)
  const fmt = new Intl.DateTimeFormat(lang, { day: 'numeric', month: 'short' })
  return `${fmt.format(localDate(start))} – ${fmt.format(last)}`
}
