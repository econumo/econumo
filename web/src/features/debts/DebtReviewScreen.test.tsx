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
import { wireDebt, wireDebtNotSetUp, wireRule, wireRuleOptions } from './testDebt'

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

const TERM_KEYS = ['amountBorrowed', 'balance', 'payment', 'schedule', 'lastPaymentDate']

const ok = (data: unknown) => HttpResponse.json({ success: true, message: '', data })

const details: DebtDetailsDto = {
  item: {
    ...wireDebt,
    startDate: '2026-10-01',
    currentBalance: '1090',
    paymentsLeft: 283,
    periodsPaid: 77,
    moneyLeft: '1100',
    overpaymentLeft: '10',
    overpaidSoFar: '15.5',
    extraPaid: '40',
    payoffDate: '2050-07-05',
    lastPaymentDate: '2050-07-05',
    nextDueDate: '2026-12-05',
  },
  snapshots: [{ id: 's0', isOrigin: 1, computedBalance: '', asOf: '2026-10-01', balance: '1200', payment: '100', schedule: 'monthly', lastPaymentDate: '2027-11-05' }],
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
  // no forecast summary by default: the Overview falls back to the debt's own figures
  server.use(
    ...coreHandlers(),
    http.get('*/api/v1/debt/get-debt', () => ok(details)),
    http.get('*/api/v1/debt/get-debt-rule-options', () => ok(wireRuleOptions)),
    http.get('*/api/v1/debt/get-debt-forecast', () => HttpResponse.json({ success: false, message: 'Debt not found', code: 0, errors: {} }, { status: 400 })),
  )
  mockMatchMedia()
  vi.mocked(trackEvent).mockClear()
  useUiStore.setState({ debtDetailsId: null, debtDetailsTab: 'overview', debtModal: null })
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

it('opens on the Overview tab, with Balance, Settings and Danger zone tabs beside it and no actions menu', async () => {
  renderScreen()
  await openScreen()

  const tabs = screen.getAllByRole('tab').map((tab) => tab.textContent)
  expect(tabs).toEqual(['Overview', 'Balance', 'Settings', 'Danger zone'])
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
  // the payments, then the principal repaid of the amount borrowed (1,200 less the 1,090 owed)
  expect(top).toHaveTextContent('77 of 360 paid (110.00 $ of 1,200.00 $)')
  expect(top).not.toHaveTextContent('Owed now')
  expect(top).not.toHaveTextContent('Paid off by')
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
  const top = screen.getByTestId('debt-review-top')
  expect(top).not.toHaveTextContent('Paid off by')
  // unchanged once paid off: the count alone, and what is owed
  expect(top).toHaveTextContent('77 of 77 paid')
  expect(top).not.toHaveTextContent('(')
  expect(top).toHaveTextContent('Owed now 0.00 $')
})

it('the principal repaid never reads below zero', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok({ ...details, item: { ...details.item, currentBalance: '1300' } })))
  renderScreen()
  await openScreen()

  expect(screen.getByTestId('debt-review-top')).toHaveTextContent('77 of 360 paid (0.00 $ of 1,200.00 $)')
})

it('the Loan card holds the loan\'s figures and nothing to edit', async () => {
  renderScreen()
  await openScreen()

  const loan = screen.getByRole('region', { name: 'Loan' })
  const figure = (label: string) => within(loan).getByText(label).nextElementSibling
  // the forecast fails here: Planned extra waits for it rather than show a plan-free number
  expect(within(loan).getAllByRole('term').map((dt) => dt.textContent)).toEqual([
    'Borrowed', 'Payment', 'Next payment', 'Last payment', 'Paid extra',
  ])
  expect(figure('Borrowed')).toHaveTextContent('1,200.00 $ · Oct 1, 2026')
  expect(figure('Payment')).toHaveTextContent('100.00 $ · Monthly')
  expect(figure('Next payment')).toHaveTextContent('Dec 5, 2026')
  // without the forecast it is the contract's last payment date, with nothing beside it
  expect(figure('Last payment')).toHaveTextContent(/^Jul 5, 2050$/)
  expect(figure('Paid extra')).toHaveTextContent('40.00 $')
  // what is owed is the hero's, interest the payments table's; the currency shows on every amount
  for (const gone of ['Owed now', 'Left to pay', 'Currency', 'Start date', 'Overpaid', 'Planned overpayment', 'Planned extra']) {
    expect(within(loan).queryByText(gone)).toBeNull()
  }
  expect(within(loan).queryByRole('button')).toBeNull()
})

