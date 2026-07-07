import type { Id } from '../types'

export interface FolderDto {
  id: Id
  name: string
  position: number
  isVisible: 0 | 1
}
