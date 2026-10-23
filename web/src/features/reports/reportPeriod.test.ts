import { rollUp } from './reportMath'
import { comparisonRanges, flowQueryArgs, rangeLabel } from './reportPeriod'
import { defaultParams } from './reportTabs'

const today = new Date(2026, 9, 15)

describe('flowQueryArgs', () => {
  it.each([
    ['thisYear', '2026-01-01'],
    ['last12', '2025-11-01'],
    ['last3y', '2024-01-01'],
    ['all', undefined],
  ] as const)('%s starts at %s and never sends to', (period, from) => {
    expect(flowQueryArgs({ ...defaultParams(null), period }, today)).toEqual({ budgetId: null, from, groupBy: undefined })
  })

  it('custom sends both ends', () => {
    const p = { ...defaultParams('b1'), period: 'custom' as const, from: '2024-03-01', to: '2025-02-01' }
    expect(flowQueryArgs(p, today)).toEqual({ budgetId: 'b1', from: '2024-03-01', to: '2025-02-01', groupBy: undefined })
  })

  it('compare fetches from January of the earlier year', () => {
    const p = { ...defaultParams(null), mode: 'compare' as const, compare: [2022, 2026] as [number, number] }
    expect(flowQueryArgs(p, today).from).toBe('2022-01-01')
    expect(flowQueryArgs({ ...p, compare: null }, today).from).toBe('2025-01-01')
  })

  it('group-by is sent only in the my-accounts scope', () => {
    expect(flowQueryArgs({ ...defaultParams(null), groupBy: 'payee' }, today).groupBy).toBe('payee')
    expect(flowQueryArgs({ ...defaultParams('b1'), groupBy: 'payee' }, today).groupBy).toBeUndefined()
  })
})

const monthsFrom = (from: string, n: number) =>
  Array.from({ length: n }, (_, i) => {
    const d = new Date(Number(from.slice(0, 4)), Number(from.slice(5, 7)) - 1 + i, 1)
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-01`
  })

describe('comparisonRanges', () => {
  it('cuts the previous year to the months of the current one', () => {
    const ms = monthsFrom('2024-01-01', 34)
    const b = rollUp(ms, ms.map(() => '1'), 'year', today)
    expect(comparisonRanges(b, 'year', today)).toEqual({ a: ['2025-01-01', '2025-10-01'], b: ['2026-01-01', '2026-10-01'] })
  })

  it('compares two complete months when the latest is the unfinished current one', () => {
    const ms = monthsFrom('2026-01-01', 10)
    const b = rollUp(ms, ms.map(() => '1'), 'month', today)
    expect(comparisonRanges(b, 'month', today)).toEqual({ a: ['2026-08-01', '2026-08-01'], b: ['2026-09-01', '2026-09-01'] })
  })

  it('a quarter one month old falls back to the two quarters before', () => {
    const ms = monthsFrom('2026-01-01', 10)
    const b = rollUp(ms, ms.map(() => '1'), 'quarter', today)
    expect(comparisonRanges(b, 'quarter', today)).toEqual({ a: ['2026-04-01', '2026-06-01'], b: ['2026-07-01', '2026-09-01'] })
  })

  it('no comparison with a single bucket or a window-cut previous bucket', () => {
    const one = monthsFrom('2026-01-01', 10)
    expect(comparisonRanges(rollUp(one, one.map(() => '1'), 'year', today), 'year', today)).toBeNull()
    const cut = monthsFrom('2025-11-01', 12)
    expect(comparisonRanges(rollUp(cut, cut.map(() => '1'), 'year', today), 'year', today)).toBeNull()
  })
})

describe('rangeLabel', () => {
  it.each([
    [['2025-01-01', '2025-12-01'], '2025'],
    [['2026-07-01', '2026-09-01'], 'Q3 2026'],
    [['2026-10-01', '2026-10-01'], 'Oct 2026'],
    [['2026-01-01', '2026-10-01'], 'Jan–Oct 2026'],
    [['2025-11-01', '2026-02-01'], 'Nov 2025 – Feb 2026'],
  ] as const)('%j → %s', (range, label) => {
    expect(rangeLabel([range[0], range[1]], 'en')).toBe(label)
  })
})
