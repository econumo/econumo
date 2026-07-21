// Client-side view of the access pair on CurrentUserDto/ConnectionDto.
// Identifiers describe access, never money: 'full_access', not 'paid' — the
// same states exist on self-hosted instances where nobody pays anything.
export type AccessState = 'trial' | 'full_access' | 'readonly'

export function deriveAccessState(level: string, until: string): AccessState {
  if (level === 'readonly') {
    return 'readonly'
  }
  return until !== '' ? 'trial' : 'full_access'
}

// accessUntil is the frozen wire format "YYYY-MM-DD HH:mm:ss" in UTC —
// lib/datetime's parseDateTime reads local time, so parse here explicitly.
function parseUtc(s: string): number {
  const [datePart, timePart = '00:00:00'] = s.split(' ')
  const [y, m, d] = datePart.split('-').map(Number)
  const [hh, mm, ss] = timePart.split(':').map(Number)
  return Date.UTC(y, m - 1, d, hh, mm, ss)
}

export function accessDaysLeft(until: string, now: Date = new Date()): number {
  return Math.ceil((parseUtc(until) - now.getTime()) / 86_400_000)
}
