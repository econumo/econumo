import type { Id } from '../types'

export interface LabelDto {
  id: Id
  ownerUserId: Id
  name: string
  /** Material Symbols ligature, stored per row. Always rendered from this value
   *  rather than derived from the kind, so user-picked icons need no UI change. */
  icon: string
  position: number
  isArchived: 0 | 1
  createdAt: string
  updatedAt: string
}
