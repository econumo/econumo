import { useState } from 'react'
import type { ComponentType } from 'react'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers, fixtureEur, fixtureUsd } from '@/test/fixtures'
import { ImportWizard } from '../ImportWizard'
import { profileColumns } from '../columns'
import { applyAiGuess, initialAnswers, redetectFormats } from '../guess'
import { parseCsvText } from '../parse'
import { prepareRows, type StepId, type WizardAnswers } from '../prepare'
import type { StepBodyProps } from '../wizard'
import { ColumnChoices } from '../ColumnChoices'
import { AmountStep } from './AmountStep'
import { DateStep } from './DateStep'
import { DescriptionStep } from './DescriptionStep'

const csv = (text: string, name = 'statement.csv') => new File([text], name, { type: 'text/csv' })

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })

function withQueries(node: React.ReactNode) {
  return <QueryClientProvider client={queryClient}>{node}</QueryClientProvider>
}

function renderWizard(text: string) {
  render(withQueries(<ImportWizard open onClose={() => {}} onComplete={vi.fn()} initialFile={csv(text)} />))
}

// A step on its own, with answers kept the way the wizard keeps them; the first
// prepared row is what the Check step's first card shows.
function StepHarness({ text, Step, onSet, open = true, roles = {} }: {
  text: string
  /** the columns the AI gave roles to: the wizard itself assigns none */
  roles?: Record<string, string>
  Step: ComponentType<StepBodyProps>
  onSet: (step: StepId, patch: Partial<WizardAnswers>) => void
  /** the list of choices starts open, as after Change */
  open?: boolean
}) {
  const [file] = useState(() => parseCsvText(text))
  const [choosing, setChoosing] = useState(open)
  const [profiles] = useState(() => profileColumns(file))
  const [answers, setAnswersState] = useState(() =>
    applyAiGuess(initialAnswers(file, { writableAccountIds: ['acc-1', 'acc-2'], userCurrencyId: 'cur-usd', locale: 'en-GB' }), roles, file, 'en-GB', new Set()),
  )
  const rows = prepareRows(file, answers, new Map())
  const setAnswers = (step: StepId, patch: Partial<WizardAnswers>) => {
    onSet(step, patch)
    setAnswersState((prev) => {
      const next = { ...prev, ...patch }
      return redetectFormats(next, file, 'en-GB', { date: next.date.column !== prev.date.column && next.date.order === prev.date.order })
    })
  }
  return withQueries(
    <>
      <Step
        state={{ file, fileName: 'statement.csv', profiles, answers, touched: new Set(), fixes: new Map(), rows, isExport: false }}
        setAnswers={setAnswers}
        setFixes={() => {}}
        goTo={() => {}}
        choosing={choosing}
        change={() => setChoosing(true)}
        closeChoices={() => setChoosing(false)}
        keepChoices={() => setChoosing(true)}
      />
      <output aria-label="first row date">{rows[0]?.date ?? ''}</output>
    </>,
  )
}

beforeEach(() => {
  queryClient.clear()
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  server.use(...coreHandlers())
})

it('with no accounts, the Account step starts on a new account in the user’s currency', async () => {
  server.use(...coreHandlers({ accounts: [] }))
  renderWizard('Date,Amount\n2026-01-02,-5.50\n')
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  expect(screen.getByRole('radio', { name: 'A new account' })).toBeChecked()
  expect(screen.queryByRole('radio', { name: 'An account I already have' })).not.toBeInTheDocument()
  expect(screen.getByLabelText('Account name')).toHaveValue('')
  expect(screen.getByLabelText('Account name')).toHaveAttribute('placeholder', 'e.g. Everyday card')
  expect(screen.getByRole('combobox', { name: 'Currency' })).toHaveValue('USD, $, US Dollar')
})

it('with no accounts, a lost account answer comes back as a new account to name', async () => {
  server.use(...coreHandlers({ accounts: [] }))
  renderWizard('Date,Amount,Wallet,Memo\n2026-01-02,-5.50,Trip,lunch\n2026-01-03,7.00,Home,salary\n')
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  await user.click(screen.getByRole('radio', { name: 'Each row says which account' }))
  await user.click(screen.getByRole('radio', { name: /from “Wallet”/ }))
  // another step takes the account column, leaving the account unanswered
  await user.click(screen.getByRole('button', { name: /^Category/ }))
  await screen.findByRole('heading', { name: 'How are the payments sorted?' })
  await user.click(screen.getByRole('radio', { name: /from “Wallet”/ }))
  await user.click(screen.getByRole('button', { name: /^Account/ }))
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  expect(screen.getByRole('radio', { name: 'A new account' })).toBeChecked()
  expect(screen.getByRole('button', { name: /^Account.*needs your attention/ })).toBeInTheDocument()
  expect(screen.getByLabelText('Account name')).toHaveValue('')
  fireEvent.change(screen.getByLabelText('Account name'), { target: { value: 'Travel card' } })
  await waitFor(() => expect(screen.queryByRole('button', { name: /^Account.*needs your attention/ })).not.toBeInTheDocument())
})

