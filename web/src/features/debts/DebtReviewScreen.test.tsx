import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { useUiStore } from '@/app/uiStore'
import { METRICS, trackEvent } from '@/lib/metrics'
import type { DebtDetailsDto } from '@/api/dto/debt'
import { DebtReviewScreen } from './DebtReviewScreen'
import { wireDebt, wireDebtNotSetUp } from './testDebt'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})

function mockMatchMedia(phone = false) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: phone, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderScreen() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/', element: <DebtReviewScreen /> }], { initialEntries: ['/'] })
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

const ok = (data: unknown) => HttpResponse.json({ success: true, message: '', data })

const details: DebtDetailsDto = {
  item: {
    ...wireDebt,
    currentBalance: '1090',
    paymentsLeft: 283,
    periodsPaid: 77,
    moneyLeft: '1100',
    overpaymentLeft: '10',
    overpaidSoFar: '15.5',
    payoffDate: '2050-07-05',
    lastPaymentDate: '2050-07-05',
    nextDueDate: '2026-12-05',
  },
  snapshots: [{ asOf: '2026-10-01', balance: '1200', payment: '100', schedule: 'monthly', lastPaymentDate: '2027-11-05' }],
  payments: [
    { transactionId: 't1', date: '2026-11-05 00:00:00', amount: '100', interest: '10', principal: '90' },
    { transactionId: 't2', date: '2026-10-05 00:00:00', amount: '100', interest: '5.5', principal: '94.5' },
  ],
}

const notSetUp: DebtDetailsDto = {
  item: wireDebtNotSetUp,
  snapshots: [],
  payments: [
    { transactionId: 't1', date: '2026-11-05 00:00:00', amount: '100', interest: '0', principal: '100' },
    { transactionId: 't2', date: '2026-10-05 00:00:00', amount: '50.5', interest: '0', principal: '50.5' },
  ],
}

function captureUpdate(answer: (body: Record<string, unknown>) => unknown = () => wireDebt) {
  const bodies: Record<string, unknown>[] = []
  server.use(
    http.post('*/api/v1/debt/update-debt', async ({ request }) => {
      const body = (await request.json()) as Record<string, unknown>
      bodies.push(body)
      return ok(answer(body))
    }),
  )
  return bodies
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers(), http.get('*/api/v1/debt/get-debt', () => ok(details)))
  mockMatchMedia()
  vi.mocked(trackEvent).mockClear()
  useUiStore.setState({ debtDetailsId: null, debtModal: null })
})

async function openScreen() {
  useUiStore.getState().openDebtDetails('d1')
  await screen.findByRole('heading', { name: 'Car loan' })
  await screen.findByTestId('debt-review-top')
}

async function openMenu(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'Debt actions' }))
  await screen.findByRole('menu')
}

it('set-up debt shows payments left first with an X of Y progress', async () => {
  renderScreen()
  await openScreen()

  const top = screen.getByTestId('debt-review-top')
  expect(within(top).getByTestId('debt-review-payments-left')).toHaveTextContent('283 payments left')
  expect(within(top).getByRole('progressbar')).toBeInTheDocument()
  expect(top).toHaveTextContent('77 of 360 paid')
  expect(top).toHaveTextContent('Owed now 1,090.00 $ · Paid off by Jul 2050')
  // the count leads the screen, before the loan's details
  const loan = screen.getByRole('region', { name: 'Loan' })
  expect(top.compareDocumentPosition(loan) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})

it('hides the progress bar while nothing is paid or left', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok({ ...details, item: { ...details.item, periodsPaid: 0, paymentsLeft: 0 } })))
  renderScreen()
  await openScreen()

  expect(screen.queryByRole('progressbar')).toBeNull()
  expect(screen.getByTestId('debt-review-top')).not.toHaveTextContent('of 0 paid')
})

it('a paid-off debt says so instead of counting payments', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok({ ...details, item: { ...details.item, isPaidOff: 1, paymentsLeft: 0, currentBalance: '0' } })))
  renderScreen()
  await openScreen()

  expect(screen.getByTestId('debt-review-payments-left')).toHaveTextContent('Paid off')
  expect(screen.getByTestId('debt-review-top')).not.toHaveTextContent('Paid off by')
})

