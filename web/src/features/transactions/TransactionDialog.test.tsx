import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts, fixtureLabels, fixtureOwner, fixtureUsd } from '@/test/fixtures'
import { useUiStore } from '@/app/uiStore'
import type { RecurringDto } from '@/api/dto/recurring'
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
  labelIds: [], date: '2026-07-03 10:00:00', ...over,
})

const wireLabel = (over: Record<string, unknown> = {}) => ({
  id: 'label2', ownerUserId: 'u1', name: 'travel', icon: 'flight', position: 1, isArchived: 0,
  createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00', ...over,
})

const captureUpdate = () => {
  const seen: { body?: Record<string, unknown> } = {}
  server.use(
    http.post('*/api/v1/transaction/update-transaction', async ({ request }) => {
      seen.body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: wireTxEcho(), accounts: fixtureAccounts } })
    }),
  )
  return seen
}

// The accessible name is "<name> <kind word>" (the kind word is untranslated
// until Task 8 lands the catalogue entries), so match on the leading name and
// pick the kind from data-kind.
const chip = (name: string, kind: 'tag' | 'label') => {
  const found = screen.getAllByRole('checkbox', { name: new RegExp('^' + name + '\\b') }).find((el) => el.getAttribute('data-kind') === kind)
  if (!found) {
    throw new Error(`no ${kind} chip named ${name}`)
  }
  return found
}

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

it('Escape with a picker open closes the picker, not the dialog', async () => {
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({ type: 'expense' })
  await screen.findByRole('heading', { name: 'Add transaction' })

  await user.click(screen.getByRole('combobox', { name: 'Category' }))
  expect(await screen.findByRole('option', { name: 'Food' })).toBeInTheDocument()

  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('option', { name: 'Food' })).not.toBeInTheDocument())
  expect(useUiStore.getState().transactionModal).not.toBeNull()

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

  const chip = await screen.findByRole('checkbox', { name: /^vacation\b/ })
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
  expect(body!.amount).toBe('9.99')
  expect(body!.categoryId).toBe('cat-food')
  expect(body!.accountRecipientId).toBeNull()
  expect(body!.amountRecipient).toBeNull()
  expect(body!.tagId).toBeNull()
  expect(body!.date).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/)
})

it('submits an expense with no category', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/transaction/create-transaction', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: wireTxEcho({ categoryId: null }), accounts: fixtureAccounts } })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({ type: 'expense' })
  await screen.findByRole('heading', { name: 'Add transaction' })
  await user.type(await screen.findByLabelText('Amount'), '5')
  await user.click(screen.getByRole('button', { name: 'Add' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body!.categoryId).toBeNull()
  expect(screen.queryByText('Category is required')).not.toBeInTheDocument()
})

it('clears an existing category via the select\'s clear row', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/transaction/update-transaction', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: wireTxEcho({ categoryId: null }), accounts: fixtureAccounts } })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({
    transaction: wireTxEcho({ categoryId: 'cat-food' }) as unknown as TransactionDto,
  })
  await screen.findByRole('heading', { name: 'Edit transaction' })
  await user.click(screen.getByRole('combobox', { name: 'Category' }))
  await user.click(await screen.findByText('—'))
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body!.categoryId).toBeNull()
})

it('tags show on expense but not income; income payee label is Sender', async () => {
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({ type: 'expense' })
  await screen.findByRole('heading', { name: 'Add transaction' })
  expect(await screen.findByRole('checkbox', { name: /^vacation\b/ })).toBeInTheDocument()
  expect(screen.getByRole('combobox', { name: 'Recipient' })).toBeInTheDocument()

  await user.click(screen.getByRole('radio', { name: 'Income' }))
  expect(screen.queryByRole('checkbox', { name: /^vacation\b/ })).not.toBeInTheDocument()
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
  expect(body!.amount).toBe('25')
  // the recipient side must follow the edited amount, not keep the old one
  expect(body!.amountRecipient).toBe('25')
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
  expect(body!.amountRecipient).toBe('9')
})

