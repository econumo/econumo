import { formatDateTime, parseDateTime, formatDate, dayKey, formatDayHeading, isToday, isYesterday, isFuture } from './datetime'

afterEach(() => vi.useRealTimers())

it('formats a Date as Y-m-d H:i:s with zero padding', () => {
  expect(formatDateTime(new Date(2026, 0, 5, 3, 7, 9))).toBe('2026-01-05 03:07:09')
})

it('round-trips through parseDateTime as local time', () => {
  const s = '2026-07-03 14:05:09'
  expect(formatDateTime(parseDateTime(s))).toBe(s)
})

it('parses a bare date with midnight time', () => {
  expect(formatDateTime(parseDateTime('2026-07-03'))).toBe('2026-07-03 00:00:00')
})

it('formats the date part and extracts day keys', () => {
  expect(formatDate(new Date(2026, 6, 3, 23, 59, 0))).toBe('2026-07-03')
  expect(dayKey('2026-07-03 14:05:09')).toBe('2026-07-03')
})

it('formats day headings with English ordinals', () => {
  expect(formatDayHeading('2026-07-01')).toBe('1st July 2026')
  expect(formatDayHeading('2026-07-02')).toBe('2nd July 2026')
  expect(formatDayHeading('2026-07-03')).toBe('3rd July 2026')
  expect(formatDayHeading('2026-07-04')).toBe('4th July 2026')
  expect(formatDayHeading('2026-07-11')).toBe('11th July 2026')
  expect(formatDayHeading('2026-07-12')).toBe('12th July 2026')
  expect(formatDayHeading('2026-07-13')).toBe('13th July 2026')
  expect(formatDayHeading('2026-07-21')).toBe('21st July 2026')
  expect(formatDayHeading('2026-07-22')).toBe('22nd July 2026')
  expect(formatDayHeading('2026-07-23')).toBe('23rd July 2026')
})

it('detects today and yesterday against the system clock', () => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date(2026, 6, 3, 12, 0, 0))
  expect(isToday('2026-07-03')).toBe(true)
  expect(isToday('2026-07-02')).toBe(false)
  expect(isYesterday('2026-07-02')).toBe(true)
  expect(isYesterday('2026-07-03')).toBe(false)
})

it('flags datetimes from tomorrow onward as future', () => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date(2026, 6, 3, 12, 0, 0))
  expect(isFuture('2026-07-03 23:59:59')).toBe(false)
  expect(isFuture('2026-07-04 00:00:00')).toBe(true)
  expect(isFuture('2026-07-05 09:00:00')).toBe(true)
})