it('Paid extra shows 0 rather than hiding', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok({ ...details, item: { ...details.item, extraPaid: '0' } })))
  renderScreen()
  await openScreen()

  expect(within(screen.getByRole('region', { name: 'Loan' })).getByText('Paid extra').nextElementSibling).toHaveTextContent('0.00 $')
})

it('a debt not set up keeps its Loan card: payment, start and currency', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok(notSetUp)))
  renderScreen()
  await openScreen()

  const loan = screen.getByRole('region', { name: 'Loan' })
  expect(within(loan).getAllByRole('term').map((dt) => dt.textContent)).toEqual(['Payment', 'Start date', 'Currency'])
  expect(within(loan).getByText('Currency').nextElementSibling).toHaveTextContent('USD')
})

describe('the Overview follows the budget\'s plans', () => {
  const summary = { items: [], budgetId: 'b1', budgetName: 'Family', paymentsLeft: 200, payoffDate: '2043-02-05', overpaymentLeft: '4.25', extraPlanned: '3000' }

  function captureSummary(answer: unknown = summary) {
    const asked: URLSearchParams[] = []
    server.use(
      http.get('*/api/v1/debt/get-debt-forecast', ({ request }) => {
        asked.push(new URL(request.url).searchParams)
        return ok(answer)
      }),
    )
    return asked
  }

  it('payments left, the progress, the payoff and the planned extra come from the forecast of the debt\'s budget', async () => {
    const asked = captureSummary()
    renderScreen()
    useUiStore.getState().openDebtDetails('d1', 'overview')
    const top = await screen.findByTestId('debt-review-top', {}, { timeout: 5000 })

    await waitFor(() => expect(within(top).getByTestId('debt-review-payments-left')).toHaveTextContent('200 payments left'))
    expect(asked).toHaveLength(1)
    expect([...asked[0].keys()]).toEqual(['id'])
    // the principal repaid does not depend on plans
    expect(top).toHaveTextContent('77 of 277 paid (110.00 $ of 1,200.00 $)')
    expect(top).not.toHaveTextContent('Paid off by')
    const loan = screen.getByRole('region', { name: 'Loan' })
    expect(within(loan).getAllByRole('term').map((dt) => dt.textContent)).toEqual([
      'Borrowed', 'Payment', 'Next payment', 'Last payment', 'Paid extra', 'Planned extra',
    ])
    expect(within(loan).getByText('Planned extra').nextElementSibling).toHaveTextContent('3,000.00 $')
    // the plans pay the loan off before the contract's date, which stays beside it
    expect(within(loan).getByText('Last payment').nextElementSibling).toHaveTextContent('Feb 5, 2043contract: Jul 5, 2050')
    expect(within(loan).queryByText('Planned overpayment')).toBeNull()
    // loading the figures is no view of the future payments
    expect(trackEvent).not.toHaveBeenCalledWith(METRICS.DEBT_FORECAST_VIEW)
  })

  it('Show future payments uses the forecast already loaded, and counts the view', async () => {
    const asked = captureSummary({ ...summary, items: [{ date: '2026-12-05', payment: '400', interest: '9.08', principal: '390.92', balance: '699.08' }] })
    const user = userEvent.setup()
    renderScreen()
    await openScreen()
    await waitFor(() => expect(screen.getByTestId('debt-review-payments-left')).toHaveTextContent('200 payments left'))

    await user.click(screen.getByRole('button', { name: 'Show future payments' }))
    expect(await screen.findAllByTestId('debt-forecast-payment')).toHaveLength(1)
    expect(asked).toHaveLength(1)
    expect(trackEvent).toHaveBeenCalledWith(METRICS.DEBT_FORECAST_VIEW)
  })

  it('a forecast that fails leaves the debt\'s own figures', async () => {
    renderScreen()
    await openScreen()

    const top = screen.getByTestId('debt-review-top')
    await waitFor(() => expect(top).toHaveTextContent('77 of 360 paid (110.00 $ of 1,200.00 $)'))
    expect(within(top).getByTestId('debt-review-payments-left')).toHaveTextContent('283 payments left')
    expect(within(screen.getByRole('region', { name: 'Loan' })).queryByText('Planned extra')).toBeNull()
    expect(screen.queryByRole('alert')).toBeNull()
  })

  // TanStack keeps the last answer when a refetch fails: those figures may predate a plan change
  it('a refetch that fails drops the forecast loaded earlier for the debt\'s own figures', async () => {
    captureSummary()
    const { queryClient } = renderScreenWithClient()
    await openScreen()
    const top = screen.getByTestId('debt-review-top')
    await waitFor(() => expect(within(top).getByTestId('debt-review-payments-left')).toHaveTextContent('200 payments left'))

    server.use(http.get('*/api/v1/debt/get-debt-forecast', () => HttpResponse.json({ success: false, message: 'Boom', code: 0, errors: {} }, { status: 400 })))
    await queryClient.invalidateQueries({ queryKey: queryKeys.debtForecast('d1') })

    await waitFor(() => expect(within(top).getByTestId('debt-review-payments-left')).toHaveTextContent('283 payments left'))
    expect(within(screen.getByRole('region', { name: 'Loan' })).queryByText('Planned extra')).toBeNull()
  })

  it('Planned extra waits for the forecast to answer', async () => {
    let answer: () => void = () => {}
    const answered = new Promise<void>((resolve) => {
      answer = resolve
    })
    server.use(
      http.get('*/api/v1/debt/get-debt-forecast', async () => {
        await answered
        return ok({ ...summary, extraPlanned: '0' })
      }),
    )
    renderScreen()
    await openScreen()

    const loan = screen.getByRole('region', { name: 'Loan' })
    expect(within(loan).getByText('Paid extra')).toBeInTheDocument()
    expect(within(loan).queryByText('Planned extra')).toBeNull()
    answer()
    // with no plans it is 0, shown like Paid extra
    await waitFor(() => expect(within(loan).getByText('Planned extra').nextElementSibling).toHaveTextContent('0.00 $'))
  })

  it('a paid-off debt or one not set up asks for no forecast', async () => {
    const asked = captureSummary()
    server.use(http.get('*/api/v1/debt/get-debt', () => ok({ ...details, item: { ...details.item, isPaidOff: 1 } })))
    const { unmount } = renderScreen()
    await openScreen()
    unmount()
    useUiStore.setState({ debtDetailsId: null })
    server.use(http.get('*/api/v1/debt/get-debt', () => ok(notSetUp)))
    renderScreen()
    await openScreen()

    expect(asked).toHaveLength(0)
  })
})

