import { describe, expect, it } from 'vitest'
import type { DailyListEntry, ViewTransaction } from '@/features/transactions/useAccountTransactions'
import { entryKey, resolveDrop } from './historyDrop'

const sep = (day: string): DailyListEntry => ({ kind: 'separator', day, label: 'date' })
const tx = (id: string, date: string, extra: Partial<ViewTransaction> = {}): DailyListEntry =>
  ({ kind: 'transaction', transaction: { id, date, isInFuture: false, ...extra } as ViewTransaction })

// day 2 (a, b, c) above day 1 (d, e)
const entries = [sep('2024-03-02'), tx('a', '2024-03-02 10:00:00'), tx('b', '2024-03-02 09:00:00'), tx('c', '2024-03-02 08:00:00'), sep('2024-03-01'), tx('d', '2024-03-01 10:00:00'), tx('e', '2024-03-01 09:00:00')]
const at = (key: string) => entries.findIndex((e) => entryKey(e) === key)

describe('resolveDrop', () => {
  it('reorders within a day', () => {
    expect(resolveDrop(entries, 'c', at('a'))).toEqual({ date: '2024-03-02', transactionIds: ['c', 'a', 'b'] })
  })

  it('moves down onto the next day header to the top of that day', () => {
    expect(resolveDrop(entries, 'b', at('sep-2024-03-01'))).toEqual({ date: '2024-03-01', transactionIds: ['b', 'd', 'e'] })
  })

  it('moves up onto a day header to the bottom of the day above', () => {
    expect(resolveDrop(entries, 'd', at('sep-2024-03-01'))).toEqual({ date: '2024-03-02', transactionIds: ['a', 'b', 'c', 'd'] })
  })

  it('moves into the middle of another day', () => {
    expect(resolveDrop(entries, 'a', at('d'))).toEqual({ date: '2024-03-01', transactionIds: ['d', 'a', 'e'] })
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
    expect(resolveDrop(withPreview, 'b', 1)).toEqual({ date: '2024-03-02', transactionIds: ['b', 'a'] })
  })
})
