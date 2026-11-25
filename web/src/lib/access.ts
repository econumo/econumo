// Client-side view of the access pair on CurrentUserDto/ConnectionDto.
// Identifiers describe access, never money: 'full_access', not 'paid' — the
// same states exist on self-hosted instances where nobody pays anything.
import type { BillingAccessUserDto } from '@/api/dto/user'

export type AccessState = 'trial' | 'full_access' | 'readonly'

export function deriveAccessState(level: 'full' | 'readonly', until: string): AccessState {
  if (level === 'readonly') {
    return 'readonly'
  }
  return until ? 'trial' : 'full_access'
}

// accessUntil (and billing's dates) are the frozen wire format "YYYY-MM-DD HH:mm:ss" in UTC —
// lib/datetime's parseDateTime reads local time, so parse here explicitly.
function parseUtc(s: string): number {
  const [datePart, timePart = '00:00:00'] = s.split(' ')
  const [y, m, d] = datePart.split('-').map(Number)
  const [hh, mm, ss] = timePart.split(':').map(Number)
  return Date.UTC(y, m - 1, d, hh, mm, ss)
}

// billing's view (get-billing-access) of a subscription that is still
// billing: its accessUntil (the paid period plus a grace) moves on by itself,
// so nothing about it is ending. Billing says so with renewing; a failed
// renewal charge brings the dated warnings back.
export function isRenewing(billing: Pick<BillingAccessUserDto, 'renewing' | 'paymentFailed'> | null | undefined): boolean {
  return billing?.renewing === true && !billing.paymentFailed
}

// An access date (accessUntil, or billing's renewsAt/endsAt) as the viewer's
// local calendar day, written the way the payment portal writes it
// ("November 25, 2026", ru "25 ноября 2026 г."), so both name the same day:
// the wire value is UTC, and its date part can be the next day locally.
export function formatAccessDate(s: string, locale: string): string {
  if (!s) return ''
  return new Date(parseUtc(s)).toLocaleDateString(locale, { day: 'numeric', month: 'long', year: 'numeric' })
}

// Whether a frozen-layout UTC date is still ahead.
export function isFutureUtc(s: string, now: Date = new Date()): boolean {
  return s !== '' && parseUtc(s) > now.getTime()
}

export function accessDaysLeft(until: string, now: Date = new Date()): number {
  return Math.ceil((parseUtc(until) - now.getTime()) / 86_400_000)
}

// The one connection the SubscriptionBanner should warn about, if any:
// read-only beats ending-soon; among ending trials the soonest expiry wins.
// Connections billing reports as renewing are never ending.
export interface ConnectionAttention {
  state: 'readonly' | 'trial'
  name: string
  daysLeft: number | null
}

export function worstConnectionAttention(
  connections: readonly { user: { id: string; name: string }; accessLevel: 'full' | 'readonly'; accessUntil: string }[],
  now: Date = new Date(),
  renewingIds: ReadonlySet<string> = new Set(),
): ConnectionAttention | null {
  let trial: ConnectionAttention | null = null
  for (const c of connections) {
    const state = deriveAccessState(c.accessLevel, c.accessUntil)
    if (state === 'trial' && !renewingIds.has(c.user.id)) {
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
