import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureCategories, fixturePayees, fixtureTags } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { useUiStore } from '@/app/uiStore'
import { DebtDialog } from './DebtDialog'
import { wireDebt } from './testDebt'

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
  const router = createMemoryRouter([{ path: '/', element: <DebtDialog /> }], { initialEntries: ['/'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return queryClient
}

const ok = (data: unknown) => HttpResponse.json({ success: true, message: '', data })

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  mockMatchMedia()
  useUiStore.setState({ debtModal: null })
})

async function openEdit(debt = wireDebt) {
  useUiStore.getState().openDebtModal({ debt })
  await screen.findByRole('heading', { name: 'Debt' })
}

const lastPaymentChip = () => screen.getByRole('button', { name: 'Last payment date' })

it('editing the balance recomputes the last payment date', async () => {
  const user = userEvent.setup()
  renderDialog()
  await openEdit()
  expect(lastPaymentChip()).toHaveTextContent('2027-11-05')

  const balance = screen.getByLabelText('Balance now')
  await user.clear(balance)
  await user.type(balance, '600')

  // six 100 payments from the next due date, 2026-12-05
  expect(lastPaymentChip()).toHaveTextContent('2027-05-05')
  expect(screen.getByLabelText('Payment')).toHaveValue('100')
})

it('editing the payment recomputes the date from the debt interest rate', async () => {
  const user = userEvent.setup()
  renderDialog()
  await openEdit({ ...wireDebt, periodRate: '0.01' })

  const payment = screen.getByLabelText('Payment')
  await user.clear(payment)
  await user.type(payment, '600')

  // 1200 at 1% a period, 600 a payment: 3 payments, Dec, Jan, Feb
  expect(lastPaymentChip()).toHaveTextContent('2027-02-05')
})

it('a payment that never covers the interest says so and keeps the date', async () => {
  const user = userEvent.setup()
  renderDialog()
  await openEdit({ ...wireDebt, periodRate: '0.05' })

  const payment = screen.getByLabelText('Payment')
  await user.clear(payment)
  await user.type(payment, '10')

  expect(await screen.findByText(/never pays off/)).toBeInTheDocument()
  expect(lastPaymentChip()).toHaveTextContent('2027-11-05')
})

it('an unusable balance or payment is not called a payment that never covers the interest', async () => {
  const user = userEvent.setup()
  renderDialog()
  await openEdit({ ...wireDebt, periodRate: '0.05' })

  const balance = screen.getByLabelText('Balance now')
  await user.clear(balance)
  await user.type(balance, '-100')
  const payment = screen.getByLabelText('Payment')
  await user.clear(payment)
  await user.type(payment, '0')

  expect(screen.queryByText(/never pays off/)).toBeNull()
})

it('editing the last payment date leaves the other fields', async () => {
  const user = userEvent.setup()
  renderDialog()
  await openEdit({ ...wireDebt, currentBalance: '900' })

  await user.click(lastPaymentChip())
  await user.click(await screen.findByRole('button', { name: /November 20th, 2027/ }))

  expect(lastPaymentChip()).toHaveTextContent('2027-11-20')
  expect(screen.getByLabelText('Balance now')).toHaveValue('900')
  expect(screen.getByLabelText('Payment')).toHaveValue('100')
  expect(screen.getByRole('combobox', { name: 'Repeats' })).toHaveTextContent('Monthly')
})

it('saving an untouched debt sends the stored snapshot, not the balance shown now', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/debt/update-debt', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return ok(wireDebt)
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  await openEdit({ ...wireDebt, currentBalance: '900' })

  const name = screen.getByLabelText('Name')
  await user.clear(name)
  await user.type(name, 'Auto loan')
  await user.click(screen.getByRole('button', { name: 'Update' }))

  await waitFor(() => expect(body).toBeDefined())
  expect(body).toMatchObject({ id: 'd1', name: 'Auto loan', balance: '1200', payment: '100', schedule: 'monthly', lastPaymentDate: '2027-11-05', startDate: null })
  await waitFor(() => expect(useUiStore.getState().debtModal).toBeNull())
})

