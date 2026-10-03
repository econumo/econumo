import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router'
import { delay, http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureCategories, fixtureOwner, fixtureTransactions } from '@/test/fixtures'
import { useUiStore } from '@/app/uiStore'
import { METRICS, trackEvent } from '@/lib/metrics'
import { GlobalSearchDialog } from './GlobalSearchDialog'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})

function LocationProbe() {
  return <div data-testid="location">{useLocation().pathname}</div>
}

function renderDialog() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/']}>
        <GlobalSearchDialog />
        <Routes>
          <Route path="*" element={<LocationProbe />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

const input = () => screen.getByRole('combobox')
const txRows = () =>
  Array.from(document.querySelectorAll('[data-testid^="tx-"]')).filter((el) => !el.getAttribute('data-testid')!.startsWith('tx-account-'))

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  server.use(
    ...coreHandlers(),
    // the analytics auth-method probe fired once the user data lands
    http.get('*/api/v1/oauth/get-identity-list', () => HttpResponse.json({ success: true, message: '', data: [] })),
  )
  useUiStore.setState({ searchOpen: true, transactionModal: null, accountModal: null })
  vi.mocked(trackEvent).mockClear()
})

it('empty query shows the recent transactions and no group headings', async () => {
  renderDialog()
  expect(await screen.findByTestId('tx-t1')).toBeInTheDocument()
  expect(screen.getByTestId('tx-t2')).toBeInTheDocument()
  // global mode names the account on every row
  expect(screen.getByTestId('tx-account-t1')).toHaveTextContent('Cash')
  for (const heading of ['Accounts', 'Categories', 'Payees', 'Tags', 'Labels', 'Transactions']) {
    expect(screen.queryByText(heading)).not.toBeInTheDocument()
  }
})

it('typing an account name shows the Accounts group; selecting it navigates and closes', async () => {
  const user = userEvent.setup()
  renderDialog()
  await screen.findByTestId('tx-t1')
  await user.type(input(), 'bank')
  expect(await screen.findByText('Accounts')).toBeInTheDocument()
  const row = screen.getByTestId('search-account-a2')
  // folder name identifies the account even in a hidden folder
  expect(row).toHaveTextContent('Savings')
  await user.click(row)
  expect(screen.getByTestId('location')).toHaveTextContent('/account/a2')
  expect(useUiStore.getState().searchOpen).toBe(false)
  expect(trackEvent).toHaveBeenCalledWith(METRICS.GLOBAL_SEARCH_SELECT, { type: 'account' })
})

it('Enter selects the highlighted result and arrows move the highlight', async () => {
  const user = userEvent.setup()
  renderDialog()
  await screen.findByTestId('tx-t1')
  await user.keyboard('{ArrowDown}{Enter}')
  // t2 (Salary) is the second row: the preview opened for it, not t1
  const preview = await screen.findByRole('dialog', { name: 'Transaction details' })
  expect(within(preview).getByText('Salary')).toBeInTheDocument()
  expect(trackEvent).toHaveBeenCalledWith(METRICS.GLOBAL_SEARCH_SELECT, { type: 'transaction' })
})

