import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { onlineManager, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { toast } from 'sonner'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import type { ImportTransactionSuggestionDto } from '@/api/dto/imports'
import { METRICS, trackEvent } from '@/lib/metrics'
import { useImportTransactionSuggestions } from './queries'
import { ImportedTransactionsSection } from './ImportedTransactionsSection'

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))
vi.mock('@/lib/metrics', async (original) => ({ ...await original<typeof import('@/lib/metrics')>(), trackEvent: vi.fn() }))
vi.mock('@/hooks/useIsCompact', () => ({ useIsCompact: () => false }))

// t2 is a local transaction (income, Salary, no payee): only the payee is suggested
const onLocal: ImportTransactionSuggestionDto = {
  id: 'l1', transactionId: 't2', accountId: 'a1', type: 'income', amount: '500', currency: 'USD', date: '2026-07-01 08:00:00',
  description: '', externalPayee: 'ACME PAYROLL', categoryId: null, payeeId: 'p1',
}
// t9 is not downloaded on this device: the row is built from the suggestion itself
const notLocal: ImportTransactionSuggestionDto = {
  id: 'l2', transactionId: 't9', accountId: 'a1', type: 'expense', amount: '4.75', currency: 'USD', date: '2026-07-03 10:00:00',
  description: '', externalPayee: 'BLUE BOTTLE #1', categoryId: 'cat-food', payeeId: null,
}

function Section() {
  const { data = [] } = useImportTransactionSuggestions()
  return <ImportedTransactionsSection items={data} />
}

function renderSection(items: ImportTransactionSuggestionDto[]) {
  server.use(
    ...coreHandlers({ importTransactionSuggestions: items }),
    http.get('*/api/v1/import/get-transaction-import-list', () => HttpResponse.json({ success: true, message: '', data: { items: [] } })),
  )
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(<QueryClientProvider client={queryClient}><Section /></QueryClientProvider>)
}

const listAfter = (rest: () => ImportTransactionSuggestionDto[]) =>
  http.get('*/api/v1/import/get-transaction-suggestion-list', () => HttpResponse.json({ success: true, message: '', data: { items: rest() } }))

beforeEach(() => {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({ matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn() }))
  localStorage.clear()
  window.econumoConfig = {}
  vi.mocked(trackEvent).mockClear()
  vi.mocked(toast.error).mockClear()
})

it('renders one row per suggestion with the suggested category and payee and a Suggested note', async () => {
  renderSection([notLocal, onLocal])
  const remote = await screen.findByTestId('tx-t9')
  expect(within(remote).getByText('Food')).toBeInTheDocument()
  expect(within(remote).getByText('BLUE BOTTLE #1')).toBeInTheDocument()
  expect(within(remote).getByText('Suggested')).toBeInTheDocument()
  // the local transaction's own category stays; the suggested payee is put in place
  await waitFor(() => expect(within(screen.getByTestId('tx-t2')).getByText('Salary')).toBeInTheDocument())
  const local = screen.getByTestId('tx-t2')
  expect(within(local).getByText('Grocer')).toBeInTheDocument()
  expect(within(local).getByText('Suggested')).toBeInTheDocument()
})

it('tapping opens the sheet; Fill in posts fill-transaction-suggestion and closes', async () => {
  const filled: unknown[] = []
  renderSection([notLocal])
  server.use(
    http.post('*/api/v1/import/fill-transaction-suggestion', async ({ request }) => {
      filled.push(await request.json())
      return HttpResponse.json({ success: true, message: '', data: { updated: 1 } })
    }),
    listAfter(() => (filled.length ? [] : [notLocal])),
  )
  const user = userEvent.setup()
  await user.click(await screen.findByTestId('tx-t9'))
  const dialog = await screen.findByRole('dialog')
  // the suggested category is marked in the sheet
  expect(within(dialog).getByText('Suggested')).toBeInTheDocument()
  await user.click(within(dialog).getByRole('button', { name: 'Fill in' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(filled).toEqual([{ id: 'l2' }])
  expect(vi.mocked(trackEvent)).toHaveBeenCalledWith(METRICS.IMPORT_TRANSACTION_SUGGESTION_FILL)
  await waitFor(() => expect(screen.queryByTestId('tx-t9')).not.toBeInTheDocument())
})

it('✕ posts dismiss and closes the sheet', async () => {
  const dismissed: unknown[] = []
  renderSection([notLocal, onLocal])
  server.use(
    http.post('*/api/v1/import/dismiss-transaction-suggestion', async ({ request }) => {
      dismissed.push(await request.json())
      return HttpResponse.json({ success: true, message: '', data: { items: [onLocal] } })
    }),
    listAfter(() => (dismissed.length ? [onLocal] : [notLocal, onLocal])),
  )
  const user = userEvent.setup()
  await user.click(await screen.findByTestId('tx-t9'))
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByRole('button', { name: 'Dismiss' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(dismissed).toEqual([{ id: 'l2' }])
  expect(vi.mocked(trackEvent)).toHaveBeenCalledWith(METRICS.IMPORT_TRANSACTION_SUGGESTION_DISMISS)
  await waitFor(() => expect(screen.queryByTestId('tx-t9')).not.toBeInTheDocument())
  expect(screen.getByTestId('tx-t2')).toBeInTheDocument()
})

it('a suggestion already gone (404) counts as done: the sheet closes and the list refetches', async () => {
  let gone = false
  renderSection([notLocal])
  server.use(
    http.post('*/api/v1/import/fill-transaction-suggestion', () => {
      gone = true
      return HttpResponse.json({ success: false, message: 'Not found', code: 0, errors: {} }, { status: 404 })
    }),
    listAfter(() => (gone ? [] : [notLocal])),
  )
  const user = userEvent.setup()
  await user.click(await screen.findByTestId('tx-t9'))
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByRole('button', { name: 'Fill in' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  await waitFor(() => expect(screen.queryByTestId('tx-t9')).not.toBeInTheDocument())
  expect(vi.mocked(toast.error)).not.toHaveBeenCalled()
  expect(vi.mocked(trackEvent)).not.toHaveBeenCalledWith(METRICS.IMPORT_TRANSACTION_SUGGESTION_FILL)
})

it('a refused fill shows the error and keeps the sheet open', async () => {
  renderSection([notLocal])
  server.use(http.post('*/api/v1/import/fill-transaction-suggestion', () =>
    HttpResponse.json({ success: false, message: 'Category is archived', code: 400, errors: {} }, { status: 400 })))
  const user = userEvent.setup()
  await user.click(await screen.findByTestId('tx-t9'))
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByRole('button', { name: 'Fill in' }))
  await waitFor(() => expect(vi.mocked(toast.error)).toHaveBeenCalledWith('Category is archived'))
  expect(screen.getByRole('dialog')).toBeInTheDocument()
})

it('buttons disabled offline', async () => {
  renderSection([notLocal])
  const user = userEvent.setup()
  await user.click(await screen.findByTestId('tx-t9'))
  const dialog = await screen.findByRole('dialog')
  onlineManager.setOnline(false)
  onTestFinished(() => onlineManager.setOnline(true))
  await waitFor(() => expect(within(dialog).getByRole('button', { name: 'Fill in' })).toBeDisabled())
  expect(within(dialog).getByRole('button', { name: 'Dismiss' })).toBeDisabled()
})