it('a balance typed and put back is untouched: no new snapshot', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/debt/update-debt', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return ok(wireDebt)
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  // payments since the snapshot: the projection of what is owed now ends
  // earlier than the stored last payment date
  await openEdit({ ...wireDebt, currentBalance: '900' })

  const balance = screen.getByLabelText('Balance now')
  await user.clear(balance)
  await user.type(balance, '500')
  await user.clear(balance)
  await user.type(balance, '900')
  expect(lastPaymentChip()).toHaveTextContent('2027-11-05')
  await user.click(screen.getByRole('button', { name: 'Update' }))

  await waitFor(() => expect(body).toBeDefined())
  expect(body).toMatchObject({ balance: '1200', payment: '100', schedule: 'monthly', lastPaymentDate: '2027-11-05' })
})

it('a changed balance is sent as typed', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/debt/update-debt', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return ok(wireDebt)
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  await openEdit()

  const balance = screen.getByLabelText('Balance now')
  await user.clear(balance)
  await user.type(balance, '600')
  await user.click(screen.getByRole('button', { name: 'Update' }))

  await waitFor(() => expect(body).toBeDefined())
  expect(body).toMatchObject({ balance: '600', payment: '100', lastPaymentDate: '2027-05-05' })
})

it('creates a debt from a category with the name and currency prefilled', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/debt/create-debt', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return ok(wireDebt)
    }),
  )
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openDebtModal({ categoryId: 'cat-food', currencyId: 'cur-eur', fromBudget: true })
  await screen.findByRole('heading', { name: 'Debt' })
  expect(screen.getByLabelText('Name')).toHaveValue('Food')

  await user.type(screen.getByLabelText('Balance now'), '500')
  await user.type(screen.getByLabelText('Payment'), '50')
  await user.click(lastPaymentChip())
  const now = new Date()
  const month = new Intl.DateTimeFormat('en', { month: 'long' }).format(now)
  await user.click(await screen.findByRole('button', { name: new RegExp(`${month} 15th, ${now.getFullYear()}`) }))
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(body).toBeDefined())
  expect(body).toMatchObject({
    name: 'Food',
    currencyId: 'cur-eur',
    categoryId: 'cat-food',
    payeeId: null,
    tagId: null,
    startDate: null,
    balance: '500',
    payment: '50',
    schedule: 'monthly',
    lastPaymentDate: `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-15`,
  })
  expect(body).not.toHaveProperty('id')
  await waitFor(() => expect(useUiStore.getState().debtModal).toBeNull())
})

it('offers expense categories only', async () => {
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openDebtModal({})
  await screen.findByRole('heading', { name: 'Debt' })

  await user.click(screen.getByRole('combobox', { name: 'Category' }))
  expect(await screen.findByText('Food')).toBeInTheDocument()
  expect(screen.queryByText('Salary')).toBeNull()
  expect(screen.queryByText('Old')).toBeNull()
})

it('requires the name, category and amounts before sending', async () => {
  let called = false
  server.use(http.post('*/api/v1/debt/create-debt', () => { called = true; return ok(wireDebt) }))
  const user = userEvent.setup()
  renderDialog()
  useUiStore.getState().openDebtModal({})
  await screen.findByRole('heading', { name: 'Debt' })

  await user.click(screen.getByRole('button', { name: 'Add' }))

  expect((await screen.findAllByText('Required field')).length).toBeGreaterThanOrEqual(4)
  expect(called).toBe(false)
})

it('shows a server refusal under its field', async () => {
  server.use(
    http.post('*/api/v1/debt/update-debt', () =>
      HttpResponse.json(
        { success: false, message: 'Form validation error', code: 400, errors: { payment: ['The payments do not cover the balance.'] } },
        { status: 400 },
      ),
    ),
  )
  const user = userEvent.setup()
  renderDialog()
  await openEdit()

  await user.click(screen.getByRole('button', { name: 'Update' }))

  expect(await screen.findByText('The payments do not cover the balance.')).toBeInTheDocument()
  expect(useUiStore.getState().debtModal).not.toBeNull()
})

it('the currency cannot change on an existing debt', async () => {
  renderDialog()
  await openEdit()
  await waitFor(() => expect(screen.getByLabelText('Currency')).toHaveTextContent('USD'))
  expect(screen.getByLabelText('Currency')).toBeDisabled()
})