it('the Loan card holds every figure of the debt', async () => {
  renderScreen()
  await openScreen()

  const loan = screen.getByRole('region', { name: 'Loan' })
  const figure = (label: string) => within(loan).getByText(label).nextElementSibling
  expect(figure('Owed now')).toHaveTextContent('1,090.00 $')
  expect(figure('Payment')).toHaveTextContent('100.00 $ · Monthly')
  expect(figure('Next payment')).toHaveTextContent('Dec 5, 2026')
  expect(figure('Loan ends')).toHaveTextContent('Jul 5, 2050')
  expect(figure('Start date')).toHaveTextContent('From the first payment')
  expect(figure('Left to pay')).toHaveTextContent('1,100.00 $')
  expect(figure('Overpaid so far')).toHaveTextContent('15.50 $')
  expect(figure('Overpayment still to come')).toHaveTextContent('10.00 $')
  expect(figure('Currency')).toHaveTextContent('USD')
})

it('a debt with a start date shows it in the Loan card', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok({ ...details, item: { ...details.item, startDate: '2026-03-01' } })))
  renderScreen()
  await openScreen()

  const loan = screen.getByRole('region', { name: 'Loan' })
  expect(within(loan).getByText('Start date').nextElementSibling).toHaveTextContent('Mar 1, 2026')
})

it('shows interest and principal per payment, newest first', async () => {
  renderScreen()
  await openScreen()

  expect(screen.getByRole('columnheader', { name: 'Interest' })).toBeInTheDocument()
  expect(screen.getByRole('columnheader', { name: 'Principal' })).toBeInTheDocument()
  const rows = screen.getAllByTestId('debt-payment')
  expect(rows).toHaveLength(2)
  expect(within(rows[0]).getAllByRole('cell').map((c) => c.textContent)).toEqual(['Nov 5, 2026', '100.00 $', '10.00 $', '90.00 $'])
  expect(within(rows[1]).getAllByRole('cell').map((c) => c.textContent)).toEqual(['Oct 5, 2026', '100.00 $', '5.50 $', '94.50 $'])
})

it('the history of balances is collapsed until asked for', async () => {
  const user = userEvent.setup()
  renderScreen()
  await openScreen()

  expect(screen.queryByTestId('debt-snapshot')).toBeNull()
  await user.click(screen.getByRole('button', { name: /History of terms/ }))

  const snapshots = screen.getAllByTestId('debt-snapshot')
  expect(snapshots).toHaveLength(1)
  expect(snapshots[0]).toHaveTextContent('Oct 1, 2026')
  expect(snapshots[0]).toHaveTextContent('1,200.00 $')
})

it('not-set-up debt shows Finish setup and no balance', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok(notSetUp)))
  renderScreen()
  await openScreen()

  const top = screen.getByTestId('debt-review-top')
  expect(top).toHaveTextContent('Paid so far')
  expect(top).toHaveTextContent('150.50 $')
  expect(within(top).getByRole('button', { name: 'Finish setup' })).toBeInTheDocument()
  expect(top).toHaveTextContent('Add what you owe and when the loan ends to see payments left')
  expect(screen.queryByText(/\d+ payments? left/)).toBeNull()
  expect(screen.queryByText(/Owed now/)).toBeNull()
  expect(screen.queryByRole('progressbar')).toBeNull()
  // the payments carry no split before the terms exist
  expect(screen.queryByRole('columnheader', { name: 'Interest' })).toBeNull()
  expect(within(screen.getAllByTestId('debt-payment')[0]).getAllByRole('cell').map((c) => c.textContent)).toEqual(['Nov 5, 2026', '100.00 $'])
})

