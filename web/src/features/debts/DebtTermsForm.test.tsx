import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { METRICS, trackEvent } from '@/lib/metrics'
import type { DebtDto } from '@/api/dto/debt'
import { DebtTermsForm } from './DebtTermsForm'
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

function renderForm(debt: DebtDto, mode: 'setup' | 'edit' = debt.isSetUp ? 'edit' : 'setup') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const onSaved = vi.fn()
  render(
    <QueryClientProvider client={queryClient}>
      <DebtTermsForm debt={debt} mode={mode} onSaved={onSaved} />
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

const lastPaymentChip = () => screen.getByRole('button', { name: 'Last payment date' })

it('editing the balance recomputes the last payment date', async () => {
  const user = userEvent.setup()
  renderForm(wireDebt)
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
  renderForm({ ...wireDebt, periodRate: '0.01' })

  const payment = screen.getByLabelText('Payment')
  await user.clear(payment)
  await user.type(payment, '600')

  // 1200 at 1% a period, 600 a payment: 3 payments, Dec, Jan, Feb
  expect(lastPaymentChip()).toHaveTextContent('2027-02-05')
})

it('a payment that never covers the interest says so and keeps the date', async () => {
  const user = userEvent.setup()
  renderForm({ ...wireDebt, periodRate: '0.05' })

  const payment = screen.getByLabelText('Payment')
  await user.clear(payment)
  await user.type(payment, '10')

  expect(await screen.findByText(/never pays off/)).toBeInTheDocument()
  expect(lastPaymentChip()).toHaveTextContent('2027-11-05')
})

it('an unusable balance or payment is not called a payment that never covers the interest', async () => {
  const user = userEvent.setup()
  renderForm({ ...wireDebt, periodRate: '0.05' })

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
  renderForm({ ...wireDebt, currentBalance: '900' })

  await user.click(lastPaymentChip())
  await user.click(await screen.findByRole('button', { name: /November 20th, 2027/ }))

  expect(lastPaymentChip()).toHaveTextContent('2027-11-20')
  expect(screen.getByLabelText('Balance now')).toHaveValue('900')
  expect(screen.getByLabelText('Payment')).toHaveValue('100')
  expect(screen.getByRole('combobox', { name: 'Repeats' })).toHaveTextContent('Monthly')
})

it('saving untouched terms sends the stored snapshot, not the balance shown now', async () => {
  const bodies = captureUpdate()
  const user = userEvent.setup()
  const onSaved = renderForm({ ...wireDebt, currentBalance: '900' })

  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(onSaved).toHaveBeenCalled())
  expect(bodies[0]).toMatchObject({
    id: 'd1', name: 'Car loan', categoryId: 'cat-food', labelId: null,
    balance: '1200', payment: '100', schedule: 'monthly', lastPaymentDate: '2027-11-05', startDate: null,
  })
  expect(trackEvent).toHaveBeenCalledWith(METRICS.DEBT_UPDATE)
  expect(trackEvent).not.toHaveBeenCalledWith(METRICS.DEBT_FINISH_SETUP)
})

it('a balance typed and put back is untouched: no new snapshot', async () => {
  const bodies = captureUpdate()
  const user = userEvent.setup()
  renderForm({ ...wireDebt, currentBalance: '900' })

  const balance = screen.getByLabelText('Balance now')
  await user.clear(balance)
  await user.type(balance, '500')
  await user.clear(balance)
  await user.type(balance, '900')
  expect(lastPaymentChip()).toHaveTextContent('2027-11-05')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toMatchObject({ balance: '1200', payment: '100', schedule: 'monthly', lastPaymentDate: '2027-11-05' })
})

it('a changed balance is sent as typed', async () => {
  const bodies = captureUpdate()
  const user = userEvent.setup()
  renderForm(wireDebt)

  const balance = screen.getByLabelText('Balance now')
  await user.clear(balance)
  await user.type(balance, '600')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toMatchObject({ balance: '600', payment: '100', lastPaymentDate: '2027-05-05' })
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

it('setup starts with payment and repeats filled, balance and date empty', () => {
  renderForm(wireDebtNotSetUp)
  expect(screen.getByLabelText('Balance now')).toHaveValue('')
  expect(screen.getByLabelText('Payment')).toHaveValue('100')
  expect(screen.getByRole('combobox', { name: 'Repeats' })).toHaveTextContent('Monthly')
  expect(lastPaymentChip()).toHaveTextContent('Choose a date')
})

it('setup does not fill the last payment date from the balance: there is no rate yet', async () => {
  const user = userEvent.setup()
  renderForm(wireDebtNotSetUp)
  await user.type(screen.getByLabelText('Balance now'), '600')
  expect(lastPaymentChip()).toHaveTextContent('Choose a date')
})

it('setup asks for the date once a balance is typed', async () => {
  let called = false
  server.use(http.post('*/api/v1/debt/update-debt', () => { called = true; return ok(wireDebt) }))
  const user = userEvent.setup()
  renderForm(wireDebtNotSetUp)
  await user.type(screen.getByLabelText('Balance now'), '600')
  await user.click(screen.getByRole('button', { name: 'Finish setup' }))

  expect(await screen.findByText('Required field')).toBeInTheDocument()
  expect(called).toBe(false)
})

it('DebtTermsForm in setup mode fires DEBT_FINISH_SETUP once', async () => {
  const bodies = captureUpdate(() => ({ ...wireDebt, balance: '600', lastPaymentDate: '2026-12-15' }))
  const user = userEvent.setup()
  const onSaved = renderForm(wireDebtNotSetUp)

  await user.type(screen.getByLabelText('Balance now'), '600')
  await user.click(lastPaymentChip())
  const now = new Date()
  const month = new Intl.DateTimeFormat('en', { month: 'long' }).format(now)
  await user.click(await screen.findByRole('button', { name: new RegExp(`${month} 15th, ${now.getFullYear()}`) }))
  await user.click(screen.getByRole('button', { name: 'Finish setup' }))

  await waitFor(() => expect(onSaved).toHaveBeenCalledTimes(1))
  expect(bodies[0]).toMatchObject({
    id: 'd1', balance: '600', payment: '100', schedule: 'monthly',
    lastPaymentDate: `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-15`,
  })
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
  await user.click(screen.getByRole('button', { name: 'Finish setup' }))

  await waitFor(() => expect(onSaved).toHaveBeenCalled())
  expect(bodies[0]).toMatchObject({ payment: '120', schedule: 'monthly' })
  expect(bodies[0]).not.toHaveProperty('balance')
  expect(bodies[0]).not.toHaveProperty('lastPaymentDate')
  expect(trackEvent).toHaveBeenCalledWith(METRICS.DEBT_UPDATE)
  expect(trackEvent).not.toHaveBeenCalledWith(METRICS.DEBT_FINISH_SETUP)
})
