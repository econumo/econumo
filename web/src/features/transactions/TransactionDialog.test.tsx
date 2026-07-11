import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts, fixtureOwner } from '@/test/fixtures'
import { useUiStore } from '@/app/uiStore'
import type { TransactionDto } from '@/api/dto/transaction'
import { TransactionDialog } from './TransactionDialog'

function mockMatchMedia() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderDialog(routePath = '/account/a1') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [{ path: '/account/:id', element: <TransactionDialog /> }, { path: '/', element: <TransactionDialog /> }],
    { initialEntries: [routePath] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return queryClient
}

const wireTxEcho = (over: Record<string, unknown> = {}) => ({
  id: 't-created', author: fixtureOwner, type: 'expense', accountId: 'a1', accountRecipientId: null,
  amount: '9.99', amountRecipient: '9.99', categoryId: 'cat-food', description: '', payeeId: null, tagId: null,
  date: '2026-07-03 10:00:00', ...over,
})

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  mockMatchMedia()
  useUiStore.setState({ transactionModal: null, switchAccountPrompt: null })
})

it('Escape closes the dialog (outside clicks stay blocked via onInteractOutside)', async () => {
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({ type: 'expense' })
  await screen.findByRole('heading', { name: 'Add transaction' })
  await user.keyboard('{Escape}')
  await waitFor(() => expect(useUiStore.getState().transactionModal).toBeNull())
})

it('clicking anywhere on a select card (label/padding) opens the picker', async () => {
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({ type: 'expense' })
  await screen.findByRole('heading', { name: 'Add transaction' })
  await user.click(screen.getByText('Category'))
  expect(await screen.findByPlaceholderText('Search or enter a new name')).toBeInTheDocument()
})

it('Tab from the amount goes to the next field, not the calculator keypad', async () => {
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({ type: 'expense' })
  await screen.findByRole('heading', { name: 'Add transaction' })

  const amount = await screen.findByLabelText('Amount')
  amount.focus()
  await user.tab()
  expect(screen.getByRole('combobox', { name: 'Category' })).toHaveFocus()
  await user.tab({ shift: true })
  expect(amount).toHaveFocus()
})

it('tag chips are keyboard-reachable and toggle with Enter and Space', async () => {
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({ type: 'expense' })
  await screen.findByRole('heading', { name: 'Add transaction' })

  const chip = await screen.findByRole('checkbox', { name: 'vacation' })
  screen.getByRole('combobox', { name: 'Recipient' }).focus()
  await user.tab()
  expect(chip).toHaveFocus()

  await user.keyboard('{Enter}')
  expect(chip).toHaveAttribute('aria-checked', 'true')
  await user.keyboard(' ')
  expect(chip).toHaveAttribute('aria-checked', 'false')
})

it('creates an expense with the exact payload shape', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/transaction/create-transaction', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: wireTxEcho(), accounts: fixtureAccounts } })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({ type: 'expense' })

  await screen.findByRole('heading', { name: 'Add transaction' })
  await user.type(await screen.findByLabelText('Amount'), '5+4.99=')
  expect(screen.getByLabelText('Amount')).toHaveValue('9.99')

  await user.click(screen.getByRole('combobox', { name: 'Category' }))
  await user.click(await screen.findByText('Food'))
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(body).toBeDefined())
  expect(body!.type).toBe('expense')
  expect(body!.accountId).toBe('a1')
  expect(body!.amount).toBe(9.99)
  expect(body!.categoryId).toBe('cat-food')
  expect(body!.accountRecipientId).toBeNull()
  expect(body!.amountRecipient).toBeNull()
  expect(body!.tagId).toBeNull()
  expect(body!.date).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/)
})

it('requires a category for non-transfers with the exact message', async () => {
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({ type: 'expense' })
  await screen.findByRole('heading', { name: 'Add transaction' })
  await user.type(await screen.findByLabelText('Amount'), '5')
  await user.click(screen.getByRole('button', { name: 'Add' }))
  expect(await screen.findByText('Category is required')).toBeInTheDocument()
})

it('tags show on expense but not income; income payee label is Sender', async () => {
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({ type: 'expense' })
  await screen.findByRole('heading', { name: 'Add transaction' })
  expect(await screen.findByRole('checkbox', { name: 'vacation' })).toBeInTheDocument()
  expect(screen.getByRole('combobox', { name: 'Recipient' })).toBeInTheDocument()

  await user.click(screen.getByRole('radio', { name: 'Income' }))
  expect(screen.queryByRole('checkbox', { name: 'vacation' })).not.toBeInTheDocument()
  expect(screen.getByRole('combobox', { name: 'Sender' })).toBeInTheDocument()
})

it('swap recomputes the recipient prefill for the new direction', async () => {
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({ type: 'transfer', accountId: 'a1' })
  await screen.findByRole('heading', { name: 'Add transaction' })
  await user.type(await screen.findByLabelText('Amount'), '100')
  await user.click(screen.getByRole('combobox', { name: 'to account' }))
  await user.click(await screen.findByText(/Euro Stash/))
  expect(await screen.findByLabelText('Amount in EUR')).toHaveValue('90')
  // EUR -> USD: 100 / 0.9
  await user.click(screen.getByRole('button', { name: 'swap accounts' }))
  expect(await screen.findByLabelText('Amount in USD')).toHaveValue('111.11')
})

