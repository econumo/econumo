import { render } from '@testing-library/react'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { setToken } from '@/lib/storage'
import { LogoutPage } from './LogoutPage'

// Tracks real call order (not just "was called") so a refactor that moves
// resetAnalyticsIdentity relative to the logout event or the token removal
// fails these tests, even though each function still behaves normally.
const order: string[] = []

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return {
    ...actual,
    trackEvent: vi.fn((...args: Parameters<typeof actual.trackEvent>) => {
      order.push('trackEvent')
      return actual.trackEvent(...args)
    }),
  }
})

vi.mock('@/lib/analytics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/analytics')>()
  return {
    ...actual,
    resetAnalyticsIdentity: vi.fn(() => {
      order.push('resetAnalyticsIdentity')
      return actual.resetAnalyticsIdentity()
    }),
  }
})

vi.mock('@/lib/storage', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/storage')>()
  return {
    ...actual,
    removeToken: vi.fn(() => {
      order.push('removeToken')
      return actual.removeToken()
    }),
  }
})

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  order.length = 0
})

it('calls logout, purges the token and redirects to /login', async () => {
  const assign = vi.fn()
  Object.defineProperty(window, 'location', { value: { ...window.location, assign }, writable: true })
  let called = false
  server.use(
    http.post('*/api/v1/user/logout-user', () => {
      called = true
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  setToken('tok')
  localStorage.setItem('econumo.query-cache', '{"stale":"finances"}')
  const router = createMemoryRouter([{ path: '/logout', element: <LogoutPage /> }], { initialEntries: ['/logout'] })
  render(<RouterProvider router={router} />)
  await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('/login'))
  expect(called).toBe(true)
  expect(localStorage.getItem('token')).toBeNull()
  expect(localStorage.getItem('econumo.query-cache')).toBeNull()
  // The logout event must flush under the outgoing user's identity, so the
  // identity reset has to land strictly between the tracked event and the
  // token removal that follows it.
  expect(order).toEqual(['trackEvent', 'resetAnalyticsIdentity', 'removeToken'])
})
