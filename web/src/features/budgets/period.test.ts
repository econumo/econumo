import { describe, expect, it } from 'vitest'
import { currentPeriod, periodOf, periodStartDayOf, periodWindow } from './period'

const iso = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`

describe('periodStartDayOf', () => {
  it('reads the day, 1 when absent or out of range', () => {
    expect(periodStartDayOf({ periodStartDay: 28 })).toBe(28)
    expect(periodStartDayOf({})).toBe(1)
    expect(periodStartDayOf({ periodStartDay: 0 })).toBe(1)
    expect(periodStartDayOf({ periodStartDay: 29 })).toBe(1)
    expect(periodStartDayOf({ periodStartDay: 2.5 })).toBe(1)
  })
})

describe('periodWindow', () => {
  it.each([
    ['2026-10-01', 1, '2026-10-01', '2026-11-01'],
    ['2026-10-01', 5, '2026-10-05', '2026-11-05'],
    ['2026-10-01', 15, '2026-10-15', '2026-11-15'],
    ['2026-10-01', 16, '2026-09-16', '2026-10-16'],
    ['2026-11-01', 28, '2026-10-28', '2026-11-28'],
    ['2027-01-01', 28, '2026-12-28', '2027-01-28'],
    ['2028-03-01', 28, '2028-02-28', '2028-03-28'],
    ['2026-10-17', 5, '2026-10-05', '2026-11-05'],
    ['2026-10-01', 0, '2026-10-01', '2026-11-01'],
    ['2026-10-01', 29, '2026-10-01', '2026-11-01'],
  ])('%s at day %i covers %s to %s', (key, startDay, start, end) => {
    expect(periodWindow(key, startDay)).toEqual({ start, end })
  })
})

describe('periodOf', () => {
  it.each([
    ['2026-10-28', 28, '2026-11-01'],
    ['2026-10-27', 28, '2026-10-01'],
    ['2026-10-30', 28, '2026-11-01'],
    ['2026-10-04', 5, '2026-09-01'],
    ['2026-12-28', 28, '2027-01-01'],
    ['2028-02-29', 28, '2028-03-01'],
    ['2026-10-31', 1, '2026-10-01'],
    ['2026-10-30 12:00:00', 28, '2026-11-01'],
  ])('%s at day %i is in %s', (day, startDay, key) => {
    expect(periodOf(day, startDay)).toBe(key)
  })

  it('round-trips every day of 2026-2028 for every start day', () => {
    for (let sd = 1; sd <= 28; sd++) {
      for (let d = new Date(2026, 0, 1); d < new Date(2029, 0, 1); d = new Date(d.getFullYear(), d.getMonth(), d.getDate() + 1)) {
        const day = iso(d)
        const key = periodOf(day, sd)
        const { start, end } = periodWindow(key, sd)
        expect(day >= start && day < end, `${day} at ${sd}: ${key} ${start}..${end}`).toBe(true)
        expect(periodOf(start, sd)).toBe(key)
        const last = new Date(Number(end.slice(0, 4)), Number(end.slice(5, 7)) - 1, Number(end.slice(8, 10)) - 1)
        expect(periodOf(iso(last), sd)).toBe(key)
      }
    }
  })
})

describe('currentPeriod', () => {
  it('is the period holding the local date of now', () => {
    expect(currentPeriod(28, new Date(2026, 9, 30))).toBe('2026-11-01')
    expect(currentPeriod(1, new Date(2026, 9, 30))).toBe('2026-10-01')
    expect(currentPeriod(28, new Date(2026, 9, 27, 23, 59))).toBe('2026-10-01')
    expect(currentPeriod(5, new Date(2026, 9, 4))).toBe('2026-09-01')
  })
})
