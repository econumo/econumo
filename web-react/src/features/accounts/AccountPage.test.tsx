import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts, fixtureOwner } from '@/test/fixtures'
import { useUiStore } from '@/app/uiStore'
import { AccountPage } from './AccountPage'
import { TransactionDialog } from '@/features/transactions/TransactionDialog'

function mockViewport(compact: boolean) {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: query.includes('1023') ? compact : false,
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }))
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      {
        path: '/account/:id',
        element: (
          <>
            <AccountPage />
            <TransactionDialog />
          </>
        ),
      },
      { path: '/', element: <div>HOME</div> },
    ],
    { initialEntries: ['/account/a1'] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return queryClient
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  useUiStore.setState({ transactionModal: null, accountModal: null, switchAccountPrompt: null })
})

it('renders the header balance and the grouped transaction rows', async () => {
  mockViewport(false)
  renderPage()
  expect(await screen.findByRole('heading', { name: 'Cash' })).toBeInTheDocument()
  expect(screen.getByTestId('account-balance')).toHaveTextContent('100.50 $')
  // expense row: category title + separate description; sign and symbol
  const expenseRow = await screen.findByTestId('tx-t1')
  expect(expenseRow).toHaveTextContent('Food')
  expect(expenseRow).toHaveTextContent('coffee beans')
  expect(expenseRow).toHaveTextContent(/-9\.99\s*\$/)
  // income row
  expect(screen.getByTestId('tx-t2')).toHaveTextContent(/\+500\.00\s*\$/)
})

it('search narrows the list', async () => {
  mockViewport(false)
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('tx-t1')
  await user.type(screen.getByLabelText('Search'), 'salary')
  expect(screen.queryByTestId('tx-t1')).not.toBeInTheDocument()
  expect(screen.getByTestId('tx-t2')).toBeInTheDocument()
})

it('delete flow: confirm dialog fires the API and applies the returned accounts', async () => {
  mockViewport(false)
  let deleted: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/transaction/delete-transaction', async ({ request }) => {
      deleted = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({
        success: true,
        message: '',
        data: {
          item: { id: 't1', author: fixtureOwner, type: 'expense', accountId: 'a1', accountRecipientId: null, amount: '9.99', amountRecipient: '9.99', categoryId: 'cat-food', description: '', payeeId: null, tagId: null, date: '2026-07-02 09:30:00' },
          accounts: [{ ...fixtureAccounts[0], balance: '110.49' }],
        },
      })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('tx-t1')
  await user.click(screen.getByRole('button', { name: 'actions t1' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
  expect(await screen.findByText('Are you sure you want to delete this transaction?')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Delete' }))
  await waitFor(() => expect(deleted).toEqual({ id: 't1' }))
  await waitFor(() => expect(screen.queryByTestId('tx-t1')).not.toBeInTheDocument())
  await waitFor(() => expect(screen.getByTestId('account-balance')).toHaveTextContent('110.49 $'))
})

it('edit opens the transaction dialog prefilled', async () => {
  mockViewport(false)
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('tx-t1')
  await user.click(screen.getByRole('button', { name: 'actions t1' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit' }))
  expect(await screen.findByText('Update transaction')).toBeInTheDocument()
  expect(screen.getByLabelText('Enter amount')).toHaveValue('9.99')
})

it('compact viewport: row click opens the preview dialog with details', async () => {
  mockViewport(true)
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('tx-t1')
  await user.click(screen.getByTestId('tx-t1'))
  const dialog = await screen.findByRole('dialog')
  expect(within(dialog).getByText('Details')).toBeInTheDocument()
  expect(within(dialog).getByText('Expense')).toBeInTheDocument()
  expect(within(dialog).getByText('coffee beans')).toBeInTheDocument()
  expect(within(dialog).getByText('2026-07-02 09:30:00')).toBeInTheDocument()
})
