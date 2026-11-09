import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { useUiStore } from '@/app/uiStore'
import { queryKeys } from '@/app/queryKeys'
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

function renderScreenWithClient() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/', element: <DebtReviewScreen /> }], { initialEntries: ['/'] })
  const view = render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return { ...view, queryClient }
}

function renderScreen() {
  return renderScreenWithClient()
}

const TERM_KEYS = ['balance', 'payment', 'schedule', 'lastPaymentDate']

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
    { transactionId: 't1', date: '2026-11-05 00:00:00', amount: '100', interest: '10', principal: '90', balance: '1090' },
    { transactionId: 't2', date: '2026-10-05 00:00:00', amount: '100', interest: '5.5', principal: '94.5', balance: '1180' },
  ],
}

const notSetUp: DebtDetailsDto = {
  item: wireDebtNotSetUp,
  snapshots: [],
  payments: [
    { transactionId: 't1', date: '2026-11-05 00:00:00', amount: '100', interest: '0', principal: '100', balance: '' },
    { transactionId: 't2', date: '2026-10-05 00:00:00', amount: '50.5', interest: '0', principal: '50.5', balance: '' },
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

// the projection starts after the caller's day: pin it before the stored next due
beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date(2026, 10, 8, 12))
})
afterEach(() => {
  vi.useRealTimers()
})

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers(), http.get('*/api/v1/debt/get-debt', () => ok(details)))
  mockMatchMedia()
  vi.mocked(trackEvent).mockClear()
  useUiStore.setState({ debtDetailsId: null, debtDetailsTab: 'overview', debtDetailsBudgetId: null, debtModal: null })
})

async function openScreen(tab?: 'overview' | 'settings') {
  useUiStore.getState().openDebtDetails('d1', tab)
  // the file's first render pays for the screen's first load: room for a busy host
  await screen.findByRole('heading', { name: 'Car loan' }, { timeout: 5000 })
  if (tab !== 'settings') {
    await screen.findByTestId('debt-review-top')
  }
}

async function openSettings(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('tab', { name: 'Settings' }))
  await screen.findByLabelText('Name')
}

async function openDanger(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('tab', { name: 'Danger zone' }))
  return screen.getByRole('tabpanel', { name: 'Danger zone' })
}

it('opens on the Overview tab, with Settings and Danger zone tabs beside it and no actions menu', async () => {
  renderScreen()
  await openScreen()

  const tabs = screen.getAllByRole('tab').map((tab) => tab.textContent)
  expect(tabs).toEqual(['Overview', 'Settings', 'Danger zone'])
  expect(screen.getByRole('tab', { name: 'Overview' })).toHaveAttribute('aria-selected', 'true')
  expect(screen.getByRole('tab', { name: 'Settings' })).toHaveAttribute('aria-selected', 'false')
  expect(screen.getByRole('tab', { name: 'Danger zone' })).toHaveAttribute('aria-selected', 'false')
  expect(screen.queryByRole('button', { name: 'Debt actions' })).toBeNull()
  expect(screen.queryByRole('menu')).toBeNull()
  expect(screen.getByRole('button', { name: 'Close' })).toBeInTheDocument()
})

it('set-up debt shows payments left first with an X of Y progress', async () => {
  renderScreen()
  await openScreen()

  const top = screen.getByTestId('debt-review-top')
  expect(within(top).getByTestId('debt-review-payments-left')).toHaveTextContent('283 payments left')
  expect(top).toHaveTextContent('77 of 360 paid')
  expect(top).toHaveTextContent('Owed now 1,090.00 $ · Paid off by Jul 2050')
  // the count leads the screen, before the loan's details
  const loan = screen.getByRole('region', { name: 'Loan' })
  expect(top.compareDocumentPosition(loan) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})

it('the progress bar shows the whole loan as its track and the paid part filled', async () => {
  renderScreen()
  await openScreen()

  const bar = within(screen.getByTestId('debt-review-top')).getByRole('progressbar', { name: '77 of 360 paid' })
  // a visible track on the card: the full width is every payment
  expect(bar.className).toContain('bg-foreground/15')
  // 77 of 360 filled
  expect(bar.querySelector<HTMLElement>('[data-slot="progress-indicator"]')?.style.transform).toBe(`translateX(-${100 - (77 / 360) * 100}%)`)
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

it('the Loan card holds every figure of the debt and nothing to edit', async () => {
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
  expect(within(loan).queryByRole('button')).toBeNull()
})

it('a debt with a start date shows it in the Loan card', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok({ ...details, item: { ...details.item, startDate: '2026-03-01' } })))
  renderScreen()
  await openScreen()

  const loan = screen.getByRole('region', { name: 'Loan' })
  expect(within(loan).getByText('Start date').nextElementSibling).toHaveTextContent('Mar 1, 2026')
})

