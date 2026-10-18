import { act, render, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { CurrentUserDto } from '@/api/dto/user'
import { fixtureUser } from '@/test/fixtures'
import { queryKeys } from './queryKeys'
import { ThemeProvider } from './ThemeProvider'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { RootLayout } from './layouts/RootLayout'
import { LoginLayout } from './layouts/LoginLayout'

vi.mock('@/components/AppUpdateBlock', () => ({ AppUpdateBlock: () => null }))

let systemDark = false
let listeners: Set<(query: MediaQueryListEvent) => void>

beforeEach(() => {
  localStorage.clear()
  document.documentElement.className = ''
  document.querySelector('meta[name="theme-color"]')?.remove()
  const themeColor = document.createElement('meta')
  themeColor.name = 'theme-color'
  themeColor.content = '#ffffff'
  document.head.appendChild(themeColor)
  systemDark = false
  listeners = new Set()
  window.matchMedia = vi.fn().mockImplementation(() => ({
    get matches() { return systemDark },
    addListener: (listener: (query: MediaQueryListEvent) => void) => listeners.add(listener),
    removeListener: (listener: (query: MediaQueryListEvent) => void) => listeners.delete(listener),
  }))
})

function changeSystem(dark: boolean) {
  act(() => {
    systemDark = dark
    for (const listener of [...listeners]) listener({ matches: dark } as MediaQueryListEvent)
  })
}

function savedUser(theme?: string): CurrentUserDto {
  return { ...fixtureUser, hasPassword: true, options: theme ? [{ name: 'theme', value: theme }] : [] }
}

function renderTheme(user?: CurrentUserDto) {
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  if (user) cache.setQueryData(queryKeys.user, user)
  render(<QueryClientProvider client={cache}><ThemeProvider><div>App</div></ThemeProvider></QueryClientProvider>)
  return cache
}

it('defaults to System and follows OS changes without a user request', async () => {
  renderTheme()
  await waitFor(() => expect(document.documentElement).toHaveClass('light'))
  changeSystem(true)
  await waitFor(() => expect(document.documentElement).toHaveClass('dark'))
  expect(document.documentElement.style.colorScheme).toBe('dark')
  expect(document.querySelector('meta[name="theme-color"]')).toHaveAttribute('content', '#27282d')
  changeSystem(false)
  await waitFor(() => expect(document.documentElement).toHaveClass('light'))
})

it('uses the database preference over the startup cache and ignores OS changes for explicit choices', async () => {
  localStorage.setItem('econumo.theme', 'light')
  renderTheme(savedUser('dark'))
  await waitFor(() => expect(document.documentElement).toHaveClass('dark'))
  expect(localStorage.getItem('econumo.theme')).toBe('dark')
  changeSystem(true)
  changeSystem(false)
  expect(document.documentElement).toHaveClass('dark')
})

it('applies restored data, preference changes and a different user with no theme option', async () => {
  const cache = renderTheme()
  act(() => { cache.setQueryData(queryKeys.user, savedUser('dark')) })
  await waitFor(() => expect(document.documentElement).toHaveClass('dark'))
  act(() => { cache.setQueryData(queryKeys.user, savedUser('light')) })
  await waitFor(() => expect(document.documentElement).toHaveClass('light'))
  changeSystem(true)
  expect(document.documentElement).toHaveClass('light')
  act(() => { cache.setQueryData(queryKeys.user, { ...savedUser(), id: 'another-user' }) })
  await waitFor(() => expect(document.documentElement).toHaveClass('dark'))
  expect(localStorage.getItem('econumo.theme')).toBe('system')
})

it('falls back to System for unknown database values', async () => {
  systemDark = true
  renderTheme(savedUser('unsupported'))
  await waitFor(() => expect(document.documentElement).toHaveClass('dark'))
  expect(localStorage.getItem('econumo.theme')).toBe('system')
})

it('accepts a change from another tab without reasserting its stale cached preference', async () => {
  const cache = renderTheme(savedUser('dark'))
  await waitFor(() => expect(document.documentElement).toHaveClass('dark'))
  act(() => {
    localStorage.setItem('econumo.theme', 'light')
    window.dispatchEvent(new StorageEvent('storage', { key: 'econumo.theme', oldValue: 'dark', newValue: 'light' }))
  })
  await waitFor(() => expect(document.documentElement).toHaveClass('light'))
  expect(localStorage.getItem('econumo.theme')).toBe('light')
  expect(cache.getQueryData<CurrentUserDto>(queryKeys.user)?.options[0].value).toBe('dark')
})

it.each(['/login', '/register'])('uses the live system appearance on %s and restores the saved theme after sign-in', async (path) => {
  // Even a cached explicit preference from the last account must not override
  // the system appearance of the public sign-in and registration screens.
  localStorage.setItem('econumo.theme', 'dark')
  const cache = new QueryClient()
  cache.setQueryData(queryKeys.user, savedUser('dark'))
  const router = createMemoryRouter([{
    element: <RootLayout />,
    children: [
      { element: <LoginLayout />, children: [
        { path: '/login', element: <div>Login form</div> },
        { path: '/register', element: <div>Registration form</div> },
      ] },
      { path: '/settings/profile', element: <div>Profile</div> },
    ],
  }], { initialEntries: [path] })
  render(<QueryClientProvider client={cache}><RouterProvider router={router} /></QueryClientProvider>)
  await waitFor(() => expect(document.documentElement).toHaveClass('light'))
  expect(localStorage.getItem('econumo.theme')).toBe('dark')
  expect(localStorage.getItem('econumo.public-theme')).toBe('system')
  expect(cache.getQueryData<CurrentUserDto>(queryKeys.user)?.options[0].value).toBe('dark')
  changeSystem(true)
  await waitFor(() => expect(document.documentElement).toHaveClass('dark'))
  await act(() => router.navigate(path === '/login' ? '/register' : '/login'))
  changeSystem(false)
  await waitFor(() => expect(document.documentElement).toHaveClass('light'))
  await act(() => router.navigate('/settings/profile'))
  await waitFor(() => expect(document.documentElement).toHaveClass('dark'))
  expect(localStorage.getItem('econumo.theme')).toBe('dark')
})
