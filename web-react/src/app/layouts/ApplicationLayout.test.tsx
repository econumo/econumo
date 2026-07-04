import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { ApplicationLayout } from './ApplicationLayout'

function mockViewport(compact: boolean) {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: query.includes('1023') ? compact : false,
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }))
}

function renderShell(path: string) {
  const router = createMemoryRouter(
    [
      {
        element: <ApplicationLayout />,
        children: [
          { path: '/', element: <div>HOME CONTENT</div> },
          { path: '/account/:id', element: <div>ACCOUNT CONTENT</div> },
        ],
      },
    ],
    { initialEntries: [path] },
  )
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
})

it('shows the loading gate, then the sidebar tree with folder totals', async () => {
  mockViewport(false)
  renderShell('/')
  expect(screen.getByText('Loading details')).toBeInTheDocument()

  expect(await screen.findByText('Cash')).toBeInTheDocument()
  expect(screen.getByText('General')).toBeInTheDocument()
  expect(screen.getByText('Savings')).toBeInTheDocument()
  // hidden folder and its account are absent
  expect(screen.queryByText('Hidden')).not.toBeInTheDocument()
  expect(screen.queryByText('Under the mattress')).not.toBeInTheDocument()
  // mixed-currency folder total: 2000 USD + 90 EUR / 0.9 = 2100 USD
  expect(screen.getByText('2,100.00 $')).toBeInTheDocument()
  // single-currency folder total (and the matching account row): native
  expect(screen.getAllByText('100.50 $').length).toBeGreaterThanOrEqual(2)
  // user block + nav
  expect(screen.getByText('Ada')).toBeInTheDocument()
  expect(screen.getByText('Budget')).toBeInTheDocument()
  expect(screen.getByText('Settings')).toBeInTheDocument()
})

it('desktop shows sidebar and workspace together', async () => {
  mockViewport(false)
  renderShell('/account/a1')
  await waitFor(() => expect(screen.getByTestId('sidebar')).toBeInTheDocument())
  expect(screen.getByTestId('workspace')).toBeInTheDocument()
  expect(screen.getByText('ACCOUNT CONTENT')).toBeInTheDocument()
})

it('compact viewport shows only the sidebar at / and only the workspace elsewhere', async () => {
  mockViewport(true)
  renderShell('/')
  await waitFor(() => expect(screen.getByTestId('sidebar')).toBeInTheDocument())
  expect(screen.queryByTestId('workspace')).not.toBeInTheDocument()
})

it('compact viewport hides the sidebar on content routes', async () => {
  mockViewport(true)
  renderShell('/account/a1')
  await waitFor(() => expect(screen.getByTestId('workspace')).toBeInTheDocument())
  expect(screen.queryByTestId('sidebar')).not.toBeInTheDocument()
})
