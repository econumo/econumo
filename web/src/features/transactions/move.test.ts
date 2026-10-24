import { describe, expect, it } from 'vitest'
import type { TransactionDto } from '@/api/dto/transaction'
import { applyLocalMove } from './move'

const row = (id: string, date: string, extra: Partial<TransactionDto> = {}) =>
  ({ id, date, accountId: 'a', accountRecipientId: null, sortKey: 1, recipientSortKey: null, ...extra }) as TransactionDto

describe('applyLocalMove', () => {
  it('renumbers the day top-down on the page account side', () => {
    const rows = [row('x', '2024-03-01 09:00:00', { sortKey: 2 }), row('y', '2024-03-01 10:00:00', { sortKey: 1 }), row('z', '2024-03-02 10:00:00', { sortKey: 1 })]
    const out = applyLocalMove(rows, { id: 'y', accountId: 'a', date: '2024-03-01', transactionIds: ['y', 'x'] })
    expect(out.find((r) => r.id === 'y')?.sortKey).toBe(2)
    expect(out.find((r) => r.id === 'x')?.sortKey).toBe(1)
    expect(out.find((r) => r.id === 'z')).toBe(rows[2])
  })

  it('changes only the date part of a row moved to another day', () => {
    const rows = [row('x', '2024-03-01 09:15:30'), row('d', '2024-03-05 07:00:00')]
    const out = applyLocalMove(rows, { id: 'x', accountId: 'a', date: '2024-03-05', transactionIds: ['d', 'x'] })
    const moved = out.find((r) => r.id === 'x')
    expect(moved?.date).toBe('2024-03-05 09:15:30')
    expect(moved?.sortKey).toBe(1)
    expect(out.find((r) => r.id === 'd')?.sortKey).toBe(2)
  })

  it('writes the recipient key when the page account receives the transfer', () => {
    const transfer = row('t', '2024-03-01 08:00:00', { accountId: 'b', accountRecipientId: 'a', sortKey: 7, recipientSortKey: 1 })
    const out = applyLocalMove([transfer, row('e', '2024-03-01 09:00:00', { sortKey: 2 })], { id: 't', accountId: 'a', date: '2024-03-01', transactionIds: ['t', 'e'] })
    const moved = out.find((r) => r.id === 't')
    expect(moved?.recipientSortKey).toBe(2)
    expect(moved?.sortKey).toBe(7)
  })
})
