import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { http, HttpResponse } from 'msw'
import { fixtureAccounts } from '@/test/fixtures'
import { AccountsStep, AccountsStepBody, type ExistingAccount } from './AccountsStep'
import { emptyDraft, loadDraft, saveDraft, type AccountRow } from './draft'

function renderStep(props: Partial<React.ComponentProps<typeof AccountsStep>> = {}, handlers: Parameters<typeof coreHandlers>[0] = {}) {
  server.use(...coreHandlers(handlers))
  const onContinue = vi.fn()
  const onSkip = vi.fn()
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <AccountsStep existing={[]} onContinue={onContinue} onSkip={onSkip} pending={false} {...props} />
    </QueryClientProvider>,
  )
  return { onContinue, onSkip }
}

const nameInputs = () => screen.getAllByLabelText('Account name')
const balanceInputs = () => screen.getAllByLabelText('Balance')
const rowsEls = () => screen.getAllByTestId('account-row')
const addAccount = (user: ReturnType<typeof userEvent.setup>) => user.click(screen.getByRole('button', { name: 'Add account' }))
const details = (user: ReturnType<typeof userEvent.setup>, i: number) => user.click(within(rowsEls()[i]).getByRole('button', { name: 'Account details' }))

async function ready() {
  // the default currency resolves once the currency list and profile load
  await waitFor(() => expect(screen.getByRole('button', { name: 'Main currency' })).toHaveTextContent(/[A-Z]{3} \(main\)/))
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

it('starts with one row and adds another with Add account', async () => {
  const user = userEvent.setup()
  renderStep()
  await ready()
  expect(nameInputs()).toHaveLength(1)
  await addAccount(user)
  expect(nameInputs()).toHaveLength(2)
  expect(nameInputs()[1]).toHaveFocus()
})

it('picks the cash icon for Cash and marks a savings name as a savings account', async () => {
  const user = userEvent.setup()
  renderStep()
  await ready()
  await user.type(nameInputs()[0], 'Cash')
  expect(within(rowsEls()[0]).getByText('payments')).toBeInTheDocument()
  expect(within(rowsEls()[0]).queryByText('Savings account')).not.toBeInTheDocument()
  await addAccount(user)
  await user.type(nameInputs()[1], 'Savings')
  expect(within(rowsEls()[1]).getByText('Savings account')).toBeInTheDocument()
  await details(user, 1)
  expect(within(rowsEls()[1]).getByRole('switch')).toBeChecked()
})

it('stops following the name once the savings switch is toggled', async () => {
  const user = userEvent.setup()
  renderStep()
  await ready()
  await details(user, 0)
  await user.click(within(rowsEls()[0]).getByRole('switch'))
  await user.type(nameInputs()[0], 'Cash')
  expect(within(rowsEls()[0]).getByRole('switch')).toBeChecked()
  expect(within(rowsEls()[0]).getByText('Savings account')).toBeInTheDocument()
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
  await user.type(nameInputs()[0], 'Aaa')
  await addAccount(user)
  await user.type(nameInputs()[1], 'Bbb')
  await addAccount(user)
  await user.type(nameInputs()[2], 'Ccc')
  await pickCurrency(user, 1, 'EUR')
  expect(screen.getByRole('button', { name: 'Main currency' })).toHaveTextContent('USD (main)')
  await pickCurrency(user, 0, 'EUR')
  expect(screen.getByRole('button', { name: 'Main currency' })).toHaveTextContent('EUR (main)')
  await details(user, 0)
  await user.click(screen.getByRole('button', { name: 'Remove account' }))
  await details(user, 0)
  await user.click(screen.getByRole('button', { name: 'Remove account' }))
  expect(screen.getByRole('button', { name: 'Main currency' })).toHaveTextContent('USD (main)')
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
  await user.click(screen.getByRole('button', { name: 'Main currency' }))
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
  await user.click(screen.getByRole('button', { name: 'Skip' }))
  expect(onSkip).toHaveBeenCalledTimes(1)
})

const imported: ExistingAccount = { id: 'acc-imp', name: 'Imported card', icon: 'credit_card', balance: '1234.5', currencyId: 'cur-eur' }
const rainy: ExistingAccount = { id: 'acc-sav', name: 'Rainy day savings', icon: 'savings', balance: '10', currencyId: 'cur-usd' }

it('lists existing accounts first, locked with their balance, and passes them on as existing rows', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep({ existing: [imported, rainy] })
  await ready()
  const first = rowsEls()[0]
  expect(first).toHaveTextContent('Imported card')
  expect(first).toHaveTextContent('1,234.50')
  expect(within(first).queryByLabelText('Account name')).not.toBeInTheDocument()
  expect(within(first).queryByLabelText('Balance')).not.toBeInTheDocument()
  expect(within(first).getByText('EUR')).toBeInTheDocument()
  // existing accounts need no new row; their savings flag is guessed from the name
  expect(nameInputs).toThrow()
  expect(within(rowsEls()[1]).getByText('Savings account')).toBeInTheDocument()
  await details(user, 0)
  expect(within(rowsEls()[0]).queryByRole('button', { name: 'Remove account' })).not.toBeInTheDocument()
  await user.click(within(rowsEls()[0]).getByRole('switch'))
  expect(screen.getByRole('button', { name: 'Continue' })).toBeEnabled()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  const [out] = onContinue.mock.calls[0]
  expect(out).toEqual([
    expect.objectContaining({ accountId: 'acc-imp', currencyId: 'cur-eur', savings: true }),
    expect.objectContaining({ accountId: 'acc-sav', currencyId: 'cur-usd', savings: true }),
  ])
})

