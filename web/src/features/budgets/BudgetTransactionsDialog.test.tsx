import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts, fixtureOwner, fixtureTransactions, fixtureWireBudget } from '@/test/fixtures'
import { formatDate, formatDateTime } from '@/lib/datetime'
import { coerceBudgetFixture } from '@/test/coerceBudget'
import { BudgetElementType, UNCATEGORIZED_ID } from '@/api/dto/budget'
import { queryKeys } from '@/app/queryKeys'
import { useUiStore } from '@/app/uiStore'
import { BudgetTransactionsDialog, TRANSFERS_TARGET_ID, type BudgetTransactionsTarget } from './BudgetTransactionsDialog'
import { useBudgetPeriodStore } from './budgetStore'
import { METRICS, trackEvent } from '@/lib/metrics'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})

const target: BudgetTransactionsTarget = { id: 'cat-food', type: BudgetElementType.CATEGORY, name: 'Food', icon: 'restaurant', currencyId: null }

const wireItems = [
  {
    // same id as fixtureTransactions t1 -> previewable
    id: 't1', author: fixtureOwner, currencyId: 'cur-usd', amount: '9.99', description: 'coffee beans',
    category: { id: 'cat-food', name: 'Food', icon: 'restaurant' }, payee: null, tag: null,
    spentAt: '2026-07-02 09:30:00',
  },
  {
    // a partner's transaction the user cannot see in the plain list -> read-only row
    id: 'tx-foreign', author: fixtureOwner, currencyId: 'cur-usd', amount: '5', description: 'partner spend',
    category: { id: 'cat-food', name: 'Food', icon: 'restaurant' }, payee: { id: 'payee-1', name: 'Corner Shop' }, tag: null,
    spentAt: '2026-07-02 10:00:00',
  },
]

function renderDialog(element: BudgetTransactionsTarget = target) {
  const budget = coerceBudgetFixture(fixtureWireBudget)
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <BudgetTransactionsDialog budget={budget} element={element} onClose={() => {}} />
    </QueryClientProvider>,
  )
  return queryClient
}

// a row is editable only once the plain transaction list (synced after the user and accounts) has it
async function plainListLoaded(queryClient: QueryClient) {
  await waitFor(() => expect(queryClient.getQueryState(queryKeys.transactions)?.status).toBe('success'))
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: {}, foldBudgetId: null })
  useUiStore.setState({ transactionModal: null, recurringModal: null })
  server.use(
    ...coreHandlers(),
    http.get('*/api/v1/budget/get-transaction-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: wireItems } }),
    ),
    // the analytics auth-method probe fired once the user data lands
    http.get('*/api/v1/oauth/get-identity-list', () => HttpResponse.json({ success: true, message: '', data: [] })),
  )
})

it('every row opens the preview; a foreign transaction shows disabled Edit/Delete', async () => {
  const user = userEvent.setup()
  renderDialog()
  const own = await screen.findByTestId('budget-tx-t1')
  expect(own.tagName).toBe('BUTTON')
  expect(own).toHaveTextContent('coffee beans')
  const foreign = screen.getByTestId('budget-tx-tx-foreign')
  expect(foreign.tagName).toBe('BUTTON')
  expect(foreign).toHaveTextContent('partner spendFood · Corner Shop')
  await user.click(foreign)
  const edit = await screen.findByRole('button', { name: 'Edit' })
  expect(edit).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Delete' })).toBeDisabled()
  // synthesized preview still carries the wire details (list row + preview card)
  expect(screen.getAllByText('partner spend').length).toBeGreaterThan(1)
})

