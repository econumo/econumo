import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18n from '@/app/i18n'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { http, HttpResponse } from 'msw'
import { fixtureAccounts } from '@/test/fixtures'
import { AccountsStep, AccountsStepBody, type ExistingAccount } from './AccountsStep'
import { emptyDraft, loadDraft, saveDraft, type AccountRow } from './draft'
import { stepRoutes, userWith } from './onboardingRoutes.test-utils'

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
const removeRow = (user: ReturnType<typeof userEvent.setup>, i: number) => user.click(within(rowsEls()[i]).getByRole('button', { name: 'Remove account' }))

async function ready() {
  // the default currency resolves once the currency list and profile load
  await waitFor(() => expect(screen.getByRole('button', { name: 'Main currency' })).toHaveTextContent(/[A-Z]{3} \(main\)/), { timeout: 3000 })
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

it('starts a user with no accounts on a prefilled Cash row: cash icon, blank balance, no savings switch', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await ready()
  expect(nameInputs()).toHaveLength(1)
  expect(nameInputs()[0]).toHaveValue('Cash')
  expect(balanceInputs()[0]).toHaveValue('')
  expect(within(rowsEls()[0]).getByText('payments')).toBeInTheDocument()
  expect(screen.queryByRole('switch')).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Account details' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(onContinue.mock.calls[0][0][0]).not.toHaveProperty('savings')
  expect(onContinue.mock.calls[0][0]).toEqual([expect.objectContaining({ name: 'Cash', balance: '0', currencyId: 'cur-usd', icon: 'payments' })])
  expect(onContinue.mock.calls[0][2]).toBe('cur-usd')
})

