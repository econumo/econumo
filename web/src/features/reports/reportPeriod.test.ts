import { netWorthQueryArgs, periodLabels } from './reportPeriod'
import { defaultParams } from './reportTabs'

const today = new Date(2026, 9, 15)

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

  it('months follow the language', () => {
    const ru = periodLabels('ru', (q, year) => `${q} кв. ${year}`)
    expect(ru.month('2026-10-01')).toMatch(/^окт/)
    expect(ru.range(['2026-10-01', '2026-12-01'])).toBe('4 кв. 2026')
    const zh = periodLabels('zh', (q, year) => `${year}第${q}季度`)
    expect(zh.month('2026-10-01')).toBe('2026年10月')
  })
})

describe('netWorthQueryArgs', () => {
  // month-end balances: the period starts from the close of the month before it
  it.each([
    ['last12', '2025-10-01'],
    ['last3y', '2023-12-01'],
    ['all', undefined],
  ] as const)('%s fetches from %s', (period, from) => {
    expect(netWorthQueryArgs({ ...defaultParams('b1'), period }, today)).toEqual({ budgetId: 'b1', from })
  })
})