it('the Overview reads which payments count as a sentence, with no pickers', async () => {
  renderScreen()
  await openScreen()

  const sentence = screen.getByRole('group', { name: 'Which payments count' })
  expect(sentence).toHaveTextContent(/Payments in \w*Food · from any payee · tagged any tag/)
  expect(within(sentence).queryAllByRole('button')).toHaveLength(0)
})

it('shows interest, principal and the balance left after each payment, newest first', async () => {
  renderScreen()
  await openScreen()

  expect(screen.getByRole('columnheader', { name: 'Interest' })).toBeInTheDocument()
  expect(screen.getByRole('columnheader', { name: 'Principal' })).toBeInTheDocument()
  expect(screen.getByRole('columnheader', { name: 'Balance' })).toBeInTheDocument()
  const rows = screen.getAllByTestId('debt-payment')
  expect(rows).toHaveLength(2)
  expect(within(rows[0]).getAllByRole('cell').map((c) => c.textContent)).toEqual(['Nov 5, 2026', '100.00 $', '10.00 $', '90.00 $', '1,090.00 $'])
  expect(within(rows[1]).getAllByRole('cell').map((c) => c.textContent)).toEqual(['Oct 5, 2026', '100.00 $', '5.50 $', '94.50 $', '1,180.00 $'])
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
  expect(screen.queryByRole('columnheader', { name: 'Balance' })).toBeNull()
  expect(within(screen.getAllByTestId('debt-payment')[0]).getAllByRole('cell').map((c) => c.textContent)).toEqual(['Nov 5, 2026', '100.00 $'])
  // no terms, no projection
  expect(screen.queryByRole('button', { name: 'Show future payments' })).toBeNull()
})

it('Paid so far counts only payments dated up to today; the list keeps the future one', async () => {
  const withFuture: DebtDetailsDto = {
    ...notSetUp,
    payments: [{ transactionId: 't3', date: '2026-11-20 00:00:00', amount: '70', interest: '0', principal: '70', balance: '' }, ...notSetUp.payments],
  }
  server.use(http.get('*/api/v1/debt/get-debt', () => ok(withFuture)))
  renderScreen()
  await openScreen()

  // today is pinned to 2026-11-08: the Nov 20 payment has not happened yet
  expect(screen.getByTestId('debt-review-top')).toHaveTextContent('150.50 $')
  expect(screen.getAllByTestId('debt-payment')).toHaveLength(3)
})

it('Finish setup switches to Settings with the balance focused', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok(notSetUp)))
  const user = userEvent.setup()
  renderScreen()
  await openScreen()

  await user.click(within(screen.getByTestId('debt-review-top')).getByRole('button', { name: 'Finish setup' }))

  expect(screen.getByRole('tab', { name: 'Settings' })).toHaveAttribute('aria-selected', 'true')
  await waitFor(() => expect(screen.getByLabelText('Balance now')).toHaveFocus())
})

it('Finish setup after a visit to Settings focuses the balance of the reloaded form', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok(notSetUp)))
  const user = userEvent.setup()
  renderScreen()
  await openScreen()
  await openSettings(user)
  await user.click(screen.getByRole('tab', { name: 'Overview' }))

  await user.click(within(screen.getByTestId('debt-review-top')).getByRole('button', { name: 'Finish setup' }))

  await waitFor(() => expect(screen.getByLabelText('Balance now')).toHaveFocus())
  // still there once the reload has landed
  await new Promise((resolve) => setTimeout(resolve, 50))
  expect(screen.getByLabelText('Balance now')).toHaveFocus()
})

