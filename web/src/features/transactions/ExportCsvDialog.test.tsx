import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts } from '@/test/fixtures'
import { ExportCsvDialog } from './ExportCsvDialog'

const partner = { id: 'u2', avatar: 'https://avatars.test/partner', name: 'Partner' }

function renderDialog() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <ExportCsvDialog open onClose={() => {}} />
    </QueryClientProvider>,
  )
  return queryClient
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  URL.createObjectURL = vi.fn(() => 'blob:mock')
  URL.revokeObjectURL = vi.fn()
})

it('pre-selects only owned accounts and toggles select all', async () => {
  const withShared = [...fixtureAccounts, { ...fixtureAccounts[0], id: 'a-shared', name: 'Partner wallet', owner: partner }]
  server.use(...coreHandlers({ accounts: withShared }))
  const user = userEvent.setup()
  renderDialog()
  expect(await screen.findByText('Select accounts')).toBeInTheDocument()
  expect(await screen.findByRole('checkbox', { name: /Cash/ })).toBeChecked()
  expect(screen.getByRole('checkbox', { name: /Partner wallet/ })).not.toBeChecked()
  // shared account present -> owner names shown
  expect(screen.getAllByText('Partner').length).toBeGreaterThan(0)

  await user.click(screen.getByRole('button', { name: 'Select all' }))
  expect(screen.getByRole('checkbox', { name: /Partner wallet/ })).toBeChecked()
  expect(screen.getByRole('button', { name: 'Deselect all' })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Deselect all' }))
  expect(screen.getByRole('checkbox', { name: /Cash/ })).not.toBeChecked()
  expect(screen.getByRole('button', { name: 'Export' })).toBeDisabled()
})

it('export fetches the selected ids and downloads transactions-<date>.csv', async () => {
  let url: URL | undefined
  server.use(
    ...coreHandlers(),
    http.get('*/api/v1/transaction/export-transaction-list', ({ request }) => {
      url = new URL(request.url)
      return new HttpResponse('transaction_id\n', { headers: { 'Content-Type': 'text/csv; charset=UTF-8' } })
    }),
  )
  const clicks: string[] = []
  const originalClick = HTMLAnchorElement.prototype.click
  HTMLAnchorElement.prototype.click = function (this: HTMLAnchorElement) {
    clicks.push(this.download)
  }
  try {
    const user = userEvent.setup()
    renderDialog()
    await screen.findByRole('checkbox', { name: /Cash/ })
    await user.click(screen.getByRole('button', { name: 'Export' }))
    await waitFor(() => expect(clicks).toHaveLength(1))
    expect(clicks[0]).toMatch(/^transactions-\d{4}-\d{2}-\d{2}\.csv$/)
    // all fixture accounts are owned -> all pre-selected
    expect(url!.searchParams.get('accountId')).toBe('a1,a2,a3,a-hidden')
  } finally {
    HTMLAnchorElement.prototype.click = originalClick
  }
})

it('export failure shows the error dialog', async () => {
  server.use(
    ...coreHandlers(),
    http.get('*/api/v1/transaction/export-transaction-list', () =>
      HttpResponse.json({ success: false, message: 'boom', code: 0 }, { status: 500 }),
    ),
  )
  const user = userEvent.setup()
  renderDialog()
  await screen.findByRole('checkbox', { name: /Cash/ })
  await user.click(screen.getByRole('button', { name: 'Export' }))
  expect(await screen.findByText('Failed to export transactions')).toBeInTheDocument()
})
