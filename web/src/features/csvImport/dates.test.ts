import { describe, expect, it } from 'vitest'
import { detectDateOrder, localeDateOrder, parseDateCell } from './dates'

describe('parseDateCell', () => {
  it('reads day-first and month-first by the given order', () => {
    expect(parseDateCell('03/04/2026', 'dmy')).toEqual({ date: '2026-04-03', time: null })
    expect(parseDateCell('03/04/2026', 'mdy')).toEqual({ date: '2026-03-04', time: null })
    expect(parseDateCell('3.4.2026', 'dmy')).toEqual({ date: '2026-04-03', time: null })
    expect(parseDateCell('03-04-2026', 'mdy')).toEqual({ date: '2026-03-04', time: null })
  })

  it('reads a leading 4-digit year regardless of order', () => {
    expect(parseDateCell('2026-03-04', 'dmy')).toEqual({ date: '2026-03-04', time: null })
    expect(parseDateCell('2026/03/04', 'mdy')).toEqual({ date: '2026-03-04', time: null })
    expect(parseDateCell('2026.03.04', 'ymd')).toEqual({ date: '2026-03-04', time: null })
  })

  it('reads time after a space or T', () => {
    expect(parseDateCell('2026-03-04 14:05:09', 'ymd')?.time).toBe('14:05:09')
    expect(parseDateCell('2026-03-04T14:05', 'ymd')?.time).toBe('14:05:00')
    expect(parseDateCell('04/03/2026 9:07', 'dmy')).toEqual({ date: '2026-03-04', time: '09:07:00' })
  })

  it('keeps the wall time of an ISO value with an offset', () => {
    expect(parseDateCell('2026-03-04T14:05:09+02:00', 'ymd')).toEqual({ date: '2026-03-04', time: '14:05:09' })
    expect(parseDateCell('2026-03-04T23:30:00Z', 'ymd')).toEqual({ date: '2026-03-04', time: '23:30:00' })
  })

  it('reads YYYYMMDD and unix seconds/millis in UTC', () => {
    expect(parseDateCell('20260304', 'dmy')).toEqual({ date: '2026-03-04', time: null })
    expect(parseDateCell('1772582400', 'dmy')).toEqual({ date: '2026-03-04', time: '00:00:00' })
    expect(parseDateCell('1772582400000', 'dmy')).toEqual({ date: '2026-03-04', time: '00:00:00' })
  })

  it('trims quotes and space', () => {
    expect(parseDateCell(' "2026-03-04" ', 'ymd')).toEqual({ date: '2026-03-04', time: null })
  })

  it('rejects invalid input', () => {
    expect(parseDateCell('31/02/2026', 'dmy')).toBeNull()
    expect(parseDateCell('2026-02-30', 'ymd')).toBeNull()
    expect(parseDateCell('2026-03-04 25:00', 'ymd')).toBeNull()
    expect(parseDateCell('20261340', 'ymd')).toBeNull()
    expect(parseDateCell('', 'ymd')).toBeNull()
    expect(parseDateCell('yesterday', 'ymd')).toBeNull()
    expect(parseDateCell('12345', 'ymd')).toBeNull()
  })
})

describe('detectDateOrder', () => {
  it('detects ymd from a leading year', () => {
    expect(detectDateOrder(['2026-03-04', '2026-04-05'], 'en-US')).toEqual({ order: 'ymd', ambiguous: false, example: null })
  })

  it('detects dmy and mdy from a part above 12', () => {
    expect(detectDateOrder(['03/04/2026', '15/04/2026'], 'en-US')).toEqual({ order: 'dmy', ambiguous: false, example: null })
    expect(detectDateOrder(['03/04/2026', '04/15/2026'], 'de-DE')).toEqual({ order: 'mdy', ambiguous: false, example: null })
  })

  it('falls back to the locale and flags ambiguity', () => {
    expect(detectDateOrder(['03/04/2026'], 'en-US')).toEqual({ order: 'mdy', ambiguous: true, example: '03/04/2026' })
    expect(detectDateOrder(['03/04/2026'], 'en-GB')).toEqual({ order: 'dmy', ambiguous: true, example: '03/04/2026' })
  })

  it('is not ambiguous without day/month shaped values', () => {
    expect(detectDateOrder(['20260304', '1772582400'], 'en-US').ambiguous).toBe(false)
    expect(detectDateOrder([], 'en-US')).toEqual({ order: 'mdy', ambiguous: false, example: null })
  })
})

describe('localeDateOrder', () => {
  it('is mdy only for en-US, en-PH and es-US', () => {
    expect(localeDateOrder('en-US')).toBe('mdy')
    expect(localeDateOrder('en-PH')).toBe('mdy')
    expect(localeDateOrder('es-US')).toBe('mdy')
    expect(localeDateOrder('en-GB')).toBe('dmy')
    expect(localeDateOrder('ru')).toBe('dmy')
  })
})
