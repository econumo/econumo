import { flowQueryArgs, periodLabels, periodRange, previousRange } from './reportPeriod'
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

describe('periodRange / previousRange', () => {
  it('a preset runs to the current month, its previous period has the same length', () => {
    const range = periodRange({ ...defaultParams(null), period: 'thisYear' }, today)!
    expect(range).toEqual(['2026-01-01', '2026-10-01'])
    expect(previousRange(range)).toEqual(['2025-03-01', '2025-12-01'])
  })

  it('all time has no range and so no previous period', () => {
    expect(periodRange({ ...defaultParams(null), period: 'all' }, today)).toBeNull()
  })

  it('custom ends at its own end month', () => {
    const p = { ...defaultParams(null), period: 'custom' as const, from: '2024-03-01', to: '2024-05-01' }
    expect(periodRange(p, today)).toEqual(['2024-03-01', '2024-05-01'])
    expect(previousRange(['2024-03-01', '2024-05-01'])).toEqual(['2023-12-01', '2024-02-01'])
  })
})

// Intl separates ranges with thin spaces in some languages
const plain = (s: string) => s.replace(/[\u2009\u202f]/g, ' ')

describe('periodLabels', () => {
  const en = periodLabels('en', (q, year) => `Q${q} ${year}`)

  it.each([
    [['2025-01-01', '2025-12-01'], '2025'],
    [['2026-07-01', '2026-09-01'], 'Q3 2026'],
    [['2026-10-01', '2026-10-01'], 'Oct 2026'],
    [['2026-01-01', '2026-10-01'], 'Jan – Oct 2026'],
    [['2025-11-01', '2026-02-01'], 'Nov 2025 – Feb 2026'],
  ] as const)('range %j → %s', (range, label) => {
    expect(plain(en.range([range[0], range[1]]))).toBe(label)
  })

  it('buckets and months follow the language', () => {
    expect(en.bucket('2026-10-01', 'quarter')).toBe('Q4 2026')
    expect(en.bucket('2026-10-01', 'year')).toBe('2026')
    expect(plain(en.monthsOfYear(1, 10))).toBe('Jan – Oct')
    const ru = periodLabels('ru', (q, year) => `${q} кв. ${year}`)
    expect(ru.month('2026-10-01')).toMatch(/^окт/)
    expect(ru.bucket('2026-10-01', 'quarter')).toBe('4 кв. 2026')
    const zh = periodLabels('zh', (q, year) => `${year}第${q}季度`)
    expect(zh.month('2026-10-01')).toBe('2026年10月')
  })
})