it('reseeds from a stored draft: created rows show as existing under their keys, the rest stay editable', async () => {
  const user = userEvent.setup()
  const rows: AccountRow[] = [
    { key: 'k1', name: 'Imported card', balance: '10.00', currencyId: 'cur-eur', savings: false, icon: 'payments' },
    { key: 'k2', name: 'Pending', balance: '5', currencyId: 'cur-eur', savings: false, icon: 'credit_card' },
  ]
  const { onContinue } = renderStep({ existing: [imported], initialRows: rows, createdIds: { k1: 'acc-imp' } })
  await ready()
  expect(rowsEls()).toHaveLength(2)
  expect(within(rowsEls()[0]).queryByLabelText('Account name')).not.toBeInTheDocument()
  expect(nameInputs()).toHaveLength(1)
  expect(nameInputs()[0]).toHaveValue('Pending')
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  const [out] = onContinue.mock.calls[0]
  expect(out.map((r: AccountRow) => [r.key, r.accountId])).toEqual([['k1', 'acc-imp'], ['k2', undefined]])
})

it('ignores a bad balance on a row with no name', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await ready()
  await user.type(nameInputs()[0], 'Card')
  await addAccount(user)
  await user.type(balanceInputs()[1], 'abc')
  expect(screen.getByRole('button', { name: 'Continue' })).toBeEnabled()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(onContinue.mock.calls[0][0]).toHaveLength(1)
})

it('disables Skip while pending', async () => {
  renderStep({ pending: true })
  expect(screen.getByRole('button', { name: 'Skip' })).toBeDisabled()
})

describe('AccountsStepBody', () => {
  const ok = (data: unknown) => HttpResponse.json({ success: true, message: '', data })

  it('a stored draft with one account created resumes without creating it again, then moves on', async () => {
    localStorage.clear()
    const rows: AccountRow[] = [
      { key: 'k1', name: 'Cash', balance: '100.5', currencyId: 'cur-usd', savings: false, icon: 'payments' },
      { key: 'k2', name: 'Travel fund', balance: '20', currencyId: 'cur-usd', savings: true, icon: 'savings' },
    ]
    saveDraft('u1', { ...emptyDraft(), ops: { 'account:k1': 'op1', 'account:k2': 'op2' }, rows, created: { k1: 'a1' } })
    const created: string[] = []
    server.use(
      ...coreHandlers({ accounts: [fixtureAccounts[0]] }),
      http.post('*/api/v1/account/create-account', async ({ request }) => {
        const body = (await request.json()) as { name: string; id: string }
        created.push(body.name)
        return ok({ item: { ...fixtureAccounts[1], id: 'a-new', name: body.name }, transaction: null })
      }),
      http.post('*/api/v1/user/update-currency', () => ok({ user: {} })),
      http.post('*/api/v1/currency/hide-all-currencies', () => ok({})),
      http.post('*/api/v1/currency/show-currency', () => ok({})),
    )
    const onDone = vi.fn(() => Promise.resolve())
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const user = userEvent.setup()
    render(
      <QueryClientProvider client={queryClient}>
        <AccountsStepBody onDone={onDone} onSkip={() => Promise.resolve()} />
      </QueryClientProvider>,
    )
    await ready()
    expect(screen.getAllByText('Cash')).toHaveLength(1)
    expect(nameInputs()).toHaveLength(1)
    expect(nameInputs()[0]).toHaveValue('Travel fund')
    await user.click(screen.getByRole('button', { name: 'Continue' }))
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1))
    expect(created).toEqual(['Travel fund'])
    const d = loadDraft('u1')!
    expect(d.accountIds).toEqual(['a1', 'a-new'])
    expect(d.savingsAccountIds).toEqual(['a-new'])
  })
})
