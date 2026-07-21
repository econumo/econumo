import { accessDaysLeft, deriveAccessState, worstConnectionAttention } from './access'

describe('deriveAccessState', () => {
  // Frozen wire contract: accessUntil === '' means no expiry (never null).
  it.each([
    ['full', '', 'full_access'],
    ['full', '2026-09-01 00:00:00', 'trial'],
    ['readonly', '', 'readonly'],
    // The server collapses an elapsed expiry to readonly before the wire,
    // so a readonly level wins regardless of the date it arrives with.
    ['readonly', '2026-01-01 00:00:00', 'readonly'],
  ] as const)('level=%s until=%s -> %s', (level, until, want) => {
    expect(deriveAccessState(level, until)).toBe(want)
  })

  it('degrades a missing pair (stale persisted cache) to full_access', () => {
    expect(deriveAccessState(undefined as never, undefined as never)).toBe('full_access')
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

describe('worstConnectionAttention', () => {
  const now = new Date('2026-07-21T12:00:00Z')
  const conn = (name: string, accessLevel: 'full' | 'readonly', accessUntil: string) => ({
    user: { name },
    accessLevel,
    accessUntil,
  })

  it('returns null when every connection has full access', () => {
    expect(worstConnectionAttention([conn('A', 'full', '')], now)).toBeNull()
    expect(worstConnectionAttention([], now)).toBeNull()
  })

  it('ignores trials ending more than 3 days out', () => {
    expect(worstConnectionAttention([conn('A', 'full', '2026-08-30 00:00:00')], now)).toBeNull()
  })

  it('reports a trial ending within 3 days with the partner name and days left', () => {
    expect(worstConnectionAttention([conn('Megan', 'full', '2026-07-23 00:00:00')], now)).toEqual({
      state: 'trial',
      name: 'Megan',
      daysLeft: 2,
    })
  })

  it('readonly beats an ending trial', () => {
    const result = worstConnectionAttention(
      [conn('A', 'full', '2026-07-22 00:00:00'), conn('B', 'readonly', '')],
      now,
    )
    expect(result).toEqual({ state: 'readonly', name: 'B', daysLeft: null })
  })

  it('an elapsed accessUntil counts as readonly', () => {
    expect(worstConnectionAttention([conn('C', 'full', '2026-07-18 00:00:00')], now)).toEqual({
      state: 'readonly',
      name: 'C',
      daysLeft: null,
    })
  })

  it('among several ending trials the soonest expiry wins', () => {
    const result = worstConnectionAttention(
      [conn('A', 'full', '2026-07-23 00:00:00'), conn('B', 'full', '2026-07-22 00:00:00')],
      now,
    )
    expect(result).toEqual({ state: 'trial', name: 'B', daysLeft: 1 })
  })
})
