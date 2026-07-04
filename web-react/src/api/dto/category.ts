import type { Id } from '../types'

export type CategoryType = 'expense' | 'income'

export interface CategoryDto {
  id: Id
  ownerUserId: Id
  name: string
  position: number
  type: CategoryType
  icon: string
  isArchived: 0 | 1
  createdAt: string
  updatedAt: string
}
