import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { onlineManager, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureCategories, fixtureLabels, fixturePayees, fixtureTags } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { useUiStore } from '@/app/uiStore'
import type { OpenDebtParams } from '@/app/uiStore'
import { DebtDialog } from './DebtDialog'
import { wireDebtNotSetUp, wireRuleOptions } from './testDebt'

function mockMatchMedia() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderDialog() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  queryClient.setQueryData(queryKeys.categories, fixtureCategories)
  queryClient.setQueryData(queryKeys.payees, fixturePayees)
  queryClient.setQueryData(queryKeys.tags, fixtureTags)
  queryClient.setQueryData(queryKeys.labels, fixtureLabels)
  const router = createMemoryRouter([{ path: '/', element: <DebtDialog /> }], { initialEntries: ['/'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return queryClient
}

const ok = (data: unknown) => HttpResponse.json({ success: true, message: '', data })

function captureCreate(answer: (body: Record<string, unknown>, call: number) => Response = () => ok({ ...wireDebtNotSetUp, id: 'd-new' })) {
  const bodies: Record<string, unknown>[] = []
  server.use(
    http.post('*/api/v1/debt/create-debt', async ({ request }) => {
      const body = (await request.json()) as Record<string, unknown>
      bodies.push(body)
      return answer(body, bodies.length)
    }),
  )
  return bodies
}

const ruleOptionRequests: string[] = []

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  ruleOptionRequests.length = 0
  server.use(
    ...coreHandlers(),
    http.get('*/api/v1/debt/get-debt-rule-options', ({ request }) => {
      ruleOptionRequests.push(new URL(request.url).searchParams.get('budgetId') ?? '')
      return ok(wireRuleOptions)
    }),
  )
  mockMatchMedia()
  useUiStore.setState({ debtModal: null, debtDetailsId: null })
})

async function openCreate(params: Partial<OpenDebtParams> = {}) {
  useUiStore.getState().openDebtModal({ budgetId: 'b1', ...params })
  await screen.findByRole('heading', { name: 'Debt' })
}

