// Presentation helpers for the Security pages (sessions / personal tokens).

// parseUtcDateTime parses the API's "YYYY-MM-DD HH:mm:ss" datetimes, which are
// UTC (lib/datetime's parseDateTime treats them as LOCAL time — correct for
// user-entered transaction dates, wrong for server-side token timestamps).
export function parseUtcDateTime(s: string): Date {
  const [datePart, timePart = '00:00:00'] = s.split(' ')
  const [y, m, d] = datePart.split('-').map(Number)
  const [hh, mm, ss] = timePart.split(':').map(Number)
  return new Date(Date.UTC(y, m - 1, d, hh, mm, ss))
}

// relativeTime renders "just now" / "5 minutes ago" / "3 days ago" for the
// sessions list. Server touch-throttling makes sub-minute precision noise, so
// anything under a minute is "just now".
export function relativeTime(utcDatetime: string, now: Date = new Date()): string {
  const then = parseUtcDateTime(utcDatetime)
  const seconds = Math.max(0, Math.floor((now.getTime() - then.getTime()) / 1000))
  if (seconds < 60) {
    return 'just now'
  }
  const rtf = new Intl.RelativeTimeFormat('en', { numeric: 'always' })
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) {
    return rtf.format(-minutes, 'minute')
  }
  const hours = Math.floor(minutes / 60)
  if (hours < 24) {
    return rtf.format(-hours, 'hour')
  }
  return rtf.format(-Math.floor(hours / 24), 'day')
}

// describeUserAgent reduces a raw User-Agent to "Browser on OS" with a
// best-effort regex — no parsing dependency; unknown agents show truncated raw.
export function describeUserAgent(ua: string): string {
  if (!ua.trim()) {
    return ''
  }
  let browser = ''
  if (/edg(e|a|ios)?\//i.test(ua)) {
    browser = 'Edge'
  } else if (/opr\/|opera/i.test(ua)) {
    browser = 'Opera'
  } else if (/firefox|fxios/i.test(ua)) {
    browser = 'Firefox'
  } else if (/chrome|crios/i.test(ua)) {
    browser = 'Chrome'
  } else if (/safari/i.test(ua)) {
    browser = 'Safari'
  }
  let os = ''
  if (/windows/i.test(ua)) {
    os = 'Windows'
  } else if (/iphone|ipad|ios/i.test(ua)) {
    os = 'iOS'
  } else if (/android/i.test(ua)) {
    os = 'Android'
  } else if (/mac os|macintosh/i.test(ua)) {
    os = 'macOS'
  } else if (/linux/i.test(ua)) {
    os = 'Linux'
  }
  if (browser && os) {
    return `${browser} on ${os}`
  }
  if (browser || os) {
    return browser || os
  }
  return ua.length > 48 ? `${ua.slice(0, 48)}…` : ua
}