it('a row without a note shows its category and payee on one line', async () => {
  server.use(
    http.get('*/api/v1/budget/get-transaction-list', () =>
      HttpResponse.json({
        success: true, message: '',
        data: { items: [{
          id: 'tx-plain', author: fixtureOwner, currencyId: 'cur-usd', amount: '7.48', description: '',
          category: { id: 'cat-food', name: 'Food', icon: 'restaurant' }, payee: { id: 'payee-1', name: 'Corner Shop' },
          tag: null, labelIds: [], spentAt: '2026-07-02 10:00:00',
        }] },
      }),
    ),
  )
  renderDialog()
  const row = await screen.findByTestId('budget-tx-tx-plain')
  expect(row).toHaveTextContent('Food · Corner Shop')
})

it('clicking a transaction opens the preview; Edit hands off to the transaction modal', async () => {
  const user = userEvent.setup()
  await plainListLoaded(renderDialog())
  await user.click(await screen.findByTestId('budget-tx-t1'))
  // preview hero shows the category and the editable footer
  const edit = await screen.findByRole('button', { name: 'Edit' })
  await user.click(edit)
  expect(useUiStore.getState().transactionModal?.transaction?.id).toBe('t1')
})

it('delete flows through the confirm dialog', async () => {
  let deletedId: unknown
  server.use(
    http.post('*/api/v1/transaction/delete-transaction', async ({ request }) => {
      deletedId = ((await request.json()) as { id: string }).id
      return HttpResponse.json({ success: true, message: '', data: { item: wireItems[0], accounts: [] } })
    }),
  )
  const user = userEvent.setup()
  await plainListLoaded(renderDialog())
  await user.click(await screen.findByTestId('budget-tx-t1'))
  await user.click(await screen.findByRole('button', { name: 'Delete' }))
  // the confirm dialog takes over
  await user.click(await screen.findByRole('button', { name: 'Delete' }))
  await vi.waitFor(() => expect(deletedId).toBe('t1'))
})

// The three cases below intercept the actual outgoing request and assert on its
// query string, rather than a mock's call arguments, so a regression in the
// params-construction spread (not just the on-click parent shape) is caught.
function captureTransactionListUrl() {
  let capturedUrl: string | undefined
  server.use(
    ...coreHandlers(),
    http.get('*/api/v1/budget/get-transaction-list', ({ request }) => {
      capturedUrl = request.url
      return HttpResponse.json({ success: true, message: '', data: { items: [] } })
    }),
  )
  return () => capturedUrl
}

it('a category listed under a tag requests both categoryId and tagId', async () => {
  const getUrl = captureTransactionListUrl()
  renderDialog({
    id: 'cat-food',
    type: BudgetElementType.CATEGORY,
    name: 'Food',
    icon: 'restaurant',
    currencyId: null,
    parent: { id: 'tag-groceries', type: BudgetElementType.TAG },
  })
  await vi.waitFor(() => expect(getUrl()).toBeDefined())
  const params = new URL(getUrl()!).searchParams
  expect(params.get('categoryId')).toBe('cat-food')
  expect(params.get('tagId')).toBe('tag-groceries')
})

it('a category listed under an envelope requests categoryId only', async () => {
  const getUrl = captureTransactionListUrl()
  renderDialog({
    id: 'cat-food',
    type: BudgetElementType.CATEGORY,
    name: 'Food',
    icon: 'restaurant',
    currencyId: null,
    parent: { id: 'env-fun', type: BudgetElementType.ENVELOPE },
  })
  await vi.waitFor(() => expect(getUrl()).toBeDefined())
  const params = new URL(getUrl()!).searchParams
  expect(params.get('categoryId')).toBe('cat-food')
  expect(params.has('tagId')).toBe(false)
})

it('a top-level tag row requests tagId only, not clobbered by a parent spread', async () => {
  const getUrl = captureTransactionListUrl()
  renderDialog({
    id: 'tag-groceries',
    type: BudgetElementType.TAG,
    name: 'Groceries',
    icon: 'local_grocery_store',
    currencyId: null,
  })
  await vi.waitFor(() => expect(getUrl()).toBeDefined())
  const params = new URL(getUrl()!).searchParams
  expect(params.get('tagId')).toBe('tag-groceries')
  expect(params.has('categoryId')).toBe(false)
})

