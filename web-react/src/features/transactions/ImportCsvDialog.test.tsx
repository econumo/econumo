import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { ImportCsvDialog } from './ImportCsvDialog'

const CSV = 'Account,Date,Amount,Category,Description\nCash,2026-01-02,-5.5,Food,coffee\nBank,2026-01-03,100,Salary,pay\n'

function renderDialog(onComplete = vi.fn()) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <ImportCsvDialog open onClose={() => {}} onComplete={onComplete} />
    </QueryClientProvider>,
  )
  return { queryClient, onComplete }
}

async function uploadCsv(text = CSV, name = 'import.csv') {
  const user = userEvent.setup()
  const input = await screen.findByLabelText('CSV File')
  await user.upload(input, new File([text], name, { type: 'text/csv' }))
  return user
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  server.use(...coreHandlers())
})

it('parsing a file reveals the mapping UI with auto-detected columns and samples', async () => {
  renderDialog()
  expect(screen.getByText('Maximum file size: 10 MB')).toBeInTheDocument()
  await uploadCsv()
  expect(await screen.findByText(/Map the columns from your CSV file/)).toBeInTheDocument()
  expect(screen.getByText('import.csv')).toBeInTheDocument()

  const accountSelect = screen.getByLabelText('Account') as HTMLSelectElement
  expect(accountSelect.value).toBe('Account')
  const dateSelect = screen.getByLabelText('Date') as HTMLSelectElement
  expect(dateSelect.value).toBe('Date')
  const amountSelect = screen.getByLabelText('Amount') as HTMLSelectElement
  expect(amountSelect.value).toBe('Amount')
  // sample values decorate the options
  expect(screen.getAllByRole('option', { name: 'Account ("Cash")' }).length).toBeGreaterThan(0)
  expect(screen.getByRole('button', { name: 'Import' })).toBeEnabled()
})

it('rejects files above 10 MB', async () => {
  renderDialog()
  const user = userEvent.setup()
  const input = await screen.findByLabelText('CSV File')
  const big = new File([new Uint8Array(10485761)], 'big.csv', { type: 'text/csv' })
  await user.upload(input, big)
  expect(await screen.findByText('Maximum file size: 10 MB')).toBeInTheDocument()
  expect(screen.queryByText(/Map the columns/)).not.toBeInTheDocument()
})

it('switching account to an existing account shows writable accounts only', async () => {
  renderDialog()
  const user = await uploadCsv()
  await user.click(screen.getByRole('button', { name: 'toggle Account mode' }))
  const select = (await screen.findByLabelText('Account')) as HTMLSelectElement
  const labels = [...select.options].map((o) => o.textContent)
  expect(labels.some((l) => l?.startsWith('Cash ('))).toBe(true)
  // Import requires an account pick now
  expect(screen.getByRole('button', { name: 'Import' })).toBeDisabled()
  await user.selectOptions(select, 'a1')
  expect(screen.getByRole('button', { name: 'Import' })).toBeEnabled()
})

it('dual amount mode requires both inflow and outflow columns', async () => {
  renderDialog()
  const user = await uploadCsv('Account,Date,In,Out\nCash,2026-01-02,,5\n')
  await waitFor(() => expect((screen.getByLabelText('Amount (Inflow)') as HTMLSelectElement).value).toBe('In'))
  expect((screen.getByLabelText('Amount (Outflow)') as HTMLSelectElement).value).toBe('Out')
  expect(screen.getByRole('button', { name: 'Import' })).toBeEnabled()
  await user.selectOptions(screen.getByLabelText('Amount (Inflow)'), '')
  expect(screen.getByRole('button', { name: 'Import' })).toBeDisabled()
})

it('happy path: import posts once, invalidates queries, and reports the result', async () => {
  let posts = 0
  server.use(
    http.post('*/api/v1/transaction/import-transaction-list', () => {
      posts += 1
      return HttpResponse.json({ success: true, message: '', data: { imported: 2, skipped: 0, errors: {} } })
    }),
  )
  const onComplete = vi.fn()
  const { queryClient } = renderDialog(onComplete)
  const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
  const user = await uploadCsv()
  await screen.findByText(/Map the columns/)
  await user.click(screen.getByRole('button', { name: 'Import' }))
  await waitFor(() => expect(onComplete).toHaveBeenCalledWith({ imported: 2, failed: 0, errors: [] }))
  expect(posts).toBe(1)
  expect(invalidateSpy).toHaveBeenCalled()
})
