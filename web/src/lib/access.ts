// Client-side view of the access pair on CurrentUserDto/ConnectionDto.
// Identifiers describe access, never money: 'full_access', not 'paid' — the
// same states exist on self-hosted instances where nobody pays anything.
export type AccessState = 'trial' | 'full_access' | 'readonly'

export function deriveAccessState(level: 'full' | 'readonly', until: string): AccessState {
  if (level === 'readonly') {
    return 'readonly'
  }
  return until ? 'trial' : 'full_access'
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

// The one connection the SubscriptionBanner should warn about, if any:
// read-only beats ending-soon; among ending trials the soonest expiry wins.
export interface ConnectionAttention {
  state: 'readonly' | 'trial'
  name: string
  daysLeft: number | null
}

export function worstConnectionAttention(
  connections: readonly { user: { name: string }; accessLevel: 'full' | 'readonly'; accessUntil: string }[],
  now: Date = new Date(),
): ConnectionAttention | null {
  let trial: ConnectionAttention | null = null
  for (const c of connections) {
    const state = deriveAccessState(c.accessLevel, c.accessUntil)
    if (state === 'trial') {
      const daysLeft = accessDaysLeft(c.accessUntil, now)
      if (daysLeft <= 0) {
        // Elapsed accessUntil: effectively read-only already.
        return { state: 'readonly', name: c.user.name, daysLeft: null }
      }
      if (daysLeft <= 3 && (trial === null || daysLeft < (trial.daysLeft ?? Infinity))) {
        trial = { state: 'trial', name: c.user.name, daysLeft }
      }
    } else if (state === 'readonly') {
      return { state: 'readonly', name: c.user.name, daysLeft: null }
    }
  }
  return trial
}
