import { screen, within } from '@testing-library/react'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { useReportTabs } from './reportTabs'
import { renderReports, reportHandlers, setViewport } from './reportsTestUtils'

vi.mock('@/lib/metrics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/metrics')>()),
  trackEvent: vi.fn(),
}))

beforeEach(() => {
  localStorage.clear()
  useReportTabs.setState({ userId: null, tabs: [], activeId: '' })
  setViewport(true)
})

it('a user with no transactions sees the empty state on every card, without errors', async () => {
  const errors = vi.spyOn(console, 'error')
  const year = new Date().getFullYear()
  const months = [`${year}-01-01`, `${year}-02-01`]
  server.use(
    ...coreHandlers({ budgets: [] }),
    ...reportHandlers(
      { currencyId: 'cur-usd', months, expense: [], income: [], transfersIn: ['0', '0'], transfersOut: ['0', '0'], savings: [] },
      { currencyId: 'cur-usd', months, accounts: [] },
    ),
  )
  renderReports()
  for (const name of [/What do I spend money on/, /Do I earn more than I spend/, /Am I getting richer or poorer/]) {
    expect(await within(await screen.findByRole('button', { name })).findByText('No transactions yet')).toBeInTheDocument()
  }
  expect(errors).not.toHaveBeenCalled()
  errors.mockRestore()
})

it('a report that failed to load leaves the card readable', async () => {
  server.use(...coreHandlers(), ...reportHandlers())
  const { http, HttpResponse } = await import('msw')
  server.use(http.get('*/api/v1/report/get-flow-report', () =>
    HttpResponse.json({ success: false, message: 'Something went wrong. Please try again.', code: 500, errors: {} }, { status: 500 })))
  renderReports()
  const spending = await screen.findByRole('button', { name: /What do I spend money on/ })
  expect(await within(spending).findByText('Something went wrong. Please try again.')).toBeInTheDocument()
})
