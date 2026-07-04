import type { Id } from '../types'
import type { UserDto } from './user'

export type BudgetRole = 'owner' | 'admin' | 'user' | 'guest'

export interface BudgetAccessDto {
  user: UserDto
  role: BudgetRole
  isAccepted: 0 | 1
}

// The list element shape (the full BudgetResult envelope arrives with the Budget page plan).
export interface BudgetMetaDto {
  id: Id
  ownerUserId: Id
  name: string
  startedAt: string
  currencyId: Id
  access: BudgetAccessDto[]
}
