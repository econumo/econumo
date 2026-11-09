import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts, fixtureOwner, fixtureTransactions } from '@/test/fixtures'
import { mutate } from '@/features/pendingWrites/store'
import type { PendingOp } from '@/features/pendingWrites/types'
import { ReportTransactionsDialog } from './ReportTransactionsDialog'
import type { AccountsDrill } from './ReportTransactionsDialog'
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
  tx('t-partner', { description: 'partner groceries', accountId: 'a-shared', author: partner, date: '2026-07-06 10:00:00' }),
  tx('t-in', { description: 'moved in', type: 'transfer', accountId: 'a-shared', accountRecipientId: 'a1', categoryId: null, payeeId: null, date: '2026-07-07 10:00:00' }),
]

function renderDialog(props: { budgetId: string | null; row: AccountsDrill; from: string; to: string }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <ReportTransactionsDialog {...props} onClose={() => {}} />
    </QueryClientProvider>,
  )
}

const cash: AccountsDrill = { id: 'a1', kind: 'account', name: 'Cash' }

it('an account: its own transactions in the range, a transfer into it included', async () => {
  server.use(...coreHandlers({ accounts, transactions }))
  renderDialog({ budgetId: null, row: cash, from: '2026-07-01', to: '2026-08-01' })
  expect(await screen.findByText('coffee beans')).toBeInTheDocument()
  expect(screen.getByText('august groceries')).toBeInTheDocument()
  expect(screen.getByText('moved in')).toBeInTheDocument()
  expect(screen.queryByText('last year groceries')).toBeNull()
  expect(screen.queryByText('partner groceries')).toBeNull()
  expect(screen.getByRole('dialog', { name: /^Cash · Jul\s–\sAug 2026$/ })).toBeInTheDocument()
})

it('a set of accounts: every transaction touching one of them', async () => {
  server.use(...coreHandlers({ accounts, transactions }))
  renderDialog({ budgetId: 'b1', row: { id: 'range', kind: 'accounts', name: 'Family', accountIds: ['a1', 'a-shared'] }, from: '2026-07-01', to: '2026-07-01' })
  expect(await screen.findByText('coffee beans')).toBeInTheDocument()
  expect(screen.getByText('partner groceries')).toBeInTheDocument()
  expect(screen.queryByText('august groceries')).toBeNull()
})

it('a budget savings account: asks the budget transaction list for every month of the range', async () => {
  const asked: string[] = []
  server.use(
    ...coreHandlers({ accounts, transactions }),
    http.get('*/api/v1/budget/get-transaction-list', ({ request }) => {
      const q = new URL(request.url).searchParams
      asked.push(`${q.get('periodStart')}|${q.get('accountId')}`)
      const items = q.get('periodStart') === '2026-07-01'
        ? [{
            id: 'w1', author: partner, currencyId: 'cur-usd', amount: '12', description: 'partner saving',
            category: null, payee: null, tag: null, labelIds: [], spentAt: '2026-07-04 12:00:00',
          }]
        : []
      return HttpResponse.json({ success: true, message: '', data: { items } })
    }),
  )
  renderDialog({ budgetId: 'b1', row: { id: 'b-sav', kind: 'savings', name: 'Bob savings' }, from: '2026-07-01', to: '2026-09-01' })
  expect(await screen.findByText('partner saving')).toBeInTheDocument()
  await waitFor(() => expect(asked.sort()).toEqual(['2026-07-01|b-sav', '2026-08-01|b-sav', '2026-09-01|b-sav']))
})

it('a month that failed to load shows an error, not a shorter list', async () => {
  server.use(
    ...coreHandlers({ accounts, transactions }),
    http.get('*/api/v1/budget/get-transaction-list', ({ request }) => {
      const month = new URL(request.url).searchParams.get('periodStart')
      if (month === '2026-08-01') return HttpResponse.json({ success: false, message: 'no', code: 500, errors: {} }, { status: 500 })
      return HttpResponse.json({ success: true, message: '', data: { items: [{
        id: `w-${month}`, author: partner, currencyId: 'cur-usd', amount: '12', description: 'a saving',
        category: null, payee: null, tag: null, labelIds: [], spentAt: `${month?.slice(0, 7)}-04 12:00:00`,
      }] } })
    }),
  )
  renderDialog({ budgetId: 'b1', row: { id: 'b-sav', kind: 'savings', name: 'Bob savings' }, from: '2026-07-01', to: '2026-08-01' })
  expect(await screen.findByText('Something went wrong. Please try again.')).toBeInTheDocument()
  expect(screen.queryByText('a saving')).toBeNull()
})

