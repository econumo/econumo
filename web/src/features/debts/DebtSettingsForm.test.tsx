import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureCategories, fixtureLabels, fixturePayees, fixtureTags, fixtureUsd } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { METRICS, trackEvent } from '@/lib/metrics'
import type { DebtDto } from '@/api/dto/debt'
import { DebtSettingsForm } from './DebtSettingsForm'
import { wireDebt, wireDebtNotSetUp } from './testDebt'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})

function mockMatchMedia() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

const ok = (data: unknown) => HttpResponse.json({ success: true, message: '', data })

function renderForm(debt: DebtDto, { focusMissing = false }: { focusMissing?: boolean } = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  queryClient.setQueryData(queryKeys.categories, fixtureCategories)
  queryClient.setQueryData(queryKeys.payees, fixturePayees)
  queryClient.setQueryData(queryKeys.tags, fixtureTags)
  queryClient.setQueryData(queryKeys.labels, fixtureLabels)
  const onSaved = vi.fn()
  const onFocused = vi.fn()
  render(
    <QueryClientProvider client={queryClient}>
      <DebtSettingsForm debt={debt} onSaved={onSaved} focusMissing={focusMissing} onFocused={onFocused} />
    </QueryClientProvider>,
  )
  return onSaved
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
  server.use(...coreHandlers())
  mockMatchMedia()
  vi.mocked(trackEvent).mockClear()
})

const TERM_KEYS = ['amountBorrowed', 'balance', 'payment', 'schedule', 'lastPaymentDate']

const startChip = () => screen.getByRole('button', { name: 'Start date' })
const lastPaymentChip = () => screen.getByRole('button', { name: 'Last payment date' })
const owedNow = () => screen.getByLabelText('Owed now (optional)')

