import { StrictMode } from 'react'
import { render, screen } from '@testing-library/react'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { RequireAuth } from './RequireAuth'
import { setToken } from '@/lib/storage'

function fakeJwt(payload: object): string {
  const b64 = (o: object) => btoa(JSON.stringify(o)).replace(/=+$/, '')
  return `${b64({ alg: 'RS256' })}.${b64(payload)}.sig`
}

function renderAt(path: string) {
  const router = createMemoryRouter(
    [
      { path: '/login', element: <div>LOGIN PAGE</div> },
      { element: <RequireAuth />, children: [{ path: '/', element: <div>SECRET</div> }] },
    ],
    { initialEntries: [path] },
  )
  // StrictMode double-renders, like the real app entry — it caught a
  // render-phase token purge losing the ?reason=expired redirect.
  render(
    <StrictMode>
      <RouterProvider router={router} />
    </StrictMode>,
  )
  return router
}

beforeEach(() => localStorage.clear())

it('renders the protected page with a valid token', () => {
  setToken(fakeJwt({ exp: Math.floor(Date.now() / 1000) + 3600 }))
  renderAt('/')
  expect(screen.getByText('SECRET')).toBeInTheDocument()
})

it('redirects to /login when there is no token', () => {
  const router = renderAt('/')
  expect(screen.getByText('LOGIN PAGE')).toBeInTheDocument()
  expect(router.state.location.search).toBe('')
})

it('redirects to /login?reason=expired and purges an expired token', () => {
  setToken(fakeJwt({ exp: Math.floor(Date.now() / 1000) - 60 }))
  const router = renderAt('/')
  expect(screen.getByText('LOGIN PAGE')).toBeInTheDocument()
  expect(router.state.location.search).toBe('?reason=expired')
  expect(localStorage.getItem('token')).toBeNull()
})
