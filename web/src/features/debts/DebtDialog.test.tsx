import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { onlineManager, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureCategories, fixtureWireBudget, fixtureWirePlan, planHandler } from '@/test/fixtures'
import { currentPeriod } from '@/features/budgets/period'
import { addMonths } from '@/features/budgets/planMath'
import { queryKeys } from '@/app/queryKeys'
import { useUiStore } from '@/app/uiStore'
import type { OpenDebtParams } from '@/app/uiStore'
import { DebtDialog } from './DebtDialog'
import { wireDebtNotSetUp } from './testDebt'

function mockMatchMedia() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderDialog() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  queryClient.setQueryData(queryKeys.categories, fixtureCategories)
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

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  mockMatchMedia()
  useUiStore.setState({ debtModal: null, debtDetailsId: null })
})

async function openCreate(params: Partial<OpenDebtParams> = {}) {
  useUiStore.getState().openDebtModal({ budgetId: 'b1', elementId: 'cat-food', ...params })
  await screen.findByRole('heading', { name: params.elementId === 'env-1' ? 'Living' : 'Food' })
}

it('the header is the element\'s icon and name, and the form asks only payment and repeats', async () => {
  renderDialog()
  await openCreate()

  expect(screen.getByRole('heading', { name: 'Food' })).toBeInTheDocument()
  expect(screen.getByText('Track a loan without an account. You can fine-tune it afterwards.')).toBeInTheDocument()
  expect(screen.getByLabelText('Payment')).toBeInTheDocument()
  expect(screen.getByRole('combobox', { name: 'Repeats' })).toHaveTextContent('every month')
  // everything else is the element's
  expect(screen.queryByLabelText('Name')).toBeNull()
  expect(screen.queryByRole('button', { name: 'Icon' })).toBeNull()
  expect(screen.queryByRole('button', { name: /^Category/ })).toBeNull()
  expect(screen.queryByRole('button', { name: /^Budget:/ })).toBeNull()
  expect(screen.queryByRole('button', { name: /^Currency/ })).toBeNull()
  expect(screen.queryByLabelText('Balance now')).toBeNull()
  expect(screen.queryByRole('button', { name: 'Last payment date' })).toBeNull()
  expect(screen.queryByText('Start date')).toBeNull()
  // icons are ligature text
  await waitFor(() => expect(screen.getByRole('heading', { name: 'Food' }).parentElement).toHaveTextContent('restaurant'))
})

it('an envelope\'s header is the envelope\'s name and icon, read from the budget', async () => {
  // the user has no default budget: the row's own budget is read
  server.use(
    http.get('*/api/v1/budget/get-budget', () => ok({ item: fixtureWireBudget })),
  )
  renderDialog()
  await openCreate({ elementId: 'env-1' })
  expect(screen.getByRole('heading', { name: 'Living' }).parentElement).toHaveTextContent('home')
})

it('a category row set to another currency shows that currency\'s symbol on the payment', async () => {
  const budget = JSON.parse(JSON.stringify(fixtureWireBudget))
  budget.structure.elements = budget.structure.elements.map((e: { id: string }) => (e.id === 'cat-food' ? { ...e, currencyId: 'cur-eur' } : e))
  server.use(http.get('*/api/v1/budget/get-budget', () => ok({ item: budget })))
  const user = userEvent.setup()
  renderDialog()
  await openCreate()
  await user.type(screen.getByLabelText('Payment'), '1,5')
  expect(await screen.findByText(/= 1\.50 €/)).toBeInTheDocument()
})

it('sends the budget, the element, the payment and the schedule', async () => {
  const bodies = captureCreate()
  const user = userEvent.setup()
  renderDialog()
  await openCreate({ elementId: 'cat-food', budgetId: 'b2' })

  await user.type(screen.getByLabelText('Payment'), '50')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toEqual({ budgetId: 'b2', elementId: 'cat-food', startDate: null, payment: '50', schedule: 'monthly' })
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
  await openCreate()

  await user.type(screen.getByLabelText('Payment'), typed)
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0].payment).toBe(sent)
})

it('a payment that is not a number is refused before sending', async () => {
  const bodies = captureCreate()
  const user = userEvent.setup()
  renderDialog()
  await openCreate()

  await user.type(screen.getByLabelText('Payment'), '5+5')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  expect(await screen.findByText('Enter a valid number')).toBeInTheDocument()
  expect(bodies).toHaveLength(0)
})