it('a new account is missing until it is named, and typing a name clears the mark', async () => {
  server.use(...coreHandlers({ accounts: [] }))
  renderWizard('Date,Amount\n2026-01-02,-5.50\n')
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  expect(screen.getByRole('button', { name: /^Account.*needs your attention/ })).toHaveAccessibleDescription('Name the new account')
  fireEvent.change(screen.getByLabelText('Account name'), { target: { value: '   ' } })
  expect(await screen.findByRole('button', { name: /^Account.*needs your attention/ })).toBeInTheDocument()
  fireEvent.change(screen.getByLabelText('Account name'), { target: { value: 'Travel card' } })
  await waitFor(() => expect(screen.queryByRole('button', { name: /^Account.*needs your attention/ })).not.toBeInTheDocument())
})

it('a typed account name is saved after a pause in typing', async () => {
  server.use(...coreHandlers({ accounts: [] }))
  renderWizard('Date,Amount\n2026-01-02,-5.50\n')
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  fireEvent.change(screen.getByLabelText('Account name'), { target: { value: 'Card' } })
  expect(screen.getByRole('button', { name: /^Account.*needs your attention/ })).toBeInTheDocument()
  await waitFor(() => expect(screen.queryByRole('button', { name: /^Account.*needs your attention/ })).not.toBeInTheDocument())
})

it('a typed account name is kept when the step is left before the pause, with the field still focused', async () => {
  server.use(...coreHandlers({ accounts: [] }))
  renderWizard('Date,Amount\n2026-01-02,-5.50\n')
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  const name = screen.getByLabelText('Account name')
  name.focus()
  fireEvent.change(name, { target: { value: 'Travel card' } })
  fireEvent.click(screen.getByRole('button', { name: /^Date/ }))
  expect(screen.getByRole('heading', { name: 'When did each payment happen?' })).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: /^Account/ }))
  expect(screen.getByLabelText('Account name')).toHaveValue('Travel card')
})

it('a new account takes the currency the file names', async () => {
  server.use(...coreHandlers({ accounts: [] }))
  renderWizard('Date,Amount,Currency\n2026-01-02,-5.50,eur\n2026-01-03,1.00,EUR\n')
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  expect(screen.getByRole('combobox', { name: 'Currency' })).toHaveValue('EUR, €, Euro')
})

it('a file naming two currencies leaves a new account in the user’s currency', async () => {
  server.use(...coreHandlers({ accounts: [] }))
  renderWizard('Date,Amount,Currency\n2026-01-02,-5.50,EUR\n2026-01-03,1.00,USD\n')
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  expect(screen.getByRole('combobox', { name: 'Currency' })).toHaveValue('USD, $, US Dollar')
})

it('with accounts, the Account step offers the account picker first', async () => {
  renderWizard('Date,Amount\n2026-01-02,-5.50\n')
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  expect(screen.getByRole('radio', { name: 'An account I already have' })).toBeChecked()
  expect(within(screen.getByRole('radiogroup', { name: 'Account' })).getAllByRole('radio').length).toBeGreaterThan(0)
})

const perRowList = () => screen.getByRole('list', { name: 'Accounts in your file' })

it('lists the accounts a per-row column names as sentences; only a new one can change its currency', async () => {
  renderWizard('Date,Amount,Wallet\n2026-01-02,-5.50,Cash\n2026-01-03,7.00,Travel card\n2026-01-04,1.00, cash \n')
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  await user.click(screen.getByRole('radio', { name: 'Each row says which account' }))
  await user.click(screen.getByRole('radio', { name: /from “Wallet”/ }))
  const rows = within(perRowList()).getAllByRole('listitem')
  expect(rows).toHaveLength(2)
  expect(rows[0]).toHaveTextContent('Cash · 2 transactions Your existing account · USD')
  expect(within(rows[0]).queryByRole('button', { name: 'Change currency' })).not.toBeInTheDocument()
  expect(within(rows[0]).queryByRole('combobox')).not.toBeInTheDocument()
  expect(rows[1]).toHaveTextContent('Travel card · 1 transaction New account · USD')
  expect(within(rows[1]).queryByRole('combobox')).not.toBeInTheDocument()
  await user.click(within(rows[1]).getByRole('button', { name: 'Change currency' }))
  expect(within(rows[1]).getByRole('combobox', { name: 'Currency for Travel card' })).toHaveValue('USD')
})