it('asks for the loan facts in order: borrowed, the dates, payment, owed now, currency', () => {
  renderForm(wireDebt)
  expect(screen.getByLabelText('Name')).toHaveValue('Car loan')
  expect(screen.getByRole('button', { name: 'Category: Food' })).toBeInTheDocument()
  const borrowed = screen.getByLabelText('Amount borrowed')
  expect(borrowed).toHaveValue('1200')
  expect(startChip()).toHaveTextContent('2026-11-05')
  expect(lastPaymentChip()).toHaveTextContent('2027-11-05')
  expect(screen.getByRole('group', { name: 'Payment, Repeats' })).toBeInTheDocument()
  // the override starts empty: what is owed follows from the recorded payments
  expect(owedNow()).toHaveValue('')
  expect(screen.getByText("Only if recorded payments don't match your statement")).toBeInTheDocument()
  const order = [borrowed, startChip(), lastPaymentChip(), screen.getByLabelText('Payment'), owedNow(), screen.getByRole('button', { name: /^Currency/ })]
  for (let i = 1; i < order.length; i++) {
    expect(order[i - 1].compareDocumentPosition(order[i]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  }
  expect(screen.getAllByRole('button', { name: 'Save' })).toHaveLength(1)
  expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument()
})

it('the dates share one row: Start date first, Last payment date second', () => {
  renderForm(wireDebt)
  const row = screen.getByRole('group', { name: 'Start date, Last payment date' })
  const start = within(row).getByRole('button', { name: 'Start date' })
  const last = within(row).getByRole('button', { name: 'Last payment date' })
  expect(start.compareDocumentPosition(last) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  // the hints stay for screen readers, off the narrow cards
  expect(within(row).getByText('When the loan began')).toHaveClass('sr-only')
  expect(within(row).getByText('When the debt ends')).toHaveClass('sr-only')
  // a set-up debt keeps its start: nothing to clear
  expect(screen.queryByRole('button', { name: 'Clear the start date' })).toBeNull()
})

it('saving untouched terms sends none but the start date, so the server keeps the stored ones', async () => {
  const bodies = captureUpdate()
  const user = userEvent.setup()
  const onSaved = renderForm({ ...wireDebt, currentBalance: '900' })

  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(onSaved).toHaveBeenCalled())
  expect(bodies[0]).toMatchObject({ id: 'd1', name: 'Car loan', currencyId: '', categoryId: 'cat-food', labelId: null, startDate: '2026-11-05' })
  for (const key of TERM_KEYS) {
    expect(bodies[0]).not.toHaveProperty(key)
  }
  expect(trackEvent).toHaveBeenCalledWith(METRICS.DEBT_UPDATE)
  expect(trackEvent).not.toHaveBeenCalledWith(METRICS.DEBT_FINISH_SETUP)
})

it('a corrected amount borrowed sends every loan fact, typed plain', async () => {
  const bodies = captureUpdate()
  const user = userEvent.setup()
  renderForm(wireDebt)

  const borrowed = screen.getByLabelText('Amount borrowed')
  await user.clear(borrowed)
  await user.type(borrowed, '1 500')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toMatchObject({ amountBorrowed: '1500', startDate: '2026-11-05', lastPaymentDate: '2027-11-05', payment: '100', schedule: 'monthly' })
  expect(bodies[0]).not.toHaveProperty('balance')
})

it('an amount borrowed typed back as stored is untouched', async () => {
  const bodies = captureUpdate()
  const user = userEvent.setup()
  renderForm(wireDebt)

  const borrowed = screen.getByLabelText('Amount borrowed')
  await user.clear(borrowed)
  await user.type(borrowed, '1,200')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  for (const key of TERM_KEYS) {
    expect(bodies[0]).not.toHaveProperty(key)
  }
})

it('owed now goes alone, without the loan facts', async () => {
  const bodies = captureUpdate()
  const user = userEvent.setup()
  renderForm(wireDebt)

  const name = screen.getByLabelText('Name')
  await user.clear(name)
  await user.type(name, 'Van loan')
  await user.type(owedNow(), '1 100.50')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toMatchObject({ name: 'Van loan', balance: '1100.5', startDate: '2026-11-05' })
  expect(bodies[0]).not.toHaveProperty('amountBorrowed')
  expect(bodies[0]).not.toHaveProperty('payment')
})

it('a negative owed now is refused before sending', async () => {
  const bodies = captureUpdate()
  const user = userEvent.setup()
  renderForm(wireDebt)

  await user.type(owedNow(), '-5')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  expect(await screen.findByText('Enter a valid number')).toBeInTheDocument()
  expect(bodies).toHaveLength(0)
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
  const onSaved = renderForm(wireDebt)

  await user.click(screen.getByRole('button', { name: 'Save' }))

  expect(await screen.findByText('The payments do not cover the balance.')).toBeInTheDocument()
  expect(onSaved).not.toHaveBeenCalled()
})

it('setup starts with payment and repeats filled, the loan facts empty', () => {
  renderForm(wireDebtNotSetUp)
  expect(screen.getByLabelText('Amount borrowed')).toHaveValue('')
  expect(owedNow()).toHaveValue('')
  expect(screen.getByLabelText('Payment')).toHaveValue('100')
  expect(screen.getByRole('combobox', { name: 'Repeats' })).toHaveTextContent('every month')
  expect(startChip()).toHaveTextContent('Choose a date')
  expect(lastPaymentChip()).toHaveTextContent('Choose a date')
})

it('setup asks for the dates once an amount borrowed is typed', async () => {
  let called = false
  server.use(http.post('*/api/v1/debt/update-debt', () => { called = true; return ok(wireDebt) }))
  const user = userEvent.setup()
  renderForm(wireDebtNotSetUp)
  await user.type(screen.getByLabelText('Amount borrowed'), '600')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  expect(await screen.findByText('Required field')).toBeInTheDocument()
  expect(called).toBe(false)
})

it('setup asks for the loan facts once owed now is typed', async () => {
  const bodies = captureUpdate()
  const user = userEvent.setup()
  renderForm(wireDebtNotSetUp)
  await user.type(owedNow(), '600')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  expect((await screen.findAllByText('Required field')).length).toBeGreaterThan(0)
  expect(bodies).toHaveLength(0)
})

it('saving a debt not set up with the loan facts finishes its setup and fires DEBT_FINISH_SETUP once', async () => {
  const bodies = captureUpdate(() => wireDebt)
  const user = userEvent.setup()
  const onSaved = renderForm(wireDebtNotSetUp)

  await user.type(screen.getByLabelText('Amount borrowed'), '1200')
  const now = new Date()
  const month = new Intl.DateTimeFormat('en', { month: 'long' }).format(now)
  await user.click(startChip())
  await user.click(await screen.findByRole('button', { name: new RegExp(`${month} 1st, ${now.getFullYear()}`) }))
  await user.click(lastPaymentChip())
  await user.click(await screen.findByRole('button', { name: new RegExp(`${month} 15th, ${now.getFullYear()}`) }))
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(onSaved).toHaveBeenCalledTimes(1))
  const ym = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
  expect(bodies[0]).toMatchObject({
    id: 'd1', amountBorrowed: '1200', startDate: `${ym}-01`, lastPaymentDate: `${ym}-15`, payment: '100', schedule: 'monthly',
  })
  expect(bodies[0]).not.toHaveProperty('balance')
  const events = vi.mocked(trackEvent).mock.calls.map((c) => c[0])
  expect(events.filter((e) => e === METRICS.DEBT_FINISH_SETUP)).toHaveLength(1)
  expect(events).not.toContain(METRICS.DEBT_UPDATE)
})

it('setup saved without terms only updates the payment and is no finished setup', async () => {
  const bodies = captureUpdate(() => ({ ...wireDebtNotSetUp, payment: '120' }))
  const user = userEvent.setup()
  const onSaved = renderForm(wireDebtNotSetUp)

  const payment = screen.getByLabelText('Payment')
  await user.clear(payment)
  await user.type(payment, '120')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(onSaved).toHaveBeenCalled())
  expect(bodies[0]).toMatchObject({ payment: '120', schedule: 'monthly' })
  expect(bodies[0]).not.toHaveProperty('balance')
  expect(bodies[0]).not.toHaveProperty('amountBorrowed')
  expect(bodies[0]).not.toHaveProperty('lastPaymentDate')
  expect(trackEvent).toHaveBeenCalledWith(METRICS.DEBT_UPDATE)
  expect(trackEvent).not.toHaveBeenCalledWith(METRICS.DEBT_FINISH_SETUP)
})