it('requires the payment before sending', async () => {
  const bodies = captureCreate()
  const user = userEvent.setup()
  renderDialog()
  await openCreate()

  await user.click(screen.getByRole('button', { name: 'Add' }))

  expect(await screen.findAllByText('Required field')).toHaveLength(1)
  expect(bodies).toHaveLength(0)
})

it('saving opens the debt\'s review screen', async () => {
  captureCreate()
  const user = userEvent.setup()
  renderDialog()
  await openCreate()

  await user.type(screen.getByLabelText('Payment'), '50')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(useUiStore.getState().debtModal).toBeNull())
  expect(useUiStore.getState().debtDetailsId).toBe('d-new')
  expect(useUiStore.getState().debtDetailsTab).toBe('overview')
})

it('an element that cannot carry a debt shows the refusal and stays open', async () => {
  const message = 'This category or envelope already tracks a debt. Mark that debt as paid off first.'
  const bodies = captureCreate(() =>
    HttpResponse.json({ success: false, message: 'Form validation error', code: 400, errors: { elementId: [message] } }, { status: 400 }),
  )
  const user = userEvent.setup()
  renderDialog()
  await openCreate()

  await user.type(screen.getByLabelText('Payment'), '50')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  expect(await screen.findByRole('alert')).toHaveTextContent(message)
  expect(bodies).toHaveLength(1)
  expect(screen.queryByText('Start date')).toBeNull()
  expect(useUiStore.getState().debtModal).not.toBeNull()
})

