import { describe, expect, it } from 'vitest'
import type { CreateTransactionDto } from '@/api/dto/transaction'
import {
  addChange,
  discard,
  markAttempted,
  nextSendable,
  resolveTempId,
  retryFailed,
  settleDone,
  settleRefused,
  targetOf,
} from './queue'
import type { OpChange, PendingOp } from './types'

const mint = { key: 'new-key', seq: 99 }

function payload(description: string, id = 'tx1'): CreateTransactionDto {
  return {
    id,
    type: 'expense',
    accountId: 'a1',
    accountRecipientId: null,
    amount: '10',
    amountRecipient: null,
    categoryId: null,
    description,
    payeeId: null,
    tagId: null,
    labelIds: [],
    date: '2026-11-08 12:00:00',
  }
}

const create = (description = 'first'): OpChange => ({ kind: 'tx-create', txId: 'tx1', payload: payload(description) })
const update = (description = 'second'): OpChange => ({ kind: 'tx-update', txId: 'tx1', payload: payload(description) })
const remove: OpChange = { kind: 'tx-delete', txId: 'tx1', previous: null }
const limit = (amount: string | null, period = '2026-11-01', elementId = 'e1', budgetId = 'b1'): OpChange => ({
  kind: 'limit',
  budgetId,
  elementId,
  period,
  amount,
})

function op(change: OpChange, base: Partial<Omit<PendingOp, keyof OpChange>> = {}): PendingOp {
  return { key: 'k1', seq: 1, attempted: false, status: 'waiting', retries: 0, notBefore: 0, ...change, ...base }
}

const failed = { status: 'failed', error: 'Refused', retries: 3, notBefore: 500 } as const

describe('targetOf', () => {
  it('is the transaction id for every transaction kind', () => {
    expect(targetOf(create())).toBe(targetOf(update()))
    expect(targetOf(update())).toBe(targetOf(remove))
  })

  it('tells limits apart by budget, element and period', () => {
    expect(targetOf(limit('1'))).toBe(targetOf(limit('2')))
    expect(targetOf(limit('1'))).not.toBe(targetOf(limit('1', '2026-12-01')))
    expect(targetOf(limit('1', '2026-11-01', 'e2'))).not.toBe(targetOf(limit('1')))
    expect(targetOf(limit('1', '2026-11-01', 'e1', 'b2'))).not.toBe(targetOf(limit('1')))
  })
})

describe('addChange', () => {
  it('adds a new op from the mint, waiting and unattempted', () => {
    expect(addChange([], create(), mint)).toEqual([op(create(), { key: 'new-key', seq: 99 })])
  })

  it('records that a request may already have reached the server', () => {
    const [added] = addChange([], create(), mint, true)
    expect(added.attempted).toBe(true)
  })

  it('folds an update into a waiting create, keeping its kind, txId and key', () => {
    expect(addChange([op(create())], update(), mint)).toEqual([op(create('second'))])
  })

  it('folds an update into a waiting update', () => {
    expect(addChange([op(update('first'))], update('second'), mint)).toEqual([op(update('second'))])
  })

  it('drops a waiting create when it is deleted, with nothing sent', () => {
    expect(addChange([op(create())], remove, mint)).toEqual([])
  })

  it('turns a waiting update into one delete', () => {
    expect(addChange([op(update())], remove, mint)).toEqual([op(remove)])
  })

  it('keeps only the last amount of a limit', () => {
    expect(addChange([op(limit('1'))], limit('2'), mint)).toEqual([op(limit('2'))])
  })

  it('keeps a limit of another period apart', () => {
    const ops = addChange([op(limit('1'))], limit('2', '2026-12-01'), mint)
    expect(ops).toEqual([op(limit('1')), op(limit('2', '2026-12-01'), { key: 'new-key', seq: 99 })])
  })

  it('keeps a change to another transaction apart', () => {
    const other: OpChange = { kind: 'tx-update', txId: 'tx2', payload: payload('x', 'tx2') }
    expect(addChange([op(create())], other, mint)).toHaveLength(2)
  })

  it('queues an update behind an attempted create, leaving it untouched', () => {
    const sent = op(create(), { attempted: true })
    expect(addChange([sent], update(), mint)).toEqual([sent, op(update(), { key: 'new-key', seq: 99 })])
  })

  it('queues a delete behind an attempted create', () => {
    const sent = op(create(), { attempted: true })
    expect(addChange([sent], remove, mint)).toEqual([sent, op(remove, { key: 'new-key', seq: 99 })])
  })

  it('folds into the change waiting behind an attempted one', () => {
    const sent = op(create(), { attempted: true })
    const behind = op(update('second'), { key: 'k2', seq: 2 })
    expect(addChange([sent, behind], update('third'), mint)).toEqual([sent, op(update('third'), { key: 'k2', seq: 2 })])
  })

  it('turns the change behind an attempted create into the delete', () => {
    const sent = op(create(), { attempted: true })
    const behind = op(update('second'), { key: 'k2', seq: 2 })
    expect(addChange([sent, behind], remove, mint)).toEqual([sent, op(remove, { key: 'k2', seq: 2 })])
  })

  it('replaces a failed update and makes it waiting again', () => {
    const [folded, ...rest] = addChange([op(update('first'), failed)], update('second'), mint)
    expect(rest).toEqual([])
    expect(folded).toEqual(op(update('second')))
    expect(folded).not.toHaveProperty('error')
  })

  it('drops a failed create when it is deleted', () => {
    expect(addChange([op(create(), failed)], remove, mint)).toEqual([])
  })

  it('never folds a change whose request may have reached the server', () => {
    const waiting = op(update('first'))
    expect(addChange([waiting], update('second'), mint, true)).toEqual([
      waiting,
      op(update('second'), { key: 'new-key', seq: 99, attempted: true }),
    ])
  })

  it('does not mutate its input', () => {
    const ops = [op(create())]
    addChange(ops, update(), mint)
    expect(ops).toEqual([op(create())])
  })
})

