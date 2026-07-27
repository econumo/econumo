import type { Id } from '../types'
import type { UserDto } from './user'
import type { AccountRole } from './account'

// account-centric; the UI derives shared items from the accounts/budgets caches instead
export interface SharedAccountRefDto {
  id: Id
  ownerUserId: Id
  role: AccountRole
}

export interface ConnectionDto {
  user: UserDto
  accessLevel: 'full' | 'readonly'
  /** frozen wire format "YYYY-MM-DD HH:mm:ss" UTC; '' = no expiry */
  accessUntil: string
  sharedAccounts: SharedAccountRefDto[]
}

export interface InviteDto {
  code: string
  /** Y-m-d H:i:s; codes live 5 minutes */
  expiredAt: string
}