it('editing a transfer re-syncs the recipient amount when the SENDER account changes', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/transaction/update-transaction', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({
        success: true, message: '',
        data: { item: wireTxEcho({ type: 'transfer', accountId: 'a3', accountRecipientId: 'a2' }), accounts: fixtureAccounts },
      })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({
    transaction: wireTxEcho({ type: 'transfer', accountId: 'a1', accountRecipientId: 'a2', amount: '10', amountRecipient: '10', categoryId: null }) as unknown as TransactionDto,
  })
  await screen.findByRole('heading', { name: 'Edit transaction' })
  // EUR -> USD at rate 0.9: 10 EUR = 11.11 USD; the stale same-currency
  // recipient amount must not survive a sender-currency change
  await user.click(screen.getByRole('combobox', { name: 'from account' }))
  await user.click(await screen.findByText(/Euro Stash/))
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body!.accountId).toBe('a3')
  expect(body!.amountRecipient).toBe('11.11')
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
  expect(body!.amountRecipient).toBe('90')
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
  // the picker is the field itself: typing filters in place
  await user.keyboard('Books')
  await user.click(await screen.findByText(/Add «Books»/))
  await waitFor(() => expect(created).toBeDefined())
  expect(created!.name).toBe('Books')
  expect(created!.type).toBe('expense')
  await waitFor(() => expect(screen.getByRole('combobox', { name: 'Category' })).toHaveValue('Books'))
})

it('posting a recurring template: regular add dialog + date prefill, submits to post-recurring-transaction (not create-transaction)', async () => {
  const wireRecurringDto: RecurringDto = {
    id: 'r1', ownerUserId: 'u1', type: 'expense', accountId: 'a1', accountRecipientId: null,
    amount: '42.5', categoryId: 'cat-food', payeeId: null, tagId: null, labelIds: [], description: 'rent',
    schedule: 'monthly', nextPaymentAt: '2026-07-05 00:00:00',
  }
  let createCalled = false
  let postBody: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/transaction/create-transaction', () => {
      createCalled = true
      return HttpResponse.json({ success: true, message: '', data: { item: wireTxEcho(), accounts: fixtureAccounts } })
    }),
    http.post('*/api/v1/recurring/post-recurring-transaction', async ({ request }) => {
      postBody = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({
        success: true, message: '',
        data: {
          item: wireTxEcho({ id: 't-posted', date: wireRecurringDto.nextPaymentAt }),
          accounts: fixtureAccounts,
          nextPaymentAt: '2026-08-05 00:00:00',
        },
      })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({ postRecurring: wireRecurringDto })

  // posting reads as the ordinary add dialog — the template only prefills it
  await screen.findByRole('heading', { name: 'Add transaction' })
  expect(screen.getByRole('button', { name: 'date' })).toHaveTextContent('2026-07-05')
  // the account list hasn't resolved yet at the moment the form seeds its
  // initial state (TransactionForm only mounts once the modal opens), so the
  // amount echoes normalizeNumber's un-padded value rather than the account's
  // fraction digits — same limitation the edit-mode seed already has
  expect(screen.getByLabelText('Amount')).toHaveValue('42.5')

  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(postBody).toBeDefined())
  expect(postBody!.recurringId).toBe('r1')
  expect(postBody!.type).toBe('expense')
  expect(postBody!.accountId).toBe('a1')
  expect(createCalled).toBe(false)
})