it('a debt not set up shows its start date in the Loan card, or that it counts from the first payment', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok({ ...notSetUp, item: { ...notSetUp.item, startDate: '2026-03-01' } })))
  renderScreen()
  await openScreen()

  const loan = screen.getByRole('region', { name: 'Loan' })
  expect(within(loan).getByText('Start date').nextElementSibling).toHaveTextContent('Mar 1, 2026')
  expect(within(loan).queryByText('Borrowed')).toBeNull()
})

it('a debt not set up without a start date counts from the first payment', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok(notSetUp)))
  renderScreen()
  await openScreen()

  const loan = screen.getByRole('region', { name: 'Loan' })
  expect(within(loan).getByText('Start date').nextElementSibling).toHaveTextContent('From the first payment')
})

describe('a guest reads the debt without changing it', () => {
  const anna = {
    id: 'r2', categoryId: 'cat-mortgage', categoryName: 'Mortgage', categoryIcon: 'home', ownerUserId: 'u2', ownerName: 'Anna',
    payeeId: null, payeeName: '', tagId: null, tagName: '', labelId: null, labelName: '',
  }
  const guestView: DebtDetailsDto = {
    ...details,
    item: { ...details.item, canEdit: 0, rules: [anna] },
    snapshots: [...details.snapshots, { id: 's1', isOrigin: 0, computedBalance: '1100', asOf: '2026-11-06', balance: '1090', payment: '100', schedule: 'monthly', lastPaymentDate: '2027-11-05' }],
  }

  beforeEach(() => {
    server.use(http.get('*/api/v1/debt/get-debt', () => ok(guestView)))
  })

  it('has no Settings or Danger zone tab, and the rules name the owner of a partner\'s category', async () => {
    renderScreen()
    await openScreen()

    expect(screen.getAllByRole('tab').map((tab) => tab.textContent)).toEqual(['Overview', 'Balance'])
    const sentence = screen.getByRole('group', { name: 'Which payments count' })
    expect(sentence).toHaveTextContent(/Payments in \w*Anna's Mortgage · from any payee · tagged any tag/)
    expect(within(sentence).queryAllByRole('button')).toHaveLength(0)
  })

  it('the Balance tab shows the history with no reconcile form and nothing to delete', async () => {
    const user = userEvent.setup()
    renderScreen()
    await openScreen()

    await user.click(screen.getByRole('tab', { name: 'Balance' }))
    expect(await screen.findAllByTestId('debt-reconciliation')).toHaveLength(2)
    expect(screen.queryByLabelText('Statement balance')).toBeNull()
    expect(screen.queryByRole('button', { name: /^Delete/ })).toBeNull()
  })

  it('opened on Settings, it shows the Overview and loads no form', async () => {
    let loads = 0
    server.use(http.get('*/api/v1/debt/get-debt', () => {
      loads++
      return ok(guestView)
    }))
    renderScreen()
    useUiStore.getState().openDebtDetails('d1', 'settings')
    expect(await screen.findByTestId('debt-review-top', {}, { timeout: 5000 })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Overview' })).toHaveAttribute('aria-selected', 'true')
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(loads).toBe(1)
  })

  it('a debt not set up offers a guest no Finish setup', async () => {
    server.use(http.get('*/api/v1/debt/get-debt', () => ok({ ...notSetUp, item: { ...notSetUp.item, canEdit: 0 } })))
    renderScreen()
    await openScreen()

    expect(screen.getByText('Paid so far')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Finish setup' })).toBeNull()
  })
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

it('not-set-up debt shows Finish setup and no balance', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok(notSetUp)))
  renderScreen()
  await openScreen()

  const top = screen.getByTestId('debt-review-top')
  expect(top).toHaveTextContent('Paid so far')
  expect(top).toHaveTextContent('150.50 $')
  expect(within(top).getByRole('button', { name: 'Finish setup' })).toBeInTheDocument()
  expect(top).toHaveTextContent("Add the amount borrowed and the loan's dates to see payments left")
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

it('Finish setup switches to Settings with the amount borrowed focused', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok(notSetUp)))
  const user = userEvent.setup()
  renderScreen()
  await openScreen()

  await user.click(within(screen.getByTestId('debt-review-top')).getByRole('button', { name: 'Finish setup' }))

  expect(screen.getByRole('tab', { name: 'Settings' })).toHaveAttribute('aria-selected', 'true')
  await waitFor(() => expect(screen.getByLabelText('Amount borrowed')).toHaveFocus())
})

it('Finish setup after a visit to Settings focuses the amount borrowed of the reloaded form', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok(notSetUp)))
  const user = userEvent.setup()
  renderScreen()
  await openScreen()
  await openSettings(user)
  await user.click(screen.getByRole('tab', { name: 'Overview' }))

  await user.click(within(screen.getByTestId('debt-review-top')).getByRole('button', { name: 'Finish setup' }))

  await waitFor(() => expect(screen.getByLabelText('Amount borrowed')).toHaveFocus())
  // still there once the reload has landed
  await new Promise((resolve) => setTimeout(resolve, 50))
  expect(screen.getByLabelText('Amount borrowed')).toHaveFocus()
})