it('the top-level Uncategorized target requests uncategorized=1 and no categoryId', async () => {
  const getUrl = captureTransactionListUrl()
  renderDialog({ id: UNCATEGORIZED_ID, type: BudgetElementType.CATEGORY, name: 'Uncategorized', icon: 'question_mark', currencyId: null })
  await vi.waitFor(() => expect(getUrl()).toBeDefined())
  const params = new URL(getUrl()!).searchParams
  expect(params.get('uncategorized')).toBe('1')
  expect(params.has('categoryId')).toBe(false)
})

it('a label target requests labelId only -- never combined with categoryId/tagId/envelopeId/uncategorized', async () => {
  const getUrl = captureTransactionListUrl()
  renderDialog({ id: 'l1', type: 'label', name: 'Kid A', icon: 'label', currencyId: null })
  await vi.waitFor(() => expect(getUrl()).toBeDefined())
  const params = new URL(getUrl()!).searchParams
  expect(params.get('labelId')).toBe('l1')
  // the backend 400s on any combination with another filter, so a broken query
  // string here would be a production regression, not just a UI nit
  expect(params.has('categoryId')).toBe(false)
  expect(params.has('tagId')).toBe(false)
  expect(params.has('envelopeId')).toBe(false)
  expect(params.has('uncategorized')).toBe(false)
  expect(Array.from(params.keys()).sort()).toEqual(['budgetId', 'labelId', 'periodStart'])
})

it("a label's category child requests labelId and categoryId together", async () => {
  const getUrl = captureTransactionListUrl()
  renderDialog({
    id: 'cat-groceries',
    type: BudgetElementType.CATEGORY,
    name: 'Groceries',
    icon: 'local_grocery_store',
    currencyId: null,
    parent: { id: 'label-kid-a', type: 'label' },
  })
  await vi.waitFor(() => expect(getUrl()).toBeDefined())
  const params = new URL(getUrl()!).searchParams
  expect(params.get('labelId')).toBe('label-kid-a')
  expect(params.get('categoryId')).toBe('cat-groceries')
  // labelId still 400s alongside tagId/envelopeId, so only the category pairs with it
  expect(params.has('tagId')).toBe(false)
  expect(params.has('envelopeId')).toBe(false)
  expect(params.has('uncategorized')).toBe(false)
})

it("a label's uncategorized child requests labelId and uncategorized=1, and no categoryId", async () => {
  const getUrl = captureTransactionListUrl()
  renderDialog({
    id: UNCATEGORIZED_ID,
    type: BudgetElementType.CATEGORY,
    name: 'Uncategorized',
    icon: 'question_mark',
    currencyId: null,
    parent: { id: 'label-kid-a', type: 'label' },
  })
  await vi.waitFor(() => expect(getUrl()).toBeDefined())
  const params = new URL(getUrl()!).searchParams
  expect(params.get('labelId')).toBe('label-kid-a')
  expect(params.get('uncategorized')).toBe('1')
  expect(params.has('categoryId')).toBe(false)
  expect(params.has('tagId')).toBe(false)
})

it("a tag's Uncategorized child requests uncategorized=1 and tagId, and no categoryId", async () => {
  const getUrl = captureTransactionListUrl()
  renderDialog({
    id: UNCATEGORIZED_ID,
    type: BudgetElementType.CATEGORY,
    name: 'Uncategorized',
    icon: 'question_mark',
    currencyId: null,
    parent: { id: 'tag-x', type: BudgetElementType.TAG },
  })
  await vi.waitFor(() => expect(getUrl()).toBeDefined())
  const params = new URL(getUrl()!).searchParams
  expect(params.get('uncategorized')).toBe('1')
  expect(params.get('tagId')).toBe('tag-x')
  expect(params.has('categoryId')).toBe(false)
})

