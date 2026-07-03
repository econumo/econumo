import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { api, apiUrl } from './client'
import { setToken, getToken } from '@/lib/storage'

const UUID_V7 = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('sends the standard headers on every request', async () => {
  let captured: Headers | undefined
  server.use(
    http.get('*/api/v1/ping', ({ request }) => {
      captured = request.headers
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  setToken('tok123')
  await api.get(apiUrl('/api/v1/ping'))
  expect(captured!.get('accept')).toBe('application/json')
  expect(captured!.get('authorization')).toBe('Bearer tok123')
  expect(captured!.get('x-timezone')).toBe(Intl.DateTimeFormat().resolvedOptions().timeZone)
  expect(captured!.get('x-request-id')).toMatch(UUID_V7)
})

it('omits Authorization when there is no token', async () => {
  let captured: Headers | undefined
  server.use(
    http.get('*/api/v1/ping', ({ request }) => {
      captured = request.headers
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  await api.get(apiUrl('/api/v1/ping'))
  expect(captured!.get('authorization')).toBeNull()
})

it('on 401 purges the token and redirects to /login?reason=expired', async () => {
  const assign = vi.fn()
  Object.defineProperty(window, 'location', {
    value: { ...window.location, assign },
    writable: true,
  })
  server.use(
    http.get('*/api/v1/secure', () =>
      HttpResponse.json({ success: false, message: 'Unauthorized', code: 0, errors: {} }, { status: 401 }),
    ),
  )
  setToken('expired-tok')
  await expect(api.get(apiUrl('/api/v1/secure'))).rejects.toThrow()
  expect(getToken()).toBeNull()
  expect(assign).toHaveBeenCalledWith('/login?reason=expired')
})

it('does NOT redirect on 401 from login-user (invalid credentials case)', async () => {
  const assign = vi.fn()
  Object.defineProperty(window, 'location', {
    value: { ...window.location, assign },
    writable: true,
  })
  server.use(
    http.post('*/api/v1/user/login-user', () =>
      HttpResponse.json({ success: false, message: 'Invalid credentials.', code: 0, errors: {} }, { status: 401 }),
    ),
  )
  await expect(api.post(apiUrl('/api/v1/user/login-user'), {})).rejects.toThrow()
  expect(assign).not.toHaveBeenCalled()
})
