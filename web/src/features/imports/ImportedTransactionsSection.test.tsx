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
  transactionCategoryId: 'cat-salary', transactionPayeeId: null,
}
// t9 is not downloaded on this device: the row is built from the suggestion itself
const notLocal: ImportTransactionSuggestionDto = {
  id: 'l2', transactionId: 't9', accountId: 'a1', type: 'expense', amount: '4.75', currency: 'USD', date: '2026-07-03 10:00:00',
  description: '', externalPayee: 'BLUE BOTTLE #1', categoryId: 'cat-food', payeeId: null,
  transactionCategoryId: null, transactionPayeeId: null,
}
// t8 is not downloaded either and already has a category: only the payee is suggested
const notLocalPayee: ImportTransactionSuggestionDto = {
  id: 'l3', transactionId: 't8', accountId: 'a1', type: 'expense', amount: '12', currency: 'USD', date: '2026-07-04 10:00:00',
  description: '', externalPayee: 'CORNER SHOP', categoryId: null, payeeId: 'p1',
  transactionCategoryId: 'cat-food', transactionPayeeId: null,
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

it('a transaction not on this device keeps its own category under a suggested payee', async () => {
  renderSection([notLocalPayee])
  const row = await screen.findByTestId('tx-t8')
  expect(within(row).getByText('Food')).toBeInTheDocument()
  expect(within(row).getByText('Grocer')).toBeInTheDocument()
  expect(within(row).queryByText('Uncategorized')).not.toBeInTheDocument()
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

const notFound = () => HttpResponse.json({ success: false, message: 'Import transaction suggestion not found', code: 0, errors: {} }, { status: 400 })

it.each([
  ['fill', 'fill-transaction-suggestion', 'Fill in', METRICS.IMPORT_TRANSACTION_SUGGESTION_FILL],
  ['dismiss', 'dismiss-transaction-suggestion', 'Dismiss', METRICS.IMPORT_TRANSACTION_SUGGESTION_DISMISS],
])('a %s refused for a suggestion no longer listed counts as done: the sheet closes, no error', async (_name, route, button, metric) => {
  let gone = false
  renderSection([notLocal])
  server.use(
    http.post(`*/api/v1/import/${route}`, () => {
      gone = true
      return notFound()
    }),
    listAfter(() => (gone ? [] : [notLocal])),
  )
  const user = userEvent.setup()
  await user.click(await screen.findByTestId('tx-t9'))
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByRole('button', { name: button }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  await waitFor(() => expect(screen.queryByTestId('tx-t9')).not.toBeInTheDocument())
  expect(vi.mocked(toast.error)).not.toHaveBeenCalled()
  expect(vi.mocked(trackEvent)).not.toHaveBeenCalledWith(metric)
})

it('a refused dismiss whose suggestion is still listed shows the error and keeps the sheet open', async () => {
  renderSection([notLocal])
  server.use(http.post('*/api/v1/import/dismiss-transaction-suggestion', notFound))
  const user = userEvent.setup()
  await user.click(await screen.findByTestId('tx-t9'))
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByRole('button', { name: 'Dismiss' }))
  await waitFor(() => expect(vi.mocked(toast.error)).toHaveBeenCalledWith('Import transaction suggestion not found'))
  expect(screen.getByRole('dialog')).toBeInTheDocument()
})

it('a refused fill whose suggestion is still listed shows the error and keeps the sheet open', async () => {
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

it.each([
  ['fill', 'fill-transaction-suggestion', 'Fill in', METRICS.IMPORT_TRANSACTION_SUGGESTION_FILL],
  ['dismiss', 'dismiss-transaction-suggestion', 'Dismiss', METRICS.IMPORT_TRANSACTION_SUGGESTION_DISMISS],
])('a %s refused with 402 shows the error and keeps the sheet open, even though the list is empty', async (_name, route, button, metric) => {
  let refused = false
  renderSection([notLocal])
  server.use(
    http.post(`*/api/v1/import/${route}`, () => {
      refused = true
      return HttpResponse.json({ success: false, message: 'Read-only access', code: 0, errors: {} }, { status: 402 })
    }),
    // a readonly user's list is empty: the refusal must not read as "gone"
    listAfter(() => (refused ? [] : [notLocal])),
  )
  const user = userEvent.setup()
  await user.click(await screen.findByTestId('tx-t9'))
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByRole('button', { name: button }))
  await waitFor(() => expect(vi.mocked(toast.error)).toHaveBeenCalledWith('Read-only access'))
  expect(screen.getByRole('dialog')).toBeInTheDocument()
  expect(vi.mocked(trackEvent)).not.toHaveBeenCalledWith(metric)
})

it('a fill refused with a coded fieldless 400 is an error even when the list no longer has the suggestion', async () => {
  let refused = false
  renderSection([notLocal])
  server.use(
    http.post('*/api/v1/import/fill-transaction-suggestion', () => {
      refused = true
      return HttpResponse.json({ success: false, message: 'Transaction is not available', code: 400, errors: {} }, { status: 400 })
    }),
    listAfter(() => (refused ? [] : [notLocal])),
  )
  const user = userEvent.setup()
  await user.click(await screen.findByTestId('tx-t9'))
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByRole('button', { name: 'Fill in' }))
  await waitFor(() => expect(vi.mocked(toast.error)).toHaveBeenCalledWith('Transaction is not available'))
  expect(screen.getByRole('dialog')).toBeInTheDocument()
  expect(vi.mocked(trackEvent)).not.toHaveBeenCalledWith(METRICS.IMPORT_TRANSACTION_SUGGESTION_FILL)
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