it("a partner's row previews with its reporting tags", async () => {
  // not in the caller's own transaction list, so the preview is SYNTHESIZED
  // from the budget wire — which now carries labelIds
  server.use(
    http.get('*/api/v1/budget/get-transaction-list', () =>
      HttpResponse.json({
        success: true, message: '',
        data: { items: [{
          id: 'tx-foreign', author: fixtureOwner, currencyId: 'cur-usd', amount: '5',
          description: 'partner spend', category: null, payee: null, tag: null,
          labelIds: ['label1'], spentAt: '2026-07-02 10:00:00',
        }] },
      }),
    ),
  )
  const user = userEvent.setup()
  renderDialog()
  await user.click(await screen.findByRole('button', { name: /partner spend/ }))
  // 'health' is the fixture label behind label1
  expect(await screen.findByText('health')).toBeInTheDocument()
})

it('a transfers target requests transfers=1 alone, and periodStart overrides the store period', async () => {
  const getUrl = captureTransactionListUrl()
  const budget = coerceBudgetFixture(fixtureWireBudget)
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}>
      <BudgetTransactionsDialog
        budget={budget}
        element={{ id: TRANSFERS_TARGET_ID, type: 'transfers', name: 'Transfers', icon: 'sync_alt', currencyId: null }}
        periodStart="2026-06-01"
        onClose={() => {}}
      />
    </QueryClientProvider>,
  )
  await vi.waitFor(() => expect(getUrl()).toBeDefined())
  const params = new URL(getUrl()!).searchParams
  expect(params.get('transfers')).toBe('1')
  expect(params.get('periodStart')).toBe('2026-06-01') // not the store's 2026-07-01
  for (const key of ['categoryId', 'tagId', 'envelopeId', 'labelId', 'uncategorized']) {
    expect(params.has(key)).toBe(false)
  }
})

it('transfer rows are signed by direction: out negative, in positive; a foreign transfer previews as a transfer', async () => {
  server.use(
    ...coreHandlers(),
    http.get('*/api/v1/budget/get-transaction-list', () =>
      HttpResponse.json({
        success: true,
        message: '',
        data: {
          items: [
            {
              id: 'tx-out', author: fixtureOwner, currencyId: 'cur-usd', amount: '150', description: 'to savings',
              category: null, payee: null, tag: null, labelIds: [], spentAt: '2026-06-03 09:00:00', direction: 'out',
            },
            {
              id: 'tx-in', author: fixtureOwner, currencyId: 'cur-usd', amount: '50', description: 'from savings',
              category: null, payee: null, tag: null, labelIds: [], spentAt: '2026-06-02 09:00:00', direction: 'in',
            },
          ],
        },
      }),
    ),
  )
  const user = userEvent.setup()
  renderDialog({ id: TRANSFERS_TARGET_ID, type: 'transfers', name: 'Transfers', icon: 'sync_alt', currencyId: null })
  const out = await screen.findByTestId('budget-tx-tx-out')
  const inn = screen.getByTestId('budget-tx-tx-in')
  expect(out).toHaveTextContent('-150.00')
  expect(inn).toHaveTextContent('50.00')
  expect(inn).not.toHaveTextContent('-50.00')

  // neither row is in the user's own list -> the read-only preview is a transfer
  // (sender AND recipient cards), not an expense
  await user.click(out)
  await screen.findByRole('button', { name: 'Edit' })
  expect(screen.getByText('Sender')).toBeInTheDocument()
  expect(screen.getByText('Recipient')).toBeInTheDocument()
})

it('a savings target requests accountId alone', async () => {
  const getUrl = captureTransactionListUrl()
  renderDialog({ id: 'acc-savings', type: BudgetElementType.SAVINGS, name: 'Emergency fund', icon: 'savings', currencyId: 'cur-usd' })
  await vi.waitFor(() => expect(getUrl()).toBeDefined())
  const params = new URL(getUrl()!).searchParams
  expect(params.get('accountId')).toBe('acc-savings')
  for (const key of ['categoryId', 'tagId', 'envelopeId', 'labelId', 'uncategorized', 'transfers', 'income']) {
    expect(params.has(key)).toBe(false)
  }
})

