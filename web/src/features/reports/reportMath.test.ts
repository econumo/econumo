import type { FlowReportDto, NetWorthReportDto, ReportRowDto } from '@/api/dto/report'
import {
  cashFlow,
  cashFlowInsight,
  changeRows,
  compareYears,
  netWorthInsight,
  netWorthSeries,
  rollUp,
  spendingInsight,
  topGroups,
} from './reportMath'

const row = (id: string, amounts: string[], over: Partial<ReportRowDto> = {}): ReportRowDto => ({
  id,
  kind: 'category',
  name: id.toUpperCase(),
  icon: '',
  isArchived: 0,
  ownerUserId: null,
  amounts,
  planned: [],
  children: [],
  ...over,
})

const months = (from: string, n: number) =>
  Array.from({ length: n }, (_, i) => {
    const d = new Date(Number(from.slice(0, 4)), Number(from.slice(5, 7)) - 1 + i, 1)
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-01`
  })

describe('rollUp', () => {
  it('month step is the identity, projecting only the current month', () => {
    const b = rollUp(['2026-09-01', '2026-10-01'], ['10', '20'], 'month', new Date(2026, 9, 16))
    expect(b.map((x) => x.total)).toEqual(['10', '20'])
    expect(b[0].partial).toBe(false)
    expect(b[0].projected).toBe('10')
    expect(b[1].partial).toBe(true)
    expect(b[1].projected).toBe('38.75')
  })

  it('groups quarters and flags a window-cut quarter', () => {
    const ms = months('2026-02-01', 5)
    const b = rollUp(ms, ['1', '2', '3', '4', '5'], 'quarter', new Date(2027, 0, 5))
    expect(b.map((x) => [x.start, x.months, x.total, x.partial])).toEqual([
      ['2026-01-01', 2, '3', true],
      ['2026-04-01', 3, '12', false],
    ])
    expect(b[0].projected).toBe('3')
  })

  it('a current bucket cut by the window start is not projected', () => {
    const b = rollUp(['2026-09-01'], ['30'], 'quarter', new Date(2026, 8, 10))
    expect(b[0]).toMatchObject({ partial: true, total: '30', projected: '30' })
  })

  it('a current quarter with its first month in the window is projected', () => {
    const b = rollUp(['2026-07-01', '2026-08-01', '2026-09-01'], ['10', '10', '5'], 'quarter', new Date(2026, 8, 15))
    expect(b[0].partial).toBe(true)
    expect(Number(b[0].projected)).toBeCloseTo((25 * 3 * 30) / (2 * 30 + 15), 6)
  })

  it('projects a partial current year: Jan-Oct 15 scales by 12 / 9.48...', () => {
    const ms = months('2026-01-01', 10)
    const b = rollUp(ms, ms.map(() => '100'), 'year', new Date(2026, 9, 15))
    expect(b).toHaveLength(1)
    expect(b[0].total).toBe('1000')
    expect(b[0].partial).toBe(true)
    const elapsed = 9 + 15 / 31
    expect(Number(b[0].projected)).toBeCloseTo((1000 * 12) / elapsed, 4)
  })

  it('a full past year is not partial', () => {
    const ms = months('2025-01-01', 12)
    const b = rollUp(ms, ms.map(() => '1'), 'year', new Date(2026, 9, 15))
    expect(b[0]).toMatchObject({ months: 12, total: '12', partial: false, projected: '12' })
  })

  it('empty input gives no buckets', () => {
    expect(rollUp([], [], 'year')).toEqual([])
  })
})

describe('topGroups', () => {
  it('takes the n largest by window total, the rest are other', () => {
    const rows = [row('a', ['1']), row('b', ['5']), row('c', ['3']), row('d', ['2'])]
    expect(topGroups(rows, 2)).toEqual({ ids: ['b', 'c'], otherIds: ['d', 'a'] })
  })
})

describe('compareYears', () => {
  it('cuts the earlier year to the current month and runs through it', () => {
    const ms = months('2025-01-01', 24)
    const amounts = ms.map((_, i) => String(i + 1))
    const r = compareYears(ms, amounts, 2025, 2026, new Date(2026, 9, 3))
    expect(r.throughMonth).toBe(10)
    expect(r.slots).toHaveLength(10)
    expect(r.slots[0]).toEqual({ month: 1, earlier: '1', later: '13' })
    expect(r.earlierTotal).toBe('55')
    expect(r.laterTotal).toBe(String(13 + 14 + 15 + 16 + 17 + 18 + 19 + 20 + 21 + 22))
  })

  it('runs all 12 months when later is a past year; null where there is no data', () => {
    const ms = months('2025-06-01', 7)
    const r = compareYears(ms, ms.map(() => '1'), 2024, 2025, new Date(2026, 9, 3))
    expect(r.throughMonth).toBe(12)
    expect(r.slots[0]).toEqual({ month: 1, earlier: null, later: null })
    expect(r.slots[5]).toEqual({ month: 6, earlier: null, later: '1' })
    expect(r.laterTotal).toBe('7')
    expect(r.earlierTotal).toBe('0')
  })
})

describe('changeRows', () => {
  const ms = months('2026-01-01', 4)
  it('sorts by |diff| and gives null pct when a is 0', () => {
    const rows = [
      row('up', ['10', '10', '30', '30']),
      row('new', ['0', '0', '5', '5']),
      row('down', ['50', '50', '10', '10']),
    ]
    const r = changeRows(rows, ms, ['2026-01-01', '2026-02-01'], ['2026-03-01', '2026-04-01'])
    expect(r.map((x) => x.id)).toEqual(['down', 'up', 'new'])
    expect(r[0]).toMatchObject({ a: '100', b: '20', diff: '-80', pct: -80, monthlyAvg: '10' })
    expect(r[1]).toMatchObject({ a: '20', b: '60', diff: '40', pct: 200, monthlyAvg: '30' })
    expect(r[2].pct).toBeNull()
  })
})

describe('cashFlow', () => {
  it('computes saved as income minus spent', () => {
    const report: FlowReportDto = {
      currencyId: 'c',
      months: ['2026-01-01', '2026-02-01'],
      expense: [row('a', ['10', '20']), row('b', ['5', '5'])],
      income: [row('i', ['100', '50'], { kind: 'income_category' })],
      transfersIn: ['1', '2'],
      transfersOut: ['3', '4'],
      savings: [],
    }
    expect(cashFlow(report)).toEqual([
      { month: '2026-01-01', income: '100', spent: '15', saved: '85', transfersIn: '1', transfersOut: '3' },
      { month: '2026-02-01', income: '50', spent: '25', saved: '25', transfersIn: '2', transfersOut: '4' },
    ])
  })
  it('empty report gives no rows', () => {
    expect(cashFlow({ currencyId: 'c', months: [], expense: [], income: [], transfersIn: [], transfersOut: [], savings: [] })).toEqual([])
  })
})

describe('netWorthSeries', () => {
  const acc = (id: string, currencyId: string, balances: string[]) => ({ id, name: id, icon: '', currencyId, ownerUserId: 'u', isDeleted: 0, balances })
  it('sums balances by group and in total', () => {
    const report: NetWorthReportDto = {
      currencyId: 'c',
      months: ['2026-01-01', '2026-02-01'],
      accounts: [acc('a', 'usd', ['10', '20']), acc('b', 'eur', ['1', '2']), acc('c', 'usd', ['5', '5'])],
    }
    const r = netWorthSeries(report, (a) => ({ id: a.currencyId, name: a.currencyId.toUpperCase() }))
    expect(r.groups).toEqual([
      { id: 'usd', name: 'USD', balances: ['15', '25'] },
      { id: 'eur', name: 'EUR', balances: ['1', '2'] },
    ])
    expect(r.total).toEqual(['16', '27'])
  })
  it('empty report gives empty arrays', () => {
    expect(netWorthSeries({ currencyId: 'c', months: [], accounts: [] }, () => ({ id: 'x', name: 'x' }))).toEqual({ groups: [], total: [] })
  })
})

describe('insights', () => {
  it('spending names the top group and its share', () => {
    const r = spendingInsight([row('a', ['30']), row('b', ['70'])])
    expect(r).toEqual({ key: 'reports.insight.spending_top', params: { name: 'B', amount: '70', share: '70' } })
  })
  it('spending with nothing spent is the empty insight', () => {
    expect(spendingInsight([]).key).toBe('reports.insight.empty')
  })
  it('cash flow reports saved or overspent', () => {
    const base = { transfersIn: '0', transfersOut: '0' }
    const up = cashFlowInsight([{ month: 'm', income: '100', spent: '60', saved: '40', ...base }])
    expect(up).toEqual({ key: 'reports.insight.cashflow_saved', params: { amount: '40', rate: '40' } })
    const down = cashFlowInsight([{ month: 'm', income: '10', spent: '60', saved: '-50', ...base }])
    expect(down).toEqual({ key: 'reports.insight.cashflow_overspent', params: { amount: '50' } })
    expect(cashFlowInsight([]).key).toBe('reports.insight.empty')
  })
  it('net worth reports the change over the window', () => {
    expect(netWorthInsight(['100', '150'])).toEqual({ key: 'reports.insight.networth_up', params: { amount: '50' } })
    expect(netWorthInsight(['100', '70'])).toEqual({ key: 'reports.insight.networth_down', params: { amount: '30' } })
    expect(netWorthInsight(['100', '100']).key).toBe('reports.insight.networth_flat')
    expect(netWorthInsight(['100']).key).toBe('reports.insight.empty')
  })
})
