import { onlineManager } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { ImportRunPage } from './ImportRunPage'
import { SimpleFINHistoryPage } from './SimpleFINHistoryPage'

vi.mock('@/hooks/useIsCompact', () => ({ useIsCompact: () => false }))

const run = {
  id: 'r1', sourceId: 's2', provider: 'simplefin', status: 'partial', trigger: 'manual',
  importedCount: 1, matchedCount: 0, amountsUpdatedCount: 0, queuedCount: 1, skippedCount: 0, failedCount: 1,
  errors: [{ externalAccountId: 'ACT-SAV', message: 'unsupported currency' }], startedAt: '2026-09-07 10:00:00', finishedAt: '2026-09-07 10:00:02',
}
const bank = { id: 's2', provider: 'simplefin', name: 'Bank', status: 'active', createdAt: '2026-09-01 00:00:00', cards: [] }
const links = [
  { id: 'l1', externalAccountId: 'ACT-CHK', externalTransactionId: 'chk-1', transactionId: 't1', status: 'linked', externalPayee: 'Blue Bottle', externalAmount: '-12.5', externalCurrency: 'USD', externalPostedAt: '2026-09-06 09:00:00' },
  { id: 'l2', externalAccountId: 'ACT-CHK', externalTransactionId: 'chk-2', transactionId: '', status: 'linked', externalPayee: 'Payroll', externalAmount: '2500', externalCurrency: 'USD', externalPostedAt: '2026-09-05 09:00:00' },
  { id: 'l3', externalAccountId: 'ACT-SAV', externalTransactionId: 'sav-1', transactionId: '', status: 'queued', externalPayee: 'Transfer', externalAmount: '-100', externalCurrency: 'USD', externalPostedAt: '2026-09-04 09:00:00' },
]

function renderAt(path: string) {
  server.use(
    ...coreHandlers({ importRuns: [run], importSources: [bank] }),
    http.get('*/api/v1/import/get-run', ({ request }) =>
      new URL(request.url).searchParams.get('id') === 'r1'
        ? HttpResponse.json({ success: true, message: '', data: { item: run, links } })
        : HttpResponse.json({ success: false, message: 'Run not found', code: 400, errors: {} }, { status: 400 })),
  )
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createMemoryRouter(
    [{ path: '/settings/simplefin/history', element: <SimpleFINHistoryPage /> }, { path: '/settings/simplefin/history/:id', element: <ImportRunPage /> }],
    { initialEntries: [path] },
  )
  render(<QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider>)
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = { IMPORT_SIMPLEFIN: true }
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({ matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn() }))
})

it('SimpleFIN history lists its runs with status, counts and per-account errors, linking to the run', async () => {
  renderAt('/settings/simplefin/history')
  expect(await screen.findByText('Completed with errors')).toBeInTheDocument()
  expect(screen.getByText(/1 imported · 0 matched/)).toBeInTheDocument()
  expect(screen.getByText(/ACT-SAV: unsupported currency/)).toBeInTheDocument()
  expect(screen.getByRole('link', { name: /Completed with errors/ })).toHaveAttribute('href', '/settings/simplefin/history/r1')
})

it('run detail shows imported, tombstone and queued rows; only the tombstone opens', async () => {
  renderAt('/settings/simplefin/history/r1')
  expect(await screen.findByText('Blue Bottle')).toBeInTheDocument()
  expect(screen.getByText(/ACT-CHK · .* · Imported$/)).toBeInTheDocument()
  expect(screen.getByText('Payroll')).toHaveClass('line-through')
  expect(screen.getByText(/Deleted since$/)).toBeInTheDocument()
  expect(screen.getByText(/Waiting for review$/)).toBeInTheDocument()
  expect(screen.getByText('-12.5 USD')).toBeInTheDocument()
  expect(screen.getAllByRole('button')).toHaveLength(1)
  expect(screen.getByRole('button', { name: /Payroll/ })).toBeInTheDocument()
})

it('a deleted row can be imported again', async () => {
  let sent: unknown
  server.use(http.post('*/api/v1/import/reimport-link', async ({ request }) => {
    sent = await request.json()
    return HttpResponse.json({ success: true, message: '', data: { status: 'matched', eventId: 'e2' } })
  }))
  renderAt('/settings/simplefin/history/r1')
  await userEvent.click(await screen.findByRole('button', { name: /Payroll/ }))
  expect(await screen.findByText(/imported and deleted later. Import it again/)).toBeInTheDocument()
  expect(screen.getByText('Account')).toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: 'Import again' }))
  await waitFor(() => expect(sent).toEqual({ linkId: 'l2' }))
  await waitFor(() => expect(screen.queryByRole('button', { name: 'Import again' })).toBeNull())
})

it('a refused reimport shows the server message and keeps the dialog open', async () => {
  server.use(http.post('*/api/v1/import/reimport-link', () =>
    HttpResponse.json({ success: false, message: 'This transaction was not deleted.', code: 400, errors: {} }, { status: 400 })))
  renderAt('/settings/simplefin/history/r1')
  await userEvent.click(await screen.findByRole('button', { name: /Payroll/ }))
  await userEvent.click(await screen.findByRole('button', { name: 'Import again' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('This transaction was not deleted.')
  expect(screen.getByRole('button', { name: 'Import again' })).toBeEnabled()
})

it('an unknown run id shows the server error', async () => {
  renderAt('/settings/simplefin/history/nope')
  expect(await screen.findByRole('alert')).toHaveTextContent('Run not found')
})

it('offline with nothing loaded on this device, a run says so instead of an empty or loading page', async () => {
  onlineManager.setOnline(false)
  onTestFinished(() => onlineManager.setOnline(true))
  renderAt('/settings/simplefin/history/r1')
  expect(await screen.findByTestId('offline-unavailable')).toHaveTextContent('Not available offline')
})

it('offline with nothing loaded on this device, the SimpleFIN history says so instead of an empty or loading page', async () => {
  onlineManager.setOnline(false)
  onTestFinished(() => onlineManager.setOnline(true))
  renderAt('/settings/simplefin/history')
  expect(await screen.findByTestId('offline-unavailable')).toHaveTextContent('Not available offline')
})