it('opened for Finish setup, the screen starts on Settings', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok(notSetUp)))
  renderScreen()
  await openScreen('settings')

  expect(screen.getByRole('tab', { name: 'Settings' })).toHaveAttribute('aria-selected', 'true')
  await waitFor(() => expect(screen.getByLabelText('Balance now')).toHaveFocus())
})

it('saving the terms of a debt not set up finishes its setup and goes back to the Overview', async () => {
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
  await user.click(await screen.findByRole('button', { name: /November 15th, 2026/ }))
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toMatchObject({ id: 'd1', balance: '600', payment: '100', schedule: 'monthly', lastPaymentDate: '2026-11-15' })
  expect(vi.mocked(trackEvent).mock.calls.filter(([event]) => event === METRICS.DEBT_FINISH_SETUP)).toHaveLength(1)
  expect(trackEvent).not.toHaveBeenCalledWith(METRICS.DEBT_UPDATE)
  expect(await screen.findByText('12 payments left')).toBeInTheDocument()
  expect(screen.getByRole('tab', { name: 'Overview' })).toHaveAttribute('aria-selected', 'true')
})

it('Settings loads the debt afresh: the balance is the one after a payment landed', async () => {
  const user = userEvent.setup()
  renderScreen()
  await openScreen()
  // a payment landed since the screen loaded
  server.use(http.get('*/api/v1/debt/get-debt', () => ok({ ...details, item: { ...details.item, currentBalance: '990' } })))

  await openSettings(user)

  expect(await screen.findByLabelText('Balance now')).toHaveValue('990')
})

it('Settings saves the name, the payments sentence and the terms in one request', async () => {
  const bodies = captureUpdate((body) => ({ ...details.item, name: body.name as string, labelId: 'label1' }))
  const user = userEvent.setup()
  renderScreen()
  await openScreen()
  await openSettings(user)

  const name = screen.getByLabelText('Name')
  await user.clear(name)
  await user.type(name, 'Van loan')
  await user.click(screen.getByRole('button', { name: 'Tag (optional): any tag' }))
  await user.click(within(await screen.findByRole('dialog', { name: 'Tag (optional)' })).getByText('health'))
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  // no terms and no currency go along: the server keeps the stored ones
  expect(bodies[0]).toMatchObject({ id: 'd1', name: 'Van loan', currencyId: '', categoryId: 'cat-food', payeeId: null, tagId: null, labelId: 'label1' })
  for (const key of TERM_KEYS) {
    expect(bodies[0]).not.toHaveProperty(key)
  }
  expect(trackEvent).toHaveBeenCalledWith(METRICS.DEBT_UPDATE)
  await waitFor(() => expect(screen.getByRole('tab', { name: 'Overview' })).toHaveAttribute('aria-selected', 'true'))
})

it('a save after an earlier one starts from the debt as saved, so the earlier terms are not sent again', async () => {
  let current = details.item
  server.use(http.get('*/api/v1/debt/get-debt', () => ok({ ...details, item: current })))
  const bodies = captureUpdate((body) => {
    current = body.balance ? { ...current, balance: '600', currentBalance: '600', lastPaymentDate: '2031-03-05' } : { ...current, name: body.name as string }
    return current
  })
  const user = userEvent.setup()
  renderScreen()
  await openScreen()

  await openSettings(user)
  const balance = screen.getByLabelText('Balance now')
  await user.clear(balance)
  await user.type(balance, '600')
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(bodies).toHaveLength(1))
  await waitFor(() => expect(screen.getByRole('tab', { name: 'Overview' })).toHaveAttribute('aria-selected', 'true'))

  await openSettings(user)
  await waitFor(() => expect(screen.getByLabelText('Balance now')).toHaveValue('600'))
  const name = screen.getByLabelText('Name')
  await user.clear(name)
  await user.type(name, 'Van loan')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(bodies).toHaveLength(2))
  expect(bodies[1]).toMatchObject({ name: 'Van loan' })
  for (const key of TERM_KEYS) {
    expect(bodies[1]).not.toHaveProperty(key)
  }
})