it('an income category requests income=1 with categoryId', async () => {
  const getUrl = captureTransactionListUrl()
  renderDialog({ id: 'cat-salary', type: BudgetElementType.INCOME_CATEGORY, name: 'Salary', icon: 'payments', currencyId: null })
  await vi.waitFor(() => expect(getUrl()).toBeDefined())
  const params = new URL(getUrl()!).searchParams
  expect(params.get('income')).toBe('1')
  expect(params.get('categoryId')).toBe('cat-salary')
  expect(params.has('envelopeId')).toBe(false)
})

it('an income envelope requests income=1 with envelopeId only', async () => {
  const getUrl = captureTransactionListUrl()
  renderDialog({ id: 'env-work', type: BudgetElementType.INCOME_ENVELOPE, name: 'Work', icon: 'work', currencyId: null })
  await vi.waitFor(() => expect(getUrl()).toBeDefined())
  const params = new URL(getUrl()!).searchParams
  expect(params.get('income')).toBe('1')
  expect(params.get('envelopeId')).toBe('env-work')
  expect(params.has('categoryId')).toBe(false)
})

it('savings rows are signed by direction, and a foreign row previews as the kind the wire names', async () => {
  server.use(
    ...coreHandlers(),
    http.get('*/api/v1/budget/get-transaction-list', () =>
      HttpResponse.json({
        success: true,
        message: '',
        data: {
          items: [
            {
              id: 'tx-interest', author: fixtureOwner, currencyId: 'cur-usd', amount: '12', description: 'interest',
              category: null, payee: null, tag: null, labelIds: [], spentAt: '2026-07-03 09:00:00', direction: 'in', type: 'income',
            },
            {
              id: 'tx-fee', author: fixtureOwner, currencyId: 'cur-usd', amount: '5', description: 'fee',
              category: null, payee: null, tag: null, labelIds: [], spentAt: '2026-07-02 09:00:00', direction: 'out', type: 'expense',
            },
          ],
        },
      }),
    ),
  )
  const user = userEvent.setup()
  renderDialog({ id: 'acc-savings', type: BudgetElementType.SAVINGS, name: 'Emergency fund', icon: 'savings', currencyId: 'cur-usd' })
  const interest = await screen.findByTestId('budget-tx-tx-interest')
  expect(interest).toHaveTextContent('12.00')
  expect(interest).not.toHaveTextContent('-12.00')
  expect(screen.getByTestId('budget-tx-tx-fee')).toHaveTextContent('-5.00')

  // an income, not a transfer: no Sender/Recipient cards
  await user.click(interest)
  await screen.findByRole('button', { name: 'Edit' })
  expect(screen.queryByText('Sender')).not.toBeInTheDocument()
})

it('opening a list fires the analytics event with its kind', async () => {
  vi.mocked(trackEvent).mockClear()
  renderDialog({ id: 'acc-savings', type: BudgetElementType.SAVINGS, name: 'Emergency fund', icon: 'savings', currencyId: 'cur-usd' })
  await vi.waitFor(() => expect(trackEvent).toHaveBeenCalledWith(METRICS.BUDGET_TRANSACTIONS_OPEN, { kind: 'savings' }))
  expect(trackEvent).toHaveBeenCalledTimes(1)
})

const ownTemplate = {
  id: 'r-gym', ownerUserId: 'u1', type: 'expense', accountId: 'a1', accountRecipientId: null,
  amount: '10', categoryId: 'cat-food', payeeId: null, tagId: null, labelIds: [], description: 'gym',
  schedule: 'weekly', nextPaymentAt: '2026-07-08 09:00:00',
}

