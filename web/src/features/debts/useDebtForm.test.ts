import { act, renderHook } from '@testing-library/react'
import { buildCreatePayload, buildTermsPayload, initialCreateState, initialTermsState, projectTerms, useDebtTermsForm } from './useDebtForm'
import { wireDebt, wireDebtNotSetUp } from './testDebt'

const debt = { ...wireDebt, periodRate: '0.01', currentBalance: '900' }

// the projection starts after the caller's day: pin it before the stored next due
beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date(2026, 10, 8, 12))
})
afterEach(() => {
  vi.useRealTimers()
})

it('projects nothing under another schedule: the stored rate and next due belong to the old one', () => {
  const form = { ...initialTermsState(debt), schedule: 'weekly' as const, balance: '600' }
  expect(projectTerms(form, debt)).toEqual({ status: 'unknown' })
})

it('an amount edited after the schedule changed leaves the last payment date alone', () => {
  const { result } = renderHook(() => useDebtTermsForm(debt))
  act(() => result.current.patch({ schedule: 'weekly' }))
  act(() => result.current.setBalance('600'))
  expect(result.current.form.lastPaymentDate).toBe(debt.lastPaymentDate)
  expect(result.current.projection).toEqual({ status: 'unknown' })
})

it('projects with the debt currency precision', () => {
  const btc = { ...wireDebt, periodRate: '0', nextDueDate: '2026-12-15', lastPaymentDate: '2027-06-15' }
  const form = { ...initialTermsState(btc), balance: '0.004', payment: '0.001' }
  expect(projectTerms(form, btc, 8)).toEqual({ status: 'date', date: '2027-03-15' })
  const { result } = renderHook(() => useDebtTermsForm(btc, 8))
  act(() => result.current.setBalance('0.004'))
  act(() => result.current.setPayment('0.001'))
  expect(result.current.form.lastPaymentDate).toBe('2027-03-15')
})

// the server counts only the dues after the new snapshot, dated today
it('projects from the first due after today when the next due is overdue or today', () => {
  const overdue = { ...wireDebt, periodRate: '0', nextDueDate: '2026-11-05', lastPaymentDate: '2027-11-05' }
  const form = { ...initialTermsState(overdue), balance: '200', payment: '100' }
  expect(projectTerms(form, overdue, 2, '2026-11-08')).toEqual({ status: 'date', date: '2027-01-05' })
  const dueToday = { ...overdue, nextDueDate: '2026-11-08', lastPaymentDate: '2027-11-08' }
  expect(projectTerms({ ...form }, dueToday, 2, '2026-11-08')).toEqual({ status: 'date', date: '2027-01-08' })
  // a future next due is the first projected payment
  expect(projectTerms(form, overdue, 2, '2026-11-01')).toEqual({ status: 'date', date: '2026-12-05' })
})

it('a debt not set up starts with an empty balance and date, and projects nothing', () => {
  const { result } = renderHook(() => useDebtTermsForm(wireDebtNotSetUp))
  expect(result.current.form).toEqual({ balance: '', payment: '100', schedule: 'monthly', lastPaymentDate: '', startDate: '' })
  act(() => result.current.setBalance('600'))
  expect(result.current.form.lastPaymentDate).toBe('')
  expect(result.current.projection).toEqual({ status: 'unknown' })
})

it('setup without balance and date sends payment, repeats and start date only', () => {
  const initial = initialTermsState(wireDebtNotSetUp)
  const payload = buildTermsPayload({ ...initial, payment: '120', startDate: '2026-01-01' }, initial, wireDebtNotSetUp)
  expect(payload).toEqual({
    id: 'd1', name: 'Car loan', currencyId: 'cur-usd', categoryId: 'cat-food', payeeId: null, tagId: null, labelId: null,
    startDate: '2026-01-01', payment: '120', schedule: 'monthly',
  })
})

it('setup with balance and date sends both terms', () => {
  const initial = initialTermsState(wireDebtNotSetUp)
  const payload = buildTermsPayload({ ...initial, balance: '900', lastPaymentDate: '2027-08-05' }, initial, wireDebtNotSetUp)
  expect(payload).toMatchObject({ balance: '900', lastPaymentDate: '2027-08-05', payment: '100', schedule: 'monthly', startDate: null })
})

it('a new debt is created without terms and keeps its label', () => {
  const state = { ...initialCreateState({ categoryId: 'cat-food' }, [], 'cur-usd'), name: ' Car ', payment: '50', labelId: 'label1' }
  const payload = buildCreatePayload(state)
  expect(payload).toEqual({
    name: 'Car', currencyId: 'cur-usd', categoryId: 'cat-food', payeeId: null, tagId: null, labelId: 'label1',
    startDate: null, payment: '50', schedule: 'monthly',
  })
  expect(payload).not.toHaveProperty('balance')
  expect(payload).not.toHaveProperty('lastPaymentDate')
})