it('Each row says which account, with no column chosen yet, marks the Account step', async () => {
  renderWizard('Date,Amount,Wallet\n2026-01-02,-5.50,Trip\n')
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  await user.click(screen.getByRole('radio', { name: 'Each row says which account' }))
  expect(screen.getByRole('button', { name: /^Account.*needs your attention/ })).toBeInTheDocument()
})

it('per-row accounts take the currency their own rows name, the user’s otherwise', async () => {
  renderWizard('Date,Amount,Wallet,Cur\n2026-01-02,-5.50,Trip,EUR\n2026-01-03,7.00,Trip,EUR\n2026-01-04,1.00,Home,USD\n2026-01-05,1.00,Mixed,EUR\n2026-01-06,1.00,Mixed,USD\n')
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  await user.click(screen.getByRole('radio', { name: 'Each row says which account' }))
  await user.click(screen.getByRole('radio', { name: /from “Wallet”/ }))
  // no part of the file is taken for the currency until the user points at one
  expect(screen.getByText("Your file doesn't say which currency")).toBeInTheDocument()
  await user.click(screen.getAllByRole('button', { name: 'Change' })[1])
  await user.click(screen.getByRole('radio', { name: /from “Cur”/ }))
  expect(screen.getByText("Your file says each account's currency")).toBeInTheDocument()
  expect(screen.getByText('from “Cur”')).toBeInTheDocument()
  const rows = within(perRowList()).getAllByRole('listitem')
  expect(rows[0]).toHaveTextContent('Trip · 2 transactions New account · EUR')
  expect(rows[1]).toHaveTextContent('Home · 1 transaction New account · USD')
  expect(rows[2]).toHaveTextContent('Mixed · 2 transactions New account · USD')
})

it('a file without a currency: new accounts take the one picked for them, and one account’s own pick wins', async () => {
  server.use(...coreHandlers({ currencies: [fixtureUsd, fixtureEur, fixtureCad] }))
  renderWizard('Date,Amount,Wallet\n2026-01-02,-5.50,Trip\n2026-01-03,7.00,Home\n')
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  await user.click(screen.getByRole('radio', { name: 'Each row says which account' }))
  await user.click(screen.getByRole('radio', { name: /from “Wallet”/ }))
  expect(screen.getByText("Your file doesn't say which currency")).toBeInTheDocument()
  // nothing in the file holds codes: Change offers only "no currency", with no whole-file list
  await user.click(screen.getAllByRole('button', { name: 'Change' })[1])
  const choices = screen.getByRole('radiogroup', { name: 'Which of these is the currency?' })
  expect(within(choices).getAllByRole('radio').map((r) => r.textContent)).toEqual(['My file has no currency'])
  expect(screen.queryByText(/so here is everything/)).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Show all' })).not.toBeInTheDocument()
  await user.click(within(choices).getByRole('radio', { name: 'My file has no currency' }))
  const all = screen.getByRole('combobox', { name: 'Currency for new accounts' })
  expect(all).toHaveValue('USD')
  // one account first gets its own currency
  const home = within(perRowList()).getAllByRole('listitem')[1]
  await user.click(within(home).getByRole('button', { name: 'Change currency' }))
  await user.click(within(home).getByRole('combobox', { name: 'Currency for Home' }))
  await user.click(await screen.findByRole('option', { name: 'CAD, C$, Canadian Dollar' }))
  // then every other new account follows the one picked for all
  await user.click(all)
  await user.click(await screen.findByRole('option', { name: 'EUR, €, Euro' }))
  const rows = within(perRowList()).getAllByRole('listitem')
  expect(rows[0]).toHaveTextContent('Trip · 1 transaction New account · EUR')
  expect(rows[1]).toHaveTextContent('Home · 1 transaction New account · CAD')
  // picking the account again keeps both currency answers
  await user.click(screen.getAllByRole('button', { name: 'Change' })[0])
  await user.click(screen.getByRole('radio', { name: /from “Wallet”/ }))
  expect(within(perRowList()).getAllByRole('listitem')[0]).toHaveTextContent('New account · EUR')
  expect(within(perRowList()).getAllByRole('listitem')[1]).toHaveTextContent('New account · CAD')
})