function upcomingItem(overrides: Record<string, unknown> = {}) {
  return {
    id: 'r-gym', author: fixtureOwner, currencyId: 'cur-usd', amount: '10', description: 'gym',
    category: { id: 'cat-food', name: 'Food', icon: 'restaurant' }, payee: null, tag: null, labelIds: [],
    spentAt: '2026-07-08 09:00:00', templateId: 'r-gym', schedule: 'weekly', accountId: 'a1', accountName: 'Cash',
    labelNames: [], isNext: true, isOverdue: false, canEdit: true, ...overrides,
  }
}

function serveUpcoming(upcoming: unknown[], recurring: unknown[] = [ownTemplate], items: unknown[] = []) {
  server.use(
    ...coreHandlers({ recurring }),
    http.get('*/api/v1/budget/get-transaction-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items, upcoming } }),
    ),
    http.get('*/api/v1/oauth/get-identity-list', () => HttpResponse.json({ success: true, message: '', data: [] })),
  )
}

function capturePost() {
  let posted: { date?: string; recurringId?: string } | undefined
  server.use(
    http.post('*/api/v1/recurring/post-recurring-transaction', async ({ request }) => {
      posted = (await request.json()) as typeof posted
      return HttpResponse.json({
        success: true, message: '',
        data: { item: { ...fixtureTransactions[0], id: 'posted' }, accounts: fixtureAccounts, nextPaymentAt: '2099-01-01 09:00:00' },
      })
    }),
  )
  return () => posted
}

async function recurringListLoaded(queryClient: QueryClient) {
  await waitFor(() => expect(queryClient.getQueryState(queryKeys.recurring)?.status).toBe('success'))
}

it('shows weekly upcoming rows in date groups', async () => {
  serveUpcoming(
    [
      upcomingItem({ spentAt: '2026-07-15 09:00:00', isNext: false }),
      upcomingItem(),
      upcomingItem({ spentAt: '2026-07-22 09:00:00', isNext: false }),
    ],
    [ownTemplate],
    [wireItems[0]],
  )
  renderDialog()
  const rows = await screen.findAllByTestId(/^budget-(tx|upcoming)-/)
  expect(rows.map((r) => r.getAttribute('data-testid'))).toEqual([
    'budget-upcoming-r-gym-2026-07-22',
    'budget-upcoming-r-gym-2026-07-15',
    'budget-upcoming-r-gym-2026-07-08',
    'budget-tx-t1',
  ])
  // every occurrence sits under its own day heading, like a real row
  for (const heading of ['22nd July 2026', '15th July 2026', '8th July 2026', '2nd July 2026']) {
    expect(screen.getByText(heading)).toBeInTheDocument()
  }
  const upcoming = screen.getByTestId('budget-upcoming-r-gym-2026-07-08')
  expect(upcoming).toHaveTextContent('gym')
  expect(upcoming).toHaveTextContent('-10.00')
  expect(upcoming.querySelector('[aria-label="Recurring transaction"]')).not.toBeNull()
  expect(upcoming.className).toContain('opacity-50')
})

it('dims a real transaction dated in the future', async () => {
  const future = formatDateTime(new Date(Date.now() + 10 * 24 * 3600 * 1000))
  serveUpcoming([], [], [{ ...wireItems[0], spentAt: future }, wireItems[1]])
  renderDialog()
  expect((await screen.findByTestId('budget-tx-t1')).className).toContain('opacity-50')
  expect(screen.getByTestId('budget-tx-tx-foreign').className).not.toContain('opacity-50')
})

it('marks an overdue payment with its due date', async () => {
  serveUpcoming([upcomingItem({ spentAt: '2026-06-28 09:00:00', isOverdue: true })])
  renderDialog()
  expect(await screen.findByTestId('budget-upcoming-r-gym-2026-06-28')).toHaveTextContent('Overdue · Jun 28')
})