describe('in the UI language', () => {
  afterEach(async () => {
    await act(() => i18n.changeLanguage('en'))
  })

  it('re-labels the untouched Cash row on a language switch and submits the current label', async () => {
    const user = userEvent.setup()
    const { onContinue } = renderStep()
    await ready()
    await act(() => i18n.changeLanguage('de'))
    const name = screen.getByLabelText('Kontoname')
    expect(name).toHaveValue('Bargeld')
    expect(within(rowsEls()[0]).getByText('payments')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Weiter' }))
    expect(onContinue.mock.calls[0][0][0]).toMatchObject({ name: 'Bargeld', labelKey: 'onboarding.accounts.default_cash', icon: 'payments' })
  })

  it('keeps a renamed Cash row as typed after a language switch', async () => {
    const user = userEvent.setup()
    renderStep()
    await ready()
    await user.clear(nameInputs()[0])
    await user.type(nameInputs()[0], 'Wallet')
    await act(() => i18n.changeLanguage('de'))
    expect(screen.getByLabelText('Kontoname')).toHaveValue('Wallet')
  })

  it('a stored draft row with a label key reloads in the current language', async () => {
    await act(() => i18n.changeLanguage('de'))
    renderStep({ initialRows: [{ key: 'k1', name: 'Cash', labelKey: 'onboarding.accounts.default_cash', balance: '', currencyId: 'cur-eur', icon: 'payments' }] })
    expect(await screen.findByLabelText('Kontoname')).toHaveValue('Bargeld')
  })
})

it('gives no Cash row to a user who already has accounts', () => {
  renderStep({ existing: [{ id: 'a1', name: 'Wallet', icon: 'payments', balance: '5', currencyId: 'cur-eur' }] })
  expect(screen.queryByDisplayValue('Cash')).not.toBeInTheDocument()
})

it('gives no Cash row to a resumed draft with rows', () => {
  renderStep({ initialRows: [{ key: 'k1', name: 'Card', balance: '1', currencyId: 'cur-eur', icon: 'credit_card' }] })
  expect(screen.queryByDisplayValue('Cash')).not.toBeInTheDocument()
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

it('picks the cash icon for Cash and never guesses the piggy bank from a savings name', async () => {
  const user = userEvent.setup()
  renderStep()
  await ready()
  await user.clear(nameInputs()[0])
  await user.type(nameInputs()[0], 'Cash')
  expect(within(rowsEls()[0]).getByText('payments')).toBeInTheDocument()
  await addAccount(user)
  await user.type(nameInputs()[1], 'Savings')
  expect(within(rowsEls()[1]).getByText('credit_card')).toBeInTheDocument()
  expect(within(rowsEls()[1]).queryByText('savings')).not.toBeInTheDocument()
})

it('removes a new row with its inline remove button', async () => {
  const user = userEvent.setup()
  renderStep()
  await ready()
  await addAccount(user)
  await user.type(nameInputs()[1], 'Card')
  await removeRow(user, 1)
  expect(nameInputs()).toHaveLength(1)
  expect(nameInputs()[0]).toHaveValue('Cash')
})

it('submits a typed balance as a normalized decimal string and the main currency', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await ready()
  await user.clear(nameInputs()[0])
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
  await user.clear(nameInputs()[0])
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
  const user = userEvent.setup()
  renderStep()
  await ready()
  await user.clear(nameInputs()[0])
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
})

it("uses the first row's currency for the other untouched rows, new rows and Main, and keeps a hand-picked row", async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await ready()
  await user.type(nameInputs()[0], 'Aaa')
  await addAccount(user)
  await user.type(nameInputs()[1], 'Bbb')
  await addAccount(user)
  await user.type(nameInputs()[2], 'Ccc')
  await pickCurrency(user, 1, 'USD')
  await pickCurrency(user, 0, 'EUR')
  const codes = () => screen.getAllByLabelText('Currency').map((el) => el.textContent)
  expect(codes()).toEqual(['EUR', 'USD', 'EUR'])
  await addAccount(user)
  await user.type(nameInputs()[3], 'Ddd')
  expect(codes()).toEqual(['EUR', 'USD', 'EUR', 'EUR'])
  expect(screen.getByRole('button', { name: 'Main currency' })).toHaveTextContent('EUR (main)')
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  const [rows, , main] = onContinue.mock.calls[0]
  expect(rows.map((r: AccountRow) => r.currencyId)).toEqual(['cur-eur', 'cur-usd', 'cur-eur', 'cur-eur'])
  expect(main).toBe('cur-eur')
})

it('lets an explicit Main pick win over the first row', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await ready()
  await user.type(nameInputs()[0], 'Aaa')
  await addAccount(user)
  await user.type(nameInputs()[1], 'Bbb')
  await pickCurrency(user, 0, 'EUR')
  await user.click(screen.getByRole('button', { name: 'Main currency' }))
  await user.click(await screen.findByRole('option', { name: /^EUR/ }))
  await pickCurrency(user, 0, 'USD')
  await pickCurrency(user, 1, 'EUR')
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(onContinue.mock.calls[0][2]).toBe('cur-eur')
})

it('with only EUR accounts and a blank row, Main is the existing currency, not the default', async () => {
  const user = userEvent.setup()
  const existing: ExistingAccount[] = [{ id: 'a1', name: 'Wallet', icon: 'payments', balance: '5', currencyId: 'cur-eur' }]
  const { onContinue } = renderStep({ existing })
  await ready()
  await addAccount(user)
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(onContinue.mock.calls[0][2]).toBe('cur-eur')
})

it('resumes with the first draft row currency as the default for rows added later', async () => {
  const user = userEvent.setup()
  const initialRows: AccountRow[] = [
    { key: 'k1', name: 'Aaa', balance: '1', currencyId: 'cur-eur', icon: 'credit_card' },
    { key: 'k2', name: 'Bbb', balance: '1', currencyId: 'cur-usd', icon: 'credit_card' },
  ]
  renderStep({ initialRows })
  await ready()
  await addAccount(user)
  expect(screen.getAllByLabelText('Currency').map((el) => el.textContent)).toEqual(['EUR', 'USD', 'EUR'])
})

