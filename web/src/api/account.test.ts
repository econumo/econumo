import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import * as accountApi from './account'

const wireCurrency = { id: 'c1', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const wireOwner = { id: 'u1', avatar: '', name: 'Ada' }
const wireAccount = {
  id: 'a1',
  owner: wireOwner,
  folderId: 'f1',
  name: 'Cash',
  position: 0,
  currency: wireCurrency,
  balance: '100.5',
  type: 1,
  icon: 'account_balance',
  sharedAccess: [],
}
const wireCorrection = {
  id: 't1',
  author: wireOwner,
  type: 'income',
  accountId: 'a1',
  accountRecipientId: null,
  amount: '100.5',
  amountRecipient: '100.5',
  categoryId: null,
  description: '',
  payeeId: null,
  tagId: null,
  date: '2026-07-03 12:00:00',
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('getAccountList unwraps the envelope and passes decimal strings through', async () => {
  server.use(
    http.get('*/api/v1/account/get-account-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [wireAccount] } }),
    ),
  )
  const items = await accountApi.getAccountList()
  expect(items).toHaveLength(1)
  expect(items[0].balance).toBe('100.5')
})

it('passes large balances through without precision loss', async () => {
  server.use(
    http.get('*/api/v1/account/get-account-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [{ ...wireAccount, balance: '12345678901234567.89' }] } }),
    ),
  )
  const items = await accountApi.getAccountList()
  expect(items[0].balance).toBe('12345678901234567.89')
})

it('createAccount returns {item, transaction} with decimal strings passed through', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/account/create-account', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { item: wireAccount, transaction: wireCorrection } })
    }),
  )
  const result = await accountApi.createAccount({ id: 'op1', name: 'Cash', currencyId: 'c1', balance: '100.5', icon: 'account_balance', folderId: null })
  expect(body).toEqual({ id: 'op1', name: 'Cash', currencyId: 'c1', balance: '100.5', icon: 'account_balance', folderId: null })
  expect(result.item.balance).toBe('100.5')
  expect(result.transaction?.amount).toBe('100.5')
})

it('createAccount passes through a null correction transaction', async () => {
  server.use(
    http.post('*/api/v1/account/create-account', () =>
      HttpResponse.json({ success: true, message: '', data: { item: wireAccount, transaction: null } }),
    ),
  )
  const result = await accountApi.createAccount({ id: 'op1', name: 'Cash', currencyId: 'c1', balance: '0', icon: 'x', folderId: 'f1' })
  expect(result.transaction).toBeNull()
})

it('folder list returns isVisible as the wire int', async () => {
  server.use(
    http.get('*/api/v1/account/get-folder-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [{ id: 'f1', name: 'General', position: 0, isVisible: 1 }] } }),
    ),
  )
  const items = await accountApi.getFolderList()
  expect(items[0].isVisible).toBe(1)
})

it('orderAccountList posts the changes array', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/account/order-account-list', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { items: [wireAccount] } })
    }),
  )
  await accountApi.orderAccountList([{ id: 'a1', folderId: 'f2', position: 3 }])
  expect(body).toEqual({ changes: [{ id: 'a1', folderId: 'f2', position: 3 }] })
})

it('grant/accept/decline/revoke access post the exact payloads', async () => {
  const bodies: unknown[] = []
  server.use(
    http.post('*/api/v1/account/grant-access', async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
    http.post('*/api/v1/account/accept-access', async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
    http.post('*/api/v1/account/decline-access', async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
    http.post('*/api/v1/account/revoke-access', async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  await accountApi.grantAccess({ accountId: 'a1', userId: 'u2', role: 'user' })
  await accountApi.acceptAccess({ accountId: 'a1' })
  await accountApi.acceptAccess({ accountId: 'a1', folderId: 'f1' })
  await accountApi.declineAccess('a1')
  await accountApi.revokeAccess({ accountId: 'a1', userId: 'u2' })
  expect(bodies).toEqual([
    { accountId: 'a1', userId: 'u2', role: 'user' },
    { accountId: 'a1', folderId: '' },
    { accountId: 'a1', folderId: 'f1' },
    { accountId: 'a1' },
    { accountId: 'a1', userId: 'u2' },
  ])
})