it('opened for Finish setup, the screen starts on Settings', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok(notSetUp)))
  renderScreen()
  await openScreen('settings')

  expect(screen.getByRole('tab', { name: 'Settings' })).toHaveAttribute('aria-selected', 'true')
  await waitFor(() => expect(screen.getByLabelText('Amount borrowed')).toHaveFocus())
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
  await user.type(await screen.findByLabelText('Amount borrowed'), '1200')
  await user.click(screen.getByRole('button', { name: 'Start date' }))
  await user.click(await screen.findByRole('button', { name: /November 1st, 2026/ }))
  await user.click(screen.getByRole('button', { name: 'Last payment date' }))
  await user.click(await screen.findByRole('button', { name: /November 15th, 2026/ }))
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toMatchObject({
    id: 'd1', amountBorrowed: '1200', startDate: '2026-11-01', payment: '100', schedule: 'monthly', lastPaymentDate: '2026-11-15',
  })
  expect(vi.mocked(trackEvent).mock.calls.filter(([event]) => event === METRICS.DEBT_FINISH_SETUP)).toHaveLength(1)
  expect(trackEvent).not.toHaveBeenCalledWith(METRICS.DEBT_UPDATE)
  expect(await screen.findByText('12 payments left')).toBeInTheDocument()
  expect(screen.getByRole('tab', { name: 'Overview' })).toHaveAttribute('aria-selected', 'true')
})