it('read-only shared accounts are disabled in the transfer account pickers', async () => {
  const partner = { id: 'u2', avatar: 'pets:sky', name: 'Partner' }
  const readOnlyShared = {
    id: 'a-ro', owner: partner, folderId: null, name: 'Partner cash', position: 9,
    currency: fixtureUsd, balance: '50', type: 1, icon: 'wallet',
    sharedAccess: [{ user: fixtureOwner, role: 'guest', isAccepted: 1 }],
  }
  server.use(...coreHandlers({ accounts: [...fixtureAccounts, readOnlyShared] }))
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({ type: 'transfer' })
  await screen.findByRole('heading', { name: 'Add transaction' })

  const toPicker = () => screen.getByRole('combobox', { name: 'to account' }) as HTMLInputElement
  await user.click(toPicker())
  const locked = await screen.findByRole('option', { name: /Partner cash/ })
  expect(locked).toHaveAttribute('aria-disabled', 'true')
  await user.click(locked)
  expect(toPicker().value).not.toContain('Partner cash')

  // a writable account remains selectable
  await user.click(await screen.findByRole('option', { name: /Bank/ }))
  await waitFor(() => expect(toPicker().value).toContain('Bank'))
})


it('editing a transaction round-trips its existing labels', async () => {
  // update-transaction REPLACES the label set: an edit that forgets to resend
  // the attached ids deletes them, which is what this asserts against
  const seen = captureUpdate()
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({ transaction: wireTxEcho({ labelIds: ['label1'] }) as unknown as TransactionDto })

  await screen.findByRole('heading', { name: 'Edit transaction' })
  await waitFor(() => expect(chip('health', 'label')).toHaveAttribute('aria-checked', 'true'))
  // an edit that touches something else entirely must not disturb the labels
  await user.type(screen.getByLabelText('Notes'), 'x')
  await user.click(screen.getByRole('button', { name: 'Update' }))

  await waitFor(() => expect(seen.body).toBeDefined())
  expect(seen.body!.labelIds).toEqual(['label1'])
  expect(seen.body!.description).toBe('x')
})

it('toggling label chips adds and removes ids independently of the tag', async () => {
  server.use(...coreHandlers({ labels: [...fixtureLabels, wireLabel()] }))
  const seen = captureUpdate()
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({ transaction: wireTxEcho({ labelIds: ['label1'] }) as unknown as TransactionDto })

  await screen.findByRole('heading', { name: 'Edit transaction' })
  await waitFor(() => expect(chip('travel', 'label')).toBeInTheDocument())
  await user.click(chip('travel', 'label'))
  expect(chip('travel', 'label')).toHaveAttribute('aria-checked', 'true')
  // both stay on: labels are a free multi-select, unlike the radio-like tag
  expect(chip('health', 'label')).toHaveAttribute('aria-checked', 'true')
  await user.click(chip('health', 'label'))
  await user.click(chip('vacation', 'tag'))
  await user.click(screen.getByRole('button', { name: 'Update' }))

  await waitFor(() => expect(seen.body).toBeDefined())
  expect(seen.body!.labelIds).toEqual(['label2'])
  expect(seen.body!.tagId).toBe('tag1')
})

it('keeps an attached archived label on the row, hides an unattached one', async () => {
  server.use(
    ...coreHandlers({
      labels: [
        ...fixtureLabels,
        wireLabel({ id: 'label-old', name: 'retired', isArchived: 1 }),
        wireLabel({ id: 'label-gone', name: 'unused', isArchived: 1 }),
      ],
    }),
  )
  const seen = captureUpdate()
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({ transaction: wireTxEcho({ labelIds: ['label-old'] }) as unknown as TransactionDto })

  await screen.findByRole('heading', { name: 'Edit transaction' })
  // archived but attached: it must stay visible AND survive the save, since
  // hiding it would silently detach it on the next write
  await waitFor(() => expect(chip('retired', 'label')).toHaveAttribute('aria-checked', 'true'))
  expect(screen.queryByRole('checkbox', { name: /^unused\b/ })).not.toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(seen.body).toBeDefined())
  expect(seen.body!.labelIds).toEqual(['label-old'])
})

it('a transfer posts an empty label set', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/transaction/update-transaction', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: wireTxEcho(), accounts: fixtureAccounts } })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({ transaction: wireTxEcho({ labelIds: ['label1'] }) as unknown as TransactionDto })
  await screen.findByRole('heading', { name: 'Edit transaction' })
  await user.click(screen.getByRole('radio', { name: 'Transfer' }))
  await user.click(screen.getByRole('combobox', { name: 'to account' }))
  await user.click(await screen.findByText(/Bank/))
  await user.click(screen.getByRole('button', { name: 'Update' }))

  await waitFor(() => expect(body).toBeDefined())
  expect(body!.labelIds).toEqual([])
  expect(body!.tagId).toBeNull()
})

