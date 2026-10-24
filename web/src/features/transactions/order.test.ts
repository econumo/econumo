import { describe, expect, it } from 'vitest'
import { compareTransactions, sideKey } from './order'

type Row = Parameters<typeof compareTransactions>[0]

const row = (id: string, date: string, extra: Partial<Row> = {}): Row => ({
  id,
  date,
  accountId: 'a',
  accountRecipientId: null,
  sortKey: 'a1',
  recipientSortKey: null,
  ...extra,
})

const order = (rows: Row[], accountId?: string) => [...rows].sort((a, b) => compareTransactions(a, b, accountId)).map((r) => r.id)

describe('compareTransactions', () => {
  it('orders a day by key, not by time', () => {
    expect(order([row('evening', '2024-03-01 20:00:00', { sortKey: 'a1' }), row('morning', '2024-03-01 08:00:00', { sortKey: 'a2' })])).toEqual(['morning', 'evening'])
  })

  it('compares fractional keys as text', () => {
    expect(order([row('low', '2024-03-01 08:00:00', { sortKey: 'a1' }), row('mid', '2024-03-01 08:00:00', { sortKey: 'a1V' }), row('high', '2024-03-01 08:00:00', { sortKey: 'a2' })])).toEqual(['high', 'mid', 'low'])
  })

  it('puts a newer day first whatever the keys', () => {
    expect(order([row('old', '2024-03-01 23:00:00', { sortKey: 'a9' }), row('new', '2024-03-02 01:00:00', { sortKey: 'a1' })])).toEqual(['new', 'old'])
  })

  it('uses the recipient key on the recipient account', () => {
    const transfer = row('t', '2024-03-01 10:00:00', { accountId: 'b', accountRecipientId: 'a', sortKey: 'a9', recipientSortKey: 'a1' })
    const expense = row('e', '2024-03-01 09:00:00', { sortKey: 'a2' })
    expect(order([transfer, expense], 'a')).toEqual(['e', 't'])
    expect(order([transfer, expense], 'b')[0]).toBe('t')
    expect(sideKey(transfer, 'a')).toBe('a1')
    expect(sideKey(transfer, 'b')).toBe('a9')
    expect(sideKey(transfer)).toBe('a9')
  })

  it('falls back to the full datetime when a server sends no keys', () => {
    const early = row('early', '2024-03-01 08:00:00', { sortKey: undefined })
    const late = row('late', '2024-03-01 20:00:00', { sortKey: undefined })
    expect(order([early, late])).toEqual(['late', 'early'])
  })

  it('breaks ties by id', () => {
    expect(order([row('b', '2024-03-01 08:00:00'), row('a', '2024-03-01 08:00:00')])).toEqual(['a', 'b'])
  })
})
