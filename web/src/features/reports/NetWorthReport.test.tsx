import { act, screen, waitFor, within } from '@testing-library/react'
import { onlineManager } from '@tanstack/react-query'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import type { NetWorthAccountDto, NetWorthReportDto } from '@/api/dto/report'
import { server } from '@/test/msw'
import { coreHandlers, fixtureBudgets, fixtureOwner } from '@/test/fixtures'
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

const months = ['2025-12-01', '2026-05-01', '2026-10-01']

const acc = (id: string, name: string, balances: string[], over: Partial<NetWorthAccountDto> = {}): NetWorthAccountDto => ({
  id, name, icon: '', currencyId: 'cur-usd', ownerUserId: 'u1', isDeleted: 0, balances, ...over,
})

// a1 sits in my "General" folder, a2 and a3 in "Savings" (fixture account list)
const mine = [
  acc('a1', 'Cash', ['1000', '1500', '2000']),
  acc('a2', 'Bank', ['5000', '5000', '6000']),
  acc('a3', 'Euro Stash', ['100', '100', '200']),
  // not in my account list: no folder; a debt below zero
  acc('loan', 'Car loan', ['-3000', '-2500', '-2000']),
  acc('zero', 'Empty wallet', ['0', '0', '0']),
]

const bob = { id: 'u2', avatar: 'face:sky', name: 'Bob' }
const sharedBudget = {
  ...fixtureBudgets[0],
  access: [{ user: fixtureOwner, role: 'owner', isAccepted: 1 }, { user: bob, role: 'user', isAccepted: 1 }],
}

const report = (accounts: NetWorthAccountDto[] = mine): NetWorthReportDto => ({ currencyId: 'cur-usd', months, accounts })

let asked: URLSearchParams[] = []

function openNetWorth(patch: Partial<ReportParams> = {}, data: NetWorthReportDto = report(), overrides: Record<string, unknown> = {}) {
  useReportTabs.setState({
    userId: 'u1',
    tabs: [{ id: 't1', kind: 'networth', title: null, params: { ...defaultParams(null), ...patch } }],
    activeId: 't1',
  })
  server.use(
    ...coreHandlers(overrides),
    http.get('*/api/v1/report/get-net-worth-report', ({ request }) => {
      asked.push(new URL(request.url).searchParams)
      return HttpResponse.json({ success: true, message: '', data: { item: data } })
    }),
    http.get('*/api/v1/oauth/get-identity-list', () => HttpResponse.json({ success: true, message: '', data: [] })),
  )
  return renderReports()
}

const plain = (s: string | null) => (s ?? '').replace(/[  ]/g, ' ')
const headline = () => screen.getByRole('region', { name: 'Summary' })
const cells = (row: HTMLElement) => within(row).getAllByRole('cell').map((c) => plain(c.textContent))
const rowNamed = (name: string) => {
  const row = screen.getAllByRole('row').find((r) => within(r).queryAllByRole('cell').length > 0 && cells(r)[0] === name)
  if (!row) throw new Error(`no row ${name}`)
  return row
}
const hasRow = (name: string) => screen.queryAllByRole('row').some((r) => within(r).queryAllByRole('cell').length > 0 && cells(r)[0] === name)

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date(2026, 9, 15, 12))
  localStorage.clear()
  vi.mocked(trackEvent).mockClear()
  asked = []
  setViewport(false)
})

afterEach(() => {
  vi.useRealTimers()
})

it('headline: the latest total and its change since the period started', async () => {
  openNetWorth()
  await screen.findByRole('table')
  expect(plain(headline().textContent)).toContain('6,200.00 $')
  expect(plain(headline().textContent)).toContain('Up 3,100.00 $ over the period')
  expect(plain(headline().textContent)).toContain('Dec 2025')
})

it('groups my accounts by my folders, debts included, and hides all-zero accounts', async () => {
  openNetWorth()
  await screen.findByRole('table')
  expect(cells(await waitFor(() => rowNamed('Savings')))).toEqual(['Savings', '5,100.00 $', '6,200.00 $', '+1,100.00 $'])
  expect(cells(rowNamed('General'))).toEqual(['General', '1,000.00 $', '2,000.00 $', '+1,000.00 $'])
  expect(cells(rowNamed('No folder'))).toEqual(['No folder', '-3,000.00 $', '-2,000.00 $', '+1,000.00 $'])
  expect(screen.queryByText('Empty wallet')).not.toBeInTheDocument()
})

