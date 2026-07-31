import { describe, expect, it } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import type { RecurringDto } from '@/api/dto/recurring'
import type { TransactionDto } from '@/api/dto/transaction'
import { buildRecurringPayload, initialRecurringFormState, nextFromAnchor, useRecurringForm } from './useRecurringForm'

const accounts = [{ id: 'a1', currency: { symbol: '$', fractionDigits: 2 } }] as never

describe('useRecurringForm', () => {
  it('create mode defaults: monthly, next payment = today, fresh id', () => {
    const s = initialRecurringFormState({}, accounts)
    expect(s.isNew).toBe(true)
    expect(s.schedule).toBe('monthly')
    expect(s.nextPaymentAt.length).toBe(19) // "YYYY-MM-DD HH:mm:ss"
    expect(s.id).toBeTruthy()
  })

  it('fromTransaction prefills fields, anchoring the next payment one interval after the transaction', () => {
    const tx = {
      id: 't1', type: 'expense', accountId: 'a1', accountRecipientId: null, amount: 42.5, amountRecipient: null,
      categoryId: 'c1', payeeId: null, tagId: null, description: 'rent', date: '2026-08-17 10:00:00',
    } as unknown as TransactionDto
    const s = initialRecurringFormState({ fromTransaction: tx }, accounts)
    expect(s.isNew).toBe(true)
    expect(s.amount).toBe('42.5')
    expect(s.categoryId).toBe('c1')
    expect(s.schedule).toBe('monthly')
    expect(s.nextPaymentAt).toBe('2026-09-17 00:00:00')
    expect(s.anchorDate).toBe(tx.date)
  })

  it('nextFromAnchor steps one interval, clamping month ends', () => {
    expect(nextFromAnchor('2026-08-17 10:00:00', 'weekly')).toBe('2026-08-24 00:00:00')
    expect(nextFromAnchor('2026-08-17 10:00:00', 'biweekly')).toBe('2026-08-31 00:00:00')
    expect(nextFromAnchor('2026-08-17 10:00:00', 'monthly')).toBe('2026-09-17 00:00:00')
    expect(nextFromAnchor('2026-08-17 10:00:00', 'quarterly')).toBe('2026-11-17 00:00:00')
    expect(nextFromAnchor('2026-08-17 10:00:00', 'yearly')).toBe('2027-08-17 00:00:00')
    // month-end clamping, mirroring the backend's scheduled-day behaviour
    expect(nextFromAnchor('2026-01-31 00:00:00', 'monthly')).toBe('2026-02-28 00:00:00')
    expect(nextFromAnchor('2028-02-29 00:00:00', 'yearly')).toBe('2029-02-28 00:00:00')
    // a year-end weekly step crosses into January
    expect(nextFromAnchor('2026-12-28 00:00:00', 'weekly')).toBe('2027-01-04 00:00:00')
  })

  it('changing the schedule re-derives the date from the anchor; a manual date stays put', () => {
    const tx = {
      id: 't1', type: 'expense', accountId: 'a1', accountRecipientId: null, amount: 42.5, amountRecipient: null,
      categoryId: 'c1', payeeId: null, tagId: null, description: 'rent', date: '2026-08-17 10:00:00',
    } as unknown as TransactionDto
    const { result } = renderHook(() => useRecurringForm({ fromTransaction: tx }, accounts))
    expect(result.current.form.nextPaymentAt).toBe('2026-09-17 00:00:00')

    // the user's example: monthly -> weekly moves Sep 17 back to Aug 24
    act(() => result.current.setSchedule('weekly'))
    expect(result.current.form.nextPaymentAt).toBe('2026-08-24 00:00:00')
    act(() => result.current.setSchedule('monthly'))
    expect(result.current.form.nextPaymentAt).toBe('2026-09-17 00:00:00')

    // a hand-picked date severs the anchor: later schedule changes keep it
    act(() => result.current.patch({ nextPaymentAt: '2026-12-01 00:00:00', anchorDate: null }))
    act(() => result.current.setSchedule('weekly'))
    expect(result.current.form.nextPaymentAt).toBe('2026-12-01 00:00:00')
  })

  it('from scratch there is no anchor: schedule changes leave the date alone', () => {
    const { result } = renderHook(() => useRecurringForm({}, accounts))
    const seeded = result.current.form.nextPaymentAt
    expect(result.current.form.anchorDate).toBeNull()
    act(() => result.current.setSchedule('yearly'))
    expect(result.current.form.nextPaymentAt).toBe(seeded)
  })

  it('edit mode seeds from the template and keeps its id', () => {
    const rt = {
      id: 'r1', ownerUserId: 'u1', type: 'expense', accountId: 'a1', accountRecipientId: null, amount: '50.5',
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
    expect(payload.amount).toBe('15')
    expect(payload.categoryId).toBeNull()
    expect(payload.accountRecipientId).toBe('a2')
  })
})
