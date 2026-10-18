import { parseDateTime } from '@/lib/datetime'

// A tap's time (externalPostedAt, and a card's lastSeenAt drawn from it) is
// the phone's own wall clock as the Shortcut reported it, stored as is like a
// transaction date, so it is read as local time: converting it from UTC would
// shift every tap by the reader's offset.
export function relativeTime(s: string, lang: string, now: Date = new Date()): string {
  const then = parseDateTime(s)
  const seconds = (then.getTime() - now.getTime()) / 1000
  const abs = Math.abs(seconds)
  const rtf = new Intl.RelativeTimeFormat(lang, { numeric: 'auto' })
  if (abs < 60) {
    return rtf.format(0, 'second')
  }
  if (abs < 3600) {
    return rtf.format(Math.round(seconds / 60), 'minute')
  }
  if (abs < 86400) {
    return rtf.format(Math.round(seconds / 3600), 'hour')
  }
  if (abs < 30 * 86400) {
    return rtf.format(Math.round(seconds / 86400), 'day')
  }
  return new Intl.DateTimeFormat(lang, { day: 'numeric', month: 'short', year: 'numeric' }).format(then)
}
