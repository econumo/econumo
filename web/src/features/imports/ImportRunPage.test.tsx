import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { ImportRunListPage } from './ImportRunListPage'
import { ImportRunPage } from './ImportRunPage'

vi.mock('@/hooks/useIsCompact', () => ({ useIsCompact: () => false }))

const run = {
  id: 'r1', sourceId: 's2', provider: 'simplefin', status: 'partial', trigger: 'manual',
  importedCount: 1, matchedCount: 0, amountsUpdatedCount: 0, queuedCount: 1, skippedCount: 0, failedCount: 1,
  errors: [{ externalAccountId: 'ACT-SAV', message: 'unsupported currency' }], startedAt: '2026-09-07 10:00:00', finishedAt: '2026-09-07 10:00:02',
}
const links = [
  { id: 'l1', externalAccountId: 'ACT-CHK', externalTransactionId: 'chk-1', transactionId: 't1', status: 'linked', externalPayee: 'Blue Bottle', externalAmount: '-12.5', externalCurrency: 'USD', externalPostedAt: '2026-09-06 09:00:00' },
  { id: 'l2', externalAccountId: 'ACT-CHK', externalTransactionId: 'chk-2', transactionId: '', status: 'linked', externalPayee: 'Payroll', externalAmount: '2500', externalCurrency: 'USD', externalPostedAt: '2026-09-05 09:00:00' },
  { id: 'l3', externalAccountId: 'ACT-SAV', externalTransactionId: 'sav-1', transactionId: '', status: 'queued', externalPayee: 'Transfer', externalAmount: '-100', externalCurrency: 'USD', externalPostedAt: '2026-09-04 09:00:00' },
]

function renderAt(path: string) {
  server.use(
    ...coreHandlers({ importRuns: [run] }),
    http.get('*/api/v1/import/get-run', ({ request }) =>
      new URL(request.url).searchParams.get('id') === 'r1'
        ? HttpResponse.json({ success: true, message: '', data: { item: run, links } })
        : HttpResponse.json({ success: false, message: 'Run not found', code: 400, errors: {} }, { status: 400 })),
  )
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createMemoryRouter(
    [{ path: '/imports/runs', element: <ImportRunListPage /> }, { path: '/imports/runs/:id', element: <ImportRunPage /> }],
    { initialEntries: [path] },
  )
  render(<QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider>)
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('lists runs with status, counts and per-account errors, linking to the run', async () => {
  renderAt('/imports/runs')
  expect(await screen.findByText('Completed with errors')).toBeInTheDocument()
  expect(screen.getByText(/1 imported · 0 matched/)).toBeInTheDocument()
  expect(screen.getByText(/ACT-SAV: unsupported currency/)).toBeInTheDocument()
  expect(screen.getByRole('link', { name: /Completed with errors/ })).toHaveAttribute('href', '/imports/runs/r1')
})

it('run detail shows imported, tombstone and queued rows read-only', async () => {
  renderAt('/imports/runs/r1')
  expect(await screen.findByText('Blue Bottle')).toBeInTheDocument()
  expect(screen.getByText(/ACT-CHK · .* · Imported$/)).toBeInTheDocument()
  expect(screen.getByText('Payroll')).toHaveClass('line-through')
  expect(screen.getByText(/Deleted since$/)).toBeInTheDocument()
  expect(screen.getByText(/Waiting for review$/)).toBeInTheDocument()
  expect(screen.getByText('-12.5 USD')).toBeInTheDocument()
  expect(screen.queryByRole('button')).toBeNull()
})

it('an unknown run id shows the server error', async () => {
  renderAt('/imports/runs/nope')
  expect(await screen.findByRole('alert')).toHaveTextContent('Run not found')
})
