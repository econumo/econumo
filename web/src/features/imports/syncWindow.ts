import type { ImportSourceDto } from '@/api/dto/imports'
import { formatDate, parseDateTime } from '@/lib/datetime'

// Default sync window: overlap the previous sync by a few days so pending
// rows that posted late are still picked up; a first sync looks back a month.
const OVERLAP_DAYS = 3
const FIRST_SYNC_DAYS = 30

export function syncStartDate(source: ImportSourceDto): string {
  const from = source.lastSyncedAt ? parseDateTime(source.lastSyncedAt) : new Date()
  from.setDate(from.getDate() - (source.lastSyncedAt ? OVERLAP_DAYS : FIRST_SYNC_DAYS))
  return formatDate(from)
}