it("a partner's savings row shows its signed amount and currency", async () => {
  server.use(
    ...coreHandlers({ accounts, transactions }),
    http.get('*/api/v1/budget/get-transaction-list', () => HttpResponse.json({ success: true, message: '', data: { items: [
      { id: 'w-in', author: partner, currencyId: 'cur-usd', amount: '100', direction: 'in', description: 'in', category: null, payee: null, tag: null, labelIds: [], spentAt: '2026-07-04 12:00:00' },
      { id: 'w-out', author: partner, currencyId: 'cur-usd', amount: '40', direction: 'out', description: 'out', category: null, payee: null, tag: null, labelIds: [], spentAt: '2026-07-03 12:00:00' },
    ] } })),
  )
  renderDialog({ budgetId: 'b1', row: { id: 'b-sav', kind: 'savings', name: 'Bob savings' }, from: '2026-07-01', to: '2026-07-01' })
  expect(await screen.findByText('100.00 $')).toBeInTheDocument()
  expect(screen.getByText('-40.00 $')).toBeInTheDocument()
})

it('a range lists the savings only the budget knows next to the local rows', async () => {
  server.use(
    ...coreHandlers({ accounts, transactions }),
    http.get('*/api/v1/budget/get-transaction-list', () => HttpResponse.json({ success: true, message: '', data: { items: [
      { id: 'w-s', author: partner, currencyId: 'cur-usd', amount: '100', direction: 'in', description: 'partner saving', category: null, payee: null, tag: null, labelIds: [], spentAt: '2026-07-04 12:00:00' },
    ] } })),
  )
  renderDialog({
    budgetId: 'b1',
    row: { id: 'range', kind: 'accounts', name: 'Family', accountIds: ['a1'], savingsIds: ['b-sav'] },
    from: '2026-07-01',
    to: '2026-07-01',
  })
  expect(await screen.findByText('partner saving')).toBeInTheDocument()
  expect(await screen.findByText('coffee beans')).toBeInTheDocument()
})

it('a transfer between two listed savings accounts is listed once', async () => {
  server.use(
    ...coreHandlers({ accounts, transactions }),
    http.get('*/api/v1/budget/get-transaction-list', () => HttpResponse.json({ success: true, message: '', data: { items: [
      { id: 'w-t', author: partner, currencyId: 'cur-usd', amount: '100', direction: 'in', description: 'between savings', category: null, payee: null, tag: null, labelIds: [], spentAt: '2026-07-04 12:00:00' },
    ] } })),
  )
  renderDialog({ budgetId: 'b1', row: { id: 'range', kind: 'accounts', name: 'Family', accountIds: [], savingsIds: ['s1', 's2'] }, from: '2026-07-01', to: '2026-07-01' })
  expect(await screen.findAllByText('between savings')).toHaveLength(1)
})

it('a savings drill leaves out a transaction whose delete is waiting and shows a waiting edit', async () => {
  const base = { key: 'k', attempted: false, status: 'waiting', retries: 0, notBefore: 0 }
  await mutate(() => [
    { ...base, key: 'k1', seq: 1, kind: 'tx-delete', txId: 'w-gone', previous: null },
    { ...base, key: 'k2', seq: 2, kind: 'tx-update', txId: 'w-edit', payload: {
      id: 'w-edit', type: 'expense', accountId: 'b-sav', accountRecipientId: null, amount: '12', amountRecipient: null,
      categoryId: null, description: 'after the edit', payeeId: null, tagId: null, labelIds: [], date: '2026-07-05 10:00:00',
    } },
  ] as PendingOp[])
  server.use(
    ...coreHandlers({ accounts, transactions }),
    http.get('*/api/v1/budget/get-transaction-list', () => HttpResponse.json({ success: true, message: '', data: { items: [
      { id: 'w-keep', author: partner, currencyId: 'cur-usd', amount: '1', description: 'kept', category: null, payee: null, tag: null, labelIds: [], spentAt: '2026-07-06 12:00:00' },
      { id: 'w-gone', author: partner, currencyId: 'cur-usd', amount: '2', description: 'deleted offline', category: null, payee: null, tag: null, labelIds: [], spentAt: '2026-07-05 12:00:00' },
      { id: 'w-edit', author: partner, currencyId: 'cur-usd', amount: '3', description: 'before the edit', category: null, payee: null, tag: null, labelIds: [], spentAt: '2026-07-04 12:00:00' },
    ] } })),
  )
  renderDialog({ budgetId: 'b1', row: { id: 'b-sav', kind: 'savings', name: 'Bob savings' }, from: '2026-07-01', to: '2026-07-01' })
  expect(await screen.findByText('kept')).toBeInTheDocument()
  expect(screen.queryByText('deleted offline')).toBeNull()
  expect(screen.getByText('after the edit')).toBeInTheDocument()
  expect(screen.queryByText('before the edit')).toBeNull()
})
