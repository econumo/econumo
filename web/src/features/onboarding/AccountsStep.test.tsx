import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { AccountsStep } from './AccountsStep'
import type { AccountRow } from './draft'

function renderStep(props: Partial<React.ComponentProps<typeof AccountsStep>> = {}, handlers: Parameters<typeof coreHandlers>[0] = {}) {
  server.use(...coreHandlers(handlers))
  const onContinue = vi.fn()
  const onSkip = vi.fn()
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <AccountsStep onContinue={onContinue} onSkip={onSkip} pending={false} {...props} />
    </QueryClientProvider>,
  )
  return { onContinue, onSkip }
}

const nameInputs = () => screen.getAllByLabelText('Account name')
const balanceInputs = () => screen.getAllByLabelText('Balance')

async function ready() {
  // the default currency resolves once the currency list and profile load
  await waitFor(() => expect(screen.getAllByLabelText('Currency')[0]).toHaveTextContent('USD'))
}

async function pickCurrency(user: ReturnType<typeof userEvent.setup>, rowIndex: number, code: string) {
  await user.click(screen.getAllByLabelText('Currency')[rowIndex])
  await user.click(await screen.findByRole('option', { name: new RegExp(`^${code}`) }))
}

beforeEach(() => {
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it('appends an empty row once the last row is typed in', async () => {
  const user = userEvent.setup()
  renderStep()
  await ready()
  expect(nameInputs()).toHaveLength(1)
  await user.type(nameInputs()[0], 'Wallet')
  expect(nameInputs()).toHaveLength(2)
})

it('picks the cash icon for Cash and turns Savings on for a savings name', async () => {
  const user = userEvent.setup()
  renderStep()
  await ready()
  await user.type(nameInputs()[0], 'Cash')
  const first = screen.getAllByTestId('account-row')[0]
  expect(within(first).getByText('payments')).toBeInTheDocument()
  expect(within(first).getByRole('switch')).not.toBeChecked()
  await user.type(nameInputs()[1], 'Savings')
  expect(within(screen.getAllByTestId('account-row')[1]).getByRole('switch')).toBeChecked()
})

it('stops following the name once the savings switch is toggled', async () => {
  const user = userEvent.setup()
  renderStep()
  await ready()
  await user.click(within(screen.getAllByTestId('account-row')[0]).getByRole('switch'))
  await user.type(nameInputs()[0], 'Cash')
  expect(within(screen.getAllByTestId('account-row')[0]).getByRole('switch')).toBeChecked()
})

it('submits a typed balance as a normalized decimal string and the main currency', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await ready()
  await user.type(nameInputs()[0], 'Card')
  await user.type(balanceInputs()[0], '1,234.50')
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(onContinue).toHaveBeenCalledTimes(1)
  const [rows, extras, main] = onContinue.mock.calls[0]
  expect(rows).toHaveLength(1)
  expect(rows[0]).toMatchObject({ name: 'Card', balance: '1234.50', currencyId: 'cur-usd', icon: 'credit_card' })
  expect(extras).toEqual([])
  expect(main).toBe('cur-usd')
})

it('treats a blank balance as 0 and rejects garbage', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await ready()
  await user.type(nameInputs()[0], 'Card')
  await user.type(balanceInputs()[0], 'abc')
  expect(screen.getByText(/Enter a number/)).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
  await user.clear(balanceInputs()[0])
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(onContinue.mock.calls[0][0][0].balance).toBe('0')
})

it('reads a comma as the en group mark, so 2,500 is two and a half thousand', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await ready()
  await user.type(nameInputs()[0], 'Card')
  await user.type(balanceInputs()[0], '2,500')
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(onContinue.mock.calls[0][0][0].balance).toBe('2500')
})