it('the inline create dialog picks the label kind and attaches the new label', async () => {
  let created: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/label/create-label', async ({ request }) => {
      created = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: wireLabel({ id: 'label-new', name: 'Books' }) } })
    }),
  )
  const seen = captureUpdate()
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({ transaction: wireTxEcho() as unknown as TransactionDto })

  await screen.findByRole('heading', { name: 'Edit transaction' })
  // the button only appears once the user query resolves (canChangeAccountData)
  await user.click(await screen.findByRole('button', { name: 'Add tag' }))
  // reporting is the default kind, so the dialog needs no kind click
  await user.type(await screen.findByLabelText('Name'), 'Books')
  await user.click(screen.getByRole('button', { name: 'Create' }))

  await waitFor(() => expect(created).toBeDefined())
  expect(created!.name).toBe('Books')
  await waitFor(() => expect(chip('Books', 'label')).toHaveAttribute('aria-checked', 'true'))

  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(seen.body).toBeDefined())
  expect(seen.body!.labelIds).toEqual(['label-new'])
})

it('a rejected inline create shows the server message and keeps the dialog open', async () => {
  // both kinds share one name namespace, so creating a reporting tag named
  // after an existing budget tag is rejected by the server, not by this form
  server.use(
    http.post('*/api/v1/label/create-label', () =>
      HttpResponse.json(
        { success: false, message: 'Name is already taken', code: 400, errors: {} },
        { status: 400 },
      ),
    ),
  )
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({ transaction: wireTxEcho() as unknown as TransactionDto })

  await screen.findByRole('heading', { name: 'Edit transaction' })
  await user.click(await screen.findByRole('button', { name: 'Add tag' }))
  await user.type(await screen.findByLabelText('Name'), 'vacation')
  await user.click(screen.getByRole('button', { name: 'Create' }))

  expect(await screen.findByText('Name is already taken')).toBeInTheDocument()
  // the dialog stays open on the still-typed name so the user can correct it
  expect(screen.getByLabelText('Name')).toHaveValue('vacation')
})


it('posting a template sends the chips the user actually left checked', async () => {
  // post-recurring-transaction inherits the template's labels when labelIds is
  // ABSENT, so the dialog must always send the field — otherwise a toggle made
  // here is silently overwritten by the template's set server-side
  const wireRecurringWithLabels: RecurringDto = {
    id: 'r1', ownerUserId: 'u1', type: 'expense', accountId: 'a1', accountRecipientId: null,
    amount: '42.5', categoryId: 'cat-food', payeeId: null, tagId: null, labelIds: ['label1'],
    description: 'rent', schedule: 'monthly', nextPaymentAt: '2026-07-05 00:00:00',
  }
  server.use(...coreHandlers({ labels: [...fixtureLabels, wireLabel()] }))
  let postBody: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/recurring/post-recurring-transaction', async ({ request }) => {
      postBody = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({
        success: true, message: '',
        data: { item: wireTxEcho({ id: 't-posted', labelIds: ['label2'] }), accounts: fixtureAccounts, nextPaymentAt: '2026-08-05 00:00:00' },
      })
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openTransactionModal({ postRecurring: wireRecurringWithLabels })

  await screen.findByRole('heading', { name: 'Add transaction' })
  // the chips start as the template's set
  await waitFor(() => expect(chip('health', 'label')).toHaveAttribute('aria-checked', 'true'))
  await user.click(chip('health', 'label'))
  await user.click(chip('travel', 'label'))
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(postBody).toBeDefined())
  expect(postBody!.recurringId).toBe('r1')
  expect(postBody!.labelIds).toEqual(['label2'])
})
