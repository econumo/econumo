import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { useUiStore } from '@/app/uiStore'
import type { DebtDetailsDto } from '@/api/dto/debt'
import { DebtDetailsSheet } from './DebtDetailsSheet'
import { wireDebt, wireDebtNotSetUp } from './testDebt'

function mockMatchMedia() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderSheet() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/', element: <DebtDetailsSheet /> }], { initialEntries: ['/'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

const ok = (data: unknown) => HttpResponse.json({ success: true, message: '', data })

const details: DebtDetailsDto = {
  item: { ...wireDebt, currentBalance: '1090', paymentsLeft: 11, moneyLeft: '1100', overpaymentLeft: '10', overpaidSoFar: '15.5' },
  snapshots: [
    { asOf: '2026-10-01', balance: '1200', payment: '100', schedule: 'monthly', lastPaymentDate: '2027-11-05' },
  ],
  payments: [
    { transactionId: 't1', date: '2026-11-05 00:00:00', amount: '100', interest: '10', principal: '90' },
    { transactionId: 't2', date: '2026-10-05 00:00:00', amount: '100', interest: '5.5', principal: '94.5' },
  ],
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers(), http.get('*/api/v1/debt/get-debt', () => ok(details)))
  mockMatchMedia()
  useUiStore.setState({ debtDetailsId: null, debtModal: null })
})

async function openSheet() {
  useUiStore.getState().openDebtDetails('d1')
  await screen.findByRole('heading', { name: 'Car loan' })
  await screen.findByText('Interest')
}

it('the details sheet shows interest and principal per payment', async () => {
  renderSheet()
  await openSheet()

  expect(screen.getByRole('columnheader', { name: 'Interest' })).toBeInTheDocument()
  expect(screen.getByRole('columnheader', { name: 'Principal' })).toBeInTheDocument()
  const rows = screen.getAllByTestId('debt-payment')
  expect(rows).toHaveLength(2)
  const first = within(rows[0]).getAllByRole('cell').map((c) => c.textContent)
  expect(first).toEqual(['2026-11-05', '100.00 $', '10.00 $', '90.00 $'])
  const second = within(rows[1]).getAllByRole('cell').map((c) => c.textContent)
  expect(second).toEqual(['2026-10-05', '100.00 $', '5.50 $', '94.50 $'])
})

it('shows the figures and the snapshot history', async () => {
  renderSheet()
  await openSheet()

  const figure = (label: string) => screen.getByText(label).nextElementSibling
  expect(figure('Balance now')).toHaveTextContent('1,090.00 $')
  expect(figure('Payments left')).toHaveTextContent('11 payments left')
  expect(figure('Left to pay')).toHaveTextContent('1,100.00 $')
  expect(figure('Overpayment left')).toHaveTextContent('10.00 $')
  expect(figure('Overpaid so far')).toHaveTextContent('15.50 $')
  expect(figure('Payoff date')).toHaveTextContent('2027-11-05')
  const snapshots = screen.getAllByTestId('debt-snapshot')
  expect(snapshots).toHaveLength(1)
  expect(snapshots[0]).toHaveTextContent('2026-10-01')
  expect(snapshots[0]).toHaveTextContent('1,200.00 $')
})

it('Edit hands the debt to the form and closes the sheet', async () => {
  const user = userEvent.setup()
  renderSheet()
  await openSheet()

  await user.click(screen.getByRole('button', { name: 'Edit' }))

  await waitFor(() => expect(useUiStore.getState().debtModal?.debt?.id).toBe('d1'))
  expect(useUiStore.getState().debtDetailsId).toBeNull()
})

it('Edit pre-fills from the debt as it is now, not as the sheet loaded it', async () => {
  const user = userEvent.setup()
  renderSheet()
  await openSheet()
  // a payment landed since the sheet loaded
  server.use(http.get('*/api/v1/debt/get-debt', () => ok({ ...details, item: { ...details.item, currentBalance: '990' } })))

  await user.click(screen.getByRole('button', { name: 'Edit' }))

  await waitFor(() => expect(useUiStore.getState().debtModal?.debt?.currentBalance).toBe('990'))
  expect(useUiStore.getState().debtDetailsId).toBeNull()
})

it('marking a debt paid off asks first, then posts', async () => {
  let called = 0
  server.use(http.post('*/api/v1/debt/mark-debt-paid-off', () => { called++; return ok(wireDebt) }))
  const user = userEvent.setup()
  renderSheet()
  await openSheet()

  await user.click(screen.getByRole('button', { name: 'Mark as paid off' }))
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
  renderSheet()
  await openSheet()

  await user.click(screen.getByRole('button', { name: 'Delete' }))
  const first = await screen.findByRole('dialog', { name: 'Delete this debt?' })
  await user.click(within(first).getByRole('button', { name: 'Delete' }))

  // the server's own words say what goes
  const second = await screen.findByRole('dialog', { name: 'Delete plans and comments?' })
  expect(within(second).getByText(refusal)).toBeInTheDocument()
  expect(bodies).toEqual([{ id: 'd1' }])
  await user.click(within(second).getByRole('button', { name: 'Delete debt' }))

  await waitFor(() => expect(bodies).toHaveLength(2))
  expect(bodies[1]).toEqual({ id: 'd1', confirmDataRemoval: true })
  await waitFor(() => expect(useUiStore.getState().debtDetailsId).toBeNull())
})

it('a delete the server accepts closes the sheet without a second question', async () => {
  const bodies: Record<string, unknown>[] = []
  server.use(
    http.post('*/api/v1/debt/delete-debt', async ({ request }) => {
      bodies.push((await request.json()) as Record<string, unknown>)
      return ok({})
    }),
  )
  const user = userEvent.setup()
  renderSheet()
  await openSheet()

  await user.click(screen.getByRole('button', { name: 'Delete' }))
  const dialog = await screen.findByRole('dialog', { name: 'Delete this debt?' })
  await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

  await waitFor(() => expect(useUiStore.getState().debtDetailsId).toBeNull())
  expect(bodies).toEqual([{ id: 'd1' }])
})

it('a debt that fails to load says why instead of showing an empty sheet', async () => {
  server.use(
    http.get('*/api/v1/debt/get-debt', () =>
      HttpResponse.json({ success: false, message: 'Debt not found', code: 0, errors: {} }, { status: 404 }),
    ),
  )
  renderSheet()
  useUiStore.getState().openDebtDetails('d1')
  expect(await screen.findByRole('alert')).toHaveTextContent('Debt not found')
})

it('a debt not set up shows no figures, offers Finish setup and not Mark as paid off', async () => {
  server.use(http.get('*/api/v1/debt/get-debt', () => ok({ item: wireDebtNotSetUp, snapshots: [], payments: [] })))
  renderSheet()
  useUiStore.getState().openDebtDetails('d1')
  await screen.findByRole('heading', { name: 'Car loan' })

  const figure = (label: string) => screen.getByText(label).nextElementSibling
  expect(figure('Balance now')).toHaveTextContent('—')
  expect(figure('Payments left')).toHaveTextContent('—')
  expect(screen.getByRole('button', { name: 'Finish setup' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Mark as paid off' })).toBeDisabled()
})