it('flags a name outside 3-64 characters once the field is left and blocks Continue', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await ready()
  await user.type(nameInputs()[0], 'AB')
  expect(screen.queryByText('Account name must be 3-64 characters')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
  await user.click(balanceInputs()[0])
  expect(screen.getByText('Account name must be 3-64 characters')).toBeInTheDocument()
  expect(nameInputs()[0]).toHaveAttribute('aria-invalid', 'true')
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(onContinue).not.toHaveBeenCalled()

  await user.type(nameInputs()[0], 'C')
  expect(screen.queryByText('Account name must be 3-64 characters')).not.toBeInTheDocument()
  await user.clear(nameInputs()[0])
  await user.type(nameInputs()[0], 'x'.repeat(65))
  expect(screen.getByText('Account name must be 3-64 characters')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
})

it('needs one named row to continue', async () => {
  renderStep()
  await ready()
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
})

it('follows the majority currency for Main and drops a currency with its last row', async () => {
  const user = userEvent.setup()
  renderStep()
  await ready()
  await user.type(nameInputs()[0], 'A')
  await user.type(nameInputs()[1], 'B')
  await user.type(nameInputs()[2], 'C')
  await pickCurrency(user, 1, 'EUR')
  expect(screen.getByText('Main: USD · based on your accounts')).toBeInTheDocument()
  await pickCurrency(user, 0, 'EUR')
  expect(screen.getByText('Main: EUR · based on your accounts')).toBeInTheDocument()
  await user.click(screen.getAllByLabelText('Remove account')[0])
  await user.click(screen.getAllByLabelText('Remove account')[0])
  expect(screen.getByText('Main: USD · based on your accounts')).toBeInTheDocument()
  expect(screen.queryByText('EUR')).not.toBeInTheDocument()
})

it('adds an extra currency and passes it on', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await ready()
  await user.type(nameInputs()[0], 'Card')
  await user.click(screen.getByRole('button', { name: /Add currency/ }))
  await user.click(await screen.findByRole('option', { name: /^EUR/ }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(onContinue.mock.calls[0][1]).toEqual(['cur-eur'])
})

it('lets the user pick another selected currency as Main', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await ready()
  await user.type(nameInputs()[0], 'Card')
  await user.click(screen.getByRole('button', { name: /Add currency/ }))
  await user.click(await screen.findByRole('option', { name: /^EUR/ }))
  await user.click(screen.getByText('Main: USD · based on your accounts'))
  await user.click(await screen.findByRole('option', { name: /^EUR/ }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(onContinue.mock.calls[0][2]).toBe('cur-eur')
})

it('notes a selected currency that has no rate', async () => {
  const user = userEvent.setup()
  renderStep({}, { rates: [{ currencyId: 'cur-usd', baseCurrencyId: 'cur-usd', rate: '1', updatedAt: '2026-07-01 00:00:00' }] })
  await ready()
  await user.type(nameInputs()[0], 'Card')
  expect(screen.queryByText(/No exchange rate yet/)).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: /Add currency/ }))
  await user.click(await screen.findByRole('option', { name: /^EUR/ }))
  expect(screen.getByText(/No exchange rate yet for EUR/)).toBeInTheDocument()
})

it('calls onSkip', async () => {
  const user = userEvent.setup()
  const { onSkip } = renderStep()
  await user.click(screen.getByRole('button', { name: 'Skip setup' }))
  expect(onSkip).toHaveBeenCalledTimes(1)
})

it('seeds from initialRows and keeps created rows read-only but their Savings switch editable', async () => {
  const user = userEvent.setup()
  const rows: AccountRow[] = [
    { key: 'k1', name: 'Cash', balance: '10.00', currencyId: 'cur-usd', savings: false, icon: 'payments' },
    { key: 'k2', name: 'Pending', balance: '5', currencyId: 'cur-eur', savings: false, icon: 'credit_card' },
  ]
  const { onContinue } = renderStep({ initialRows: rows, initialExtraCurrencyIds: [], createdKeys: new Set(['k1']) })
  await ready()
  const rowEls = screen.getAllByTestId('account-row')
  expect(rowEls).toHaveLength(3)
  const created = rowEls[0]
  expect(within(created).getByLabelText('Account name')).toHaveAttribute('readonly')
  expect(within(created).getByLabelText('Balance')).toHaveAttribute('readonly')
  expect(within(created).getByLabelText('Currency')).toBeDisabled()
  expect(within(created).queryByLabelText('Remove account')).not.toBeInTheDocument()
  expect(within(rowEls[1]).getByLabelText('Remove account')).toBeInTheDocument()
  await user.click(within(created).getByRole('switch'))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  const [out] = onContinue.mock.calls[0]
  expect(out.map((r: AccountRow) => r.key)).toEqual(['k1', 'k2'])
  expect(out[0].savings).toBe(true)
})

it('ignores a bad balance on a row with no name', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await ready()
  await user.type(nameInputs()[0], 'Card')
  await user.type(balanceInputs()[1], 'abc')
  expect(screen.getByRole('button', { name: 'Continue' })).toBeEnabled()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(onContinue.mock.calls[0][0]).toHaveLength(1)
})

it('disables Skip setup while pending', async () => {
  renderStep({ pending: true })
  expect(screen.getByRole('button', { name: 'Skip setup' })).toBeDisabled()
})