it('edits survive a look at the Overview and back', async () => {
  const user = userEvent.setup()
  renderScreen()
  await openScreen()
  await openSettings(user)

  const name = screen.getByLabelText('Name')
  await user.clear(name)
  await user.type(name, 'Van loan')
  await user.click(screen.getByRole('tab', { name: 'Overview' }))
  await user.click(screen.getByRole('tab', { name: 'Settings' }))

  expect(screen.getByLabelText('Name')).toHaveValue('Van loan')
})

it('the Danger zone tab holds Mark as paid off and Delete, each with what it does; Settings ends at Save', async () => {
  const user = userEvent.setup()
  renderScreen()
  await openScreen()
  await openSettings(user)

  const settings = screen.getByRole('tabpanel', { name: 'Settings' })
  expect(within(settings).getByRole('button', { name: 'Save' })).toBeInTheDocument()
  expect(within(settings).queryByRole('button', { name: 'Delete' })).toBeNull()
  expect(within(settings).queryByRole('button', { name: 'Mark as paid off' })).toBeNull()
  expect(within(settings).queryByText('Danger zone')).toBeNull()

  const zone = await openDanger(user)
  const paidOff = within(zone).getByRole('button', { name: 'Mark as paid off' })
  const del = within(zone).getByRole('button', { name: 'Delete' })
  expect(paidOff.compareDocumentPosition(del) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(zone).toHaveTextContent('Sets the balance to 0 and stops the countdown.')
  expect(zone).toHaveTextContent('Removes the debt. Its payments go back to the category.')
})

it('Danger zone hides Mark as paid off while not set up', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok(notSetUp)))
  const user = userEvent.setup()
  renderScreen()
  await openScreen()
  const zone = await openDanger(user)

  expect(within(zone).queryByRole('button', { name: 'Mark as paid off' })).toBeNull()
  expect(within(zone).queryByText('Sets the balance to 0 and stops the countdown.')).toBeNull()
  expect(within(zone).getByRole('button', { name: 'Delete' })).toBeInTheDocument()
})

it('Danger zone hides Mark as paid off once the debt is paid off', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok({ ...details, item: { ...details.item, isPaidOff: 1 } })))
  const user = userEvent.setup()
  renderScreen()
  await openScreen()
  const zone = await openDanger(user)

  expect(within(zone).queryByRole('button', { name: 'Mark as paid off' })).toBeNull()
})

it('marking a debt paid off asks first, then posts', async () => {
  let called = 0
  server.use(http.post('*/api/v1/debt/mark-debt-paid-off', () => { called++; return ok(wireDebt) }))
  const user = userEvent.setup()
  renderScreen()
  await openScreen()
  const zone = await openDanger(user)

  await user.click(within(zone).getByRole('button', { name: 'Mark as paid off' }))
  expect(called).toBe(0)
  const dialog = await screen.findByRole('dialog', { name: 'Mark as paid off?' })
  await user.click(within(dialog).getByRole('button', { name: 'Mark as paid off' }))

  await waitFor(() => expect(called).toBe(1))
})

it('after Mark as paid off, Settings starts from the paid-off debt, so a later edit does not send the old balance', async () => {
  let current = details.item
  server.use(
    http.get('*/api/v1/debt/get-debt', () => ok({ ...details, item: current })),
    http.post('*/api/v1/debt/mark-debt-paid-off', () => {
      current = { ...current, currentBalance: '0', isPaidOff: 1, paymentsLeft: 0 }
      return ok(current)
    }),
  )
  const bodies = captureUpdate(() => current)
  const user = userEvent.setup()
  renderScreen()
  await openScreen()
  await openSettings(user)
  expect(screen.getByLabelText('Balance now')).toHaveValue('1090')

  await user.click(within(await openDanger(user)).getByRole('button', { name: 'Mark as paid off' }))
  await user.click(within(await screen.findByRole('dialog', { name: 'Mark as paid off?' })).getByRole('button', { name: 'Mark as paid off' }))
  // back on the Overview, like a save
  await waitFor(() => expect(screen.getByRole('tab', { name: 'Overview' })).toHaveAttribute('aria-selected', 'true'))

  await openSettings(user)
  await waitFor(() => expect(screen.getByLabelText('Balance now')).toHaveValue('0'))
  const payment = screen.getByLabelText('Payment')
  await user.clear(payment)
  await user.type(payment, '120')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toMatchObject({ balance: '0', payment: '120' })
})