it('Settings loads the debt afresh: a fact corrected on another device shows', async () => {
  const user = userEvent.setup()
  renderScreen()
  await openScreen()
  // corrected elsewhere since the screen loaded
  server.use(http.get('*/api/v1/debt/get-debt', () => ok({ ...details, item: { ...details.item, amountBorrowed: '1500' } })))

  await openSettings(user)

  expect(await screen.findByLabelText('Amount borrowed')).toHaveValue('1500')
})

it('Settings saves the name, the payments sentence and the terms in one request', async () => {
  const bodies = captureUpdate((body) => ({ ...details.item, name: body.name as string, rules: [{ ...wireRule, labelId: 'label1', labelName: 'health' }] }))
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
  expect(bodies[0]).toMatchObject({ id: 'd1', name: 'Van loan', currencyId: '', rules: [{ id: 'r1', categoryId: 'cat-food', payeeId: null, tagId: null, labelId: 'label1' }] })
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
    current = body.amountBorrowed ? { ...current, amountBorrowed: '1500' } : { ...current, name: body.name as string }
    return current
  })
  const user = userEvent.setup()
  renderScreen()
  await openScreen()

  await openSettings(user)
  const borrowed = screen.getByLabelText('Amount borrowed')
  await user.clear(borrowed)
  await user.type(borrowed, '1500')
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(bodies).toHaveLength(1))
  await waitFor(() => expect(screen.getByRole('tab', { name: 'Overview' })).toHaveAttribute('aria-selected', 'true'))

  await openSettings(user)
  await waitFor(() => expect(screen.getByLabelText('Amount borrowed')).toHaveValue('1500'))
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
  expect(zone).toHaveTextContent("Sets what's owed to 0 and stops counting down the payments.")
  expect(zone).toHaveTextContent('Removes the debt. Its payments count in their categories again.')
})

it('Danger zone hides Mark as paid off while not set up', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok(notSetUp)))
  const user = userEvent.setup()
  renderScreen()
  await openScreen()
  const zone = await openDanger(user)

  expect(within(zone).queryByRole('button', { name: 'Mark as paid off' })).toBeNull()
  expect(within(zone).queryByText("Sets what's owed to 0 and stops counting down the payments.")).toBeNull()
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

