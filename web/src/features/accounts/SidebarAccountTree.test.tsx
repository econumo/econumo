import { render, screen, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts } from '@/test/fixtures'
import { SidebarAccountTree } from './SidebarAccountTree'

function renderTree(compact: boolean) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q.includes('1023') ? compact : false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createMemoryRouter([{ path: '/', element: <SidebarAccountTree /> }], { initialEntries: ['/'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers({ accounts: fixtureAccounts.map((a) => (a.id === 'a2' ? { ...a, type: 3 } : a)) }))
})

it.each([false, true])('marks savings accounts with a Savings label (compact=%s)', async (compact) => {
  renderTree(compact)
  const bank = (await screen.findByTitle('Bank')).closest('li')!
  expect(within(bank).getByText('Savings')).toBeInTheDocument()
  const cash = screen.getByTitle('Cash').closest('li')!
  expect(within(cash).queryByText('Savings')).toBeNull()
})
