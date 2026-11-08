import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { useUiStore } from '@/app/uiStore'
import { DebtsSettingsPage } from './DebtsSettingsPage'
import { wireDebt, wireDebtNotSetUp } from './testDebt'

function mockMatchMedia(compact = false) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: compact, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/settings/debts', element: <DebtsSettingsPage /> }], { initialEntries: ['/settings/debts'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

const ok = (data: unknown) => HttpResponse.json({ success: true, message: '', data })

const paidOff = { ...wireDebt, id: 'd2', name: 'Phone plan', currentBalance: '0', paymentsLeft: 0, isPaidOff: 1 as const }

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  mockMatchMedia()
  useUiStore.setState({ debtModal: null, debtDetailsId: null })
})

it('Settings lists debts with payments left', async () => {
  server.use(...coreHandlers(), http.get('*/api/v1/debt/get-debt-list', () => ok({ items: [paidOff, wireDebt] })))
  renderPage()

  const row = await screen.findByTestId('debt-d1')
  expect(row).toHaveTextContent('Car loan')
  expect(row).toHaveTextContent('12 payments left')
  expect(row).toHaveTextContent('1,200.00 $')
  // a finished debt carries its badge instead of a count, and trails the open ones
  const done = screen.getByTestId('debt-d2')
  expect(within(done).getByText('Paid off')).toBeInTheDocument()
  expect(done).not.toHaveTextContent('payments left')
  const order = screen.getAllByRole('listitem').map((li) => li.textContent)
  expect(order[0]).toContain('Car loan')
  expect(order[1]).toContain('Phone plan')
})

it('Settings list shows Finish setup for a debt without terms', async () => {
  server.use(...coreHandlers(), http.get('*/api/v1/debt/get-debt-list', () => ok({ items: [{ ...wireDebtNotSetUp, id: 'd3', name: 'Study' }] })))
  const user = userEvent.setup()
  renderPage()

  const row = await screen.findByTestId('debt-d3')
  expect(row).toHaveTextContent('Study')
  expect(row).toHaveTextContent('Finish setup')
  expect(row).not.toHaveTextContent('payments left')
  expect(row).not.toHaveTextContent('$')
  await user.click(row)
  expect(useUiStore.getState().debtDetailsId).toBe('d3')
})

it('uses the singular for one payment left', async () => {
  server.use(...coreHandlers(), http.get('*/api/v1/debt/get-debt-list', () => ok({ items: [{ ...wireDebt, paymentsLeft: 1 }] })))
  renderPage()
  expect(await screen.findByText('1 payment left')).toBeInTheDocument()
})

it('tapping a debt opens its details', async () => {
  server.use(...coreHandlers(), http.get('*/api/v1/debt/get-debt-list', () => ok({ items: [wireDebt] })))
  const user = userEvent.setup()
  renderPage()

  await user.click(await screen.findByTestId('debt-d1'))

  expect(useUiStore.getState().debtDetailsId).toBe('d1')
})

it('the create button opens an empty debt form', async () => {
  server.use(...coreHandlers(), http.get('*/api/v1/debt/get-debt-list', () => ok({ items: [] })))
  const user = userEvent.setup()
  renderPage()

  expect(await screen.findByText('No debts yet')).toBeInTheDocument()
  await user.click(screen.getByTestId('debt-create'))

  expect(useUiStore.getState().debtModal).toEqual({})
})
