import { useState } from 'react'
import { render, screen, waitFor, within } from '@testing-library/react'
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

const linesOf = (from: string) => within(screen.getByText(from).parentElement as HTMLElement).getAllByRole('listitem').map((li) => li.textContent)
const GROUPS = 'Date,Amount,Memo,Group\n2026-01-02,-5,Lunch at the corner cafe,Strata\n2026-01-03,-5,Car insurance for November,Interest\n2026-01-04,-5,Weekly shop,Strata\n2026-01-05,-5,Petrol,Interest\n'

it('a guessed category answers with its values and where they come from; Change opens the choices', async () => {
  renderStep({ step: 'category', text: GROUPS, extras: { ...noExtras, category: 3 } })
  const user = userEvent.setup()
  expect(linesOf('from “Group”')).toEqual(['Strata', 'Interest'])
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

it('with nothing assigned but something that fits, the step asks with nothing picked, None included', () => {
  renderStep({ step: 'category', text: GROUPS })
  expect(screen.getByRole('radiogroup', { name: 'Which of these is the category?' })).toBeInTheDocument()
  expect(screen.getByRole('radio', { name: 'The file has no category' })).not.toBeChecked()
  expect(screen.queryByRole('radio', { checked: true })).not.toBeInTheDocument()
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
  expect(screen.getAllByText('Food')[0]).toBeInTheDocument()
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
  expect(linesOf('from “Memo”')).toEqual(['Lunch at the corner cafe', 'Car insurance for November', 'Weekly shop', 'and 1 more'])
  await user.click(screen.getByRole('button', { name: 'Change' }))
  expect(screen.getByRole('radiogroup', { name: 'Which of these says who was paid?' })).toBeInTheDocument()
  expect(screen.getByRole('radio', { name: /from “Group”/ })).toBeInTheDocument()
})

const rowsOf = (list: string) => within(screen.getByRole('list', { name: list })).getAllByRole('listitem').map((li) => li.textContent)

it('lists the categories in the file with their transactions, matching the owner’s case-insensitively, archived ones included', async () => {
  renderStep({ step: 'category', text: 'Date,Amount,Group\n2026-01-02,-5,FOOD\n2026-01-03,-5,old\n2026-01-04,-5,Travel\n2026-01-05,-5,travel\n', extras: { ...noExtras, category: 2 } })
  await screen.findAllByText('Your existing category')
  expect(rowsOf('Categories in your file')).toEqual([
    'Travel · 2 transactions New category',
    'FOOD · 1 transaction Your existing category',
    'old · 1 transaction Your existing category',
  ])
})

it('the list counts only rows that will be imported', async () => {
  renderStep({ step: 'category', text: 'Date,Amount,Group\n2026-01-02,-5,Travel\nsoon,-5,Garden\n', extras: { ...noExtras, category: 2 } })
  expect(await screen.findByText('New category')).toBeInTheDocument()
  expect(rowsOf('Categories in your file')).toEqual(['Travel · 1 transaction New category'])
})

it('a name both kinds of row use says which kind is new, or that both are', async () => {
  renderStep({ step: 'category', text: 'Date,Amount,Group\n2026-01-02,-5,Food\n2026-01-03,5,food\n2026-01-04,5,Bonus\n2026-01-05,-5,Bonus\n', extras: { ...noExtras, category: 2 } })
  await waitFor(() =>
    expect(rowsOf('Categories in your file')).toEqual(['Food · 2 transactions New category · income', 'Bonus · 2 transactions New category · income and expense']),
  )
})

it('a long list shows the eight most used, then and N more expands the rest', async () => {
  const lines = Array.from({ length: 10 }, (_, i) => Array.from({ length: 10 - i }, () => `2026-01-02,-5,Group ${i}`).join('\n')).join('\n')
  renderStep({ step: 'category', text: `Date,Amount,Group\n${lines}\n`, extras: { ...noExtras, category: 2 } })
  const user = userEvent.setup()
  await screen.findAllByText('New category')
  expect(rowsOf('Categories in your file')).toHaveLength(8)
  expect(rowsOf('Categories in your file')[0]).toBe('Group 0 · 10 transactions New category')
  await user.click(screen.getByRole('button', { name: 'and 2 more' }))
  expect(rowsOf('Categories in your file')).toHaveLength(10)
  expect(screen.queryByRole('button', { name: /more/ })).not.toBeInTheDocument()
})

it('no list while categories are left out', () => {
  renderStep({ step: 'category', text: GROUPS })
  expect(screen.queryByRole('list', { name: 'Categories in your file' })).not.toBeInTheDocument()
})

const sharedAccount = (role: 'admin' | 'user') => ({
  ...fixtureAccounts[0],
  id: 'shared',
  owner: { id: 'u2', avatar: 'face:amber', name: 'Bo' },
  sharedAccess: [{ user: fixtureOwner, role, isAccepted: 1 }],
})

it('into a shared account with a user grant, a category the owner lacks reads left out', async () => {
  server.use(...coreHandlers({ accounts: [...fixtureAccounts, sharedAccount('user')] }))
  renderStep({ step: 'category', text: 'Date,Amount,Group\n2026-01-02,-5,Gadgets\n', extras: { ...noExtras, category: 2 }, account: { kind: 'existing', accountId: 'shared' } })
  expect(await screen.findByText('Left out (shared account)')).toBeInTheDocument()
  expect(screen.queryByText('New category')).not.toBeInTheDocument()
})

it('into a shared account with an admin grant, new categories read as new', async () => {
  server.use(...coreHandlers({ accounts: [...fixtureAccounts, sharedAccount('admin')] }))
  renderStep({ step: 'category', text: 'Date,Amount,Group\n2026-01-02,-5,Gadgets\n', extras: { ...noExtras, category: 2 }, account: { kind: 'existing', accountId: 'shared' } })
  expect(await screen.findByText('New category')).toBeInTheDocument()
  expect(screen.queryByText('Left out (shared account)')).not.toBeInTheDocument()
})

it('lists the payees in the file, new or the user’s own', async () => {
  renderStep({ step: 'payee', text: 'Date,Amount,Shop\n2026-01-02,-5,grocer\n2026-01-03,-5,Cafe\n2026-01-04,-5,Cafe\n', extras: { ...noExtras, payee: 2 } })
  await screen.findByText('Your existing payee')
  expect(rowsOf('Payees in your file')).toEqual(['Cafe · 2 transactions New payee', 'grocer · 1 transaction Your existing payee'])
})

it('into a shared account with a user grant, a payee the owner lacks reads left out', async () => {
  server.use(...coreHandlers({ accounts: [...fixtureAccounts, sharedAccount('user')] }))
  renderStep({ step: 'payee', text: 'Date,Amount,Shop\n2026-01-02,-5,Cafe\n', extras: { ...noExtras, payee: 2 }, account: { kind: 'existing', accountId: 'shared' } })
  expect(await screen.findByText('Left out (shared account)')).toBeInTheDocument()
})
