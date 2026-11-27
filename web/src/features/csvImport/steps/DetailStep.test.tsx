import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts, fixtureOwner } from '@/test/fixtures'
import { profileColumns } from '../columns'
import { parseCsvText } from '../parse'
import { prepareRows, type StepId, type WizardAnswers } from '../prepare'
import { claimPicked } from '../wizard'
import { CategoryStep, PayeeStep } from './DetailStep'

const noExtras: WizardAnswers['extras'] = { payee: null, category: null, tag: null, labels: null, labelsSeparator: ';' }

interface HarnessProps {
  step: 'category' | 'payee'
  text: string
  extras?: WizardAnswers['extras']
  description?: number | null
  account?: WizardAnswers['account']
}

function Harness({ step, text, extras = noExtras, description = null, account = { kind: 'new', name: 'Main', currencyId: 'cur-usd' } }: HarnessProps) {
  const [file] = useState(() => parseCsvText(text))
  const [profiles] = useState(() => profileColumns(file))
  const [touched, setTouched] = useState<ReadonlySet<StepId>>(() => new Set())
  const [choosing, setChoosing] = useState(false)
  const [answers, setAnswersState] = useState<WizardAnswers>(
    () =>
      ({
        account,
        date: { column: 0, order: 'ymd' },
        amount: { mode: 'single', column: 1, inColumn: null, outColumn: null, flip: false, sep: '.' },
        description,
        extras,
      }) as WizardAnswers,
  )
  const rows = prepareRows(file, answers, new Map())
  const Step = step === 'category' ? CategoryStep : PayeeStep
  return (
    <>
      <Step
        state={{ file, fileName: 'a.csv', profiles, answers, touched, fixes: new Map(), rows, isExport: false }}
        setAnswers={(changed, patch) => {
          setAnswersState((prev) => claimPicked(prev, { ...prev, ...patch }))
          setTouched((prev) => new Set(prev).add(changed))
        }}
        setFixes={() => {}}
        goTo={() => {}}
        choosing={choosing}
        change={() => setChoosing(true)}
        closeChoices={() => setChoosing(false)}
        keepChoices={() => setChoosing(true)}
      />
      <output aria-label="answers">{JSON.stringify({ description: answers.description, ...answers.extras })}</output>
    </>
  )
}

function renderStep(props: HarnessProps) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <Harness {...props} />
    </QueryClientProvider>,
  )
}

const answersOf = () => JSON.parse(screen.getByLabelText('answers').textContent ?? '{}')

beforeEach(() => {
  server.use(...coreHandlers())
})

const GROUPS = 'Date,Amount,Memo,Group\n2026-01-02,-5,Lunch at the corner cafe,Strata\n2026-01-03,-5,Car insurance for November,Interest\n2026-01-04,-5,Weekly shop,Strata\n2026-01-05,-5,Petrol,Interest\n'

