import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
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

it('asks for the loan facts in order: borrowed, the dates, payment', () => {
  renderForm(wireDebt)
  // the name, icon, rules and currency are the element's, not edited here
  expect(screen.queryByLabelText('Name')).toBeNull()
  expect(screen.queryByRole('button', { name: 'Icon' })).toBeNull()
  expect(screen.queryByRole('button', { name: /^Category/ })).toBeNull()
  expect(screen.queryByRole('button', { name: /^Currency/ })).toBeNull()
  const borrowed = screen.getByLabelText('Amount borrowed')
  expect(borrowed).toHaveValue('1200')
  expect(startChip()).toHaveTextContent('Nov 5, 2026')
  expect(lastPaymentChip()).toHaveTextContent('Nov 5, 2027')
  expect(screen.getByRole('group', { name: 'Payment, Repeats' })).toBeInTheDocument()
  // a statement balance is the Balance tab's
  expect(screen.queryByLabelText(/Owed now/)).toBeNull()
  const order = [borrowed, startChip(), lastPaymentChip(), screen.getByLabelText('Payment')]
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

it('saving untouched terms sends none, nor the start date, so the server keeps the stored ones', async () => {
  const bodies = captureUpdate()
  const user = userEvent.setup()
  const onSaved = renderForm({ ...wireDebt, currentBalance: '900' })

  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(onSaved).toHaveBeenCalled())
  expect(bodies[0]).toEqual({ id: 'd1' })
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
  expect(bodies[0]).toMatchObject({ amountBorrowed: '1500', lastPaymentDate: '2027-11-05', payment: '100', schedule: 'monthly' })
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

it('Cancel puts back what was loaded', async () => {
  const user = userEvent.setup()
  renderForm(wireDebt)

  const borrowed = screen.getByLabelText('Amount borrowed')
  await user.clear(borrowed)
  await user.type(borrowed, '600')
  await user.click(screen.getByRole('button', { name: 'Cancel' }))

  expect(screen.getByLabelText('Amount borrowed')).toHaveValue('1200')
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