it('a start before the previous debt ended reveals the start date field and resends with it', async () => {
  const early = 'The start date must be after the end of the previous debt on this category or envelope.'
  const bodies = captureCreate((_, call) =>
    call === 1
      ? HttpResponse.json({ success: false, message: 'Form validation error', code: 400, errors: { startDate: [early] } }, { status: 400 })
      : ok({ ...wireDebtNotSetUp, id: 'd-new' }),
  )
  const user = userEvent.setup()
  renderDialog()
  await openCreate()

  await user.type(screen.getByLabelText('Payment'), '50')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  expect(await screen.findByText(early)).toBeInTheDocument()
  expect(useUiStore.getState().debtModal).not.toBeNull()

  await user.click(screen.getByRole('button', { name: 'Start date' }))
  const now = new Date()
  const month = new Intl.DateTimeFormat('en', { month: 'long' }).format(now)
  await user.click(await screen.findByRole('button', { name: new RegExp(month + ' 1st, ' + now.getFullYear()) }))
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(bodies).toHaveLength(2))
  expect(bodies[0].startDate).toBeNull()
  expect(bodies[1].startDate).toBe(now.getFullYear() + '-' + String(now.getMonth() + 1).padStart(2, '0') + '-01')
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

function planWith(cells: { actual: string; planned: string }[]) {
  const current = currentPeriod(1)
  return {
    ...fixtureWirePlan,
    months: [-3, -2, -1, 0].map((d) => addMonths(current, d)),
    structure: {
      ...fixtureWirePlan.structure,
      elements: fixtureWirePlan.structure.elements.map((e) => (e.id === 'cat-food' ? { ...e, cells } : e)),
    },
  }
}

describe('the prefilled payment', () => {
  it('is this period\'s plan when it is above the spending', async () => {
    server.use(planHandler(planWith([
      { actual: '10', planned: '' }, { actual: '10', planned: '' }, { actual: '10', planned: '' },
      { actual: '40', planned: '2647.82' },
    ])))
    renderDialog()
    await openCreate()
    await waitFor(() => expect(screen.getByLabelText('Payment')).toHaveValue('2647.82'))
  })

  it('is the average of the three periods before when this one has nothing', async () => {
    server.use(planHandler(planWith([
      { actual: '300', planned: '' }, { actual: '330', planned: '' }, { actual: '0', planned: '' },
      { actual: '0', planned: '' },
    ])))
    renderDialog()
    await openCreate()
    await waitFor(() => expect(screen.getByLabelText('Payment')).toHaveValue('210.00'))
  })

  it('never overwrites what was typed before the data arrived', async () => {
    let release: () => void = () => {}
    const gate = new Promise<void>((resolve) => {
      release = resolve
    })
    const plan = planWith([
      { actual: '0', planned: '' }, { actual: '0', planned: '' }, { actual: '0', planned: '' },
      { actual: '0', planned: '500' },
    ])
    server.use(
      http.get('*/api/v1/budget/get-budget-plan', async () => {
        await gate
        return ok({ item: plan })
      }),
    )
    renderDialog()
    await openCreate()
    await userEvent.type(screen.getByLabelText('Payment'), '75')
    release()
    await act(async () => {
      await new Promise((r) => setTimeout(r, 100))
    })
    expect(screen.getByLabelText('Payment')).toHaveValue('75')
  })
})

describe('Return by a date', () => {
  const now = new Date(2026, 10, 10, 12)
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(now)
  })
  afterEach(() => {
    vi.useRealTimers()
  })
  const monthName = new Intl.DateTimeFormat('en', { month: 'long' }).format(now)
  const ym = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
  const dayButton = (ordinal: string) => screen.findByRole('button', { name: new RegExp(`${monthName} ${ordinal}, ${now.getFullYear()}`) })

  async function chooseReturnByADate(user: ReturnType<typeof userEvent.setup>) {
    await user.click(screen.getByRole('combobox', { name: 'Repeats' }))
    await user.click(await screen.findByRole('option', { name: 'return by a date' }))
  }

  async function pick(user: ReturnType<typeof userEvent.setup>, field: string, ordinal: string) {
    await user.click(screen.getByRole('button', { name: field }))
    await user.click(await dayButton(ordinal))
  }

  it('swaps the payment for the amount and the return-by day', async () => {
    const user = userEvent.setup()
    renderDialog()
    await openCreate()
    await chooseReturnByADate(user)

    expect(screen.queryByLabelText('Payment')).toBeNull()
    expect(screen.getByLabelText('Amount')).toHaveAttribute('inputmode', 'decimal')
    expect(screen.getByRole('combobox', { name: 'Repeats' })).toHaveTextContent('return by a date')
    expect(screen.queryByRole('button', { name: 'Borrowed on' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Return by' })).toHaveTextContent('Choose a date')

    // back to a schedule: the payment form as before
    await user.click(screen.getByRole('combobox', { name: 'Repeats' }))
    await user.click(await screen.findByRole('option', { name: 'every month' }))
    expect(screen.getByLabelText('Payment')).toBeInTheDocument()
    expect(screen.queryByLabelText('Amount')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Return by' })).toBeNull()
  })

  it('sends the amount, today as the start and the return-by day in the one create, payment 0, and opens the debt', async () => {
    const bodies = captureCreate(() => ok({ ...wireDebtNotSetUp, id: 'd-new', isSetUp: 1, schedule: 'none', payment: '0' }))
    const user = userEvent.setup()
    renderDialog()
    await openCreate({ elementId: 'cat-food', budgetId: 'b2' })
    await chooseReturnByADate(user)

    await user.type(screen.getByLabelText('Amount'), '3 000')
    await pick(user, 'Return by', '15th')
    await user.click(screen.getByRole('button', { name: 'Add' }))

    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0]).toEqual({
      budgetId: 'b2', elementId: 'cat-food', schedule: 'none', payment: '0',
      amountBorrowed: '3000', startDate: `${ym}-10`, lastPaymentDate: `${ym}-15`,
    })
    await waitFor(() => expect(useUiStore.getState().debtDetailsId).toBe('d-new'))
  })

  it('refuses a missing amount or return-by day before sending', async () => {
    const bodies = captureCreate()
    const user = userEvent.setup()
    renderDialog()
    await openCreate()
    await chooseReturnByADate(user)

    await user.click(screen.getByRole('button', { name: 'Add' }))
    expect(await screen.findAllByText('Required field')).toHaveLength(2)
    expect(bodies).toHaveLength(0)
  })

  it('offers no return-by day up to today', async () => {
    const bodies = captureCreate()
    const user = userEvent.setup()
    renderDialog()
    await openCreate()
    await chooseReturnByADate(user)

    await user.click(screen.getByRole('button', { name: 'Return by' }))
    expect(await dayButton('10th')).toBeDisabled()
    expect(await dayButton('11th')).toBeEnabled()
    expect(bodies).toHaveLength(0)
  })
})