it('a guessed category answers with its values and where they come from; Change opens the choices', async () => {
  renderStep({ step: 'category', text: GROUPS, extras: { ...noExtras, category: 3 } })
  const user = userEvent.setup()
  expect(screen.getByText('Strata · Interest')).toBeInTheDocument()
  expect(screen.getByText('from “Group”')).toBeInTheDocument()
  expect(screen.queryByRole('radio')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Change' }))
  expect(screen.getByRole('radiogroup', { name: 'Which of these is the category?' })).toBeInTheDocument()
  expect(screen.getByRole('radio', { name: 'Strata · Interest from “Group”' })).toBeChecked()
})

it('short names that repeat come first as the category; free text follows under Other parts of your file', async () => {
  renderStep({ step: 'category', text: GROUPS, extras: { ...noExtras, category: 3 } })
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Change' }))
  expect(screen.getAllByRole('radio')[0]).toHaveAccessibleName(/from “Group”/)
  expect(screen.getByText('Other parts of your file')).toBeInTheDocument()
  expect(screen.getByRole('radio', { name: /from “Memo”/ })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Show all' })).not.toBeInTheDocument()
})

it('with nothing that looks like a category, the whole file is listed under a note', async () => {
  renderStep({ step: 'category', text: 'Date,Amount,Memo\n2026-01-02,-5,Lunch at the corner cafe\n2026-01-03,-5,Car insurance for November\n', description: 2 })
  await userEvent.click(screen.getByRole('button', { name: 'Change' }))
  expect(screen.getByText('Nothing in your file looks like a category, so here is everything.')).toBeInTheDocument()
  expect(screen.getByRole('radio', { name: /from “Date”/ })).toBeInTheDocument()
})

it('picking None leaves categories out and says so calmly', async () => {
  renderStep({ step: 'category', text: GROUPS, extras: { ...noExtras, category: 3 } })
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Change' }))
  await user.click(screen.getByRole('radio', { name: 'The file has no category' }))
  expect(answersOf().category).toBeNull()
  expect(screen.getByText('Categories are left out. You can sort transactions into categories later.')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Change' })).toBeInTheDocument()
})

it('with nothing guessed but something that fits, the step asks', () => {
  renderStep({ step: 'category', text: GROUPS })
  expect(screen.getByRole('radiogroup', { name: 'Which of these is the category?' })).toBeInTheDocument()
  expect(screen.getByRole('radio', { name: 'The file has no category' })).toBeChecked()
})

it('a file with no categories answers so calmly; Change lists the whole file', async () => {
  renderStep({ step: 'category', text: 'Date,Amount,Memo\n2026-01-02,-5,Lunch\n', description: 2 })
  const user = userEvent.setup()
  expect(screen.getByText('Your file has no categories. You can sort transactions into categories later.')).toBeInTheDocument()
  expect(screen.queryByRole('radio')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Change' }))
  expect(screen.getByRole('radio', { name: 'The file has no category' })).toBeChecked()
  expect(screen.getByRole('radio', { name: /from “Date”/ })).toHaveAccessibleDescription('used for date')
})

it('a column Description reads is under Other parts of your file; picking it takes it from Description', async () => {
  renderStep({ step: 'category', text: 'Date,Amount,Memo,Shop\n2026-01-02,-5,Food,Cafe\n2026-01-03,-5,Food,Cafe\n', description: 2 })
  const user = userEvent.setup()
  expect(screen.getByText('Other parts of your file')).toBeInTheDocument()
  expect(screen.getByRole('radio', { name: 'Food from “Memo”' })).toHaveAccessibleDescription('used for description')
  await user.click(screen.getByRole('radio', { name: 'Food from “Memo”' }))
  expect(answersOf()).toMatchObject({ category: 2, description: null })
  expect(screen.getByText('Food')).toBeInTheDocument()
})

it('a payee pick takes its column from the category: a column belongs to one detail', async () => {
  renderStep({ step: 'payee', text: 'Date,Amount,Shop\n2026-01-02,-5,Cafe\n', extras: { ...noExtras, category: 2 } })
  const user = userEvent.setup()
  expect(screen.getByText('Your file doesn’t say who was paid. You can add payees later.')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Change' }))
  await user.click(screen.getByRole('radio', { name: 'Cafe from “Shop”' }))
  expect(answersOf()).toMatchObject({ payee: 2, category: null })
})

it('any text is offered as the payee, the guessed one answers with its values', async () => {
  renderStep({ step: 'payee', text: GROUPS, extras: { ...noExtras, payee: 2 } })
  const user = userEvent.setup()
  expect(screen.getByText('Lunch at the corner cafe · Car insurance for November · Weekly shop …')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Change' }))
  expect(screen.getByRole('radiogroup', { name: 'Which of these says who was paid?' })).toBeInTheDocument()
  expect(screen.getByRole('radio', { name: /from “Group”/ })).toBeInTheDocument()
})

it('counts new categories case-insensitively against the owner’s, archived ones included', async () => {
  renderStep({ step: 'category', text: 'Date,Amount,Group\n2026-01-02,-5,FOOD\n2026-01-03,-5,old\n2026-01-04,-5,Travel\n2026-01-05,-5,travel\n', extras: { ...noExtras, category: 2 } })
  expect(await screen.findByText('1 new category will be created')).toBeInTheDocument()
})

it('a name found only on a row that will be skipped is not counted as created', async () => {
  renderStep({ step: 'category', text: 'Date,Amount,Group\n2026-01-02,-5,Travel\nsoon,-5,Garden\n', extras: { ...noExtras, category: 2 } })
  expect(await screen.findByText('1 new category will be created')).toBeInTheDocument()
})

it('counts a category within its kind: an income row naming an expense category makes a new one', async () => {
  renderStep({ step: 'category', text: 'Date,Amount,Group\n2026-01-02,-5,Food\n2026-01-03,5,food\n', extras: { ...noExtras, category: 2 } })
  expect(await screen.findByText('1 new category will be created')).toBeInTheDocument()
})

const sharedAccount = (role: 'admin' | 'user') => ({
  ...fixtureAccounts[0],
  id: 'shared',
  owner: { id: 'u2', avatar: 'face:amber', name: 'Bo' },
  sharedAccess: [{ user: fixtureOwner, role, isAccepted: 1 }],
})

it('into a shared account with a user grant, says new categories are left out instead of counting them', async () => {
  server.use(...coreHandlers({ accounts: [...fixtureAccounts, sharedAccount('user')] }))
  renderStep({ step: 'category', text: 'Date,Amount,Group\n2026-01-02,-5,Gadgets\n', extras: { ...noExtras, category: 2 }, account: { kind: 'existing', accountId: 'shared' } })
  expect(await screen.findByText('Categories this account’s owner doesn’t have yet are left out: only the owner or an admin can add them.')).toBeInTheDocument()
  expect(screen.queryByText(/will be created/)).not.toBeInTheDocument()
})

it('into a shared account with an admin grant, counts the new categories as usual', async () => {
  server.use(...coreHandlers({ accounts: [...fixtureAccounts, sharedAccount('admin')] }))
  renderStep({ step: 'category', text: 'Date,Amount,Group\n2026-01-02,-5,Gadgets\n', extras: { ...noExtras, category: 2 }, account: { kind: 'existing', accountId: 'shared' } })
  expect(await screen.findByText('1 new category will be created')).toBeInTheDocument()
  expect(screen.queryByText(/only the owner or an admin/)).not.toBeInTheDocument()
})

it('into a shared account with a user grant, the payee step says new payees are left out', async () => {
  server.use(...coreHandlers({ accounts: [...fixtureAccounts, sharedAccount('user')] }))
  renderStep({ step: 'payee', text: 'Date,Amount,Shop\n2026-01-02,-5,Cafe\n', extras: { ...noExtras, payee: 2 }, account: { kind: 'existing', accountId: 'shared' } })
  expect(await screen.findByText('Payees this account’s owner doesn’t have yet are left out: only the owner or an admin can add them.')).toBeInTheDocument()
})