it('Finish setup saves terms and the screen switches to payments left', async () => {
  let setUp = false
  server.use(http.get('*/api/v1/debt/get-debt', () => ok(setUp ? { ...details, item: { ...details.item, paymentsLeft: 12, periodsPaid: 0 } } : notSetUp)))
  const bodies = captureUpdate(() => {
    setUp = true
    return { ...wireDebt, balance: '600' }
  })
  const user = userEvent.setup()
  renderScreen()
  await openScreen()

  await user.click(within(screen.getByTestId('debt-review-top')).getByRole('button', { name: 'Finish setup' }))
  await user.type(await screen.findByLabelText('Balance now'), '600')
  await user.click(screen.getByRole('button', { name: 'Last payment date' }))
  const now = new Date()
  const month = new Intl.DateTimeFormat('en', { month: 'long' }).format(now)
  await user.click(await screen.findByRole('button', { name: new RegExp(`${month} 15th, ${now.getFullYear()}`) }))
  await user.click(screen.getByRole('button', { name: 'Finish setup' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toMatchObject({ id: 'd1', balance: '600', payment: '100', schedule: 'monthly' })
  expect(trackEvent).toHaveBeenCalledWith(METRICS.DEBT_FINISH_SETUP)
  expect(await screen.findByText('12 payments left')).toBeInTheDocument()
  expect(screen.queryByLabelText('Balance now')).toBeNull()
})

it('Edit opens the terms with the debt as it is now, and Cancel goes back', async () => {
  const user = userEvent.setup()
  renderScreen()
  await openScreen()
  // a payment landed since the screen loaded
  server.use(http.get('*/api/v1/debt/get-debt', () => ok({ ...details, item: { ...details.item, currentBalance: '990' } })))

  await user.click(within(screen.getByRole('region', { name: 'Loan' })).getByRole('button', { name: 'Edit' }))

  expect(await screen.findByLabelText('Balance now')).toHaveValue('990')
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(await screen.findByTestId('debt-review-top')).toBeInTheDocument()
  expect(screen.queryByLabelText('Balance now')).toBeNull()
})

it('editing payments-from saves the label', async () => {
  const bodies = captureUpdate(() => ({ ...details.item, labelId: 'label1' }))
  const user = userEvent.setup()
  renderScreen()
  await openScreen()

  // nothing to save until a pick changes
  expect(screen.queryByRole('button', { name: 'Save' })).toBeNull()
  await user.click(screen.getByRole('button', { name: 'Reporting tag (optional): any reporting tag' }))
  const sheet = await screen.findByRole('dialog', { name: 'Reporting tag (optional)' })
  await user.click(within(sheet).getByText('health'))
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  // the stored terms go back unchanged: no new snapshot
  expect(bodies[0]).toMatchObject({
    id: 'd1', name: 'Car loan', categoryId: 'cat-food', payeeId: null, tagId: null, labelId: 'label1',
    balance: '1200', payment: '100', schedule: 'monthly', lastPaymentDate: '2050-07-05',
  })
  expect(trackEvent).toHaveBeenCalledWith(METRICS.DEBT_UPDATE)
  await waitFor(() => expect(screen.queryByRole('button', { name: 'Save' })).toBeNull())
})

it('a refused payments-from change shows why under the sentence', async () => {
  server.use(
    http.post('*/api/v1/debt/update-debt', () =>
      HttpResponse.json(
        { success: false, message: 'Form validation error', code: 400, errors: { categoryId: ['Another debt already counts these payments.'] } },
        { status: 400 },
      ),
    ),
  )
  const user = userEvent.setup()
  renderScreen()
  await openScreen()

  await user.click(screen.getByRole('button', { name: 'Reporting tag (optional): any reporting tag' }))
  await user.click(within(await screen.findByRole('dialog', { name: 'Reporting tag (optional)' })).getByText('health'))
  await user.click(screen.getByRole('button', { name: 'Save' }))

  expect(await screen.findByText('Another debt already counts these payments.')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Save' })).toBeInTheDocument()
})

it('Rename in the menu saves a new name', async () => {
  const bodies = captureUpdate(() => ({ ...details.item, name: 'Van loan' }))
  const user = userEvent.setup()
  renderScreen()
  await openScreen()

  await openMenu(user)
  await user.click(screen.getByRole('menuitem', { name: 'Rename' }))
  const prompt = await screen.findByRole('dialog', { name: 'Rename' })
  const input = within(prompt).getByLabelText('Name')
  expect(input).toHaveValue('Car loan')
  await user.clear(input)
  await user.type(input, 'Van loan')
  await user.click(within(prompt).getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toMatchObject({ id: 'd1', name: 'Van loan', balance: '1200', lastPaymentDate: '2050-07-05', labelId: null })
})

it('menu hides Mark as paid off while not set up', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok(notSetUp)))
  const user = userEvent.setup()
  renderScreen()
  await openScreen()

  await openMenu(user)
  expect(screen.getAllByRole('menuitem').map((m) => m.textContent)).toEqual(['Rename', 'Delete'])
})

it('menu hides Mark as paid off once the debt is paid off', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok({ ...details, item: { ...details.item, isPaidOff: 1 } })))
  const user = userEvent.setup()
  renderScreen()
  await openScreen()

  await openMenu(user)
  expect(screen.queryByRole('menuitem', { name: 'Mark as paid off' })).toBeNull()
})

