import { useState } from 'react'
import type { ComponentType } from 'react'
import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { ImportWizard } from '../ImportWizard'
import { profileColumns } from '../columns'
import { guessAnswers, redetectFormats } from '../guess'
import { parseCsvText } from '../parse'
import { prepareRows, type StepId, type WizardAnswers } from '../prepare'
import type { StepBodyProps } from '../wizard'
import { AmountStep } from './AmountStep'
import { DateStep } from './DateStep'

const csv = (text: string, name = 'statement.csv') => new File([text], name, { type: 'text/csv' })

function withQueries(node: React.ReactNode) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={queryClient}>{node}</QueryClientProvider>
}

function renderWizard(text: string) {
  render(withQueries(<ImportWizard open onClose={() => {}} onComplete={vi.fn()} initialFile={csv(text)} />))
}

// A step on its own, with answers kept the way the wizard keeps them; the first
// prepared row is what the Check step's first card shows.
function StepHarness({ text, Step, onSet }: { text: string; Step: ComponentType<StepBodyProps>; onSet: (step: StepId, patch: Partial<WizardAnswers>) => void }) {
  const [file] = useState(() => parseCsvText(text))
  const [profiles] = useState(() => profileColumns(file))
  const [answers, setAnswersState] = useState(() =>
    guessAnswers(file, profiles, { hasWritableAccounts: true, userCurrencyId: 'cur-usd', locale: 'en-GB', defaultAccountName: 'statement' }),
  )
  const rows = prepareRows(file, answers, new Map())
  const setAnswers = (step: StepId, patch: Partial<WizardAnswers>) => {
    onSet(step, patch)
    setAnswersState((prev) => {
      const next = { ...prev, ...patch }
      return redetectFormats(next, file, 'en-GB', { date: next.date.column !== prev.date.column && next.date.order === prev.date.order })
    })
  }
  return (
    <>
      <Step
        state={{ file, fileName: 'statement.csv', profiles, answers, touched: new Set(), fixes: new Map(), rows, isExport: false }}
        setAnswers={setAnswers}
        setFixes={() => {}}
        goTo={() => {}}
      />
      <output aria-label="first row date">{rows[0]?.date ?? ''}</output>
    </>
  )
}

beforeEach(() => {
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
  await screen.findByRole('heading', { name: 'Which account is this statement from?' })
  expect(screen.getByRole('radio', { name: 'A new account' })).toBeChecked()
  expect(screen.queryByRole('radio', { name: 'An account I already have' })).not.toBeInTheDocument()
  expect(screen.getByLabelText('Account name')).toHaveValue('statement')
  expect(screen.getByRole('combobox', { name: 'Currency' })).toHaveValue('USD, $, US Dollar')
})

it('a typed account name is saved after a pause in typing', async () => {
  server.use(...coreHandlers({ accounts: [] }))
  renderWizard('Date,Amount\n2026-01-02,-5.50\n')
  await screen.findByRole('heading', { name: 'Which account is this statement from?' })
  fireEvent.change(screen.getByLabelText('Account name'), { target: { value: '' } })
  expect(screen.getByRole('button', { name: 'Account' })).toBeInTheDocument()
  expect(await screen.findByRole('button', { name: /^Account.*needs your attention/ })).toBeInTheDocument()
})

it('a typed account name is kept when the step is left before the pause, with the field still focused', async () => {
  server.use(...coreHandlers({ accounts: [] }))
  renderWizard('Date,Amount\n2026-01-02,-5.50\n')
  await screen.findByRole('heading', { name: 'Which account is this statement from?' })
  const name = screen.getByLabelText('Account name')
  name.focus()
  fireEvent.change(name, { target: { value: 'Travel card' } })
  fireEvent.click(screen.getByRole('button', { name: /^Date/ }))
  expect(screen.getByRole('heading', { name: 'Which column has the date?' })).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: /^Account/ }))
  expect(screen.getByLabelText('Account name')).toHaveValue('Travel card')
})

