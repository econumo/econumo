import { useState } from 'react'
import type { ComponentType } from 'react'
import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers, fixtureEur, fixtureUsd } from '@/test/fixtures'
import { ImportWizard } from '../ImportWizard'
import { profileColumns } from '../columns'
import { guessAnswers, redetectFormats } from '../guess'
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
function StepHarness({ text, Step, onSet, open = true }: {
  text: string
  Step: ComponentType<StepBodyProps>
  onSet: (step: StepId, patch: Partial<WizardAnswers>) => void
  /** the list of choices starts open, as after Change */
  open?: boolean
}) {
  const [file] = useState(() => parseCsvText(text))
  const [choosing, setChoosing] = useState(open)
  const [profiles] = useState(() => profileColumns(file))
  const [answers, setAnswersState] = useState(() =>
    guessAnswers(file, profiles, { writableAccountIds: ['acc-1', 'acc-2'], userCurrencyId: 'cur-usd', locale: 'en-GB', defaultAccountName: 'statement' }),
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
  expect(screen.getByLabelText('Account name')).toHaveValue('statement')
  expect(screen.getByRole('combobox', { name: 'Currency' })).toHaveValue('USD, $, US Dollar')
})

it('a typed account name is saved after a pause in typing', async () => {
  server.use(...coreHandlers({ accounts: [] }))
  renderWizard('Date,Amount\n2026-01-02,-5.50\n')
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  fireEvent.change(screen.getByLabelText('Account name'), { target: { value: '' } })
  expect(screen.getByRole('button', { name: 'Account' })).toBeInTheDocument()
  expect(await screen.findByRole('button', { name: /^Account.*needs your attention/ })).toBeInTheDocument()
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

it('lists the accounts a per-row column names, with a currency only for the new ones', async () => {
  renderWizard('Date,Amount,Wallet\n2026-01-02,-5.50,Cash\n2026-01-03,7.00,Travel card\n2026-01-04,1.00, cash \n')
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account are these transactions from?' })
  await user.click(screen.getByRole('radio', { name: 'Each row says which account' }))
  await user.click(screen.getByRole('radio', { name: /from “Wallet”/ }))
  const list = screen.getByRole('list', { name: /^Your file names the account for each transaction/ })
  const rows = within(list).getAllByRole('listitem')
  expect(rows).toHaveLength(2)
  expect(rows[0]).toHaveTextContent('Cash · 2 transactionsExisting account')
  expect(within(rows[0]).queryByRole('combobox')).not.toBeInTheDocument()
  expect(within(rows[1]).getByRole('combobox', { name: 'Currency for Travel card' })).toHaveValue('USD, $, US Dollar')
})

it('Each row says which account, with no column chosen yet, marks the Account step', async () => {
  renderWizard('Date,Amount,Wallet\n2026-01-02,-5.50,Cash\n')
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
  expect(screen.getByRole('combobox', { name: 'Currency for Trip' })).toHaveValue('EUR, €, Euro')
  expect(screen.getByRole('combobox', { name: 'Currency for Home' })).toHaveValue('USD, $, US Dollar')
  expect(screen.getByRole('combobox', { name: 'Currency for Mixed' })).toHaveValue('USD, $, US Dollar')
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

it('an Econumo export prefills a per-row account’s currency from account_currency', async () => {
  server.use(...coreHandlers({ currencies: [fixtureUsd, fixtureEur, fixtureCad] }))
  const user = userEvent.setup()
  await openExportAccountStep(user)
  expect(screen.getByRole('radio', { name: 'Each row says which account' })).toBeChecked()
  expect(screen.getByRole('combobox', { name: 'Currency for WS Cash' })).toHaveValue('CAD, C$, Canadian Dollar')
  expect(screen.queryByText(/The file says CAD/)).not.toBeInTheDocument()
  // choosing the account column again reads the currency from the file the same way
  await user.click(screen.getByRole('button', { name: 'Change' }))
  await user.click(screen.getByRole('button', { name: 'Show all' }))
  await user.click(screen.getByRole('radio', { name: /from “payee”/ }))
  expect(screen.queryByRole('combobox', { name: 'Currency for WS Cash' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Change' }))
  await user.click(screen.getByRole('radio', { name: /from “account_name”/ }))
  expect(screen.getByRole('combobox', { name: 'Currency for WS Cash' })).toHaveValue('CAD, C$, Canadian Dollar')
})

it('a per-row account whose file currency is not set up here keeps the user’s currency, with a note', async () => {
  await openExportAccountStep(userEvent.setup())
  const row = within(screen.getByRole('list', { name: /^Your file names the account for each transaction/ })).getByRole('listitem')
  expect(within(row).getByRole('combobox', { name: 'Currency for WS Cash' })).toHaveValue('USD, $, US Dollar')
  expect(row).toHaveTextContent("The file says CAD, which isn't set up here. You can add it in Settings.")
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
  render(<StepHarness text={'Date,Valued,Amount\n2026-01-02,2026-01-05,-5\n'} Step={DateStep} onSet={onSet} />)
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

it('only dates are offered as the date; Show all brings the rest, with what they are used for', async () => {
  render(<StepHarness text={'Date,Amount,Memo\n2026-01-02,-5,lunch\n'} Step={DateStep} onSet={vi.fn()} />)
  const user = userEvent.setup()
  expect(screen.getAllByRole('radio')).toHaveLength(1)
  expect(screen.queryByRole('radio', { name: /from “Amount”/ })).not.toBeInTheDocument()
  expect(screen.queryByRole('radio', { name: 'lunch from “Memo”' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Show all' }))
  expect(screen.getByRole('radio', { name: 'lunch from “Memo”' })).toBeInTheDocument()
  expect(screen.getByRole('radio', { name: /from “Amount”/ })).toHaveAccessibleName('-5.00 from “Amount”')
  expect(screen.getByRole('radio', { name: /from “Amount”/ })).toHaveAccessibleDescription('used for amount')
  expect(screen.queryByRole('button', { name: 'Show all' })).not.toBeInTheDocument()
})

it('asks how to read an ambiguous date, and the answer changes how the first row reads', async () => {
  render(<StepHarness text={'Date,Amount\n03/04/2026,-5\n05/06/2026,1\n'} Step={DateStep} onSet={vi.fn()} />)
  const user = userEvent.setup()
  expect(screen.getByText('Is 03/04/2026 April 3 or March 4?')).toBeInTheDocument()
  expect(screen.getByRole('radio', { name: 'April 3, 2026' })).toBeChecked()
  expect(screen.getByRole('status', { name: 'first row date' })).toHaveTextContent('2026-04-03')
  await user.click(screen.getByRole('radio', { name: 'March 4, 2026' }))
  expect(screen.getByRole('status', { name: 'first row date' })).toHaveTextContent('2026-03-04')
})

it('counts the rows whose date cannot be read', () => {
  render(<StepHarness text={'Date,Amount\n2026-01-02,-5\n2026-01-03,1\n2026-01-04,1\n2026-01-05,1\n2026-01-06,1\n2026-01-07,1\n2026-01-08,1\n2026-01-09,1\nsoon,1\nlater,2\n'} Step={DateStep} onSet={vi.fn()} />)
  expect(screen.getByText('2 rows have no readable date and will be skipped')).toBeInTheDocument()
})

it('one amount column shows how spending looks with a switch to flip it; separate columns hide the switch', async () => {
  render(<StepHarness text={'Date,Amount,In,Out\n2026-01-02,-4.50,,4.50\n'} Step={AmountStep} onSet={vi.fn()} />)
  const user = userEvent.setup()
  expect(screen.getByText('Spending shows as: -4.50')).toBeInTheDocument()
  await user.click(screen.getByRole('switch', { name: 'Flip the signs' }))
  expect(screen.getByText('Spending shows as: 4.50')).toBeInTheDocument()
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
  expect(moneyOut().getByRole('radio', { name: /from “Out”/ })).toBeChecked()
  expect(moneyIn().getByRole('radio', { name: /from “Out”/ })).toHaveAccessibleDescription('used for money out')
  await user.click(moneyOut().getByRole('radio', { name: 'None of these is money out' }))
  expect(onSet).toHaveBeenLastCalledWith('amount', { amount: expect.objectContaining({ inColumn: 2, outColumn: null }) })
})

it('the amount step answers with the first amounts as they will be added', () => {
  render(<StepHarness text={'Date,Amount,Memo\n2026-01-02,-4.50,Coffee\n2026-01-03,100,Salary\n'} Step={AmountStep} onSet={vi.fn()} open={false} />)
  const items = within(screen.getByRole('list', { name: 'How payments will look' })).getAllByRole('listitem')
  expect(items).toHaveLength(2)
  expect(items[0]).toHaveTextContent(/^−4\.5/)
  expect(items[1]).toHaveTextContent(/^\+100/)
  expect(items[1]).toHaveClass('text-income')
  expect(items[0]).not.toHaveClass('text-income')
})

it('rows whose amount cannot be read are a quiet note, not a warning', () => {
  render(<StepHarness text={'Date,Amount\n2026-01-02,-5\n2026-01-03,1\n2026-01-04,1\n2026-01-05,1\n2026-01-06,1\n2026-01-07,1\n2026-01-08,1\n2026-01-09,1\n2026-01-10,1\n2026-01-11,?\n'} Step={AmountStep} onSet={vi.fn()} />)
  expect(screen.getByText('1 row has no readable amount and will be skipped')).toHaveClass('text-muted-foreground')
})

it('spending is shown with the file’s first negative amount, as it will be imported', async () => {
  render(<StepHarness text={'Date,Amount\n2026-01-02,12.00\n2026-01-03,-4.50\n'} Step={AmountStep} onSet={vi.fn()} />)
  const user = userEvent.setup()
  expect(screen.getByText('Spending shows as: -4.50')).toBeInTheDocument()
  await user.click(screen.getByRole('switch', { name: 'Flip the signs' }))
  expect(screen.getByText('Spending shows as: 4.50')).toBeInTheDocument()
})

it('spending is written the way the file writes it, a decimal comma included', async () => {
  render(<StepHarness text={'Datum;Betrag\n03.11.2026;-12,50\n05.11.2026;2.500,00\n'} Step={AmountStep} onSet={vi.fn()} />)
  const user = userEvent.setup()
  expect(screen.getByText('Spending shows as: -12,50')).toBeInTheDocument()
  await user.click(screen.getByRole('switch', { name: 'Flip the signs' }))
  expect(screen.getByText('Spending shows as: 12,50')).toBeInTheDocument()
})

it('with no negative amount, spending is shown with the file’s first amount, as it will be imported', async () => {
  render(<StepHarness text={'Date,Amount\n2026-01-02,12.00\n2026-01-03,3.00\n'} Step={AmountStep} onSet={vi.fn()} />)
  const user = userEvent.setup()
  expect(screen.getByText('Spending shows as: 12.00')).toBeInTheDocument()
  await user.click(screen.getByRole('switch', { name: 'Flip the signs' }))
  expect(screen.getByText('Spending shows as: -12.00')).toBeInTheDocument()
})

it('a guessed answer shows no choices; Change puts them in its place without answering anything', async () => {
  const onSet = vi.fn()
  render(<StepHarness text={'Date,Amount,Memo\n2026-01-02,-5,REWE Markt\n2026-01-03,7,Gehalt\n'} Step={DescriptionStep} onSet={onSet} open={false} />)
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

it('with money in and out, the list stays open for the second pick and Done goes back to the answer', async () => {
  render(<StepHarness text={'Date,Amount,In,Out\n2026-01-02,-4.50,,4.50\n2026-01-03,7.00,7.00,\n'} Step={AmountStep} onSet={vi.fn()} open={false} />)
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Change' }))
  await user.click(screen.getByRole('radio', { name: 'Money in and money out' }))
  await user.click(within(screen.getByRole('radiogroup', { name: 'Which of these is money in?' })).getByRole('radio', { name: /from “In”/ }))
  expect(screen.getByRole('radiogroup', { name: 'Which of these is money out?' })).toBeInTheDocument()
  await user.click(within(screen.getByRole('radiogroup', { name: 'Which of these is money out?' })).getByRole('radio', { name: /from “Out”/ }))
  await user.click(screen.getByRole('button', { name: 'Done' }))
  expect(screen.queryByRole('radiogroup', { name: 'Which of these is money out?' })).not.toBeInTheDocument()
  expect(screen.getByText('from “In” · from “Out”')).toBeInTheDocument()
})

it('what does not fit waits behind Show all, unless it is the chosen answer, which comes first', async () => {
  const text =
    'Date,Amount,Ref,Memo\n' +
    '2026-01-02,-5,01a1be03-f8df-7603-b2b1-2c3d4e5f6a7b,Coffee\n' +
    '2026-01-03,7,b298d99e-4fb1-42ca-97a2-9f00d01aeb19,Salary\n'
  const file = parseCsvText(text)
  const profiles = profileColumns(file)
  const onSelect = vi.fn()
  const { rerender } = render(<ColumnChoices profiles={profiles} label="Which?" selected={3} onSelect={onSelect} candidates={[3]} />)
  const user = userEvent.setup()
  expect(screen.getAllByRole('radio')).toHaveLength(1)
  expect(screen.getByRole('radio', { name: 'Coffee · Salary from “Memo”' })).toBeChecked()
  await user.click(screen.getByRole('button', { name: 'Show all' }))
  expect(screen.getAllByRole('radio').map((r) => r.textContent)).toEqual([
    'Coffee · Salaryfrom “Memo”',
    'Jan 2, 2026 · Jan 3, 2026from “Date”',
    '-5.00 · 7.00from “Amount”',
    expect.stringMatching(/from “Ref”$/),
  ])
  rerender(<ColumnChoices key="fresh" profiles={profiles} label="Which?" selected={2} onSelect={onSelect} candidates={[3]} />)
  expect(screen.getAllByRole('radio')[0]).toHaveAccessibleName(/from “Ref”/)
  expect(screen.getByRole('radio', { name: /from “Ref”/ })).toBeChecked()
  expect(screen.getByRole('button', { name: 'Show all' })).toBeInTheDocument()
  await user.click(screen.getByRole('radio', { name: /from “Memo”/ }))
  expect(onSelect).toHaveBeenCalledWith(3)
})

it('with nothing that fits, every part of the file is listed straight away', () => {
  const file = parseCsvText('Date,Amount\n2026-01-02,-5\n')
  render(<ColumnChoices profiles={profileColumns(file)} label="Which?" selected={null} onSelect={vi.fn()} candidates={[]} />)
  expect(screen.getAllByRole('radio')).toHaveLength(2)
  expect(screen.queryByRole('button', { name: 'Show all' })).not.toBeInTheDocument()
})

it('a file without a header row shows its values with no names beside them', () => {
  const file = parseCsvText('2026-01-01,-1,Tea\n2026-01-02,-5,Coffee\n2026-01-03,7,Salary\n')
  render(<ColumnChoices profiles={profileColumns(file)} label="Which?" selected={2} onSelect={vi.fn()} candidates={[2]} />)
  expect(screen.getByRole('radio', { name: 'Coffee · Salary' })).toBeChecked()
  expect(screen.queryByText(/from “/)).not.toBeInTheDocument()
})
