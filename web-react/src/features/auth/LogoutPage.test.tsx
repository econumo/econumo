import { render } from '@testing-library/react'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { setToken } from '@/lib/storage'
import { LogoutPage } from './LogoutPage'

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
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
  const router = createMemoryRouter([{ path: '/logout', element: <LogoutPage /> }], { initialEntries: ['/logout'] })
  render(<RouterProvider router={router} />)
  await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('/login'))
  expect(called).toBe(true)
  expect(localStorage.getItem('token')).toBeNull()
})
