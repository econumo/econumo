import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider, QueryClient } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { formatDateTime } from '@/lib/datetime'
import { RecurringSettingsPage } from './RecurringSettingsPage'

function mockMatchMedia() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [{ path: '/settings/recurring', element: <RecurringSettingsPage /> }],
    { initialEntries: ['/settings/recurring'] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return queryClient
}

// a year out, so the "not overdue" assertions never age into failures
const futurePaymentAt = formatDateTime(new Date(Date.now() + 365 * 24 * 3600 * 1000))

const wireRecurring = {
  id: 'r1', ownerUserId: 'u1', type: 'expense', accountId: 'a1', accountRecipientId: null,
  amount: '50.5', categoryId: 'cat-food', payeeId: null, tagId: null, description: 'rent',
  schedule: 'monthly', nextPaymentAt: futurePaymentAt,
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  mockMatchMedia()
})

it('lists templates with schedule and next payment date', async () => {
  server.use(
    ...coreHandlers(),
    http.get('*/api/v1/recurring/get-recurring-transaction-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [wireRecurring] } })),
  )
  renderPage()
  expect(await screen.findByText('rent')).toBeInTheDocument()
  expect(screen.getByText('Monthly')).toBeInTheDocument()
})

it('highlights overdue templates and not future-dated ones', async () => {
  const overdue = { ...wireRecurring, id: 'r-overdue', nextPaymentAt: '2020-01-01 00:00:00' }
  server.use(
    ...coreHandlers(),
    http.get('*/api/v1/recurring/get-recurring-transaction-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [wireRecurring, overdue] } })),
  )
  renderPage()
  await screen.findByTestId('recurring-r-overdue')
  expect(screen.getByTestId('recurring-summary-r-overdue')).toHaveClass('text-destructive')
  expect(screen.getByTestId('recurring-summary-r1')).not.toHaveClass('text-destructive')
})

it('shows the empty state when there are no templates', async () => {
  server.use(
    ...coreHandlers(),
    http.get('*/api/v1/recurring/get-recurring-transaction-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [] } })),
  )
  renderPage()
  expect(await screen.findByText('No recurring transactions yet')).toBeInTheDocument()
})

it('tapping a row opens the view dialog, and deleting it asks for confirmation first', async () => {
  server.use(
    ...coreHandlers(),
    http.get('*/api/v1/recurring/get-recurring-transaction-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [wireRecurring] } })),
    http.post('*/api/v1/recurring/delete-recurring-transaction', () =>
      HttpResponse.json({ success: true, message: '', data: {} })),
  )
  const user = userEvent.setup()
  renderPage()

  await user.click(await screen.findByTestId('recurring-r1'))
  expect(await screen.findByText('Recurring transaction')).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Delete' }))
  expect(await screen.findByText('Delete this recurring transaction?')).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Delete' }))
  await screen.findByText('No recurring transactions yet')
})

it('skipping from the view dialog advances the template and closes the dialog', async () => {
  // a month past the fixture date, so the advanced dayKey provably differs
  const advancedPaymentAt = formatDateTime(new Date(Date.now() + 395 * 24 * 3600 * 1000))
  let skipCalls = 0
  server.use(
    ...coreHandlers(),
    http.get('*/api/v1/recurring/get-recurring-transaction-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [wireRecurring] } })),
    http.post('*/api/v1/recurring/skip-recurring-transaction', () => {
      skipCalls += 1
      return HttpResponse.json({
        success: true, message: '',
        data: { item: { ...wireRecurring, nextPaymentAt: advancedPaymentAt } },
      })
    }),
  )
  const user = userEvent.setup()
  renderPage()

  await user.click(await screen.findByTestId('recurring-r1'))
  expect(await screen.findByText('Recurring transaction')).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Skip' }))
  await waitFor(() => expect(screen.queryByText('Recurring transaction')).toBeNull())
  expect(skipCalls).toBe(1)
  expect(screen.getByTestId('recurring-summary-r1')).toHaveTextContent(advancedPaymentAt.slice(0, 10))
})
