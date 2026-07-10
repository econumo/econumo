import { act, render, screen, waitFor, within } from '@testing-library/react'
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
  expect(await screen.findByRole('heading', { name: 'Edit transaction' })).toBeInTheDocument()
  expect(screen.getByLabelText('Amount')).toHaveValue('9.99')
})

it('windows long lists: first chunk renders, scroll sentinel loads more', async () => {
  mockViewport(false)
  const ioCallbacks: IntersectionObserverCallback[] = []
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      constructor(cb: IntersectionObserverCallback) {
        ioCallbacks.push(cb)
      }
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  )
  const many = Array.from({ length: 250 }, (_, i) => ({
    id: `bulk-${i}`, author: fixtureOwner, type: 'expense', accountId: 'a1', accountRecipientId: null,
    amount: '1', amountRecipient: '1', categoryId: 'cat-food', description: `bulk ${i}`,
    payeeId: null, tagId: null, date: '2026-07-02 09:30:00',
  }))
  server.use(...coreHandlers({ transactions: many }))
  renderPage()
  await screen.findByTestId('tx-bulk-0')
  const rendered = () => document.querySelectorAll('[data-testid^="tx-"]').length
  expect(rendered()).toBeLessThanOrEqual(100)
  const before = rendered()
  act(() => {
    for (const cb of ioCallbacks) {
      cb([{ isIntersecting: true }] as IntersectionObserverEntry[], {} as IntersectionObserver)
    }
  })
  await waitFor(() => expect(rendered()).toBeGreaterThan(before))
  vi.unstubAllGlobals()
})

it('desktop: clicking anywhere on the row opens the transaction preview', async () => {
  mockViewport(false)
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByTestId('tx-t1'))
  const dialog = await screen.findByRole('dialog', { name: 'Transaction details' })
  expect(within(dialog).getByRole('button', { name: 'Edit' })).toBeInTheDocument()
  expect(within(dialog).getByRole('button', { name: 'Delete' })).toBeInTheDocument()
})

it('desktop: the kebab menu still offers edit/delete without opening the preview', async () => {
  mockViewport(false)
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('tx-t1')
  await user.click(screen.getByRole('button', { name: 'actions t1' }))
  expect(await screen.findByRole('menuitem', { name: 'Edit' })).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: 'Delete' })).toBeInTheDocument()
  expect(screen.queryByRole('dialog', { name: 'Transaction details' })).not.toBeInTheDocument()
})

it('shared account: rows overlay the author avatar on the category icon', async () => {
  mockViewport(false)
  const partner = { id: 'u2', avatar: 'https://avatars.test/partner', name: 'Partner' }
  server.use(
    ...coreHandlers({
      accounts: [{ ...fixtureAccounts[0], sharedAccess: [{ user: partner, role: 'user' }] }],
    }),
  )
  renderPage()
  const row = await screen.findByTestId('tx-t1')
  const avatar = within(row).getByRole('img', { name: 'Ada' })
  expect(avatar).toHaveAttribute('src', `${fixtureOwner.avatar}?s=30`)
})

it('private account: rows show no author avatar', async () => {
  mockViewport(false)
  renderPage()
  const row = await screen.findByTestId('tx-t1')
  expect(within(row).queryByRole('img')).not.toBeInTheDocument()
})

it('compact viewport: add-transaction lives in the footer, header keeps only settings', async () => {
  mockViewport(true)
  renderPage()
  await screen.findByTestId('tx-t1')
  const header = screen.getByRole('banner')
  expect(within(header).queryByRole('button', { name: 'Add transaction' })).not.toBeInTheDocument()
  expect(within(header).getByRole('button', { name: 'Configure' })).toBeInTheDocument()
  const footer = screen.getByRole('contentinfo')
  expect(within(footer).getByRole('button', { name: 'Add transaction' })).toBeInTheDocument()
})

it('preview labels the payee by money direction (expense pays TO a recipient)', async () => {
  mockViewport(true)
  server.use(
    ...coreHandlers({
      transactions: [{
        id: 't-payee', author: fixtureOwner, type: 'expense', accountId: 'a1', accountRecipientId: null,
        amount: '5', amountRecipient: '5', categoryId: 'cat-food', description: '', payeeId: 'p1', tagId: null,
        date: '2026-07-02 09:30:00',
      }],
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByTestId('tx-t-payee'))
  const dialog = await screen.findByRole('dialog')
  expect(within(dialog).getByText('Recipient')).toBeInTheDocument()
  expect(within(dialog).getByText('Grocer')).toBeInTheDocument()
  expect(within(dialog).queryByText('Payee')).not.toBeInTheDocument()
})

it('compact + shared account: preview hero overlays the author avatar', async () => {
  mockViewport(true)
  const partner = { id: 'u2', avatar: 'https://avatars.test/partner', name: 'Partner' }
  server.use(
    ...coreHandlers({
      accounts: [{ ...fixtureAccounts[0], sharedAccess: [{ user: partner, role: 'user' }] }],
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByTestId('tx-t1'))
  const dialog = await screen.findByRole('dialog')
  // the hero avatar (with the author name as alt/title) replaces a separate author row
  expect(within(dialog).getByRole('img', { name: 'Ada' })).toHaveAttribute('src', `${fixtureOwner.avatar}?s=30`)
  expect(within(dialog).queryByText('Author')).not.toBeInTheDocument()
})

it('compact viewport: row click opens the preview dialog with details', async () => {
  mockViewport(true)
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('tx-t1')
  await user.click(screen.getByTestId('tx-t1'))
  // header is visually hidden; the title still names the dialog for a11y
  const dialog = await screen.findByRole('dialog', { name: 'Transaction details' })
  expect(within(dialog).getByText('Expense')).toBeInTheDocument()
  expect(within(dialog).getByText('coffee beans')).toBeInTheDocument()
  expect(within(dialog).getByText('2026-07-02 09:30:00')).toBeInTheDocument()
  // hero: category name with its icon and the signed amount
  expect(within(dialog).getByText('Food')).toBeInTheDocument()
  expect(within(dialog).getByText('restaurant')).toBeInTheDocument()
  expect(within(dialog).getByText(/-9\.99\s*\$/)).toBeInTheDocument()
  // private account: no author avatar anywhere in the preview
  expect(within(dialog).queryByRole('img')).not.toBeInTheDocument()
  // footer is one row: delete icon, wide Edit, collapse icon (no full Cancel button)
  expect(within(dialog).getByRole('button', { name: 'Delete' })).toBeInTheDocument()
  expect(within(dialog).getByRole('button', { name: 'Edit' })).toBeInTheDocument()
  expect(within(dialog).getByRole('button', { name: 'Cancel' })).toBeInTheDocument()
  await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
})
