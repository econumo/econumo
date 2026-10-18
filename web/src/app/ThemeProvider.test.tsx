import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { useTheme } from 'next-themes'
import { THEME_STORAGE_KEY } from '@/lib/theme'
import { queryKeys } from './queryKeys'
import { ThemeProvider } from './ThemeProvider'
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

function ThemeControls() {
  const { setTheme } = useTheme()
  return <>{['system', 'light', 'dark'].map((theme) => <button key={theme} onClick={() => setTheme(theme)}>{theme}</button>)}</>
}

function renderTheme() {
  return render(<ThemeProvider><ThemeControls /></ThemeProvider>)
}

it('defaults to System and follows live OS changes', async () => {
  renderTheme()
  await waitFor(() => expect(document.documentElement).toHaveClass('light'))
  changeSystem(true)
  await waitFor(() => expect(document.documentElement).toHaveClass('dark'))
  expect(document.documentElement.style.colorScheme).toBe('dark')
  expect(document.querySelector('meta[name="theme-color"]')).toHaveAttribute('content', '#27282d')
  changeSystem(false)
  await waitFor(() => expect(document.documentElement).toHaveClass('light'))
})

it('restores a device override, saves it locally, and preserves it after a reload', async () => {
  localStorage.setItem(THEME_STORAGE_KEY, 'dark')
  const first = renderTheme()
  await waitFor(() => expect(document.documentElement).toHaveClass('dark'))
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'light' }))
  expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('light')
  changeSystem(true)
  expect(document.documentElement).toHaveClass('light')
  first.unmount()
  renderTheme()
  await waitFor(() => expect(document.documentElement).toHaveClass('light'))
  await user.click(screen.getByRole('button', { name: 'system' }))
  await waitFor(() => expect(document.documentElement).toHaveClass('dark'))
  expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('system')
})

it('falls back to System for invalid local values', async () => {
  localStorage.setItem(THEME_STORAGE_KEY, 'unsupported')
  systemDark = true
  renderTheme()
  await waitFor(() => expect(document.documentElement).toHaveClass('dark'))
  expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('system')
})

it('follows theme changes in another tab on the same device', async () => {
  localStorage.setItem(THEME_STORAGE_KEY, 'dark')
  renderTheme()
  await waitFor(() => expect(document.documentElement).toHaveClass('dark'))
  act(() => {
    localStorage.setItem(THEME_STORAGE_KEY, 'light')
    window.dispatchEvent(new StorageEvent('storage', { key: THEME_STORAGE_KEY, oldValue: 'dark', newValue: 'light' }))
  })
  await waitFor(() => expect(document.documentElement).toHaveClass('light'))
  expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('light')
})

it.each(['/login', '/register'])('keeps this device’s override on %s and across account changes', async (path) => {
  localStorage.setItem(THEME_STORAGE_KEY, 'dark')
  const cache = new QueryClient()
  cache.setQueryData(queryKeys.user, { id: 'first-user', options: [{ name: 'theme', value: 'light' }] })
  const router = createMemoryRouter([{
    element: <RootLayout />,
    children: [
      { element: <LoginLayout />, children: [
        { path: '/login', element: <div>Login form</div> },
        { path: '/register', element: <div>Registration form</div> },
      ] },
      { path: '/settings/profile', element: <ThemeControls /> },
    ],
  }], { initialEntries: [path] })
  render(<QueryClientProvider client={cache}><RouterProvider router={router} /></QueryClientProvider>)
  await waitFor(() => expect(document.documentElement).toHaveClass('dark'))
  await act(() => router.navigate(path === '/login' ? '/register' : '/login'))
  expect(document.documentElement).toHaveClass('dark')
  await act(() => router.navigate('/settings/profile'))
  act(() => { cache.setQueryData(queryKeys.user, { id: 'another-user', options: [] }) })
  expect(document.documentElement).toHaveClass('dark')
  expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('dark')
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'system' }))
  await act(() => router.navigate('/login'))
  expect(document.documentElement).toHaveClass('light')
  changeSystem(true)
  await waitFor(() => expect(document.documentElement).toHaveClass('dark'))
})
