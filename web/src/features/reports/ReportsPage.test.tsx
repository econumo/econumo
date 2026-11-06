import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { server } from '@/test/msw'
import { coreHandlers, fixtureBudgets, fixtureUser } from '@/test/fixtures'
import { METRICS, trackEvent } from '@/lib/metrics'
import { useUiStore } from '@/app/uiStore'
import { defaultParams, useReportTabs } from './reportTabs'
import { renderReports, reportHandlers, setViewport } from './reportsTestUtils'

vi.mock('@/lib/metrics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/metrics')>()),
  trackEvent: vi.fn(),
}))

const year = new Date().getFullYear()

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  useReportTabs.setState({ userId: null, tabs: [], activeId: '' })
  vi.mocked(trackEvent).mockClear()
  setViewport(false)
})

it('first visit shows one New tab with the report list and this year’s headlines', async () => {
  server.use(...coreHandlers(), ...reportHandlers())
  renderReports()
  expect(await screen.findByRole('tab', { name: 'New tab' })).toHaveAttribute('aria-selected', 'true')
  expect(screen.getAllByRole('tab')).toHaveLength(1)
  const spending = screen.getByRole('button', { name: /What do I spend money on/ })
  expect(await within(spending).findByText('550 $')).toBeInTheDocument()
  expect(within(spending).getByText(`spent in ${year}`)).toBeInTheDocument()
  expect(within(spending).getByText('Groceries is the largest: 500 $, 91% of the total')).toBeInTheDocument()
  const cashflow = screen.getByRole('button', { name: /Do I earn more than I spend/ })
  expect(within(cashflow).getByText('2,450 $')).toBeInTheDocument()
  expect(within(cashflow).getByText('82% of income')).toBeInTheDocument()
  const networth = screen.getByRole('button', { name: /Am I getting richer or poorer/ })
  expect(await within(networth).findByText('2,500 $')).toBeInTheDocument()
  expect(within(networth).getByText('Up 1,500 $ this year')).toBeInTheDocument()
  const forecast = screen.getByRole('button', { name: /What will happen to my money/ })
  expect(forecast).toBeDisabled()
  expect(within(forecast).getByText('Coming later')).toBeInTheDocument()
  expect(trackEvent).toHaveBeenCalledWith(METRICS.REPORTS_OPEN)
})

it('clicking a card turns the tab into that report', async () => {
  server.use(...coreHandlers(), ...reportHandlers())
  renderReports()
  await userEvent.click(await screen.findByRole('button', { name: /Do I earn more than I spend/ }))
  expect(await screen.findByRole('tab', { name: 'Cash flow' })).toHaveAttribute('aria-selected', 'true')
  expect(screen.getByRole('heading', { name: 'Do I earn more than I spend?' })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /What do I spend money on/ })).toBeNull()
  expect(trackEvent).toHaveBeenCalledWith(METRICS.REPORTS_OPEN_REPORT, { report: 'cashflow', scope: 'mine' })
})

it('the scope picker lists the budgets and My accounts; the chosen report inherits it', async () => {
  server.use(...coreHandlers(), ...reportHandlers())
  renderReports()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('combobox', { name: 'Scope' }))
  await user.click(await screen.findByRole('option', { name: fixtureBudgets[0].name }))
  await user.click(screen.getByRole('button', { name: /What do I spend money on/ }))
  expect(useReportTabs.getState().tabs[0]).toMatchObject({ kind: 'spending', params: { budgetId: 'b1' } })
  expect(trackEvent).toHaveBeenCalledWith(METRICS.REPORTS_OPEN_REPORT, { report: 'spending', scope: 'budget' })
})

it('a first visit starts in the default budget', async () => {
  const user = { ...fixtureUser, options: fixtureUser.options.map((o) => (o.name === 'budget' ? { ...o, value: 'b2' } : o)) }
  server.use(...coreHandlers({ user }), ...reportHandlers())
  renderReports()
  expect(await screen.findByRole('combobox', { name: 'Scope' })).toHaveTextContent(fixtureBudgets[1].name)
})

it('opening the page activates the first tab', async () => {
  useReportTabs.setState({
    userId: 'u1',
    tabs: [
      { id: 't1', kind: 'networth', title: null, params: defaultParams(null) },
      { id: 't2', kind: 'list', title: 'Mine', params: defaultParams(null) },
    ],
    activeId: 't2',
  })
  server.use(...coreHandlers(), ...reportHandlers())
  renderReports()
  expect(await screen.findByRole('tab', { name: 'Net worth' })).toHaveAttribute('aria-selected', 'true')
  expect(screen.getByRole('tab', { name: 'Mine' })).toHaveAttribute('aria-selected', 'false')
  expect(await screen.findByRole('table', { name: 'Am I getting richer or poorer?' })).toBeInTheDocument()
})

