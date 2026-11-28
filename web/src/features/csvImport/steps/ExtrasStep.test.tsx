import { useState } from 'react'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts, fixtureOwner } from '@/test/fixtures'
import { profileColumns } from '../columns'
import { parseCsvText } from '../parse'
import { prepareRows, type WizardAnswers } from '../prepare'
import { ExtrasStep } from './ExtrasStep'

const noExtras: WizardAnswers['extras'] = { payee: null, category: null, tag: null, labels: null, labelsSeparator: ';' }

function Harness({ text, extras = noExtras, account = { kind: 'new', name: 'Main', currencyId: 'cur-usd' } }: { text: string; extras?: WizardAnswers['extras']; account?: WizardAnswers['account'] }) {
  const [file] = useState(() => parseCsvText(text))
  const [profiles] = useState(() => profileColumns(file))
  const [answers, setAnswersState] = useState<WizardAnswers>(() => ({
    account,
    date: { column: 0, order: 'ymd' },
    amount: { mode: 'single', column: 1, flip: false, sep: '.' },
    description: null,
    extras,
  }) as WizardAnswers)
  const rows = prepareRows(file, answers, new Map())
  return (
    <>
      <ExtrasStep
        state={{ file, fileName: 'a.csv', profiles, answers, touched: new Set(), fixes: new Map(), rows, isExport: false }}
        setAnswers={(_step, patch) => setAnswersState((prev) => ({ ...prev, ...patch }))}
        setFixes={() => {}}
        goTo={() => {}}
        choosing
        change={() => {}}
        closeChoices={() => {}}
        keepChoices={() => {}}
      />
      <output aria-label="extras">{JSON.stringify(answers.extras)}</output>
    </>
  )
}

function renderStep(text: string, extras?: WizardAnswers['extras'], account?: WizardAnswers['account']) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <Harness text={text} extras={extras} account={account} />
    </QueryClientProvider>,
  )
}

const extrasOf = () => JSON.parse(screen.getByLabelText('extras').textContent ?? '{}')

beforeEach(() => {
  server.use(...coreHandlers())
})

const optionsOf = (role: string) => within(screen.getByRole('combobox', { name: role })).getAllByRole('option').map((o) => o.textContent)

it('offers each detail what the earlier steps left free, values first', () => {
  renderStep('Date,Amount,Memo,Group\n2026-01-02,-5,lunch,Food\n')
  expect(optionsOf('Budget tag')).toEqual(['Don’t bring it', 'lunch — from “Memo”', 'Food — from “Group”'])
  expect(optionsOf('Reporting tags')).toEqual(optionsOf('Budget tag'))
  expect(screen.getByRole('combobox', { name: 'Reporting tags' })).toHaveValue('')
  expect(screen.queryByRole('combobox', { name: 'Category' })).not.toBeInTheDocument()
  expect(screen.queryByRole('combobox', { name: 'Payee' })).not.toBeInTheDocument()
})

it('the category and payee columns are not offered: their own steps read them', () => {
  renderStep('Date,Amount,Memo,Group,Shop\n2026-01-02,-5,lunch,Food,Cafe\n', { ...noExtras, category: 3, payee: 4 })
  expect(optionsOf('Budget tag')).toEqual(['Don’t bring it', 'lunch — from “Memo”'])
})

it('ids and currency codes are offered after the likely parts, under Other parts of your file', () => {
  renderStep('Date,Amount,Ref,Memo,Cur\n2026-01-02,-5,01a1be03-f8df-7603-b2b1-2c3d4e5f6a7b,lunch,EUR\n')
  expect(optionsOf('Budget tag')).toEqual(['Don’t bring it', 'lunch — from “Memo”', expect.stringMatching(/from “Ref”$/), 'EUR — from “Cur”'])
  const group = within(screen.getByRole('combobox', { name: 'Budget tag' })).getByRole('group', { name: 'Other parts of your file' })
  expect(within(group).getAllByRole('option').map((o) => o.textContent)).toEqual([expect.stringMatching(/from “Ref”$/), 'EUR — from “Cur”'])
  expect(screen.queryByRole('button', { name: 'Show all' })).not.toBeInTheDocument()
  expect(screen.queryByText(/so here is everything/)).not.toBeInTheDocument()
})

it('with nothing that looks like a tag or a label, every part of the file is offered under a note', () => {
  renderStep('Date,Amount,Ref\n2026-01-02,-5,01a1be03-f8df-7603-b2b1-2c3d4e5f6a7b\n')
  expect(screen.getByText('Nothing in your file looks like a tag or a label, so here is everything.')).toBeInTheDocument()
  expect(optionsOf('Budget tag')).toEqual(['Don’t bring it', expect.stringMatching(/from “Ref”$/)])
})

it('choosing a column for a detail brings it over', async () => {
  renderStep('Date,Amount,Memo,Group\n2026-01-02,-5,lunch,Food\n')
  const user = userEvent.setup()
  await user.selectOptions(screen.getByRole('combobox', { name: 'Budget tag' }), 'Food — from “Group”')
  expect(extrasOf().tag).toBe(3)
  await user.selectOptions(screen.getByRole('combobox', { name: 'Budget tag' }), 'Don’t bring it')
  expect(extrasOf().tag).toBeNull()
})

