import { StrictMode } from 'react'
import { act, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { RequireAuth } from './RequireAuth'
import { setToken } from '@/lib/storage'
import { takePostLoginRedirect } from '@/features/authserver/postLoginRedirect'
import { writeConfig } from '@/features/appLock/config'
import { initAppLock, resetAppLockForTests, unlock } from '@/features/appLock/lockStore'
import { setSecureContext, signIn, testConfig } from '@/features/appLock/testUtils'

function renderAt(path: string) {
  const router = createMemoryRouter(
    [
      { path: '/login', element: <div>LOGIN PAGE</div> },
      { element: <RequireAuth />, children: [{ path: '/', element: <div>SECRET</div> }] },
    ],
    { initialEntries: [path] },
  )
  render(
    <StrictMode>
      <RouterProvider router={router} />
    </StrictMode>,
  )
  return router
}

beforeEach(() => localStorage.clear())

it('renders the protected page with an opaque access token', () => {
  // Opaque tokens carry no client-readable expiry; presence is enough.
  // Server-side expiry surfaces as a 401 handled by the api client interceptor.
  setToken('eco_ses_3q2-8phN5aXWuVLbtRzGYPFJ0kcmD1jgAoSEwCiK7dU')
  renderAt('/')
  expect(screen.getByText('SECRET')).toBeInTheDocument()
})

it('redirects to /login when there is no token', () => {
  const router = renderAt('/')
  expect(screen.getByText('LOGIN PAGE')).toBeInTheDocument()
  expect(router.state.location.search).toBe('')
})

it('remembers a consent URL for after sign-in, and nothing else', () => {
  sessionStorage.clear()
  const consent = '/oauth/authorize?client_id=a&state=b'
  const router = createMemoryRouter(
    [
      { path: '/login', element: <div>LOGIN PAGE</div> },
      { element: <RequireAuth />, children: [{ path: '/oauth/authorize', element: <div>CONSENT</div> }, { path: '/', element: <div>SECRET</div> }] },
    ],
    { initialEntries: [consent] },
  )
  render(<RouterProvider router={router} />)
  expect(screen.getByText('LOGIN PAGE')).toBeInTheDocument()
  expect(takePostLoginRedirect()).toBe(consent)
})

it('does not remember an ordinary page', () => {
  sessionStorage.clear()
  renderAt('/')
  expect(takePostLoginRedirect()).toBe('/')
})

describe('the app lock', () => {
  beforeEach(() => {
    sessionStorage.clear()
    setSecureContext(true)
    resetAppLockForTests()
    window.matchMedia = ((query: string) => ({ matches: false, media: query, addEventListener: () => {}, removeEventListener: () => {} })) as unknown as typeof window.matchMedia
  })

  afterEach(() => {
    resetAppLockForTests()
    setSecureContext(false)
  })

  function renderLocked() {
    const router = createMemoryRouter(
      [{ element: <RequireAuth />, children: [{ path: '/', element: <div>SECRET</div> }] }],
      { initialEntries: ['/'] },
    )
    render(
      <QueryClientProvider client={new QueryClient()}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    )
  }

  it('shows the lock screen instead of the page', () => {
    signIn('u1')
    writeConfig(testConfig({ userId: 'u1' }))
    initAppLock()
    renderLocked()
    expect(screen.getByRole('heading', { name: 'Enter PIN' })).toBeInTheDocument()
    expect(screen.queryByText('SECRET')).not.toBeInTheDocument()
  })

  it('locks on a cold start over plain http too', () => {
    setSecureContext(false)
    signIn('u1')
    writeConfig(testConfig({ userId: 'u1' }))
    initAppLock()
    renderLocked()
    expect(screen.getByRole('heading', { name: 'Enter PIN' })).toBeInTheDocument()
    expect(screen.queryByText('SECRET')).not.toBeInTheDocument()
  })

  it("ignores another user's lock", () => {
    signIn('u1')
    writeConfig(testConfig({ userId: 'u2' }))
    initAppLock()
    renderLocked()
    expect(screen.getByText('SECRET')).toBeInTheDocument()
  })

  it('shows the page again once unlocked', () => {
    signIn('u1')
    writeConfig(testConfig({ userId: 'u1' }))
    initAppLock()
    renderLocked()
    act(() => unlock())
    expect(screen.getByText('SECRET')).toBeInTheDocument()
  })
})
