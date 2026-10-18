import { relativeTime } from './relativeTime'

// tap times are wall-clock values, so "now" is built on the same local clock
const now = new Date(2026, 9, 12, 3, 0, 0)

it('picks the unit by distance', () => {
  expect(relativeTime('2026-10-12 02:59:30', 'en', now)).toBe('now')
  expect(relativeTime('2026-10-12 02:30:00', 'en', now)).toBe('30 minutes ago')
  expect(relativeTime('2026-10-11 22:00:00', 'en', now)).toBe('5 hours ago')
  expect(relativeTime('2026-10-11 03:00:00', 'en', now)).toBe('yesterday')
  expect(relativeTime('2026-10-02 03:00:00', 'en', now)).toBe('10 days ago')
})

it('reads a tap time as local wall clock, not UTC', () => {
  // a tap at this very minute on the reader's clock is "now", whatever the zone
  const d = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  const stamp = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  expect(relativeTime(stamp, 'en', d)).toBe('now')
})

it('falls back to a date beyond a month', () => {
  expect(relativeTime('2026-08-01 12:00:00', 'en', now)).toBe('Aug 1, 2026')
})
