import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider, QueryClient } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { formatDateTime } from '@/lib/datetime'
import { useUiStore } from '@/app/uiStore'
import { RecurringSettingsPage } from './RecurringSettingsPage'

function mockMatchMedia(compact = false) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: compact, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
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
  // the ui store is module-scoped, so a leftover request would leak between tests
  useUiStore.setState({ transactionModal: null, recurringModal: null })
})

it('lists templates as transaction rows', async () => {
  server.use(...coreHandlers({ recurring: [wireRecurring] }))
  renderPage()
  expect(await screen.findByText('rent')).toBeInTheDocument()
})

it('does not repeat the schedule or the next payment date in the list', async () => {
  // both live in the preview and the edit form; the row is the transaction shape
  server.use(...coreHandlers({ recurring: [wireRecurring] }))
  renderPage()
  await screen.findByText('rent')
  expect(screen.queryByText('Monthly')).toBeNull()
  expect(screen.queryByTestId('recurring-summary-r1')).toBeNull()
  expect(screen.queryByText(futurePaymentAt.slice(0, 10))).toBeNull()
})

it('shows the empty state when there are no templates', async () => {
  server.use(...coreHandlers({ recurring: [] }))
  renderPage()
  expect(await screen.findByText('No recurring transactions yet')).toBeInTheDocument()
})

it('clicking a row opens the action menu with post/skip/edit/delete', async () => {
  server.use(...coreHandlers({ recurring: [wireRecurring] }))
  const user = userEvent.setup()
  renderPage()

  await user.click(await screen.findByTestId('recurring-r1'))
  const menu = await screen.findByRole('menu')
  const items = Array.from(menu.querySelectorAll('[role="menuitem"]')).map((el) => el.textContent)
  expect(items).toEqual(['Post', 'Skip', 'Edit', 'Delete'])
})

it('the kebab opens the same menu without the row handler reopening it', async () => {
  server.use(...coreHandlers({ recurring: [wireRecurring] }))
  const user = userEvent.setup()
  renderPage()

  await user.click(await screen.findByRole('button', { name: 'actions rent' }))
  expect(await screen.findByRole('menu')).toBeInTheDocument()
})

it('deleting from the menu asks for confirmation first', async () => {
  server.use(
    ...coreHandlers({ recurring: [wireRecurring] }),
    http.post('*/api/v1/recurring/delete-recurring-transaction', () =>
      HttpResponse.json({ success: true, message: '', data: {} })),
  )
  const user = userEvent.setup()
  renderPage()

  await user.click(await screen.findByTestId('recurring-r1'))
  await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
  expect(await screen.findByText('Delete this recurring transaction?')).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Delete' }))
  await screen.findByText('No recurring transactions yet')
})

it('skipping from the menu advances the template', async () => {
  // a month past the fixture date, so the advanced dayKey provably differs
  const advancedPaymentAt = formatDateTime(new Date(Date.now() + 395 * 24 * 3600 * 1000))
  let skipCalls = 0
  server.use(
    ...coreHandlers({ recurring: [wireRecurring] }),
    http.post('*/api/v1/recurring/skip-recurring-transaction', () => {
      skipCalls += 1
      return HttpResponse.json({
        success: true, message: '',
        data: { item: { ...wireRecurring, nextPaymentAt: advancedPaymentAt } },
      })
    }),
  )
  const user = userEvent.setup()
  const queryClient = renderPage()

  await user.click(await screen.findByTestId('recurring-r1'))
  await user.click(await screen.findByRole('menuitem', { name: 'Skip' }))
  // the list no longer prints the date, so assert the request and that the
  // refreshed template is what the cache now holds
  await waitFor(() => expect(skipCalls).toBe(1))
  await waitFor(() => {
    const cached = queryClient.getQueryData<{ nextPaymentAt: string }[]>(['recurring'])
    expect(cached?.[0]?.nextPaymentAt).toBe(advancedPaymentAt)
  })
})

it('posting from the menu opens the transaction dialog for that template', async () => {
  server.use(...coreHandlers({ recurring: [wireRecurring] }))
  const user = userEvent.setup()
  renderPage()

  await user.click(await screen.findByTestId('recurring-r1'))
  await user.click(await screen.findByRole('menuitem', { name: 'Post' }))
  // the dialog itself lives in the app shell, so assert the store request instead
  await waitFor(() => expect(useUiStore.getState().transactionModal?.postRecurring?.id).toBe('r1'))
})

it('compact: tapping a row opens the action sheet, not a kebab menu', async () => {
  // a tiny kebab is a poor touch target, so compact rows get a bottom sheet
  mockMatchMedia(true)
  server.use(...coreHandlers({ recurring: [wireRecurring] }))
  const user = userEvent.setup()
  renderPage()

  await user.click(await screen.findByTestId('recurring-r1'))
  for (const label of ['Post', 'Skip', 'Edit', 'Delete']) {
    expect(await screen.findByRole('button', { name: label })).toBeInTheDocument()
  }
  expect(screen.queryByRole('button', { name: 'actions rent' })).toBeNull()
})

it('renders the account list row, undimmed', async () => {
  server.use(...coreHandlers({ recurring: [wireRecurring] }))
  renderPage()
  await screen.findByText('rent')
  // the shared TransactionRow, not a bespoke layout
  const row = document.querySelector('[data-testid="tx-r1"]') as HTMLElement
  expect(row).not.toBeNull()
  // every row here is a template, so dimming them all would just look disabled
  expect(row.className).not.toContain('opacity-50')
})

it('labels the create action "Add transaction"', async () => {
  server.use(...coreHandlers({ recurring: [] }))
  renderPage()
  expect(await screen.findByTestId('recurring-create')).toHaveTextContent('Add transaction')
})
