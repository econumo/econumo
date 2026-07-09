import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import * as transactionApi from './transaction'
import { getCategoryList } from './category'
import { getCurrencyRateList } from './currency'

const wireOwner = { id: 'u1', avatar: '', name: 'Ada' }
const wireTx = {
  id: 't1',
  author: wireOwner,
  type: 'expense',
  accountId: 'a1',
  accountRecipientId: null,
  amount: '9.99',
  amountRecipient: '9.99',
  categoryId: 'cat1',
  description: 'coffee',
  payeeId: null,
  tagId: null,
  date: '2026-07-01 09:30:00',
}
const wireAccount = {
  id: 'a1', owner: wireOwner, folderId: 'f1', name: 'Cash', position: 0,
  currency: { id: 'c1', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 },
  balance: '90.01', type: 1, icon: 'wallet', sharedAccess: [],
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('getTransactionList passes amount strings through', async () => {
  server.use(
    http.get('*/api/v1/transaction/get-transaction-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [wireTx] } }),
    ),
  )
  const items = await transactionApi.getTransactionList()
  expect(items[0].amount).toBe('9.99')
  expect(items[0].amountRecipient).toBe('9.99')
})

it('createTransaction returns {item, accounts} with decimal strings passed through', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    http.post('*/api/v1/transaction/create-transaction', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: wireTx, accounts: [wireAccount] } })
    }),
  )
  const form = {
    id: 'op-tx-1', type: 'expense' as const, accountId: 'a1', accountRecipientId: null,
    amount: '9.99', amountRecipient: null, categoryId: 'cat1', description: '', payeeId: null, tagId: null,
    date: '2026-07-01 09:30:00',
  }
  const result = await transactionApi.createTransaction(form)
  expect(body).toEqual(form)
  expect(result.item.amount).toBe('9.99')
  expect(result.accounts[0].balance).toBe('90.01')
})

it('deleteTransaction posts the id and returns the refreshed accounts', async () => {
  server.use(
    http.post('*/api/v1/transaction/delete-transaction', () =>
      HttpResponse.json({ success: true, message: '', data: { item: wireTx, accounts: [wireAccount] } }),
    ),
  )
  const result = await transactionApi.deleteTransaction('t1')
  expect(result.accounts[0].balance).toBe('90.01')
})

it('category list and currency rates smoke (envelope + rate pass-through)', async () => {
  server.use(
    http.get('*/api/v1/category/get-category-list', () =>
      HttpResponse.json({
        success: true, message: '',
        data: { items: [{ id: 'cat1', ownerUserId: 'u1', name: 'Food', position: 0, type: 'expense', icon: 'restaurant', isArchived: 0, createdAt: '2026-01-01 00:00:00', updatedAt: '2026-01-01 00:00:00' }] },
      }),
    ),
    http.get('*/api/v1/currency/get-currency-rate-list', () =>
      HttpResponse.json({
        success: true, message: '',
        data: { items: [{ currencyId: 'c2', baseCurrencyId: 'c1', rate: '1.08', updatedAt: '2026-07-01 00:00:00' }] },
      }),
    ),
  )
  const categories = await getCategoryList()
  expect(categories[0].isArchived).toBe(0)
  const rates = await getCurrencyRateList()
  expect(rates[0].rate).toBe('1.08')
})

it('exportTransactionList sends the comma-joined accountId param and resolves a Blob', async () => {
  let url: URL | undefined
  server.use(
    http.get('*/api/v1/transaction/export-transaction-list', ({ request }) => {
      url = new URL(request.url)
      return new HttpResponse('transaction_id,account_name\n', { headers: { 'Content-Type': 'text/csv; charset=UTF-8' } })
    }),
  )
  const blob = await transactionApi.exportTransactionList(['a1', 'a2'])
  expect(url!.searchParams.get('accountId')).toBe('a1,a2')
  expect(blob).toBeInstanceOf(Blob)
  expect(await blob.text()).toBe('transaction_id,account_name\n')
})

// jsdom multipart bodies don't survive the MSW interceptor (no Blob.stream),
// so inspect the FormData at the axios boundary instead of over the wire.
it('importTransactionList posts multipart form data and unwraps the result', async () => {
  const { api } = await import('./client')
  const postSpy = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true, message: '', data: { imported: 2, skipped: 0, errors: {} } } })
  try {
    const file = new File(['Account,Date,Amount\nCash,2026-01-02,-5\n'], 'chunk_0.csv', { type: 'text/csv' })
    const result = await transactionApi.importTransactionList(
      file,
      { account: 'Account', date: 'Date', amount: 'Amount', tag: null },
      { accountId: 'a1' },
    )
    expect(result).toEqual({ imported: 2, skipped: 0, errors: {} })
    const [url, body] = postSpy.mock.calls[0] as [string, FormData]
    expect(url).toContain('/api/v1/transaction/import-transaction-list')
    const sent = body.get('file') as File
    expect(sent.name).toBe('chunk_0.csv')
    expect(await sent.text()).toBe('Account,Date,Amount\nCash,2026-01-02,-5\n')
    expect(JSON.parse(body.get('mapping') as string)).toEqual({ account: 'Account', date: 'Date', amount: 'Amount', tag: null })
    expect(body.get('accountId')).toBe('a1')
    expect(body.get('date')).toBeNull()
  } finally {
    postSpy.mockRestore()
  }
})
