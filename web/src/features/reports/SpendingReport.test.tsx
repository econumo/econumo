import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import type { FlowReportDto, ReportRowDto } from '@/api/dto/report'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { METRICS, trackEvent } from '@/lib/metrics'
import { defaultParams, useReportTabs } from './reportTabs'
import type { ReportParams } from './reportTabs'
import { renderReports, setViewport } from './reportsTestUtils'

vi.mock('recharts', async (importOriginal) =>
  (await import('./rechartsTestMock')).fixedSizeRecharts(await importOriginal<object>()))

vi.mock('@/lib/metrics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/metrics')>()),
  trackEvent: vi.fn(),
}))

// Jan 2025 .. Oct 2026; "today" is 2026-10-15
const months = Array.from({ length: 22 }, (_, i) => {
  const d = new Date(2025, i, 1)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-01`
})
const flat = (v: string) => months.map(() => v)

const row = (id: string, name: string, amounts: string[], over: Partial<ReportRowDto> = {}): ReportRowDto => ({
  id, kind: 'category', name, icon: '', isArchived: 0, ownerUserId: null, amounts, planned: [], children: [], ...over,
})

const expense = [
  // 2025: 100 a month; 2026: 150 a month -> Jan-Oct 1000 vs 1500
  row('cat-food', 'Groceries', months.map((m) => (m < '2026' ? '100' : '150'))),
  row('cat-rent', 'Rent', flat('90')),
  row('cat-car', 'Transport', flat('40')),
  row('cat-fun', 'Fun', flat('30')),
  row('cat-gift', 'Gifts', flat('20')),
  row('cat-pet', 'Pets', flat('10')),
  row('cat-misc', 'Misc', flat('5')),
]

const report = (over: Partial<FlowReportDto> = {}): FlowReportDto => ({
  currencyId: 'cur-usd', months, expense, income: [], transfersIn: flat('0'), transfersOut: flat('0'), savings: [], ...over,
})

let lastQuery: URLSearchParams | null = null
function flowHandler(data: FlowReportDto = report()) {
  return http.get('*/api/v1/report/get-flow-report', ({ request }) => {
    lastQuery = new URL(request.url).searchParams
    return HttpResponse.json({ success: true, message: '', data: { item: data } })
  })
}

function openSpending(patch: Partial<ReportParams> = {}, data?: FlowReportDto) {
  useReportTabs.setState({
    userId: 'u1',
    tabs: [{ id: 't1', kind: 'spending', title: null, params: { ...defaultParams(null), ...patch } }],
    activeId: 't1',
  })
  server.use(
    ...coreHandlers(),
    flowHandler(data),
    http.get('*/api/v1/oauth/get-identity-list', () => HttpResponse.json({ success: true, message: '', data: [] })),
  )
  return renderReports()
}

const params = () => useReportTabs.getState().tabs[0].params
const legendNames = () => within(screen.getByRole('list', { name: 'Legend' })).getAllByRole('listitem').map((li) => li.textContent)

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date(2026, 9, 15, 12))
  localStorage.clear()
  lastQuery = null
  vi.mocked(trackEvent).mockClear()
  setViewport(false)
})

afterEach(() => {
  vi.useRealTimers()
})

it('stacks the top five rows and everything else', async () => {
  openSpending()
  await screen.findByRole('list', { name: 'Legend' })
  expect(legendNames()).toEqual(['Groceries', 'Rent', 'Transport', 'Fun', 'Gifts', 'Everything else'])
  await waitFor(() => expect(document.body.querySelectorAll('.recharts-bar').length).toBeGreaterThanOrEqual(6))
  expect(lastQuery?.get('from')).toBe('2024-01-01')
  expect(lastQuery?.has('to')).toBe(false)
})

it('headline compares this year so far with the same months last year', async () => {
  openSpending()
  expect(await screen.findByText('Spent in Jan–Oct 2026')).toBeInTheDocument()
  // (150+90+40+30+20+10+5) * 10
  expect(screen.getByText('3,450 $')).toBeInTheDocument()
  expect(screen.getByText(/\+500 \$ · \+17%/)).toBeInTheDocument()
  expect(screen.getByText('vs Jan–Oct 2025', { exact: false })).toBeInTheDocument()
})

it('the current year is projected and footnoted', async () => {
  openSpending()
  expect(await screen.findByText(/2026 is not over yet/)).toBeInTheDocument()
  expect(screen.getByText(/On pace for/)).toBeInTheDocument()
})

it('clicking a table row narrows the chart to it and stores the selection', async () => {
  openSpending()
  const user = userEvent.setup()
  const table = await screen.findByRole('table')
  await user.click(within(table).getByRole('button', { name: 'Groceries' }))
  expect(params().selectedId).toBe('cat-food')
  expect(legendNames()).toEqual(['Groceries'])
  expect(trackEvent).toHaveBeenCalledWith(METRICS.REPORTS_SELECT_ROW, { selected: true })
  expect(await screen.findByRole('tab', { name: 'Groceries' })).toBeInTheDocument()
  await user.click(within(table).getByRole('button', { name: 'Groceries' }))
  expect(params().selectedId).toBeNull()
})

it('a selected row that is not in the report is ignored', async () => {
  openSpending({ selectedId: 'gone' })
  await screen.findByRole('list', { name: 'Legend' })
  expect(legendNames()).toHaveLength(6)
})

it('compare years: like-for-like Jan–Oct totals', async () => {
  openSpending({ mode: 'compare', compare: [2025, 2026], selectedId: 'cat-food' })
  expect(await screen.findByText('Spent in Jan–Oct 2026')).toBeInTheDocument()
  expect(screen.getAllByText('1,500 $')[0]).toBeInTheDocument()
  expect(screen.getAllByText(/\+500 \$ · \+50%/)[0]).toBeInTheDocument()
  expect(screen.getByText(/vs Jan–Oct 2025/)).toBeInTheDocument()
  expect(lastQuery?.get('from')).toBe('2025-01-01')
})

it('switching mode fires its metric', async () => {
  openSpending()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('radio', { name: 'Compare years' }))
  expect(params().mode).toBe('compare')
  expect(trackEvent).toHaveBeenCalledWith(METRICS.REPORTS_SPENDING_MODE, { mode: 'compare' })
})

const budgetReport = report({
  expense: [row('cat-food', 'Groceries', flat('100'), { planned: months.map((m) => (m === '2026-03-01' ? '' : '120')) })],
})

it('planned line only in the budget scope with one row selected and a month step', async () => {
  openSpending({ budgetId: 'b1', selectedId: 'cat-food', step: 'month' }, budgetReport)
  await screen.findByRole('list', { name: 'Legend' })
  await waitFor(() => expect(document.body.querySelector('.recharts-line')).not.toBeNull())
  expect(screen.getByText('Planned')).toBeInTheDocument()
})

it.each([
  ['my accounts', { budgetId: null, selectedId: 'cat-food', step: 'month' as const }],
  ['no row selected', { budgetId: 'b1', step: 'month' as const }],
  ['year step', { budgetId: 'b1', selectedId: 'cat-food', step: 'year' as const }],
])('no planned line: %s', async (_, patch) => {
  openSpending(patch, budgetReport)
  await screen.findByRole('list', { name: 'Legend' })
  expect(document.body.querySelector('.recharts-line')).toBeNull()
})

it('a bar of a selected row opens its transactions', async () => {
  openSpending({ selectedId: 'cat-food', step: 'year' })
  await screen.findByRole('list', { name: 'Legend' })
  await waitFor(() => expect(document.body.querySelectorAll('.recharts-bar').length).toBeGreaterThan(0))
  const bars = document.body.querySelectorAll('.recharts-bar')
  const series = bars[bars.length - 1]
  fireEvent.click(series.querySelector('.recharts-rectangle')!)
  expect(await screen.findByRole('dialog')).toBeInTheDocument()
  expect(trackEvent).toHaveBeenCalledWith(METRICS.REPORTS_DRILL_DOWN, { scope: 'mine', mode: 'overTime' })
})
