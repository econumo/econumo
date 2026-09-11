import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { exchangeHandoff, getProviderList, startLogin, unlinkIdentity } from './oauth'

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('reads the provider list from the envelope', async () => {
  server.use(http.get('*/api/v1/oauth/get-provider-list', () =>
    HttpResponse.json({ success: true, message: '', data: [{ id: 'google', name: 'Google' }] })))
  expect(await getProviderList()).toEqual([{ id: 'google', name: 'Google' }])
})

it('start-login posts provider and client and returns the url with the flow secret', async () => {
  let body: unknown
  server.use(http.post('*/api/v1/oauth/start-login', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ success: true, message: '', data: { url: 'https://idp/authorize?x=1', flow: 'f1' } })
  }))
  expect(await startLogin('google', 'web')).toEqual({ url: 'https://idp/authorize?x=1', flow: 'f1' })
  expect(body).toEqual({ provider: 'google', client: 'web' })
})

it('exchange-handoff posts the code with the flow secret and a 401 does not redirect to /login?reason=expired', async () => {
  let body: unknown
  server.use(http.post('*/api/v1/oauth/exchange-handoff', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ token: 'eco_ses_x', user: { id: 'u1', options: [], accessLevel: 'full', accessUntil: '' } })
  }))
  const res = await exchangeHandoff('code', 'f1')
  expect(res.token).toBe('eco_ses_x')
  expect(body).toEqual({ code: 'code', flow: 'f1' })
  server.use(http.post('*/api/v1/oauth/exchange-handoff', () =>
    HttpResponse.json({ success: false, message: 'bad', code: 401, errors: {} }, { status: 401 })))
  localStorage.setItem('token', 'keep-me')
  await expect(exchangeHandoff('bad', 'f1')).rejects.toBeTruthy()
  expect(localStorage.getItem('token')).toBe('keep-me')
})

it('unlink posts the provider', async () => {
  let body: unknown
  server.use(http.post('*/api/v1/oauth/unlink-identity', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ success: true, message: '', data: {} })
  }))
  await unlinkIdentity('apple')
  expect(body).toEqual({ provider: 'apple' })
})