it('the total row equals the sum of the groups', async () => {
  openNetWorth()
  await screen.findByRole('table')
  expect(cells(rowNamed('Total'))).toEqual(['Total', '3,100.00 $', '6,200.00 $', '+3,100.00 $'])
})

it('a group expands to its accounts', async () => {
  openNetWorth()
  await screen.findByRole('table')
  expect(screen.queryByText('Euro Stash')).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: 'Show the accounts in Savings' }))
  expect(cells(rowNamed('Bank'))).toEqual(['Bank', '5,000.00 $', '6,000.00 $', '+1,000.00 $'])
  expect(cells(rowNamed('Euro Stash'))).toEqual(['Euro Stash', '100.00 $', '200.00 $', '+100.00 $'])
  expect(trackEvent).toHaveBeenCalledWith(METRICS.REPORTS_NETWORTH_EXPAND, { scope: 'mine' })
  await userEvent.click(screen.getByRole('button', { name: 'Hide the accounts in Savings' }))
  expect(screen.queryByText('Euro Stash')).not.toBeInTheDocument()
  expect(vi.mocked(trackEvent).mock.calls.filter(([m]) => m === METRICS.REPORTS_NETWORTH_EXPAND)).toHaveLength(1)
})

it("budget scope: a partner's accounts are listed under the partner's name", async () => {
  const partner = acc('a2', 'Bob savings', ['700', '800', '900'], { ownerUserId: 'u2' })
  openNetWorth({ budgetId: 'b1' }, report([mine[0], partner]), { budgets: [sharedBudget, fixtureBudgets[1]] })
  await screen.findByRole('table')
  // a2 sits in my "Savings" folder on my side; the owner wins
  expect(cells(await waitFor(() => rowNamed('Bob')))).toEqual(['Bob', '700.00 $', '900.00 $', '+200.00 $'])
  expect(cells(rowNamed('General'))).toEqual(['General', '1,000.00 $', '2,000.00 $', '+1,000.00 $'])
  expect(hasRow('Savings')).toBe(false)
  expect(asked[asked.length - 1].get('budgetId')).toBe('b1')
})

it('period presets map onto the tab and fetch from the close before the period', async () => {
  openNetWorth()
  await screen.findByRole('table')
  expect(asked[0].get('from')).toBe('2023-12-01')
  await userEvent.click(screen.getByRole('radio', { name: '1y' }))
  expect(useReportTabs.getState().tabs[0].params.period).toBe('last12')
  await waitFor(() => expect(asked[asked.length - 1].get('from')).toBe('2025-10-01'))
  expect(trackEvent).toHaveBeenCalledWith(METRICS.REPORTS_CHANGE_PERIOD, { period: 'last12' })
  await userEvent.click(screen.getByRole('radio', { name: 'All' }))
  await waitFor(() => expect(asked[asked.length - 1].get('from')).toBeNull())
})

it('empty state when no account holds a balance', async () => {
  openNetWorth({}, report([acc('zero', 'Empty wallet', ['0', '0', '0'])]))
  expect(await screen.findByText('No account balances yet')).toBeInTheDocument()
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
})

it('a single-month report shows the balance without a change line', async () => {
  openNetWorth({}, { currencyId: 'cur-usd', months: ['2026-10-01'], accounts: [acc('a1', 'Cash', ['2000'])] })
  await screen.findByRole('table')
  expect(plain(headline().textContent)).toContain('2,000.00 $')
  expect(plain(headline().textContent)).not.toContain('No transactions yet')
  expect(plain(headline().textContent)).not.toContain('Oct 2026')
})

it('offline: cached figures say when they were fetched', async () => {
  openNetWorth()
  await screen.findByRole('table')
  expect(screen.queryByText(/^Updated /)).not.toBeInTheDocument()
  act(() => onlineManager.setOnline(false))
  onTestFinished(() => onlineManager.setOnline(true))
  expect(screen.getByText(/^Updated .*2026/)).toBeInTheDocument()
  act(() => onlineManager.setOnline(true))
  expect(screen.queryByText(/^Updated /)).not.toBeInTheDocument()
})

it('a failed refresh keeps the figures and says when they were fetched', async () => {
  const { queryClient } = openNetWorth()
  await screen.findByRole('table')
  server.use(http.get('*/api/v1/report/get-net-worth-report', () => HttpResponse.json({ success: false, message: 'boom', code: 0 }, { status: 500 })))
  await act(() => queryClient.refetchQueries({ queryKey: ['reportNetWorth'] }))
  expect(await screen.findByText(/^Updated /)).toBeInTheDocument()
  expect(screen.getByRole('table')).toBeInTheDocument()
})
