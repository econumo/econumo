import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18n from '@/app/i18n'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { http, HttpResponse } from 'msw'
import { fixtureAccounts, fixtureEur, fixtureUsd } from '@/test/fixtures'
import * as metrics from '@/lib/metrics'
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

const mainChip = () => screen.getByRole('button', { name: /^Main currency/ })

async function ready() {
  // the default currency resolves once the currency list and profile load
  await waitFor(() => expect(mainChip()).toHaveAccessibleName(/^Main currency: [A-Z]{3}$/), { timeout: 3000 })
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
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
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
    await user.click(screen.getByRole('button', { name: 'Speichern und weiter' }))
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
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
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
  expect(screen.getByRole('button', { name: 'Save and continue' })).toBeDisabled()
  await user.clear(balanceInputs()[0])
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
  expect(onContinue.mock.calls[0][0][0].balance).toBe('0')
})

it('reads a comma as the en group mark, so 2,500 is two and a half thousand', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await ready()
  await user.type(nameInputs()[0], 'Card')
  await user.type(balanceInputs()[0], '2,500')
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
  expect(onContinue.mock.calls[0][0][0].balance).toBe('2500')
})

it('flags a name outside 3-64 characters once the field is left and blocks Continue', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await ready()
  await user.clear(nameInputs()[0])
  await user.type(nameInputs()[0], 'AB')
  expect(screen.queryByText('Account name must be 3-64 characters')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Save and continue' })).toBeDisabled()
  await user.click(balanceInputs()[0])
  expect(screen.getByText('Account name must be 3-64 characters')).toBeInTheDocument()
  expect(nameInputs()[0]).toHaveAttribute('aria-invalid', 'true')
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
  expect(onContinue).not.toHaveBeenCalled()

  await user.type(nameInputs()[0], 'C')
  expect(screen.queryByText('Account name must be 3-64 characters')).not.toBeInTheDocument()
  await user.clear(nameInputs()[0])
  await user.type(nameInputs()[0], 'x'.repeat(65))
  expect(screen.getByText('Account name must be 3-64 characters')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Save and continue' })).toBeDisabled()
})

it('needs one named row to continue', async () => {
  const user = userEvent.setup()
  renderStep()
  await ready()
  await user.clear(nameInputs()[0])
  expect(screen.getByRole('button', { name: 'Save and continue' })).toBeDisabled()
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
  expect(mainChip()).toHaveAccessibleName('Main currency: EUR')
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
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
  await user.click(mainChip())
  await user.click(await screen.findByRole('option', { name: /^EUR/ }))
  await pickCurrency(user, 0, 'USD')
  await pickCurrency(user, 1, 'EUR')
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
  expect(onContinue.mock.calls[0][2]).toBe('cur-eur')
})

it('with only EUR accounts and a blank row, Main is the existing currency, not the default', async () => {
  const user = userEvent.setup()
  const existing: ExistingAccount[] = [{ id: 'a1', name: 'Wallet', icon: 'payments', balance: '5', currencyId: 'cur-eur' }]
  const { onContinue } = renderStep({ existing })
  await ready()
  await addAccount(user)
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
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
  expect(mainChip()).toHaveAccessibleName('Main currency: EUR')
  await user.type(nameInputs()[0], 'Aaa')
  expect(mainChip()).toHaveAccessibleName('Main currency: EUR')
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
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
  expect(onContinue.mock.calls[0][1]).toEqual(['cur-eur'])
})

it('lets the user pick another selected currency as Main', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await ready()
  await user.type(nameInputs()[0], 'Card')
  await user.click(screen.getByRole('button', { name: /Add currency/ }))
  await user.click(await screen.findByRole('option', { name: /^EUR/ }))
  await user.click(mainChip())
  await user.click(await screen.findByRole('option', { name: /^EUR/ }))
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
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
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
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

it('lists existing accounts first with their name locked and their balance editable, and passes them on as existing rows', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep({ existing: [imported, rainy] })
  await ready()
  const first = rowsEls()[0]
  expect(first).toHaveTextContent('Imported card')
  expect(within(first).getByLabelText('Balance of Imported card')).toHaveValue('1234.50')
  expect(within(first).queryByLabelText('Account name')).not.toBeInTheDocument()
  expect(within(first).getByText('EUR')).toBeInTheDocument()
  // existing accounts need no new row, and have no remove button or savings switch
  expect(nameInputs).toThrow()
  expect(within(rowsEls()[0]).queryByRole('button', { name: 'Remove account' })).not.toBeInTheDocument()
  expect(screen.queryByRole('switch')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Save and continue' })).toBeEnabled()
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
  const [out] = onContinue.mock.calls[0]
  expect(out).toEqual([
    expect.objectContaining({ accountId: 'acc-imp', currencyId: 'cur-eur' }),
    expect.objectContaining({ accountId: 'acc-sav', currencyId: 'cur-usd' }),
  ])
})

it('reviews existing accounts: Archive and Restore flip rows, hidden ones read muted, and Continue sends the diff', async () => {
  const user = userEvent.setup()
  const archived: ExistingAccount = { id: 'acc-old', name: 'Old card', icon: 'credit_card', balance: '0', currencyId: 'cur-usd', hidden: true }
  const { onContinue } = renderStep({ existing: [imported, rainy, archived] })
  await ready()
  expect(screen.queryByText(/Archive folder/)).not.toBeInTheDocument()
  const oldRow = rowsEls()[2]
  expect(oldRow).toHaveAttribute('data-hidden', 'true')
  expect(within(oldRow).getByText('Archived')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Archive Imported card' }))
  expect(rowsEls()[0]).toHaveAttribute('data-hidden', 'true')
  await user.click(screen.getByRole('button', { name: 'Restore Old card' }))
  expect(rowsEls()[2]).not.toHaveAttribute('data-hidden')
  // flipped twice is no change
  await user.click(screen.getByRole('button', { name: 'Archive Rainy day savings' }))
  await user.click(screen.getByRole('button', { name: 'Restore Rainy day savings' }))
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
  expect(onContinue.mock.calls[0][3]).toEqual({ hide: ['acc-imp'], unhide: ['acc-old'], currencies: [], balances: [] })
})

it('an existing account takes a currency from its chip; Continue sends it as a currencies diff and the name stays locked', async () => {
  const user = userEvent.setup()
  const lead: ExistingAccount = { id: 'acc-lead', name: 'Main card', icon: 'credit_card', balance: '5', currencyId: 'cur-usd' }
  const other: ExistingAccount = { id: 'acc-other', name: 'Other card', icon: 'credit_card', balance: '7', currencyId: 'cur-usd' }
  const { onContinue } = renderStep({ existing: [lead, other] })
  await ready()
  await pickCurrency(user, 1, 'EUR')
  expect(within(rowsEls()[1]).getByRole('button', { name: 'Currency' })).toHaveTextContent('EUR')
  expect(within(rowsEls()[1]).queryByLabelText('Account name')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
  const [rows, , main, review] = onContinue.mock.calls[0]
  expect(review.currencies).toEqual([{ accountId: 'acc-other', currencyId: 'cur-eur' }])
  expect(rows).toEqual([
    expect.objectContaining({ accountId: 'acc-lead', currencyId: 'cur-usd' }),
    expect.objectContaining({ accountId: 'acc-other', currencyId: 'cur-eur' }),
  ])
  // the first row is unchanged, so the main currency is too
  expect(main).toBe('cur-usd')
})

it('the first existing account leads: untouched rows, existing or new, follow its pick and so does the main currency', async () => {
  const user = userEvent.setup()
  const lead: ExistingAccount = { id: 'acc-lead', name: 'Main card', icon: 'credit_card', balance: '5', currencyId: 'cur-usd' }
  const follower: ExistingAccount = { id: 'acc-follow', name: 'Second card', icon: 'credit_card', balance: '7', currencyId: 'cur-usd' }
  const byHand: ExistingAccount = { id: 'acc-hand', name: 'Third card', icon: 'credit_card', balance: '9', currencyId: 'cur-eur' }
  const { onContinue } = renderStep({ existing: [lead, follower, byHand] })
  await ready()
  await pickCurrency(user, 2, 'USD')
  await addAccount(user)
  await user.type(nameInputs()[0], 'Travel fund')
  await pickCurrency(user, 0, 'EUR')
  expect(within(rowsEls()[1]).getByRole('button', { name: 'Currency' })).toHaveTextContent('EUR')
  expect(within(rowsEls()[2]).getByRole('button', { name: 'Currency' })).toHaveTextContent('USD')
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
  const [rows, , main, review] = onContinue.mock.calls[0]
  expect(review.currencies).toEqual([
    { accountId: 'acc-lead', currencyId: 'cur-eur' },
    { accountId: 'acc-follow', currencyId: 'cur-eur' },
    { accountId: 'acc-hand', currencyId: 'cur-usd' },
  ])
  expect(rows[3]).toMatchObject({ name: 'Travel fund', currencyId: 'cur-eur' })
  expect(main).toBe('cur-eur')
  expect(mainChip()).toHaveAccessibleName('Main currency: EUR')
})

it('an existing account\'s balance is editable: a changed one goes out in balances, an unreadable one blocks Continue', async () => {
  const user = userEvent.setup()
  const lead: ExistingAccount = { id: 'acc-lead', name: 'Main card', icon: 'credit_card', balance: '5', currencyId: 'cur-usd' }
  const other: ExistingAccount = { id: 'acc-other', name: 'Other card', icon: 'credit_card', balance: '7.5', currencyId: 'cur-usd' }
  const { onContinue } = renderStep({ existing: [lead, other] })
  await ready()
  const leadBalance = screen.getByLabelText('Balance of Main card')
  expect(leadBalance).toHaveValue('5.00')
  await user.clear(leadBalance)
  await user.type(leadBalance, 'abc')
  expect(screen.getByText('Enter a number, for example 1234.50')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Save and continue' })).toBeDisabled()
  await user.clear(leadBalance)
  await user.type(leadBalance, '1,250.75')
  // retyping the same amount is no change
  await user.clear(screen.getByLabelText('Balance of Other card'))
  await user.type(screen.getByLabelText('Balance of Other card'), '7.50')
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
  const [rows, , , review] = onContinue.mock.calls[0]
  expect(review.balances).toEqual([{ accountId: 'acc-lead', balance: '1250.75' }])
  expect(rows[0]).toMatchObject({ accountId: 'acc-lead', balance: '1250.75' })
})

it('labels the toggle Archive / Restore and shows no hint under the list', async () => {
  const archived: ExistingAccount = { id: 'acc-old', name: 'Old card', icon: 'credit_card', balance: '0', currencyId: 'cur-usd', hidden: true }
  renderStep({ existing: [imported, archived] })
  await ready()
  expect(screen.getByRole('button', { name: 'Archive Imported card' })).toHaveTextContent('Archive')
  expect(screen.getByRole('button', { name: 'Restore Old card' })).toHaveTextContent('Restore')
  expect(screen.queryByText(/Archive folder/)).not.toBeInTheDocument()
})

const gbp = { id: 'cur-gbp', code: 'GBP', name: 'Pound', symbol: '£', fractionDigits: 2, scope: 'global', isHidden: 0, isDeleted: 0 }

it('currencies are one row of chips: the main one first and filled, then the others, then Add currency', async () => {
  renderStep({ existing: [rainy] }, { currencies: [fixtureUsd, fixtureEur, gbp] })
  await ready()
  const list = screen.getByRole('list', { name: 'Selected currencies' })
  expect(within(list).getAllByRole('button').map((b) => b.textContent)).toEqual(['USD· Main', 'Add currency'])
  expect(screen.getByText('Tap a currency to make it main or remove it.')).toBeInTheDocument()
})

it('the main chip opens the full currency list: the pick becomes main and joins the chips, the old main stays', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep({ existing: [rainy] }, { currencies: [fixtureUsd, fixtureEur, gbp] })
  await ready()
  await user.click(mainChip())
  expect(screen.getAllByRole('option').map((o) => o.textContent?.slice(0, 3))).toEqual(expect.arrayContaining(['USD', 'EUR', 'GBP']))
  await user.click(screen.getByRole('option', { name: /^GBP/ }))
  await waitFor(() => expect(mainChip()).toHaveAccessibleName('Main currency: GBP'))
  expect(screen.getByRole('button', { name: 'USD' })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
  expect(onContinue.mock.calls[0][1]).toEqual(['cur-gbp'])
  expect(onContinue.mock.calls[0][2]).toBe('cur-gbp')
})

it('a non-main chip offers Make main and Remove; an account\'s currency cannot be removed', async () => {
  const user = userEvent.setup()
  const euro: ExistingAccount = { id: 'acc-eur', name: 'Euro card', icon: 'credit_card', balance: '1', currencyId: 'cur-eur' }
  const { onContinue } = renderStep({ existing: [rainy, euro] }, { currencies: [fixtureUsd, fixtureEur, gbp] })
  await ready()
  // EUR is a listed account's currency: Make main only
  await user.click(screen.getByRole('button', { name: 'EUR' }))
  expect(await screen.findByRole('menuitem', { name: 'Make main' })).toBeInTheDocument()
  expect(screen.queryByRole('menuitem', { name: 'Remove' })).not.toBeInTheDocument()
  await user.keyboard('{Escape}')
  // an added one can be removed
  await user.click(screen.getByRole('button', { name: /Add currency/ }))
  await user.click(await screen.findByRole('option', { name: /^GBP/ }))
  await user.click(await screen.findByRole('button', { name: 'GBP' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Remove' }))
  await waitFor(() => expect(screen.queryByRole('button', { name: 'GBP' })).not.toBeInTheDocument())
  // Make main moves the main chip
  await user.click(screen.getByRole('button', { name: 'EUR' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Make main' }))
  await waitFor(() => expect(mainChip()).toHaveAccessibleName('Main currency: EUR'))
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
  expect(onContinue.mock.calls[0][1]).toEqual([])
  expect(onContinue.mock.calls[0][2]).toBe('cur-eur')
})

it('Add currency only adds: the main currency stays', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep({ existing: [rainy] }, { currencies: [fixtureUsd, fixtureEur, gbp] })
  await ready()
  await user.click(screen.getByRole('button', { name: /Add currency/ }))
  await user.click(await screen.findByRole('option', { name: /^GBP/ }))
  expect(await screen.findByRole('button', { name: 'GBP' })).toBeInTheDocument()
  expect(mainChip()).toHaveAccessibleName('Main currency: USD')
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
  expect(onContinue.mock.calls[0][1]).toEqual(['cur-gbp'])
  expect(onContinue.mock.calls[0][2]).toBe('cur-usd')
})

it('cannot continue with every account hidden and no new row', async () => {
  const user = userEvent.setup()
  renderStep({ existing: [imported] })
  await ready()
  await user.click(screen.getByRole('button', { name: 'Archive Imported card' }))
  expect(screen.getByRole('button', { name: 'Save and continue' })).toBeDisabled()
})

it('withdraws the untouched Cash row once accounts of the user arrive', async () => {
  server.use(...coreHandlers())
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const props = { onContinue: vi.fn(), onSkip: vi.fn(), pending: false }
  const ui = (existing: ExistingAccount[]) => (
    <QueryClientProvider client={queryClient}>
      <AccountsStep existing={existing} {...props} />
    </QueryClientProvider>
  )
  const { rerender } = render(ui([]))
  expect(nameInputs()[0]).toHaveValue('Cash')
  rerender(ui([imported]))
  expect(screen.queryByLabelText('Account name')).not.toBeInTheDocument()
  expect(rowsEls()).toHaveLength(1)
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
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
  const [out] = onContinue.mock.calls[0]
  expect(out.map((r: AccountRow) => [r.key, r.accountId])).toEqual([['account:acc-imp', 'acc-imp'], ['k2', undefined]])
})

it('a draft restored after a failed submit keeps the untouched accounts following the first row', async () => {
  const gbp = { id: 'cur-gbp', code: 'GBP', name: 'Pound', symbol: '£', fractionDigits: 2, scope: 'global', isHidden: 0, isDeleted: 0 }
  const card: ExistingAccount = { id: 'acc-card', name: 'Card', icon: 'credit_card', balance: '5', currencyId: 'cur-usd' }
  const cash: ExistingAccount = { id: 'acc-cash', name: 'Cash', icon: 'payments', balance: '7', currencyId: 'cur-usd' }
  const user = userEvent.setup()
  const first = renderStep({ existing: [card, cash] }, { currencies: [fixtureUsd, fixtureEur, gbp] })
  await ready()
  await pickCurrency(user, 0, 'EUR')
  expect(within(rowsEls()[1]).getByRole('button', { name: 'Currency' })).toHaveTextContent('EUR')
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
  // the submit failed: the form comes back from the rows it sent
  const [sent] = first.onContinue.mock.calls[0]
  cleanup()
  const second = renderStep({ existing: [card, cash], initialRows: sent }, { currencies: [fixtureUsd, fixtureEur, gbp] })
  await ready()
  expect(within(rowsEls()[1]).getByRole('button', { name: 'Currency' })).toHaveTextContent('EUR')
  await pickCurrency(user, 0, 'GBP')
  expect(within(rowsEls()[1]).getByRole('button', { name: 'Currency' })).toHaveTextContent('GBP')
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
  expect(second.onContinue.mock.calls[0][3].currencies).toEqual([
    { accountId: 'acc-card', currencyId: 'cur-gbp' },
    { accountId: 'acc-cash', currencyId: 'cur-gbp' },
  ])
})

it('a cleared balance on an existing account is no change, never a zero balance', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep({ existing: [imported] })
  await ready()
  await user.clear(screen.getByLabelText('Balance of Imported card'))
  expect(screen.getByRole('button', { name: 'Save and continue' })).toBeEnabled()
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
  const [rows, , , review] = onContinue.mock.calls[0]
  expect(review.balances).toEqual([])
  expect(rows[0]).toMatchObject({ accountId: 'acc-imp', balance: '' })
})

it('ignores a bad balance on a row with no name', async () => {
  const user = userEvent.setup()
  const { onContinue } = renderStep()
  await ready()
  await user.type(nameInputs()[0], 'Card')
  await addAccount(user)
  await user.type(balanceInputs()[1], 'abc')
  expect(screen.getByRole('button', { name: 'Save and continue' })).toBeEnabled()
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
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
    await user.click(screen.getByRole('button', { name: 'Save and continue' }))
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1))
    expect(routes.srv.calls).toHaveLength(1)
    const body = routes.srv.calls[0].body
    expect(body.accounts).toEqual([{ key: expect.any(String), name: 'Travel fund', balance: '20', currencyId: body.mainCurrencyId, icon: expect.any(String) }])
    expect(body.currencyIds).toEqual(expect.arrayContaining([fixtureAccounts[0].currency.id, body.mainCurrencyId]))
    expect(loadDraft('u1')!.budgetId).toBe('b-onb')
    expect(body.hide).toBeUndefined()
    expect(body.unhide).toBeUndefined()
  })

  it('waits for the refetch of an account list cached before an import: no Cash row is offered or created', async () => {
    localStorage.clear()
    const routes = stepRoutes({ option: 'import,categories' })
    server.use(...coreHandlers({ accounts: [fixtureAccounts[0]], user: userWith('import,categories') }), ...routes.handlers)
    const user = userEvent.setup()
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    queryClient.setQueryData(['accounts'], [])
    void queryClient.invalidateQueries({ queryKey: ['accounts'], refetchType: 'none' })
    const onDone = vi.fn(() => Promise.resolve())
    render(
      <QueryClientProvider client={queryClient}>
        <AccountsStepBody onDone={onDone} onSkip={() => Promise.resolve()} />
      </QueryClientProvider>,
    )
    await ready()
    expect(screen.queryByLabelText('Account name')).not.toBeInTheDocument()
    expect(rowsEls()[0]).toHaveTextContent(fixtureAccounts[0].name)
    await user.click(screen.getByRole('button', { name: 'Save and continue' }))
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1))
    expect(routes.srv.calls[0].body.accounts).toEqual([])
  })

  it('an account in a hidden folder shows as hidden; Restore and Archive go out as unhide and hide', async () => {
    localStorage.clear()
    const routes = stepRoutes({ option: 'categories' })
    const inHidden = { ...fixtureAccounts[1], id: 'a-hid', name: 'Old savings', folderId: 'f-hidden' }
    server.use(...coreHandlers({ accounts: [fixtureAccounts[0], inHidden], user: userWith('categories') }), ...routes.handlers)
    const track = vi.spyOn(metrics, 'trackEvent').mockImplementation(() => undefined)
    const user = userEvent.setup()
    const { onDone } = renderBody()
    await ready()
    expect(rowsEls()[1]).toHaveAttribute('data-hidden', 'true')
    await user.click(screen.getByRole('button', { name: 'Restore Old savings' }))
    await user.click(screen.getByRole('button', { name: `Archive ${fixtureAccounts[0].name}` }))
    await user.click(screen.getByRole('button', { name: 'Save and continue' }))
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1))
    expect(routes.srv.calls[0].body).toMatchObject({ accounts: [], hide: [fixtureAccounts[0].id], unhide: ['a-hid'] })
    expect(track).toHaveBeenCalledWith(metrics.METRICS.ONBOARDING_ACCOUNTS_COMPLETE, expect.objectContaining({ accounts: 0, hidden: 1, shown: 1 }))
    track.mockRestore()
  })

  it('changing the first imported account\'s currency posts it and the untouched accounts that follow as currencies, with the main currency', async () => {
    localStorage.clear()
    const routes = stepRoutes({ option: 'import,categories' })
    server.use(...coreHandlers({ accounts: fixtureAccounts, user: userWith('import,categories') }), ...routes.handlers)
    const user = userEvent.setup()
    const { onDone } = renderBody()
    await ready()
    // Cash (USD) leads; Bank (USD) follows; Euro Stash is EUR already; the hidden account keeps its currency
    await pickCurrency(user, 0, 'EUR')
    await user.click(screen.getByRole('button', { name: 'Save and continue' }))
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1))
    const body = routes.srv.calls[0].body
    expect(body.currencies).toEqual([
      { accountId: 'a1', currencyId: 'cur-eur' },
      { accountId: 'a2', currencyId: 'cur-eur' },
    ])
    expect(body.mainCurrencyId).toBe('cur-eur')
    expect(body.accounts).toEqual([])
    expect(body.currencyIds).toContain('cur-eur')
  })

  it('an edited imported balance goes out in balances with the locale\'s decimal mark', async () => {
    localStorage.clear()
    await act(() => i18n.changeLanguage('de'))
    try {
      const routes = stepRoutes({ option: 'import,categories' })
      server.use(...coreHandlers({ accounts: [fixtureAccounts[0]], user: userWith('import,categories') }), ...routes.handlers)
      const user = userEvent.setup()
      const { onDone } = renderBody()
      await waitFor(() => expect(screen.getByLabelText('Kontostand von Cash')).toHaveValue('100,50'))
      await user.clear(screen.getByLabelText('Kontostand von Cash'))
      await user.type(screen.getByLabelText('Kontostand von Cash'), '80,25')
      await user.click(screen.getByRole('button', { name: 'Speichern und weiter' }))
      await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1))
      expect(routes.srv.calls[0].body.balances).toEqual([{ accountId: 'a1', balance: '80.25' }])
    } finally {
      await act(() => i18n.changeLanguage('en'))
    }
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
    await user.click(screen.getByRole('button', { name: 'Save and continue' }))
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
    await user.click(screen.getByRole('button', { name: 'Save and continue' }))
    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(onDone).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Save and continue' }))
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
    expect(screen.queryByRole('button', { name: 'Save and continue' })).not.toBeInTheDocument()
    fail = false
    await user.click(screen.getByRole('button', { name: 'Try again' }))
    expect(await screen.findByText('Euro Stash')).toBeInTheDocument()
  })
})
