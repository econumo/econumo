import { screen, waitFor, within } from '@testing-library/react'
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

// Jan .. Oct 2026; "today" is 2026-10-15
const months = Array.from({ length: 10 }, (_, i) => `2026-${String(i + 1).padStart(2, '0')}-01`)
const flat = (v: string) => months.map(() => v)

const row = (id: string, name: string, amounts: string[], over: Partial<ReportRowDto> = {}): ReportRowDto => ({
  id, kind: 'category', name, icon: '', isArchived: 0, ownerUserId: null, amounts, planned: [], children: [], ...over,
})

const report = (over: Partial<FlowReportDto> = {}): FlowReportDto => ({
  currencyId: 'cur-usd',
  months,
  income: [row('cat-salary', 'Salary', flat('5000'), { kind: 'income_category' })],
  expense: [
    row('cat-rent', 'Rent', flat('3000')),
    row('cat-food', 'Groceries', flat('1000')),
    // May: 6000 spent against 5000 earned
    row('cat-trip', 'Trip', months.map((m) => (m === '2026-05-01' ? '2000' : '0'))),
  ],
  transfersIn: flat('0'),
  transfersOut: flat('0'),
  savings: [],
  ...over,
})

function openCashFlow(patch: Partial<ReportParams> = {}, data: FlowReportDto = report()) {
  useReportTabs.setState({
    userId: 'u1',
    tabs: [{ id: 't1', kind: 'cashflow', title: null, params: { ...defaultParams(null), period: 'thisYear', step: 'month', ...patch } }],
    activeId: 't1',
  })
  server.use(
    ...coreHandlers(),
    http.get('*/api/v1/report/get-flow-report', () => HttpResponse.json({ success: true, message: '', data: { item: data } })),
    http.get('*/api/v1/oauth/get-identity-list', () => HttpResponse.json({ success: true, message: '', data: [] })),
  )
  return renderReports()
}

const params = () => useReportTabs.getState().tabs[0].params
const headline = () => screen.getByRole('region', { name: 'Summary' })
const plain = (s: string | null) => (s ?? '').replace(/[  ]/g, ' ')
const flowChart = () => screen.getByRole('figure', { name: 'Money flow' })

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date(2026, 9, 15, 12))
  localStorage.clear()
  vi.mocked(trackEvent).mockClear()
  setViewport(false)
})

afterEach(() => {
  vi.useRealTimers()
})

it('headline: saved and its share of income, with the income and spent totals', async () => {
  openCashFlow()
  await screen.findByRole('table')
  // 10 * 5000 earned, 10 * 4000 + 2000 spent
  expect(within(headline()).getByText('Saved 8,000.00 $, 16% of income')).toBeInTheDocument()
  expect(plain(headline().textContent)).toContain('Jan – Oct 2026')
  expect(within(headline()).getByText('50,000.00 $')).toBeInTheDocument()
  expect(within(headline()).getByText('42,000.00 $')).toBeInTheDocument()
})

it('headline: spending over income reads as overspent', async () => {
  openCashFlow({}, report({ income: [row('cat-salary', 'Salary', flat('100'), { kind: 'income_category' })] }))
  await screen.findByRole('table')
  const line = within(headline()).getByText('Spent 41,000.00 $ more than earned')
  expect(line.className).toContain('text-expense')
})

it('table: one row per bucket, newest first; a negative month shows a red saved', async () => {
  openCashFlow()
  const table = await screen.findByRole('table')
  expect(within(table).getAllByRole('columnheader').map((th) => th.textContent)).toEqual(['Period', 'Income', 'Spent', 'Saved'])
  const rows = within(table).getAllByRole('row').slice(1)
  expect(rows[0]).toHaveTextContent('Oct 2026*')
  const may = rows.find((r) => r.textContent?.startsWith('May 2026'))!
  const saved = within(may).getByText('−1,000.00 $')
  expect(saved.className).toContain('text-expense')
  const apr = rows.find((r) => r.textContent?.startsWith('Apr 2026'))!
  expect(within(apr).getByText('+1,000.00 $').className).toContain('text-income')
  expect(screen.getByText(/Oct 2026 is not over yet/)).toBeInTheDocument()
})

it('table: transfer columns appear when money moved in or out', async () => {
  openCashFlow({}, report({ transfersIn: months.map((m) => (m === '2026-03-01' ? '250' : '0')) }))
  const table = await screen.findByRole('table')
  expect(within(table).getAllByRole('columnheader').map((th) => th.textContent)).toEqual([
    'Period', 'Income', 'Spent', 'Saved', 'Transferred in', 'Transferred out',
  ])
  expect(within(table).getByText('250.00 $')).toBeInTheDocument()
})

it('bars draw income, spending and the dashed saved line', async () => {
  openCashFlow()
  await screen.findByRole('table')
  await waitFor(() => expect(document.body.querySelectorAll('.recharts-bar').length).toBe(2))
  expect(document.body.querySelector('.recharts-line')).not.toBeNull()
  const legend = within(screen.getByRole('list', { name: 'Legend' }))
  expect(legend.getAllByRole('listitem').map((li) => li.textContent)).toEqual(['Income', 'Spending', 'Net saved'])
})

