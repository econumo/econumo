import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { acceptInvite, deleteConnection, generateInvite, getConnectionList, revokeAccountAccess, setAccountAccess } from './connection'

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  localStorage.setItem('token', 't')
})

const wireConnection = {
  user: { id: 'u2', avatar: 'https://a/u2', name: 'Partner' },
  sharedAccounts: [{ id: 'a1', ownerUserId: 'u1', role: 'user' }],
}

it('getConnectionList unwraps items', async () => {
  server.use(
    http.get('*/api/v1/connection/get-connection-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [wireConnection] } }),
    ),
  )
  expect(await getConnectionList()).toEqual([wireConnection])
})

it('generateInvite posts an empty body and unwraps item', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/connection/generate-invite', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { item: { code: 'aB3f9', expiredAt: '2026-07-03 12:05:00' } } })
    }),
  )
  expect(await generateInvite()).toEqual({ code: 'aB3f9', expiredAt: '2026-07-03 12:05:00' })
  expect(body).toEqual({})
})

it('acceptInvite posts the code and returns the refreshed list', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/connection/accept-invite', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { items: [wireConnection] } })
    }),
  )
  expect(await acceptInvite('aB3f9')).toEqual([wireConnection])
  expect(body).toEqual({ code: 'aB3f9' })
})

it('deleteConnection posts the user id under "id"', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/connection/delete-connection', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  await deleteConnection('u2')
  expect(body).toEqual({ id: 'u2' })
})

it('set/revoke account access post the exact payloads', async () => {
  const bodies: unknown[] = []
  server.use(
    http.post('*/api/v1/connection/set-account-access', async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
    http.post('*/api/v1/connection/revoke-account-access', async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  await setAccountAccess({ accountId: 'a1', userId: 'u2', role: 'user' })
  await revokeAccountAccess({ accountId: 'a1', userId: 'u2' })
  expect(bodies).toEqual([
    { accountId: 'a1', userId: 'u2', role: 'user' },
    { accountId: 'a1', userId: 'u2' },
  ])
})
