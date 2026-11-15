import type { DebtDetailsDto } from '@/api/dto/debt'
import { paidOffBounds, paidOffProblem, shiftDay } from './debtDates'
import { wireDebt } from './testDebt'

const snap = (id: string, asOf: string, isOrigin: 0 | 1) => ({ id, isOrigin, computedBalance: '', asOf, balance: '100', payment: '10', schedule: 'monthly' as const, lastPaymentDate: '2030-01-01' })

const details = (snapshots: DebtDetailsDto['snapshots']): DebtDetailsDto => ({ item: { ...wireDebt, startDate: '2026-10-01' }, snapshots, rateChanges: [], payments: [] })

it('shiftDay moves across month ends', () => {
  expect(shiftDay('2026-10-31', 1)).toBe('2026-11-01')
  expect(shiftDay('2026-03-01', -1)).toBe('2026-02-28')
})

describe('paid-off dates', () => {
  const today = '2026-11-14'

  it('opens the day after the start and closes at today', () => {
    const bounds = paidOffBounds(details([snap('s0', '2026-10-01', 1)]), today)
    expect(bounds).toMatchObject({ min: '2026-10-02', max: today, statement: '' })
  })

  it('starts at the latest statement, a day on it allowed', () => {
    const bounds = paidOffBounds(details([snap('s0', '2026-10-01', 1), snap('s1', '2026-11-03', 0), snap('s2', '2026-10-20', 0)]), today)
    expect(bounds.min).toBe('2026-11-03')
    expect(paidOffProblem('2026-11-03', bounds)).toBeNull()
    expect(paidOffProblem('2026-11-02', bounds)).toBe('before_statement')
  })

  it('names why a date is refused', () => {
    const bounds = paidOffBounds(details([snap('s0', '2026-10-01', 1)]), today)
    expect(paidOffProblem('2026-12-01', bounds)).toBe('future')
    expect(paidOffProblem('2026-10-01', bounds)).toBe('before_start')
    expect(paidOffProblem('2026-10-02', bounds)).toBeNull()
    expect(paidOffProblem('2026-11-14', bounds)).toBeNull()
  })

  it('rules nothing out before the debt has loaded', () => {
    expect(paidOffProblem('2026-10-01', paidOffBounds(undefined, today))).toBeNull()
  })
})
