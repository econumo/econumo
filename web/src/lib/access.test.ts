import { accessDaysLeft, deriveAccessState } from './access'

describe('deriveAccessState', () => {
  // Frozen wire contract: accessUntil === '' means no expiry (never null).
  it.each([
    ['full', '', 'full_access'],
    ['full', '2026-09-01 00:00:00', 'trial'],
    ['readonly', '', 'readonly'],
    // The server collapses an elapsed expiry to readonly before the wire,
    // so a readonly level wins regardless of the date it arrives with.
    ['readonly', '2026-01-01 00:00:00', 'readonly'],
  ])('level=%s until=%s -> %s', (level, until, want) => {
    expect(deriveAccessState(level, until)).toBe(want)
  })
})

describe('accessDaysLeft', () => {
  const now = new Date(Date.UTC(2026, 6, 20, 10, 0, 0)) // 2026-07-20 10:00 UTC

  it('counts partial days as a full day (ceiling)', () => {
    expect(accessDaysLeft('2026-07-21 00:00:00', now)).toBe(1)
    expect(accessDaysLeft('2026-07-23 10:00:01', now)).toBe(4)
  })

  it('is exact on whole-day boundaries', () => {
    expect(accessDaysLeft('2026-07-23 10:00:00', now)).toBe(3)
  })

  it('crosses month boundaries', () => {
    expect(accessDaysLeft('2026-08-01 00:00:00', now)).toBe(12)
  })

  it('parses the timestamp as UTC, not local time', () => {
    // 14:00 UTC is 4h after a 10:00 UTC "now" regardless of the test
    // machine's zone: exactly 1 day when ceiled.
    expect(accessDaysLeft('2026-07-20 14:00:00', now)).toBe(1)
  })

  it('goes to zero and negative once the moment passes', () => {
    expect(accessDaysLeft('2026-07-20 10:00:00', now)).toBe(0)
    expect(accessDaysLeft('2026-07-19 09:00:00', now)).toBe(-1)
  })
})