it('with accounts, the Account step offers the account picker first', async () => {
  renderWizard('Date,Amount\n2026-01-02,-5.50\n')
  await screen.findByRole('heading', { name: 'Which account is this statement from?' })
  expect(screen.getByRole('radio', { name: 'An account I already have' })).toBeChecked()
  expect(screen.getByRole('combobox', { name: 'Account' })).toBeInTheDocument()
})

it('lists the accounts a per-row column names, with a currency only for the new ones', async () => {
  renderWizard('Date,Amount,Wallet\n2026-01-02,-5.50,Cash\n2026-01-03,7.00,Travel card\n2026-01-04,1.00, cash \n')
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Which account is this statement from?' })
  await user.click(screen.getByRole('radio', { name: 'Each row says which account' }))
  await user.click(screen.getByRole('radio', { name: /^Wallet/ }))
  const list = screen.getByRole('list', { name: 'Accounts in this file' })
  const rows = within(list).getAllByRole('listitem')
  expect(rows).toHaveLength(2)
  expect(rows[0]).toHaveTextContent('CashExisting account')
  expect(within(rows[0]).queryByRole('combobox')).not.toBeInTheDocument()
  expect(within(rows[1]).getByRole('combobox', { name: 'Currency for Travel card' })).toHaveValue('USD, $, US Dollar')
})

it('choosing a date card updates the answer and marks the Date step as answered', async () => {
  const onSet = vi.fn()
  render(<StepHarness text={'Date,Valued,Amount\n2026-01-02,2026-01-05,-5\n'} Step={DateStep} onSet={onSet} />)
  const user = userEvent.setup()
  expect(screen.getByRole('radio', { name: /^Date/ })).toBeChecked()
  await user.click(screen.getByRole('radio', { name: /^Valued/ }))
  expect(onSet).toHaveBeenCalledWith('date', { date: expect.objectContaining({ column: 1 }) })
  expect(screen.getByRole('radio', { name: /^Valued/ })).toBeChecked()
  expect(screen.getByRole('status', { name: 'first row date' })).toHaveTextContent('2026-01-05')
})

it('asks how to read an ambiguous date, and the answer changes how the first row reads', async () => {
  render(<StepHarness text={'Date,Amount\n03/04/2026,-5\n05/06/2026,1\n'} Step={DateStep} onSet={vi.fn()} />)
  const user = userEvent.setup()
  expect(screen.getByText('Is 03/04/2026 the April 3, 2026 or March 4, 2026?')).toBeInTheDocument()
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
  await user.click(screen.getByRole('radio', { name: 'Separate columns for money in and money out' }))
  expect(screen.queryByRole('switch', { name: 'Flip the signs' })).not.toBeInTheDocument()
  expect(screen.getByRole('radiogroup', { name: 'Money in' })).toBeInTheDocument()
  expect(screen.getByRole('radiogroup', { name: 'Money out' })).toBeInTheDocument()
})

it('spending is shown with the file’s first negative amount, as it will be imported', async () => {
  render(<StepHarness text={'Date,Amount\n2026-01-02,12.00\n2026-01-03,-4.50\n'} Step={AmountStep} onSet={vi.fn()} />)
  const user = userEvent.setup()
  expect(screen.getByText('Spending shows as: -4.50')).toBeInTheDocument()
  await user.click(screen.getByRole('switch', { name: 'Flip the signs' }))
  expect(screen.getByText('Spending shows as: 4.50')).toBeInTheDocument()
})

it('with no negative amount, spending is shown with the file’s first amount, as it will be imported', async () => {
  render(<StepHarness text={'Date,Amount\n2026-01-02,12.00\n2026-01-03,3.00\n'} Step={AmountStep} onSet={vi.fn()} />)
  const user = userEvent.setup()
  expect(screen.getByText('Spending shows as: 12.00')).toBeInTheDocument()
  await user.click(screen.getByRole('switch', { name: 'Flip the signs' }))
  expect(screen.getByText('Spending shows as: -12.00')).toBeInTheDocument()
})
