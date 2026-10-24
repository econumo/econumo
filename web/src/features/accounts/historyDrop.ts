import { arrayMove } from '@dnd-kit/sortable'
import type { Id } from '@/api/types'
import type { DailyListEntry } from '@/features/transactions/useAccountTransactions'

export function entryKey(entry: DailyListEntry): string {
  if (entry.kind === 'separator') return `sep-${entry.day}`
  const tx = entry.transaction
  return tx.recurring ? `${tx.id}@${tx.date}` : tx.id
}

/** Whether the entry `key` takes drops. Everything does except the first
 *  header: above it there is no day to land in, so a row dragged up there
 *  falls on the first row instead, at the top of the first day, where the
 *  gap the list opens and the drop agree. */
export function acceptsDrop(entries: DailyListEntry[], key: string): boolean {
  return entries.length === 0 || entryKey(entries[0]) !== key
}

export interface HistoryDrop {
  /** the day the row lands in, Y-m-d */
  date: string
  /** the real transaction now directly above it in that day; null = top of the day */
  aboveId: Id | null
  /** the whole list as it looks after the drop, shown until the server answers */
  entries: DailyListEntry[]
}

/** Where dropping `activeId` at flat index `overIndex` leaves it: the day group
 *  it lands in (the nearest header above) and the transaction right above it.
 *  Recurring previews are not transactions yet, so they are skipped. Null when
 *  the drop changes nothing or lands above the first header. */
export function resolveDrop(entries: DailyListEntry[], activeId: Id, overIndex: number): HistoryDrop | null {
  const from = entries.findIndex((e) => e.kind === 'transaction' && !e.transaction.recurring && e.transaction.id === activeId)
  if (from < 0 || overIndex < 0 || overIndex >= entries.length || from === overIndex) {
    return null
  }
  const moved = arrayMove(entries, from, overIndex)
  let aboveId: Id | null = null
  for (let i = overIndex - 1; i >= 0; i--) {
    const entry = moved[i]
    if (entry.kind === 'separator') {
      return { date: entry.day, aboveId, entries: moved }
    }
    if (aboveId === null && !entry.transaction.recurring) {
      aboveId = entry.transaction.id
    }
  }
  return null
}
