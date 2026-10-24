import { arrayMove } from '@dnd-kit/sortable'
import type { Id } from '@/api/types'
import type { DailyListEntry } from '@/features/transactions/useAccountTransactions'

export function entryKey(entry: DailyListEntry): string {
  if (entry.kind === 'separator') return `sep-${entry.day}`
  const tx = entry.transaction
  return tx.recurring ? `${tx.id}@${tx.date}` : tx.id
}

export interface HistoryDrop {
  /** the day the row lands in, Y-m-d */
  date: string
  /** that day's real transactions afterwards, top first */
  transactionIds: Id[]
}

/** Where dropping `activeId` at flat index `overIndex` leaves it: the day group
 *  it lands in (the nearest header above) and that group's order. Recurring
 *  previews are not transactions yet and take no part. Null when the drop
 *  changes nothing or lands above the first header. */
export function resolveDrop(entries: DailyListEntry[], activeId: Id, overIndex: number): HistoryDrop | null {
  const from = entries.findIndex((e) => e.kind === 'transaction' && !e.transaction.recurring && e.transaction.id === activeId)
  if (from < 0 || overIndex < 0 || overIndex >= entries.length || from === overIndex) {
    return null
  }
  const moved = arrayMove(entries, from, overIndex)
  let start = overIndex
  while (start >= 0 && moved[start].kind !== 'separator') start--
  if (start < 0) {
    return null
  }
  const header = moved[start]
  if (header.kind !== 'separator') {
    return null
  }
  const transactionIds: Id[] = []
  for (let i = start + 1; i < moved.length && moved[i].kind !== 'separator'; i++) {
    const entry = moved[i]
    if (entry.kind === 'transaction' && !entry.transaction.recurring) {
      transactionIds.push(entry.transaction.id)
    }
  }
  return { date: header.day, transactionIds }
}