describe('settling', () => {
  it('markAttempted flags only that op', () => {
    const other = op(limit('1'), { key: 'k2', seq: 2 })
    expect(markAttempted([op(create()), other], 'k1')).toEqual([op(create(), { attempted: true }), other])
  })

  it('settleDone removes the op and leaves the one behind it', () => {
    const behind = op(update(), { key: 'k2', seq: 2 })
    expect(settleDone([op(create(), { attempted: true }), behind], 'k1')).toEqual([behind])
  })

  it('settleRefused marks failed with the reason and releases the key', () => {
    expect(settleRefused([op(update(), { attempted: true, retries: 2, notBefore: 9 })], 'k1', 'No')).toEqual([
      op(update(), { status: 'failed', error: 'No' }),
    ])
  })

  it('settleRefused folds the update behind an attempted create into it', () => {
    const ops = [op(create(), { attempted: true }), op(update('second'), { key: 'k2', seq: 2 })]
    expect(settleRefused(ops, 'k1', 'No')).toEqual([op(create('second'), { status: 'failed', error: 'No' })])
  })

  it('settleRefused removes an attempted create and the delete behind it', () => {
    const ops = [op(create(), { attempted: true }), op(remove, { key: 'k2', seq: 2 })]
    expect(settleRefused(ops, 'k1', 'No')).toEqual([])
  })

  it('settleRefused leaves other targets alone', () => {
    const other = op(limit('1'), { key: 'k2', seq: 2 })
    expect(settleRefused([op(update(), { attempted: true }), other], 'k1', 'No')).toEqual([
      op(update(), { status: 'failed', error: 'No' }),
      other,
    ])
  })

  it('retryFailed makes a failed op waiting with a clean slate', () => {
    const [retried] = retryFailed([op(update(), failed)], 'k1')
    expect(retried).toEqual(op(update()))
    expect(retried).not.toHaveProperty('error')
  })

  it('discard removes the op', () => {
    expect(discard([op(update(), failed)], 'k1')).toEqual([])
  })
})

describe('resolveTempId', () => {
  it('rewrites txId and payload.id of the ops behind the create', () => {
    const ops = [op(update(), { key: 'k2', seq: 2 })]
    const [resolved] = resolveTempId(ops, 'tx1', 'real')
    expect(resolved).toMatchObject({ txId: 'real', payload: { id: 'real' } })
  })

  it('rewrites the txId of a delete', () => {
    expect(resolveTempId([op(remove)], 'tx1', 'real')[0]).toMatchObject({ txId: 'real' })
  })

  it('leaves other ops alone', () => {
    const others = [op(limit('1')), op({ kind: 'tx-update', txId: 'tx2', payload: payload('x', 'tx2') }, { key: 'k2' })]
    expect(resolveTempId(others, 'tx1', 'real')).toEqual(others)
  })
})

describe('nextSendable', () => {
  const now = 1000
  const a = op(limit('1'), { key: 'a', seq: 1 })
  const b = op(limit('2', '2026-12-01'), { key: 'b', seq: 2 })

  it('returns null on an empty queue', () => {
    expect(nextSendable([], now)).toBeNull()
  })

  it('returns the oldest first, whatever the array order', () => {
    expect(nextSendable([b, a], now)).toBe(a)
  })

  it('skips a failed op', () => {
    expect(nextSendable([{ ...a, status: 'failed' }, b], now)).toBe(b)
  })

  it('skips an op backing off', () => {
    expect(nextSendable([{ ...a, notBefore: now + 1 }, b], now)).toBe(b)
  })

  it('sends an op once its backoff is over', () => {
    expect(nextSendable([{ ...a, notBefore: now }], now)).toEqual({ ...a, notBefore: now })
  })

  it('never returns an op with an earlier op on its target', () => {
    const backingOff = op(create(), { attempted: true, notBefore: now + 1 })
    const behind = op(update(), { key: 'k2', seq: 2 })
    expect(nextSendable([backingOff, behind], now)).toBeNull()
  })

  it('still sends other targets past a blocked one', () => {
    const backingOff = op(create(), { attempted: true, notBefore: now + 1 })
    const behind = op(update(), { key: 'k2', seq: 2 })
    const other = op(limit('1'), { key: 'k3', seq: 3 })
    expect(nextSendable([backingOff, behind, other], now)).toBe(other)
  })
})
