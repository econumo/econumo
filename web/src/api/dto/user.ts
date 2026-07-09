import type { Id } from '../types'

export const UserOptions = {
  CURRENCY: 'currency',
  CURRENCY_ID: 'currency_id',
  REPORT_PERIOD: 'report_period',
  BUDGET: 'budget',
  ONBOARDING: 'onboarding',
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
