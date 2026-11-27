import type { Id } from '../types'

export const UserOptions = {
  CURRENCY: 'currency',
  CURRENCY_ID: 'currency_id',
  REPORT_PERIOD: 'report_period',
  BUDGET: 'budget',
  ONBOARDING: 'onboarding',
  ANALYTICS: 'analytics',
  AI: 'ai',
  LAST_SEEN_VERSION: 'last_seen_version',
} as const
export type UserOptions = (typeof UserOptions)[keyof typeof UserOptions]

export interface UserDto {
  id: Id
  avatar: string
  name: string
}

export interface UserOptionDto {
  name: UserOptions
  value: string | null
}

export interface CurrentUserDto {
  id: Id
  name: string
  email: string
  avatar: string
  options: UserOptionDto[]
  accessLevel: 'full' | 'readonly'
  /** frozen wire format "YYYY-MM-DD HH:mm:ss" UTC; '' = no expiry */
  accessUntil: string
  /** frozen wire format "YYYY-MM-DD HH:mm:ss" UTC */
  createdAt: string
  /** false for an account created via OAuth that never set a password */
  hasPassword: boolean
  /** months of inactivity before the account is deleted; 0 = this server never deletes */
  inactivityMonths: number
  /** what the user may choose, ascending; [] when the server never deletes. May not contain inactivityMonths */
  inactivityChoices: number[]
  /** @deprecated */
  currency: string
  /** @deprecated */
  reportPeriod: string
}

// The login-user response body itself — bare, not wrapped in the envelope.
export interface UserLoginItemDto {
  user: CurrentUserDto
  token: string
}

export interface CurrentUserResponseDto {
  data: { user: CurrentUserDto }
}

// --- Security: sessions + personal access tokens ---

export interface SessionDto {
  id: Id
  userAgent: string
  createdAt: string
  lastUsedAt: string
  isCurrent: boolean
  /** OAuth provider id the session was opened through; '' for a password session */
  provider: string
}

export interface PersonalTokenDto {
  id: Id
  name: string
  scope: 'full' | 'ingest'
  createdAt: string
  lastUsedAt: string
  expiresAt: string | null
}

// The create response — the ONLY place the raw token ever appears.
export interface CreatedPersonalTokenDto {
  id: Id
  name: string
  token: string
  createdAt: string
  expiresAt: string | null
}

export interface LogoutResultDto {
  result: string
  /** IdP end-session URL to navigate to; '' when the session logs out locally */
  logoutUrl: string
  /** OAuth provider id the session was opened through; '' for password sessions */
  provider: string
}

/** billing's view of the caller; dates "YYYY-MM-DD HH:mm:ss" UTC, '' = none */
export interface BillingAccessUserDto {
  /** paid access in effect now */
  paid: boolean
  /** a subscription that is still billing */
  renewing: boolean
  /** its next renewal; can be just behind us until the payment lands */
  renewsAt: string
  /** when paid access ends without renewing (a scheduled cancellation) */
  endsAt: string
  /** a renewal charge failed and is being retried */
  paymentFailed: boolean
  /** last discounted period of the subscription */
  discountUntil: string
}

export interface BillingAccessConnectionDto {
  id: Id
  paid: boolean
  renewing: boolean
}

/** available false (billing off or not answering): user null, connections empty */
export interface BillingAccessDto {
  available: boolean
  user: BillingAccessUserDto | null
  connections: BillingAccessConnectionDto[]
}
