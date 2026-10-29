import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { fixtureTransactions } from '@/test/fixtures'
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
let prevQuery: URLSearchParams | null = null
let flowRequests = 0
// the headline's previous-period request is the one carrying `to`
function flowHandler(data: FlowReportDto = report(), previous: FlowReportDto = report({ expense: [] })) {
  return http.get('*/api/v1/report/get-flow-report', ({ request }) => {
    const q = new URL(request.url).searchParams
    flowRequests++
    if (q.has('to')) {
      prevQuery = q
      return HttpResponse.json({ success: true, message: '', data: { item: previous } })
    }
    lastQuery = q
    return HttpResponse.json({ success: true, message: '', data: { item: data } })
  })
}

function openSpending(patch: Partial<ReportParams> = {}, data?: FlowReportDto, previous?: FlowReportDto) {
  useReportTabs.setState({
    userId: 'u1',
    tabs: [{ id: 't1', kind: 'spending', title: null, params: { ...defaultParams(null), ...patch } }],
    activeId: 't1',
  })
  server.use(
    ...coreHandlers(),
    flowHandler(data, previous),
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
  prevQuery = null
  flowRequests = 0
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

const headline = () => screen.getByRole('region', { name: 'Summary' })
const plain = (s: string | null) => (s ?? '').replace(/[\u2009\u202f]/g, ' ')

it('headline: the selected period against the equal-length period before it', async () => {
  // this year = Jan-Oct 2026; the ten months before it = Mar-Dec 2025
  const prevMonths = months.slice(2, 12)
  const previous = report({ months: prevMonths, expense: expense.map((r) => ({ ...r, amounts: r.amounts.slice(2, 12) })) })
  openSpending({ period: 'thisYear' }, report({ months: months.slice(12), expense: expense.map((r) => ({ ...r, amounts: r.amounts.slice(12) })) }), previous)
  await screen.findByRole('list', { name: 'Legend' })
  await waitFor(() => expect(within(headline()).getByText(/\+500\.00 \$ · \+17%/)).toBeInTheDocument())
  expect(plain(headline().textContent)).toContain('Spent in Jan – Oct 2026')
  // (150+90+40+30+20+10+5) * 10
  expect(within(headline()).getByText('3,450.00 $')).toBeInTheDocument()
  expect(plain(headline().textContent)).toContain('vs Mar – Dec 2025')
  expect(within(headline()).getByText(/Groceries is the largest/)).toBeInTheDocument()
})

it('headline: no change when nothing precedes the period', async () => {
  openSpending()
  await screen.findByRole('list', { name: 'Legend' })
  // 2025: 12 * 295, 2026: 10 * 345
  expect(within(headline()).getByText('6,990.00 $')).toBeInTheDocument()
  expect(plain(headline().textContent)).not.toContain('vs')
})

it('table: the last bucket against the one before it, named after the buckets', async () => {
  openSpending()
  const table = await screen.findByRole('table')
  const head = within(table).getAllByRole('columnheader').map((th) => th.textContent)
  expect(head).toEqual(['Name', '2025', '2026*', 'Change', 'Monthly avg'])
  const groceries = within(table).getByRole('button', { name: 'Groceries' }).closest('tr')!
  expect(within(groceries).getByText('1,200.00 $')).toBeInTheDocument()
  expect(within(groceries).getByText('1,500.00 $')).toBeInTheDocument()
})

it('money keeps the currency fraction digits', async () => {
  const data = report({ expense: [row('cat-food', 'Groceries', months.map((m) => (m === '2026-09-01' ? '481.75' : '100')))] })
  openSpending({ step: 'month' }, data)
  const table = await screen.findByRole('table')
  expect(within(table).getAllByText('481.75 $')[0]).toBeInTheDocument()
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
  await screen.findByRole('list', { name: 'Legend' })
  expect(plain(headline().textContent)).toContain('Spent in Jan – Oct 2026')
  expect(within(headline()).getByText('1,500.00 $')).toBeInTheDocument()
  expect(within(headline()).getByText(/\+500\.00 \$ · \+50%/)).toBeInTheDocument()
  expect(plain(headline().textContent)).toContain('vs Jan – Oct 2025')
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

it('compare year pickers keep stored years older than ten years', async () => {
  openSpending({ mode: 'compare', compare: [2010, 2026] })
  expect(await screen.findByRole('combobox', { name: 'Earlier year' })).toHaveTextContent('2010')
})

it('with a row selected, Show transactions opens the drill-down from the keyboard', async () => {
  openSpending({ selectedId: 'cat-food' })
  const user = userEvent.setup()
  const show = await screen.findByRole('button', { name: 'Show transactions' })
  show.focus()
  await user.keyboard('{Enter}')
  expect(await screen.findByRole('dialog')).toBeInTheDocument()
  expect(trackEvent).toHaveBeenCalledWith(METRICS.REPORTS_DRILL_DOWN, { scope: 'mine', mode: 'overTime' })
})

it('deleting a transaction from the drill-down refetches the report', async () => {
  let deleted: unknown
  openSpending({ selectedId: 'cat-food' })
  server.use(
    http.post('*/api/v1/transaction/delete-transaction', async ({ request }) => {
      deleted = ((await request.json()) as { id: string }).id
      return HttpResponse.json({ success: true, message: '', data: { item: fixtureTransactions[0], accounts: [] } })
    }),
  )
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Show transactions' }))
  await user.click(await screen.findByText('coffee beans'))
  await user.click(await screen.findByRole('button', { name: 'Delete' }))
  const before = flowRequests
  await user.click(await screen.findByRole('button', { name: 'Delete' }))
  await waitFor(() => expect(deleted).toBe('t1'))
  await waitFor(() => expect(flowRequests).toBeGreaterThan(before))
})

it('Month step: the table compares the last two complete months, the chart keeps the running one', async () => {
  vi.setSystemTime(new Date(2026, 9, 23, 12))
  openSpending({ step: 'month' })
  const table = await screen.findByRole('table')
  const head = within(table).getAllByRole('columnheader').map((th) => th.textContent)
  expect(head).toEqual(['Name', 'Aug 2026', 'Sep 2026', 'Change', 'Monthly avg'])
  expect(screen.getByText(/Oct 2026 is not over yet/)).toBeInTheDocument()
})

it('headline change for a selected envelope child compares that child only', async () => {
  const child = (amounts: string[]) => ({ id: 'cat-rent', kind: 'category', name: 'Rent', icon: '', isArchived: 0, amounts })
  const envelope = (amounts: string[], childAmounts: string[]) =>
    row('env-home', 'Home', amounts, { kind: 'envelope', children: [child(childAmounts)] })
  const ten = (v: string) => Array.from({ length: 10 }, () => v)
  const cur = report({ months: months.slice(12), expense: [envelope(ten('300'), ten('200'))] })
  const previous = report({ months: months.slice(2, 12), expense: [envelope(ten('250'), ten('150'))] })
  openSpending({ period: 'thisYear', budgetId: 'b1', selectedId: 'cat-rent' }, cur, previous)
  await screen.findByRole('list', { name: 'Legend' })
  // Rent: 2000 now against 1500 before
  await waitFor(() => expect(within(headline()).getByText(/\+500\.00 \$ · \+33%/)).toBeInTheDocument())
  expect(within(headline()).getByText('2,000.00 $')).toBeInTheDocument()
})

it('headline omits the change when the selected row is missing from the previous period', async () => {
  const ten = (v: string) => Array.from({ length: 10 }, () => v)
  const cur = report({ months: months.slice(12), expense: [row('cat-new', 'New', ten('10')), row('cat-old', 'Old', ten('5'))] })
  const previous = report({ months: months.slice(2, 12), expense: [row('cat-old', 'Old', ten('5'))] })
  openSpending({ period: 'thisYear', selectedId: 'cat-new' }, cur, previous)
  await screen.findByRole('list', { name: 'Legend' })
  await waitFor(() => expect(flowRequests).toBeGreaterThanOrEqual(2))
  expect(within(headline()).getByText('100.00 $')).toBeInTheDocument()
  expect(plain(headline().textContent)).not.toContain('vs')
})

it('an ended budget: the headline compares the window the server returned', async () => {
  // this year is asked for, but the budget ended in March: Jan-Mar 2026 vs Oct-Dec 2025
  const three = (v: string) => [v, v, v]
  const cur = report({ months: months.slice(12, 15), expense: [row('cat-food', 'Groceries', three('200'))] })
  const previous = report({ months: months.slice(9, 12), expense: [row('cat-food', 'Groceries', three('100'))] })
  openSpending({ period: 'thisYear', budgetId: 'b1' }, cur, previous)
  await screen.findByRole('list', { name: 'Legend' })
  await waitFor(() => expect(within(headline()).getByText(/\+300\.00 \$ · \+100%/)).toBeInTheDocument())
  expect(prevQuery?.get('from')).toBe('2025-10-01')
  expect(prevQuery?.get('to')).toBe('2025-12-01')
  expect(plain(headline().textContent)).toContain('Spent in Q1 2026')
  expect(plain(headline().textContent)).toContain('vs Q4 2025')
})

it('headline omits the change when the previous window came back different', async () => {
  const three = (v: string) => [v, v, v]
  const cur = report({ months: months.slice(12, 15), expense: [row('cat-food', 'Groceries', three('200'))] })
  // the server cut the previous window short: not a like-for-like comparison
  const previous = report({ months: months.slice(11, 12), expense: [row('cat-food', 'Groceries', ['100'])] })
  openSpending({ period: 'thisYear', budgetId: 'b1' }, cur, previous)
  await screen.findByRole('list', { name: 'Legend' })
  await waitFor(() => expect(prevQuery).not.toBeNull())
  await waitFor(() => expect(flowRequests).toBeGreaterThanOrEqual(2))
  expect(within(headline()).getByText('600.00 $')).toBeInTheDocument()
  expect(plain(headline().textContent)).not.toContain('vs')
})