it('create form asks only name, payments from, payment and repeats', async () => {
  renderDialog()
  await openCreate()

  expect(screen.getByText('Track a loan without an account. You can fine-tune it afterwards.')).toBeInTheDocument()
  expect(screen.getByLabelText('Name')).toBeInTheDocument()
  // the budget is always the caller's: never asked here
  expect(screen.queryByRole('button', { name: /^Budget:/ })).toBeNull()
  expect(await screen.findByRole('button', { name: 'Category: choose a category' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Tag (optional): any tag' })).toBeInTheDocument()
  expect(screen.getByLabelText('Payment')).toBeInTheDocument()
  expect(screen.getByRole('combobox', { name: 'Repeats' })).toHaveTextContent('every month')
  // the currency is the category row's, or the user's: never asked here
  expect(screen.queryByRole('button', { name: /^Currency/ })).toBeNull()
  expect(screen.queryByText('Currency')).toBeNull()

  expect(screen.queryByLabelText('Balance now')).toBeNull()
  expect(screen.queryByRole('button', { name: 'Last payment date' })).toBeNull()
  expect(screen.queryByText('Start date (optional)')).toBeNull()
})

it('the payment is a plain decimal input beside its repeats picker, with no calculator keypad', async () => {
  renderDialog()
  await openCreate()

  const payment = screen.getByLabelText('Payment')
  expect(payment).toHaveAttribute('inputmode', 'decimal')
  expect(document.querySelector('[data-calculator-keypad]')).toBeNull()
  const row = screen.getByRole('group', { name: 'Payment, Repeats' })
  expect(row).toContainElement(payment)
  expect(row).toContainElement(screen.getByRole('combobox', { name: 'Repeats' }))
})

it.each([
  ['2000', '2000'],
  ['2 000', '2000'],
  ['2,000.50', '2000.5'],
  ['1.5', '1.5'],
  ['1,5', '1.5'],
])('a payment typed as %s is sent as %s', async (typed, sent) => {
  const bodies = captureCreate()
  const user = userEvent.setup()
  renderDialog()
  await openCreate({ categoryId: 'cat-food' })

  await user.type(screen.getByLabelText('Payment'), typed)
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0].payment).toBe(sent)
})

it('a payment that is not a number is refused before sending', async () => {
  const bodies = captureCreate()
  const user = userEvent.setup()
  renderDialog()
  await openCreate({ categoryId: 'cat-food' })

  await user.type(screen.getByLabelText('Payment'), '5+5')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  expect(await screen.findByText('Enter a valid number')).toBeInTheDocument()
  expect(bodies).toHaveLength(0)
})

it('converting from a category locks the category and prefills the name', async () => {
  const bodies = captureCreate()
  const user = userEvent.setup()
  renderDialog()
  await openCreate({ categoryId: 'cat-food', currencyId: 'cur-eur', budgetId: 'b1' })

  expect(screen.getByLabelText('Name')).toHaveValue('Food')
  expect(screen.getByRole('button', { name: 'Category: Food' })).toBeDisabled()

  await user.type(screen.getByLabelText('Payment'), '50')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toEqual({
    budgetId: 'b1', name: 'Food', currencyId: 'cur-eur', rules: [{ categoryId: 'cat-food', payeeId: null, tagId: null, labelId: null }],
    startDate: null, payment: '50', schedule: 'monthly',
  })
})

it('from a budget row the budget is the row\'s: no budget picker, the first category locked', async () => {
  const bodies = captureCreate()
  const user = userEvent.setup()
  renderDialog()
  await openCreate({ categoryId: 'cat-food', currencyId: 'cur-usd', budgetId: 'b2' })

  expect(screen.queryByRole('button', { name: /^Budget:/ })).toBeNull()
  expect(screen.getByRole('button', { name: 'Category: Food' })).toBeDisabled()
  await waitFor(() => expect(ruleOptionRequests).toContain('b2'))

  await user.click(screen.getByRole('button', { name: 'Add a category' }))
  await user.click(screen.getByRole('button', { name: 'Category: choose a category' }))
  await user.click(within(await screen.findByRole('dialog', { name: 'Category' })).getByText('Mortgage'))
  await user.type(screen.getByLabelText('Payment'), '50')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toMatchObject({
    budgetId: 'b2',
    rules: [
      { categoryId: 'cat-food', payeeId: null, tagId: null, labelId: null },
      { categoryId: 'cat-mortgage', payeeId: null, tagId: null, labelId: null },
    ],
  })
})

it('the payload carries the given budget and every rule', async () => {
  const bodies = captureCreate()
  const user = userEvent.setup()
  renderDialog()
  await openCreate()

  await waitFor(() => expect(ruleOptionRequests).toContain('b1'))
  await user.type(screen.getByLabelText('Name'), 'House')
  await user.click(screen.getByRole('button', { name: 'Category: choose a category' }))
  await user.click(within(await screen.findByRole('dialog', { name: 'Category' })).getByText('Food'))
  await user.click(screen.getByRole('button', { name: 'Add a category' }))
  await user.click(screen.getByRole('button', { name: 'Category: choose a category' }))
  await user.click(within(await screen.findByRole('dialog', { name: 'Category' })).getByText('Mortgage'))
  await user.type(screen.getByLabelText('Payment'), '50')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toEqual({
    budgetId: 'b1', name: 'House',
    rules: [
      { categoryId: 'cat-food', payeeId: null, tagId: null, labelId: null },
      { categoryId: 'cat-mortgage', payeeId: null, tagId: null, labelId: null },
    ],
    startDate: null, payment: '50', schedule: 'monthly',
  })
})

it('the icon button before the name shows the category\'s icon and sends only a picked one', async () => {
  const bodies = captureCreate()
  const user = userEvent.setup()
  renderDialog()
  await openCreate({ categoryId: 'cat-food' })
  await waitFor(() => expect(ruleOptionRequests).toContain('b1'))

  const iconButton = screen.getByRole('button', { name: 'Icon' })
  expect(iconButton.compareDocumentPosition(screen.getByLabelText('Name')) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  // icons are ligature text
  await waitFor(() => expect(iconButton).toHaveTextContent('restaurant'))
  await user.click(iconButton)
  const picker = await screen.findByRole('listbox', { name: 'Icon' })
  await user.click(within(picker).getByRole('option', { name: 'savings' }))
  await waitFor(() => expect(screen.queryByRole('listbox', { name: 'Icon' })).toBeNull())
  expect(iconButton).toHaveTextContent('savings')

  await user.type(screen.getByLabelText('Payment'), '50')
  await user.click(screen.getByRole('button', { name: 'Add' }))
  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toMatchObject({ icon: 'savings' })
})

it('saving opens the debt\'s review screen', async () => {
  captureCreate()
  const user = userEvent.setup()
  renderDialog()
  await openCreate({ categoryId: 'cat-food' })

  await user.type(screen.getByLabelText('Payment'), '50')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(useUiStore.getState().debtModal).toBeNull())
  expect(useUiStore.getState().debtDetailsId).toBe('d-new')
  expect(useUiStore.getState().debtDetailsTab).toBe('overview')
})

it('a key conflict shows its message under the rules and keeps the start date hidden', async () => {
  const conflict = 'These payment rules could match the same payments. Give them different payees, tags or reporting tags, or, for different debts, different start dates.'
  const bodies = captureCreate(() =>
    HttpResponse.json({ success: false, message: 'Form validation error', code: 400, errors: { rules: [conflict] } }, { status: 400 }),
  )
  const user = userEvent.setup()
  renderDialog()
  await openCreate({ categoryId: 'cat-food' })

  await user.type(screen.getByLabelText('Payment'), '50')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  expect(await screen.findByText(conflict)).toBeInTheDocument()
  expect(bodies).toHaveLength(1)
  expect(screen.queryByRole('button', { name: 'Start date (optional)' })).toBeNull()
  expect(screen.queryByText('Start date (optional)')).toBeNull()
  expect(useUiStore.getState().debtModal).not.toBeNull()
})

it('a start date already taken reveals the start date field and resends with it', async () => {
  const taken = 'Another debt with the same category, payee, tag and reporting tag already starts on this date.'
  const bodies = captureCreate((_, call) =>
    call === 1
      ? HttpResponse.json({ success: false, message: 'Form validation error', code: 400, errors: { startDate: [taken] } }, { status: 400 })
      : ok({ ...wireDebtNotSetUp, id: 'd-new' }),
  )
  const user = userEvent.setup()
  renderDialog()
  await openCreate({ categoryId: 'cat-food' })

  await user.type(screen.getByLabelText('Payment'), '50')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  expect(await screen.findByText(taken)).toBeInTheDocument()
  expect(useUiStore.getState().debtModal).not.toBeNull()

  await user.click(screen.getByRole('button', { name: 'Start date' }))
  const now = new Date()
  const month = new Intl.DateTimeFormat('en', { month: 'long' }).format(now)
  await user.click(await screen.findByRole('button', { name: new RegExp(`${month} 1st, ${now.getFullYear()}`) }))
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(bodies).toHaveLength(2))
  expect(bodies[0].startDate).toBeNull()
  expect(bodies[1].startDate).toBe(`${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-01`)
  await waitFor(() => expect(useUiStore.getState().debtDetailsId).toBe('d-new'))
})