it('removing the first row makes the new first row currency the default', async () => {
  const user = userEvent.setup()
  renderStep()
  await ready()
  await user.type(nameInputs()[0], 'Aaa')
  await addAccount(user)
  await user.type(nameInputs()[1], 'Bbb')
  await pickCurrency(user, 1, 'EUR')
  await removeRow(user, 0)
  await addAccount(user)
  expect(screen.getAllByLabelText('Currency').map((el) => el.textContent)).toEqual(['EUR', 'EUR'])
})

it("Main follows the first row's pick even while that row is blank, and clears when the only row goes", async () => {
  const user = userEvent.setup()
  renderStep()
  await ready()
  await addAccount(user)
  await user.type(nameInputs()[1], 'Bbb')
  await pickCurrency(user, 1, 'USD')
  await pickCurrency(user, 0, 'EUR')
  const main = () => screen.getByRole('button', { name: 'Main currency' })
  expect(main()).toHaveTextContent('EUR (main)')
  await user.type(nameInputs()[0], 'Aaa')
  expect(main()).toHaveTextContent('EUR (main)')
  await removeRow(user, 1)
  await removeRow(user, 0)
  await addAccount(user)
  expect(screen.getAllByLabelText('Currency').map((el) => el.textContent)).toEqual(['USD'])
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

it('says which currencies to pick under the Currencies label', async () => {
  renderStep()
  await ready()
  expect(screen.getByText('Select the currencies you use most often. You can add more later in Settings.')).toBeInTheDocument()
})

it('changes a new row\'s icon through the icon picker and keeps it when the name changes', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await ready()
  await user.click(within(rowsEls()[0]).getByRole('button', { name: 'Change icon' }))
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByRole('option', { name: 'account_balance_wallet' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(within(rowsEls()[0]).getByText('account_balance_wallet')).toBeInTheDocument()
  await user.clear(nameInputs()[0])
  await user.type(nameInputs()[0], 'Savings')
  expect(within(rowsEls()[0]).getByText('account_balance_wallet')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(onContinue.mock.calls[0][0]).toEqual([expect.objectContaining({ name: 'Savings', icon: 'account_balance_wallet' })])
})

it('keeps the icon of an existing account read-only', async () => {
  renderStep({ existing: [imported] })
  await ready()
  expect(within(rowsEls()[0]).queryByRole('button', { name: 'Change icon' })).not.toBeInTheDocument()
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
  // existing accounts need no new row, and have no remove button or savings switch
  expect(nameInputs).toThrow()
  expect(within(rowsEls()[0]).queryByRole('button', { name: 'Remove account' })).not.toBeInTheDocument()
  expect(screen.queryByRole('switch')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Continue' })).toBeEnabled()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  const [out] = onContinue.mock.calls[0]
  expect(out).toEqual([
    expect.objectContaining({ accountId: 'acc-imp', currencyId: 'cur-eur' }),
    expect.objectContaining({ accountId: 'acc-sav', currencyId: 'cur-usd' }),
  ])
})

it('reseeds from a stored draft: rows standing for existing accounts stay locked, the rest stay editable', async () => {
  const user = userEvent.setup()
  const rows: AccountRow[] = [
    { key: 'k1', name: 'Imported card', balance: '10.00', currencyId: 'cur-eur', icon: 'payments', accountId: 'acc-imp' },
    { key: 'k2', name: 'Pending', balance: '5', currencyId: 'cur-eur', icon: 'credit_card' },
  ]
  const { onContinue } = renderStep({ existing: [imported], initialRows: rows })
  await ready()
  expect(rowsEls()).toHaveLength(2)
  expect(within(rowsEls()[0]).queryByLabelText('Account name')).not.toBeInTheDocument()
  expect(nameInputs()).toHaveLength(1)
  expect(nameInputs()[0]).toHaveValue('Pending')
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  const [out] = onContinue.mock.calls[0]
  expect(out.map((r: AccountRow) => [r.key, r.accountId])).toEqual([['account:acc-imp', 'acc-imp'], ['k2', undefined]])
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
  const renderBody = (onDone = vi.fn(() => Promise.resolve())) => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={queryClient}>
        <AccountsStepBody onDone={onDone} onSkip={() => Promise.resolve()} />
      </QueryClientProvider>,
    )
    return { onDone, queryClient }
  }

  it('Continue posts one save-accounts with the new rows, the currencies and the main one, then moves on', async () => {
    localStorage.clear()
    const routes = stepRoutes({ option: 'categories' })
    server.use(...coreHandlers({ accounts: [fixtureAccounts[0]], user: userWith('categories') }), ...routes.handlers)
    const user = userEvent.setup()
    const { onDone } = renderBody()
    await ready()
    await addAccount(user)
    await user.type(nameInputs()[0], 'Travel fund')
    await user.type(balanceInputs()[0], '20')
    await user.click(screen.getByRole('button', { name: 'Continue' }))
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1))
    expect(routes.srv.calls).toHaveLength(1)
    const body = routes.srv.calls[0].body
    expect(body.accounts).toEqual([{ key: expect.any(String), name: 'Travel fund', balance: '20', currencyId: body.mainCurrencyId, icon: expect.any(String) }])
    expect(body.currencyIds).toEqual(expect.arrayContaining([fixtureAccounts[0].currency.id, body.mainCurrencyId]))
    expect(loadDraft('u1')!.budgetId).toBe('b-onb')
  })

  it('a stored draft from a failed submit reseeds the form and replays its operation id', async () => {
    localStorage.clear()
    const rows: AccountRow[] = [{ key: 'k2', name: 'Travel fund', balance: '20', currencyId: 'cur-usd', icon: 'savings' }]
    saveDraft('u1', { ...emptyDraft(), ops: { accounts: 'op-acc' }, rows })
    const routes = stepRoutes()
    server.use(...coreHandlers({ accounts: [fixtureAccounts[0]], user: userWith('') }), ...routes.handlers)
    const user = userEvent.setup()
    const { onDone } = renderBody()
    await ready()
    expect(nameInputs()).toHaveLength(1)
    expect(nameInputs()[0]).toHaveValue('Travel fund')
    await user.click(screen.getByRole('button', { name: 'Continue' }))
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1))
    expect(routes.srv.calls.map((c) => [c.body.id, c.body.accounts.map((a: AccountRow) => a.key)])).toEqual([['op-acc', ['k2']]])
    expect(loadDraft('u1')!.ops.accounts).toBeUndefined()
  })

  it('a failed save shows the error, stays on the step and keeps the id for the retry', async () => {
    localStorage.clear()
    const routes = stepRoutes()
    routes.srv.failNext.accounts = 'http500'
    server.use(...coreHandlers({ accounts: [], user: userWith('') }), ...routes.handlers)
    const user = userEvent.setup()
    const { onDone } = renderBody()
    await ready()
    await user.click(screen.getByRole('button', { name: 'Continue' }))
    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(onDone).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Continue' }))
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1))
    expect(routes.srv.calls[1].body.id).toBe(routes.srv.calls[0].body.id)
  })
})

describe('AccountsStepBody recovery', () => {
  const ok = (data: unknown) => HttpResponse.json({ success: true, message: '', data })

  it('a failed account list shows the error with a retry instead of an empty form', async () => {
    let fail = true
    server.use(
      http.get('*/api/v1/account/get-account-list', () =>
        fail ? HttpResponse.json({ success: false, message: 'Server down' }, { status: 500 }) : ok({ items: fixtureAccounts }),
      ),
      ...coreHandlers(),
    )
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const user = userEvent.setup()
    render(
      <QueryClientProvider client={queryClient}>
        <AccountsStepBody onDone={() => Promise.resolve()} onSkip={() => Promise.resolve()} />
      </QueryClientProvider>,
    )
    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Continue' })).not.toBeInTheDocument()
    fail = false
    await user.click(screen.getByRole('button', { name: 'Try again' }))
    expect(await screen.findByText('Euro Stash')).toBeInTheDocument()
  })
})
