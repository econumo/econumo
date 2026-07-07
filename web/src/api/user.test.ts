import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import * as userApi from './user'

const user = {
  id: '01890000-0000-7000-8000-000000000001',
  name: 'Ada',
  email: 'ada@example.test',
  avatar: '',
  options: [],
  currency: 'USD',
  reportPeriod: 'month',
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('login posts username/password to login-user and unwraps data', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/login-user', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ user, token: 'jwt-token' })
    }),
  )
  const result = await userApi.login('ada@example.test', 'secret')
  expect(body).toEqual({ username: 'ada@example.test', password: 'secret' })
  expect(result.token).toBe('jwt-token')
  expect(result.user.name).toBe('Ada')
})

it('login rejects on 401 invalid credentials', async () => {
  server.use(
    http.post('*/api/v1/user/login-user', () =>
      HttpResponse.json({ success: false, message: 'Invalid credentials.', code: 0, errors: {} }, { status: 401 }),
    ),
  )
  await expect(userApi.login('ada@example.test', 'wrong')).rejects.toThrow()
})

it('register posts email/password/name to register-user', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/register-user', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { user } })
    }),
  )
  await userApi.register('ada@example.test', 'secret', 'Ada')
  expect(body).toEqual({ email: 'ada@example.test', password: 'secret', name: 'Ada' })
})

it('getUserData unwraps data.user', async () => {
  server.use(
    http.get('*/api/v1/user/get-user-data', () =>
      HttpResponse.json({ success: true, message: '', data: { user } }),
    ),
  )
  await expect(userApi.getUserData()).resolves.toEqual(user)
})

it('completeOnboarding returns the refreshed user', async () => {
  server.use(
    http.post('*/api/v1/user/complete-onboarding', () =>
      HttpResponse.json({ success: true, message: '', data: { user: { ...user, options: [{ name: 'onboarding', value: 'completed' }] } } }),
    ),
  )
  const result = await userApi.completeOnboarding()
  expect(result.options).toEqual([{ name: 'onboarding', value: 'completed' }])
})

it('remindPassword and resetPassword hit their endpoints', async () => {
  const calls: string[] = []
  server.use(
    http.post('*/api/v1/user/remind-password', () => {
      calls.push('remind')
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
    http.post('*/api/v1/user/reset-password', async ({ request }) => {
      calls.push('reset')
      expect(await request.json()).toEqual({ username: 'ada@example.test', code: '123456789012', password: 'newpass' })
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  await userApi.remindPassword('ada@example.test')
  await userApi.resetPassword('ada@example.test', '123456789012', 'newpass')
  expect(calls).toEqual(['remind', 'reset'])
})
