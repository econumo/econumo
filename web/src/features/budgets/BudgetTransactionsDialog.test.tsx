import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureOwner, fixtureWireBudget } from '@/test/fixtures'
import { coerceBudgetFixture } from '@/test/coerceBudget'
import { BudgetElementType, UNCATEGORIZED_ID } from '@/api/dto/budget'
import { useUiStore } from '@/app/uiStore'
import { BudgetTransactionsDialog, type BudgetTransactionsTarget } from './BudgetTransactionsDialog'
import { useBudgetPeriodStore } from './budgetStore'

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
    category: { id: 'cat-food', name: 'Food', icon: 'restaurant' }, payee: null, tag: null,
    spentAt: '2026-07-02 10:00:00',
  },
]

function renderDialog(element: BudgetTransactionsTarget = target) {
  const budget = coerceBudgetFixture(fixtureWireBudget)
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}>
      <BudgetTransactionsDialog budget={budget} element={element} onClose={() => {}} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: {}, foldBudgetId: null })
  useUiStore.setState({ transactionModal: null })
  server.use(
    ...coreHandlers(),
    http.get('*/api/v1/budget/get-transaction-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: wireItems } }),
    ),
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
  await user.click(foreign)
  const edit = await screen.findByRole('button', { name: 'Edit' })
  expect(edit).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Delete' })).toBeDisabled()
  // synthesized preview still carries the wire details (list row + preview card)
  expect(screen.getAllByText('partner spend').length).toBeGreaterThan(1)
})

it('clicking a transaction opens the preview; Edit hands off to the transaction modal', async () => {
  const user = userEvent.setup()
  renderDialog()
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
  renderDialog()
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