it('after Mark as paid off, Settings starts from the paid-off debt, so a later edit sends no balance', async () => {
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

  await user.click(within(await openDanger(user)).getByRole('button', { name: 'Mark as paid off' }))
  await user.click(within(await screen.findByRole('dialog', { name: 'Mark as paid off?' })).getByRole('button', { name: 'Mark as paid off' }))
  // back on the Overview, like a save
  await waitFor(() => expect(screen.getByRole('tab', { name: 'Overview' })).toHaveAttribute('aria-selected', 'true'))

  await openSettings(user)
  await waitFor(() => expect(screen.getByLabelText('Payment')).toHaveValue('100'))
  const payment = screen.getByLabelText('Payment')
  await user.clear(payment)
  await user.type(payment, '120')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toMatchObject({ amountBorrowed: '1200', payment: '120' })
  expect(bodies[0]).not.toHaveProperty('balance')
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

// the form of the previous visit is not editable while its replacement loads: an edit made then
// would mark it dirty, drop the fresh debt and send the old loan facts back
it('a second visit to Settings shows the loader, not the previous form, while the debt reloads', async () => {
  const user = userEvent.setup()
  renderScreen()
  await openScreen()
  await openSettings(user)
  await user.click(screen.getByRole('tab', { name: 'Overview' }))
  let release: () => void = () => {}
  const gate = new Promise<void>((resolve) => {
    release = resolve
  })
  server.use(
    http.get('*/api/v1/debt/get-debt', async () => {
      await gate
      return ok({ ...details, item: { ...details.item, amountBorrowed: '1500' } })
    }),
  )

  await user.click(screen.getByRole('tab', { name: 'Settings' }))

  const panel = screen.getByRole('tabpanel', { name: 'Settings' })
  expect(within(panel).getByRole('status')).toBeInTheDocument()
  expect(within(panel).queryByLabelText('Name')).toBeNull()
  release()
  expect(await within(panel).findByLabelText('Amount borrowed')).toHaveValue('1500')
})

it('a removal_unconfirmed answer asks and resends with confirmDataRemoval', async () => {
  const bodies: Record<string, unknown>[] = []
  const refusal = 'This deletes the planned amounts and comments of this debt in its budget; confirm to continue.'
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
  const confirm = await screen.findByRole('dialog', { name: 'Delete this debt?' })
  // a debt has one budget, and the payments, a partner's included, stay as they are
  expect(confirm).toHaveTextContent('The debt leaves its budget. The payments themselves are not changed.')
  await user.click(within(confirm).getByRole('button', { name: 'Delete' }))

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
    paymentsLeft: 283,
    payoffDate: '2050-07-05',
    overpaymentLeft: '10',
    extraPlanned: '0',
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

  it('Show future payments loads the forecast into the top of the payments table, with its plan named', async () => {
    const asked = captureForecast()
    const user = userEvent.setup()
    renderScreen()
    useUiStore.getState().openDebtDetails('d1', 'overview')
    await screen.findByTestId('debt-review-top', {}, { timeout: 5000 })
    // the Overview's figures load it on open; the list stays closed
    await waitFor(() => expect(asked).toHaveLength(1))
    expect(screen.queryByTestId('debt-forecast-payment')).toBeNull()

    const show = screen.getByRole('button', { name: 'Show future payments' })
    expect(show).toHaveAttribute('aria-expanded', 'false')
    await user.click(show)

    const rows = await screen.findAllByTestId('debt-forecast-payment')
    expect(asked).toHaveLength(1)
    expect(asked[0].get('id')).toBe('d1')
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
    captureForecast({ ...forecast, items, budgetId: '', budgetName: '' })
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

  it('each opening counts once; the list shares the Overview\'s forecast, refetched when a plan changes', async () => {
    const asked = captureForecast()
    const user = userEvent.setup()
    const { queryClient } = renderScreenWithClient()
    await openScreen()
    await waitFor(() => expect(asked).toHaveLength(1))

    await user.click(screen.getByRole('button', { name: 'Show future payments' }))
    await screen.findAllByTestId('debt-forecast-payment')
    expect(asked).toHaveLength(1)
    expect(vi.mocked(trackEvent).mock.calls.filter(([m]) => m === METRICS.DEBT_FORECAST_VIEW)).toHaveLength(1)

    // a plan saved elsewhere invalidates it: refetched, not counted again
    await queryClient.invalidateQueries({ queryKey: queryKeys.debtForecast('d1') })
    await waitFor(() => expect(asked).toHaveLength(2))
    expect(vi.mocked(trackEvent).mock.calls.filter(([m]) => m === METRICS.DEBT_FORECAST_VIEW)).toHaveLength(1)

    // hidden and shown again: a new view of the same, current forecast
    await user.click(screen.getByRole('button', { name: 'Hide future payments' }))
    await user.click(screen.getByRole('button', { name: 'Show future payments' }))
    await screen.findAllByTestId('debt-forecast-payment')
    expect(asked).toHaveLength(2)
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

  it('says when only the regular payments count', async () => {
    captureForecast({ ...forecast, budgetId: '', budgetName: '' })
    const user = userEvent.setup()
    renderScreen()
    await openScreen()

    await user.click(screen.getByRole('button', { name: 'Show future payments' }))

    await screen.findAllByTestId('debt-forecast-payment')
    expect(screen.getByText('Regular payments only')).toBeInTheDocument()
  })

  it('a paid-off debt offers no future payments', async () => {
    server.use(http.get('*/api/v1/debt/get-debt', () => ok({ ...details, item: { ...details.item, isPaidOff: 1 } })))
    renderScreen()
    await openScreen()

    expect(screen.queryByRole('button', { name: 'Show future payments' })).toBeNull()
  })
})

const withStatement: DebtDetailsDto = {
  ...details,
  item: { ...details.item, balance: '1000', currentBalance: '990' },
  snapshots: [
    details.snapshots[0],
    { id: 's1', isOrigin: 0, computedBalance: '1120', asOf: '2026-10-31', balance: '1000', payment: '100', schedule: 'monthly', lastPaymentDate: '2050-07-05' },
  ],
}

async function openBalance(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('tab', { name: 'Balance' }))
  return screen.getByRole('tabpanel', { name: 'Balance' })
}

it('a debt not set up has no Balance tab', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok(notSetUp)))
  renderScreen()
  await openScreen()
  expect(screen.getAllByRole('tab').map((tab) => tab.textContent)).toEqual(['Overview', 'Settings', 'Danger zone'])
})

it('the Balance tab shows Econumo today, the last statement and the difference, and the history with the origin first', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok(withStatement)))
  const user = userEvent.setup()
  renderScreen()
  await openScreen()
  const panel = await openBalance(user)

  const summary = within(panel).getByTestId('debt-balance-summary')
  const figure = (label: string) => within(summary).getByText(label).nextElementSibling
  expect(figure("Econumo's balance today")).toHaveTextContent('990.00 $')
  expect(figure('Last statement')).toHaveTextContent('1,000.00 $ on Oct 31, 2026')
  expect(figure('Difference')).toHaveTextContent('-120.00 $')
  const rows = within(panel).getAllByTestId('debt-reconciliation')
  expect(rows).toHaveLength(2)
  expect(rows[0]).toHaveTextContent('Oct 1, 2026')
  expect(rows[0]).toHaveTextContent('Amount borrowed')
  expect(within(rows[0]).queryByRole('button')).toBeNull()
  expect(within(rows[1]).getAllByRole('cell').map((c) => c.textContent)).toEqual(['Oct 31, 2026', '1,000.00 $', '1,120.00 $', '-120.00 $', ''])
})

