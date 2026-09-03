import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { ImportsDataPage } from './ImportsDataPage'

vi.mock('@/hooks/useIsCompact', () => ({ useIsCompact: () => false }))

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/settings/data', element: <ImportsDataPage /> }], { initialEntries: ['/settings/data'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it('renders the CSV rows and opens the import dialog', async () => {
  server.use(...coreHandlers())
  const user = userEvent.setup()
  renderPage()
  expect(await screen.findByText('Import & export')).toBeInTheDocument()
  expect(screen.getByText('Export CSV')).toBeInTheDocument()
  await user.click(screen.getByText('Import CSV'))
  expect(await screen.findByText('Maximum file size: 10 MB')).toBeInTheDocument()
})

it('holds no Apple Wallet section — that has its own page', async () => {
  server.use(...coreHandlers())
  renderPage()
  await screen.findByText('Import & export')
  expect(screen.queryByText('Apple Wallet')).not.toBeInTheDocument()
})