// a currency changed on another device since the form loaded stays
it('shows the currency and sends none while it is kept, so the server keeps the stored one', async () => {
  const bodies = captureUpdate()
  const user = userEvent.setup()
  renderForm(wireDebt)

  await waitFor(() => expect(screen.getByRole('button', { name: /^Currency/ })).toHaveTextContent('USD'))
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toMatchObject({ currencyId: '' })
})

// sending the stored 1000.50 back would be rounded to 1001 yen on the server,
// a change of terms whose snapshot drops the payments made since
it('a currency changed alone sends no terms, even into a currency with fewer digits', async () => {
  server.use(
    http.get('*/api/v1/currency/get-currency-list', () =>
      ok({ items: [fixtureUsd, { ...fixtureUsd, id: 'cur-jpy', code: 'JPY', name: 'Yen', symbol: '¥', fractionDigits: 0 }] }),
    ),
  )
  const bodies = captureUpdate()
  const user = userEvent.setup()
  renderForm({ ...wireDebt, balance: '1000.50', currentBalance: '900.50', payment: '100.25' })

  await user.click(await screen.findByRole('button', { name: /^Currency: USD/ }))
  await user.click(within(await screen.findByRole('dialog', { name: 'Currency' })).getByText(/JPY/))
  expect(screen.getByRole('button', { name: /^Currency/ })).toHaveTextContent('JPY')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toMatchObject({ currencyId: 'cur-jpy' })
  for (const key of TERM_KEYS) {
    expect(bodies[0]).not.toHaveProperty(key)
  }
})

// a currency changed on another device since the form loaded stays
it('shows the currency and sends none while it is kept, so the server keeps the stored one', async () => {
  const bodies = captureUpdate()
  const user = userEvent.setup()
  renderForm(wireDebt)

  await waitFor(() => expect(screen.getByRole('button', { name: /^Currency/ })).toHaveTextContent('USD'))
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toMatchObject({ currencyId: '' })
})

// sending the stored 1000.50 back would be rounded to 1001 yen on the server
it('a currency changed alone sends no terms, even into a currency with fewer digits', async () => {
  server.use(
    http.get('*/api/v1/currency/get-currency-list', () =>
      ok({ items: [fixtureUsd, { ...fixtureUsd, id: 'cur-jpy', code: 'JPY', name: 'Yen', symbol: '¥', fractionDigits: 0 }] }),
    ),
  )
  const bodies = captureUpdate()
  const user = userEvent.setup()
  renderForm({ ...wireDebt, amountBorrowed: '1000.50', currentBalance: '900.50', payment: '100.25' })

  await user.click(await screen.findByRole('button', { name: /^Currency: USD/ }))
  await user.click(within(await screen.findByRole('dialog', { name: 'Currency' })).getByText(/JPY/))
  expect(screen.getByRole('button', { name: /^Currency/ })).toHaveTextContent('JPY')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toMatchObject({ currencyId: 'cur-jpy' })
  for (const key of TERM_KEYS) {
    expect(bodies[0]).not.toHaveProperty(key)
  }
})