const WS_EXPORT =
  'transaction_id,account_name,account_currency,category,description,tag,labels,payee,amount,date\n' +
  'a1,WS Cash,CAD,Interest,Transfer of 1 CAD,,,WealthSimple,-4.5,2026-11-08 14:05:09\n' +
  'a2,WS Cash,CAD,Strata,,,,ICBC,100,2026-11-07 09:00:00\n'
const fixtureCad = { ...fixtureUsd, id: 'cur-cad', code: 'CAD', name: 'Canadian Dollar', symbol: 'C$' }

// an export opens on the Check step; the Account step is one tap away in the step list
async function openExportAccountStep(user: ReturnType<typeof userEvent.setup>) {
  renderWizard(WS_EXPORT)
  await user.click(await screen.findByRole('button', { name: /^Account/ }))
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
}

it('an Econumo export reads each account’s currency from account_currency', async () => {
  server.use(...coreHandlers({ currencies: [fixtureUsd, fixtureEur, fixtureCad] }))
  const user = userEvent.setup()
  await openExportAccountStep(user)
  expect(screen.getByRole('radio', { name: 'Each row says which account' })).toBeChecked()
  expect(screen.getByText("Your file says each account's currency")).toBeInTheDocument()
  expect(screen.getByText('from “account_currency”')).toBeInTheDocument()
  expect(within(perRowList()).getByRole('listitem')).toHaveTextContent('WS Cash · 2 transactions New account · CAD')
  expect(screen.queryByText(/isn't set up here/)).not.toBeInTheDocument()
  // choosing the account column again keeps reading the currency from the file
  await user.click(screen.getAllByRole('button', { name: 'Change' })[0])
  expect(screen.getByText('Other parts of your file')).toBeInTheDocument()
  await user.click(screen.getByRole('radio', { name: /from “account_name”/ }))
  expect(within(perRowList()).getByRole('listitem')).toHaveTextContent('WS Cash · 2 transactions New account · CAD')
})

it('the currency answer offers code columns, and My file has no currency moves new accounts to the picked one', async () => {
  server.use(...coreHandlers({ currencies: [fixtureUsd, fixtureEur, fixtureCad] }))
  const user = userEvent.setup()
  await openExportAccountStep(user)
  await user.click(screen.getAllByRole('button', { name: 'Change' })[1])
  const choices = screen.getByRole('radiogroup', { name: 'Which of these is the currency?' })
  // the part of the file holding codes first, then none, then the rest under the divider
  expect(within(choices).getAllByRole('radio').map((r) => r.textContent).slice(0, 2)).toEqual(['CADfrom “account_currency”', 'My file has no currency'])
  expect(within(choices).getByText('Other parts of your file')).toBeInTheDocument()
  await user.click(within(choices).getByRole('radio', { name: 'My file has no currency' }))
  expect(screen.getByText("Your file doesn't say which currency")).toBeInTheDocument()
  await user.click(screen.getByRole('combobox', { name: 'Currency for new accounts' }))
  await user.click(await screen.findByRole('option', { name: 'EUR, €, Euro' }))
  expect(within(perRowList()).getByRole('listitem')).toHaveTextContent('WS Cash · 2 transactions New account · EUR')
  // the account step's answer is kept when the account column is picked again
  await user.click(screen.getAllByRole('button', { name: 'Change' })[0])
  await user.click(screen.getByRole('radio', { name: /from “account_name”/ }))
  expect(within(perRowList()).getByRole('listitem')).toHaveTextContent('WS Cash · 2 transactions New account · EUR')
})

it('the currency answer never offers the account column itself', async () => {
  server.use(...coreHandlers({ currencies: [fixtureUsd, fixtureEur, fixtureCad] }))
  const user = userEvent.setup()
  await openExportAccountStep(user)
  await user.click(screen.getAllByRole('button', { name: 'Change' })[1])
  const choices = screen.getByRole('radiogroup', { name: 'Which of these is the currency?' })
  expect(within(choices).getByText('Other parts of your file')).toBeInTheDocument()
  expect(within(choices).queryByRole('radio', { name: /from “account_name”/ })).not.toBeInTheDocument()
})

it('with nothing else that fits the category, a part the currency reads is listed as used, and taking it there leaves new accounts on the picked currency', async () => {
  renderWizard('Date,Amount,Wallet,Cur\n2026-01-02,-5.50,Trip,EUR\n2026-01-03,7.00,Trip,EUR\n2026-01-04,1.00,Home,EUR\n')
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  await user.click(screen.getByRole('radio', { name: 'Each row says which account' }))
  await user.click(screen.getByRole('radio', { name: /from “Wallet”/ }))
  await user.click(screen.getAllByRole('button', { name: 'Change' })[1])
  await user.click(screen.getByRole('radio', { name: /from “Cur”/ }))
  expect(within(perRowList()).getAllByRole('listitem')[0]).toHaveTextContent('New account · EUR')
  await user.click(screen.getByRole('button', { name: /^Category/ }))
  await screen.findByRole('heading', { name: 'How are the payments sorted?' })
  await user.click(screen.getByRole('button', { name: 'Change' }))
  const offered = () => screen.getByRole('radiogroup', { name: 'Which of these is the category?' })
  expect(screen.getByText('Nothing in your file looks like a category, so here is everything.')).toBeInTheDocument()
  const cur = within(offered()).getByRole('radio', { name: /from “Cur”/ })
  expect(cur).toHaveAccessibleDescription('used for currency')
  await user.click(cur)
  await user.click(screen.getByRole('button', { name: /^Account/ }))
  expect(await screen.findByText("Your file doesn't say which currency")).toBeInTheDocument()
  expect(within(perRowList()).getAllByRole('listitem')[0]).toHaveTextContent('New account · USD')
})

it('a per-row account whose file currency is not set up here is new in the user’s currency, with a note', async () => {
  await openExportAccountStep(userEvent.setup())
  const row = within(perRowList()).getByRole('listitem')
  expect(row).toHaveTextContent("WS Cash · 2 transactions New account · USDChange currencyThe file says CAD, which isn't set up here. You can add it in Settings.")
})

it('a new account for a file in a currency not set up here says so under its currency', async () => {
  renderWizard('Date,Amount,Currency\n2026-01-02,-5.50,CAD\n2026-01-03,7.00,CAD\n')
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  await user.click(screen.getByRole('radio', { name: 'A new account' }))
  expect(screen.getByRole('combobox', { name: 'Currency' })).toHaveValue('USD, $, US Dollar')
  expect(screen.getByText("The file says CAD, which isn't set up here. You can add it in Settings.")).toBeInTheDocument()
})

it('choosing another column makes it the date and marks the Date step as answered', async () => {
  const onSet = vi.fn()
  render(<StepHarness text={'Date,Valued,Amount\n2026-01-02,2026-01-05,-5\n'} Step={DateStep} roles={{ date: 'Date' }} onSet={onSet} />)
  const user = userEvent.setup()
  expect(screen.getByRole('radio', { name: /from “Date”/ })).toBeChecked()
  // values come first, written as dates; the file's own name follows
  expect(screen.getByRole('radio', { name: /from “Valued”/ })).toHaveAccessibleName('Jan 5, 2026 from “Valued”')
  await user.click(screen.getByRole('radio', { name: /from “Valued”/ }))
  expect(onSet).toHaveBeenCalledWith('date', { date: expect.objectContaining({ column: 1 }) })
  expect(screen.getByRole('status', { name: 'first row date' })).toHaveTextContent('2026-01-05')
  // the pick closes the list; the answer reads the chosen values as dates
  expect(screen.queryByRole('radio')).not.toBeInTheDocument()
  expect(screen.getByText('Jan 5, 2026')).toBeInTheDocument()
  expect(screen.getByText('from “Valued”')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Change' }))
  expect(screen.getByRole('radio', { name: /from “Valued”/ })).toBeChecked()
})

it('the date candidates come first; every other part follows under a divider, with what it is used for', () => {
  render(<StepHarness text={'Date,Amount,Memo\n2026-01-02,-5,lunch\n'} Step={DateStep} roles={{ amount: 'Amount' }} onSet={vi.fn()} />)
  expect(screen.getAllByRole('radio')[0]).toHaveAccessibleName(/from “Date”/)
  expect(screen.getAllByRole('radio')).toHaveLength(3)
  expect(screen.getByText('Other parts of your file')).toBeInTheDocument()
  expect(screen.getByRole('radio', { name: /from “Amount”/ })).toHaveAccessibleDescription('used for amount')
  expect(screen.getByRole('radio', { name: 'lunch from “Memo”' })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Show all' })).not.toBeInTheDocument()
})

it('a file with only a date column says it is the only part that looks like a date', () => {
  render(<StepHarness text={'Date\n2026-01-02\n'} Step={DateStep} onSet={vi.fn()} />)
  expect(screen.getByText('This is the only part of your file that looks like a date.')).toBeInTheDocument()
})

it('with no date in the file, every part is listed under a note, with what each is used for', () => {
  render(<StepHarness text={'Amount,Memo\n-5,lunch\n'} Step={DateStep} roles={{ amount: 'Amount' }} onSet={vi.fn()} />)
  expect(screen.getByText('Nothing in your file looks like a date, so here is everything.')).toBeInTheDocument()
  expect(screen.getByRole('radio', { name: 'lunch from “Memo”' })).toBeInTheDocument()
  expect(screen.getByRole('radio', { name: /from “Amount”/ })).toHaveAccessibleDescription('used for amount')
  expect(screen.queryByRole('button', { name: 'Show all' })).not.toBeInTheDocument()
})

it('asks how to read an ambiguous date, and the answer changes how the first row reads', async () => {
  render(<StepHarness text={'Date,Amount\n03/04/2026,-5\n05/06/2026,1\n'} Step={DateStep} roles={{ date: 'Date' }} onSet={vi.fn()} />)
  const user = userEvent.setup()
  expect(screen.getByText('Is 03/04/2026 April 3 or March 4?')).toBeInTheDocument()
  expect(screen.getByRole('radio', { name: 'April 3, 2026' })).toBeChecked()
  expect(screen.getByRole('status', { name: 'first row date' })).toHaveTextContent('2026-04-03')
  await user.click(screen.getByRole('radio', { name: 'March 4, 2026' }))
  expect(screen.getByRole('status', { name: 'first row date' })).toHaveTextContent('2026-03-04')
})

it('counts the rows whose date cannot be read', () => {
  render(<StepHarness text={'Date,Amount\n2026-01-02,-5\n2026-01-03,1\n2026-01-04,1\n2026-01-05,1\n2026-01-06,1\n2026-01-07,1\n2026-01-08,1\n2026-01-09,1\nsoon,1\nlater,2\n'} Step={DateStep} roles={{ date: 'Date' }} onSet={vi.fn()} />)
  expect(screen.getByText('2 rows have no readable date and will be skipped')).toBeInTheDocument()
})

it('one amount column shows how spending looks with a switch to flip it; separate columns hide the switch', async () => {
  render(<StepHarness text={'Date,Amount,In,Out\n2026-01-02,-4.50,,4.50\n'} Step={AmountStep} roles={{ amount: 'Amount' }} onSet={vi.fn()} />)
  const user = userEvent.setup()
  expect(screen.getByText('Spending shows as: −100.00')).toBeInTheDocument()
  await user.click(screen.getByRole('switch', { name: 'Flip the signs' }))
  expect(screen.getByText('Spending shows as: 100.00')).toBeInTheDocument()
  await user.click(screen.getByRole('radio', { name: 'Money in and money out' }))
  expect(screen.queryByRole('switch', { name: 'Flip the signs' })).not.toBeInTheDocument()
  expect(screen.getByRole('radiogroup', { name: 'Which of these is money in?' })).toBeInTheDocument()
  expect(screen.getByRole('radiogroup', { name: 'Which of these is money out?' })).toBeInTheDocument()
})

it('with separate columns, each side picks its own column, and either side can be left without one', async () => {
  const onSet = vi.fn()
  render(<StepHarness text={'Date,Amount,In,Out\n2026-01-02,-4.50,,4.50\n2026-01-03,,7.00,\n'} Step={AmountStep} onSet={onSet} />)
  const user = userEvent.setup()
  await user.click(screen.getByRole('radio', { name: 'Money in and money out' }))
  const moneyIn = () => within(screen.getByRole('radiogroup', { name: 'Which of these is money in?' }))
  const moneyOut = () => within(screen.getByRole('radiogroup', { name: 'Which of these is money out?' }))
  await user.click(moneyIn().getByRole('radio', { name: /from “In”/ }))
  expect(moneyIn().getByRole('radio', { name: /from “In”/ })).toBeChecked()
  await user.click(moneyOut().getByRole('radio', { name: /from “Out”/ }))
  expect(onSet).toHaveBeenLastCalledWith('amount', { amount: expect.objectContaining({ inColumn: 2, outColumn: 3 }) })
  await user.click(screen.getByRole('button', { name: 'Change' }))
  expect(moneyIn().getByRole('radio', { name: /from “Out”/ })).toHaveAccessibleDescription('used for money out')
  await user.click(moneyOut().getByRole('radio', { name: 'None of these is money out' }))
  expect(onSet).toHaveBeenLastCalledWith('amount', { amount: expect.objectContaining({ inColumn: 2, outColumn: null }) })
})

it('the amount step answers with the first amounts as they will be added', () => {
  render(<StepHarness text={'Date,Amount,Memo\n2026-01-02,-4.50,Coffee\n2026-01-03,100,Salary\n'} Step={AmountStep} roles={{ amount: 'Amount' }} onSet={vi.fn()} open={false} />)
  const items = within(screen.getByRole('list', { name: 'How payments will look' })).getAllByRole('listitem')
  expect(items).toHaveLength(2)
  expect(items[0]).toHaveTextContent(/^−4\.5/)
  expect(items[1]).toHaveTextContent(/^\+100/)
  expect(items[1]).toHaveClass('text-income')
  expect(items[0]).not.toHaveClass('text-income')
})

it('rows whose amount cannot be read are a quiet note, not a warning', () => {
  render(<StepHarness text={'Date,Amount\n2026-01-02,-5\n2026-01-03,1\n2026-01-04,1\n2026-01-05,1\n2026-01-06,1\n2026-01-07,1\n2026-01-08,1\n2026-01-09,1\n2026-01-10,1\n2026-01-11,?\n'} Step={AmountStep} roles={{ date: 'Date', amount: 'Amount' }} onSet={vi.fn()} />)
  expect(screen.getByText('1 row has no readable amount and will be skipped')).toHaveClass('text-muted-foreground')
})

it('spending is shown as a round example, not a value from the file', async () => {
  render(<StepHarness text={'Date,Amount\n2026-01-02,12.00\n2026-01-03,-1\n'} Step={AmountStep} roles={{ amount: 'Amount' }} onSet={vi.fn()} />)
  const user = userEvent.setup()
  expect(screen.getByText('Spending shows as: −100.00')).toBeInTheDocument()
  await user.click(screen.getByRole('switch', { name: 'Flip the signs' }))
  expect(screen.getByText('Spending shows as: 100.00')).toBeInTheDocument()
})

it('the spending example is written in the file’s decimal notation', async () => {
  render(<StepHarness text={'Datum;Betrag\n03.11.2026;-12,50\n05.11.2026;2.500,00\n'} Step={AmountStep} roles={{ amount: 'Betrag' }} onSet={vi.fn()} />)
  const user = userEvent.setup()
  expect(screen.getByText('Spending shows as: −100,00')).toBeInTheDocument()
  await user.click(screen.getByRole('switch', { name: 'Flip the signs' }))
  expect(screen.getByText('Spending shows as: 100,00')).toBeInTheDocument()
})

it('the description step lists no names from the file', () => {
  render(<StepHarness text={'Date,Amount,Memo\n2026-01-02,-5,REWE Markt\n2026-01-03,7,REWE Markt\n'} Step={DescriptionStep} roles={{ description: 'Memo' }} onSet={vi.fn()} open={false} />)
  expect(screen.getByText('REWE Markt')).toBeInTheDocument()
  expect(screen.queryByRole('list')).not.toBeInTheDocument()
  expect(screen.queryByText(/in your file/)).not.toBeInTheDocument()
})

it('a guessed answer shows no choices; Change puts them in its place without answering anything', async () => {
  const onSet = vi.fn()
  render(<StepHarness text={'Date,Amount,Memo\n2026-01-02,-5,REWE Markt\n2026-01-03,7,Gehalt\n'} Step={DescriptionStep} roles={{ description: 'Memo' }} onSet={onSet} open={false} />)
  const user = userEvent.setup()
  expect(screen.getByText('REWE Markt · Gehalt')).toBeInTheDocument()
  expect(screen.getByText('from “Memo”')).toBeInTheDocument()
  expect(screen.queryByRole('radio')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Change' }))
  expect(screen.getByRole('radiogroup', { name: 'Which of these says what it was for?' })).toBeInTheDocument()
  expect(screen.getByRole('radio', { name: 'REWE Markt · Gehalt from “Memo”' })).toBeChecked()
  // the list replaces the answer: it is not shown twice, and there is no second Change
  expect(screen.queryByText('from “Memo”', { selector: 'div' })).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Change' })).not.toBeInTheDocument()
  expect(onSet).not.toHaveBeenCalled()
  // picking the one already chosen is an answer too, and goes back to it
  await user.click(screen.getByRole('radio', { name: 'REWE Markt · Gehalt from “Memo”' }))
  expect(onSet).toHaveBeenCalledWith('description', { description: 2 })
  expect(screen.queryByRole('radiogroup')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Change' })).toBeInTheDocument()
})

it('with money in and out, the list stays open for the second pick and folds back to the answer after it', async () => {
  render(<StepHarness text={'Date,Amount,In,Out\n2026-01-02,-4.50,,4.50\n2026-01-03,7.00,7.00,\n'} Step={AmountStep} roles={{ amount: 'Amount' }} onSet={vi.fn()} open={false} />)
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Change' }))
  await user.click(screen.getByRole('radio', { name: 'Money in and money out' }))
  await user.click(within(screen.getByRole('radiogroup', { name: 'Which of these is money in?' })).getByRole('radio', { name: /from “In”/ }))
  expect(screen.getByRole('radiogroup', { name: 'Which of these is money out?' })).toBeInTheDocument()
  await user.click(within(screen.getByRole('radiogroup', { name: 'Which of these is money out?' })).getByRole('radio', { name: /from “Out”/ }))
  expect(screen.queryByRole('radiogroup', { name: 'Which of these is money out?' })).not.toBeInTheDocument()
  expect(screen.getByText('from “In” · from “Out”')).toBeInTheDocument()
})

const notes = { nothingFitsNote: 'Nothing fits.', onlyOneNote: 'Only one fits.' }

it('what does not fit follows the likely parts, unless it is the chosen answer, which comes first', () => {
  const text =
    'Date,Amount,Ref,Memo\n' +
    '2026-01-02,-5,01a1be03-f8df-7603-b2b1-2c3d4e5f6a7b,Coffee\n' +
    '2026-01-03,7,b298d99e-4fb1-42ca-97a2-9f00d01aeb19,Salary\n'
  const file = parseCsvText(text)
  const profiles = profileColumns(file)
  const onSelect = vi.fn()
  const { rerender } = render(<ColumnChoices profiles={profiles} label="Which?" selected={3} onSelect={onSelect} candidates={[3]} {...notes} />)
  expect(screen.getAllByRole('radio').map((r) => r.textContent)).toEqual([
    'Coffee · Salaryfrom “Memo”',
    'Jan 2, 2026 · Jan 3, 2026from “Date”',
    '-5.00 · 7.00from “Amount”',
    expect.stringMatching(/from “Ref”$/),
  ])
  expect(screen.getByRole('radio', { name: 'Coffee · Salary from “Memo”' })).toBeChecked()
  rerender(<ColumnChoices key="fresh" profiles={profiles} label="Which?" selected={2} onSelect={onSelect} candidates={[3]} {...notes} />)
  expect(screen.getAllByRole('radio')[0]).toHaveAccessibleName(/from “Ref”/)
  expect(screen.getByRole('radio', { name: /from “Ref”/ })).toBeChecked()
  expect(screen.queryByRole('button', { name: 'Show all' })).not.toBeInTheDocument()
})

it('with nothing that fits, every part of the file is listed straight away', () => {
  const file = parseCsvText('Date,Amount\n2026-01-02,-5\n')
  render(<ColumnChoices profiles={profileColumns(file)} label="Which?" selected={null} onSelect={vi.fn()} candidates={[]} {...notes} />)
  expect(screen.getByText('Nothing fits.')).toBeInTheDocument()
  expect(screen.getAllByRole('radio')).toHaveLength(2)
  expect(screen.queryByRole('button', { name: 'Show all' })).not.toBeInTheDocument()
})

it('a file without a header row shows its values with no names beside them', () => {
  const file = parseCsvText('2026-01-01,-1,Tea\n2026-01-02,-5,Coffee\n2026-01-03,7,Salary\n')
  render(<ColumnChoices profiles={profileColumns(file)} label="Which?" selected={2} onSelect={vi.fn()} candidates={[2]} {...notes} />)
  expect(screen.getByRole('radio', { name: 'Coffee · Salary' })).toBeChecked()
  expect(screen.queryByText(/from “/)).not.toBeInTheDocument()
})
