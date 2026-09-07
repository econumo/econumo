import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { ImportQueueBanner } from './ImportQueueBanner'

const queued = (linkId: string) => ({
  linkId, sourceId: 's1', externalAccountId: 'Apple Card', accountId: '', payee: 'Blue Bottle', amount: '12.5',
  currency: 'USD', type: 'expense', postedAt: '2026-08-20 10:42:03', reason: 'unmapped',
})

function renderBanner(path = '/') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createMemoryRouter(
    [{ path: '/', element: <ImportQueueBanner /> }, { path: '/imports/queue', element: <ImportQueueBanner /> }],
    { initialEntries: [path] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('renders nothing while the queue is empty', async () => {
  server.use(...coreHandlers())
  renderBanner()
  await new Promise((r) => setTimeout(r, 20))
  expect(screen.queryByText(/waiting for review/)).toBeNull()
})

it('counts the queued rows, pluralised, and links to the queue', async () => {
  server.use(...coreHandlers({ importQueue: { queued: [queued('l1'), queued('l2')], skipped: [], failed: [] } }))
  renderBanner()
  expect(await screen.findByText('2 imported transactions are waiting for review')).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Review' })).toHaveAttribute('href', '/imports/queue')
})

it('stays hidden on the queue page itself', async () => {
  server.use(...coreHandlers({ importQueue: { queued: [queued('l1')], skipped: [], failed: [] } }))
  renderBanner('/imports/queue')
  await new Promise((r) => setTimeout(r, 20))
  expect(screen.queryByText(/waiting for review/)).toBeNull()
})
