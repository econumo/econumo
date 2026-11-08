import type { RecurringSchedule } from '@/api/dto/recurring'

const MAX_DUES = 1200
const DATE = /^(\d{4})-(\d{2})-(\d{2})/

function daysInMonth(year: number, month0: number): number {
  return new Date(Date.UTC(year, month0 + 1, 0)).getUTCDate()
}

function formatDate(t: Date): string {
  const y = String(t.getUTCFullYear()).padStart(4, '0')
  const m = String(t.getUTCMonth() + 1).padStart(2, '0')
  const d = String(t.getUTCDate()).padStart(2, '0')
  return `${y}-${m}-${d}`
}

// Same stepping as the server's NextOccurrence: weekly/biweekly add 7/14 days,
// month-based schedules keep the anchor day clamped to the target month's length.
function nextDue(current: Date, schedule: RecurringSchedule, anchor: number): Date {
  if (schedule === 'weekly' || schedule === 'biweekly') {
    return new Date(current.getTime() + (schedule === 'weekly' ? 7 : 14) * 86_400_000)
  }
  const months = schedule === 'quarterly' ? 3 : schedule === 'yearly' ? 12 : 1
  const first = new Date(Date.UTC(current.getUTCFullYear(), current.getUTCMonth() + months, 1))
  const day = Math.min(anchor, daysInMonth(first.getUTCFullYear(), first.getUTCMonth()))
  return new Date(Date.UTC(first.getUTCFullYear(), first.getUTCMonth(), day))
}

function parseDay(date: string): Date | null {
  const match = DATE.exec(date)
  if (!match) return null
  const d = new Date(Date.UTC(Number(match[1]), Number(match[2]) - 1, Number(match[3])))
  return Number.isNaN(d.getTime()) ? null : d
}

/**
 * The first due of the series after `today` (Y-m-d), starting from
 * `nextDueDate`: a snapshot saved today counts only the dues after it, so an
 * overdue due, or one due today, is not part of new terms. `anchorDay` is the
 * series' day of month, as for projectLastPaymentDate. null for unusable input.
 */
export function firstDueAfter(nextDueDate: string, schedule: RecurringSchedule, today: string, anchorDay?: number): string | null {
  let due = parseDay(nextDueDate)
  const day = parseDay(today)
  if (!due || !day) return null
  const anchor = anchorDay ?? due.getUTCDate()
  for (let i = 0; i < MAX_DUES * 5 && due.getTime() <= day.getTime(); i++) {
    due = nextDue(due, schedule, anchor)
  }
  return due.getTime() > day.getTime() ? formatDate(due) : null
}

/**
 * The date of the payment that clears the debt: from `balance`, each due adds
 * interest (balance * periodRate) and takes `payment`, until the balance is at
 * most half the currency's smallest unit (`fractionDigits`, default 2). null when the payment never covers the interest (or the
 * input is unusable). `anchorDay` is the day of month the series keeps
 * (31 -> Feb 28 -> Mar 31); it defaults to nextDue's own day, so pass it when
 * nextDue itself was clamped.
 */
export function projectLastPaymentDate(
  balance: number,
  payment: number,
  periodRate: number,
  schedule: RecurringSchedule,
  nextDueDate: string,
  anchorDay?: number,
  fractionDigits = 2,
): string | null {
  const epsilon = 0.5 * 10 ** -fractionDigits
  const match = DATE.exec(nextDueDate)
  if (!match || !Number.isFinite(balance) || !Number.isFinite(payment) || !Number.isFinite(periodRate)) return null
  if (balance < 0 || periodRate < 0) return null
  let due = new Date(Date.UTC(Number(match[1]), Number(match[2]) - 1, Number(match[3])))
  if (Number.isNaN(due.getTime())) return null
  if (balance <= epsilon) return formatDate(due)
  if (payment <= 0 || payment <= balance * periodRate) return null

  const anchor = anchorDay ?? due.getUTCDate()
  let remaining = balance
  for (let i = 0; i < MAX_DUES; i++) {
    remaining -= payment - remaining * periodRate
    if (remaining <= epsilon) return formatDate(due)
    due = nextDue(due, schedule, anchor)
  }
  return null
}