it('a column belongs to one detail: giving it to a second takes it from the first', async () => {
  renderStep('Date,Amount,One,Two\n2026-01-02,-5,a,b\n')
  const user = userEvent.setup()
  await user.selectOptions(screen.getByRole('combobox', { name: 'Budget tag' }), 'a — from “One”')
  expect(extrasOf().tag).toBe(2)
  await user.selectOptions(screen.getByRole('combobox', { name: 'Reporting tags' }), 'a — from “One”')
  expect(extrasOf()).toMatchObject({ labels: 2, tag: null })
  expect(screen.getByRole('combobox', { name: 'Budget tag' })).toHaveValue('')
})

const rowsOf = (list: string) => within(screen.getByRole('list', { name: list })).getAllByRole('listitem').map((li) => li.textContent)

it('lists reporting tags split by the separator; a budget tag’s name is left out', async () => {
  renderStep('Date,Amount,Marks\n2026-01-02,-5,Health;VACATION;trip\n2026-01-03,-5,Trip; fun;trip\n', { ...noExtras, labels: 2 })
  await screen.findAllByText('Your existing tag')
  expect(rowsOf('Reporting tags in your file')).toEqual([
    'trip · 2 transactions New tag',
    'Health · 1 transaction Your existing tag',
    'VACATION · 1 transaction Left out: already a budget tag',
    'fun · 1 transaction New tag',
  ])
})

it('a budget tag named like a reporting tag reads left out', async () => {
  renderStep('Date,Amount,Trip\n2026-01-02,-5,Health\n', { ...noExtras, tag: 2 })
  expect(await screen.findByText('Left out: already a reporting tag')).toBeInTheDocument()
})

it('lists budget tags whole, beside the reporting tags', async () => {
  renderStep('Date,Amount,Trip,Marks\n2026-01-02,-5,Vacation,a;b\n2026-01-03,-5,Rome;Milan,a\n', { ...noExtras, tag: 2, labels: 3 })
  await screen.findByText('Your existing tag')
  expect(rowsOf('Budget tags in your file')).toEqual(['Vacation · 1 transaction Your existing tag', 'Rome;Milan · 1 transaction New tag'])
  expect(rowsOf('Reporting tags in your file')).toEqual(['a · 2 transactions New tag', 'b · 1 transaction New tag'])
})

it('offers the separator only for a labels column and keeps a custom one', async () => {
  renderStep('Date,Amount,Marks\n2026-01-02,-5,a/b\n')
  const user = userEvent.setup()
  expect(screen.queryByRole('radiogroup', { name: /separated by/ })).not.toBeInTheDocument()
  await user.selectOptions(screen.getByRole('combobox', { name: 'Reporting tags' }), 'a/b — from “Marks”')
  expect(screen.getByRole('radio', { name: ';' })).toBeChecked()
  await user.type(screen.getByLabelText('Something else'), '/')
  expect(extrasOf().labelsSeparator).toBe('/')
  expect(rowsOf('Reporting tags in your file')).toEqual(['a · 1 transaction New tag', 'b · 1 transaction New tag'])
})

it('tags on separate lines of one cell split by New line', async () => {
  renderStep('Date,Amount,Marks\n2026-01-02,-5,"a\nb"\n', { ...noExtras, labels: 2 })
  const user = userEvent.setup()
  expect(rowsOf('Reporting tags in your file')).toHaveLength(1)
  await user.click(screen.getByRole('radio', { name: 'New line' }))
  expect(extrasOf().labelsSeparator).toBe('\n')
  expect(rowsOf('Reporting tags in your file')).toEqual(['a · 1 transaction New tag', 'b · 1 transaction New tag'])
})

const sharedAccount = (role: 'admin' | 'user') => ({
  ...fixtureAccounts[0],
  id: 'shared',
  owner: { id: 'u2', avatar: 'face:amber', name: 'Bo' },
  sharedAccess: [{ user: fixtureOwner, role, isAccepted: 1 }],
})

it('into a shared account with a user grant, a tag the owner lacks reads left out', async () => {
  server.use(...coreHandlers({ accounts: [...fixtureAccounts, sharedAccount('user')] }))
  renderStep('Date,Amount,Marks\n2026-01-02,-5,Gadgets\n', { ...noExtras, labels: 2 }, { kind: 'existing', accountId: 'shared' })
  expect(await screen.findByText('Left out (shared account)')).toBeInTheDocument()
  expect(screen.queryByText('New tag')).not.toBeInTheDocument()
})

it('into a shared account with an admin grant, new tags read as new', async () => {
  server.use(...coreHandlers({ accounts: [...fixtureAccounts, sharedAccount('admin')] }))
  renderStep('Date,Amount,Marks\n2026-01-02,-5,Gadgets\n', { ...noExtras, labels: 2 }, { kind: 'existing', accountId: 'shared' })
  expect(await screen.findByText('New tag')).toBeInTheDocument()
  expect(screen.queryByText('Left out (shared account)')).not.toBeInTheDocument()
})
