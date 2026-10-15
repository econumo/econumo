// Import timestamps (a card's lastSeenAt) are UTC wall-clock datetimes, unlike
// transaction dates, so they must not go through the local-time parseDateTime.
export function parseUtcDateTime(s: string): Date {
  return new Date(`${s.replace(' ', 'T')}Z`)
}

export function relativeTime(s: string, lang: string, now: Date = new Date()): string {
  const then = parseUtcDateTime(s)
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
