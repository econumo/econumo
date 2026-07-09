import type { Id } from '../types'

export interface TagDto {
  id: Id
  ownerUserId: Id
  name: string
  position: number
  isArchived: 0 | 1
  createdAt: string
  updatedAt: string
}
