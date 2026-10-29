import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
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
  id, name, icon: '', currencyId: 'cur-usd', ownerUserId: 'u1', balances, ...over,
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
// the name cell leads with an icon or avatar ligature; the name is its last element
const cells = (row: HTMLElement) =>
  within(row).getAllByRole('cell').map((c, i) => plain(i === 0 ? (c.lastElementChild?.textContent ?? c.textContent) : c.textContent))
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

it('headline: the latest total and its change over the chosen period', async () => {
  openNetWorth()
  await screen.findByRole('table')
  expect(plain(headline().textContent)).toContain('6,200.00 $')
  expect(plain(headline().textContent)).toContain('+3,100.00 $ in the last 3 years')
})

it('my accounts: folders, debts included, all-zero accounts hidden', async () => {
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

it('a folder expands to its accounts', async () => {
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

it('an account opens its transactions over the shown period', async () => {
  openNetWorth()
  await screen.findByRole('table')
  await userEvent.click(screen.getByRole('button', { name: 'Show the accounts in General' }))
  await userEvent.click(screen.getByRole('button', { name: 'Show the transactions of Cash' }))
  const dialog = await screen.findByRole('dialog')
  expect(within(dialog).getByText(/^Cash · /)).toBeInTheDocument()
  expect(await within(dialog).findByText('coffee beans')).toBeInTheDocument()
  expect(trackEvent).toHaveBeenCalledWith(METRICS.REPORTS_DRILL_DOWN, { scope: 'mine', mode: 'networth' })
})

it('an account outside my account list is not clickable', async () => {
  openNetWorth()
  await screen.findByRole('table')
  await userEvent.click(screen.getByRole('button', { name: 'Show the accounts in No folder' }))
  expect(rowNamed('Car loan')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Show the transactions of Car loan' })).not.toBeInTheDocument()
})

it('dragging across the chart shows the change over the selected range', async () => {
  openNetWorth()
  await screen.findByRole('table')
  const surface = document.querySelector('.recharts-wrapper') as HTMLElement
  // recharts resolves the hovered month on mouse move (on the next frame), then reports it on down/up
  const at = async (clientX: number) => {
    fireEvent.mouseMove(surface, { clientX, clientY: 100 })
    await act(() => new Promise((r) => setTimeout(r, 50)))
  }
  await at(45)
  fireEvent.mouseDown(surface, { clientX: 45, clientY: 100 })
  await at(220)
  fireEvent.mouseUp(surface, { clientX: 220, clientY: 100 })
  await waitFor(() => expect(screen.getByText('Dec 2025 – May 2026')).toBeInTheDocument())
  // the range replaces the period presets
  expect(screen.queryByRole('radiogroup', { name: 'Period' })).not.toBeInTheDocument()
  expect(plain(headline().textContent)).toContain('May 2026')
  expect(plain(headline().textContent)).toContain('4,100.00 $')
  expect(plain(headline().textContent)).toContain('+1,000.00 $ since Dec 2025')
  expect(cells(rowNamed('Total'))).toEqual(['Total', '3,100.00 $', '4,100.00 $', '+1,000.00 $'])
  expect(trackEvent).toHaveBeenCalledWith(METRICS.REPORTS_NETWORTH_RANGE, { scope: 'mine' })
  await userEvent.click(screen.getByRole('button', { name: 'Clear the selection' }))
  expect(plain(headline().textContent)).toContain('Today')
  expect(screen.getByRole('radiogroup', { name: 'Period' })).toBeInTheDocument()
  expect(cells(rowNamed('Total'))).toEqual(['Total', '3,100.00 $', '6,200.00 $', '+3,100.00 $'])
})

it('budget scope: one row per person, partners first and me last; savings by name, the rest as Everyday accounts', async () => {
  const data = report([
    acc('a1', 'Cash', ['1000', '1500', '2000']),
    acc('dep', 'Deposit', ['200', '300', '400'], { isSavings: 1 }),
    acc('b-sav', 'Bob savings', ['700', '800', '900'], { ownerUserId: 'u2', isSavings: 1 }),
    acc('b-wal', 'Bob wallet', ['50', '60', '70'], { ownerUserId: 'u2' }),
  ])
  openNetWorth({ budgetId: 'b1' }, data, { budgets: [sharedBudget, fixtureBudgets[1]] })
  await screen.findByRole('table')
  const names = await waitFor(() => {
    const list = screen.getAllByRole('row').slice(1).map((r) => cells(r)[0])
    expect(list.length).toBe(3)
    return list
  })
  expect(names[0]).toBe('Bob')
  expect(names[1]).toBe('Ada')
  expect(names[2]).toBe('Total')
  expect(hasRow('Savings')).toBe(false)

  await userEvent.click(screen.getByRole('button', { name: 'Show the accounts in Bob' }))
  expect(cells(rowNamed('Bob savings')).slice(1)).toEqual(['700.00 $', '900.00 $', '+200.00 $'])
  expect(cells(rowNamed('Everyday accounts')).slice(1)).toEqual(['50.00 $', '70.00 $', '+20.00 $'])
  expect(screen.queryByText('Bob wallet')).not.toBeInTheDocument()
  expect(asked[asked.length - 1].get('budgetId')).toBe('b1')
})

it("budget scope: a savings account, a partner's too, lists its transactions from the budget's list", async () => {
  const data = report([
    acc('a1', 'Cash', ['1000', '1500', '2000']),
    acc('b-sav', 'Bob savings', ['700', '800', '900'], { ownerUserId: 'u2', isSavings: 1 }),
    acc('b-wal', 'Bob wallet', ['50', '60', '70'], { ownerUserId: 'u2' }),
  ])
  const listed: string[] = []
  openNetWorth({ budgetId: 'b1' }, data, { budgets: [sharedBudget, fixtureBudgets[1]] })
  server.use(
    http.get('*/api/v1/budget/get-transaction-list', ({ request }) => {
      const q = new URL(request.url).searchParams
      listed.push(`${q.get('periodStart')}|${q.get('accountId')}`)
      const items = q.get('periodStart') === '2026-07-01'
        ? [{
            id: 'w1', author: bob, currencyId: 'cur-usd', amount: '100', description: 'monthly saving',
            category: null, payee: null, tag: null, labelIds: [], spentAt: '2026-07-04 12:00:00',
          }]
        : []
      return HttpResponse.json({ success: true, message: '', data: { items } })
    }),
  )
  await screen.findByRole('table')
  await userEvent.click(await screen.findByRole('button', { name: 'Show the accounts in Bob' }))
  expect(screen.queryByRole('button', { name: 'Show the transactions of Everyday accounts' })).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: 'Show the transactions of Bob savings' }))
  expect(await screen.findByText('monthly saving')).toBeInTheDocument()
  // the change from Dec 2025 covers the months after it
  expect(listed).toContain('2026-01-01|b-sav')
  expect(listed).toContain('2026-10-01|b-sav')
  expect(listed).not.toContain('2025-12-01|b-sav')
})

it('budget scope: a person without savings members is a single row', async () => {
  const data = report([acc('a1', 'Cash', ['1000', '1500', '2000']), acc('b-wal', 'Bob wallet', ['50', '60', '70'], { ownerUserId: 'u2' })])
  openNetWorth({ budgetId: 'b1' }, data, { budgets: [sharedBudget] })
  await screen.findByRole('table')
  await waitFor(() => expect(screen.getAllByRole('row')).toHaveLength(4))
  expect(screen.queryByRole('button', { name: /Show the accounts/ })).not.toBeInTheDocument()
})

it('period presets map onto the tab and fetch from the close before the period', async () => {
  openNetWorth()
  await screen.findByRole('table')
  expect(asked[0].get('from')).toBe('2023-12-01')
  await userEvent.click(screen.getByRole('radio', { name: '1y' }))
  expect(useReportTabs.getState().tabs[0].params.period).toBe('last12')
  await waitFor(() => expect(asked[asked.length - 1].get('from')).toBe('2025-10-01'))
  expect(trackEvent).toHaveBeenCalledWith(METRICS.REPORTS_CHANGE_PERIOD, { period: 'last12' })
  expect(plain(headline().textContent)).toContain('in the last year')
  await userEvent.click(screen.getByRole('radio', { name: 'All' }))
  await waitFor(() => expect(asked[asked.length - 1].get('from')).toBeNull())
})

it('the budget picker lists active budgets only and disappears without any', async () => {
  openNetWorth({}, report(), { budgets: [{ ...fixtureBudgets[0], isArchived: 1 }] })
  await screen.findByRole('table')
  expect(screen.queryByRole('combobox', { name: 'Scope' })).not.toBeInTheDocument()
})

it('empty state when no account holds a balance', async () => {
  openNetWorth({}, report([acc('zero', 'Empty wallet', ['0', '0', '0'])]))
  expect(await screen.findByText('No account balances yet')).toBeInTheDocument()
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
  // the period stays switchable, so All can still reach older balances
  expect(screen.getByRole('radio', { name: 'All' })).toBeInTheDocument()
})

it('a single-month report shows the balance without a change line', async () => {
  openNetWorth({}, { currencyId: 'cur-usd', months: ['2026-10-01'], accounts: [acc('a1', 'Cash', ['2000'])] })
  await screen.findByRole('table')
  expect(plain(headline().textContent)).toContain('2,000.00 $')
  expect(plain(headline().textContent)).not.toContain('in the last')
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