it('payment and repeats read as one line', async () => {
  const bodies = captureUpdate()
  const user = userEvent.setup()
  renderForm(wireDebtNotSetUp)

  const card = screen.getByRole('group', { name: 'Payment, Repeats' })
  const repeats = within(card).getByRole('combobox', { name: 'Repeats' })
  expect(card).toContainElement(screen.getByLabelText('Payment'))
  await user.click(repeats)
  await user.click(await screen.findByRole('option', { name: 'every 2 weeks' }))
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toMatchObject({ schedule: 'biweekly' })
})

it('a rename and a new reporting tag go in one request with no terms and no currency', async () => {
  const bodies = captureUpdate()
  const user = userEvent.setup()
  renderForm({ ...wireDebt, currentBalance: '900' })

  const name = screen.getByLabelText('Name')
  await user.clear(name)
  await user.type(name, 'Van loan')
  await user.click(screen.getByRole('button', { name: 'Tag (optional): any tag' }))
  await user.click(within(await screen.findByRole('dialog', { name: 'Tag (optional)' })).getByText('health'))
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toMatchObject({ id: 'd1', name: 'Van loan', currencyId: '', categoryId: 'cat-food', tagId: null, labelId: 'label1' })
  for (const key of TERM_KEYS) {
    expect(bodies[0]).not.toHaveProperty(key)
  }
})

it('an empty name is refused before sending', async () => {
  const bodies = captureUpdate()
  const user = userEvent.setup()
  renderForm(wireDebt)

  await user.clear(screen.getByLabelText('Name'))
  await user.click(screen.getByRole('button', { name: 'Save' }))

  expect(await screen.findByText('Required field')).toBeInTheDocument()
  expect(bodies).toHaveLength(0)
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
  const onSaved = renderForm(wireDebt)

  await user.click(screen.getByRole('button', { name: 'Tag (optional): any tag' }))
  await user.click(within(await screen.findByRole('dialog', { name: 'Tag (optional)' })).getByText('health'))
  await user.click(screen.getByRole('button', { name: 'Save' }))

  expect(await screen.findByText('Another debt already counts these payments.')).toBeInTheDocument()
  expect(onSaved).not.toHaveBeenCalled()
})

it('Cancel puts back what was loaded', async () => {
  const user = userEvent.setup()
  renderForm(wireDebt)

  const name = screen.getByLabelText('Name')
  await user.clear(name)
  await user.type(name, 'Van loan')
  const borrowed = screen.getByLabelText('Amount borrowed')
  await user.clear(borrowed)
  await user.type(borrowed, '600')
  await user.type(owedNow(), '300')
  await user.click(screen.getByRole('button', { name: 'Cancel' }))

  expect(screen.getByLabelText('Name')).toHaveValue('Car loan')
  expect(screen.getByLabelText('Amount borrowed')).toHaveValue('1200')
  expect(owedNow()).toHaveValue('')
})

it('asked to, it focuses the first missing loan fact of a debt not set up', () => {
  renderForm(wireDebtNotSetUp, { focusMissing: true })
  expect(screen.getByLabelText('Amount borrowed')).toHaveFocus()
})

it('asked to, it focuses the start date once the amount borrowed is there', () => {
  renderForm({ ...wireDebtNotSetUp, amountBorrowed: '1200', isSetUp: 1, startDate: '' }, { focusMissing: true })
  expect(startChip()).toHaveFocus()
})

it('an amount typed with a separator shows how it is read, in the display format', async () => {
  const user = userEvent.setup()
  renderForm(wireDebt)

  const borrowed = screen.getByLabelText('Amount borrowed')
  await user.clear(borrowed)
  await user.type(borrowed, '1.500')
  expect(await screen.findByText('= 1.50 $')).toBeInTheDocument()
  await user.clear(borrowed)
  await user.type(borrowed, '1,500')
  expect(await screen.findByText('= 1,500.00 $')).toBeInTheDocument()
  // nothing to tell apart without a separator
  await user.clear(borrowed)
  await user.type(borrowed, '1500')
  expect(screen.queryByText(/^= /)).toBeNull()

  const payment = screen.getByLabelText('Payment')
  await user.clear(payment)
  await user.type(payment, '2 000,5')
  expect(await screen.findByText('= 2,000.50 $')).toBeInTheDocument()
})

it('a debt not set up can drop its start date', async () => {
  const bodies = captureUpdate(() => wireDebtNotSetUp)
  const user = userEvent.setup()
  renderForm({ ...wireDebtNotSetUp, startDate: '2026-03-01' })

  await user.click(screen.getByRole('button', { name: 'Clear the start date' }))
  expect(startChip()).toHaveTextContent('Choose a date')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toMatchObject({ startDate: null })
})