it('Settings shows a loader, not just its buttons, while the debt loads', async () => {
  const user = userEvent.setup()
  renderScreen()
  await openScreen()
  let release: () => void = () => {}
  const gate = new Promise<void>((resolve) => {
    release = resolve
  })
  server.use(
    http.get('*/api/v1/debt/get-debt', async () => {
      await gate
      return ok(details)
    }),
  )

  await user.click(screen.getByRole('tab', { name: 'Settings' }))

  const panel = screen.getByRole('tabpanel', { name: 'Settings' })
  expect(within(panel).getByRole('status')).toBeInTheDocument()
  expect(within(panel).queryByRole('button', { name: 'Save' })).toBeNull()
  release()
  expect(await within(panel).findByLabelText('Name')).toBeInTheDocument()
  expect(within(panel).queryByRole('status')).toBeNull()
  expect(within(panel).getByRole('button', { name: 'Save' })).toBeInTheDocument()
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
  await user.click(within(await openDanger(user)).getByRole('button', { name: 'Delete' }))
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
  await user.click(within(await openDanger(user)).getByRole('button', { name: 'Delete' }))
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

it('the review screen is full screen on a phone', async () => {
  mockMatchMedia(true)
  const { baseElement } = renderScreen()
  await openScreen()

  const content = baseElement.querySelector('[data-slot="dialog-content"]')
  expect(content?.className).toContain('h-dvh')
  expect(content?.className).toContain('w-screen')
  expect(baseElement.querySelector('[data-slot="drawer-content"]')).toBeNull()
})

it('the review screen is a large centred dialog on a desktop, not a full page', async () => {
  const { baseElement } = renderScreen()
  await openScreen()

  const content = baseElement.querySelector('[data-slot="dialog-content"]')
  // as wide as its content column, not most of the screen
  expect(content?.className).toContain('sm:max-w-[min(90vw,52rem)]')
  expect(content?.className).toContain('h-[90vh]')
  expect(content?.className).toContain('overflow-hidden')
  expect(content?.className).not.toContain('h-dvh')
})

describe('the header and the tabs are one block above the scrolling content', () => {
  it.each([
    ['desktop', false],
    ['phone', true],
  ])('on a %s the tab bar sits in the opaque, bordered header block, outside the scrolling body', async (_, phone) => {
    mockMatchMedia(phone)
    const { baseElement } = renderScreen()
    await openScreen()

    const tablist = screen.getByRole('tablist')
    const block = baseElement.querySelector('[data-slot="dialog-sticky-header"]')
    const body = baseElement.querySelector('[data-slot="dialog-body"]')
    expect(block).not.toBeNull()
    expect(block).toContainElement(screen.getByRole('heading', { name: 'Car loan' }))
    expect(block).toContainElement(tablist)
    expect(body).not.toContainElement(tablist)
    expect(block?.className).toContain('border-b')
    expect(block?.className).toContain('bg-popover')
    expect(block?.className).toContain('shrink-0')
  })

  it('every tab carries its own underline on the block border, as wide as its label', async () => {
    renderScreen()
    await openScreen()

    // the list hugs its triggers and lines them up at the bottom: no fixed
    // height for a taller trigger to overflow
    const tablist = screen.getByRole('tablist')
    expect(tablist.className).toContain('items-end')
    expect(tablist.className).toContain('h-auto')
    expect(tablist.className).not.toMatch(/(^|\s)border-b(\s|$)/)
    for (const tab of screen.getAllByRole('tab')) {
      // the 2px rule overlaps the block's 1px border; no side padding widens it
      expect(tab.className).toContain('-mb-px')
      expect(tab.className).toContain('border-b-2')
      expect(tab.className).toContain('px-0')
      expect(tab.className).toContain('h-auto')
      expect(tab.className).toContain('after:hidden')
    }
  })
})

describe('future payments', () => {
  const forecast = {
    items: [
      { date: '2026-12-05', payment: '400', interest: '9.08', principal: '390.92', balance: '699.08' },
      { date: '2027-01-05', payment: '100', interest: '5.83', principal: '94.17', balance: '604.91' },
    ],
    budgetId: 'b1',
    budgetName: 'Family',
  }

  function captureForecast(answer: unknown = forecast) {
    const asked: URLSearchParams[] = []
    server.use(
      http.get('*/api/v1/debt/get-debt-forecast', ({ request }) => {
        asked.push(new URL(request.url).searchParams)
        return ok(answer)
      }),
    )
    return asked
  }

  it('Show future payments loads the forecast of the budget the screen came from into the top of the payments table, with its plan named', async () => {
    const asked = captureForecast()
    const user = userEvent.setup()
    renderScreen()
    useUiStore.getState().openDebtDetails('d1', 'overview', 'b1')
    await screen.findByTestId('debt-review-top', {}, { timeout: 5000 })
    expect(asked).toHaveLength(0)
    expect(screen.queryByTestId('debt-forecast-payment')).toBeNull()

    const show = screen.getByRole('button', { name: 'Show future payments' })
    expect(show).toHaveAttribute('aria-expanded', 'false')
    await user.click(show)

    const rows = await screen.findAllByTestId('debt-forecast-payment')
    expect(asked).toHaveLength(1)
    expect(asked[0].get('id')).toBe('d1')
    expect(asked[0].get('budgetId')).toBe('b1')
    // one table, newest first: the latest projected payment on top, down to
    // the next due, a Recorded line, then the booked payments
    expect(screen.getAllByRole('table')).toHaveLength(1)
    expect(screen.queryByRole('heading', { name: 'Future payments' })).toBeNull()
    expect(within(rows[0]).getAllByRole('cell').map((c) => c.textContent)).toEqual(['Jan 5, 2027', '100.00 $', '5.83 $', '94.17 $', '604.91 $'])
    expect(within(rows[1]).getAllByRole('cell').map((c) => c.textContent)).toEqual(['Dec 5, 2026', '400.00 $', '9.08 $', '390.92 $', '699.08 $'])
    const today = screen.getByTestId('debt-payments-recorded')
    expect(today).toHaveTextContent('Recorded')
    const booked = screen.getAllByTestId('debt-payment')
    expect(rows[1].compareDocumentPosition(today) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(today.compareDocumentPosition(booked[0]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    // projected rows read apart from the booked ones
    expect(rows[0].className).toContain('italic')
    expect(rows[0].className).toContain('text-muted-foreground')
    expect(booked[0].className).not.toContain('italic')
    // the toggle and the note sit above the table, next to the heading
    const note = screen.getByText('Includes extra payments planned in Family')
    const table = screen.getByRole('table')
    expect(note.compareDocumentPosition(table) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Hide future payments' }).compareDocumentPosition(table) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(trackEvent).toHaveBeenCalledWith(METRICS.DEBT_FORECAST_VIEW)

    const hide = screen.getByRole('button', { name: 'Hide future payments' })
    expect(hide).toHaveAttribute('aria-expanded', 'true')
    await user.click(hide)
    expect(screen.queryByTestId('debt-forecast-payment')).toBeNull()
    expect(screen.queryByTestId('debt-payments-recorded')).toBeNull()
    expect(screen.queryByText('Includes extra payments planned in Family')).toBeNull()
    expect(screen.getByRole('button', { name: 'Show future payments' })).toBeInTheDocument()
  })

  it('a long forecast shows its payoff end and the payments nearest today, the middle behind "Show N more payments"', async () => {
    const items = Array.from({ length: 30 }, (_, i) => {
      const d = new Date(Date.UTC(2026, 11 + i, 5)).toISOString().slice(0, 10)
      return { date: d, payment: '100', interest: '1', principal: '99', balance: String(3000 - 99 * (i + 1)) }
    })
    captureForecast({ items, budgetId: '', budgetName: '' })
    const user = userEvent.setup()
    renderScreen()
    await openScreen()

    await user.click(screen.getByRole('button', { name: 'Show future payments' }))

    const rows = await screen.findAllByTestId('debt-forecast-payment')
    // 3 at the payoff end, then the 12 nearest today
    expect(rows).toHaveLength(15)
    expect(rows[0]).toHaveTextContent('May 5, 2029')
    expect(rows[2]).toHaveTextContent('Mar 5, 2029')
    expect(rows[3]).toHaveTextContent('Nov 5, 2027')
    expect(rows[14]).toHaveTextContent('Dec 5, 2026')
    const more = screen.getByRole('button', { name: 'Show 15 more payments' })
    expect(rows[2].compareDocumentPosition(more) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    await user.click(more)
    expect(screen.getAllByTestId('debt-forecast-payment')).toHaveLength(30)
    expect(screen.queryByRole('button', { name: /more payments/ })).toBeNull()
  })

  it('each opening loads the list afresh and counts once; a refetch in the background is no new view', async () => {
    const asked = captureForecast()
    const user = userEvent.setup()
    const { queryClient } = renderScreenWithClient()
    await openScreen()

    await user.click(screen.getByRole('button', { name: 'Show future payments' }))
    await screen.findAllByTestId('debt-forecast-payment')
    expect(vi.mocked(trackEvent).mock.calls.filter(([m]) => m === METRICS.DEBT_FORECAST_VIEW)).toHaveLength(1)

    // a plan saved elsewhere invalidates it: refetched, not counted again
    await queryClient.invalidateQueries({ queryKey: queryKeys.debtForecast('d1', '') })
    await waitFor(() => expect(asked).toHaveLength(2))
    expect(vi.mocked(trackEvent).mock.calls.filter(([m]) => m === METRICS.DEBT_FORECAST_VIEW)).toHaveLength(1)

    // hidden and shown again: never served from a stale cache
    await user.click(screen.getByRole('button', { name: 'Hide future payments' }))
    await user.click(screen.getByRole('button', { name: 'Show future payments' }))
    await waitFor(() => expect(asked).toHaveLength(3))
    expect(vi.mocked(trackEvent).mock.calls.filter(([m]) => m === METRICS.DEBT_FORECAST_VIEW)).toHaveLength(2)
  })

  it('a projected payment dated before today is marked overdue', async () => {
    captureForecast({
      ...forecast,
      items: [{ date: '2026-11-05', payment: '100', interest: '9.08', principal: '90.92', balance: '999.08' }, ...forecast.items],
    })
    const user = userEvent.setup()
    renderScreen()
    await openScreen()

    await user.click(screen.getByRole('button', { name: 'Show future payments' }))

    const rows = await screen.findAllByTestId('debt-forecast-payment')
    // today is pinned to 2026-11-08; the overdue due is the one nearest the Recorded line
    expect(within(rows[2]).getAllByRole('cell')[0]).toHaveTextContent('Nov 5, 2026overdue')
    expect(within(rows[1]).getAllByRole('cell')[0]).toHaveTextContent(/^Dec 5, 2026$/)
  })

  it('opened from Settings it asks with no budget, and says when only the regular payments count', async () => {
    const asked = captureForecast({ ...forecast, budgetId: '', budgetName: '' })
    const user = userEvent.setup()
    renderScreen()
    await openScreen()

    await user.click(screen.getByRole('button', { name: 'Show future payments' }))

    await screen.findAllByTestId('debt-forecast-payment')
    expect(asked[0].get('budgetId')).toBeNull()
    expect(screen.getByText('Regular payments only')).toBeInTheDocument()
  })

  it('a paid-off debt offers no future payments', async () => {
    server.use(http.get('*/api/v1/debt/get-debt', () => ok({ ...details, item: { ...details.item, isPaidOff: 1 } })))
    renderScreen()
    await openScreen()

    expect(screen.queryByRole('button', { name: 'Show future payments' })).toBeNull()
  })
})
