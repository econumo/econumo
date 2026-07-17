import { describe, expect, it } from 'vitest'
import type { RecurringDto } from '@/api/dto/recurring'
import type { TransactionDto } from '@/api/dto/transaction'
import { buildRecurringPayload, initialRecurringFormState } from './useRecurringForm'

const accounts = [{ id: 'a1', currency: { symbol: '$', fractionDigits: 2 } }] as never

describe('useRecurringForm', () => {
  it('create mode defaults: monthly, next payment = today, fresh id', () => {
    const s = initialRecurringFormState({}, accounts)
    expect(s.isNew).toBe(true)
    expect(s.schedule).toBe('monthly')
    expect(s.nextPaymentAt.length).toBe(19) // "YYYY-MM-DD HH:mm:ss"
    expect(s.id).toBeTruthy()
  })

  it('fromTransaction prefills fields but not the date', () => {
    const tx = {
      id: 't1', type: 'expense', accountId: 'a1', accountRecipientId: null, amount: 42.5, amountRecipient: null,
      categoryId: 'c1', payeeId: null, tagId: null, description: 'rent', date: '2026-06-01 10:00:00',
    } as unknown as TransactionDto
    const s = initialRecurringFormState({ fromTransaction: tx }, accounts)
    expect(s.isNew).toBe(true)
    expect(s.amount).toBe('42.5')
    expect(s.categoryId).toBe('c1')
    expect(s.nextPaymentAt).not.toBe(tx.date)
  })

  it('edit mode seeds from the template and keeps its id', () => {
    const rt = {
      id: 'r1', ownerUserId: 'u1', type: 'expense', accountId: 'a1', accountRecipientId: null, amount: 50.5,
      categoryId: 'c1', payeeId: null, tagId: null, description: 'rent', schedule: 'weekly', nextPaymentAt: '2026-08-31 00:00:00',
    } as RecurringDto
    const s = initialRecurringFormState({ recurring: rt }, accounts)
    expect(s.isNew).toBe(false)
    expect(s.id).toBe('r1')
    expect(s.schedule).toBe('weekly')
    expect(s.nextPaymentAt).toBe('2026-08-31 00:00:00')
  })

  it('buildRecurringPayload evaluates the amount and nulls classifier ids for transfers', () => {
    const s = initialRecurringFormState({}, accounts)
    const payload = buildRecurringPayload({ ...s, type: 'transfer', accountId: 'a1', accountRecipientId: 'a2', amount: '10+5', categoryId: 'c1' })
    expect(payload.amount).toBe(15)
    expect(payload.categoryId).toBeNull()
    expect(payload.accountRecipientId).toBe('a2')
  })
})
