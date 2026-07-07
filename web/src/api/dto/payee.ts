import type { Id } from '../types'

export interface PayeeDto {
  id: Id
  ownerUserId: Id
  name: string
  position: number
  isArchived: 0 | 1
  createdAt: string
  updatedAt: string
}