it('marking a debt paid off asks first, then posts', async () => {
  let called = 0
  server.use(http.post('*/api/v1/debt/mark-debt-paid-off', () => { called++; return ok(wireDebt) }))
  const user = userEvent.setup()
  renderScreen()
  await openScreen()

  await openMenu(user)
  await user.click(screen.getByRole('menuitem', { name: 'Mark as paid off' }))
  expect(called).toBe(0)
  const dialog = await screen.findByRole('dialog', { name: 'Mark as paid off?' })
  await user.click(within(dialog).getByRole('button', { name: 'Mark as paid off' }))

  await waitFor(() => expect(called).toBe(1))
})

it('a removal_unconfirmed answer asks and resends with confirmDataRemoval', async () => {
  const bodies: Record<string, unknown>[] = []
  const refusal = 'This deletes the planned amounts and comments of this debt in your budgets; confirm to continue.'
  server.use(
    http.post('*/api/v1/debt/delete-debt', async ({ request }) => {
      const body = (await request.json()) as Record<string, unknown>
      bodies.push(body)
      if (!body.confirmDataRemoval) {
        return HttpResponse.json(
          { success: false, message: 'Form validation error', code: 400, errors: { confirmDataRemoval: [refusal] } },
          { status: 400 },
        )
      }
      return ok({})
    }),
  )
  const user = userEvent.setup()
  renderScreen()
  await openScreen()

  await openMenu(user)
  await user.click(screen.getByRole('menuitem', { name: 'Delete' }))
  const first = await screen.findByRole('dialog', { name: 'Delete this debt?' })
  await user.click(within(first).getByRole('button', { name: 'Delete' }))

  const second = await screen.findByRole('dialog', { name: 'Delete plans and comments?' })
  expect(within(second).getByText(refusal)).toBeInTheDocument()
  expect(bodies).toEqual([{ id: 'd1' }])
  await user.click(within(second).getByRole('button', { name: 'Delete debt' }))

  await waitFor(() => expect(bodies).toHaveLength(2))
  expect(bodies[1]).toEqual({ id: 'd1', confirmDataRemoval: true })
  await waitFor(() => expect(useUiStore.getState().debtDetailsId).toBeNull())
})

it('a delete the server accepts closes the screen without a second question', async () => {
  const bodies: Record<string, unknown>[] = []
  server.use(
    http.post('*/api/v1/debt/delete-debt', async ({ request }) => {
      bodies.push((await request.json()) as Record<string, unknown>)
      return ok({})
    }),
  )
  const user = userEvent.setup()
  renderScreen()
  await openScreen()

  await openMenu(user)
  await user.click(screen.getByRole('menuitem', { name: 'Delete' }))
  await user.click(within(await screen.findByRole('dialog', { name: 'Delete this debt?' })).getByRole('button', { name: 'Delete' }))

  await waitFor(() => expect(useUiStore.getState().debtDetailsId).toBeNull())
  expect(bodies).toEqual([{ id: 'd1' }])
})

it('a debt that fails to load says why instead of showing an empty screen', async () => {
  server.use(
    http.get('*/api/v1/debt/get-debt', () =>
      HttpResponse.json({ success: false, message: 'Debt not found', code: 0, errors: {} }, { status: 404 }),
    ),
  )
  renderScreen()
  useUiStore.getState().openDebtDetails('d1')
  expect(await screen.findByRole('alert')).toHaveTextContent('Debt not found')
})

it.each([
  ['phone', true],
  ['desktop', false],
])('the review screen is full screen on %s', async (_name, phone) => {
  mockMatchMedia(phone)
  const { baseElement } = renderScreen()
  await openScreen()

  const content = baseElement.querySelector('[data-slot="dialog-content"]')
  expect(content?.className).toContain('h-dvh')
  expect(content?.className).toContain('w-screen')
  expect(content?.className).toContain('rounded-none')
  expect(baseElement.querySelector('[data-slot="drawer-content"]')).toBeNull()
})
