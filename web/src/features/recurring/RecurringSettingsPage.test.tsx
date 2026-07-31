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

it('shows the interval beside the title and the next payment under the amount', async () => {
  server.use(...coreHandlers({ recurring: [wireRecurring] }))
  renderPage()
  const row = (await screen.findByTestId('recurring-r1')) as HTMLElement
  // interval trails the title line
  expect(row).toHaveTextContent('Monthly')
  // next payment under the amount, not overdue -> not red
  const next = screen.getByTestId('recurring-next-r1')
  expect(next).toHaveTextContent(futurePaymentAt.slice(0, 10))
  expect(next.className).not.toContain('text-destructive')
})

it('marks a due template by turning its next payment red', async () => {
  const due = { ...wireRecurring, id: 'r-due', nextPaymentAt: '2020-01-01 00:00:00' }
  server.use(...coreHandlers({ recurring: [wireRecurring, due] }))
  renderPage()
  expect((await screen.findByTestId('recurring-next-r-due')).className).toContain('text-destructive')
  expect(screen.getByTestId('recurring-next-r1').className).not.toContain('text-destructive')
})

it('shows the empty state when there are no templates', async () => {
  server.use(...coreHandlers({ recurring: [] }))
  renderPage()
  expect(await screen.findByText('No recurring transactions yet')).toBeInTheDocument()
})

it('clicking a row opens the action menu with post/edit/delete', async () => {
  server.use(...coreHandlers({ recurring: [wireRecurring] }))
  const user = userEvent.setup()
  renderPage()

  await user.click(await screen.findByTestId('recurring-r1'))
  const menu = await screen.findByRole('menu')
  const items = Array.from(menu.querySelectorAll('[role="menuitem"]')).map((el) => el.textContent)
  // skip belongs to the account list, where a due template is actionable in place
  expect(items).toEqual(['Post', 'Edit', 'Delete'])
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

it('offers no skip action, and never calls skip', async () => {
  // skipping a payment is an account-list action (a due template shown among the
  // transactions it will become), not template management
  let skipCalls = 0
  server.use(
    ...coreHandlers({ recurring: [wireRecurring] }),
    http.post('*/api/v1/recurring/skip-recurring-transaction', () => {
      skipCalls += 1
      return HttpResponse.json({ success: true, message: '', data: { item: wireRecurring } })
    }),
  )
  const user = userEvent.setup()
  renderPage()

  await user.click(await screen.findByTestId('recurring-r1'))
  await screen.findByRole('menu')
  expect(screen.queryByRole('menuitem', { name: 'Skip' })).toBeNull()
  expect(skipCalls).toBe(0)
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
  for (const label of ['Post', 'Edit', 'Delete']) {
    expect(await screen.findByRole('button', { name: label })).toBeInTheDocument()
  }
  expect(screen.queryByRole('button', { name: 'Skip' })).toBeNull()
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

it('groups templates by account, ordered by day then month with the year ignored', async () => {
  server.use(
    ...coreHandlers({
      recurring: [
        // Bank group listed after Cash (account order), despite the earliest day
        { ...wireRecurring, id: 'r-bank', accountId: 'a2', nextPaymentAt: '2026-03-09 00:00:00' },
        { ...wireRecurring, id: 'r-day17', nextPaymentAt: '2026-01-17 00:00:00' },
        // same day, later YEAR but earlier month — must sort first (year ignored)
        { ...wireRecurring, id: 'r-day5-jun', nextPaymentAt: '2027-06-05 00:00:00' },
        { ...wireRecurring, id: 'r-day5-dec', nextPaymentAt: '2026-12-05 00:00:00' },
      ],
    }),
  )
  renderPage()
  await screen.findByTestId('recurring-r-bank')

  const ids = Array.from(document.querySelectorAll('[data-testid^="recurring-r"]')).map((el) =>
    el.getAttribute('data-testid'),
  )
  expect(ids).toEqual(['recurring-r-day5-jun', 'recurring-r-day5-dec', 'recurring-r-day17', 'recurring-r-bank'])
  // the rows never name the account, so the captions carry it
  expect(screen.getByText('Cash')).toBeInTheDocument()
  expect(screen.getByText('Bank')).toBeInTheDocument()
})

it('labels the create action "Add transaction"', async () => {
  server.use(...coreHandlers({ recurring: [] }))
  renderPage()
  expect(await screen.findByTestId('recurring-create')).toHaveTextContent('Add transaction')
})
