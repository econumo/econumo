import { describe, expect, it } from 'vitest'
import type { TransactionDto } from '@/api/dto/transaction'
import { advancePage, buildPagesFromBoot, isOlderThan, mergeTransactions } from './window'

const author = { id: 'u1', name: 'U', avatar: 'face:fuchsia' }
function tx(id: string, date: string, accountId: string, accountRecipientId: string | null = null): TransactionDto {
  return {
    id, date, accountId, accountRecipientId,
    type: accountRecipientId ? 'transfer' : 'expense',
    amount: 1, amountRecipient: accountRecipientId ? 1 : null,
    categoryId: null, description: '', payeeId: null, tagId: null,
    author,
  } as unknown as TransactionDto
}

describe('isOlderThan', () => {
  it('orders by date desc then id asc', () => {
    expect(isOlderThan({ date: '2026-06-01 10:00:00', id: 'b' }, { date: '2026-06-02 10:00:00', id: 'a' })).toBe(true)
    expect(isOlderThan({ date: '2026-06-02 10:00:00', id: 'b' }, { date: '2026-06-02 10:00:00', id: 'a' })).toBe(true) // same date, larger id = older position
    expect(isOlderThan({ date: '2026-06-02 10:00:00', id: 'a' }, { date: '2026-06-02 10:00:00', id: 'a' })).toBe(false)
  })
})

describe('buildPagesFromBoot', () => {
  it('sets the horizon at the Nth-newest row touching the account, ignoring stray transfers', () => {
    // Account B window (perAccountLimit=2): tx3 (Jun 5), tx4 (Jun 4).
    // tx1 is a transfer A->B from Jun 1 that arrived via A's window: it must
    // NOT widen B's horizon.
    const items = [
      tx('tx3', '2026-06-05 10:00:00', 'B'),
      tx('tx4', '2026-06-04 10:00:00', 'B'),
      tx('tx1', '2026-06-01 10:00:00', 'A', 'B'),
    ]
    const pages = buildPagesFromBoot(items, [
      { id: 'A', nextCursor: null, hasMore: false },
      { id: 'B', nextCursor: 'cursorB', hasMore: true },
    ], 2)
    expect(pages['A']).toEqual({ nextCursor: null, hasMore: false, oldestLoaded: null })
    expect(pages['B'].oldestLoaded).toEqual({ date: '2026-06-04 10:00:00', id: 'tx4' })
    expect(pages['B'].nextCursor).toBe('cursorB')
  })
})

describe('mergeTransactions', () => {
  it('dedupes by id, preferring the freshly fetched row', () => {
    const merged = mergeTransactions(
      [tx('a', '2026-06-05 10:00:00', 'A'), tx('b', '2026-06-04 10:00:00', 'A')],
      [tx('b', '2026-06-04 10:00:00', 'A'), tx('c', '2026-06-03 10:00:00', 'A')],
    )
    expect(merged.map((t) => t.id).sort()).toEqual(['a', 'b', 'c'])
  })
})

describe('advancePage', () => {
  const prev = { nextCursor: 'c1', hasMore: true, oldestLoaded: { date: '2026-06-04 10:00:00', id: 'b' } }
  it('advances the horizon to the last fetched row', () => {
    const next = advancePage(prev, { nextCursor: 'c2', hasMore: true }, [tx('c', '2026-06-03 10:00:00', 'A')])
    expect(next).toEqual({ nextCursor: 'c2', hasMore: true, oldestLoaded: { date: '2026-06-03 10:00:00', id: 'c' } })
  })
  it('keeps the previous horizon when the page is empty', () => {
    const next = advancePage(prev, { nextCursor: null, hasMore: false }, [])
    expect(next).toEqual({ nextCursor: null, hasMore: false, oldestLoaded: prev.oldestLoaded })
  })
})
