import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { analyticsAllowed } from '@/lib/analyticsPreference'
import * as userApi from './user'

const user = {
  id: '01890000-0000-7000-8000-000000000001',
  name: 'Ada',
  email: 'ada@example.test',
  avatar: '',
  options: [],
  currency: 'USD',
  reportPeriod: 'month',
  accessLevel: 'full',
  accessUntil: '',
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

it('updateAvatar posts icon/color to update-avatar and returns the refreshed user', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/update-avatar', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { user: { ...user, avatar: 'pets' } } })
    }),
  )
  const result = await userApi.updateAvatar('pets', 'teal')
  expect(body).toEqual({ icon: 'pets', color: 'teal' })
  expect(result.avatar).toBe('pets')
})

// Real translation, not a mock of it: these go through the actual
// options.find(...).value !== '0' logic in login()/getUserData() and assert
// the resulting mirror through analyticsAllowed(), so a "simplification"
// like switching to `=== '1'` (which would silently opt everyone out on any
// unrecognized/absent value) fails these tests.
describe('analytics preference mirror', () => {
  const cases: [string, { name: string; value: string | null }[], boolean][] = [
    ["analytics: '0'", [{ name: 'analytics', value: '0' }], false],
    ["analytics: '1'", [{ name: 'analytics', value: '1' }], true],
    ['no analytics entry', [], true],
  ]

  it.each(cases)('login with %s -> analyticsAllowed() is %s', async (_label, options, expected) => {
    server.use(
      http.post('*/api/v1/user/login-user', () =>
        HttpResponse.json({ user: { ...user, options }, token: 'jwt-token' }),
      ),
    )
    await userApi.login('ada@example.test', 'secret')
    expect(analyticsAllowed()).toBe(expected)
  })

  it.each(cases)('getUserData with %s -> analyticsAllowed() is %s', async (_label, options, expected) => {
    server.use(
      http.get('*/api/v1/user/get-user-data', () =>
        HttpResponse.json({ success: true, message: '', data: { user: { ...user, options } } }),
      ),
    )
    await userApi.getUserData()
    expect(analyticsAllowed()).toBe(expected)
  })
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