it('reconciling sends the statement balance and date and fires DEBT_RECONCILE', async () => {
  const bodies: Record<string, unknown>[] = []
  server.use(
    http.post('*/api/v1/debt/reconcile-debt', async ({ request }) => {
      bodies.push((await request.json()) as Record<string, unknown>)
      return ok({ ...details.item, currentBalance: '950' })
    }),
  )
  const user = userEvent.setup()
  renderScreen()
  await openScreen()
  const panel = await openBalance(user)

  await user.type(within(panel).getByLabelText('Statement balance'), '1 050')
  await user.click(within(panel).getByRole('button', { name: 'Statement date' }))
  await user.click(await screen.findByRole('button', { name: /November 2nd, 2026/ }))
  await user.click(within(panel).getByRole('button', { name: 'Reconcile' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toEqual({ id: 'd1', balance: '1050', balanceDate: '2026-11-02' })
  expect(trackEvent).toHaveBeenCalledWith(METRICS.DEBT_RECONCILE)
  await waitFor(() => expect(within(panel).getByLabelText('Statement balance')).toHaveValue(''))
})

it('the statement date defaults to today and must not precede the start date', async () => {
  let called = false
  server.use(
    http.get('*/api/v1/debt/get-debt', () => ok({ ...details, item: { ...details.item, startDate: '2026-11-05' } })),
    http.post('*/api/v1/debt/reconcile-debt', () => { called = true; return ok(details.item) }),
  )
  const user = userEvent.setup()
  renderScreen()
  await openScreen()
  const panel = await openBalance(user)

  expect(within(panel).getByRole('button', { name: 'Statement date' })).toHaveTextContent('Nov 8, 2026')
  await user.click(within(panel).getByRole('button', { name: 'Reconcile' }))
  expect(await within(panel).findByText('Required field')).toBeInTheDocument()
  await user.type(within(panel).getByLabelText('Statement balance'), '1000')
  await user.click(within(panel).getByRole('button', { name: 'Statement date' }))
  await user.click(await screen.findByRole('button', { name: /November 2nd, 2026/ }))
  await user.click(within(panel).getByRole('button', { name: 'Reconcile' }))
  expect(await within(panel).findByText(/Pick a date after the loan's start/)).toBeInTheDocument()
  expect(called).toBe(false)
})

it('deleting a statement asks first, then sends its id and fires DEBT_RECONCILIATION_DELETE', async () => {
  const bodies: Record<string, unknown>[] = []
  server.use(
    http.get('*/api/v1/debt/get-debt', () => ok(withStatement)),
    http.post('*/api/v1/debt/delete-debt-reconciliation', async ({ request }) => {
      bodies.push((await request.json()) as Record<string, unknown>)
      return ok(details.item)
    }),
  )
  const user = userEvent.setup()
  renderScreen()
  await openScreen()
  const panel = await openBalance(user)

  await user.click(within(panel).getByRole('button', { name: 'Delete the statement of Oct 31, 2026' }))
  const dialog = await screen.findByRole('dialog', { name: 'Delete this statement?' })
  await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toEqual({ id: 'd1', reconciliationId: 's1' })
  expect(trackEvent).toHaveBeenCalledWith(METRICS.DEBT_RECONCILIATION_DELETE)
})

it('a statement of 0 reads "Marked paid off", shows no difference, and deleting it says the debt reopens', async () => {
  const paidOff: DebtDetailsDto = {
    ...withStatement,
    item: { ...withStatement.item, balance: '0', currentBalance: '0', isPaidOff: 1 },
    snapshots: [withStatement.snapshots[0], { ...withStatement.snapshots[1], balance: '0', computedBalance: '120' }],
  }
  server.use(http.get('*/api/v1/debt/get-debt', () => ok(paidOff)))
  const user = userEvent.setup()
  renderScreen()
  await openScreen()
  const panel = await openBalance(user)

  expect(within(within(panel).getByTestId('debt-balance-summary')).queryByText('Difference')).toBeNull()
  const row = within(panel).getAllByTestId('debt-reconciliation')[1]
  expect(row).toHaveTextContent('Marked paid off')
  expect(within(row).getAllByRole('cell')[3]).toHaveTextContent('—')
  await user.click(within(row).getByRole('button', { name: 'Delete the statement of Oct 31, 2026' }))
  const dialog = await screen.findByRole('dialog', { name: 'Delete the paid-off mark?' })
  expect(dialog).toHaveTextContent('The debt will be open again')
})

it('the Overview has no history of terms: the Balance tab holds the history', async () => {
  renderScreen()
  await openScreen()
  expect(screen.queryByRole('button', { name: /History of terms/ })).toBeNull()
})
