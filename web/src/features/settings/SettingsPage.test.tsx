import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import i18n from '@/app/i18n'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { SettingsPage } from './SettingsPage'

function mockViewport(compact: boolean) {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: query.includes('1023') ? compact : false,
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }))
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/settings', element: <SettingsPage /> },
      { path: '/settings/budgets', element: <div>BUDGETS PAGE</div> },
    ],
    { initialEntries: ['/settings'] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return queryClient
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
})

afterEach(async () => {
  // the language-switch test leaves i18n on 'ru'; restore 'en' for later tests
  await i18n.changeLanguage('en')
  document.documentElement.lang = 'en'
})

it('renders the menu rows with exact labels and navigates', async () => {
  mockViewport(false)
  const user = userEvent.setup()
  renderPage()
  expect(await screen.findByText('Settings')).toBeInTheDocument()
  // Full sync moved to the sidebar footer refresh button
  expect(screen.queryByText('Full sync')).not.toBeInTheDocument()
  // grouped menu; currency moved to the profile page
  expect(screen.getByText('Finances')).toBeInTheDocument()
  expect(screen.getByText('Classification')).toBeInTheDocument()
  expect(screen.getByText('Data')).toBeInTheDocument()
  expect(screen.queryByText('Default currency')).not.toBeInTheDocument()
  expect(screen.getByText('Shared access')).toBeInTheDocument()
  expect(screen.getByText('Accounts')).toBeInTheDocument()
  expect(screen.getByText('Payees')).toBeInTheDocument()
  // en + ru locales are registered, so the Preferences/Language row shows
  expect(screen.getByText('Preferences')).toBeInTheDocument()
  expect(screen.getByText('Language')).toBeInTheDocument()

  await user.click(screen.getByText('Budgets'))
  expect(await screen.findByText('BUDGETS PAGE')).toBeInTheDocument()
})

it('Language row opens a dialog to switch locale and closes on pick', async () => {
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByText('Language'))
  expect(await screen.findByRole('dialog')).toBeInTheDocument()
  expect(screen.getByText('English')).toBeInTheDocument()
  expect(screen.getByText('Русский')).toBeInTheDocument()
  await user.click(screen.getByText('Русский'))
  await screen.findByText('Язык')
  expect(document.documentElement.lang).toBe('ru')
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})


it('shows a plain version label for non-release builds', async () => {
  window.econumoConfig = { VERSION: 'dev' }
  renderPage()
  const label = await screen.findByText('Econumo dev')
  expect(label.tagName).toBe('SPAN')
})

it('links a semver version to its release notes page', async () => {
  window.econumoConfig = { VERSION: 'v1.2.3' }
  renderPage()
  const label = await screen.findByText('Econumo v1.2.3')
  expect(label).toHaveAttribute('href', 'https://econumo.com/releases/v1.2.3/')
})

it('links to the API docs', async () => {
  window.econumoConfig = { API_URL: 'https://api.example.test' }
  renderPage()
  const link = await screen.findByRole('link', { name: 'API' })
  expect(link).toHaveAttribute('href', 'https://api.example.test/api/doc')
})

it('Import CSV and Export CSV rows open their dialogs', async () => {
  server.use(...coreHandlers())
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByText('Import CSV'))
  expect(await screen.findByText('Maximum file size: 10 MB')).toBeInTheDocument()
})