it('a group with 7 matches shows 5 rows and a Show all row that lifts the cap', async () => {
  const stamp = { createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' }
  const fuel = Array.from({ length: 7 }, (_, i) => ({
    id: `cat-fuel-${i}`, ownerUserId: 'u1', name: `Fuel ${i}`, position: 10 + i, type: 'expense', icon: 'local_gas_station', isArchived: 0, ...stamp,
  }))
  server.use(...coreHandlers({ categories: [...fixtureCategories, ...fuel] }))
  const user = userEvent.setup()
  renderDialog()
  await screen.findByTestId('tx-t1')
  await user.type(input(), 'fuel')
  await screen.findByText('Categories')
  const categoryRows = () => document.querySelectorAll('[data-testid^="search-category-"]')
  expect(categoryRows()).toHaveLength(5)
  await user.click(screen.getByText('Show all (7)'))
  expect(categoryRows()).toHaveLength(7)
  expect(screen.queryByText('Show all (7)')).not.toBeInTheDocument()
})

it('archived classifications are dimmed and badged', async () => {
  const user = userEvent.setup()
  renderDialog()
  await screen.findByTestId('tx-t1')
  await user.type(input(), 'old')
  const row = await screen.findByTestId('search-category-cat-archived')
  expect(row).toHaveTextContent('Archived')
  expect(row.className).toContain('opacity-60')
})

it('selecting a classification narrows to its transactions and clears the input', async () => {
  const user = userEvent.setup()
  renderDialog()
  await screen.findByTestId('tx-t1')
  await user.type(input(), 'food')
  await user.click(await screen.findByTestId('search-category-cat-food'))
  expect(input()).toHaveValue('')
  expect(trackEvent).toHaveBeenCalledWith(METRICS.GLOBAL_SEARCH_SELECT, { type: 'category' })
  await waitFor(() => expect(screen.queryByTestId('tx-t2')).not.toBeInTheDocument())
  expect(screen.getByTestId('tx-t1')).toBeInTheDocument()
  expect(screen.queryByText('Categories')).not.toBeInTheDocument()
  expect(useUiStore.getState().searchOpen).toBe(true)
})

it('typing gibberish shows Nothing found', async () => {
  const user = userEvent.setup()
  renderDialog()
  await screen.findByTestId('tx-t1')
  await user.type(input(), 'zzqqxx')
  expect(await screen.findByText('Nothing found')).toBeInTheDocument()
  expect(txRows()).toHaveLength(0)
})

it('does not claim Nothing found while the user data is still loading', async () => {
  server.use(
    http.get('*/api/v1/user/get-user-data', async () => {
      await delay('infinite')
      return HttpResponse.json({})
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  await screen.findByTestId('tx-t1')
  await user.type(input(), 'zzqqxx')
  await waitFor(() => expect(txRows()).toHaveLength(0))
  expect(screen.queryByText('Nothing found')).not.toBeInTheDocument()
})

it('a transaction opens the preview; Delete confirms and posts', async () => {
  let deletedId: unknown
  server.use(
    http.post('*/api/v1/transaction/delete-transaction', async ({ request }) => {
      deletedId = ((await request.json()) as { id: string }).id
      return HttpResponse.json({ success: true, message: '', data: { item: fixtureTransactions[0], accounts: [] } })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  await user.click(await screen.findByTestId('tx-t1'))
  expect(await screen.findByRole('button', { name: 'Edit' })).toBeEnabled()
  await user.click(screen.getByRole('button', { name: 'Delete' }))
  await user.click(await screen.findByRole('button', { name: 'Delete' }))
  await waitFor(() => expect(deletedId).toBe('t1'))
  // the search stays open; the list refreshes in place
  expect(useUiStore.getState().searchOpen).toBe(true)
})

it('Edit hands the transaction to the form and closes the search', async () => {
  const user = userEvent.setup()
  renderDialog()
  await user.click(await screen.findByTestId('tx-t1'))
  await user.click(await screen.findByRole('button', { name: 'Edit' }))
  expect(useUiStore.getState().transactionModal?.transaction?.id).toBe('t1')
  expect(useUiStore.getState().searchOpen).toBe(false)
})

it('renders the first 100 of 250 transactions', async () => {
  const many = Array.from({ length: 250 }, (_, i) => ({
    id: `m${i}`, author: fixtureOwner, type: 'expense', accountId: 'a1', accountRecipientId: null,
    amount: '1', amountRecipient: '1', categoryId: 'cat-food', description: `item ${i}`,
    payeeId: null, tagId: null, date: `2026-06-${String(1 + (i % 25)).padStart(2, '0')} 10:00:00`,
  }))
  server.use(...coreHandlers({ transactions: many }))
  renderDialog()
  await waitFor(() => expect(txRows().length).toBeGreaterThan(0))
  expect(txRows()).toHaveLength(100)
})

it('focuses the input on open and starts empty on every reopen', async () => {
  const user = userEvent.setup()
  renderDialog()
  await screen.findByTestId('tx-t1')
  await waitFor(() => expect(input()).toHaveFocus())
  await user.type(input(), 'coffee')
  act(() => useUiStore.getState().closeSearch())
  await waitFor(() => expect(screen.queryByRole('combobox')).not.toBeInTheDocument())
  act(() => useUiStore.setState({ searchOpen: true }))
  expect(await screen.findByRole('combobox')).toHaveValue('')
  await waitFor(() => expect(input()).toHaveFocus())
})

it('the account ⋯ menu does not select the row; Edit opens the form over the search', async () => {
  const user = userEvent.setup()
  renderDialog()
  await screen.findByTestId('tx-t1')
  await user.type(input(), 'bank')
  await user.click(await screen.findByRole('button', { name: 'account actions Bank' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit' }))
  expect(useUiStore.getState().accountModal?.account?.id).toBe('a2')
  expect(screen.getByTestId('location')).toHaveTextContent(/^\/$/)
  expect(trackEvent).not.toHaveBeenCalledWith(METRICS.GLOBAL_SEARCH_SELECT, expect.anything())
})

it('an outside click still dismisses the search after a row menu was closed with Escape', async () => {
  const user = userEvent.setup()
  renderDialog()
  await screen.findByTestId('tx-t1')
  await user.type(input(), 'food')
  await user.click(await screen.findByRole('button', { name: 'actions Food' }))
  expect(await screen.findByRole('menuitem', { name: 'Edit' })).toBeInTheDocument()
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('menuitem', { name: 'Edit' })).not.toBeInTheDocument())
  expect(useUiStore.getState().searchOpen).toBe(true)
  await user.click(document.querySelector('[data-slot="dialog-overlay"]')!)
  expect(useUiStore.getState().searchOpen).toBe(false)
})
