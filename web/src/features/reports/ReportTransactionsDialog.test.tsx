import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts, fixtureOwner, fixtureTransactions } from '@/test/fixtures'
import { ReportTransactionsDialog } from './ReportTransactionsDialog'
import { budgetSelector } from './reportRows'
import type { DrillRow } from './reportRows'
import { setViewport } from './reportsTestUtils'

beforeEach(() => setViewport(false))

const partner = { id: 'u2', avatar: 'face:sky', name: 'Bob' }
const accounts = [
  ...fixtureAccounts,
  { ...fixtureAccounts[0], id: 'a-shared', name: 'Bob cash', owner: partner, sharedAccess: [{ user: fixtureOwner, role: 'user' }] },
]
const tx = (id: string, over: Record<string, unknown>) => ({ ...fixtureTransactions[0], id, ...over })
const transactions = [
  ...fixtureTransactions,
  tx('t-aug', { description: 'august groceries', date: '2026-08-10 10:00:00' }),
  tx('t-old', { description: 'last year groceries', date: '2025-07-10 10:00:00' }),
  tx('t-other-cat', { description: 'not food', categoryId: 'cat-archived', date: '2026-07-05 10:00:00' }),
  tx('t-partner', { description: 'partner groceries', accountId: 'a-shared', author: partner, date: '2026-07-06 10:00:00' }),
]

function renderDialog(props: { budgetId: string | null; row: DrillRow; from: string; to: string; expected?: { amount: string; currencyId: string } }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <ReportTransactionsDialog scope={{ budgetId: props.budgetId }} row={props.row} from={props.from} to={props.to} expected={props.expected} onClose={() => {}} />
    </QueryClientProvider>,
  )
}

const food: DrillRow = { id: 'cat-food', kind: 'category', name: 'Food', side: 'expense' }

it('my accounts: lists my own matching transactions in the range', async () => {
  server.use(...coreHandlers({ accounts, transactions }))
  renderDialog({ budgetId: null, row: food, from: '2026-07-01', to: '2026-08-01' })
  expect(await screen.findByText('coffee beans')).toBeInTheDocument()
  expect(screen.getByText('august groceries')).toBeInTheDocument()
  expect(screen.queryByText('last year groceries')).toBeNull()
  expect(screen.queryByText('not food')).toBeNull()
  expect(screen.queryByText('partner groceries')).toBeNull()
  expect(screen.getByRole('dialog', { name: /^Food · Jul\s–\sAug 2026$/ })).toBeInTheDocument()
})

const deletedNote = /deleted accounts are counted in the report/

it('my accounts: no note while the listed transactions add up to the report figure', async () => {
  server.use(...coreHandlers({ accounts, transactions }))
  renderDialog({ budgetId: null, row: food, from: '2026-07-01', to: '2026-08-01', expected: { amount: '19.98', currencyId: 'cur-usd' } })
  expect(await screen.findByText('coffee beans')).toBeInTheDocument()
  expect(screen.queryByText(deletedNote)).toBeNull()
})

it('my accounts: a note when the report counts more than is listed (deleted accounts)', async () => {
  server.use(...coreHandlers({ accounts, transactions }))
  renderDialog({ budgetId: null, row: food, from: '2026-07-01', to: '2026-08-01', expected: { amount: '50', currencyId: 'cur-usd' } })
  expect(await screen.findByText('coffee beans')).toBeInTheDocument()
  expect(screen.getByText(deletedNote)).toBeInTheDocument()
})

it('my accounts: no note when listed rows are in another currency than the report', async () => {
  server.use(...coreHandlers({ accounts, transactions }))
  renderDialog({ budgetId: null, row: food, from: '2026-07-01', to: '2026-08-01', expected: { amount: '50', currencyId: 'cur-eur' } })
  expect(await screen.findByText('coffee beans')).toBeInTheDocument()
  expect(screen.queryByText(deletedNote)).toBeNull()
})

it('my accounts: a payee row matches by payee and side', async () => {
  server.use(...coreHandlers({ accounts, transactions }))
  renderDialog({ budgetId: null, row: { id: 'p1', kind: 'payee', name: 'Grocer', side: 'expense' }, from: '2026-07-01', to: '2026-07-01' })
  expect(await screen.findByText('coffee beans')).toBeInTheDocument()
  expect(screen.queryByText('august groceries')).toBeNull()
})

it('budget scope: asks the budget transaction list for every month of the range', async () => {
  const asked: string[] = []
  server.use(
    ...coreHandlers({ accounts, transactions }),
    http.get('*/api/v1/budget/get-transaction-list', ({ request }) => {
      const q = new URL(request.url).searchParams
      asked.push(`${q.get('periodStart')}|${q.get('categoryId')}|${q.get('tagId')}`)
      const items = q.get('periodStart') === '2026-07-01'
        ? [{
            id: 'w1', author: partner, currencyId: 'cur-usd', amount: '12', description: 'partner lunch',
            category: { id: 'cat-food', name: 'Food', icon: 'restaurant' }, payee: null, tag: null, labelIds: [],
            spentAt: '2026-07-04 12:00:00',
          }]
        : []
      return HttpResponse.json({ success: true, message: '', data: { items } })
    }),
  )
  renderDialog({ budgetId: 'b1', row: { ...food, parent: { id: 'tag1', kind: 'tag' } }, from: '2026-07-01', to: '2026-09-01' })
  expect(await screen.findByText('partner lunch')).toBeInTheDocument()
  await waitFor(() => expect(asked.sort()).toEqual([
    '2026-07-01|cat-food|tag1', '2026-08-01|cat-food|tag1', '2026-09-01|cat-food|tag1',
  ]))
})

it('budget selectors mirror the Budget page', () => {
  expect(budgetSelector({ id: 'e1', kind: 'envelope', name: '', side: 'expense' })).toEqual({ envelopeId: 'e1' })
  expect(budgetSelector({ id: 'tag1', kind: 'tag', name: '', side: 'expense' })).toEqual({ tagId: 'tag1' })
  expect(budgetSelector({ id: 'uncategorized', kind: 'category', name: '', side: 'expense' })).toEqual({ uncategorized: true })
  expect(budgetSelector({ id: 'c1', kind: 'income_category', name: '', side: 'income' })).toEqual({ income: true, categoryId: 'c1' })
  expect(budgetSelector({ id: 'e2', kind: 'income_envelope', name: '', side: 'income' })).toEqual({ income: true, envelopeId: 'e2' })
  expect(budgetSelector({ id: 'uncategorized', kind: 'income_category', name: '', side: 'income' })).toBeNull()
})