it('a tab whose budget is gone falls back to My accounts', async () => {
  useReportTabs.setState({
    userId: 'u1',
    tabs: [{ id: 't1', kind: 'list', title: null, params: defaultParams('b-gone') }],
    activeId: 't1',
  })
  server.use(...coreHandlers(), ...reportHandlers())
  renderReports()
  await waitFor(() => expect(useReportTabs.getState().tabs[0].params.budgetId).toBeNull())
  expect(await screen.findByRole('combobox', { name: 'Scope' })).toHaveTextContent('My accounts')
})

it('another user’s stored tabs are not shown', async () => {
  useReportTabs.setState({
    userId: 'someone-else',
    tabs: [{ id: 't1', kind: 'networth', title: 'Theirs', params: defaultParams(null) }],
    activeId: 't1',
  })
  server.use(...coreHandlers(), ...reportHandlers())
  renderReports()
  expect(await screen.findByRole('tab', { name: 'New tab' })).toBeInTheDocument()
  expect(screen.queryByRole('tab', { name: 'Theirs' })).toBeNull()
})

it('tab actions fire their metrics: new, rename, close', async () => {
  server.use(...coreHandlers(), ...reportHandlers())
  renderReports()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'New tab' }))
  expect(screen.getAllByRole('tab')).toHaveLength(2)
  expect(trackEvent).toHaveBeenCalledWith(METRICS.REPORTS_TAB_NEW)
  await user.dblClick(screen.getAllByRole('tab')[1])
  await user.clear(screen.getByRole('textbox', { name: 'Tab name' }))
  await user.type(screen.getByRole('textbox', { name: 'Tab name' }), 'Mine{Enter}')
  expect(await screen.findByRole('tab', { name: 'Mine' })).toBeInTheDocument()
  expect(trackEvent).toHaveBeenCalledWith(METRICS.REPORTS_TAB_RENAME)
  await user.click(screen.getByRole('button', { name: 'Close Mine' }))
  expect(screen.getAllByRole('tab')).toHaveLength(1)
  expect(trackEvent).toHaveBeenCalledWith(METRICS.REPORTS_TAB_CLOSE)
})

it('phone: long-press Duplicate copies the tab and fires its metric', async () => {
  setViewport(true)
  server.use(...coreHandlers(), ...reportHandlers())
  renderReports()
  const user = userEvent.setup()
  await user.keyboard('{Shift>}')
  await user.click(await screen.findByRole('tab', { name: 'New tab' }))
  await user.keyboard('{/Shift}')
  await user.click(await screen.findByRole('menuitem', { name: 'Duplicate' }))
  expect(screen.getAllByRole('tab')).toHaveLength(2)
  expect(trackEvent).toHaveBeenCalledWith(METRICS.REPORTS_TAB_DUPLICATE)
})

it('without ECONUMO_DEV the list offers Net worth and Suggest a report only', async () => {
  window.econumoConfig = { DEV: false, FEEDBACK: true }
  server.use(...coreHandlers(), ...reportHandlers())
  renderReports()
  expect(await screen.findByRole('button', { name: /Am I getting richer or poorer/ })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /What do I spend money on/ })).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /Do I earn more than I spend/ })).not.toBeInTheDocument()
  expect(screen.queryByText('What will happen to my money?')).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: /Suggest a report/ }))
  expect(useUiStore.getState().feedbackOpen).toBe(true)
  expect(useUiStore.getState().feedbackType).toBe('feature_request')
  expect(useUiStore.getState().feedbackTopic).toBe('reports')
  expect(trackEvent).toHaveBeenCalledWith(METRICS.REPORTS_SUGGEST)
})

it('without ECONUMO_DEV a tab left on Spending goes back to the report list', async () => {
  window.econumoConfig = { DEV: false, FEEDBACK: true }
  useReportTabs.setState({
    userId: 'u1',
    tabs: [{ id: 't1', kind: 'spending', title: null, params: defaultParams(null) }],
    activeId: 't1',
  })
  server.use(...coreHandlers(), ...reportHandlers())
  renderReports()
  expect(await screen.findByRole('button', { name: /Am I getting richer or poorer/ })).toBeInTheDocument()
  await waitFor(() => expect(useReportTabs.getState().tabs[0].kind).toBe('list'))
})

it('with feedback switched off there is no Suggest a report card', async () => {
  window.econumoConfig = { DEV: false, FEEDBACK: false }
  server.use(...coreHandlers(), ...reportHandlers())
  renderReports()
  expect(await screen.findByRole('button', { name: /Am I getting richer or poorer/ })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /Suggest a report/ })).not.toBeInTheDocument()
})
