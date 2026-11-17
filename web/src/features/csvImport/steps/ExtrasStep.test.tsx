import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { profileColumns } from '../columns'
import { parseCsvText } from '../parse'
import { prepareRows, type WizardAnswers } from '../prepare'
import { ExtrasStep } from './ExtrasStep'

const noExtras: WizardAnswers['extras'] = { payee: null, category: null, tag: null, labels: null, labelsSeparator: ';' }

function Harness({ text, extras = noExtras }: { text: string; extras?: WizardAnswers['extras'] }) {
  const [file] = useState(() => parseCsvText(text))
  const [profiles] = useState(() => profileColumns(file))
  const [answers, setAnswersState] = useState<WizardAnswers>(() => ({
    account: { kind: 'new', name: 'Main', currencyId: 'cur-usd' },
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
      />
      <output aria-label="extras">{JSON.stringify(answers.extras)}</output>
    </>
  )
}

function renderStep(text: string, extras?: WizardAnswers['extras']) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <Harness text={text} extras={extras} />
    </QueryClientProvider>,
  )
}

const extrasOf = () => JSON.parse(screen.getByLabelText('extras').textContent ?? '{}')

beforeEach(() => {
  server.use(...coreHandlers())
})

it('lists only the columns the earlier steps left free, with their values', () => {
  renderStep('Date,Amount,Memo,Group\n2026-01-02,-5,lunch,Food\n')
  expect(screen.getByText('Memo')).toBeInTheDocument()
  expect(screen.getByText('lunch')).toBeInTheDocument()
  expect(screen.getByText('Group')).toBeInTheDocument()
  expect(screen.queryByText('Date')).not.toBeInTheDocument()
  expect(screen.queryByText('Amount')).not.toBeInTheDocument()
})

it('a role belongs to one column: giving it to a second column takes it from the first', async () => {
  renderStep('Date,Amount,One,Two\n2026-01-02,-5,a,b\n')
  const user = userEvent.setup()
  await user.selectOptions(screen.getByLabelText('Use “One” as'), 'category')
  expect(extrasOf().category).toBe(2)
  await user.selectOptions(screen.getByLabelText('Use “Two” as'), 'category')
  expect(extrasOf().category).toBe(3)
  expect(screen.getByLabelText('Use “One” as')).toHaveValue('')
})

it('counts new categories case-insensitively against the owner’s, archived ones included', async () => {
  renderStep('Date,Amount,Group\n2026-01-02,-5,FOOD\n2026-01-03,-5,old\n2026-01-04,-5,Travel\n2026-01-05,-5,travel\n', {
    ...noExtras,
    category: 2,
  })
  expect(await screen.findByText('1 new category will be created')).toBeInTheDocument()
})

it('counts new labels split by the separator, sharing one name space with tags', async () => {
  renderStep('Date,Amount,Marks\n2026-01-02,-5,Health;VACATION;trip\n2026-01-03,-5,Trip; fun\n', { ...noExtras, labels: 2 })
  expect(await screen.findByText('2 new tags will be created')).toBeInTheDocument()
})

it('offers the separator only for a labels column and keeps a custom one', async () => {
  renderStep('Date,Amount,Marks\n2026-01-02,-5,a/b\n')
  const user = userEvent.setup()
  expect(screen.queryByRole('radiogroup')).not.toBeInTheDocument()
  await user.selectOptions(screen.getByLabelText('Use “Marks” as'), 'labels')
  expect(screen.getByRole('radio', { name: ';' })).toBeChecked()
  await user.type(screen.getByLabelText('Something else'), '/')
  expect(extrasOf().labelsSeparator).toBe('/')
  expect(await screen.findByText('2 new tags will be created')).toBeInTheDocument()
})
