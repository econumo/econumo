import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers, fixtureBudgets } from '@/test/fixtures'
import { METRICS, trackEvent } from '@/lib/metrics'
import { ReportFilters } from './ReportFilters'
import { defaultParams, useReportTabs } from './reportTabs'
import type { ReportParams } from './reportTabs'
import { setViewport } from './reportsTestUtils'

vi.mock('@/lib/metrics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/metrics')>()),
  trackEvent: vi.fn(),
}))

const params = () => useReportTabs.getState().tabs[0].params

function renderFilters(patch: Partial<ReportParams> = {}, props: { showGroupBy?: boolean; showStep?: boolean } = { showGroupBy: true }) {
  useReportTabs.setState({
    userId: 'u1',
    tabs: [{ id: 't1', kind: 'spending', title: null, params: { ...defaultParams(null), ...patch } }],
    activeId: 't1',
  })
  const Wrapper = () => <ReportFilters tab={useReportTabs((s) => s.tabs[0])} {...props} />
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <Wrapper />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  vi.mocked(trackEvent).mockClear()
  setViewport(false)
  server.use(...coreHandlers())
})

it('group-by offers category, payee and tag in the my-accounts scope', async () => {
  renderFilters()
  const user = userEvent.setup()
  await user.click(screen.getByRole('combobox', { name: 'Group by' }))
  expect(screen.getByRole('option', { name: 'Payee' })).toBeInTheDocument()
  await user.click(screen.getByRole('option', { name: 'Payee' }))
  expect(params().groupBy).toBe('payee')
  expect(trackEvent).toHaveBeenCalledWith(METRICS.REPORTS_CHANGE_GROUP_BY, { groupBy: 'payee' })
})

it('payee group-by is hidden in the budget scope', () => {
  renderFilters({ budgetId: 'b1' })
  expect(screen.queryByRole('combobox', { name: 'Group by' })).toBeNull()
})

it('changing scope clears the selected row and fires its metric', async () => {
  renderFilters({ selectedId: 'cat-food' })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('combobox', { name: 'Scope' }))
  await user.click(await screen.findByRole('option', { name: 'Main budget' }))
  expect(params()).toMatchObject({ budgetId: 'b1', selectedId: null })
  expect(trackEvent).toHaveBeenCalledWith(METRICS.REPORTS_CHANGE_SCOPE, { scope: 'budget' })
})

it('period presets and a custom month range', async () => {
  renderFilters()
  const user = userEvent.setup()
  await user.click(screen.getByRole('combobox', { name: 'Period' }))
  await user.click(screen.getByRole('option', { name: 'Custom' }))
  expect(params().period).toBe('custom')
  expect(trackEvent).toHaveBeenCalledWith(METRICS.REPORTS_CHANGE_PERIOD, { period: 'custom' })
  const from = screen.getByLabelText('From')
  fireEvent.change(from, { target: { value: '2025-02' } })
  expect(params().from).toBe('2025-02-01')
})

it('step is a segmented control', async () => {
  renderFilters()
  const user = userEvent.setup()
  await user.click(screen.getByRole('radio', { name: 'Quarter' }))
  expect(params().step).toBe('quarter')
  expect(trackEvent).toHaveBeenCalledWith(METRICS.REPORTS_CHANGE_STEP, { step: 'quarter' })
})

it('archived budgets are left out, and with none active the scope picker is gone', async () => {
  server.use(...coreHandlers({ budgets: [{ ...fixtureBudgets[0], isArchived: 1 }] }))
  renderFilters()
  await screen.findByRole('combobox', { name: 'Period' })
  await new Promise((r) => setTimeout(r, 50))
  expect(screen.queryByRole('combobox', { name: 'Scope' })).not.toBeInTheDocument()
})
