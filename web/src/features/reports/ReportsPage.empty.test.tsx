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

const card = () => screen.findByRole('button', { name: /Am I getting richer or poorer/ })

it('a user with no transactions sees the empty state on the card, without errors', async () => {
  const errors = vi.spyOn(console, 'error')
  const year = new Date().getFullYear()
  server.use(
    ...coreHandlers({ budgets: [] }),
    ...reportHandlers({ currencyId: 'cur-usd', months: [`${year}-01-01`, `${year}-02-01`], accounts: [] }),
  )
  renderReports()
  expect(await within(await card()).findByText('No transactions yet')).toBeInTheDocument()
  expect(errors).not.toHaveBeenCalled()
  errors.mockRestore()
})

it('a report that failed to load leaves the card readable', async () => {
  server.use(...coreHandlers(), ...reportHandlers())
  const { http, HttpResponse } = await import('msw')
  server.use(http.get('*/api/v1/report/get-net-worth-report', () =>
    HttpResponse.json({ success: false, message: 'Something went wrong. Please try again.', code: 500, errors: {} }, { status: 500 })))
  renderReports()
  expect(await within(await card()).findByText('Something went wrong. Please try again.')).toBeInTheDocument()
})

it('offline with no report cached (the request failed on a dead network), the card says it is not on this device', async () => {
  const { onlineManager, act } = { ...(await import('@tanstack/react-query')), ...(await import('react')) }
  const { http, HttpResponse } = await import('msw')
  server.use(http.get('*/api/v1/report/get-net-worth-report', () => HttpResponse.error()), ...coreHandlers(), ...reportHandlers())
  renderReports()
  // the request died on the network before the browser noticed the drop
  expect(await within(await card()).findByText('Something went wrong. Please try again.')).toBeInTheDocument()
  act(() => onlineManager.setOnline(false))
  onTestFinished(() => onlineManager.setOnline(true))
  expect(await within(await card()).findByText("This data isn't saved on this device. Connect to the internet to load it.")).toBeInTheDocument()
})
