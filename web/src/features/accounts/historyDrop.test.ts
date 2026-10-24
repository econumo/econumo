import { describe, expect, it } from 'vitest'
import type { DailyListEntry, ViewTransaction } from '@/features/transactions/useAccountTransactions'
import { acceptsDrop, entryKey, resolveDrop } from './historyDrop'

const sep = (day: string): DailyListEntry => ({ kind: 'separator', day, label: 'date' })
const tx = (id: string, date: string, extra: Partial<ViewTransaction> = {}): DailyListEntry =>
  ({ kind: 'transaction', transaction: { id, date, isInFuture: false, ...extra } as ViewTransaction })

// day 2 (a, b, c) above day 1 (d, e)
const entries = [sep('2024-03-02'), tx('a', '2024-03-02 10:00:00'), tx('b', '2024-03-02 09:00:00'), tx('c', '2024-03-02 08:00:00'), sep('2024-03-01'), tx('d', '2024-03-01 10:00:00'), tx('e', '2024-03-01 09:00:00')]
const at = (key: string) => entries.findIndex((e) => entryKey(e) === key)

describe('resolveDrop', () => {
  it('reorders within a day', () => {
    expect(resolveDrop(entries, 'c', at('a'))).toMatchObject({ fromDate: '2024-03-02', date: '2024-03-02', aboveId: null })
  })

  it('moves down onto the next day header to the top of that day', () => {
    expect(resolveDrop(entries, 'b', at('sep-2024-03-01'))).toMatchObject({ fromDate: '2024-03-02', date: '2024-03-01', aboveId: null })
  })

  it('moves up onto a day header to the bottom of the day above', () => {
    expect(resolveDrop(entries, 'd', at('sep-2024-03-01'))).toMatchObject({ fromDate: '2024-03-01', date: '2024-03-02', aboveId: 'c' })
  })

  it('moves into the middle of another day', () => {
    expect(resolveDrop(entries, 'a', at('d'))).toMatchObject({ date: '2024-03-01', aboveId: 'd' })
  })

  it('returns the list as it looks after the drop', () => {
    const drop = resolveDrop(entries, 'c', at('a'))
    expect(drop?.entries.map(entryKey)).toEqual(['sep-2024-03-02', 'c', 'a', 'b', 'sep-2024-03-01', 'd', 'e'])
  })

  it('ignores a drop that changes nothing', () => {
    expect(resolveDrop(entries, 'b', at('b'))).toBeNull()
  })

  it('ignores a drop above the first day header', () => {
    expect(resolveDrop(entries, 'a', at('sep-2024-03-02'))).toBeNull()
  })

  it('leaves recurring previews out of the day order', () => {
    const recurring = { id: 'r1', nextPaymentAt: '2024-03-02 00:00:00' } as ViewTransaction['recurring']
    const withPreview = [sep('2024-03-02'), tx('r1', '2024-02-20 00:00:00', { recurring }), tx('a', '2024-03-02 10:00:00'), tx('b', '2024-03-02 09:00:00')]
    expect(resolveDrop(withPreview, 'b', 1)).toMatchObject({ date: '2024-03-02', aboveId: null })
  })
})

describe('acceptsDrop', () => {
  // above the first header there is no day, so it takes no drops: a row dragged
  // up there lands on the first row instead, at the top of the first day
  it('refuses the first header and accepts every other entry', () => {
    expect(acceptsDrop(entries, 'sep-2024-03-02')).toBe(false)
    expect(acceptsDrop(entries, 'sep-2024-03-01')).toBe(true)
    expect(acceptsDrop(entries, 'a')).toBe(true)
    expect(resolveDrop(entries, 'd', at('a'))).toMatchObject({ date: '2024-03-02', aboveId: null })
  })
})
