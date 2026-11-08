import { describe, expect, it } from 'vitest'
import { projectLastPaymentDate } from './debtMath'

// Mirrors the server's implied-rate bisection so the reference loan is exact.
function impliedRate(balance: number, payment: number, n: number): number {
  let lo = 0
  let hi = 1
  for (let i = 0; i < 200; i++) {
    const mid = (lo + hi) / 2
    const pv = (payment * (1 - Math.pow(1 + mid, -n))) / mid
    if (pv > balance) lo = mid
    else hi = mid
  }
  return (lo + hi) / 2
}

describe('projectLastPaymentDate', () => {
  it('reference loan: 300000 at 2000 a month ends 240 dues after the first', () => {
    const r = Number(impliedRate(300000, 2000, 240).toFixed(10))
    expect(r).toBeCloseTo(0.0042676, 6)
    // 240 payments starting 2026-06-20: the last is 239 months later
    expect(projectLastPaymentDate(300000, 2000, r, 'monthly', '2026-06-20')).toBe('2046-05-20')
  })

  it('interest-free: ceil(balance / payment) dues', () => {
    expect(projectLastPaymentDate(1000, 300, 0, 'monthly', '2026-06-15')).toBe('2026-09-15')
    expect(projectLastPaymentDate(900, 300, 0, 'monthly', '2026-06-15')).toBe('2026-08-15')
  })

  it('returns null when the payment does not cover the interest', () => {
    expect(projectLastPaymentDate(100000, 400, 0.004, 'monthly', '2026-06-15')).toBeNull()
    expect(projectLastPaymentDate(100000, 500, 0.005, 'monthly', '2026-06-15')).toBeNull()
  })

  it('a zero balance is paid at the next due', () => {
    expect(projectLastPaymentDate(0, 100, 0.01, 'monthly', '2026-06-15')).toBe('2026-06-15')
  })

  it('clamps month-end dues to the month length and restores the day', () => {
    // 31st -> Feb 28 -> Mar 31 -> Apr 30
    expect(projectLastPaymentDate(500, 100, 0, 'monthly', '2027-01-31')).toBe('2027-05-31')
    expect(projectLastPaymentDate(200, 100, 0, 'monthly', '2027-01-31')).toBe('2027-02-28')
    expect(projectLastPaymentDate(300, 100, 0, 'monthly', '2027-01-31')).toBe('2027-03-31')
    expect(projectLastPaymentDate(400, 100, 0, 'monthly', '2027-01-31')).toBe('2027-04-30')
    // leap year
    expect(projectLastPaymentDate(200, 100, 0, 'monthly', '2028-01-31')).toBe('2028-02-29')
  })

  it('an explicit anchor day survives a clamped next due', () => {
    expect(projectLastPaymentDate(300, 100, 0, 'monthly', '2027-02-28', 31)).toBe('2027-04-30')
    expect(projectLastPaymentDate(200, 100, 0, 'monthly', '2027-02-28', 31)).toBe('2027-03-31')
  })

  it('steps weekly, biweekly, quarterly and yearly schedules', () => {
    expect(projectLastPaymentDate(300, 100, 0, 'weekly', '2026-06-01')).toBe('2026-06-15')
    expect(projectLastPaymentDate(300, 100, 0, 'biweekly', '2026-06-01')).toBe('2026-06-29')
    expect(projectLastPaymentDate(300, 100, 0, 'quarterly', '2026-01-31')).toBe('2026-07-31')
    expect(projectLastPaymentDate(200, 100, 0, 'quarterly', '2026-11-30')).toBe('2027-02-28')
    expect(projectLastPaymentDate(300, 100, 0, 'yearly', '2028-02-29')).toBe('2030-02-28')
  })

  it('stops at half the currency smallest unit: an 8-digit currency keeps paying below a cent', () => {
    expect(projectLastPaymentDate(0.004, 0.001, 0, 'monthly', '2026-06-15', undefined, 8)).toBe('2026-09-15')
    // the default is a 2-digit currency
    expect(projectLastPaymentDate(0.004, 0.001, 0, 'monthly', '2026-06-15')).toBe('2026-06-15')
    // a 0-digit currency is paid off once less than half a unit is left
    expect(projectLastPaymentDate(1000.4, 100, 0, 'monthly', '2026-06-15', undefined, 0)).toBe('2027-03-15')
  })

  it('returns null for unusable input', () => {
    expect(projectLastPaymentDate(100, 0, 0, 'monthly', '2026-06-15')).toBeNull()
    expect(projectLastPaymentDate(100, 10, 0, 'monthly', 'nope')).toBeNull()
    expect(projectLastPaymentDate(Number.NaN, 10, 0, 'monthly', '2026-06-15')).toBeNull()
  })
})
