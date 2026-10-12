import { parseUtcDateTime, relativeTime } from './relativeTime'

const now = new Date('2026-10-12T03:00:00Z')

it('parses a stored datetime as UTC', () => {
  expect(parseUtcDateTime('2026-10-12 00:23:22').toISOString()).toBe('2026-10-12T00:23:22.000Z')
})

it('picks the unit by distance', () => {
  expect(relativeTime('2026-10-12 02:59:30', 'en', now)).toBe('now')
  expect(relativeTime('2026-10-12 02:30:00', 'en', now)).toBe('30 minutes ago')
  expect(relativeTime('2026-10-11 22:00:00', 'en', now)).toBe('5 hours ago')
  expect(relativeTime('2026-10-11 03:00:00', 'en', now)).toBe('yesterday')
  expect(relativeTime('2026-10-02 03:00:00', 'en', now)).toBe('10 days ago')
})

it('falls back to a date beyond a month', () => {
  expect(relativeTime('2026-08-01 12:00:00', 'en', now)).toBe('Aug 1, 2026')
})