it('sends no currency when no category row names one: the budget\'s is taken', async () => {
  const bodies = captureCreate()
  const user = userEvent.setup()
  renderDialog()
  await openCreate({ categoryId: 'cat-food' })

  await user.type(screen.getByLabelText('Payment'), '50')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).not.toHaveProperty('currencyId')
})

it('picks a reporting tag in the tag slot and sends it', async () => {
  const bodies = captureCreate()
  const user = userEvent.setup()
  renderDialog()
  await openCreate({ categoryId: 'cat-food' })

  await user.click(screen.getByRole('button', { name: 'Tag (optional): any tag' }))
  const sheet = await screen.findByRole('dialog', { name: 'Tag (optional)' })
  await user.click(within(sheet).getByText('health'))
  await user.type(screen.getByLabelText('Payment'), '50')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0].rules).toEqual([{ categoryId: 'cat-food', payeeId: null, labelId: 'label1', tagId: null }])
})

it('requires the name, category and payment before sending', async () => {
  const bodies = captureCreate()
  const user = userEvent.setup()
  renderDialog()
  await openCreate()

  await user.click(screen.getByRole('button', { name: 'Add' }))

  expect(await screen.findAllByText('Required field')).toHaveLength(3)
  expect(bodies).toHaveLength(0)
})

it('a refused currency reveals the currency row, and the pick is resent', async () => {
  const refused = 'Currency is not available'
  const bodies = captureCreate((_, call) =>
    call === 1
      ? HttpResponse.json({ success: false, message: 'Form validation error', code: 400, errors: { currencyId: [refused] } }, { status: 400 })
      : ok({ ...wireDebtNotSetUp, id: 'd-new' }),
  )
  const user = userEvent.setup()
  renderDialog()
  await openCreate({ categoryId: 'cat-food', currencyId: 'cur-gone' })

  expect(screen.queryByRole('button', { name: /^Currency/ })).toBeNull()
  await user.type(screen.getByLabelText('Payment'), '50')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  expect(await screen.findByText(refused)).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: /^Currency/ }))
  await user.click(within(await screen.findByRole('dialog', { name: 'Currency' })).getByText(/EUR/))
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(bodies).toHaveLength(2))
  expect(bodies[0].currencyId).toBe('cur-gone')
  expect(bodies[1].currencyId).toBe('cur-eur')
  await waitFor(() => expect(useUiStore.getState().debtDetailsId).toBe('d-new'))
})

it('cannot add a debt offline, and says why', async () => {
  renderDialog()
  await openCreate()
  act(() => onlineManager.setOnline(false))
  try {
    expect(screen.getByRole('button', { name: 'Add' })).toBeDisabled()
    expect(screen.getByTestId('offline-notice')).toBeInTheDocument()
  } finally {
    act(() => onlineManager.setOnline(true))
  }
})