it('phone: the Flow switch is not rendered and a stored Flow view shows the bars', async () => {
  setViewport(true)
  openCashFlow({ view: 'flow' })
  await screen.findByRole('table')
  expect(screen.queryByRole('radio', { name: 'Flow' })).toBeNull()
  expect(screen.queryByRole('figure', { name: 'Money flow' })).toBeNull()
  expect(screen.getByRole('figure', { name: 'Cash flow chart' })).toBeInTheDocument()
})

it('desktop: switching to Flow draws the Sankey and fires its metric', async () => {
  openCashFlow()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('radio', { name: 'Flow' }))
  expect(params().view).toBe('flow')
  expect(trackEvent).toHaveBeenCalledWith(METRICS.REPORTS_CASHFLOW_VIEW, { view: 'flow' })
  const chart = await waitFor(flowChart)
  await waitFor(() => expect(within(chart).getByText('Saved')).toBeInTheDocument())
  for (const name of ['Salary', 'Income', 'Rent', 'Groceries', 'Trip']) {
    expect(within(chart).getByText(name)).toBeInTheDocument()
  }
  expect(within(chart).getByText('8,000.00 $')).toBeInTheDocument()
  expect(within(chart).queryByText('From balance')).toBeNull()
  // the table stays below the flow
  expect(screen.getByRole('table')).toBeInTheDocument()
})

it('Flow: spending over income draws from the balance, and both sides add up', async () => {
  const data = report({ income: [row('cat-salary', 'Salary', flat('4000'), { kind: 'income_category' })] })
  openCashFlow({ view: 'flow' }, data)
  const chart = await waitFor(flowChart)
  await waitFor(() => expect(within(chart).getByText('From balance')).toBeInTheDocument())
  // 40,000 earned + 2,000 from balance = 42,000 available = 42,000 spent
  expect(within(chart).getAllByText('2,000.00 $')).toHaveLength(2) // Trip and From balance
  const hub = within(chart).getByText('Available').closest('text')!
  expect(within(hub as unknown as HTMLElement).getByText('42,000.00 $')).toBeInTheDocument()
  expect(within(chart).queryByText('Income')).toBeNull()
  expect(within(headline()).getByText('40,000.00 $')).toBeInTheDocument()
  expect(within(chart).queryByText('Saved')).toBeNull()
})

it('Flow: transfers in and out are their own nodes, and the hub reads Available', async () => {
  const data = report({ transfersIn: flat('100'), transfersOut: flat('50') })
  openCashFlow({ view: 'flow' }, data)
  const chart = await waitFor(flowChart)
  await waitFor(() => expect(within(chart).getByText('Transferred in')).toBeInTheDocument())
  expect(within(chart).getByText('Transferred out')).toBeInTheDocument()
  const hub = within(chart).getByText('Available').closest('text')!
  // 50,000 earned + 1,000 transferred in
  expect(within(hub as unknown as HTMLElement).getByText('51,000.00 $')).toBeInTheDocument()
  // 51,000 - 42,000 spent - 500 transferred out
  expect(within(chart).getByText('8,500.00 $')).toBeInTheDocument()
})

it('Flow in a budget without savings members draws one Saved node', async () => {
  openCashFlow({ view: 'flow', budgetId: 'b1' })
  const chart = await waitFor(flowChart)
  await waitFor(() => expect(within(chart).getByText('Saved')).toBeInTheDocument())
  expect(within(chart).queryByText('Stayed in everyday accounts')).toBeNull()
  expect(within(chart).queryByText('Moved to savings accounts')).toBeNull()
})

it('Flow in a budget splits Saved into savings and everyday accounts', async () => {
  const data = report({ savings: [row('acc-savings', 'Deposit', flat('500'), { kind: 'savings' })] })
  openCashFlow({ view: 'flow', budgetId: 'b1' }, data)
  const chart = await waitFor(flowChart)
  await waitFor(() => expect(within(chart).getByText('Moved to savings accounts')).toBeInTheDocument())
  expect(within(chart).getByText('5,000.00 $')).toBeInTheDocument()
  expect(within(chart).getByText('Stayed in everyday accounts')).toBeInTheDocument()
  expect(within(chart).getByText('3,000.00 $')).toBeInTheDocument()
})

it('a period with transfers only still shows its table', async () => {
  openCashFlow({}, report({ income: [], expense: [], transfersIn: months.map((m) => (m === '2026-05-01' ? '300' : '0')) }))
  expect(await screen.findByRole('table')).toBeInTheDocument()
  expect(screen.queryByText('No transactions yet')).toBeNull()
})

it('an empty period says so', async () => {
  openCashFlow({}, report({ income: [], expense: [] }))
  expect(await screen.findByText('No transactions yet')).toBeInTheDocument()
  expect(screen.queryByRole('table')).toBeNull()
})