it('editing a transfer allows changing the sender account', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/transaction/update-transaction', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({
        success: true, message: '',
        data: { item: wireTxEcho({ type: 'transfer', accountId: 'a2', accountRecipientId: 'a3' }), accounts: fixtureAccounts },
      })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({
    // The wire echo carries string amounts, as the API returns them.
    transaction: wireTxEcho({ type: 'transfer', accountId: 'a1', accountRecipientId: 'a3', amountRecipient: '9' }) as unknown as TransactionDto,
  })
  await screen.findByRole('heading', { name: 'Edit transaction' })
  const fromSelect = screen.getByRole('combobox', { name: 'from account' })
  expect(fromSelect).toBeEnabled()
  await user.click(fromSelect)
  await user.click(await screen.findByText(/Bank/))
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body!.accountId).toBe('a2')
  expect(body!.accountRecipientId).toBe('a3')
})

it('editing a same-currency transfer amount re-syncs the recipient amount', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/transaction/update-transaction', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({
        success: true, message: '',
        data: { item: wireTxEcho({ type: 'transfer', accountId: 'a1', accountRecipientId: 'a2' }), accounts: fixtureAccounts },
      })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({
    transaction: wireTxEcho({ type: 'transfer', accountId: 'a1', accountRecipientId: 'a2', amount: '9.99', amountRecipient: '9.99', categoryId: null }) as unknown as TransactionDto,
  })
  await screen.findByRole('heading', { name: 'Edit transaction' })
  const amount = screen.getByLabelText('Amount')
  await user.clear(amount)
  await user.type(amount, '25')
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body!.amount).toBe(25)
  // the recipient side must follow the edited amount, not keep the old one
  expect(body!.amountRecipient).toBe(25)
})

it('editing a transfer re-syncs the recipient amount when the destination account changes', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/transaction/update-transaction', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({
        success: true, message: '',
        data: { item: wireTxEcho({ type: 'transfer', accountId: 'a1', accountRecipientId: 'a3' }), accounts: fixtureAccounts },
      })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({
    transaction: wireTxEcho({ type: 'transfer', accountId: 'a1', accountRecipientId: 'a2', amount: '10', amountRecipient: '10', categoryId: null }) as unknown as TransactionDto,
  })
  await screen.findByRole('heading', { name: 'Edit transaction' })
  // USD -> EUR at rate 0.9: the old same-currency recipient amount must not survive
  await user.click(screen.getByRole('combobox', { name: 'to account' }))
  await user.click(await screen.findByText(/Euro Stash/))
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body!.accountRecipientId).toBe('a3')
  expect(body!.amountRecipient).toBe(9)
})

it('cross-currency transfer prefills the converted recipient amount and prompts to switch', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/transaction/create-transaction', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({
        success: true, message: '',
        data: { item: wireTxEcho({ type: 'transfer', accountRecipientId: 'a3' }), accounts: fixtureAccounts },
      })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({ type: 'transfer', accountId: 'a1' })

  await screen.findByRole('heading', { name: 'Add transaction' })
  await user.type(await screen.findByLabelText('Amount'), '100')
  // recipient: a3 is the EUR account (rate 0.9) -> 100 USD = 90 EUR
  await user.click(screen.getByRole('combobox', { name: 'to account' }))
  await user.click(await screen.findByText(/Euro Stash/))
  const recipientAmount = await screen.findByLabelText('Amount in EUR')
  expect(recipientAmount).toHaveValue('90')

  await user.click(screen.getByRole('button', { name: 'Add' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body!.type).toBe('transfer')
  expect(body!.accountRecipientId).toBe('a3')
  expect(body!.amountRecipient).toBe(90)
  expect(body!.categoryId).toBeNull()
  await waitFor(() => expect(useUiStore.getState().switchAccountPrompt).toBe('a3'))
})

it('creates a category on the fly and selects it', async () => {
  let created: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/category/create-category', async ({ request }) => {
      created = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({
        success: true, message: '',
        data: { item: { id: 'cat-new', ownerUserId: 'u1', name: 'Books', position: 9, type: 'expense', icon: '', isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' } },
      })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({ type: 'expense' })
  await screen.findByRole('heading', { name: 'Add transaction' })
  await user.click(screen.getByRole('combobox', { name: 'Category' }))
  // the modal popover hides the rest of the dialog from the a11y tree while
  // open; the search input autofocuses, so type into the focused element
  await user.keyboard('Books')
  await user.click(await screen.findByText(/Add «Books»/))
  await waitFor(() => expect(created).toBeDefined())
  expect(created!.name).toBe('Books')
  expect(created!.type).toBe('expense')
  await waitFor(() => expect(screen.getByRole('combobox', { name: 'Category' })).toHaveTextContent('Books'))
})