it('next payment offers post and skip', async () => {
  serveUpcoming([upcomingItem()])
  let skipped: unknown
  server.use(
    http.post('*/api/v1/recurring/skip-recurring-transaction', async ({ request }) => {
      skipped = ((await request.json()) as { id: string }).id
      return HttpResponse.json({ success: true, message: '', data: { item: { ...ownTemplate, nextPaymentAt: '2026-07-15 09:00:00' } } })
    }),
  )
  const user = userEvent.setup()
  await recurringListLoaded(renderDialog())
  await user.click(await screen.findByTestId('budget-upcoming-r-gym-2026-07-08'))
  expect(await screen.findByRole('button', { name: 'Post' })).toBeEnabled()
  await user.click(screen.getByRole('button', { name: 'Skip' }))
  await vi.waitFor(() => expect(skipped).toBe('r-gym'))
})

it('later cycle has no post or skip', async () => {
  serveUpcoming([upcomingItem({ spentAt: '2026-07-15 09:00:00', isNext: false })])
  const user = userEvent.setup()
  await recurringListLoaded(renderDialog())
  await user.click(await screen.findByTestId('budget-upcoming-r-gym-2026-07-15'))
  expect(await screen.findByText('Can be posted after the earlier payment')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Post' })).toBeNull()
  expect(screen.queryByRole('button', { name: 'Skip' })).toBeNull()
  // the template is the caller's own, so its editor stays one tap away
  await user.click(screen.getByRole('button', { name: 'Edit' }))
  expect(useUiStore.getState().recurringModal?.recurring?.id).toBe('r-gym')
})

it('renders partner template read-only', async () => {
  serveUpcoming(
    [upcomingItem({ id: 'r-partner', templateId: 'r-partner', accountId: 'acc-partner', accountName: 'Partner card', canEdit: false, description: 'internet' })],
    [],
  )
  const user = userEvent.setup()
  await recurringListLoaded(renderDialog())
  await user.click(await screen.findByTestId('budget-upcoming-r-partner-2026-07-08'))
  expect(await screen.findByText('Partner card')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Post' })).toBeNull()
  expect(screen.queryByRole('button', { name: 'Skip' })).toBeNull()
  expect(screen.queryByRole('button', { name: 'Edit' })).toBeNull()
  expect(screen.queryByText('Can be posted after the earlier payment')).toBeNull()
})

it('a payment the caller may edit but cannot see in their recurring list is read-only', async () => {
  serveUpcoming([upcomingItem({ id: 'r-other', templateId: 'r-other' })], [])
  const user = userEvent.setup()
  await recurringListLoaded(renderDialog())
  await user.click(await screen.findByTestId('budget-upcoming-r-other-2026-07-08'))
  expect(await screen.findByText('Cash')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Post' })).toBeNull()
  expect(screen.queryByRole('button', { name: 'Edit' })).toBeNull()
})

it('post from budget uses scheduled date when in future', async () => {
  const due = formatDateTime(new Date(Date.now() + 10 * 24 * 3600 * 1000))
  serveUpcoming([upcomingItem({ spentAt: due })], [{ ...ownTemplate, nextPaymentAt: due }])
  const getPosted = capturePost()
  const user = userEvent.setup()
  await recurringListLoaded(renderDialog())
  await user.click(await screen.findByTestId(`budget-upcoming-r-gym-${due.split(' ')[0]}`))
  await user.click(await screen.findByRole('button', { name: 'Post' }))
  await vi.waitFor(() => expect(getPosted()?.date).toBe(due))
  expect(getPosted()?.recurringId).toBe('r-gym')
})

it('post overdue uses now', async () => {
  serveUpcoming([upcomingItem({ spentAt: '2026-06-28 09:00:00', isOverdue: true })], [{ ...ownTemplate, nextPaymentAt: '2026-06-28 09:00:00' }])
  const getPosted = capturePost()
  const user = userEvent.setup()
  await recurringListLoaded(renderDialog())
  await user.click(await screen.findByTestId('budget-upcoming-r-gym-2026-06-28'))
  await user.click(await screen.findByRole('button', { name: 'Post' }))
  await vi.waitFor(() => expect(getPosted()).toBeDefined())
  expect(getPosted()?.date?.split(' ')[0]).toBe(formatDate(new Date()))
})
